package users

import "testing"

func TestAdd(t *testing.T) {
	d := NewDirectory()
	d.Add("A@x.io", "Ann")
	if d.byEmail["a@x.io"] != "Ann" {
		t.Fatalf("byEmail = %v", d.byEmail)
	}
}
