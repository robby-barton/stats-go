package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger() *zap.Logger {
	ec := zap.NewProductionEncoderConfig()
	ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
	ec.EncodeTime = zapcore.ISO8601TimeEncoder
	ec.EncodeCaller = zapcore.ShortCallerEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(ec)
	core := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), zap.InfoLevel)
	return zap.New(core, zap.AddCaller())
}
