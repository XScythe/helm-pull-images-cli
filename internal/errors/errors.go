// Package errors provides custom error types and context-aware error wrapping.
//
// This follows the kubectl pattern of structured error handling with:
// - Categorized errors (ValidationError, NetworkError, etc.)
// - Error chaining for root cause analysis
// - Context information for debugging
// - Type assertions for error handling
//
// Example usage:
//
//	if err := someOperation(); err != nil {
//		return errors.Wrap(errors.NetworkError, "failed to fetch chart", err).
//			WithContext("url", url).
//			WithContext("chart", chartName)
//	}
//
//	// Check error type
//	if errors.IsType(err, errors.NetworkError) {
//		// Handle network errors specifically
//	}
package errors

import (
	"fmt"
)

// ErrorType categorizes errors for better handling.
type ErrorType string

const (
	// ValidationError indicates invalid input.
	ValidationError ErrorType = "validation"
	// ConfigError indicates configuration issues.
	ConfigError ErrorType = "config"
	// NetworkError indicates network/HTTP issues.
	NetworkError ErrorType = "network"
	// ResourceError indicates issues accessing resources (files, registries).
	ResourceError ErrorType = "resource"
	// ExecutionError indicates errors during execution.
	ExecutionError ErrorType = "execution"
	// InternalError indicates internal logic errors.
	InternalError ErrorType = "internal"
)

// AppError is a structured error with type and context.
type AppError struct {
	Type    ErrorType
	Message string
	Err     error
	Context map[string]interface{}
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap allows error chain inspection.
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError with the given type and message.
func New(errType ErrorType, message string) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Context: make(map[string]interface{}),
	}
}

// Wrap wraps an error with additional context.
func Wrap(errType ErrorType, message string, err error) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Err:     err,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to an error.
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	e.Context[key] = value
	return e
}

// ValidationError creates a validation error.
func NewValidation(message string) *AppError {
	return New(ValidationError, message)
}

// NewConfig creates a config error.
func NewConfig(message string, err error) *AppError {
	return Wrap(ConfigError, message, err)
}

// NewNetwork creates a network error.
func NewNetwork(message string, err error) *AppError {
	return Wrap(NetworkError, message, err)
}

// NewResource creates a resource error.
func NewResource(message string, err error) *AppError {
	return Wrap(ResourceError, message, err)
}

// NewExecution creates an execution error.
func NewExecution(message string, err error) *AppError {
	return Wrap(ExecutionError, message, err)
}

// NewInternal creates an internal error.
func NewInternal(message string, err error) *AppError {
	return Wrap(InternalError, message, err)
}

// IsType checks if an error is of a specific type.
func IsType(err error, errType ErrorType) bool {
	if ae, ok := err.(*AppError); ok {
		return ae.Type == errType
	}
	return false
}
