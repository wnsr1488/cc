package geo

import "testing"

func TestCIDRContentHashIgnoresOrder(t *testing.T) {
	a := cidrContentHash([]string{"2.0.0.0/24", "1.0.0.0/24"})
	b := cidrContentHash([]string{"1.0.0.0/24", "2.0.0.0/24"})
	if a != b {
		t.Fatalf("expected same hash, got %q and %q", a, b)
	}
}

func TestCIDRContentHashDetectsChange(t *testing.T) {
	a := cidrContentHash([]string{"1.0.0.0/24"})
	b := cidrContentHash([]string{"1.0.0.0/24", "2.0.0.0/24"})
	if a == b {
		t.Fatalf("expected different hashes")
	}
}
