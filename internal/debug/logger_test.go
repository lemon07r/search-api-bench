package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestLoggerConcurrentLifecycleProducesCompleteLogs(t *testing.T) {
	outputDir := t.TempDir()
	logger := NewLogger(true, true, outputDir)
	logger.LogProviderInit("provider", nil)

	const testsCount = 120
	var wg sync.WaitGroup
	for i := 0; i < testsCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			testLog := logger.StartTest("provider", fmt.Sprintf("test-%d", i), "search")
			logger.LogRequest(testLog, "GET", "https://example.com", map[string]string{"Authorization": "Bearer secret"}, "")
			logger.LogResponse(testLog, 200, map[string]string{"Content-Type": "application/json"}, "{\"ok\":true}", 11, 10*time.Millisecond)
			if i%5 == 0 {
				logger.LogError(testLog, "transient failure", "timeout", "search execution")
				logger.SetStatus(testLog, "failed")
			} else {
				logger.SetStatus(testLog, "completed")
			}
			logger.EndTest(testLog)
		}()
	}
	wg.Wait()

	if err := logger.Finalize(); err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	providerFile := logger.GetProviderPath("provider")
	rawProvider, err := os.ReadFile(providerFile)
	if err != nil {
		t.Fatalf("failed reading provider debug file: %v", err)
	}

	var providerLog ProviderLog
	if err := json.Unmarshal(rawProvider, &providerLog); err != nil {
		t.Fatalf("failed parsing provider debug file: %v", err)
	}

	if providerLog.SchemaVersion != debugSchemaVersion {
		t.Fatalf("expected schema_version %d, got %d", debugSchemaVersion, providerLog.SchemaVersion)
	}
	if len(providerLog.Tests) != testsCount {
		t.Fatalf("expected %d tests, got %d", testsCount, len(providerLog.Tests))
	}

	seenIDs := make(map[string]struct{}, testsCount)
	for _, testLog := range providerLog.Tests {
		if testLog == nil {
			t.Fatal("found nil test log entry")
		}
		if testLog.ID == "" {
			t.Fatal("expected test id to be populated")
		}
		if _, exists := seenIDs[testLog.ID]; exists {
			t.Fatalf("duplicate test id %q", testLog.ID)
		}
		seenIDs[testLog.ID] = struct{}{}
		if testLog.EndTime == nil {
			t.Fatalf("test %q missing end_time", testLog.TestName)
		}
		if testLog.Response == nil {
			t.Fatalf("test %q missing response", testLog.TestName)
		}
		if testLog.Status == "running" || testLog.Status == "" {
			t.Fatalf("test %q has invalid terminal status %q", testLog.TestName, testLog.Status)
		}
	}

	rawSession, err := os.ReadFile(logger.GetSessionPath())
	if err != nil {
		t.Fatalf("failed reading session debug file: %v", err)
	}

	var session map[string]interface{}
	if err := json.Unmarshal(rawSession, &session); err != nil {
		t.Fatalf("failed parsing session debug file: %v", err)
	}

	if got, ok := session["schema_version"].(float64); !ok || int(got) != debugSchemaVersion {
		t.Fatalf("expected session schema_version %d, got %#v", debugSchemaVersion, session["schema_version"])
	}
}

func TestEndTestSetsCompletedStatusByDefault(t *testing.T) {
	logger := NewLogger(true, false, t.TempDir())
	logger.LogProviderInit("provider", nil)

	testLog := logger.StartTest("provider", "name", "search")
	if testLog.Status != "running" {
		t.Fatalf("expected status running on start, got %q", testLog.Status)
	}

	logger.EndTest(testLog)
	if testLog.Status != "completed" {
		t.Fatalf("expected completed status after EndTest, got %q", testLog.Status)
	}
}

func TestEndTestPreservesFailedStatus(t *testing.T) {
	logger := NewLogger(true, false, t.TempDir())
	logger.LogProviderInit("provider", nil)

	testLog := logger.StartTest("provider", "name", "search")
	logger.SetStatus(testLog, "failed")
	logger.EndTest(testLog)

	if testLog.Status != "failed" {
		t.Fatalf("expected failed status to be preserved, got %q", testLog.Status)
	}
}
