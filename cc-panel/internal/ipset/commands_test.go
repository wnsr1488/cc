package ipset

import "testing"

func TestAddIPCommand(t *testing.T) {
	command, err := AddIPCommand(BlacklistSet, "8.8.8.8", 600)
	if err != nil {
		t.Fatalf("AddIPCommand() error = %v", err)
	}
	want := "ipset add cc_blacklist 8.8.8.8 -exist timeout 600"
	if command != want {
		t.Fatalf("AddIPCommand() = %q, want %q", command, want)
	}
}

func TestAddIPCommandRejectsInvalidIP(t *testing.T) {
	if _, err := AddIPCommand(BlacklistSet, "8.8.8.8; rm -rf /", 0); err == nil {
		t.Fatal("AddIPCommand() error = nil, want validation error")
	}
}

func TestDeleteIPCommand(t *testing.T) {
	command, err := DeleteIPCommand(WhitelistSet, "1.1.1.1")
	if err != nil {
		t.Fatalf("DeleteIPCommand() error = %v", err)
	}
	want := "ipset del cc_whitelist 1.1.1.1 -exist"
	if command != want {
		t.Fatalf("DeleteIPCommand() = %q, want %q", command, want)
	}
}
