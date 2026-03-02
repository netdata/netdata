// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"gopkg.in/yaml.v3"
)

const (
	providerIPToASN = "iptoasn"
	providerDBIP    = "dbip_lite"
	providerCustom  = "custom"
)

const (
	defaultUserConfigDir  = "/etc/netdata"
	defaultStockConfigDir = "/usr/lib/netdata/conf.d"
	defaultCacheDir       = "/var/cache/netdata"
)

const (
	formatIPToASNCombinedTSV = "iptoasn_combined_tsv"
	formatDBIPAsnCSV         = "dbip_asn_csv"
	formatDBIPCountryCSV     = "dbip_country_csv"
)

const (
	compressionAuto = "auto"
	compressionNone = "none"
	compressionGzip = "gzip"
	compressionZip  = "zip"
)

type config struct {
	source sourceConfig
	output outputConfig
	policy policyConfig
	http   httpConfig
}

type sourceConfig struct {
	provider string

	combined datasetSpec
	asn      datasetSpec
	country  datasetSpec
}

type datasetSpec struct {
	url         string
	path        string
	format      string
	compression string
}

type outputConfig struct {
	directory    string
	asnFile      string
	countryFile  string
	metadataFile string
}

type policyConfig struct {
	localhostCIDRs   []string
	privateCIDRs     []string
	interestingCIDRs []string
}

type httpConfig struct {
	timeout   time.Duration
	userAgent string
}

type fileConfig struct {
	Source *yamlSourceConfig `yaml:"source"`
	Output *yamlOutputConfig `yaml:"output"`
	Policy *yamlPolicyConfig `yaml:"policy"`
	HTTP   *yamlHTTPConfig   `yaml:"http"`
}

type yamlSourceConfig struct {
	Provider string          `yaml:"provider"`
	Combined yamlDatasetSpec `yaml:"combined"`
	Asn      yamlDatasetSpec `yaml:"asn"`
	Country  yamlDatasetSpec `yaml:"country"`
}

type yamlDatasetSpec struct {
	URL         string `yaml:"url"`
	Path        string `yaml:"path"`
	Format      string `yaml:"format"`
	Compression string `yaml:"compression"`
}

type yamlOutputConfig struct {
	Directory    string `yaml:"directory"`
	AsnFile      string `yaml:"asn_file"`
	CountryFile  string `yaml:"country_file"`
	MetadataFile string `yaml:"metadata_file"`
}

type yamlPolicyConfig struct {
	LocalhostCIDRs   []string `yaml:"localhost_cidrs"`
	PrivateCIDRs     []string `yaml:"private_cidrs"`
	InterestingCIDRs []string `yaml:"interesting_cidrs"`
}

type yamlHTTPConfig struct {
	Timeout   string `yaml:"timeout"`
	UserAgent string `yaml:"user_agent"`
}

func defaultConfig() config {
	return config{
		source: sourceConfig{
			provider: providerIPToASN,
			combined: datasetSpec{
				url:         "https://iptoasn.com/data/ip2asn-combined.tsv.gz",
				format:      formatIPToASNCombinedTSV,
				compression: compressionAuto,
			},
		},
		output: outputConfig{
			directory:    filepath.Join(defaultCacheRoot(), "topology-ip-intel"),
			asnFile:      "topology-ip-asn.mmdb",
			countryFile:  "topology-ip-country.mmdb",
			metadataFile: "topology-ip-intel.json",
		},
		policy: policyConfig{
			localhostCIDRs: []string{
				"127.0.0.0/8",
				"::1/128",
			},
			privateCIDRs: []string{
				"10.0.0.0/8",
				"172.16.0.0/12",
				"192.168.0.0/16",
				"100.64.0.0/10",
				"fc00::/7",
				"fe80::/10",
			},
			interestingCIDRs: []string{},
		},
		http: httpConfig{
			timeout:   2 * time.Minute,
			userAgent: "netdata-topology-ip-intel-downloader/1.0",
		},
	}
}

