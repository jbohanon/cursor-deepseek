package logger

import (
	"context"
	"fmt"
	"time"

	"github.com/danilofalcao/cursor-deepseek/internal/constants"
	contextutils "github.com/danilofalcao/cursor-deepseek/internal/utils/context"
)

var (
	Fallback = New(context.Background(), "fallback", DEBUG, make(chan string))
)

type Logger struct {
	name   string
	ctx    context.Context
	level  LogLevel
	exitCh chan string
}

func New(ctx context.Context, name string, level LogLevel, exitCh chan string) *Logger {
	return &Logger{
		name:   name,
		ctx:    ctx,
		level:  level,
		exitCh: exitCh,
	}
}

func out(ctx context.Context, s string, level LogLevel) {
	if reqId := contextutils.GetRequestID(ctx); reqId != "" {
		outWithReqId(s, level, reqId)
		return
	}
	fmt.Printf("[%s][%s] %s\n", time.Now().Local().Format(time.DateTime), level.String(), s)
}

func outWithReqId(s string, level LogLevel, reqId string) {
	fmt.Printf("[%s][%s][%s] %s\n", time.Now().Local().Format(time.DateTime), level.String(), reqId, s)
}

func (l *Logger) Clone(name string) (*Logger, context.Context) {
	lgr := New(l.ctx, name, l.level, l.exitCh)
	ctx := context.WithValue(l.ctx, constants.LoggerKey, lgr)
	return lgr, ctx
}

func (l *Logger) WithLevel(level LogLevel) *Logger {
	l.level = level
	return l
}

func (l *Logger) Trace(ctx context.Context, s string) {
	if l.level > TRACE {
		return
	}
	out(ctx, s, TRACE)
}

func (l *Logger) Tracef(ctx context.Context, s string, args ...any) {
	l.Info(ctx, fmt.Sprintf(s, args...))
}

func (l *Logger) Debug(ctx context.Context, s string) {
	if l.level > DEBUG {
		return
	}
	out(ctx, s, DEBUG)
}

func (l *Logger) Debugf(ctx context.Context, s string, args ...any) {
	l.Debug(ctx, fmt.Sprintf(s, args...))

}

func (l *Logger) Info(ctx context.Context, s string) {
	if l.level > INFO {
		return
	}
	out(ctx, s, INFO)
}

func (l *Logger) Infof(ctx context.Context, s string, args ...any) {
	l.Info(ctx, fmt.Sprintf(s, args...))
}

func (l *Logger) Warn(ctx context.Context, s string) {
	if l.level > WARN {
		return
	}
	out(ctx, s, WARN)
}

func (l *Logger) Warnf(ctx context.Context, s string, args ...any) {
	l.Warn(ctx, fmt.Sprintf(s, args...))
}

func (l *Logger) Error(ctx context.Context, s string) {
	if l.level > ERROR {
		return
	}
	out(ctx, s, ERROR)
}

func (l *Logger) Errorf(ctx context.Context, s string, args ...any) {
	l.Error(ctx, fmt.Sprintf(s, args...))
}

func (l *Logger) Fatal(ctx context.Context, s string) {
	if l.level > FATAL {
		return
	}
	out(ctx, s, FATAL)
	l.exitCh <- s
}

func (l *Logger) Fatalf(ctx context.Context, s string, args ...any) {
	l.Fatal(ctx, fmt.Sprintf(s, args...))
}
