// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	modeSingle    = "single"
	modeMultisite = "multisite"

	defaultUpdateEvery = 120
	defaultPrefix      = "netdata-s3check/"
	defaultMaxRetries  = 1
	defaultTimeout     = time.Second * 2
	// Keep retry delay small and include the SDK retry-after ceiling in deadlines.
	maxRetryBackoff = time.Second
	maxRetryAfter   = 5 * time.Second

	cycleProcessingMargin = 5 * time.Second
	jobLockPollInterval   = 20 * time.Millisecond

	defaultRPOThresholdMS       = int64(15 * time.Minute / time.Millisecond)
	defaultReplicationTimeoutMS = int64(30 * time.Minute / time.Millisecond)
	defaultDeleteThresholdMS    = int64(5 * time.Minute / time.Millisecond)
	defaultDeleteTimeoutMS      = int64(15 * time.Minute / time.Millisecond)
	maxRetries                  = 2
	maxTimeout                  = time.Second * 10
	probePayloadBytes           = 4096
	cleanupBatchSize            = 2
	maxNormalAPIOperations      = 11 // owner LIST plus two keyed version/DELETE/absence-proof cleanups
	maxMultisiteAPIOperations   = 11 // immediate lifecycle; reconciliation and cleanup remain within the same bound
	shutdownAPIOperations       = 5  // HEAD, DELETE, final HEAD, bucket proof
	multisiteShutdownOperations = 8  // active exact keys only; pending batches recover on the next runtime cycle
	destructiveRetryOperations  = 6  // two keyed proofs/DELETEs plus both final owner-prefix LIST proofs
	minBucketNameLength         = 3
	maxBucketNameLength         = 63
	maxPrefixLength             = 256
	maxLatencyThresholdMS       = int64(3600000)
	maxMultisiteObjectiveMS     = int64(24 * time.Hour / time.Millisecond)
	minSiteLabelLength          = 1
	maxSiteLabelLength          = 64
)

