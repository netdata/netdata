// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"gopkg.in/yaml.v3"
)

const (
	providerIPToASN     = "iptoasn"
	providerDBIP        = "dbip"
	providerCAIDA       = "caida"
	providerMaxMind     = "maxmind"
	providerIP2Location = "ip2location"
	providerIPDeny      = "ipdeny"
	providerIPIP        = "ipip"
)

const (
	sourceFamilyASN = "asn"
	sourceFamilyGeo = "geo"
)

const (
	artifactIPToASNCombined        = "combined"
	artifactDBIPASNLite            = "asn-lite"
	artifactDBIPCountryLite        = "country-lite"
	artifactDBIPCityLite           = "city-lite"
	artifactCAIDAPrefix2AS         = "prefix2as"
	artifactMaxMindGeoLite2ASN     = "geolite2-asn"
	artifactMaxMindGeoLite2Country = "geolite2-country"
	artifactIP2LocationCountryLite = "country-lite"
	artifactIPDenyCountryZones     = "country-zones"
	artifactIPIPCountry            = "country"
)

const (
	defaultUserConfigDir  = "/etc/netdata"
	defaultStockConfigDir = "/usr/lib/netdata/conf.d"
	defaultCacheDir       = "/var/cache/netdata"
)

const (
	formatMMDB = "mmdb"
	formatCSV  = "csv"
	formatTSV  = "tsv"
	formatCIDR = "cidr"
	formatTXT  = "txt"
)

type config struct {
	sources []sourceEntry
	output  outputConfig
	policy  policyConfig
	http    httpConfig
}

type sourceEntry struct {
	name string

	family   string
	provider string
	artifact string
	format   string

	url  string
	path string
}

type outputConfig struct {
	directory    string
	asnFile      string
	geoFile      string
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
	Sources []yamlSourceEntry `yaml:"sources"`
	Output  *yamlOutputConfig `yaml:"output"`
	Policy  *yamlPolicyConfig `yaml:"policy"`
	HTTP    *yamlHTTPConfig   `yaml:"http"`
}

type yamlSourceEntry struct {
	Name     string `yaml:"name"`
	Family   string `yaml:"family"`
	Provider string `yaml:"provider"`
	Artifact string `yaml:"artifact"`
	Format   string `yaml:"format"`
	URL      string `yaml:"url"`
	Path     string `yaml:"path"`
}

