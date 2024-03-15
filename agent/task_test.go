package agent

import (
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestHandlePingTask(t *testing.T) {
	setup()
	agentMock := new(AgentMock)
	agentMock.On("ReportTaskResult", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		t.Logf("ReportTaskResult: %v", args)
	})
	GlobalAgent = agentMock
	task := Task{
		Id:   "123",
		Type: "ping",
		OriginData: []byte(`{
			"host": "8.8.8.8",
			"count": 5,
			"timeout": 50
		}`),
	}
	_, err := handlePingTask(task)
	if err != nil {
		t.Error(err)
	}
}
