package agent

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-co-op/gocron/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"time"
)

var GlobalAgent IAgent
var (
	Version string
	Config  string
	Dir     string
)

type IAgent interface {
	Start(ctx context.Context)
	Stop()
	Ready() bool
	GetConfig(key string) string
	GetConfigWithGlobal(key string, global bool) string
	ReportStat(stat string)
	ReportTraffic(traffic string)
	ReportTaskResult(taskId string, success bool, extra string)
	ReportLog(log string)

	UpdateJobCron(cronKey string)
}

type Agent struct {
	AgentId   string
	DB        *redis.Client
	Scheduler gocron.Scheduler
	Jobs      map[string]gocron.Job

	ready      bool
	subscribes map[string]*redis.PubSub
}
type TaskResult struct {
	Id      string `json:"id"`
	Success bool   `json:"success"`
	Extra   string `json:"extra"`
}

type Options struct {
	Addr     string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

func NewAgent(option Options, agentId string) *Agent {
	rdb := redis.NewClient(&redis.Options{
		Addr:     option.Addr,
		Username: option.Username,
		Password: option.Password,
		DB:       option.DB,
	})

	_, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		LogR.Fatal("redis 连接失败", zap.Error(err))
	}

	s, err := gocron.NewScheduler()
	if err != nil {
		LogR.Fatal("scheduler 初始化失败", zap.Error(err))
	}

	agent := Agent{
		AgentId:   agentId,
		DB:        rdb,
		Scheduler: s,

		ready: true,
	}

	return &agent
}

func (agent *Agent) Start(ctx context.Context) {
	agent.startJob()

	subscribe := agent.DB.Subscribe(ctx, "agent_task_"+agent.AgentId)
	agent.subscribes = map[string]*redis.PubSub{
		"agent_task_" + agent.AgentId: subscribe,
	}
	ch := subscribe.Channel()
	LogR.Info("agent started successfully")

	for {
		select {
		case message := <-ch:
			go func(msg *redis.Message) {
				var agentTask Task
				err := json.Unmarshal([]byte(msg.Payload), &agentTask)
				LogR.Debug(fmt.Sprintf("收到任务 %s [%s]", agentTask.Type, agentTask.Id), zap.String("task", msg.Payload))
				if err != nil {
					LogR.Error("反序列任务数据失败", zap.Error(err))
					return
				}
				taskHandler := TaskHandlers[agentTask.Type]
				if taskHandler != nil {
					agentTask.OriginData = []byte(msg.Payload)
					_, err := taskHandler(agentTask)
					if err != nil {
						LogR.Error("任务处理失败", zap.Error(err))
						agent.ReportTaskResult(agentTask.Id, false, err.Error())
						return
					}
				} else {
					LogR.Sugar().Errorf("没有 %s 类型的处理程序 ", agentTask.Type)
				}
			}(message)
		case <-ctx.Done():
			return
		}
	}
}

func (agent *Agent) Stop() {
	err := agent.Scheduler.Shutdown()
	if err != nil {
		Log.Error("scheduler shutdown fail", zap.Error(err))
	}
	for _, subscribe := range agent.subscribes {
		err := subscribe.Close()
		if err != nil {
			Log.Error("redis subscribe close fail", zap.Error(err))
		}
	}
	LogR.Info("agent stopped successfully")
	err = agent.DB.Close()
	if err != nil {
		Log.Error("redis close fail", zap.Error(err))
	}
}

func (agent *Agent) Ready() bool {
	return agent.ready
}

func (agent *Agent) GetConfig(key string) string {
	return agent.GetConfigWithGlobal(key, false)
}

func (agent *Agent) GetConfigWithGlobal(key string, global bool) string {
	redisKey := "agent_config"
	if !global {
		redisKey += ":" + agent.AgentId
	}
	cmd := agent.DB.HGet(context.Background(), redisKey, key)
	value, err := cmd.Result()
	if err != nil {
		LogR.Error(fmt.Sprintf("获取 %s 配置失败", key), zap.Error(err))
	}
	return value
}

func (agent *Agent) ReportStat(status string) {
	LogR.Debug("上报节点服务状态", zap.String("status", status))
	cmd := agent.DB.LPush(context.Background(), "agent_status:"+agent.AgentId, status)
	_, err := cmd.Result()
	if err != nil {
		LogR.Error("上报节点服务状态失败", zap.Error(err))
	}
}

func (agent *Agent) ReportTraffic(traffic string) {
	traffic = fmt.Sprintf(`{"time":%d,"traffic":"%s"}`, time.Now().UnixMilli(), traffic)
	LogR.Debug("上报节点流量", zap.String("traffic", traffic))
	cmd := agent.DB.LPush(context.Background(), "agent_traffic:"+agent.AgentId, traffic)
	_, err := cmd.Result()
	if err != nil {
		LogR.Error("上报节点流量失败", zap.Error(err))
	}
}

