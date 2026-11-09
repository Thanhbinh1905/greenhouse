package users

import "testing"

func TestAcceptFindIgnoresCase(t *testing.T) {
	d := NewDirectory()
	d.Add("ann@example.com", "Ann")
	if name, ok := d.Find("Ann@Example.COM"); !ok || name != "Ann" {
		t.Fatalf("Find = %q, %t", name, ok)
	}
}

func TestRegressFindExactAndMissing(t *testing.T) {
	d := NewDirectory()
	d.Add("Bob@example.com", "Bob")
	if name, ok := d.Find("bob@example.com"); !ok || name != "Bob" {
		t.Fatalf("Find = %q, %t", name, ok)
	}
	if _, ok := d.Find("carol@example.com"); ok {
		t.Fatal("Find returned an unknown address")
	}
}
