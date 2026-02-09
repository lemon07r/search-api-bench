// Package progress provides a terminal progress bar and status display
// that stays pinned at the bottom of the screen during benchmark execution.
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
)

// TestStatus represents the current state of a test
type TestStatus int

const (
	// StatusPending indicates a test is waiting to run
	StatusPending TestStatus = iota
	// StatusRunning indicates a test is currently running
	StatusRunning
	// StatusSuccess indicates a test completed successfully
	StatusSuccess
	// StatusFailed indicates a test failed
	StatusFailed
)

// RunningTest tracks a currently executing test
type RunningTest struct {
	Name      string
	Provider  string
	StartTime time.Time
	Status    TestStatus
}

// Manager handles the progress display
type Manager struct {
	enabled       bool
	totalTests    int
	completed     int
	passed        int
	failed        int
	runningTests  map[string]*RunningTest // key: "provider:testName"
	mu            sync.RWMutex
	bar           *progressbar.ProgressBar
	startTime     time.Time
	showProviders []string
}

// NewManager creates a new progress manager
func NewManager(totalTests int, providers []string, enabled bool) *Manager {
	m := &Manager{
		enabled:       enabled,
		totalTests:    totalTests,
		runningTests:  make(map[string]*RunningTest),
		startTime:     time.Now(),
		showProviders: providers,
	}

	if enabled {
		m.setupProgressBar()
	}

	return m
}

// setupProgressBar initializes the progress bar
func (m *Manager) setupProgressBar() {
	// Custom template for our progress bar
	barTemplate := `{{printf "%-30s" .Description}} {{bar . }} {{counters . }} {{percent . }} {{rtime . "ETA: ~%s remaining"}}`

	m.bar = progressbar.NewOptions(m.totalTests,
		progressbar.OptionSetDescription("Benchmark Progress"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetWidth(40),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("tests"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "|",
			BarEnd:        "|",
		}),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetElapsedTime(true),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprintln(os.Stderr)
		}),
		progressbar.OptionSetWriter(io.Discard), // We'll handle rendering manually
	)

	// Store template for manual rendering
	_ = barTemplate
}

// StartTest marks a test as started
func (m *Manager) StartTest(provider, testName string) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", provider, testName)
	m.runningTests[key] = &RunningTest{
		Name:      testName,
		Provider:  provider,
		StartTime: time.Now(),
		Status:    StatusRunning,
	}

	m.render()
}

// CompleteTest marks a test as completed
func (m *Manager) CompleteTest(provider, testName string, success bool, _ error) {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", provider, testName)
	if test, exists := m.runningTests[key]; exists {
		test.Status = StatusSuccess
		if !success {
			test.Status = StatusFailed
		}
		// Keep it in running tests briefly for display, will be cleaned on next render
	}

	m.completed++
	if success {
		m.passed++
	} else {
		m.failed++
	}

	delete(m.runningTests, key)
	m.render()
}

// render draws the progress display
func (m *Manager) render() {
	if !m.enabled {
		return
	}

	// Clear screen and move cursor to top
	fmt.Print("\033[2J\033[H")

	// Print header
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Search API Benchmark Tool                             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Calculate progress bar
	percent := float64(m.completed) / float64(m.totalTests)
	filled := int(percent * 40)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 40-filled)

	// Overall progress
	fmt.Printf("  Overall Progress: [%s] %d/%d tests (%.0f%%)\n", bar, m.completed, m.totalTests, percent*100)

	// ETA calculation
	if m.completed > 0 && m.completed < m.totalTests {
		elapsed := time.Since(m.startTime)
		avgPerTest := elapsed / time.Duration(m.completed)
		remaining := m.totalTests - m.completed
		eta := time.Duration(remaining) * avgPerTest
		fmt.Printf("  ETA: ~%s remaining (avg %.1fs per test)\n", formatDuration(eta), avgPerTest.Seconds())
	}
	fmt.Println()

	// Results summary
	fmt.Printf("  Results: ✓ %d passed | ✗ %d failed | ○ %d pending\n",
		m.passed, m.failed, m.totalTests-m.completed)
	fmt.Println()

	// Currently running tests by provider
	if len(m.runningTests) > 0 {
		fmt.Println("  Currently Running:")

		// Group by provider
		byProvider := make(map[string][]*RunningTest)
		for _, test := range m.runningTests {
			byProvider[test.Provider] = append(byProvider[test.Provider], test)
		}

		// Show each provider's activity
		for _, provider := range m.showProviders {
			tests, hasRunning := byProvider[provider]
			if hasRunning && len(tests) > 0 {
				for _, test := range tests {
					elapsed := time.Since(test.StartTime).Round(time.Millisecond)
					fmt.Printf("    • %-12s: %-35s (%s elapsed)\n", provider, truncate(test.Name, 35), formatDuration(elapsed))
				}
			}
		}
		fmt.Println()
	}

	// Recent completed (keep last few for context)
	fmt.Println("  Recent activity (scrolls up):")
}

// PrintAbove prints a message above the progress bar
func (m *Manager) PrintAbove(format string, args ...interface{}) {
	if !m.enabled {
		fmt.Printf(format+"\n", args...)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Print the message
	fmt.Printf(format+"\n", args...)

	// Re-render progress bar below
	m.render()
}

// Finish marks the benchmark as complete
func (m *Manager) Finish() {
	if !m.enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear screen for final summary
	fmt.Print("\033[2J\033[H")
}

// IsEnabled returns whether progress display is enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// truncate truncates a string to max length with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
