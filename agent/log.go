package agent

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger
var LogR *zap.Logger

var LogLevels = []string{"debug", "info", "warn", "error", "panic", "fatal"}
var LogDestinations = []string{"console", "remote"}

var dest string

type RemoteWriteSyncer struct {
}

func (r RemoteWriteSyncer) Write(p []byte) (n int, err error) {
	if GlobalAgent == nil || !GlobalAgent.Ready() {
		Log.Info(string(p))
	} else {
		GlobalAgent.ReportLog(string(p))
	}
	return len(p), nil
}

func ReInitLog(level string) {
	InitLog(level, dest)
}

func InitLog(level string, destination string) {
	dest = destination
	l, _ := zap.ParseAtomicLevel(level)

	config := zap.NewDevelopmentConfig()
	config.Level = l
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	Log, _ = config.Build()

	if destination != "remote" {
		LogR = Log
		return
	}

	LogR, _ = zap.NewProduction()

	LogR = LogR.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&RemoteWriteSyncer{}),
			l,
		)
	}))
}

func SyncLog() {
	_ = Log.Sync()
	_ = LogR.Sync()
}
