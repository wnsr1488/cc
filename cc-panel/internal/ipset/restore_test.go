package ipset

import (
	"strings"
	"testing"
)

func TestRestoreScriptFlushesSet(t *testing.T) {
	script := RestoreScript(GeoWhitelistSet, []string{"1.0.0.0/24", "2.0.0.0/24"}, true)
	if !strings.Contains(script, "flush cc_geo_whitelist") {
		t.Fatalf("expected flush in restore script:\n%s", script)
	}
	if !strings.Contains(script, "add cc_geo_whitelist 1.0.0.0/24") {
		t.Fatalf("expected cidr entries in restore script:\n%s", script)
	}
	if strings.Contains(script, "create cc_geo_whitelist") {
		t.Fatalf("restore script should not create set:\n%s", script)
	}
}

func TestSnapshotScriptUsesMatchingCreate(t *testing.T) {
	script := SnapshotScript(GeoWhitelistSet, []string{"1.0.0.0/24"})
	if !strings.Contains(script, "ipset create cc_geo_whitelist hash:net timeout 0 -exist") {
		t.Fatalf("expected matching create in snapshot script:\n%s", script)
	}
	if !strings.Contains(script, "flush cc_geo_whitelist") {
		t.Fatalf("expected flush in snapshot script:\n%s", script)
	}
}
