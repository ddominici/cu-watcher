package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"strings"
)

type Config struct {
	Level      string
	File       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
}

func New(cfg Config) *zap.Logger {
	m := cfg

	level := zapcore.InfoLevel
	switch strings.ToLower(m.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	consoleEnc := zapcore.NewConsoleEncoder(encCfg)
	fileEnc := zapcore.NewJSONEncoder(encCfg)

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   m.File,
		MaxSize:    m.MaxSizeMB,
		MaxBackups: m.MaxBackups,
		MaxAge:     m.MaxAgeDays,
		Compress:   true,
	})

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEnc, zapcore.AddSync(os.Stdout), level),
		zapcore.NewCore(fileEnc, fileWriter, level),
	)

	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
}

func F(key string, val any) zap.Field { return zap.Any(key, val) }
func E(err error) zap.Field           { return zap.Error(err) }
