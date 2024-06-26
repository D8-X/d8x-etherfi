package utils

import (
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

type TokenBucket struct {
	tokens      int
	capacity    int
	refillRate  float64 // Tokens per second
	lastRefill  time.Time
	refillMutex sync.Mutex
}

func NewTokenBucket(capacity int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	tokensToAdd := int(float64(elapsed.Nanoseconds()) / 1e9 * tb.refillRate)
	if tokensToAdd > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		tb.lastRefill = now
	}
}

func (tb *TokenBucket) WaitForToken(topic string, doLog bool) {
	for {
		if tb.Take() {
			if doLog {
				slog.Info(topic + ": rpc token obtained")
			}
			return
		}
		if doLog {
			slog.Info(topic + ": too many RPC requests, slowing down ")
		}
		time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
	}
}

func (tb *TokenBucket) Take() bool {
	tb.refillMutex.Lock()
	defer tb.refillMutex.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}
