package tls_client

import "fmt"

type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// DebugEnabledLogger is an optional extension implemented by loggers that can
// report whether Debug calls are currently observable. Unknown logger
// implementations are treated as enabled to preserve the historical behavior.
type DebugEnabledLogger interface {
	DebugEnabled() bool
}

func loggerDebugEnabled(logger Logger) bool {
	if logger == nil {
		return false
	}
	if enabledLogger, ok := logger.(DebugEnabledLogger); ok {
		return enabledLogger.DebugEnabled()
	}
	return true
}

type noopLogger struct{}

func NewNoopLogger() Logger {
	return &noopLogger{}
}

func (n noopLogger) Debug(_ string, _ ...any) {}

func (n noopLogger) DebugEnabled() bool { return false }

func (n noopLogger) Info(_ string, _ ...any) {}

func (n noopLogger) Warn(_ string, _ ...any) {}

func (n noopLogger) Error(_ string, _ ...any) {}

type debugLogger struct {
	logger Logger
}

func NewDebugLogger(logger Logger) Logger {
	return &debugLogger{
		logger: logger,
	}
}

func (n debugLogger) Debug(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func (n debugLogger) DebugEnabled() bool { return true }

func (n debugLogger) Info(format string, args ...any) {
	n.logger.Info(format, args...)
}

func (n debugLogger) Warn(format string, args ...any) {
	n.logger.Warn(format, args...)
}

func (n debugLogger) Error(format string, args ...any) {
	n.logger.Error(format, args...)
}

type logger struct{}

func NewLogger() Logger {
	return &logger{}
}

func (n logger) Debug(_ string, _ ...any) {}

func (n logger) DebugEnabled() bool { return false }

func (n logger) Info(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func (n logger) Warn(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func (n logger) Error(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// Interface guards are a cheap way to make sure all methods are implemented, this is a static check and does not affect runtime performance.
var (
	_ Logger = (*logger)(nil)
	_ Logger = (*debugLogger)(nil)
	_ Logger = (*noopLogger)(nil)
)
