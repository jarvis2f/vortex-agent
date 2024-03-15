package agent

import (
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestReportStatExecutor(t *testing.T) {
	agentMock := new(AgentMock)
	agentMock.On("ReportStat", mock.Anything).Run(func(args mock.Arguments) {
		t.Log(args)
	})
	GlobalAgent = agentMock

	ReportStatExecutor()
}

func TestReportTrafficExecutor(t *testing.T) {
	setup()
	agentMock := new(AgentMock)
	agentMock.On("ReportTraffic", mock.Anything).Run(func(args mock.Arguments) {
		t.Log(args)
	})
	GlobalAgent = agentMock

	ReportTrafficExecutor()
}

func TestGetIpInfo(t *testing.T) {
	setup()
	ipInfo, err := getIpInfo()
	if err != nil {
		t.Error(err)
	}
	t.Log(ipInfo)
}