func (agent *Agent) ReportTaskResult(taskId string, success bool, extra string) {
	LogR.Debug(fmt.Sprintf("上报节点任务执行结果 [%s]", taskId), zap.String("taskId", taskId), zap.Bool("success", success), zap.String("extra", extra))
	taskResult := TaskResult{
		Id:      taskId,
		Success: success,
		Extra:   extra,
	}
	msg, err := json.Marshal(taskResult)
	if err != nil {
		LogR.Error("序列化任务执行结果失败", zap.Error(err))
		return
	}
	cmd := agent.DB.Publish(context.Background(), "agent_task_result_"+agent.AgentId, msg)
	_, err = cmd.Result()
	if err != nil {
		LogR.Error("上报节点任务执行结果失败", zap.Error(err))
	}
}

func (agent *Agent) ReportLog(log string) {
	Log.Debug("上报节点日志", zap.String("log", log))
	cmd := agent.DB.LPush(context.Background(), "agent_log:"+agent.AgentId, log)
	_, err := cmd.Result()
	if err != nil {
		Log.Error("上报节点日志失败", zap.Error(err))
	}
}

//<-----------------------------Job---------------------------------->

var JobDefinitions = map[string]any{
	"AGENT_REPORT_STAT_JOB":    ReportStatExecutor,
	"AGENT_REPORT_TRAFFIC_JOB": ReportTrafficExecutor,
}

func (agent *Agent) startJob() {
	agent.Jobs = make(map[string]gocron.Job)
	for name, function := range JobDefinitions {
		job := agent.createJob(name+"_CRON", function)
		agent.Jobs[name] = job
	}
	agent.Scheduler.Start()
}

func (agent *Agent) createJob(cronKey string, function any) gocron.Job {
	cron := agent.GetConfig(cronKey)
	job, err := agent.Scheduler.NewJob(
		gocron.CronJob(cron, false),
		gocron.NewTask(function),
	)
	if err != nil {
		LogR.Error("创建节点定时任务失败", zap.Error(err))
	}
	return job
}

func (agent *Agent) UpdateJobCron(cronKey string) {
	name := cronKey[:len(cronKey)-5]
	cron := agent.GetConfig(cronKey)
	job := agent.Jobs[name]
	job, err := agent.Scheduler.Update(job.ID(), gocron.CronJob(cron, false), gocron.NewTask(JobDefinitions[name]))
	if err != nil {
		LogR.Error("更新节点定时任务执行 Cron 失败", zap.Error(err))
	}
	agent.Jobs[name] = job
	LogR.Sugar().Infof("更新节点定时任务 %s Cron 至 %s 成功", name, cron)
}

//<-----------------------------utils---------------------------------->

func Sign(payload interface{}, secret string) string {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		Log.Sugar().Fatalf("sign payload fail: %s", err)
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payloadBytes)
	signature := h.Sum(nil)

	return hex.EncodeToString(signature)
}

func Decrypt(encryptedData string, key []byte) ([]byte, error) {
	encryptedBytes, err := hex.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("hex decoding error: %v", err)
	}

	iv := key[32:48]
	key = key[:32]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("AES cipher creation error: %v", err)
	}

	mode := cipher.NewCBCDecrypter(block, iv)

	plaintext := make([]byte, len(encryptedBytes))
	mode.CryptBlocks(plaintext, encryptedBytes)

	plaintext, err = pkcs7UnPad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("padding removal error: %v", err)
	}

	return plaintext, nil
}

func pkcs7UnPad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("pkcs7UnPad: data is empty")
	}
	unPadding := int(data[length-1])
	if unPadding > length || unPadding == 0 {
		return nil, fmt.Errorf("pkcs7UnPad: invalid padding")
	}
	for _, b := range data[length-unPadding:] {
		if int(b) != unPadding {
			return nil, fmt.Errorf("pkcs7UnPad: invalid padding")
		}
	}
	return data[:length-unPadding], nil
}

func ValidateLogLevel(logLevel string) error {
	if logLevel == "" {
		return nil
	}
	match := false
	for _, option := range LogLevels {
		if logLevel == option {
			match = true
			break
		}
	}
	if !match {
		return fmt.Errorf("invalid option: %s. Valid options are: %v", logLevel, LogLevels)
	}
	return nil
}

func ValidateLogDest(logDest string) error {
	if logDest == "" {
		return nil
	}
	match := false
	for _, option := range LogDestinations {
		if logDest == option {
			match = true
			break
		}
	}
	if !match {
		return fmt.Errorf("invalid option: %s. Valid options are: %v", logDest, LogDestinations)
	}
	return nil
}
