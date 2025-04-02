package journaldexporter

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

func getBootID() string {
	switch runtime.GOOS {
	case "linux":
		if bs, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
			return strings.TrimSpace(string(bs))
		}
	case "darwin", "dragonfly", "freebsd", "netbsd", "openbsd":
		cmd := exec.Command("sysctl", "kern.boottime")
		if bs, err := cmd.Output(); err == nil && len(bs) > 0 {
			return uuid.NewSHA1(uuid.NameSpaceDNS, bs).String()
		}
	case "windows":
		cmd := exec.Command("powershell", "-Command",
			"(Get-CimInstance -ClassName win32_operatingsystem).LastBootUpTime.ToString('o')")
		if bs, err := cmd.Output(); err == nil && len(bs) > 0 {
			return uuid.NewSHA1(uuid.NameSpaceDNS, bs).String()
		}

		cmd = exec.Command("wmic", "os", "get", "LastBootUpTime")
		if bs, err := cmd.Output(); err == nil && len(bs) > 0 {
			return uuid.NewSHA1(uuid.NameSpaceDNS, bs).String()
		}
	}

	return uuid.NewString()
}

func getMachineID() string {
	switch runtime.GOOS {
	case "linux":
		if bs, err := os.ReadFile("/etc/machine-id"); err == nil {
			return strings.TrimSpace(string(bs))
		}
	case "dragonfly", "freebsd", "netbsd", "openbsd":
		if bs, err := os.ReadFile("/etc/hostid"); err == nil {
			return strings.TrimSpace(string(bs))
		}
	case "windows":
		cmd := exec.Command("powershell", "-Command",
			"Get-ItemProperty -Path 'HKLM:\\SOFTWARE\\Microsoft\\Cryptography' -Name 'MachineGuid'")
		if bs, err := cmd.Output(); err == nil && len(bs) > 0 {
			re := regexp.MustCompile(`MachineGuid\s+:\s+([0-9a-fA-F-]+)`)
			matches := re.FindSubmatch(bs)
			if len(matches) >= 2 {
				return string(matches[1])
			}
		}

		cmd = exec.Command("wmic", "csproduct", "get", "UUID")
		if bs, err := cmd.Output(); err == nil && len(bs) > 0 {
			lines := strings.Split(string(bs), "\n")
			if len(lines) > 1 {
				return strings.TrimSpace(lines[1])
			}
		}
	}

	hostname, _ := os.Hostname()

	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(hostname)).String()
}
