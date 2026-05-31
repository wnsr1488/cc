package monitor

import "testing"

func TestParseOutput(t *testing.T) {
	metric, err := parseOutput(1, "12.50 45.00 63.00 1.20 0.98 0.75 120 30 1024 2048 5 999\n")
	if err != nil {
		t.Fatalf("parseOutput: %v", err)
	}
	if metric.CPUUsage != 12.5 || metric.MemoryUsage != 45 || metric.DiskUsage != 63 {
		t.Fatalf("unexpected usage values: %+v", metric)
	}
	if metric.TCPEstablished != 120 || metric.BlockedIPCount != 5 || metric.IptablesDropHits != 999 {
		t.Fatalf("unexpected counters: %+v", metric)
	}
}

func TestParseOutputRequiresAllFields(t *testing.T) {
	_, err := parseOutput(1, "1 2 3")
	if err == nil {
		t.Fatal("expected error for short output")
	}
}
