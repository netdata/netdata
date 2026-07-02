package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const pluginPrimaryConfigFile = "ebpf.d.conf"

// pluginConfigFile holds the raw merged values parsed from any ebpfgo plugin
// config file ([global] and [ebpf programs] sections).  All fields are
// pointers so that an absent key is distinguishable from an explicit value;
// apply() merges a later file on top of an earlier one.
type pluginConfigFile struct {
	Cachestat       *bool // [ebpf programs] cachestat key
	Socket          *bool // [ebpf programs] socket key
	DNS             *bool // [ebpf programs] dns key
	UpdateEvery     *int
	AppsEnabled     *bool
	Cgroups         *bool
	PidTable        *uint32
	MapsPerCore     *bool
	BTFPath         *string
	Lifetime        *int
	ObjectFlavor    *string
	CollectPidLevel *int // "collect pid" key → BPF apps collection level (0=real parent, 1=parent, 2=all)
}

// loadPluginConfigFiles loads the plugin-wide ebpf.d.conf from stock then
// user config directories and returns the merged result.  Programs that need
// per-program overrides (e.g. cachestat.conf) should call this first, then
// merge their own file on top.
func loadPluginConfigFiles() (pluginConfigFile, bool, error) {
	userRoot, stockRoot := pluginConfigRoots()

	var merged pluginConfigFile
	found := false
	for _, path := range []string{
		filepath.Join(stockRoot, pluginPrimaryConfigFile),
		filepath.Join(userRoot, pluginPrimaryConfigFile),
	} {
		cfg, ok, err := parsePluginConfigFile(path)
		if err != nil {
			return pluginConfigFile{}, false, err
		}
		if !ok {
			continue
		}
		found = true
		merged.apply(cfg)
	}

	return merged, found, nil
}

// loadCollectorConfigFiles merges the plugin-wide ebpf.d.conf with a
// collector-specific override file, following the same 4-path load order used
// by the old C plugin: stock global → stock override → user global → user override.
// It is the shared implementation behind loadCachestatConfigFiles and
// loadSocketConfigFiles; adding a new collector requires only a one-liner wrapper.
func loadCollectorConfigFiles(legacyFile string) (pluginConfigFile, bool, error) {
	userRoot, stockRoot := pluginConfigRoots()

	var merged pluginConfigFile
	found := false
	for _, path := range []string{
		filepath.Join(stockRoot, pluginPrimaryConfigFile),
		filepath.Join(stockRoot, legacyFile),
		filepath.Join(userRoot, pluginPrimaryConfigFile),
		filepath.Join(userRoot, legacyFile),
	} {
		cfg, ok, err := parsePluginConfigFile(path)
		if err != nil {
			return pluginConfigFile{}, false, err
		}
		if !ok {
			continue
		}
		found = true
		merged.apply(cfg)
	}

	return merged, found, nil
}

func pluginConfigRoots() (userRoot, stockRoot string) {
	userRoot = os.Getenv("NETDATA_USER_CONFIG_DIR")
	if userRoot == "" {
		userRoot = "/etc/netdata"
	}

	stockRoot = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
	if stockRoot == "" {
		stockRoot = "/usr/lib/netdata/conf.d"
	}

	return userRoot, stockRoot
}

