package agent

import (
	"context"
	"github.com/stretchr/testify/mock"
	"testing"
)

func setup() {
	InitLog("debug", "console")
	Dir = "/root/vortex-agent/scripts"
}

type AgentMock struct {
	mock.Mock
}

func (a *AgentMock) Start(ctx context.Context) {
	a.Called(ctx)
}

func (a *AgentMock) Stop() {
	a.Called()
}

func (a *AgentMock) Ready() bool {
	return a.Called().Get(0).(bool)
}

func (a *AgentMock) GetConfig(key string) string {
	return a.Called(key).Get(0).(string)
}

func (a *AgentMock) GetConfigWithGlobal(key string, global bool) string {
	return a.Called(key, global).Get(0).(string)
}

func (a *AgentMock) ReportStat(status string) {
	a.Called(status)
}

func (a *AgentMock) ReportTraffic(traffic string) {
	a.Called(traffic)
}

func (a *AgentMock) ReportTaskResult(taskId string, success bool, extra string) {
	a.Called(taskId, success, extra)
}

func (a *AgentMock) ReportLog(log string) {
	a.Called(log)
}

func (a *AgentMock) UpdateJobCron(cronKey string) {
	a.Called(cronKey)
}

func TestDecrypt(t *testing.T) {
	data := "f08d752c4db6a14005bc082f841d06c9400d8c1c5b73c30ea113f11f55056faa96a881309acfd129f3fa10c0b9d28f61b31387e7ccf44fb92391923cc97c189a08d9cfca4ef93579263d42d09727dcd94dfd27ebf342bc6041bfcc3c8d62e2e473b2e7950be03f9390b8657964a28ee4ebfe9699dd3eed57ff1e95b7f5252520"
	key := []byte("291274495f723c22738f0a09145f0c6004cfe506efecfe210218b0837a2582e4")
	decrypt, err := Decrypt(data, key)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(decrypt))
}
