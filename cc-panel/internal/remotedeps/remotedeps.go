package remotedeps

import (
	"strings"
	"time"

	"github.com/example/cc-panel/internal/sshx"
)

const InstallTimeout = 5 * time.Minute

func EnsureFirewallToolsScript() string {
	return asRootShell(ensureFirewallToolsBody())
}

func EnsureFirewallToolsBody() string {
	return strings.TrimPrefix(ensureFirewallToolsBody(), "set -e\n")
}

func ensureFirewallToolsBody() string {
	return strings.Join([]string{
		"set -e",
		`CC_MISSING=""`,
		`for cmd in iptables ipset iptables-save; do`,
		`  command -v "$cmd" >/dev/null 2>&1 || CC_MISSING="$CC_MISSING $cmd"`,
		`done`,
		`if [ -n "$CC_MISSING" ]; then`,
		`  echo "发现缺失依赖:$CC_MISSING，开始自动安装..." >&2`,
		`  SUDO=""`,
		`  if [ "$(id -u)" -ne 0 ]; then`,
		`    command -v sudo >/dev/null 2>&1 || { echo "当前 SSH 用户不是 root，且未安装 sudo，无法自动修复依赖:$CC_MISSING" >&2; exit 127; }`,
		`    SUDO="sudo"`,
		`  fi`,
		`  if command -v apt-get >/dev/null 2>&1; then`,
		`    $SUDO env DEBIAN_FRONTEND=noninteractive apt-get update`,
		`    $SUDO env DEBIAN_FRONTEND=noninteractive apt-get install -y ipset iptables`,
		`  elif command -v dnf >/dev/null 2>&1; then`,
		`    $SUDO dnf install -y ipset iptables`,
		`  elif command -v yum >/dev/null 2>&1; then`,
		`    $SUDO yum install -y ipset iptables`,
		`  else`,
		`    echo "无法识别目标机器包管理器，请手动安装 iptables、ipset、iptables-save" >&2`,
		`    exit 127`,
		`  fi`,
		`fi`,
		`for cmd in iptables ipset iptables-save; do`,
		`  command -v "$cmd" >/dev/null 2>&1 || { echo "$cmd 仍缺失，自动修复失败" >&2; exit 127; }`,
		`done`,
	}, "\n")
}

func WithFirewallTools(command string) string {
	if strings.TrimSpace(command) == "" {
		return EnsureFirewallToolsScript()
	}
	return asRootShell(ensureFirewallToolsBody() + "\n" + command)
}

func AsRoot(command string) string {
	return asRootShell(command)
}

func ScriptBody(body string) string {
	return strings.Join([]string{
		"set -e",
		`if [ "$(id -u)" -ne 0 ]; then`,
		`  command -v sudo >/dev/null 2>&1 || { echo "当前 SSH 用户不是 root，且未安装 sudo，无法自动切换 root" >&2; exit 127; }`,
		`  exec sudo -n /bin/sh -s`,
		`fi`,
		body,
	}, "\n")
}

func WithInstallTimeout(creds sshx.Credentials) sshx.Credentials {
	if creds.Timeout < InstallTimeout {
		creds.Timeout = InstallTimeout
	}
	return creds
}

func asRootShell(script string) string {
	return strings.Join([]string{
		"set -e",
		`if [ "$(id -u)" -eq 0 ]; then`,
		script,
		"else",
		`  command -v sudo >/dev/null 2>&1 || { echo "当前 SSH 用户不是 root，且未安装 sudo，无法自动切换 root" >&2; exit 127; }`,
		`  sudo -n /bin/sh <<'CC_PANEL_ROOT'`,
		script,
		"CC_PANEL_ROOT",
		"fi",
	}, "\n")
}
