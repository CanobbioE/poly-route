package logger

import (
	"log/slog"
	"os"
)

// Logger is the generic logger interface.
type Logger interface {
	// Info logs a message at INFO level.
	Info(msg string, fields ...any)
	// Error logs a message at ERROR level.
	Error(msg string, fields ...any)
	// Debug logs a message at DEBUG level.
	Debug(msg string, fields ...any)
	// Warn logs a message at WARN level.
	Warn(msg string, fields ...any)
	// Fatal logs a message at FATAL level.
	Fatal(msg string, fields ...any)
}

// LazyLogger represents a structured Logger that offers the capability to lazily add fields.
type LazyLogger interface {
	Logger
	// WithLazy adds the given fields to the logger lazily.
	WithLazy(fields ...any) Logger
}

// NewSlog creates a new LazyLogger implemented by SlogAdapter.
func NewSlog(l *slog.Logger) LazyLogger {
	return &SlogAdapter{logger: l}
}

// SlogAdapter wraps a [slog.Logger] so that it implements the LazyLogger interface.
type SlogAdapter struct {
	logger *slog.Logger
}

// WithLazy adds the given fields to the logger lazily.
func (s *SlogAdapter) WithLazy(fields ...any) Logger {
	return &SlogAdapter{s.logger.With(fields...)}
}

// Info logs the msg at slog.LevelInfo with the given fields.
func (s *SlogAdapter) Info(msg string, fields ...any) {
	s.logger.With(fields...).Info(msg)
}

// Error logs the msg at slog.LevelError with the given fields.
func (s *SlogAdapter) Error(msg string, fields ...any) {
	s.logger.With(fields...).Error(msg)
}

// Debug logs the msg at slog.LevelDebug with the given fields.
func (s *SlogAdapter) Debug(msg string, fields ...any) {
	s.logger.With(fields...).Debug(msg)
}

// Warn logs the msg at slog.LevelWarn with the given fields.
func (s *SlogAdapter) Warn(msg string, fields ...any) {
	s.logger.With(fields...).Warn(msg)
}

// Fatal logs the msg at slog.LevelError with the given fields
// to which FATAL=true is appended, then it exists with code 1.
func (s *SlogAdapter) Fatal(msg string, fields ...any) {
	s.logger.With(append(fields, "FATAL", true)...).Error(msg)
	os.Exit(1) //nolint:revive // allow deep exit in this instance.
}

// NoOpLogger is a LazyLogger that performs no actions, used for testing purposes.
type NoOpLogger struct{}

// Info does nothing.
func (*NoOpLogger) Info(_ string, _ ...any) {}

// Error does nothing.
func (*NoOpLogger) Error(_ string, _ ...any) {}

// Debug does nothing.
func (*NoOpLogger) Debug(_ string, _ ...any) {}

// Warn does nothing.
func (*NoOpLogger) Warn(_ string, _ ...any) {}

// Fatal does nothing.
func (*NoOpLogger) Fatal(_ string, _ ...any) {}

// WithLazy does nothing.
func (n *NoOpLogger) WithLazy(_ ...any) Logger { return n }
