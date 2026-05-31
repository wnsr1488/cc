package policy

import "testing"

func TestParseConnectionCounts(t *testing.T) {
	items, err := parseConnectionCounts("120 1.2.3.4\n50 5.6.7.8\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].IP != "1.2.3.4" || items[0].Count != 120 {
		t.Fatalf("unexpected parse result: %+v", items)
	}
}

func TestParseConnectionCountsSkipsInvalid(t *testing.T) {
	items, err := parseConnectionCounts("bad line\n10 not-an-ip\n8 8.8.8.8\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].IP != "8.8.8.8" {
		t.Fatalf("unexpected parse result: %+v", items)
	}
}
