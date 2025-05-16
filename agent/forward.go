package agent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"path/filepath"

	"go.uber.org/zap"
)

var gostConfigPath = "/etc/gost/config.json"
var realmConfigDir = "/etc/realm/configs"

type ForwardTask struct {
	Task
	Action     string
	Method     string
	Options    json.RawMessage
	ForwardId  string
	AgentPort  int
	TargetPort int
	Target     string
}

type ForwardTaskResult struct {
	AgentPort int `json:"agentPort"`
}

type ForwardTaskHandleFunc func(forwardTask ForwardTask) (interface{}, error)

var ForwardTaskHandlers = map[string]map[string]ForwardTaskHandleFunc{
	"add": {
		"IPTABLES": handleForwardTaskAddIptables,
		"GOST":     handleForwardTaskAddGOST,
		"REALM":    handleForwardTaskAddREALM,
	},
	"delete": {
		"IPTABLES": handleForwardTaskDeleteIptables,
		"GOST":     handleForwardTaskDeleteGOST,
		"REALM":    handleForwardTaskDeleteREALM,
	},
}

// <-----------------------------iptables---------------------------------->
func handleForwardTaskAddIptables(forwardTask ForwardTask) (interface{}, error) {
	agentPort := forwardTask.AgentPort
	SelectAvailablePort(&agentPort)

	LogR.Sugar().Debugf("使用 iptables 进行端口转发, %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	out := ShellExecutor(Shell{
		Command:  "iptables.sh",
		Args:     []string{"forward", strconv.Itoa(agentPort), forwardTask.Target, strconv.Itoa(forwardTask.TargetPort)},
		Internal: true,
	})
	if out == nil {
		return nil, fmt.Errorf("转发失败。查看日志了解详细信息")
	}
	LogR.Sugar().Debugf("转发成功. %d -> %s:%d \n %s", agentPort, forwardTask.Target, forwardTask.TargetPort, string(out))
	result := ForwardTaskResult{
		AgentPort: agentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))

	return forwardTask, nil
}

func handleForwardTaskDeleteIptables(forwardTask ForwardTask) (interface{}, error) {
	agentPort := forwardTask.AgentPort

	LogR.Sugar().Debugf("删除 iptables 端口转发, %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	out := ShellExecutor(Shell{
		Command:  "iptables.sh",
		Args:     []string{"delete", strconv.Itoa(agentPort)},
		Internal: true,
	})
	if out == nil {
		return nil, fmt.Errorf("删除转发失败。查看日志了解详细信息")
	}
	LogR.Sugar().Debugf("删除转发成功. %d -> %s:%d \n %s", agentPort, forwardTask.Target, forwardTask.TargetPort, string(out))
	result := ForwardTaskResult{
		AgentPort: agentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))

	return forwardTask, nil
}

//<-----------------------------iptables end---------------------------------->

// <-----------------------------GOST---------------------------------->
func handleForwardTaskAddGOST(forwardTask ForwardTask) (interface{}, error) {
	agentPort := forwardTask.AgentPort
	SelectAvailablePort(&agentPort)

	options := string(forwardTask.Options)
	// 替换options中的端口占位符 ForwardId-agentPort
	placeholder := fmt.Sprintf("%s-agentPort", forwardTask.ForwardId)
	options = strings.ReplaceAll(options, placeholder, fmt.Sprintf(":%d", agentPort))

	LogR.Sugar().Debugf("使用 GOST 进行端口转发, %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	if err := writeGOSTConfig([]byte(options)); err != nil {
		return nil, err
	}
	if err := restartGOST(); err != nil {
		return nil, err
	}

	LogR.Sugar().Debugf("转发成功. %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	result := ForwardTaskResult{
		AgentPort: agentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))
	return forwardTask, nil
}

func handleForwardTaskDeleteGOST(forwardTask ForwardTask) (interface{}, error) {
	options := forwardTask.Options
	if err := writeGOSTConfig(options); err != nil {
		return nil, err
	}
	if err := restartGOST(); err != nil {
		return nil, err
	}
	LogR.Sugar().Debugf("删除转发成功. %d -> %s:%d", forwardTask.AgentPort, forwardTask.Target, forwardTask.TargetPort)
	result := ForwardTaskResult{
		AgentPort: forwardTask.AgentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))
	return forwardTask, nil
}

