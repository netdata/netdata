// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
)

const (
	defaultUpdateEvery = 120
	defaultPrefix      = "netdata-s3check/"
	defaultTimeout     = 10 * time.Second

	defaultWriteObjective  = 15 * time.Minute
	defaultWriteTimeout    = 30 * time.Minute
	defaultDeleteObjective = 5 * time.Minute
	defaultDeleteTimeout   = 15 * time.Minute

	maxRequestTimeout       = time.Minute
	maxProbeHorizon         = 24 * time.Hour
	maxPrefixBytes          = 900
	maxConnectionNameLength = 64
)

type Config struct {
	Name               string `yaml:"name,omitempty"                json:"name,omitempty"`
	Vnode              string `yaml:"vnode,omitempty"               json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	Mode               string                 `yaml:"mode"                           json:"mode"`
	ModeLifecycle      *LifecycleModeConfig   `yaml:"mode_lifecycle,omitempty"       json:"mode_lifecycle,omitempty"`
	ModeCephMultisite  *ReplicationModeConfig `yaml:"mode_ceph_multisite,omitempty"  json:"mode_ceph_multisite,omitempty"`
	ModeAWSReplication *ReplicationModeConfig `yaml:"mode_aws_replication,omitempty" json:"mode_aws_replication,omitempty"`
}

type LifecycleModeConfig struct {
	Prefix string   `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Source S3Config `yaml:"source"           json:"source"`
}

type ReplicationModeConfig struct {
	Prefix      string   `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Source      S3Config `yaml:"source"           json:"source"`
	Destination S3Config `yaml:"destination"      json:"destination"`

	WriteObjective  confopt.LongDuration `yaml:"write_objective,omitempty"  json:"write_objective,omitempty"`
	WriteTimeout    confopt.LongDuration `yaml:"write_timeout,omitempty"    json:"write_timeout,omitempty"`
	DeleteObjective confopt.LongDuration `yaml:"delete_objective,omitempty" json:"delete_objective,omitempty"`
	DeleteTimeout   confopt.LongDuration `yaml:"delete_timeout,omitempty"   json:"delete_timeout,omitempty"`
}

type selectedModeConfig struct {
	Mode        contract.Mode
	Prefix      string
	Source      S3Config
	Destination *S3Config

	WriteObjective  confopt.LongDuration
	WriteTimeout    confopt.LongDuration
	DeleteObjective confopt.LongDuration
	DeleteTimeout   confopt.LongDuration
}

// S3Config describes one source or destination connection. Name is presentation-only.
type S3Config struct {
	Name     string `yaml:"name,omitempty"     json:"name,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Region   string `yaml:"region"             json:"region"`
	Bucket   string `yaml:"bucket"             json:"bucket"`

	Credentials *awsauth.StaticCredentialConfig `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	AssumeRole  *awsauth.AssumeRoleConfig       `yaml:"assume_role,omitempty" json:"assume_role,omitempty"`

	PathStyle        *bool                `yaml:"path_style,omitempty" json:"path_style,omitempty"`
	Timeout          confopt.LongDuration `yaml:"timeout,omitempty"    json:"timeout,omitempty"`
	ProxyURL         string               `yaml:"proxy_url,omitempty"  json:"proxy_url,omitempty"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.Mode == "" {
		c.Mode = string(contract.ModeLifecycle)
	}
	switch contract.Mode(c.Mode) {
	case contract.ModeLifecycle:
		if c.ModeLifecycle == nil {
			c.ModeLifecycle = &LifecycleModeConfig{}
		}
		c.ModeLifecycle.applyDefaults()
	case contract.ModeCephMultisite:
		if c.ModeCephMultisite == nil {
			c.ModeCephMultisite = &ReplicationModeConfig{}
		}
		c.ModeCephMultisite.applyDefaults()
	case contract.ModeAWSReplication:
		if c.ModeAWSReplication == nil {
			c.ModeAWSReplication = &ReplicationModeConfig{}
		}
		c.ModeAWSReplication.applyDefaults()
	}
}