func parsePluginConfigFile(path string) (pluginConfigFile, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pluginConfigFile{}, false, nil
		}
		return pluginConfigFile{}, false, err
	}
	defer file.Close()

	var cfg pluginConfigFile
	scanner := bufio.NewScanner(file)
	inGlobal := false
	inEbpfPrograms := false
	found := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			inGlobal = section == "global"
			inEbpfPrograms = section == "ebpf programs"
			continue
		}

		if inEbpfPrograms {
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			switch key {
			case "cachestat":
				b, ok := parseConfigBool(value)
				if !ok {
					return pluginConfigFile{}, false, fmt.Errorf("%s: invalid cachestat %q", path, value)
				}
				cfg.Cachestat = boolPtr(b)
				found = true
			case "socket":
				b, ok := parseConfigBool(value)
				if !ok {
					return pluginConfigFile{}, false, fmt.Errorf("%s: invalid socket %q", path, value)
				}
				cfg.Socket = boolPtr(b)
				found = true
			case "dns":
				b, ok := parseConfigBool(value)
				if !ok {
					return pluginConfigFile{}, false, fmt.Errorf("%s: invalid dns %q", path, value)
				}
				cfg.DNS = boolPtr(b)
				found = true
			}
			continue
		}

		if !inGlobal {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "update every":
			n, err := strconv.Atoi(value)
			if err != nil {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid update every %q: %w", path, value, err)
			}
			cfg.UpdateEvery = intPtr(n)
			found = true
		case "apps":
			b, ok := parseConfigBool(value)
			if !ok {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid apps %q", path, value)
			}
			cfg.AppsEnabled = boolPtr(b)
			found = true
		case "cgroups":
			b, ok := parseConfigBool(value)
			if !ok {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid cgroups %q", path, value)
			}
			cfg.Cgroups = boolPtr(b)
			found = true
		case "pid table size":
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid pid table size %q: %w", path, value, err)
			}
			cfg.PidTable = uint32Ptr(uint32(n))
			found = true
		case "maps per core":
			b, ok := parseConfigBool(value)
			if !ok {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid maps per core %q", path, value)
			}
			cfg.MapsPerCore = boolPtr(b)
			found = true
		case "btf path":
			cfg.BTFPath = stringPtr(value)
			found = true
		case "lifetime":
			n, err := strconv.Atoi(value)
			if err != nil {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid lifetime %q: %w", path, value, err)
			}
			cfg.Lifetime = intPtr(n)
			found = true
		case "ebpf object flavor":
			flavor, ok := parseObjectFlavor(value)
			if !ok {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid ebpf object flavor %q", path, value)
			}
			cfg.ObjectFlavor = stringPtr(flavor)
			found = true
		case "ebpf type format":
			// Legacy key from the old ebpf.plugin; maps to ebpf object flavor.
			// "legacy" forces the kprobe-based tracing path; "co-re" and "auto"
			// leave flavor selection to auto-detection.
			if strings.EqualFold(value, "legacy") {
				cfg.ObjectFlavor = stringPtr("tracing")
			}
			found = true
		case "ebpf co-re tracing":
			// Legacy key from the old ebpf.plugin.  "probe" means kprobe
			// attachment, which is equivalent to the tracing (legacy) object
			// flavor.  "trampoline" is the default and needs no override.
			if strings.EqualFold(value, "probe") {
				cfg.ObjectFlavor = stringPtr("tracing")
			}
			found = true
		case "collect pid":
			// Legacy key from the old ebpf.plugin; controls the BPF apps
			// collection level written to the cstat_ctrl map.
			level := parseCollectPidLevel(value)
			if level < 0 {
				return pluginConfigFile{}, false, fmt.Errorf("%s: invalid collect pid %q", path, value)
			}
			cfg.CollectPidLevel = intPtr(level)
			found = true
		}
	}

	if err := scanner.Err(); err != nil {
		return pluginConfigFile{}, false, err
	}

	return cfg, found, nil
}

func (c *pluginConfigFile) apply(other pluginConfigFile) {
	if other.Cachestat != nil {
		c.Cachestat = other.Cachestat
	}
	if other.Socket != nil {
		c.Socket = other.Socket
	}
	if other.DNS != nil {
		c.DNS = other.DNS
	}
	if other.UpdateEvery != nil {
		c.UpdateEvery = other.UpdateEvery
	}
	if other.AppsEnabled != nil {
		c.AppsEnabled = other.AppsEnabled
	}
	if other.Cgroups != nil {
		c.Cgroups = other.Cgroups
	}
	if other.PidTable != nil {
		c.PidTable = other.PidTable
	}
	if other.MapsPerCore != nil {
		c.MapsPerCore = other.MapsPerCore
	}
	if other.BTFPath != nil {
		c.BTFPath = other.BTFPath
	}
	if other.Lifetime != nil {
		c.Lifetime = other.Lifetime
	}
	if other.ObjectFlavor != nil {
		c.ObjectFlavor = other.ObjectFlavor
	}
	if other.CollectPidLevel != nil {
		c.CollectPidLevel = other.CollectPidLevel
	}
}

func intPtr(v int) *int {
	return &v
}

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func stringPtr(v string) *string {
	return &v
}

func parseConfigBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "on", "1":
		return true, true
	case "no", "false", "off", "0":
		return false, true
	default:
		return false, false
	}
}

// parseCollectPidLevel maps the "collect pid" legacy key to a numeric BPF
// apps-level value (0=real parent, 1=parent, 2=all).  Returns -1 on unknown.
func parseCollectPidLevel(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "real parent":
		return 0
	case "parent":
		return 1
	case "all":
		return 2
	default:
		return -1
	}
}

func parseObjectFlavor(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", " ", "-", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")

	switch normalized {
	case "buffer", "buffer ring", "ring buffer":
		return "buffer", true
	case "arena":
		return "arena", true
	case "tracing", "legacy":
		return "tracing", true
	default:
		return "", false
	}
}
