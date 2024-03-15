package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.uber.org/zap"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	lastUpdateTime uint64
)

type CPUInfo struct {
	ModelName string  `json:"modelName"`
	Cores     int32   `json:"cores"`
	Mhz       float64 `json:"mhz"`
}

func ReportStatExecutor() {
	var info map[string]interface{}
	var stats map[string]interface{}

	dif := uint64(time.Now().UnixMilli()) - lastUpdateTime
	if dif > 86400000 {
		h, err := host.Info()
		if err != nil {
			LogR.Error("get host info fail", zap.Error(err))
			return
		}
		cpuInfo := make(map[string]CPUInfo)
		ci, err := cpu.Info()
		if err == nil {
			for _, c := range ci {
				if _, ok := cpuInfo[c.ModelName]; !ok {
					cpuInfo[c.ModelName] = CPUInfo{
						ModelName: c.ModelName,
						Cores:     c.Cores,
						Mhz:       c.Mhz,
					}
				} else {
					cpuInfo[c.ModelName] = CPUInfo{
						ModelName: c.ModelName,
						Cores:     cpuInfo[c.ModelName].Cores + c.Cores,
						Mhz:       math.Max(cpuInfo[c.ModelName].Mhz, c.Mhz),
					}
				}
			}
		}
		var cpuInfoSlice []CPUInfo
		for _, v := range cpuInfo {
			cpuInfoSlice = append(cpuInfoSlice, v)
		}
		ipInfo, err := getIpInfo()
		if err != nil {
			LogR.Error("get ip info fail", zap.Error(err))
		}

		info = map[string]interface{}{
			"host": h,
			"cpu":  cpuInfoSlice,
			"ip": map[string]interface{}{
				"ipv4":    ipInfo.IPv4,
				"ipv6":    ipInfo.IPv6,
				"country": ipInfo.CountryISO,
			},
			"version": Version,
		}
		lastUpdateTime = uint64(time.Now().UnixMilli())
	}
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		LogR.Error("get cpu percent fail", zap.Error(err))
	}

	var firstInnerNetInTransfer, firstInnerNetOutTransfer uint64
	firstTime := time.Now()
	nc, err := net.IOCounters(true)
	if err == nil {
		for _, v := range nc {
			firstInnerNetInTransfer += v.BytesRecv
			firstInnerNetOutTransfer += v.BytesSent
		}
	}
	time.Sleep(1 * time.Second)
	var secondInnerNetInTransfer, secondInnerNetOutTransfer uint64
	secondTime := time.Now()
	nc, err = net.IOCounters(true)
	if err == nil {
		for _, v := range nc {
			secondInnerNetInTransfer += v.BytesRecv
			secondInnerNetOutTransfer += v.BytesSent
		}
	}
	var netInSpeed, netOutSpeed float64
	if firstInnerNetInTransfer != 0 && firstInnerNetOutTransfer != 0 && secondInnerNetInTransfer != 0 && secondInnerNetOutTransfer != 0 {
		elapsedTime := secondTime.Sub(firstTime).Seconds()
		netInSpeed = float64(secondInnerNetInTransfer-firstInnerNetInTransfer) / elapsedTime
		netOutSpeed = float64(secondInnerNetOutTransfer-firstInnerNetOutTransfer) / elapsedTime
	}

	memory, _ := mem.VirtualMemory()

	stats = map[string]interface{}{
		"cpu": cpuPercent,
		"memory": map[string]interface{}{
			"total": memory.Total,
			"used":  memory.Used,
		},
		"network": map[string]interface{}{
			"inTransfer":  secondInnerNetInTransfer,
			"outTransfer": secondInnerNetOutTransfer,
			"inSpeed":     netInSpeed,
			"outSpeed":    netOutSpeed,
		},
	}

	status := map[string]interface{}{
		"info":  info,
		"stats": stats,
		"time":  uint64(time.Now().UnixMilli()),
	}

	statusJson, _ := json.Marshal(status)

	GlobalAgent.ReportStat(string(statusJson))
}

func ReportTrafficExecutor() {
	out := ShellExecutor(Shell{
		Command:  "iptables.sh",
		Args:     []string{"list_all"},
		Internal: true,
	})
	GlobalAgent.ReportTraffic(base64.StdEncoding.EncodeToString(out))
}

type Shell struct {
	Command  string
	Args     []string
	Internal bool
}

func ShellExecutor(shell Shell) []byte {
	command := shell.Command
	args := shell.Args
	var cmd *exec.Cmd
	if shell.Internal {
		command, err := getShellAbsolutePath(command)
		if err != nil {
			LogR.Error("get shell absolute path fail", zap.Error(err))
			return nil
		}
		args = append([]string{command}, args...)
		LogR.Sugar().Debugf("执行内部脚本命令：/bin/bash %s", args)
		cmd = exec.Command("/bin/bash", args...)
	} else {
		LogR.Sugar().Debugf("执行命令：%s %s", command, args)
		cmd = exec.Command(command, args...)
	}
	out, err := cmd.Output()
	LogR.Sugar().Debugf("执行结果：%s", out)
	if err != nil {
		var exitError *exec.ExitError
		var stderr string
		if errors.As(err, &exitError) {
			stderr = string(exitError.Stderr)
		}
		LogR.Error("shell execute fail.", zap.Error(err), zap.String("stderr", stderr))
		return nil
	}
	return out
}

func getShellAbsolutePath(shellName string) (string, error) {
	if Dir != "" {
		fullPath := filepath.Join(Dir, shellName)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	wd, err := os.Getwd()
	if err == nil {
		fullPath := filepath.Join(wd, shellName)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		fullPath := filepath.Join(exeDir, shellName)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("unable to find the absolute path for %s", shellName)
}

type IpInfo struct {
	IPv4       string `json:"ipv4"`
	IPv6       string `json:"ipv6"`
	CountryISO string `json:"country"`
}

func (ipInfo *IpInfo) String() string {
	s, _ := json.Marshal(ipInfo)
	return string(s)
}

func getIpInfo() (*IpInfo, error) {
	ipv4Info := ShellExecutor(Shell{
		Command: "curl",
		Args:    []string{"-4", "https://ipconfig.io/json"},
	})

	var ipv4InfoMap = make(map[string]interface{})
	if ipv4Info != nil && len(ipv4Info) != 0 {
		err := json.Unmarshal(ipv4Info, &ipv4InfoMap)
		if err != nil {
			return nil, err
		}
	} else {
		ipv4InfoMap["ip"] = ""
	}

	ipv6Info := ShellExecutor(Shell{
		Command: "curl",
		Args:    []string{"-6", "https://ipconfig.io/json"},
	})

	var ipv6InfoMap = make(map[string]interface{})
	if ipv6Info != nil && len(ipv6Info) != 0 {
		err := json.Unmarshal(ipv6Info, &ipv6InfoMap)
		if err != nil {
			return nil, err
		}
	} else {
		ipv6InfoMap["ip"] = ""
	}

	return &IpInfo{
		IPv4:       ipv4InfoMap["ip"].(string),
		IPv6:       ipv6InfoMap["ip"].(string),
		CountryISO: ipv4InfoMap["country_iso"].(string),
	}, nil
}
