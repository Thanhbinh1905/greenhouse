package cache

import "testing"

func TestSetGet(t *testing.T) {
	c := New()
	c.Set("k", "v")
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("Get = %q, %t", v, ok)
	}
}
