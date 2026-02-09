// Package debug provides comprehensive logging for troubleshooting and analysis.
package debug

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TimingBreakdown captures detailed HTTP timing using httptrace
type TimingBreakdown struct {
	DNSLookup       time.Duration `json:"dns_lookup"`
	TCPConnection   time.Duration `json:"tcp_connection"`
	TLSHandshake    time.Duration `json:"tls_handshake"`
	TimeToFirstByte time.Duration `json:"time_to_first_byte"`
	TotalDuration   time.Duration `json:"total_duration"`
}

// Logger handles comprehensive debug logging for benchmark runs
type Logger struct {
	mu          sync.RWMutex
	enabled     bool
	fullCapture bool
	startTime   time.Time
	session     *Session
	outputDir   string
	outputPath  string
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
	
	// internal metadata storage for non-serializable objects (not exported to JSON)
	internalMeta map[string]interface{}
}

// RequestLog captures HTTP request details
type RequestLog struct {
	Timestamp    time.Time         `json:"timestamp"`
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyPreview  string            `json:"body_preview,omitempty"`
	BodyFull     string            `json:"body_full,omitempty"`
	Timing       *TimingBreakdown  `json:"timing,omitempty"`
	RetryAttempt int               `json:"retry_attempt,omitempty"`
}

