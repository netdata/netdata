// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	defaultUpdateEvery = 30
	defaultTimeout     = 15 * time.Second

	transportDCOM  = "dcom"
	transportWinRM = "winrm"
	authNTLM       = "ntlm"
	authKerberos   = "kerberos"

	defaultKDCPort = "88"
)

type Config struct {
	Vnode              string                      `yaml:"vnode,omitempty"               json:"vnode,omitempty"`
	UpdateEvery        int                         `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectionRetry int                         `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Host               string                      `yaml:"host"                          json:"host"`
	Transport          confopt.Enum[transportSpec] `yaml:"transport,omitempty"           json:"transport,omitempty"`
	TLSCA              string                      `yaml:"tls_ca,omitempty"              json:"tls_ca,omitempty"`
	Auth               string                      `yaml:"auth"                          json:"auth"`
	Username           string                      `yaml:"username"                      json:"username"`
	Password           string                      `yaml:"password"                      json:"password"`
	KDC                []string                    `yaml:"kdc,omitempty"                 json:"kdc,omitempty"`
	Timeout            confopt.Duration            `yaml:"timeout,omitempty"             json:"timeout,omitempty"`
	Collectors         Collectors                  `yaml:"collectors,omitempty"          json:"collectors,omitzero"`
}

type transportSpec struct{}

func (transportSpec) Values() []string { return []string{transportDCOM, transportWinRM} }
func (transportSpec) Default() string  { return transportDCOM }

// Collectors switches the metric groups. An omitted group is enabled. Pointers
// keep an explicit false distinct from omission when the framework decodes or
// reserializes a job.
type Collectors struct {
	CPU     *bool `yaml:"cpu,omitempty"     json:"cpu,omitempty"`
	Memory  *bool `yaml:"memory,omitempty"  json:"memory,omitempty"`
	System  *bool `yaml:"system,omitempty"  json:"system,omitempty"`
	Network *bool `yaml:"network,omitempty" json:"network,omitempty"`
	Disk    *bool `yaml:"disk,omitempty"    json:"disk,omitempty"`
	SMB     *bool `yaml:"smb,omitempty"     json:"smb,omitempty"`
	Time    *bool `yaml:"time,omitempty"    json:"time,omitempty"`
}

func groupEnabled(option *bool) bool { return option == nil || *option }

func defaultConfig() Config {
	return Config{
		UpdateEvery: defaultUpdateEvery,
		Transport:   transportDCOM,
		Timeout:     confopt.Duration(defaultTimeout),
	}
}

// validate normalizes host and transport and reports the first invalid option.
// The query transports rely on it and do not validate their inputs again. Its
// messages never quote a configured value, so they are all public.
func (c *Config) validate() error {
	c.Host = strings.TrimSpace(c.Host)
	if !validHost(c.Host) {
		return publicError("host must be a Windows hostname or IP address without a URL or port")
	}
	if err := c.Transport.Validate(); err != nil {
		// The validation error quotes the configured value, so it stays private.
		return dyncfg.NewPublicError("transport must be dcom or winrm", fmt.Errorf("transport %w", err))
	}
	c.Transport = c.Transport.Normalized()
	if c.TLSCA != "" && c.Transport != transportWinRM {
		return publicError("tls_ca requires the winrm transport")
	}
	if c.Auth != authNTLM && c.Auth != authKerberos {
		return publicError("auth must be ntlm or kerberos")
	}
	if logonAccount(c.Username) == "" {
		return publicError("username is required")
	}
	if c.Password == "" {
		return publicError("password is required")
	}
	if err := c.validateKerberos(); err != nil {
		return err
	}
	if c.Timeout <= 0 {
		return publicError("timeout must be positive")
	}
	groups := c.Collectors
	if !groupEnabled(groups.CPU) && !groupEnabled(groups.Memory) && !groupEnabled(groups.System) &&
		!groupEnabled(groups.Network) && !groupEnabled(groups.Disk) && !groupEnabled(groups.SMB) &&
		!groupEnabled(groups.Time) {
		return publicError("at least one collector group must be enabled")
	}
	return nil
}

// validateKerberos checks the options that Kerberos needs or that only it uses.
func (c *Config) validateKerberos() error {
	if c.Auth != authKerberos {
		if len(c.KDC) != 0 {
			return publicError("kdc requires auth kerberos")
		}
		return nil
	}
	if c.Transport != transportDCOM {
		return publicError("auth kerberos requires the dcom transport")
	}
	// Active Directory registers service principals for host names, not
	// addresses.
	if _, err := netip.ParseAddr(c.Host); err == nil {
		return publicError("auth kerberos requires host to be the Windows host's DNS name, not an IP address")
	}
	if _, _, ok := kerberosPrincipal(c.Username); !ok {
		return publicError("auth kerberos requires username as user@domain, with the Active Directory DNS domain")
	}
	if _, ok := kdcAddresses(c.KDC); !ok {
		return publicError("kdc entries must be host names or IP addresses, each with an optional port")
	}
	return nil
}

// kerberosPrincipal splits user@domain into the account and its realm, the
// uppercase Active Directory DNS domain.
func kerberosPrincipal(username string) (account, realm string, ok bool) {
	account, domain, found := strings.Cut(strings.TrimSpace(username), "@")
	account = strings.TrimSpace(account)
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if !found || account == "" || strings.ContainsAny(account, `\/@`) || !validDomain(domain) {
		return "", "", false
	}
	return account, strings.ToUpper(domain), true
}

// validDomain accepts a DNS name of letters, digits, hyphens and dots.
func validDomain(domain string) bool {
	if domain == "" {
		return false
	}
	for label := range strings.SplitSeq(domain, ".") {
		if label == "" || strings.IndexFunc(label, func(r rune) bool {
			return !(r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
		}) >= 0 {
			return false
		}
	}
	return true
}

// kdcAddresses returns each KDC as host:port, with the Kerberos port when the
// entry has none.
func kdcAddresses(entries []string) ([]string, bool) {
	addresses := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		host, port, err := net.SplitHostPort(entry)
		if err != nil {
			// A bare host name or IP address, IPv6 included.
			host, port = entry, defaultKDCPort
		}
		if number, err := strconv.ParseUint(port, 10, 16); err != nil || number == 0 || !validHost(host) {
			return nil, false
		}
		addresses = append(addresses, net.JoinHostPort(host, port))
	}
	return addresses, true
}

// validHost accepts hostnames and IP literals, including scoped IPv6 addresses.
func validHost(host string) bool {
	if host == "" || host == "." || strings.ContainsAny(host, " /\\?#@[]") ||
		strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	return !strings.ContainsAny(host, ":%")
}

// logonAccount returns the account part of a DOMAIN\user, user@domain or plain logon name.
func logonAccount(username string) string {
	if _, account, ok := strings.Cut(username, `\`); ok {
		return strings.TrimSpace(account)
	}
	account, _, _ := strings.Cut(username, "@")
	return strings.TrimSpace(account)
}
