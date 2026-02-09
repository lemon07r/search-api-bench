// Package debug provides comprehensive logging for troubleshooting and analysis.
package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger handles comprehensive debug logging for benchmark runs
type Logger struct {
	mu         sync.RWMutex
	enabled    bool
	startTime  time.Time
	session    *Session
	outputPath string
}

// Session represents the entire debug session
type Session struct {
	StartTime  time.Time               `json:"start_time"`
	EndTime    *time.Time              `json:"end_time,omitempty"`
	Providers  map[string]*ProviderLog `json:"providers"`
	SystemInfo map[string]interface{}  `json:"system_info"`
}

// ProviderLog contains debug data for a single provider
type ProviderLog struct {
	Name      string    `json:"name"`
	InitTime  time.Time `json:"init_time"`
	InitError string    `json:"init_error,omitempty"`
	Tests     []TestLog `json:"tests"`
}

// TestLog contains debug data for a single test
type TestLog struct {
	TestName  string                 `json:"test_name"`
	TestType  string                 `json:"test_type"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Requests  []RequestLog           `json:"requests"`
	Response  *ResponseLog           `json:"response,omitempty"`
	Errors    []ErrorLog             `json:"errors"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// RequestLog captures HTTP request details
type RequestLog struct {
	Timestamp   time.Time         `json:"timestamp"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyPreview string            `json:"body_preview,omitempty"`
}

// ResponseLog captures HTTP response details
type ResponseLog struct {
	Timestamp   time.Time         `json:"timestamp"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyPreview string            `json:"body_preview,omitempty"`
	BodySize    int               `json:"body_size"`
	Duration    time.Duration     `json:"duration"`
}

// ErrorLog captures error details with context
type ErrorLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Category  string    `json:"category,omitempty"`
	Context   string    `json:"context,omitempty"`
}

// NewLogger creates a new debug logger
func NewLogger(enabled bool, outputDir string) *Logger {
	logger := &Logger{
		enabled:   enabled,
		startTime: time.Now(),
		session: &Session{
			StartTime: time.Now(),
			Providers: make(map[string]*ProviderLog),
			SystemInfo: map[string]interface{}{
				"go_version": "1.21+",
				"timestamp":  time.Now().Format(time.RFC3339),
			},
		},
	}

	if enabled {
		logger.outputPath = filepath.Join(outputDir, "debug.json")
	}

	return logger
}

// IsEnabled returns whether debug logging is enabled
func (l *Logger) IsEnabled() bool {
	return l.enabled
}

// LogProviderInit logs provider initialization
func (l *Logger) LogProviderInit(providerName string, err error) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	providerLog := &ProviderLog{
		Name:     providerName,
		InitTime: time.Now(),
		Tests:    []TestLog{},
	}

	if err != nil {
		providerLog.InitError = err.Error()
	}

	l.session.Providers[providerName] = providerLog
}

// StartTest begins logging a new test
func (l *Logger) StartTest(providerName, testName, testType string) *TestLog {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	providerLog, exists := l.session.Providers[providerName]
	if !exists {
		providerLog = &ProviderLog{
			Name:     providerName,
			InitTime: time.Now(),
			Tests:    []TestLog{},
		}
		l.session.Providers[providerName] = providerLog
	}

	testLog := TestLog{
		TestName:  testName,
		TestType:  testType,
		StartTime: time.Now(),
		Requests:  []RequestLog{},
		Errors:    []ErrorLog{},
		Metadata:  make(map[string]interface{}),
	}

	providerLog.Tests = append(providerLog.Tests, testLog)
	return &providerLog.Tests[len(providerLog.Tests)-1]
}

// LogRequest logs an HTTP request
func (l *Logger) LogRequest(testLog *TestLog, method, url string, headers map[string]string, bodyPreview string) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	reqLog := RequestLog{
		Timestamp:   time.Now(),
		Method:      method,
		URL:         url,
		Headers:     headers,
		BodyPreview: truncateString(bodyPreview, 500),
	}

	testLog.Requests = append(testLog.Requests, reqLog)
}

// LogResponse logs an HTTP response
func (l *Logger) LogResponse(testLog *TestLog, statusCode int, headers map[string]string, bodyPreview string, bodySize int, duration time.Duration) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	testLog.Response = &ResponseLog{
		Timestamp:   time.Now(),
		StatusCode:  statusCode,
		Headers:     headers,
		BodyPreview: truncateString(bodyPreview, 1000),
		BodySize:    bodySize,
		Duration:    duration,
	}
}

// LogError logs an error with context
func (l *Logger) LogError(testLog *TestLog, message, category, context string) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	errLog := ErrorLog{
		Timestamp: time.Now(),
		Message:   message,
		Category:  category,
		Context:   context,
	}

	testLog.Errors = append(testLog.Errors, errLog)
}

// SetMetadata adds metadata to a test log
func (l *Logger) SetMetadata(testLog *TestLog, key string, value interface{}) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	testLog.Metadata[key] = value
}

// EndTest marks a test as complete
func (l *Logger) EndTest(testLog *TestLog) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	testLog.EndTime = &now
	testLog.Duration = now.Sub(testLog.StartTime)
}

// Finalize completes the debug session and writes the log file
func (l *Logger) Finalize() error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.session.EndTime = &now

	// Ensure output directory exists
	dir := filepath.Dir(l.outputPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create debug output directory: %w", err)
	}

	// Write JSON file
	data, err := json.MarshalIndent(l.session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug data: %w", err)
	}

	if err := os.WriteFile(l.outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write debug file: %w", err)
	}

	return nil
}

// GetOutputPath returns the path where debug data will be written
func (l *Logger) GetOutputPath() string {
	return l.outputPath
}

// truncateString limits a string to a maximum length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