func restartGOST() error {
	out := ShellExecutor(Shell{
		Command:  "systemctl",
		Args:     []string{"restart", "gost"},
		Internal: false,
	})
	if out == nil {
		return fmt.Errorf("重启GOST失败, 查看日志了解详细信息")
	}
	return nil
}

func getGOSTConfig() interface{} {
	_, err := os.Stat(gostConfigPath)
	if os.IsNotExist(err) {
		return nil
	}

	configFile, err := os.ReadFile(gostConfigPath)
	if err != nil {
		LogR.Error("获取GOST配置文件失败", zap.Error(err))
		return nil
	}

	var config interface{}
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		LogR.Error("解析GOST配置文件失败", zap.Error(err))
		return nil
	}
	return config
}

func writeGOSTConfig(config []byte) error {
	if _, err := os.Stat(gostConfigPath); os.IsNotExist(err) {
		path := strings.Split(gostConfigPath, "/")
		dir := strings.Join(path[:len(path)-1], "/")
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("创建GOST配置文件目录失败: %w", err)
		}
	}
	err := os.WriteFile(gostConfigPath, config, 0644)
	if err != nil {
		return fmt.Errorf("写入GOST配置文件失败: %w", err)
	}
	return nil
}

//<-----------------------------GOST end---------------------------------->

// <-----------------------------REALM---------------------------------->

func handleForwardTaskAddREALM(forwardTask ForwardTask) (interface{}, error) {
	agentPort := forwardTask.AgentPort
	SelectAvailablePort(&agentPort)


    optionsBytes := []byte(forwardTask.Options)
	
	// Check if forwardTask.AgentPort is not set (assuming 0 means not set)
	if forwardTask.AgentPort == 0 {
		// Parse options JSON
		var optionsJson map[string]interface{}
		if err := json.Unmarshal(optionsBytes, &optionsJson); err != nil {
			return nil, fmt.Errorf("unmarshal options failed: %w", err)
		}
		
		// Check if endpoints array exists and has at least one element
		endpoints, ok := optionsJson["endpoints"].([]interface{})
		if ok && len(endpoints) > 0 {
			// Get the first endpoint
			endpoint, ok := endpoints[0].(map[string]interface{})
			if ok {
				// Update the listen field
				endpoint["listen"] = fmt.Sprintf("0.0.0.0:%d", agentPort)
				// Update the endpoints array
				endpoints[0] = endpoint
				optionsJson["endpoints"] = endpoints
				
				// Marshal the updated options back to JSON
				newOptionsBytes, err := json.Marshal(optionsJson)
				if err != nil {
					return nil, fmt.Errorf("marshal updated options failed: %w", err)
				}
				optionsBytes = newOptionsBytes
			}
		}
	}
	
	LogR.Sugar().Debugf("使用 Realm 进行端口转发, %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	if err := writeREALMConfig([]byte(optionsBytes), forwardTask.ForwardId); err != nil {
		return nil, err
	}
	if err := restartREALM(); err != nil {
		return nil, err
	}

	LogR.Sugar().Debugf("转发成功. %d -> %s:%d", agentPort, forwardTask.Target, forwardTask.TargetPort)
	result := ForwardTaskResult{
		AgentPort: agentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))
	return forwardTask, nil
}

func handleForwardTaskDeleteREALM(forwardTask ForwardTask) (interface{}, error) {
	configFilePath := fmt.Sprintf("%s/%s.json", realmConfigDir, forwardTask.ForwardId)
	if err := os.Remove(configFilePath); err != nil {
		return nil, fmt.Errorf("删除REALM配置文件失败: %w", err)
	}

	if err := restartREALM(); err != nil {
		return nil, err
	}
	
	LogR.Sugar().Debugf("删除转发成功. %d -> %s:%d", forwardTask.AgentPort, forwardTask.Target, forwardTask.TargetPort)
	result := ForwardTaskResult{
		AgentPort: forwardTask.AgentPort,
	}
	resultJson, _ := json.Marshal(result)
	GlobalAgent.ReportTaskResult(forwardTask.Id, true, base64.StdEncoding.EncodeToString(resultJson))
	return forwardTask, nil
}