func loadConfig(explicitPath string) (config, string, error) {
	cfg := defaultConfig()
	path := strings.TrimSpace(explicitPath)
	if path == "" {
		path = discoverDefaultConfigPath()
	}
	if path == "" {
		if err := cfg.normalizeAndValidate(); err != nil {
			return config{}, "", err
		}
		return cfg, "", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := cfg.normalizeAndValidate(); err != nil {
				return config{}, "", err
			}
			return cfg, "", nil
		}
		return config{}, "", fmt.Errorf("failed to read config %s: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(content, &fc); err != nil {
		return config{}, "", fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	if err := cfg.apply(fc); err != nil {
		return config{}, "", err
	}
	if err := cfg.normalizeAndValidate(); err != nil {
		return config{}, "", err
	}
	return cfg, path, nil
}

func discoverDefaultConfigPath() string {
	candidates := []string{
		filepath.Join(defaultUserConfigRoot(), "topology-ip-intel.yaml"),
		filepath.Join(defaultStockConfigRoot(), "topology-ip-intel.yaml"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func defaultUserConfigRoot() string {
	dir := strings.TrimSpace(buildinfo.UserConfigDir)
	if dir == "" {
		return defaultUserConfigDir
	}
	return dir
}

func defaultStockConfigRoot() string {
	dir := strings.TrimSpace(buildinfo.StockConfigDir)
	if dir == "" {
		return defaultStockConfigDir
	}
	return dir
}

func defaultCacheRoot() string {
	dir := strings.TrimSpace(buildinfo.CacheDir)
	if dir == "" {
		return defaultCacheDir
	}
	return dir
}

func (cfg *config) apply(fc fileConfig) error {
	if fc.Source != nil {
		cfg.source = mergeSourceConfig(cfg.source, *fc.Source)
	}
	if fc.Output != nil {
		cfg.output = mergeOutputConfig(cfg.output, *fc.Output)
	}
	if fc.Policy != nil {
		cfg.policy = mergePolicyConfig(cfg.policy, *fc.Policy)
	}
	if fc.HTTP != nil {
		nextHTTP, err := mergeHTTPConfig(cfg.http, *fc.HTTP)
		if err != nil {
			return err
		}
		cfg.http = nextHTTP
	}
	return nil
}

func mergeSourceConfig(dst sourceConfig, src yamlSourceConfig) sourceConfig {
	if src.Provider != "" {
		nextProvider := strings.ToLower(strings.TrimSpace(src.Provider))
		if nextProvider != dst.provider {
			dst = sourceConfig{provider: nextProvider}
		}
		dst.provider = nextProvider
	}
	dst.combined = mergeDatasetSpec(dst.combined, src.Combined)
	dst.asn = mergeDatasetSpec(dst.asn, src.Asn)
	dst.country = mergeDatasetSpec(dst.country, src.Country)
	return dst
}

func mergeDatasetSpec(dst datasetSpec, src yamlDatasetSpec) datasetSpec {
	if src.URL != "" {
		dst.url = strings.TrimSpace(src.URL)
	}
	if src.Path != "" {
		dst.path = strings.TrimSpace(src.Path)
	}
	if src.Format != "" {
		dst.format = strings.ToLower(strings.TrimSpace(src.Format))
	}
	if src.Compression != "" {
		dst.compression = strings.ToLower(strings.TrimSpace(src.Compression))
	}
	return dst
}

func mergeOutputConfig(dst outputConfig, src yamlOutputConfig) outputConfig {
	if src.Directory != "" {
		dst.directory = strings.TrimSpace(src.Directory)
	}
	if src.AsnFile != "" {
		dst.asnFile = strings.TrimSpace(src.AsnFile)
	}
	if src.CountryFile != "" {
		dst.countryFile = strings.TrimSpace(src.CountryFile)
	}
	if src.MetadataFile != "" {
		dst.metadataFile = strings.TrimSpace(src.MetadataFile)
	}
	return dst
}

func mergePolicyConfig(dst policyConfig, src yamlPolicyConfig) policyConfig {
	if src.LocalhostCIDRs != nil {
		dst.localhostCIDRs = cloneTrimmedList(src.LocalhostCIDRs)
	}
	if src.PrivateCIDRs != nil {
		dst.privateCIDRs = cloneTrimmedList(src.PrivateCIDRs)
	}
	if src.InterestingCIDRs != nil {
		dst.interestingCIDRs = cloneTrimmedList(src.InterestingCIDRs)
	}
	return dst
}

func mergeHTTPConfig(dst httpConfig, src yamlHTTPConfig) (httpConfig, error) {
	if src.Timeout != "" {
		timeout, err := time.ParseDuration(strings.TrimSpace(src.Timeout))
		if err != nil {
			return httpConfig{}, fmt.Errorf("invalid http.timeout %q: %w", src.Timeout, err)
		}
		dst.timeout = timeout
	}
	if src.UserAgent != "" {
		dst.userAgent = strings.TrimSpace(src.UserAgent)
	}
	return dst, nil
}

func cloneTrimmedList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func (cfg *config) normalizeAndValidate() error {
	cfg.source.provider = strings.ToLower(strings.TrimSpace(cfg.source.provider))
	if cfg.source.combined.compression == "" {
		cfg.source.combined.compression = compressionAuto
	}
	if cfg.source.asn.compression == "" {
		cfg.source.asn.compression = compressionAuto
	}
	if cfg.source.country.compression == "" {
		cfg.source.country.compression = compressionAuto
	}

	switch cfg.source.provider {
	case providerIPToASN:
		if cfg.source.combined.format == "" {
			cfg.source.combined.format = formatIPToASNCombinedTSV
		}
	case providerDBIP:
		if cfg.source.asn.format == "" {
			cfg.source.asn.format = formatDBIPAsnCSV
		}
		if cfg.source.country.format == "" {
			cfg.source.country.format = formatDBIPCountryCSV
		}
	case providerCustom:
		// no-op
	default:
		return fmt.Errorf("unsupported source.provider %q", cfg.source.provider)
	}

	return cfg.validate()
}

func (cfg config) validate() error {
	switch cfg.source.provider {
	case providerIPToASN:
		if err := validateDatasetSpec("source.combined", cfg.source.combined); err != nil {
			return err
		}
		if cfg.source.combined.format != formatIPToASNCombinedTSV {
			return fmt.Errorf("source.combined.format must be %q for provider %q", formatIPToASNCombinedTSV, providerIPToASN)
		}
	case providerDBIP:
		if err := validateDatasetSpec("source.asn", cfg.source.asn); err != nil {
			return err
		}
		if err := validateDatasetSpec("source.country", cfg.source.country); err != nil {
			return err
		}
		if cfg.source.asn.format != formatDBIPAsnCSV {
			return fmt.Errorf("source.asn.format must be %q for provider %q", formatDBIPAsnCSV, providerDBIP)
		}
		if cfg.source.country.format != formatDBIPCountryCSV {
			return fmt.Errorf("source.country.format must be %q for provider %q", formatDBIPCountryCSV, providerDBIP)
		}
	case providerCustom:
		combinedConfigured := cfg.source.combined.path != "" || cfg.source.combined.url != ""
		asnConfigured := cfg.source.asn.path != "" || cfg.source.asn.url != ""
		countryConfigured := cfg.source.country.path != "" || cfg.source.country.url != ""

		if combinedConfigured {
			if err := validateDatasetSpec("source.combined", cfg.source.combined); err != nil {
				return err
			}
		}
		if asnConfigured {
			if err := validateDatasetSpec("source.asn", cfg.source.asn); err != nil {
				return err
			}
		}
		if countryConfigured {
			if err := validateDatasetSpec("source.country", cfg.source.country); err != nil {
				return err
			}
		}
		if !combinedConfigured && !asnConfigured && !countryConfigured {
			return errors.New("source: provider custom requires at least one configured dataset")
		}
	}

	if strings.TrimSpace(cfg.output.directory) == "" {
		return errors.New("output.directory is required")
	}
	if strings.TrimSpace(cfg.output.asnFile) == "" {
		return errors.New("output.asn_file is required")
	}
	if strings.TrimSpace(cfg.output.countryFile) == "" {
		return errors.New("output.country_file is required")
	}
	if strings.TrimSpace(cfg.output.metadataFile) == "" {
		return errors.New("output.metadata_file is required")
	}

	if cfg.http.timeout <= 0 {
		return errors.New("http.timeout must be > 0")
	}
	if strings.TrimSpace(cfg.http.userAgent) == "" {
		return errors.New("http.user_agent is required")
	}

	if filepath.Base(cfg.output.asnFile) != cfg.output.asnFile {
		return errors.New("output.asn_file must be a file name, not a path")
	}
	if filepath.Base(cfg.output.countryFile) != cfg.output.countryFile {
		return errors.New("output.country_file must be a file name, not a path")
	}
	if filepath.Base(cfg.output.metadataFile) != cfg.output.metadataFile {
		return errors.New("output.metadata_file must be a file name, not a path")
	}

	return nil
}

func validateDatasetSpec(scope string, spec datasetSpec) error {
	if strings.TrimSpace(spec.path) == "" && strings.TrimSpace(spec.url) == "" {
		return fmt.Errorf("%s requires either path or url", scope)
	}
	if strings.TrimSpace(spec.path) != "" && strings.TrimSpace(spec.url) != "" {
		return fmt.Errorf("%s must define only one of path or url", scope)
	}
	if strings.TrimSpace(spec.format) == "" {
		return fmt.Errorf("%s.format is required", scope)
	}
	switch strings.ToLower(strings.TrimSpace(spec.compression)) {
	case "", compressionAuto, compressionNone, compressionGzip, compressionZip:
	default:
		return fmt.Errorf("%s.compression must be one of auto|none|gzip|zip", scope)
	}
	return nil
}
