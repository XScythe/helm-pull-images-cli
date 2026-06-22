package errors

import (
	"errors"
	"testing"
)

func TestNewAppError(t *testing.T) {
	err := New(ValidationError, "test message")
	if err.Type != ValidationError {
		t.Errorf("expected type %v, got %v", ValidationError, err.Type)
	}
	if err.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", err.Message)
	}
}

func TestWrapError(t *testing.T) {
	rootErr := errors.New("root cause")
	wrapped := Wrap(NetworkError, "network failed", rootErr)

	if wrapped.Type != NetworkError {
		t.Errorf("expected type %v, got %v", NetworkError, wrapped.Type)
	}
	if wrapped.Err != rootErr {
		t.Errorf("expected wrapped error to be preserved")
	}
	if !errors.Is(wrapped, rootErr) {
		t.Errorf("expected wrapped error to satisfy errors.Is")
	}
}

func TestErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		contains string
	}{
		{
			"error with type and message only",
			New(ConfigError, "invalid config"),
			"[config] invalid config",
		},
		{
			"error with type, message, and cause",
			Wrap(ExecutionError, "execution failed", errors.New("disk full")),
			"[execution] execution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			if !contains(errStr, tt.contains) {
				t.Errorf("Error() = %q, expected to contain %q", errStr, tt.contains)
			}
		})
	}
}

func TestIsType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		errType  ErrorType
		wantTrue bool
	}{
		{
			"matches validation error",
			NewValidation("invalid input"),
			ValidationError,
			true,
		},
		{
			"does not match different type",
			NewValidation("invalid input"),
			NetworkError,
			false,
		},
		{
			"non-app error returns false",
			errors.New("generic error"),
			ValidationError,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsType(tt.err, tt.errType)
			if got != tt.wantTrue {
				t.Errorf("IsType() = %v, want %v", got, tt.wantTrue)
			}
		})
	}
}

func TestWithContext(t *testing.T) {
	err := New(ValidationError, "test")
	err.WithContext("field", "username").WithContext("value", "admin")

	if err.Context["field"] != "username" {
		t.Errorf("expected context field 'username', got %v", err.Context["field"])
	}
	if err.Context["value"] != "admin" {
		t.Errorf("expected context value 'admin', got %v", err.Context["value"])
	}
}

func contains(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
