// Package config provides dependency injection configuration consistent with helm patterns.
//
// The Config struct holds all shared dependencies, enabling:
// - Testing with mock implementations
// - Consistent configuration across all commands
// - Easy swapping of implementations at runtime
//
// This follows the helm/kubectl pattern of centralizing configuration and passing it through
// the application layers.
//
// Example usage:
//
//	cfg := config.New().
//		WithVerbose(true).
//		WithLogger(log.New(slog.LevelDebug))
//
//	// Pass to functions
//	result, err := chartmirror.Run(ctx, cfg, opts)
package config

import (
	"log/slog"
	"net/http"

	"helm-pull-images-cli/internal/log"
)

// Config holds all shared dependencies, following the helm/kubectl pattern of dependency injection.
// This enables:
// - Testing with mock implementations
// - Consistent configuration across all commands
// - Easy swapping of implementations at runtime
type Config struct {
	Logger     *log.Logger
	HTTPClient *http.Client
	Verbose    bool
	Debug      bool
}

// New creates a new Config with default settings.
func New() *Config {
	level := slog.LevelInfo
	cfg := &Config{
		Logger:     log.New(level),
		HTTPClient: &http.Client{},
		Verbose:    false,
		Debug:      false,
	}
	return cfg
}

// WithVerbose sets verbose logging mode.
func (c *Config) WithVerbose(verbose bool) *Config {
	c.Verbose = verbose
	if verbose {
		c.Logger = log.New(slog.LevelDebug)
	}
	return c
}

// WithDebug sets debug mode.
func (c *Config) WithDebug(debug bool) *Config {
	c.Debug = debug
	if debug {
		c.Logger = log.New(slog.LevelDebug)
	}
	return c
}

// WithHTTPClient sets a custom HTTP client (useful for testing).
func (c *Config) WithHTTPClient(client *http.Client) *Config {
	c.HTTPClient = client
	return c
}

// WithLogger sets a custom logger (useful for testing).
func (c *Config) WithLogger(logger *log.Logger) *Config {
	c.Logger = logger
	return c
}
