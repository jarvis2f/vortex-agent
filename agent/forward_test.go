package agent

import (
	"encoding/json"
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestGetRandomPort(t *testing.T) {
	setup()
	agentMock := new(AgentMock)
	agentMock.On("GetConfig", mock.Anything).Return("1024-49151")
	GlobalAgent = agentMock
	port := GetRandomPort()
	t.Log(port)
}

func TestPortIsUsed(t *testing.T) {
	setup()
	used := PortIsUsed(65535)
	t.Log(used)
}

func TestUnmarshalForwardTask(t *testing.T) {
	task := "{\"action\":\"add\",\"id\":\"clrvmi7pg00154ggo6g9mji10\",\"method\":\"GOST\",\"options\":{\"services\":[{\"name\":\"forward-clrvmi7m100124ggo5xvukx3w\",\"addr\":\"clrvmi7m100124ggo5xvukx3w-agentPort\",\"handler\":{\"type\":\"relay\",\"chain\":\"chain-clrvmi7m100124ggo5xvukx3w\"},\"listener\":{\"type\":\"tcp\"}}],\"chains\":[{\"name\":\"chain-clrvmi7m100124ggo5xvukx3w\",\"hops\":[{\"name\":\"hop-clrvmi7m100124ggo5xvukx3w\",\"nodes\":[{\"name\":\"node-clrvmi7m100124ggo5xvukx3w\",\"addr\":\"0.0.0.0:3456\",\"connector\":{\"type\":\"relay\"},\"dialer\":{\"tls\":{\"serverName\":\"0.0.0.0\"}}}]}]}]},\"forwardId\":\"clrvmi7m100124ggo5xvukx3w\",\"type\":\"forward\",\"agentPort\":0,\"targetPort\":3456,\"target\":\"0.0.0.0\"}"

	var forwardTask ForwardTask
	err := json.Unmarshal([]byte(task), &forwardTask)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(forwardTask)
	t.Log(string(forwardTask.Options))
}
