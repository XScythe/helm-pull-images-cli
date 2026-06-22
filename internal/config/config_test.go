package config

import (
	"log/slog"
	"net/http"
	"testing"

	"helm-pull-images-cli/internal/log"
)

func TestNewConfig(t *testing.T) {
	cfg := New()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if cfg.HTTPClient == nil {
		t.Fatal("expected non-nil HTTP client")
	}
	if cfg.Verbose {
		t.Error("expected verbose to be false by default")
	}
	if cfg.Debug {
		t.Error("expected debug to be false by default")
	}
}

func TestWithVerbose(t *testing.T) {
	cfg := New()
	result := cfg.WithVerbose(true)

	if result != cfg {
		t.Error("expected WithVerbose to return same config for chaining")
	}
	if !cfg.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestWithDebug(t *testing.T) {
	cfg := New()
	result := cfg.WithDebug(true)

	if result != cfg {
		t.Error("expected WithDebug to return same config for chaining")
	}
	if !cfg.Debug {
		t.Error("expected debug to be true")
	}
}

func TestWithHTTPClient(t *testing.T) {
	cfg := New()
	customClient := &http.Client{Timeout: 0}

	result := cfg.WithHTTPClient(customClient)

	if result != cfg {
		t.Error("expected WithHTTPClient to return same config for chaining")
	}
	if cfg.HTTPClient != customClient {
		t.Error("expected custom HTTP client to be set")
	}
}

func TestWithLogger(t *testing.T) {
	cfg := New()
	customLogger := log.New(slog.LevelDebug)

	result := cfg.WithLogger(customLogger)

	if result != cfg {
		t.Error("expected WithLogger to return same config for chaining")
	}
	if cfg.Logger != customLogger {
		t.Error("expected custom logger to be set")
	}
}

func TestConfigChaining(t *testing.T) {
	customClient := &http.Client{}
	customLogger := log.New(slog.LevelDebug)

	cfg := New().
		WithVerbose(true).
		WithDebug(true).
		WithHTTPClient(customClient).
		WithLogger(customLogger)

	if !cfg.Verbose {
		t.Error("expected verbose to be true after chaining")
	}
	if !cfg.Debug {
		t.Error("expected debug to be true after chaining")
	}
	if cfg.HTTPClient != customClient {
		t.Error("expected custom HTTP client to persist after chaining")
	}
	if cfg.Logger != customLogger {
		t.Error("expected custom logger to persist after chaining")
	}
}