func restartREALM() error {
	out := ShellExecutor(Shell{
		Command:  "systemctl",
		Args:     []string{"restart", "realm"},
		Internal: false,
	})
	if out == nil {
		return fmt.Errorf("重启Realm失败, 查看日志了解详细信息")
	}
	return nil
}

func writeREALMConfig(config []byte, forwardId string) error {
    // 确保目录存在
    if _, err := os.Stat(realmConfigDir); os.IsNotExist(err) {
        if err := os.MkdirAll(realmConfigDir, 0755); err != nil {
            return fmt.Errorf("创建REALM配置文件目录失败: %w", err)
        }
    }

    // 把 rawJSON 缩进格式化
    var configBuf bytes.Buffer
    if err := json.Indent(&configBuf, config, "", "  "); err != nil {
        return fmt.Errorf("JSON 格式化失败: %w", err)
    }

    // 写文件
    configFilePath := filepath.Join(realmConfigDir, forwardId+".json")
    if err := os.WriteFile(configFilePath, configBuf.Bytes(), 0644); err != nil {
        return fmt.Errorf("写入REALM配置文件失败: %w", err)
    }
    return nil
}

//<-----------------------------REALM end---------------------------------->

func SelectAvailablePort(port *int) {
	if *port == 0 {
		*port = GenerateUnusedPort()
		LogR.Sugar().Debugf("未指定端口, 使用随机端口: %d", *port)
	} else {
		if PortIsUsed(*port) {
			LogR.Sugar().Debugf("端口 %d 已被占用, 使用随机端口: %d", *port, *port)
			*port = GenerateUnusedPort()
		}
	}
}

func GenerateUnusedPort() int {
	port := GetRandomPort()
	for {
		if !PortIsUsed(port) {
			break
		}
		port = GetRandomPort()
	}
	return port
}

func GetRandomPort() int {
	minPort := 1024
	maxPort := 49151
	portRange := GlobalAgent.GetConfig("AGENT_PORT_RANGE")
	if portRange != "" {
		r := strings.Split(portRange, "-")
		if len(r) == 2 {
			minPort, _ = strconv.Atoi(r[0])
			maxPort, _ = strconv.Atoi(r[1])
		}
	}
	return rand.Intn(maxPort-minPort+1) + minPort
}

func PortIsUsed(port int) bool {
	cmd := exec.Command("netstat", "-tuln", "|", "grep", ":"+strconv.Itoa(port))
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error running netstat command:", err)
		return false
	}
	if strings.Contains(string(output), ":"+strconv.Itoa(port)) {
		LogR.Sugar().Debugf("端口 %d 已被占用", port)
		return true
	}
	// 如果没有被占用，返回false
	return false
}

func AddPortTrafficMonitor(localPort int, remoteHost string, remotePort int) {
	out := ShellExecutor(Shell{
		Command:  "iptables.sh",
		Args:     []string{"monitor", strconv.Itoa(localPort), remoteHost, strconv.Itoa(remotePort)},
		Internal: true,
	})

	if out == nil {
		LogR.Sugar().Debugf("添加端口流量监控失败. %d -> %s:%d", localPort, remoteHost, remotePort)
	} else {
		LogR.Sugar().Debugf("添加端口流量监控成功. %d -> %s:%d", localPort, remoteHost, remotePort)
	}
}

func DeletePortTrafficMonitor(localPort int) {
	out := ShellExecutor(Shell{
		Command:  "iptables.sh",
		Args:     []string{"delete", strconv.Itoa(localPort)},
		Internal: true,
	})

	if out == nil {
		LogR.Sugar().Debugf("删除端口流量监控失败. %d", localPort)
	} else {
		LogR.Sugar().Debugf("删除端口流量监控成功. %d", localPort)
	}
}