func (c Config) withDefaults() Config {
	result := c
	if c.ModeLifecycle != nil {
		mode := *c.ModeLifecycle
		result.ModeLifecycle = &mode
	}
	if c.ModeCephMultisite != nil {
		mode := *c.ModeCephMultisite
		result.ModeCephMultisite = &mode
	}
	if c.ModeAWSReplication != nil {
		mode := *c.ModeAWSReplication
		result.ModeAWSReplication = &mode
	}
	result.applyDefaults()
	return result
}

func (c *LifecycleModeConfig) applyDefaults() {
	if c.Prefix == "" {
		c.Prefix = defaultPrefix
	}
	c.Source.applyDefaults("source")
}

func (c *ReplicationModeConfig) applyDefaults() {
	if c.Prefix == "" {
		c.Prefix = defaultPrefix
	}
	c.Source.applyDefaults("source")
	c.Destination.applyDefaults("destination")
	if c.WriteObjective.Duration() == 0 {
		c.WriteObjective = confopt.LongDuration(defaultWriteObjective)
	}
	if c.WriteTimeout.Duration() == 0 {
		c.WriteTimeout = confopt.LongDuration(defaultWriteTimeout)
	}
	if c.DeleteObjective.Duration() == 0 {
		c.DeleteObjective = confopt.LongDuration(defaultDeleteObjective)
	}
	if c.DeleteTimeout.Duration() == 0 {
		c.DeleteTimeout = confopt.LongDuration(defaultDeleteTimeout)
	}
}

func (c *S3Config) applyDefaults(name string) {
	if c.Name == "" {
		c.Name = name
	}
	if c.PathStyle == nil {
		c.PathStyle = new(true)
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = confopt.LongDuration(defaultTimeout)
	}
}

func (c *Config) validate() error {
	_, err := c.validatedModeConfig()
	return err
}

func (c *Config) validatedModeConfig() (*selectedModeConfig, error) {
	var errs []error
	// The framework assigns the job name before Init. Direct construction must
	// provide it explicitly because it scopes the durable ownership journal.
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, errors.New("name is required"))
	} else if c.Name != strings.TrimSpace(c.Name) {
		errs = append(errs, errors.New("name must not contain surrounding whitespace"))
	}
	if c.UpdateEvery <= 0 {
		errs = append(errs, errors.New("update_every must be positive"))
	}
	mode := contract.Mode(c.Mode)
	switch mode {
	case contract.ModeLifecycle, contract.ModeCephMultisite, contract.ModeAWSReplication:
	default:
		errs = append(errs, fmt.Errorf(
			"mode %q is invalid: expected %q, %q, or %q",
			c.Mode, contract.ModeLifecycle, contract.ModeCephMultisite, contract.ModeAWSReplication,
		))
	}
	selected, err := c.selectedModeConfig()
	if err != nil {
		errs = append(errs, err)
	}
	if selected != nil {
		if err := selected.validate(c.UpdateEvery); err != nil {
			errs = append(errs, err)
		}
	}
	return selected, errors.Join(errs...)
}

