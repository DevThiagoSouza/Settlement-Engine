package logging

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

type Logger interface {
	Debug(ctx context.Context, msg string, fields ...interface{})
	Info(ctx context.Context, msg string, fields ...interface{})
	Warn(ctx context.Context, msg string, fields ...interface{})
	Error(ctx context.Context, msg string, fields ...interface{})
	Fatal(ctx context.Context, msg string, fields ...interface{})
	WithRequestID(requestID string) Logger
}

type SimpleLogger struct {
	level     LogLevel
	requestID string
	logger    *log.Logger
}

func NewLogger(level LogLevel) *SimpleLogger {
	return &SimpleLogger{
		level:  level,
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func levelToString(level LogLevel) string {
	switch level {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

func (l *SimpleLogger) formatMessage(level LogLevel, msg string, fields ...interface{}) string {
	timestamp := time.Now().Format(time.RFC3339)
	levelStr := levelToString(level)

	fieldStr := ""
	if len(fields) > 0 {
		fieldStr = fmt.Sprint(fields...)
	}

	requestID := ""
	if l.requestID != "" {
		requestID = fmt.Sprintf(" [%s]", l.requestID)
	}

	return fmt.Sprintf("%s %s%s: %s %s", timestamp, levelStr, requestID, msg, fieldStr)
}

func (l *SimpleLogger) Debug(ctx context.Context, msg string, fields ...interface{}) {
	if l.level <= DEBUG {
		l.logger.Println(l.formatMessage(DEBUG, msg, fields...))
	}
}

func (l *SimpleLogger) Info(ctx context.Context, msg string, fields ...interface{}) {
	if l.level <= INFO {
		l.logger.Println(l.formatMessage(INFO, msg, fields...))
	}
}

func (l *SimpleLogger) Warn(ctx context.Context, msg string, fields ...interface{}) {
	if l.level <= WARN {
		l.logger.Println(l.formatMessage(WARN, msg, fields...))
	}
}

func (l *SimpleLogger) Error(ctx context.Context, msg string, fields ...interface{}) {
	if l.level <= ERROR {
		l.logger.Println(l.formatMessage(ERROR, msg, fields...))
	}
}

func (l *SimpleLogger) Fatal(ctx context.Context, msg string, fields ...interface{}) {
	l.logger.Fatal(l.formatMessage(FATAL, msg, fields...))
}

func (l *SimpleLogger) WithRequestID(requestID string) Logger {
	return &SimpleLogger{
		level:     l.level,
		requestID: requestID,
		logger:    l.logger,
	}
}
