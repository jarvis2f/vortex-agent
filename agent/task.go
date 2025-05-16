package agent

import (
	"context"
	"net"
	"os/exec"
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
	AgentPort int
	ForwardMethod string
}

func checkServiceStatusByPort(port string, expectedService string) (bool, string) {
    cmd := exec.Command("ss", "-tunlp", "|", "grep", ":"+port)
    output, err := cmd.CombinedOutput()
    
    if err != nil {
        cmd = exec.Command("bash", "-c", "ss -tunlp | grep :"+port)
        output, err = cmd.CombinedOutput()
        
        if err != nil {
            return false, fmt.Sprintf("检查端口 %s 失败: %v", port, err)
        }
    }
    
    
    outputStr := string(output)
    if outputStr == "" {
        return false, fmt.Sprintf("端口 %s 未被任何服务使用", port)
    }
    
    isActive := strings.Contains(strings.ToLower(outputStr), strings.ToLower(expectedService))
    
    return isActive, outputStr
}

func tcpPing(ctx context.Context, host, port string, timeoutMs int) (float64, error) {
    dialCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
    defer cancel()

    start := time.Now()
    conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", net.JoinHostPort(host, port))
    elapsed := float64(time.Since(start).Microseconds()) / 1000.0
    if err != nil {
        return elapsed, err
    }
    conn.Close()
    return elapsed, nil
}

func handlePingTask(task Task) (interface{}, error) {
	var pingTask PingTask
	err := json.Unmarshal(task.OriginData, &pingTask)
	if err != nil {
		return nil, err
	}

	// 拆 host:port（默认 80）
    host, port, err := net.SplitHostPort(pingTask.Host)
	
	if err != nil {
        host = pingTask.Host
        port = "80"
    }

	pinger, err := probing.NewPinger(host)
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

	// 服务状态
	type serviceStatus struct {
		IsActive bool   `json:"is_active"`
		Details  string `json:"details"`
	}
    // 最终结果
    type combinedResult struct {
        ICMP probing.Statistics `json:"icmp"`
        TCP  []float64          `json:"tcp_rtts_ms"`
		ServiceStatus serviceStatus `json:"service_status"`
    }

	pinger.OnFinish = func(stats *probing.Statistics) {

		count := pingTask.Count
        if count <= 0 {
            count = 1
        }

        timeoutMs := int(pingTask.TimeOut * 1000) // 转换为毫秒

		// 收集 TCP RTT
        var rtts []float64
        for i := 0; i < count; i++ {
            if rtt, err := tcpPing(context.Background(), host, port, timeoutMs); err == nil {
                rtts = append(rtts, rtt)
            }
        }

		// 检查服务状态
		var svcStatus serviceStatus
		if pingTask.ForwardMethod == "REALM" || pingTask.ForwardMethod == "GOST" {
			expectedService := strings.ToLower(pingTask.ForwardMethod)
			portStr := fmt.Sprintf("%d", pingTask.AgentPort)
			isActive, details := checkServiceStatusByPort(portStr, expectedService)
			
			svcStatus = serviceStatus{
				IsActive: isActive,
				Details:  details,
			}
		}

        combined := combinedResult{
            ICMP: *stats,
            TCP:  rtts,
			ServiceStatus: svcStatus,
        }

		b, _ := json.Marshal(&combined)

		GlobalAgent.ReportTaskResult(task.Id, true, base64.StdEncoding.EncodeToString(b))
	}
	if err := pinger.Run(); err != nil {
		return nil, err
	}
	return nil, nil
}
