package cache

import (
	"testing"
	"time"
)

func TestAcceptMissReleasesLock(t *testing.T) {
	c := New()
	done := make(chan int)
	go func() {
		c.Get("absent")
		c.Set("k", "v")
		done <- c.Misses()
	}()
	select {
	case n := <-done:
		if n != 1 {
			t.Fatalf("Misses = %d, want 1", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cache miss left the mutex locked")
	}
}

func TestRegressHit(t *testing.T) {
	c := New()
	c.Set("a", "1")
	c.Set("a", "2")
	if v, ok := c.Get("a"); !ok || v != "2" {
		t.Fatalf("Get = %q, %t", v, ok)
	}
	if n := c.Misses(); n != 0 {
		t.Fatalf("Misses = %d, want 0", n)
	}
}
