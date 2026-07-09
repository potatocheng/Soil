package net

// Logger 是可观测性接入点，默认使用 no-op
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger 是默认空实现
type noopLogger struct{}

func (n noopLogger) Debug(_ string, _ ...any) {}
func (n noopLogger) Info(_ string, _ ...any)  {}
func (n noopLogger) Warn(_ string, _ ...any)  {}
func (n noopLogger) Error(_ string, _ ...any) {}

var defaultLogger Logger = noopLogger{}
