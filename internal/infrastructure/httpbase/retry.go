package httpbase

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxRetryDelay = 60 * time.Second

// RetryDelay returns the bounded Retry-After delay or an exponential fallback.
func RetryDelay(value string, retry int, now time.Time) time.Duration {
	delay := time.Second
	if retry > 0 {
		delay <<= min(retry, 6)
	}

	value = strings.TrimSpace(value)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		if seconds > int(maxRetryDelay/time.Second) {
			return maxRetryDelay
		}
		delay = time.Duration(seconds) * time.Second
	} else if retryAt, err := http.ParseTime(value); err == nil {
		delay = retryAt.Sub(now)
		if delay < 0 {
			delay = 0
		}
	}
	if delay > maxRetryDelay {
		return maxRetryDelay
	}
	return delay
}

// Wait blocks for delay or returns early when ctx is canceled.
func Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