var (
	bucketNameRE        = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	multisiteProbeKeyRE = regexp.MustCompile(`^probe-[0-9]{1,20}-[a-f0-9]{16}-[a-f0-9]{32}\.bin$`)
	siteLabelRE         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

type Config struct {
	Name               string `yaml:"name,omitempty"                json:"name,omitempty"`
	Vnode              string `yaml:"vnode,omitempty"              json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty"       json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

	Mode        string             `yaml:"mode,omitempty" json:"mode,omitempty"`
	SourceSite  string             `yaml:"source_site,omitempty" json:"source_site,omitempty"`
	Destination *DestinationConfig `yaml:"destination,omitempty" json:"destination,omitempty"`

	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Region   string `yaml:"region"   json:"region"`
	Bucket   string `yaml:"bucket"   json:"bucket"`
	Prefix   string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`

	PathStyle          bool  `yaml:"path_style,omitempty" json:"path_style,omitempty"`
	MaxRetries         int   `yaml:"max_retries,omitempty" json:"max_retries,omitempty"`
	LatencyThresholdMS int64 `yaml:"latency_threshold_ms,omitempty" json:"latency_threshold_ms,omitempty"`

	RPOThresholdMS       int64 `yaml:"rpo_threshold_ms,omitempty" json:"rpo_threshold_ms,omitempty"`
	ReplicationTimeoutMS int64 `yaml:"replication_timeout_ms,omitempty" json:"replication_timeout_ms,omitempty"`
	DeleteThresholdMS    int64 `yaml:"delete_threshold_ms,omitempty" json:"delete_threshold_ms,omitempty"`
	DeleteTimeoutMS      int64 `yaml:"delete_timeout_ms,omitempty" json:"delete_timeout_ms,omitempty"`
	VerifyDelete         bool  `yaml:"verify_delete" json:"verify_delete"`

	web.ClientConfig `yaml:",inline" json:""`
}

type DestinationConfig struct {
	Site string `yaml:"site" json:"site"`

	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Region   string `yaml:"region"   json:"region"`
	Bucket   string `yaml:"bucket"   json:"bucket"`
	Prefix   string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	AccessKeyID     string `yaml:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`

	PathStyle         *bool            `yaml:"path_style,omitempty" json:"path_style,omitempty"`
	Timeout           confopt.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	NotFollowRedirect *bool            `yaml:"not_follow_redirects,omitempty" json:"not_follow_redirects,omitempty"`
	ProxyURL          string           `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	tlscfg.TLSConfig  `yaml:",inline"    json:""`
	ForceHTTP2        *bool `yaml:"force_http2,omitempty" json:"-"`
}

type endpointConfig struct {
	Endpoint string
	Region   string
	Bucket   string
	Prefix   string

	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	PathStyle bool

	web.ClientConfig
}

func isValidBucketName(bucket string) bool {
	if len(bucket) < minBucketNameLength || len(bucket) > maxBucketNameLength || !bucketNameRE.MatchString(bucket) {
		return false
	}
	if strings.Contains(bucket, "..") || strings.Contains(bucket, ".-") || strings.Contains(bucket, "-.") {
		return false
	}
	if net.ParseIP(bucket) != nil {
		return false
	}
	return true
}

func isValidSiteLabel(site string) bool {
	return len(site) >= minSiteLabelLength && len(site) <= maxSiteLabelLength && siteLabelRE.MatchString(site)
}

func (c *Collector) sourceEndpoint() endpointConfig {
	return endpointConfig{
		Endpoint:        c.Endpoint,
		Region:          c.Region,
		Bucket:          c.Bucket,
		Prefix:          c.Prefix,
		AccessKeyID:     c.AccessKeyID,
		SecretAccessKey: c.SecretAccessKey,
		SessionToken:    c.SessionToken,
		PathStyle:       c.PathStyle,
		ClientConfig:    c.ClientConfig,
	}
}

func destinationEndpointConfig(destination DestinationConfig) endpointConfig {
	pathStyle := true
	if destination.PathStyle != nil {
		pathStyle = *destination.PathStyle
	}
	rejectRedirects := true
	if destination.NotFollowRedirect != nil {
		rejectRedirects = *destination.NotFollowRedirect
	}
	timeout := destination.Timeout
	if timeout == 0 {
		timeout = confopt.Duration(defaultTimeout)
	}
	return endpointConfig{
		Endpoint:        destination.Endpoint,
		Region:          destination.Region,
		Bucket:          destination.Bucket,
		Prefix:          destination.Prefix,
		AccessKeyID:     destination.AccessKeyID,
		SecretAccessKey: destination.SecretAccessKey,
		SessionToken:    destination.SessionToken,
		PathStyle:       pathStyle,
		ClientConfig: web.ClientConfig{
			Timeout:           timeout,
			NotFollowRedirect: rejectRedirects,
			ProxyURL:          destination.ProxyURL,
			TLSConfig:         destination.TLSConfig,
		},
	}
}

func validateProxyURL(raw, label string) error {
	if raw == "" {
		return nil
	}
	proxy, err := url.Parse(raw)
	if err != nil || proxy.Scheme == "" || proxy.Host == "" {
		return fmt.Errorf("%s proxy_url is not a valid absolute URL", label)
	}
	return nil
}

func validateEndpointConfig(endpoint *endpointConfig, label string) error {
	switch {
	case endpoint.Endpoint == "":
		return fmt.Errorf("%s endpoint is not set", label)
	case endpoint.Region == "":
		return fmt.Errorf("%s region is not set", label)
	case endpoint.Bucket == "":
		return fmt.Errorf("%s bucket is not set", label)
	case endpoint.AccessKeyID == "":
		return fmt.Errorf("%s access_key_id is not set", label)
	case endpoint.SecretAccessKey == "":
		return fmt.Errorf("%s secret_access_key is not set", label)
	}

	parsed, err := url.Parse(endpoint.Endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s endpoint must be an absolute HTTP(S) URL", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s endpoint must use http or https", label)
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf(
			"%s endpoint must contain only a scheme and host, without credentials, path, query, or fragment",
			label,
		)
	}
	endpoint.Endpoint = strings.TrimSuffix(endpoint.Endpoint, "/")

	if !isValidBucketName(endpoint.Bucket) {
		return fmt.Errorf("%s bucket is not a valid S3 bucket name", label)
	}

	if endpoint.Prefix == "" {
		endpoint.Prefix = defaultPrefix
	}
	if len(endpoint.Prefix) > maxPrefixLength || strings.HasPrefix(endpoint.Prefix, "/") || !strings.HasSuffix(endpoint.Prefix, "/") ||
		strings.Contains(endpoint.Prefix, "//") {
		return fmt.Errorf("%s prefix must end with '/', must not start with '/', and must not contain an empty segment", label)
	}
	for part := range strings.SplitSeq(strings.Trim(endpoint.Prefix, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%s prefix must not contain '.', '..', or empty segments", label)
		}
	}

	if endpoint.Timeout.Duration() < 500*time.Millisecond || endpoint.Timeout.Duration() > maxTimeout {
		return fmt.Errorf("%s timeout must be between 0.5 and 10 seconds", label)
	}
	if err := validateProxyURL(endpoint.ProxyURL, label); err != nil {
		return err
	}
	return nil
}

func canonicalEndpointKey(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := canonicalConfigHostname(parsed.Hostname())
	port := canonicalPort(scheme, parsed.Port())
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return scheme + "://" + host
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

func canonicalConfigHostname(host string) string {
	host = strings.TrimSuffix(host, ".")
	// The durable route fingerprint keeps the operator's interface-name spelling.
	// Runtime interface indexes can change across reboot, so they are resolved only
	// for same-service validation, never for persisted ownership identity.
	host = strings.Replace(host, "%25", "%", 1)
	address, zone, hasZone := strings.Cut(host, "%")
	if hasZone && zone != "" {
		if ip := net.ParseIP(strings.ToLower(address)); ip != nil && ip.To4() == nil {
			if number, numErr := strconv.ParseUint(zone, 10, 32); numErr == nil {
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

func canonicalRuntimeHostname(host string) string {
	host = strings.TrimSuffix(host, ".")
	// RFC 6874 URLs escape the IPv6 zone separator as %25. net.ParseIP rejects
	// zoned addresses, and the zone itself is case-sensitive on many platforms.
	host = strings.Replace(host, "%25", "%", 1)
	address, zone, hasZone := strings.Cut(host, "%")
	if hasZone && zone != "" {
		if ip := net.ParseIP(strings.ToLower(address)); ip != nil && ip.To4() == nil {
			// Go resolves an interface name to its index and otherwise interprets
			// the textual zone as a decimal index.
			if iface, nameErr := net.InterfaceByName(zone); nameErr == nil {
				zone = strconv.FormatUint(uint64(iface.Index), 10)
			} else if number, numErr := strconv.ParseUint(zone, 10, 32); numErr == nil {
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

func endpointBucketKey(endpoint endpointConfig) string {
	parsed, _ := url.Parse(endpoint.Endpoint)
	host := canonicalRuntimeHostname(parsed.Hostname())
	bucketPrefix := strings.ToLower(endpoint.Bucket) + "."
	if !endpoint.PathStyle && strings.HasPrefix(host, bucketPrefix) {
		host = strings.TrimPrefix(host, bucketPrefix)
	}
	port := canonicalPort(strings.ToLower(parsed.Scheme), parsed.Port())
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(host, port) + "/" + endpoint.Bucket
}

func (c *Collector) validateConfig() error {
	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
	}
	if c.Mode == "" {
		c.Mode = modeSingle
	}
	if c.Mode != modeSingle && c.Mode != modeMultisite {
		return fmt.Errorf("mode must be %s or %s", modeSingle, modeMultisite)
	}

	if c.ClientConfig.ForceHTTP2 {
		return errors.New("force_http2 is not supported because it bypasses proxy configuration")
	}

	source := c.sourceEndpoint()
	if err := validateEndpointConfig(&source, "source"); err != nil {
		return err
	}
	c.Endpoint, c.Region, c.Bucket, c.Prefix = source.Endpoint, source.Region, source.Bucket, source.Prefix
	c.PathStyle = source.PathStyle
	c.ClientConfig = source.ClientConfig

	if c.Mode == modeSingle {
		if c.Destination != nil {
			return errors.New("destination is accepted only in multisite mode")
		}
	} else {
		if c.Name == "" {
			return errors.New("name is not set")
		}
		if c.Destination == nil {
			return errors.New("destination is not set")
		}
		if !isValidSiteLabel(c.SourceSite) {
			return errors.New("source_site must be 1-64 characters and start with a letter or digit")
		}
		if !isValidSiteLabel(c.Destination.Site) {
			return errors.New("destination.site must be 1-64 characters and start with a letter or digit")
		}
		if c.Destination.ForceHTTP2 != nil && *c.Destination.ForceHTTP2 {
			return errors.New("destination force_http2 is not supported because it bypasses proxy configuration")
		}
		if strings.EqualFold(c.SourceSite, c.Destination.Site) {
			return errors.New("source_site and destination.site must identify different sites")
		}
		destination := c.destinationEndpoint()
		if err := validateEndpointConfig(&destination, "destination"); err != nil {
			return err
		}
		c.Destination.Endpoint, c.Destination.Region = destination.Endpoint, destination.Region
		c.Destination.Bucket, c.Destination.Prefix = destination.Bucket, destination.Prefix
		c.Destination.Timeout = destination.Timeout
		if c.Destination.PathStyle == nil {
			c.Destination.PathStyle = &destination.PathStyle
		}
		if c.Destination.NotFollowRedirect == nil {
			c.Destination.NotFollowRedirect = &destination.NotFollowRedirect
		}
		if endpointBucketKey(source) == endpointBucketKey(destination) {
			return errors.New("source and destination must not use the same endpoint and bucket")
		}

		if c.RPOThresholdMS <= 0 || c.RPOThresholdMS > maxMultisiteObjectiveMS {
			return fmt.Errorf("rpo_threshold_ms must be between 1 and %d", maxMultisiteObjectiveMS)
		}
		if c.ReplicationTimeoutMS <= 0 || c.ReplicationTimeoutMS > maxMultisiteObjectiveMS {
			return fmt.Errorf("replication_timeout_ms must be between 1 and %d", maxMultisiteObjectiveMS)
		}
		if c.RPOThresholdMS > c.ReplicationTimeoutMS {
			return errors.New("rpo_threshold_ms must not exceed replication_timeout_ms")
		}
		if c.RPOThresholdMS < int64(c.UpdateEvery)*1000 {
			return errors.New("rpo_threshold_ms must be at least update_every because visibility is polled once per cycle")
		}
		if c.DeleteThresholdMS <= 0 || c.DeleteThresholdMS > maxMultisiteObjectiveMS {
			return fmt.Errorf("delete_threshold_ms must be between 1 and %d", maxMultisiteObjectiveMS)
		}
		if c.DeleteTimeoutMS <= 0 || c.DeleteTimeoutMS > maxMultisiteObjectiveMS {
			return fmt.Errorf("delete_timeout_ms must be between 1 and %d", maxMultisiteObjectiveMS)
		}
		if c.DeleteThresholdMS > c.DeleteTimeoutMS {
			return errors.New("delete_threshold_ms must not exceed delete_timeout_ms")
		}
		if c.DeleteThresholdMS < int64(c.UpdateEvery)*1000 {
			return errors.New("delete_threshold_ms must be at least update_every because disappearance is polled once per cycle")
		}
	}

	if c.MaxRetries < 0 || c.MaxRetries > maxRetries {
		return fmt.Errorf("max_retries must be between 0 and %d", maxRetries)
	}
	if c.LatencyThresholdMS < 0 || c.LatencyThresholdMS > maxLatencyThresholdMS {
		return fmt.Errorf("latency_threshold_ms must be between 0 and %d", maxLatencyThresholdMS)
	}

	if c.worstCaseDuration() > time.Duration(c.UpdateEvery)*time.Second {
		return fmt.Errorf(
			"worst-case probe duration %s does not fit update_every %ds; increase update_every or reduce timeout/max_retries",
			c.worstCaseDuration(), c.UpdateEvery,
		)
	}
	return nil
}

func (c *Collector) perOperationDuration() time.Duration {
	return operationDeadline(c.Timeout.Duration(), c.MaxRetries)
}

func operationDeadline(timeout time.Duration, retries int) time.Duration {
	attempts := retries + 1
	return timeout*time.Duration(attempts) + time.Duration(retries)*(maxRetryBackoff+maxRetryAfter)
}

func (c *Collector) worstCaseDuration() time.Duration {
	deadline := operationDeadline(c.Timeout.Duration(), c.MaxRetries)
	operations := maxNormalAPIOperations
	if c.Mode == modeMultisite {
		destinationDeadline := operationDeadline(c.Destination.Timeout.Duration(), c.MaxRetries)
		deadline = max(deadline, destinationDeadline)
		operations = maxMultisiteAPIOperations
	}
	return deadline*time.Duration(operations) + cycleProcessingMargin
}

func (c *Collector) multisiteDestructiveRetryGrace() time.Duration {
	deadline := operationDeadline(c.Timeout.Duration(), c.MaxRetries)
	if destinationDeadline := operationDeadline(c.Destination.Timeout.Duration(), c.MaxRetries); destinationDeadline > deadline {
		deadline = destinationDeadline
	}
	return deadline*time.Duration(destructiveRetryOperations) + cycleProcessingMargin
}

func (c *Collector) multisiteCleanupHorizon() time.Duration {
	horizon := time.Duration(c.ReplicationTimeoutMS) * time.Millisecond
	if deleteHorizon := time.Duration(c.DeleteTimeoutMS) * time.Millisecond; deleteHorizon > horizon {
		horizon = deleteHorizon
	}
	if minimum := time.Duration(c.UpdateEvery) * time.Second; minimum > horizon {
		horizon = minimum
	}
	return horizon
}

func (c *Collector) shutdownCleanupDuration() time.Duration {
	deadline := operationDeadline(c.Timeout.Duration(), c.MaxRetries)
	operations := shutdownAPIOperations
	if c.Mode == modeMultisite {
		destinationDeadline := operationDeadline(c.Destination.Timeout.Duration(), c.MaxRetries)
		deadline = max(deadline, destinationDeadline)
		operations = multisiteShutdownOperations
	}
	return deadline*time.Duration(operations) + cycleProcessingMargin
}

func (c *Collector) destinationEndpoint() endpointConfig {
	return destinationEndpointConfig(*c.Destination)
}
