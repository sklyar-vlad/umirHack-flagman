package auth

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginLimiterWindowCapacityAndConcurrency(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Now()
	limiter.now = func() time.Time { return now }
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.allow("email", "same@example.test", 5) {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 5 {
		t.Fatal("concurrent limit bypassed")
	}
	for i := 1; i < maxLoginKeys; i++ {
		if !limiter.allow("ip", fmt.Sprint(i), 20) {
			t.Fatal("capacity exhausted early")
		}
	}
	if limiter.allow("ip", "new", 20) {
		t.Fatal("unbounded limiter map")
	}
	now = now.Add(loginWindow)
	if !limiter.allow("email", "same@example.test", 5) || len(limiter.entries) != 1 {
		t.Fatal("expired windows did not recover")
	}
}
