package iptables

import (
	"strings"
	"testing"
)

func TestDeployScriptIncludesWhitelistBeforeDrops(t *testing.T) {
	script := DeployScript(false)
	whitelist := strings.Index(script, commentWhitelist)
	blacklist := strings.Index(script, commentBlacklist)
	if whitelist == -1 || blacklist == -1 {
		t.Fatalf("DeployScript() missing required rules:\n%s", script)
	}
	// Rules are ensured from lowest to highest priority; whitelist block appears after blacklist in script.
	if whitelist < blacklist {
		t.Fatalf("whitelist ensure should run after blacklist ensure so it ends up first in INPUT chain")
	}
}

func TestDeployScriptCreatesIPSetsBeforeIptablesRules(t *testing.T) {
	script := DeployScript(false)
	createWhitelist := strings.Index(script, "ipset create cc_whitelist")
	ruleWhitelist := strings.Index(script, commentWhitelist)
	if createWhitelist == -1 || ruleWhitelist == -1 {
		t.Fatalf("DeployScript() missing ipset creation or iptables rule:\n%s", script)
	}
	if createWhitelist > ruleWhitelist {
		t.Fatalf("ipset must be created before iptables references it")
	}
}

func TestDeployScriptStrictModeAddsDefaultDrop(t *testing.T) {
	script := DeployScript(true)
	if !strings.Contains(script, StrictWhitelistComment) {
		t.Fatalf("strict DeployScript() missing default drop rule:\n%s", script)
	}
	if !strings.Contains(script, commentAllowEstablished) {
		t.Fatalf("strict DeployScript() missing established accept rule:\n%s", script)
	}
}

func TestDeployScriptNonStrictRemovesDefaultDrop(t *testing.T) {
	script := DeployScript(false)
	if strings.Contains(script, ensureAppendRule(StrictWhitelistComment, `-j DROP`)) {
		t.Fatalf("non-strict DeployScript() must not append default drop rule:\n%s", script)
	}
	if !strings.Contains(script, removeStrictWhitelistRules()) {
		t.Fatalf("non-strict DeployScript() should remove strict-only rules when disabled:\n%s", script)
	}
}

func TestDeployScriptEnsuresMissingRulesOnly(t *testing.T) {
	script := DeployScript(false)
	if !strings.Contains(script, "if ! iptables -C INPUT -m comment --comment") {
		t.Fatalf("DeployScript() should check existing rules before insert:\n%s", script)
	}
	if !strings.Contains(script, "if ! iptables -C INPUT -m set --match-set cc_whitelist") {
		t.Fatalf("DeployScript() should accept legacy rules without comment:\n%s", script)
	}
}

func TestStopScriptRemovesAllRules(t *testing.T) {
	script := StopScript()
	required := []string{
		commentWhitelist,
		commentGeoWhitelist,
		commentBlacklist,
		commentTempBlock,
		commentGeoBlock,
		commentRateBlock,
		StrictWhitelistComment,
		commentAllowEstablished,
		commentAllowLo,
	}
	for _, comment := range required {
		if !strings.Contains(script, comment) {
			t.Fatalf("StopScript() missing rule removal for %q:\n%s", comment, script)
		}
	}
	if !strings.Contains(script, "iptables -D INPUT -m set --match-set cc_whitelist") {
		t.Fatalf("StopScript() should remove legacy rules without comment:\n%s", script)
	}
}

func TestDeployScriptStrictModeDoesNotRemoveStrictRulesBeforeEnsure(t *testing.T) {
	script := InitScript(true)
	if strings.Contains(script, removeStrictWhitelistRules()) {
		t.Fatalf("strict InitScript() must not remove strict rules before ensuring them:\n%s", script)
	}
}
