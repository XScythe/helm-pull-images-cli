// Package log provides structured logging utilities consistent with kubectl/helm patterns.
//
// This package offers:
// - Multiple log levels (Info, Debug, Warn, Error)
// - Structured logging with key-value pairs
// - Context-aware output
//
// Example usage:
//
//	logger := log.New(slog.LevelInfo)
//	logger.Info("pulling chart", "chart", "bitnami/nginx", "version", "13.1.5")
//	logger.Error("failed to pull", err, "chart", "bitnami/nginx")
package log

import (
	"fmt"
	"log/slog"
	"os"
)

// Logger provides structured logging with context awareness.
type Logger struct {
	logger *slog.Logger
	level  slog.Level
}

// New creates a new Logger with the specified level.
func New(level slog.Level) *Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return &Logger{
		logger: slog.New(handler),
		level:  level,
	}
}

// Info logs an informational message.
func (l *Logger) Info(msg string, args ...interface{}) {
	l.logger.Info(msg, toAttrs(args)...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.logger.Debug(msg, toAttrs(args)...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.logger.Warn(msg, toAttrs(args)...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, err error, args ...interface{}) {
	allArgs := append([]interface{}{"error", err}, args...)
	l.logger.Error(msg, toAttrs(allArgs)...)
}

// toAttrs converts key-value pairs to slog attributes slice and returns as variadic.
func toAttrs(args []interface{}) (result []any) {
	if len(args)%2 != 0 {
		args = append(args, "invalid arguments")
	}
	for i := 0; i < len(args); i += 2 {
		key := fmt.Sprintf("%v", args[i])
		result = append(result, slog.Any(key, args[i+1]))
	}
	return result
}

// V returns true if the logger is configured to output at the given level.
func (l *Logger) V(level slog.Level) bool {
	return l.logger.Enabled(nil, level)
}