func (c *selectedModeConfig) validate(updateEvery int) error {
	var errs []error
	if err := validatePrefix(c.Prefix); err != nil {
		errs = append(errs, err)
	}
	if err := c.Source.validate("source"); err != nil {
		errs = append(errs, err)
	}
	if c.Destination != nil {
		if err := c.Destination.validate("destination"); err != nil {
			errs = append(errs, err)
		}
		if sameS3Location(c.Source, *c.Destination) {
			errs = append(errs, errors.New("source and destination must identify different S3 locations"))
		}
		if err := c.validateObjectives(updateEvery); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c selectedModeConfig) validateObjectives(updateEvery int) error {
	writeObjective := c.WriteObjective.Duration()
	writeTimeout := c.WriteTimeout.Duration()
	deleteObjective := c.DeleteObjective.Duration()
	deleteTimeout := c.DeleteTimeout.Duration()
	interval := time.Duration(updateEvery) * time.Second
	var errs []error
	if writeObjective < interval || writeObjective > maxProbeHorizon {
		errs = append(errs, fmt.Errorf("write_objective must be between update_every and %s", maxProbeHorizon))
	}
	if writeTimeout < writeObjective || writeTimeout > maxProbeHorizon {
		errs = append(errs, fmt.Errorf("write_timeout must be between write_objective and %s", maxProbeHorizon))
	}
	if deleteObjective < interval || deleteObjective > maxProbeHorizon {
		errs = append(errs, fmt.Errorf("delete_objective must be between update_every and %s", maxProbeHorizon))
	}
	if deleteTimeout < deleteObjective || deleteTimeout > maxProbeHorizon {
		errs = append(errs, fmt.Errorf("delete_timeout must be between delete_objective and %s", maxProbeHorizon))
	}
	return errors.Join(errs...)
}

func (c *Config) selectedModeConfig() (*selectedModeConfig, error) {
	var errs []error
	reject := func(name string, configured bool) {
		if configured {
			errs = append(errs, fmt.Errorf("%s is only valid when mode is %s", name, strings.TrimPrefix(name, "mode_")))
		}
	}

	var selected *selectedModeConfig
	switch contract.Mode(c.Mode) {
	case contract.ModeLifecycle:
		if c.ModeLifecycle == nil {
			errs = append(errs, errors.New("mode_lifecycle is required when mode is lifecycle"))
		} else {
			selected = &selectedModeConfig{
				Mode:   contract.ModeLifecycle,
				Prefix: c.ModeLifecycle.Prefix,
				Source: c.ModeLifecycle.Source,
			}
		}
		reject("mode_ceph_multisite", c.ModeCephMultisite != nil)
		reject("mode_aws_replication", c.ModeAWSReplication != nil)
	case contract.ModeCephMultisite:
		if c.ModeCephMultisite == nil {
			errs = append(errs, errors.New("mode_ceph_multisite is required when mode is ceph_multisite"))
		} else {
			selected = selectedReplicationConfig(contract.ModeCephMultisite, c.ModeCephMultisite)
		}
		reject("mode_lifecycle", c.ModeLifecycle != nil)
		reject("mode_aws_replication", c.ModeAWSReplication != nil)
	case contract.ModeAWSReplication:
		if c.ModeAWSReplication == nil {
			errs = append(errs, errors.New("mode_aws_replication is required when mode is aws_replication"))
		} else {
			selected = selectedReplicationConfig(contract.ModeAWSReplication, c.ModeAWSReplication)
		}
		reject("mode_lifecycle", c.ModeLifecycle != nil)
		reject("mode_ceph_multisite", c.ModeCephMultisite != nil)
	}
	return selected, errors.Join(errs...)
}

func selectedReplicationConfig(mode contract.Mode, config *ReplicationModeConfig) *selectedModeConfig {
	return &selectedModeConfig{
		Mode:            mode,
		Prefix:          config.Prefix,
		Source:          config.Source,
		Destination:     &config.Destination,
		WriteObjective:  config.WriteObjective,
		WriteTimeout:    config.WriteTimeout,
		DeleteObjective: config.DeleteObjective,
		DeleteTimeout:   config.DeleteTimeout,
	}
}

func (c *S3Config) validate(path string) error {
	var errs []error
	if strings.TrimSpace(c.Name) == "" || c.Name != strings.TrimSpace(c.Name) {
		errs = append(errs, fmt.Errorf("%s.name must be non-empty and have no surrounding whitespace", path))
	} else if utf8.RuneCountInString(c.Name) > maxConnectionNameLength {
		errs = append(errs, fmt.Errorf("%s.name must not exceed %d characters", path, maxConnectionNameLength))
	}
	if strings.TrimSpace(c.Region) == "" || c.Region != strings.TrimSpace(c.Region) {
		errs = append(errs, fmt.Errorf("%s.region must be non-empty and have no surrounding whitespace", path))
	}
	if strings.TrimSpace(c.Bucket) == "" || c.Bucket != strings.TrimSpace(c.Bucket) {
		errs = append(errs, fmt.Errorf("%s.bucket must be non-empty and have no surrounding whitespace", path))
	}
	if err := validateEndpoint(c.Endpoint, path+".endpoint"); err != nil {
		errs = append(errs, err)
	}
	if c.Timeout.Duration() <= 0 || c.Timeout.Duration() > maxRequestTimeout {
		errs = append(errs, fmt.Errorf("%s.timeout must be between 1ns and %s", path, maxRequestTimeout))
	}
	if err := validateProxyURL(c.ProxyURL, path+".proxy_url"); err != nil {
		errs = append(errs, err)
	}
	if c.Credentials != nil {
		if err := c.Credentials.ValidateWithPath(path + ".credentials"); err != nil {
			errs = append(errs, err)
		}
	}
	if err := validateAssumeRole(c.AssumeRole, path+".assume_role"); err != nil {
		errs = append(errs, err)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		errs = append(errs, fmt.Errorf("%s.tls_cert and %s.tls_key must be configured together", path, path))
	}
	return errors.Join(errs...)
}

func (c S3Config) credentialConfig() awsauth.CredentialConfig {
	if c.Credentials == nil {
		return awsauth.CredentialConfig{
			Type: awsauth.CredentialTypeDefault,
		}
	}
	return awsauth.CredentialConfig{
		Type:       awsauth.CredentialTypeStatic,
		TypeStatic: c.Credentials,
	}
}

func validatePrefix(prefix string) error {
	switch {
	case prefix == "":
		return errors.New("prefix is required")
	case len(prefix) > maxPrefixBytes:
		return fmt.Errorf("prefix must not exceed %d bytes", maxPrefixBytes)
	case strings.HasPrefix(prefix, "/"):
		return errors.New("prefix must not start with '/'")
	case !strings.HasSuffix(prefix, "/"):
		return errors.New("prefix must end with '/'")
	default:
		return nil
	}
}

func validateEndpoint(value, path string) error {
	if value == "" {
		return nil
	}
	if value != strings.TrimSpace(value) {
		return errors.New(path + " must not contain surrounding whitespace")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New(path + " must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New(path + " must use http or https")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return errors.New(path + " must contain only a scheme and host")
	}
	return nil
}

func validateProxyURL(value, path string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New(path + " must be an absolute URL")
	}
	return nil
}

func validateAssumeRole(role *awsauth.AssumeRoleConfig, path string) error {
	if role == nil {
		return nil
	}
	return role.ValidateWithPath(path)
}

func sameS3Location(left, right S3Config) bool {
	return s3LocationKey(left, true) == s3LocationKey(right, true)
}

func s3LocationKey(config S3Config, resolveZone bool) string {
	parsed, err := url.Parse(config.Endpoint)
	if err != nil {
		return config.Endpoint + "/" + config.Bucket
	}
	host := canonicalHostname(parsed.Hostname(), resolveZone)
	bucketPrefix := strings.ToLower(config.Bucket) + "."
	if !boolValue(config.PathStyle) && strings.HasPrefix(host, bucketPrefix) {
		host = strings.TrimPrefix(host, bucketPrefix)
	}
	scheme := strings.ToLower(parsed.Scheme)
	port := canonicalPort(scheme, parsed.Port())
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port) + "/" + config.Bucket
}

