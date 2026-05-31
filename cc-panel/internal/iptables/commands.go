package iptables

import (
	"fmt"
	"strings"

	"github.com/example/cc-panel/internal/ipset"
)

const (
	StrictWhitelistComment   = "cc_panel_strict_whitelist"
	commentAllowEstablished  = "cc_panel_allow_established"
	commentAllowLo           = "cc_panel_allow_lo"
	commentWhitelist         = "cc_panel_whitelist"
	commentGeoWhitelist      = "cc_panel_geo_whitelist"
	commentBlacklist         = "cc_panel_blacklist"
	commentTempBlock         = "cc_panel_temp_block"
	commentGeoBlock          = "cc_panel_geo_block"
	commentRateBlock         = "cc_panel_rate_block"
)

type inputRule struct {
	comment string
	args    string
}

func removeStrictWhitelistRules() string {
	return strings.Join([]string{
		removeInputRuleByComment(StrictWhitelistComment, `-j DROP`),
		removeInputRuleByComment(commentAllowEstablished, `-m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT`),
		removeInputRuleByComment(commentAllowLo, `-i lo -j ACCEPT`),
	}, "\n")
}

func removeInputRuleByComment(comment, args string) string {
	return fmt.Sprintf(`while iptables -D INPUT -m comment --comment "%[1]s" %[2]s 2>/dev/null; do :; done`, comment, args)
}

func removeLegacyInputRule(args string) string {
	return fmt.Sprintf(`while iptables -D INPUT %[1]s 2>/dev/null; do :; done`, args)
}

// ensureInputRule inserts a rule at the top only when neither the tagged rule nor
// an equivalent legacy rule (without comment) already exists.
func ensureInputRule(comment, args string) string {
	return fmt.Sprintf(`if ! iptables -C INPUT -m comment --comment "%[1]s" %[2]s 2>/dev/null; then
  if ! iptables -C INPUT %[2]s 2>/dev/null; then
    iptables -I INPUT 1 -m comment --comment "%[1]s" %[2]s
  fi
fi`, comment, args)
}

func ensureAppendRule(comment, args string) string {
	return fmt.Sprintf(`if ! iptables -C INPUT -m comment --comment "%[1]s" %[2]s 2>/dev/null; then
  iptables -A INPUT -m comment --comment "%[1]s" %[2]s
fi`, comment, args)
}

func InitScript(strictWhitelist bool) string {
	lines := []string{"set -e"}
	if !strictWhitelist {
		lines = append(lines, removeStrictWhitelistRules())
	}

	rules := []inputRule{
		{commentWhitelist, `-m set --match-set cc_whitelist src -j ACCEPT`},
		{commentGeoWhitelist, `-m set --match-set cc_geo_whitelist src -j ACCEPT`},
		{commentBlacklist, `-m set --match-set cc_blacklist src -j DROP`},
		{commentTempBlock, `-m set --match-set cc_temp_block src -j DROP`},
		{commentGeoBlock, `-m set --match-set cc_geo_block src -j DROP`},
		{commentRateBlock, `-m set --match-set cc_rate_block src -j DROP`},
	}
	if strictWhitelist {
		rules = append([]inputRule{
			{commentAllowEstablished, `-m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT`},
			{commentAllowLo, `-i lo -j ACCEPT`},
		}, rules...)
	}
	for i := len(rules) - 1; i >= 0; i-- {
		lines = append(lines, ensureInputRule(rules[i].comment, rules[i].args))
	}
	if strictWhitelist {
		lines = append(lines, ensureAppendRule(StrictWhitelistComment, `-j DROP`))
	}
	return strings.Join(lines, "\n")
}

func DeployScript(strictWhitelist bool) string {
	return strings.Join([]string{
		"set -e",
		"command -v ipset >/dev/null 2>&1 || { echo 'ipset is required' >&2; exit 1; }",
		"command -v iptables >/dev/null 2>&1 || { echo 'iptables is required' >&2; exit 1; }",
	}, "\n") + "\n" + ipset.InitScript() + "\n" + InitScript(strictWhitelist)
}

// StopScript removes cc-panel iptables rules from INPUT without destroying ipset data.
func StopScript() string {
	lines := []string{"set -e"}
	rules := []inputRule{
		{commentWhitelist, `-m set --match-set cc_whitelist src -j ACCEPT`},
		{commentGeoWhitelist, `-m set --match-set cc_geo_whitelist src -j ACCEPT`},
		{commentBlacklist, `-m set --match-set cc_blacklist src -j DROP`},
		{commentTempBlock, `-m set --match-set cc_temp_block src -j DROP`},
		{commentGeoBlock, `-m set --match-set cc_geo_block src -j DROP`},
		{commentRateBlock, `-m set --match-set cc_rate_block src -j DROP`},
		{commentAllowEstablished, `-m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT`},
		{commentAllowLo, `-i lo -j ACCEPT`},
	}
	for _, rule := range rules {
		lines = append(lines, removeInputRuleByComment(rule.comment, rule.args))
		lines = append(lines, removeLegacyInputRule(rule.args))
	}
	lines = append(lines, removeInputRuleByComment(StrictWhitelistComment, `-j DROP`))
	return strings.Join(lines, "\n")
}
