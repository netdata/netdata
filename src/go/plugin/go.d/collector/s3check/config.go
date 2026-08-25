// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	defaultUpdateEvery = 90
	defaultPrefix      = "netdata-s3check/"
	defaultMaxRetries  = 1
	defaultTimeout     = time.Second * 2
	// Keep retry delay small and include the SDK retry-after ceiling in deadlines.
	maxRetryBackoff = time.Second
	maxRetryAfter   = 5 * time.Second

	maxRetries             = 2
	maxTimeout             = time.Second * 10
	probePayloadBytes      = 4096
	cleanupBatchSize       = 3
	maxCleanupListKeys     = cleanupBatchSize + 1
	maxNormalAPIOperations = 8 // prefix LIST, PUT, GET, LIST, DELETE, HEAD, retry DELETE, final HEAD
	shutdownAPIOperations  = 3 // HEAD, DELETE, final HEAD
	minBucketNameLength    = 3
	maxBucketNameLength    = 63
	maxPrefixLength        = 256
	maxLatencyThresholdMS  = int64(3600000)
)

var bucketNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var probeKeyRE = regexp.MustCompile(`^probe-[0-9]{1,20}-[a-f0-9]{16}\.bin$`)

type Config struct {
	Vnode              string `yaml:"vnode,omitempty"              json:"vnode,omitempty"`
	UpdateEvery        int    `yaml:"update_every,omitempty"       json:"update_every,omitempty"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`

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

	web.ClientConfig `yaml:",inline" json:""`
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

func (c *Collector) validateConfig() error {
	switch {
	case c.Endpoint == "":
		return errors.New("endpoint is not set")
	case c.Region == "":
		return errors.New("region is not set")
	case c.Bucket == "":
		return errors.New("bucket is not set")
	case c.AccessKeyID == "":
		return errors.New("access_key_id is not set")
	case c.SecretAccessKey == "":
		return errors.New("secret_access_key is not set")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return errors.New("endpoint must be an absolute HTTP(S) URL")
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return errors.New("endpoint must use http or https")
	}
	if endpoint.User != nil || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("endpoint must contain only a scheme and host, without credentials, path, query, or fragment")
	}
	c.Endpoint = strings.TrimSuffix(c.Endpoint, "/")

	if !isValidBucketName(c.Bucket) {
		return errors.New("bucket is not a valid S3 bucket name")
	}

	if c.Prefix == "" {
		c.Prefix = defaultPrefix
	}
	if len(c.Prefix) > maxPrefixLength || strings.HasPrefix(c.Prefix, "/") || !strings.HasSuffix(c.Prefix, "/") ||
		strings.Contains(c.Prefix, "//") {
		return errors.New("prefix must end with '/', must not start with '/', and must not contain an empty segment")
	}
	for _, part := range strings.Split(strings.Trim(c.Prefix, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("prefix must not contain '.', '..', or empty segments")
		}
	}

	if c.Timeout.Duration() < 500*time.Millisecond || c.Timeout.Duration() > maxTimeout {
		return errors.New("timeout must be between 0.5 and 10 seconds")
	}
	if c.MaxRetries < 0 || c.MaxRetries > maxRetries {
		return fmt.Errorf("max_retries must be between 0 and %d", maxRetries)
	}
	if c.LatencyThresholdMS < 0 || c.LatencyThresholdMS > maxLatencyThresholdMS {
		return fmt.Errorf("latency_threshold_ms must be between 0 and %d", maxLatencyThresholdMS)
	}

	if c.UpdateEvery <= 0 {
		c.UpdateEvery = defaultUpdateEvery
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
	attempts := c.MaxRetries + 1
	retries := c.MaxRetries
	return c.Timeout.Duration()*time.Duration(attempts) +
		time.Duration(retries)*(maxRetryBackoff+maxRetryAfter)
}

func (c *Collector) worstCaseDuration() time.Duration {
	return c.perOperationDuration() * time.Duration(maxNormalAPIOperations)
}

func (c *Collector) shutdownCleanupDuration() time.Duration {
	return c.perOperationDuration() * time.Duration(shutdownAPIOperations)
}
