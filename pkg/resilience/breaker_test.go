package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	breaker := NewCircuitBreaker(2, 20*time.Millisecond)
	if !breaker.Allow() {
		t.Fatal("new breaker should allow requests")
	}
	breaker.RecordFailure()
	breaker.RecordFailure()
	if breaker.Allow() {
		t.Fatal("breaker should open at threshold")
	}
	time.Sleep(25 * time.Millisecond)
	if !breaker.Allow() {
		t.Fatal("breaker should allow a probe after cooldown")
	}
	breaker.RecordSuccess()
	if !breaker.Allow() {
		t.Fatal("successful probe should close breaker")
	}
}
