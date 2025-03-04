package logger

import (
	"strings"
)

const (
	TRACE = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
)

func LevelFromString(level string) LogLevel {
	defaultLogLevel := LogLevel(INFO)
	switch strings.ToLower(level) {
	case "trace":
		return TRACE
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	case "fatal", "panic":
		return FATAL
	default:
		return defaultLogLevel
	}

}

func (l LogLevel) String() string {
	switch l {
	case TRACE:
		return "TRACE"
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
	}

	return "UNKNOWN"
}

type LogLevel int

func (LogLevel) Trace() int {
	return TRACE
}

func (LogLevel) Debug() int {
	return DEBUG
}

func (LogLevel) Info() int {
	return INFO
}

func (LogLevel) Warn() int {
	return WARN
}

func (LogLevel) Error() int {
	return ERROR
}

func (LogLevel) Fatal() int {
	return FATAL
}
