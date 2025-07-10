package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

// Logger global logger instance
var Logger logr.Logger

// LogLevel represents log level
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// InitLogger initializes the logger
func InitLogger(level LogLevel) error {
	// Set verbosity based on level
	var verbosity int
	switch level {
	case LogLevelDebug:
		verbosity = 4
	case LogLevelInfo:
		verbosity = 2
	case LogLevelWarn:
		verbosity = 1
	case LogLevelError:
		verbosity = 0
	default:
		verbosity = 2
	}

	// Create custom LogSink
	sink := NewLogSink(verbosity)
	Logger = logr.New(sink).WithName("chart-release-manager")

	Logger.Info("Logger initialized", "level", level, "verbosity", verbosity)

	return nil
}

// WithName creates a sub-logger with name
func WithName(name string) logr.Logger {
	return Logger.WithName(name)
}

// WithValues creates a sub-logger with key-value pairs
func WithValues(keysAndValues ...interface{}) logr.Logger {
	return Logger.WithValues(keysAndValues...)
}

// FlushLogs flushes log buffer
func FlushLogs() {
}

// LogSink custom log sink
type LogSink struct {
	name      string
	verbosity int
	values    map[string]interface{}
}

// NewLogSink creates a new LogSink
func NewLogSink(verbosity int) logr.LogSink {
	return &LogSink{
		verbosity: verbosity,
		values:    make(map[string]interface{}),
	}
}

// Init initializes the log sink
func (l *LogSink) Init(info logr.RuntimeInfo) {
	// No special initialization needed
}

// Enabled checks if the specified level is enabled
func (l *LogSink) Enabled(level int) bool {
	return level <= l.verbosity
}

// Info logs an info level message
func (l *LogSink) Info(level int, msg string, keysAndValues ...interface{}) {
	if !l.Enabled(level) {
		return
	}

	levelStr := "INFO"
	if level > 0 {
		levelStr = fmt.Sprintf("V%d", level)
	}

	// Build log message
	output := l.formatMessage(levelStr, msg, keysAndValues...)
	fmt.Fprintf(os.Stderr, "%s\n", output)
}

// Error logs an error level message
func (l *LogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	// Build key-value pairs including error
	kvs := append([]interface{}{"error", err}, keysAndValues...)
	output := l.formatMessage("ERROR", msg, kvs...)
	fmt.Fprintf(os.Stderr, "%s\n", output)
}

// WithValues adds key-value pairs
func (l *LogSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	newSink := &LogSink{
		name:      l.name,
		verbosity: l.verbosity,
		values:    make(map[string]interface{}),
	}

	// Copy existing values
	for k, v := range l.values {
		newSink.values[k] = v
	}

	// Add new key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			newSink.values[key] = keysAndValues[i+1]
		}
	}

	return newSink
}

// WithName adds name to logger
func (l *LogSink) WithName(name string) logr.LogSink {
	newSink := &LogSink{
		verbosity: l.verbosity,
		values:    make(map[string]interface{}),
	}

	// Copy existing values
	for k, v := range l.values {
		newSink.values[k] = v
	}

	// Set name
	if l.name != "" {
		newSink.name = l.name + "." + name
	} else {
		newSink.name = name
	}

	return newSink
}

// formatMessage formats log message
func (l *LogSink) formatMessage(level, msg string, keysAndValues ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z")

	var parts []string
	parts = append(parts, fmt.Sprintf("%s %s", timestamp, level))

	if l.name != "" {
		parts = append(parts, fmt.Sprintf("%s", l.name))
	}

	parts = append(parts, msg)

	// Add existing key-value pairs
	for k, v := range l.values {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	// Add new key-value pairs
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			value := keysAndValues[i+1]
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}

	return strings.Join(parts, " ")
}
