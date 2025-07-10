package internal

import (
	"errors"
	"testing"
)

func TestLogger(t *testing.T) {
	// Initialize logger
	err := InitLogger(LogLevelDebug)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Test basic logging
	Logger.Info("This is an info message")
	Logger.Info("Info with values", "key1", "value1", "key2", 42)

	// Test error logging
	testErr := errors.New("test error")
	Logger.Error(testErr, "This is an error message", "context", "test")

	// Test named logger
	namedLogger := WithName("test-component")
	namedLogger.Info("Message from named logger")

	// Test valued logger
	valuedLogger := WithValues("service", "test-service", "version", "1.0.0")
	valuedLogger.Info("Message from valued logger")

	// Test verbose level logging
	Logger.V(1).Info("Verbose level 1 message")
	Logger.V(2).Info("Verbose level 2 message")
	Logger.V(3).Info("Verbose level 3 message")

	// Test combined usage
	componentLogger := WithName("database").WithValues("connection", "primary")
	componentLogger.Info("Database operation completed", "duration", "150ms", "rows", 5)
	componentLogger.Error(testErr, "Database operation failed", "query", "SELECT * FROM users")

	FlushLogs()
}
