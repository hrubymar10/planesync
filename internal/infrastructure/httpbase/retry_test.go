package httpbase

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		retry int
		want  time.Duration
	}{
		{name: "seconds", value: "3", want: 3 * time.Second},
		{name: "HTTP date", value: now.Add(5 * time.Second).Format(http.TimeFormat), want: 5 * time.Second},
		{name: "fallback", retry: 2, want: 4 * time.Second},
		{name: "bounded", value: "120", want: 60 * time.Second},
		{name: "large integer bounded", value: "9223372036854775807", want: 60 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RetryDelay(test.value, test.retry, now); got != test.want {
				t.Errorf("RetryDelay() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestWaitHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Wait(ctx, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v, want context cancellation", err)
	}
}
