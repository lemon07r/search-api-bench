package jina

import "testing"

func TestNewClient_DefaultRetryPolicies(t *testing.T) {
	t.Setenv("JINA_API_KEY", "")
	t.Setenv("JINA_TIMEOUT", "")
	t.Setenv("JINA_SEARCH_TIMEOUT", "")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchRetryCfg.MaxRetries != 2 {
		t.Fatalf("expected search retries=2, got %d", client.searchRetryCfg.MaxRetries)
	}
	if client.extractRetryCfg.MaxRetries != 1 {
		t.Fatalf("expected extract retries=1, got %d", client.extractRetryCfg.MaxRetries)
	}
	if client.searchTimeout != defaultSearchTimeout {
		t.Fatalf("expected default search timeout %v, got %v", defaultSearchTimeout, client.searchTimeout)
	}
}

func TestNewClient_SearchTimeoutOverride(t *testing.T) {
	t.Setenv("JINA_SEARCH_TIMEOUT", "3s")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.searchTimeout.String() != "3s" {
		t.Fatalf("expected search timeout 3s, got %v", client.searchTimeout)
	}
}
