package ipset

import (
	"fmt"
	"strings"
)

func RestoreLines(setName string, cidrs []string, flush bool) []string {
	lines := make([]string, 0, len(cidrs)+1)
	if flush {
		lines = append(lines, fmt.Sprintf("flush %s", setName))
	}
	for _, cidr := range cidrs {
		lines = append(lines, fmt.Sprintf("add %s %s -exist", setName, cidr))
	}
	return lines
}

func RestoreScript(setName string, cidrs []string, flush bool) string {
	return strings.Join(RestoreLines(setName, cidrs, flush), "\n")
}

// SnapshotScript rebuilds a set using the same create parameters as InitScript.
func SnapshotScript(setName string, cidrs []string) string {
	maxelem := len(cidrs) + 4096
	if maxelem < 65536 {
		maxelem = 65536
	}
	lines := []string{
		fmt.Sprintf("ipset create %s hash:net timeout 0 -exist", setName),
	}
	if len(cidrs) > 60000 {
		lines = []string{
			fmt.Sprintf("ipset destroy %s 2>/dev/null || true", setName),
			fmt.Sprintf("ipset create %s hash:net timeout 0 maxelem %d", setName, maxelem),
		}
	}
	lines = append(lines,
		"ipset restore -exist <<'CC_PANEL_IPSET'",
		RestoreScript(setName, cidrs, true),
		"CC_PANEL_IPSET",
	)
	return strings.Join(lines, "\n")
}

func IncrementalAddScript(setName string, cidrs []string) string {
	lines := []string{
		fmt.Sprintf("ipset create %s hash:net timeout 0 -exist", setName),
	}
	for _, cidr := range cidrs {
		lines = append(lines, fmt.Sprintf("ipset add %s %s -exist", setName, cidr))
	}
	return strings.Join(lines, "\n")
}
