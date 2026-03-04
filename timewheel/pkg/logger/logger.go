package logger

import (
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"timewheel/internal/config"
)

var (
	// 全局日志实例
	globalLogger *zap.Logger
	// Sugar logger 用于格式化输出
	sugar *zap.SugaredLogger
)

// Init 初始化日志
func Init(cfg *config.LoggingConfig) error {
	// 创建日志目录
	if cfg.Output.Type == "file" || cfg.Output.Type == "both" {
		logDir := filepath.Dir(cfg.Output.Path)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}
	}

	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 根据格式选择编码器
	var encoder zapcore.Encoder
	if cfg.Format == "console" {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 配置日志级别
	level := getZapLevel(cfg.Level)

	// 配置输出
	var cores []zapcore.Core

	switch cfg.Output.Type {
	case "stdout":
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))
	case "file":
		writer := getLogWriter(cfg)
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(writer), level))
	case "both":
		// 文件输出（JSON格式）
		fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
		writer := getLogWriter(cfg)
		cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(writer), level))
		// 控制台输出（文本格式）
		consoleEncoderConfig := encoderConfig
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
		cores = append(cores, zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level))
	default:
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level))
	}

	// 创建 logger
	core := zapcore.NewTee(cores...)
	globalLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar = globalLogger.Sugar()

	return nil
}

// getZapLevel 获取 zap 日志级别
func getZapLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// getLogWriter 获取日志写入器（支持轮转）
func getLogWriter(cfg *config.LoggingConfig) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   cfg.Output.Path,
		MaxSize:    cfg.Rotation.MaxSize,    // MB
		MaxAge:     cfg.Rotation.MaxAge,     // days
		MaxBackups: cfg.Rotation.MaxBackups,
		Compress:   cfg.Rotation.Compress,
		LocalTime:  true,
	}
}

// L 获取全局 logger
func L() *zap.Logger {
	if globalLogger == nil {
		// 返回 nop logger
		return zap.NewNop()
	}
	return globalLogger
}

// S 获取全局 sugared logger
func S() *zap.SugaredLogger {
	if sugar == nil {
		return zap.NewNop().Sugar()
	}
	return sugar
}

// Sync 刷新日志缓冲
func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}

// With 创建带有字段的 logger
func With(fields ...zap.Field) *zap.Logger {
	return L().With(fields...)
}

// Debug 调试日志
func Debug(msg string, fields ...zap.Field) {
	L().Debug(msg, fields...)
}

// Info 信息日志
func Info(msg string, fields ...zap.Field) {
	L().Info(msg, fields...)
}

// Warn 警告日志
func Warn(msg string, fields ...zap.Field) {
	L().Warn(msg, fields...)
}

// Error 错误日志
func Error(msg string, fields ...zap.Field) {
	L().Error(msg, fields...)
}

// Fatal 致命错误日志（会调用 os.Exit）
func Fatal(msg string, fields ...zap.Field) {
	L().Fatal(msg, fields...)
}

// Panic panic 日志（会 panic）
func Panic(msg string, fields ...zap.Field) {
	L().Panic(msg, fields...)
}

// Named 创建命名 logger
func Named(name string) *zap.Logger {
	return L().Named(name)
}

// 包级别的 SugaredLogger 方法
func Debugf(template string, args ...interface{}) {
	S().Debugf(template, args...)
}

func Infof(template string, args ...interface{}) {
	S().Infof(template, args...)
}

func Warnf(template string, args ...interface{}) {
	S().Warnf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	S().Errorf(template, args...)
}

func Fatalf(template string, args ...interface{}) {
	S().Fatalf(template, args...)
}

func Panicf(template string, args ...interface{}) {
	S().Panicf(template, args...)
}