// ResponseLog captures HTTP response details
type ResponseLog struct {
	Timestamp   time.Time         `json:"timestamp"`
	StatusCode  int               `json:"status_code"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyPreview string            `json:"body_preview,omitempty"`
	BodyFull    string            `json:"body_full,omitempty"`
	BodySize    int               `json:"body_size"`
	Duration    time.Duration     `json:"duration"`
	Timing      *TimingBreakdown  `json:"timing,omitempty"`
}

// ErrorLog captures error details with context
type ErrorLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Category  string    `json:"category,omitempty"`
	Context   string    `json:"context,omitempty"`
}

// NewLogger creates a new debug logger
// enabled: enables basic debug logging
// fullCapture: when true, captures complete request/response bodies (requires enabled=true)
// outputDir: base output directory for debug files
func NewLogger(enabled bool, fullCapture bool, outputDir string) *Logger {
	logger := &Logger{
		enabled:     enabled,
		fullCapture: fullCapture,
		startTime:   time.Now(),
		outputDir:   outputDir,
		session: &Session{
			StartTime: time.Now(),
			Providers: make(map[string]*ProviderLog),
			SystemInfo: map[string]interface{}{
				"go_version":   "1.21+",
				"timestamp":    time.Now().Format(time.RFC3339),
				"full_capture": fullCapture,
			},
		},
	}

	if enabled {
		logger.outputPath = filepath.Join(outputDir, "debug")
	}

	return logger
}

// IsEnabled returns whether debug logging is enabled
func (l *Logger) IsEnabled() bool {
	return l.enabled
}

// IsFullCapture returns whether full body capture is enabled
func (l *Logger) IsFullCapture() bool {
	return l.fullCapture
}

// NewTraceContext creates an httptrace.ClientTrace that populates timing data
// Returns a context with the trace attached and a *TimingBreakdown to be filled
func (l *Logger) NewTraceContext(testLog *TestLog) (*TimingBreakdown, func()) {
	if !l.enabled || testLog == nil {
		return nil, func() {}
	}

	timing := &TimingBreakdown{}
	var dnsStart, tcpStart, tlsStart, firstByteTime time.Time

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			timing.DNSLookup = time.Since(dnsStart)
		},
		ConnectStart: func(_, _ string) {
			tcpStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			timing.TCPConnection = time.Since(tcpStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			timing.TLSHandshake = time.Since(tlsStart)
		},
		GotFirstResponseByte: func() {
			firstByteTime = time.Now()
		},
	}

	// Return timing struct and a function to finalize total duration
	finalize := func() {
		if !firstByteTime.IsZero() {
			timing.TimeToFirstByte = firstByteTime.Sub(dnsStart)
		}
		// TotalDuration should be set by the caller after request completes
	}

	// Store trace in internal metadata (not serialized to JSON)
	l.mu.Lock()
	if testLog.internalMeta == nil {
		testLog.internalMeta = make(map[string]interface{})
	}
	testLog.internalMeta["_trace"] = trace
	testLog.internalMeta["_timing"] = timing
	l.mu.Unlock()

	return timing, finalize
}

// GetTraceFromTest retrieves the httptrace.ClientTrace associated with a test log
func (l *Logger) GetTraceFromTest(testLog *TestLog) *httptrace.ClientTrace {
	if !l.enabled || testLog == nil {
		return nil
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	if testLog.internalMeta == nil {
		return nil
	}
	if trace, ok := testLog.internalMeta["_trace"].(*httptrace.ClientTrace); ok {
		return trace
	}
	return nil
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

	if l.fullCapture {
		reqLog.BodyFull = bodyPreview
	}

	testLog.Requests = append(testLog.Requests, reqLog)
}

// LogRequestWithTiming logs an HTTP request with timing information
func (l *Logger) LogRequestWithTiming(testLog *TestLog, method, url string, headers map[string]string, bodyPreview string, timing *TimingBreakdown, retryAttempt int) {
	if !l.enabled || testLog == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	reqLog := RequestLog{
		Timestamp:    time.Now(),
		Method:       method,
		URL:          url,
		Headers:      headers,
		BodyPreview:  truncateString(bodyPreview, 500),
		Timing:       timing,
		RetryAttempt: retryAttempt,
	}

	if l.fullCapture {
		reqLog.BodyFull = bodyPreview
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

	if l.fullCapture {
		testLog.Response.BodyFull = bodyPreview
	}
}

// LogResponseWithTiming logs an HTTP response with detailed timing
func (l *Logger) LogResponseWithTiming(testLog *TestLog, statusCode int, headers map[string]string, bodyPreview string, bodySize int, duration time.Duration, timing *TimingBreakdown) {
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
		Timing:      timing,
	}

	if l.fullCapture {
		testLog.Response.BodyFull = bodyPreview
	}

	// Update timing total duration
	if timing != nil {
		timing.TotalDuration = duration
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

// Finalize completes the debug session and writes per-provider log files
func (l *Logger) Finalize() error {
	if !l.enabled {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.session.EndTime = &now

	// Create debug subdirectory
	debugDir := l.outputPath
	if err := os.MkdirAll(debugDir, 0750); err != nil {
		return fmt.Errorf("failed to create debug output directory: %w", err)
	}

	// Write session metadata
	sessionPath := filepath.Join(debugDir, "session.json")
	sessionData := map[string]interface{}{
		"start_time":  l.session.StartTime,
		"end_time":    l.session.EndTime,
		"system_info": l.session.SystemInfo,
		"providers":   make([]string, 0, len(l.session.Providers)),
	}
	for name := range l.session.Providers {
		sessionData["providers"] = append(sessionData["providers"].([]string), name)
	}

	data, err := json.MarshalIndent(sessionData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}
	if err := os.WriteFile(sessionPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	// Write per-provider files
	for providerName, providerLog := range l.session.Providers {
		providerPath := filepath.Join(debugDir, providerName+".json")
		data, err := json.MarshalIndent(providerLog, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal provider data for %s: %w", providerName, err)
		}
		if err := os.WriteFile(providerPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write provider file for %s: %w", providerName, err)
		}
	}

	return nil
}

// GetOutputPath returns the path where debug data will be written (debug directory)
func (l *Logger) GetOutputPath() string {
	return l.outputPath
}

// GetSessionPath returns the path to the session.json file
func (l *Logger) GetSessionPath() string {
	if !l.enabled {
		return ""
	}
	return filepath.Join(l.outputPath, "session.json")
}

// GetProviderPath returns the path to a specific provider's debug file
func (l *Logger) GetProviderPath(providerName string) string {
	if !l.enabled {
		return ""
	}
	return filepath.Join(l.outputPath, providerName+".json")
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
