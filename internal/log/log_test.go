package log

import (
	"log/slog"
	"testing"
)

func TestNewLoggerDefaults(t *testing.T) {
	logger := New(slog.LevelInfo)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if !logger.V(slog.LevelInfo) {
		t.Error("expected info level to be enabled")
	}
	if logger.V(slog.LevelDebug) {
		t.Error("expected debug level to be disabled at info level")
	}
}

func TestVLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		checkLvl slog.Level
		want     bool
	}{
		{"debug enabled at debug", slog.LevelDebug, slog.LevelDebug, true},
		{"info enabled at debug", slog.LevelDebug, slog.LevelInfo, true},
		{"debug disabled at info", slog.LevelInfo, slog.LevelDebug, false},
		{"info enabled at info", slog.LevelInfo, slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New(tt.level)
			got := logger.V(tt.checkLvl)
			if got != tt.want {
				t.Errorf("V(%v) = %v, want %v", tt.checkLvl, got, tt.want)
			}
		})
	}
}
