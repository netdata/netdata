// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

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

	maxRequestTimeout = time.Minute
	maxProbeHorizon   = 24 * time.Hour
	maxPrefixBytes    = 900
)

type Config struct {
	Name               string `yaml:"name,omitempty" json:"name,omitempty"`
	Vnode              string `yaml:"vnode,omitempty" json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	Mode   string   `yaml:"mode" json:"mode"`
	Prefix string   `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Source S3Config `yaml:"source" json:"source"`

	Destination *S3Config `yaml:"destination,omitempty" json:"destination,omitempty"`

	WriteObjective  confopt.LongDuration `yaml:"write_objective,omitempty" json:"write_objective,omitempty"`
	WriteTimeout    confopt.LongDuration `yaml:"write_timeout,omitempty" json:"write_timeout,omitempty"`
	DeleteObjective confopt.LongDuration `yaml:"delete_objective,omitempty" json:"delete_objective,omitempty"`
	DeleteTimeout   confopt.LongDuration `yaml:"delete_timeout,omitempty" json:"delete_timeout,omitempty"`
}

// S3Config describes one source or destination connection. Name is presentation-only.
type S3Config struct {
	Name     string `yaml:"name,omitempty" json:"name,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
	Region   string `yaml:"region" json:"region"`
	Bucket   string `yaml:"bucket" json:"bucket"`

	Credentials awsauth.CredentialConfig  `yaml:"credentials" json:"credentials"`
	AssumeRole  *awsauth.AssumeRoleConfig `yaml:"assume_role,omitempty" json:"assume_role,omitempty"`

	PathStyle        *bool                `yaml:"path_style,omitempty" json:"path_style,omitempty"`
	Timeout          confopt.LongDuration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	ProxyURL         string               `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

func (c *Config) applyDefaults() {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.Mode == "" {
		c.Mode = string(contract.ModeLifecycle)
	}
	if c.Prefix == "" {
		c.Prefix = defaultPrefix
	}
	c.Source.applyDefaults("source")
	if c.Destination != nil {
		c.Destination.applyDefaults("destination")
	}
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
	if c.Credentials.Type == "" {
		c.Credentials.Type = awsauth.CredentialTypeDefault
	}
	if c.PathStyle == nil {
		c.PathStyle = new(true)
	}
	if c.Timeout.Duration() == 0 {
		c.Timeout = confopt.LongDuration(defaultTimeout)
	}
}

func (c *Config) validate() error {
	var errs []error
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, errors.New("name is required"))
	} else if c.Name != strings.TrimSpace(c.Name) {
		errs = append(errs, errors.New("name must not contain surrounding whitespace"))
	}
	if c.UpdateEvery <= 0 {
		errs = append(errs, errors.New("update_every must be positive"))
	}
	switch contract.Mode(c.Mode) {
	case contract.ModeLifecycle, contract.ModeCephMultisite, contract.ModeAWSReplication:
	default:
		errs = append(errs, fmt.Errorf(
			"mode %q is invalid: expected %q, %q, or %q",
			c.Mode, contract.ModeLifecycle, contract.ModeCephMultisite, contract.ModeAWSReplication,
		))
	}
	if err := validatePrefix(c.Prefix); err != nil {
		errs = append(errs, err)
	}
	if err := c.Source.validate("source"); err != nil {
		errs = append(errs, err)
	}

	if modeUsesDestination(c.Mode) {
		if c.Destination == nil {
			errs = append(errs, errors.New("destination is required for replication modes"))
		} else {
			if err := c.Destination.validate("destination"); err != nil {
				errs = append(errs, err)
			}
			if sameS3Location(c.Source, *c.Destination) {
				errs = append(errs, errors.New("source and destination must identify different S3 locations"))
			}
		}
		if err := c.validateObjectives(); err != nil {
			errs = append(errs, err)
		}
	} else if c.Destination != nil {
		errs = append(errs, errors.New("destination is only valid in replication modes"))
	}
	return errors.Join(errs...)
}

func (c Config) validateObjectives() error {
	writeObjective := c.WriteObjective.Duration()
	writeTimeout := c.WriteTimeout.Duration()
	deleteObjective := c.DeleteObjective.Duration()
	deleteTimeout := c.DeleteTimeout.Duration()
	interval := time.Duration(c.UpdateEvery) * time.Second
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

func (c *S3Config) validate(path string) error {
	var errs []error
	if strings.TrimSpace(c.Name) == "" || c.Name != strings.TrimSpace(c.Name) {
		errs = append(errs, fmt.Errorf("%s.name must be non-empty and have no surrounding whitespace", path))
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
	if err := c.Credentials.ValidateWithPath(path + ".credentials"); err != nil {
		errs = append(errs, err)
	}
	if err := validateAssumeRole(c.AssumeRole, path+".assume_role"); err != nil {
		errs = append(errs, err)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		errs = append(errs, fmt.Errorf("%s.tls_cert and %s.tls_key must be configured together", path, path))
	}
	return errors.Join(errs...)
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
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
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
	var errs []error
	if strings.TrimSpace(role.RoleARN) == "" || role.RoleARN != strings.TrimSpace(role.RoleARN) {
		errs = append(errs, errors.New(path+".role_arn must be non-empty and have no surrounding whitespace"))
	}
	if role.ExternalID != strings.TrimSpace(role.ExternalID) {
		errs = append(errs, errors.New(path+".external_id must not contain surrounding whitespace"))
	}
	return errors.Join(errs...)
}

func modeUsesDestination(mode string) bool {
	return mode == string(contract.ModeCephMultisite) || mode == string(contract.ModeAWSReplication)
}

func sameS3Location(left, right S3Config) bool {
	return canonicalEndpoint(left.Endpoint) == canonicalEndpoint(right.Endpoint) && left.Bucket == right.Bucket
}

func canonicalEndpoint(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}

func (c Config) ownershipFingerprint() string {
	parts := []string{
		c.Mode,
		canonicalEndpoint(c.Source.Endpoint),
		c.Source.Bucket,
		c.Prefix,
	}
	if c.Destination != nil {
		parts = append(parts, canonicalEndpoint(c.Destination.Endpoint), c.Destination.Bucket)
	}
	return journal.Fingerprint(parts...)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