func canonicalPort(scheme, port string) string {
	if port == "" {
		return ""
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return port
	}
	if (scheme == "http" && number == 80) || (scheme == "https" && number == 443) {
		return ""
	}
	return strconv.FormatUint(number, 10)
}

func canonicalHostname(host string, resolveZone bool) string {
	host = strings.TrimSuffix(host, ".")
	host = strings.Replace(host, "%25", "%", 1)
	address, zone, hasZone := strings.Cut(host, "%")
	if hasZone && zone != "" {
		if ip := net.ParseIP(strings.ToLower(address)); ip != nil && ip.To4() == nil {
			if resolveZone {
				if iface, err := net.InterfaceByName(zone); err == nil {
					zone = strconv.FormatUint(uint64(iface.Index), 10)
				}
			}
			if number, err := strconv.ParseUint(zone, 10, 32); err == nil {
				zone = strconv.FormatUint(number, 10)
			}
			return ip.String() + "%" + zone
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(host)
}

func (c *selectedModeConfig) ownershipFingerprint() string {
	parts := []string{
		string(c.Mode),
		s3LocationKey(c.Source, false),
		c.Prefix,
	}
	if c.Destination != nil {
		parts = append(parts, s3LocationKey(*c.Destination, false))
	}
	return journal.Fingerprint(parts...)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
