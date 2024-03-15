package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	probing "github.com/prometheus-community/pro-bing"
	"strings"
	"time"
)

type Task struct {
	Id         string
	Type       string
	OriginData []byte
}

type TaskHandleFunc func(task Task) (interface{}, error)

var TaskHandlers = map[string]TaskHandleFunc{
	"hello": func(task Task) (interface{}, error) {
		GlobalAgent.ReportTaskResult(task.Id, true, "hello")
		return "hello", nil
	},
	"config_change": handleConfigChange,
	"forward":       handleForwardTask,
	"shell":         handleShellTask,
	"ping":          handlePingTask,
	"report_stat": func(task Task) (interface{}, error) {
		ReportStatExecutor()
		GlobalAgent.ReportTaskResult(task.Id, true, "请检查日志中的状态报告")
		return nil, nil
	},
	"report_traffic": func(task Task) (interface{}, error) {
		ReportTrafficExecutor()
		GlobalAgent.ReportTaskResult(task.Id, true, "请检查日志中的流量报告")
		return nil, nil
	},
}

type ConfigChangeTask struct {
	Task
	Key   string
	Value string
}

func handleConfigChange(task Task) (interface{}, error) {
	var configChangeTask ConfigChangeTask
	err := json.Unmarshal(task.OriginData, &configChangeTask)
	if err != nil {
		return nil, err
	}
	configKey := configChangeTask.Key
	configValue := configChangeTask.Value
	if configKey[len(configKey)-5:] == "_CRON" {
		GlobalAgent.UpdateJobCron(configKey)
	}
	if configKey == "AGENT_GOST_CONFIG" && configValue != "" {
		err := writeGOSTConfig([]byte(configValue))
		if err != nil {
			return nil, err
		}
		err = restartGOST()
		if err != nil {
			return nil, err
		}
	}
	if configKey == "AGENT_LOG_LEVEL" && configValue != "" {
		ReInitLog(configValue)
	}
	return nil, nil
}

func handleForwardTask(task Task) (interface{}, error) {
	var forwardTask ForwardTask
	err := json.Unmarshal(task.OriginData, &forwardTask)
	if err != nil {
		return nil, err
	}

	handle := ForwardTaskHandlers[forwardTask.Action][forwardTask.Method]
	if handle == nil {
		return nil, fmt.Errorf("不支持的转发方式: %s - %s", forwardTask.Action, forwardTask.Method)
	}
	return handle(forwardTask)
}

type ShellTask struct {
	Task
	Shell    string
	Internal bool
}

func handleShellTask(task Task) (interface{}, error) {
	var shellTask ShellTask
	err := json.Unmarshal(task.OriginData, &shellTask)
	if err != nil {
		return nil, err
	}
	s := strings.Split(shellTask.Shell, " ")
	out := ShellExecutor(Shell{
		Command:  s[0],
		Args:     s[1:],
		Internal: shellTask.Internal,
	})
	GlobalAgent.ReportTaskResult(task.Id, true, base64.StdEncoding.EncodeToString(out))
	return string(out), nil
}

type PingTask struct {
	Task
	Host    string
	Count   int
	TimeOut int64
}

func handlePingTask(task Task) (interface{}, error) {
	var pingTask PingTask
	err := json.Unmarshal(task.OriginData, &pingTask)
	if err != nil {
		return nil, err
	}
	pinger, err := probing.NewPinger(pingTask.Host)
	if err != nil {
		return nil, err
	}
	pinger.Count = 1
	if pingTask.Count > 0 {
		pinger.Count = pingTask.Count
	}
	if pingTask.TimeOut > 0 {
		pinger.Timeout = time.Duration(pingTask.TimeOut) * time.Second
	}
	pinger.OnFinish = func(stats *probing.Statistics) {
		b, _ := json.Marshal(stats)
		GlobalAgent.ReportTaskResult(task.Id, true, base64.StdEncoding.EncodeToString(b))
	}
	if err := pinger.Run(); err != nil {
		return nil, err
	}
	return nil, nil
}
