package quality

import "testing"

func TestIsRetryableQualityStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "retry on 429", status: 429, body: "rate limit", want: true},
		{name: "retry on 503", status: 503, body: "service unavailable", want: true},
		{
			name:   "retry on transient 400",
			status: 400,
			body:   `{"detail":"Unable to process request. Repeat the request later."}`,
			want:   true,
		},
		{name: "do not retry on permanent 400", status: 400, body: `{"detail":"invalid request"}`, want: false},
		{name: "do not retry on 401", status: 401, body: `{"detail":"unauthorized"}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableQualityStatus(tt.status, tt.body)
			if got != tt.want {
				t.Fatalf("isRetryableQualityStatus(%d, %q) = %v, want %v", tt.status, tt.body, got, tt.want)
			}
		})
	}
}
