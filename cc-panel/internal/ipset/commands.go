package ipset

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	BlacklistSet    = "cc_blacklist"
	WhitelistSet    = "cc_whitelist"
	TempBlockSet    = "cc_temp_block"
	GeoWhitelistSet = "cc_geo_whitelist"
	GeoBlockSet     = "cc_geo_block"
	RateBlockSet    = "cc_rate_block"
)

func InitScript() string {
	return strings.Join([]string{
		"set -e",
		"ipset create cc_blacklist hash:ip timeout 0 -exist",
		"ipset create cc_whitelist hash:ip timeout 0 -exist",
		"ipset create cc_temp_block hash:ip timeout 3600 -exist",
		"ipset create cc_geo_whitelist hash:net timeout 0 -exist",
		"ipset create cc_geo_block hash:net timeout 0 -exist",
		"ipset create cc_rate_block hash:ip timeout 600 -exist",
	}, "\n")
}

func AddIPCommand(setName, ip string, timeoutSeconds int) (string, error) {
	if err := validateSetAndIP(setName, ip); err != nil {
		return "", err
	}
	return formatAddIP(setName, ip, timeoutSeconds), nil
}

func BulkAddIPScript(setName string, ips []string, timeoutSeconds int) (string, error) {
	if len(ips) == 0 {
		return "", fmt.Errorf("ips is required")
	}
	lines := []string{ensureSetCreateLine(setName)}
	for _, ip := range ips {
		if err := validateSetAndIP(setName, ip); err != nil {
			return "", err
		}
		add := formatAddIP(setName, ip, timeoutSeconds)
		lines = append(lines, fmt.Sprintf(`if ipset test %s %s 2>/dev/null; then echo "SKIP %s"; else %s && echo "ADDED %s"; fi`, setName, ip, ip, add, ip))
	}
	return strings.Join(lines, "\n"), nil
}

func ensureSetCreateLine(setName string) string {
	switch setName {
	case WhitelistSet:
		return "ipset create cc_whitelist hash:ip timeout 0 -exist"
	case BlacklistSet:
		return "ipset create cc_blacklist hash:ip timeout 0 -exist"
	default:
		return fmt.Sprintf("ipset create %s hash:ip timeout 0 -exist", setName)
	}
}

func AddTimedIPCommand(setName, ip string, timeoutSeconds int) (string, error) {
	if err := validateTimedSetAndIP(setName, ip); err != nil {
		return "", err
	}
	if timeoutSeconds <= 0 {
		return "", fmt.Errorf("timeout must be greater than 0 for timed block set %q", setName)
	}
	return formatAddIP(setName, ip, timeoutSeconds), nil
}

func formatAddIP(setName, ip string, timeoutSeconds int) string {
	parts := []string{"ipset", "add", setName, ip, "-exist"}
	if timeoutSeconds > 0 {
		parts = append(parts, "timeout", strconv.Itoa(timeoutSeconds))
	}
	return strings.Join(parts, " ")
}

func AddCIDRCommand(setName, cidr string) (string, error) {
	if setName != GeoBlockSet && setName != GeoWhitelistSet {
		return "", fmt.Errorf("unsupported ipset %q", setName)
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return "", fmt.Errorf("invalid cidr %q", cidr)
	}
	return fmt.Sprintf("ipset add %s %s -exist", setName, cidr), nil
}

func DeleteIPCommand(setName, ip string) (string, error) {
	if err := validateSetAndIP(setName, ip); err != nil {
		return "", err
	}
	return formatDeleteIP(setName, ip), nil
}

func DeleteTimedIPCommand(setName, ip string) (string, error) {
	if err := validateTimedSetAndIP(setName, ip); err != nil {
		return "", err
	}
	return formatDeleteIP(setName, ip), nil
}

func formatDeleteIP(setName, ip string) string {
	return fmt.Sprintf("ipset del %s %s -exist", setName, ip)
}

func validateSetAndIP(setName, ip string) error {
	if setName != BlacklistSet && setName != WhitelistSet {
		return fmt.Errorf("unsupported ipset %q", setName)
	}
	return validateIP(ip)
}

func validateTimedSetAndIP(setName, ip string) error {
	if setName != TempBlockSet && setName != RateBlockSet {
		return fmt.Errorf("unsupported timed block ipset %q", setName)
	}
	return validateIP(ip)
}

func validateIP(ip string) error {
	ip = NormalizeIP(ip)
	if parsed := net.ParseIP(ip); parsed == nil {
		return fmt.Errorf("invalid ip %q", ip)
	}
	return nil
}

// NormalizeIP converts IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) to plain IPv4.
func NormalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if !strings.HasPrefix(strings.ToLower(ip), "::ffff:") {
		return ip
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		if v4 := parsed.To4(); v4 != nil {
			return v4.String()
		}
	}
	return strings.TrimPrefix(ip, "::ffff:")
}

func CleanupMappedIPv4BlockScript() string {
	return strings.Join([]string{
		`for set in cc_rate_block cc_temp_block; do`,
		`  ipset list "$set" 2>/dev/null | awk '/^::ffff:/{print $1}' | while read -r ip; do`,
		`    [ -n "$ip" ] || continue`,
		`    ipset del "$set" "$ip" -exist 2>/dev/null || true`,
		`  done`,
		`done`,
	}, "\n")
}