type yamlOutputConfig struct {
	Directory    string `yaml:"directory"`
	AsnFile      string `yaml:"asn_file"`
	GeoFile      string `yaml:"geo_file"`
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

type builtInSourceSpec struct {
	pageURL       string
	directURL     string
	defaultFormat string
	allowedFamily map[string]struct{}
	allowedFormat map[string]struct{}
}

func defaultConfig() config {
	return config{
		sources: []sourceEntry{
			{
				family:   sourceFamilyASN,
				provider: providerDBIP,
				artifact: artifactDBIPASNLite,
				format:   formatMMDB,
			},
			{
				family:   sourceFamilyGeo,
				provider: providerDBIP,
				artifact: artifactDBIPCityLite,
				format:   formatMMDB,
			},
		},
		output: outputConfig{
			directory:    filepath.Join(defaultCacheRoot(), "topology-ip-intel"),
			asnFile:      "topology-ip-asn.mmdb",
			geoFile:      "topology-ip-geo.mmdb",
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

func builtInSource(provider, artifact string) (builtInSourceSpec, bool) {
	switch {
	case provider == providerIPToASN && artifact == artifactIPToASNCombined:
		return builtInSourceSpec{
			directURL:     "https://iptoasn.com/data/ip2asn-combined.tsv.gz",
			defaultFormat: formatTSV,
			allowedFamily: map[string]struct{}{
				sourceFamilyASN: {},
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatTSV: {},
			},
		}, true
	case provider == providerDBIP && artifact == artifactDBIPASNLite:
		return builtInSourceSpec{
			pageURL:       "https://db-ip.com/db/download/ip-to-asn-lite",
			defaultFormat: formatMMDB,
			allowedFamily: map[string]struct{}{
				sourceFamilyASN: {},
			},
			allowedFormat: map[string]struct{}{
				formatMMDB: {},
				formatCSV:  {},
			},
		}, true
	case provider == providerDBIP && artifact == artifactDBIPCountryLite:
		return builtInSourceSpec{
			pageURL:       "https://db-ip.com/db/download/ip-to-country-lite",
			defaultFormat: formatMMDB,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatMMDB: {},
				formatCSV:  {},
			},
		}, true
	case provider == providerDBIP && artifact == artifactDBIPCityLite:
		return builtInSourceSpec{
			pageURL:       "https://db-ip.com/db/download/ip-to-city-lite",
			defaultFormat: formatMMDB,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatMMDB: {},
				formatCSV:  {},
			},
		}, true
	case provider == providerCAIDA && artifact == artifactCAIDAPrefix2AS:
		return builtInSourceSpec{
			pageURL:       "https://data.caida.org/datasets/routing/routeviews-prefix2as/pfx2as-creation.log",
			defaultFormat: formatTSV,
			allowedFamily: map[string]struct{}{
				sourceFamilyASN: {},
			},
			allowedFormat: map[string]struct{}{
				formatTSV: {},
			},
		}, true
	case provider == providerMaxMind && artifact == artifactMaxMindGeoLite2ASN:
		return builtInSourceSpec{
			directURL:     "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-ASN&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz",
			defaultFormat: formatMMDB,
			allowedFamily: map[string]struct{}{
				sourceFamilyASN: {},
			},
			allowedFormat: map[string]struct{}{
				formatMMDB: {},
			},
		}, true
	case provider == providerMaxMind && artifact == artifactMaxMindGeoLite2Country:
		return builtInSourceSpec{
			directURL:     "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country-CSV&license_key=${MAXMIND_LICENSE_KEY}&suffix=zip",
			defaultFormat: formatCSV,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatCSV: {},
			},
		}, true
	case provider == providerIP2Location && artifact == artifactIP2LocationCountryLite:
		return builtInSourceSpec{
			directURL:     "https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.CSV.ZIP",
			defaultFormat: formatCSV,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatCSV: {},
			},
		}, true
	case provider == providerIPDeny && artifact == artifactIPDenyCountryZones:
		return builtInSourceSpec{
			directURL:     "https://www.ipdeny.com/ipblocks/data/countries/all-zones.tar.gz",
			defaultFormat: formatCIDR,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatCIDR: {},
			},
		}, true
	case provider == providerIPIP && artifact == artifactIPIPCountry:
		return builtInSourceSpec{
			directURL:     "https://cdn.ipip.net/17mon/country.zip",
			defaultFormat: formatTXT,
			allowedFamily: map[string]struct{}{
				sourceFamilyGeo: {},
			},
			allowedFormat: map[string]struct{}{
				formatTXT: {},
			},
		}, true
	default:
		return builtInSourceSpec{}, false
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
	if len(fc.Sources) > 0 {
		sources, err := yamlSourceEntries(fc.Sources)
		if err != nil {
			return err
		}
		cfg.sources = sources
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

func yamlSourceEntries(values []yamlSourceEntry) ([]sourceEntry, error) {
	out := make([]sourceEntry, 0, len(values))
	for i, value := range values {
		source := sourceEntry{
			name:     strings.TrimSpace(value.Name),
			family:   strings.ToLower(strings.TrimSpace(value.Family)),
			provider: strings.ToLower(strings.TrimSpace(value.Provider)),
			artifact: strings.ToLower(strings.TrimSpace(value.Artifact)),
			format:   strings.ToLower(strings.TrimSpace(value.Format)),
			url:      strings.TrimSpace(value.URL),
			path:     strings.TrimSpace(value.Path),
		}
		if source.family == "" {
			return nil, fmt.Errorf("sources[%d].family is required", i)
		}
		out = append(out, source)
	}
	return out, nil
}

func mergeOutputConfig(dst outputConfig, src yamlOutputConfig) outputConfig {
	if src.Directory != "" {
		dst.directory = strings.TrimSpace(src.Directory)
	}
	if src.AsnFile != "" {
		dst.asnFile = strings.TrimSpace(src.AsnFile)
	}
	if src.GeoFile != "" {
		dst.geoFile = strings.TrimSpace(src.GeoFile)
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

func defaultSourceName(source sourceEntry) string {
	return fmt.Sprintf("%s-%s", source.provider, source.artifact)
}

func inferFormatFromLocation(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		raw = parsed.Path
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasSuffix(lower, ".mmdb"),
		strings.HasSuffix(lower, ".mmdb.gz"),
		strings.HasSuffix(lower, ".mmdb.zip"):
		return formatMMDB
	case strings.HasSuffix(lower, ".csv"),
		strings.HasSuffix(lower, ".csv.gz"),
		strings.HasSuffix(lower, ".csv.zip"):
		return formatCSV
	case strings.HasSuffix(lower, ".tsv"),
		strings.HasSuffix(lower, ".tsv.gz"),
		strings.HasSuffix(lower, ".tsv.zip"):
		return formatTSV
	default:
		return ""
	}
}

func normalizeSourceEntry(source *sourceEntry) error {
	source.name = strings.TrimSpace(source.name)
	source.family = strings.ToLower(strings.TrimSpace(source.family))
	source.provider = strings.ToLower(strings.TrimSpace(source.provider))
	source.artifact = strings.ToLower(strings.TrimSpace(source.artifact))
	source.format = strings.ToLower(strings.TrimSpace(source.format))
	source.url = strings.TrimSpace(source.url)
	source.path = strings.TrimSpace(source.path)

	if source.name == "" && source.provider != "" && source.artifact != "" {
		source.name = defaultSourceName(*source)
	}

	if source.format == "" {
		if inferred := inferFormatFromLocation(source.url); inferred != "" {
			source.format = inferred
		} else if inferred := inferFormatFromLocation(source.path); inferred != "" {
			source.format = inferred
		} else if spec, ok := builtInSource(source.provider, source.artifact); ok {
			source.format = spec.defaultFormat
		}
	}
	return nil
}

func (cfg *config) normalizeAndValidate() error {
	for i := range cfg.sources {
		if err := normalizeSourceEntry(&cfg.sources[i]); err != nil {
			return err
		}
	}

	cfg.output.directory = strings.TrimSpace(cfg.output.directory)
	cfg.output.asnFile = strings.TrimSpace(cfg.output.asnFile)
	cfg.output.geoFile = strings.TrimSpace(cfg.output.geoFile)
	cfg.output.metadataFile = strings.TrimSpace(cfg.output.metadataFile)
	cfg.http.userAgent = strings.TrimSpace(cfg.http.userAgent)

	return cfg.validate()
}

func validateSourceEntry(source sourceEntry, scope string) error {
	if source.family != sourceFamilyASN && source.family != sourceFamilyGeo {
		return fmt.Errorf("%s.family must be one of asn|geo", scope)
	}
	if source.provider == "" {
		return fmt.Errorf("%s.provider is required", scope)
	}
	if source.artifact == "" {
		return fmt.Errorf("%s.artifact is required", scope)
	}

	spec, ok := builtInSource(source.provider, source.artifact)
	if !ok {
		return fmt.Errorf(
			"%s references unsupported provider/artifact %q/%q",
			scope,
			source.provider,
			source.artifact,
		)
	}

	if _, ok := spec.allowedFamily[source.family]; !ok {
		return fmt.Errorf(
			"%s provider/artifact %q/%q is not compatible with family %q",
			scope,
			source.provider,
			source.artifact,
			source.family,
		)
	}

	if source.url != "" && source.path != "" {
		return fmt.Errorf("%s must define at most one of url or path", scope)
	}

	if source.format == "" {
		return fmt.Errorf("%s.format could not be determined", scope)
	}
	if _, ok := spec.allowedFormat[source.format]; !ok {
		return fmt.Errorf(
			"%s.format %q is not supported for provider/artifact %q/%q",
			scope,
			source.format,
			source.provider,
			source.artifact,
		)
	}
	return nil
}

func (cfg config) validate() error {
	for i, source := range cfg.sources {
		if err := validateSourceEntry(source, fmt.Sprintf("sources[%d]", i)); err != nil {
			return err
		}
	}

	if cfg.output.directory == "" {
		return errors.New("output.directory is required")
	}
	if cfg.output.asnFile == "" {
		return errors.New("output.asn_file is required")
	}
	if cfg.output.geoFile == "" {
		return errors.New("output.geo_file is required")
	}
	if cfg.output.metadataFile == "" {
		return errors.New("output.metadata_file is required")
	}
	if cfg.http.timeout <= 0 {
		return errors.New("http.timeout must be > 0")
	}
	if cfg.http.userAgent == "" {
		return errors.New("http.user_agent is required")
	}
	if filepath.Base(cfg.output.asnFile) != cfg.output.asnFile {
		return errors.New("output.asn_file must be a file name, not a path")
	}
	if filepath.Base(cfg.output.geoFile) != cfg.output.geoFile {
		return errors.New("output.geo_file must be a file name, not a path")
	}
	if filepath.Base(cfg.output.metadataFile) != cfg.output.metadataFile {
		return errors.New("output.metadata_file must be a file name, not a path")
	}

	return nil
}

func (cfg config) familySources(family string) []sourceEntry {
	out := make([]sourceEntry, 0, len(cfg.sources))
	for _, source := range cfg.sources {
		if source.family == family {
			out = append(out, source)
		}
	}
	return out
}

func (cfg config) hasFamily(family string) bool {
	for _, source := range cfg.sources {
		if source.family == family {
			return true
		}
	}
	return false
}
