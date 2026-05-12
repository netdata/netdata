package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	cachestatDefaultUpdateEvery = 10
	cachestatDefaultBTFPath     = "/sys/kernel/btf"
	cachestatLegacyConfigFile   = "ebpf.d/cachestat.conf"
	cachestatPrimaryConfigFile  = "ebpf.d.conf"
	cachestatLegacyConfigName   = "cachestat.conf"
)

type cachestatConfigFile struct {
	UpdateEvery *int
	PidTable    *uint32
	MapsPerCore *bool
	BTFPath     *string
	Lifetime    *int
}

func loadCachestatConfigFiles() (cachestatConfigFile, error) {
	userRoot, stockRoot := cachestatConfigRoots()

	var merged cachestatConfigFile
	for _, path := range []string{
		filepath.Join(stockRoot, cachestatPrimaryConfigFile),
		filepath.Join(stockRoot, cachestatLegacyConfigFile),
		filepath.Join(userRoot, cachestatPrimaryConfigFile),
		filepath.Join(userRoot, cachestatLegacyConfigFile),
	} {
		cfg, ok, err := parseCachestatConfigFile(path)
		if err != nil {
			return cachestatConfigFile{}, err
		}
		if !ok {
			continue
		}
		merged.apply(cfg)
	}

	return merged, nil
}

func cachestatConfigRoots() (userRoot, stockRoot string) {
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

func parseCachestatConfigFile(path string) (cachestatConfigFile, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cachestatConfigFile{}, false, nil
		}
		return cachestatConfigFile{}, false, err
	}
	defer file.Close()

	var cfg cachestatConfigFile
	scanner := bufio.NewScanner(file)
	inGlobal := false
	found := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inGlobal = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "global")
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
				return cachestatConfigFile{}, false, fmt.Errorf("%s: invalid update every %q: %w", path, value, err)
			}
			cfg.UpdateEvery = intPtr(n)
			found = true
		case "pid table size":
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return cachestatConfigFile{}, false, fmt.Errorf("%s: invalid pid table size %q: %w", path, value, err)
			}
			cfg.PidTable = uint32Ptr(uint32(n))
			found = true
		case "maps per core":
			b, ok := parseConfigBool(value)
			if !ok {
				return cachestatConfigFile{}, false, fmt.Errorf("%s: invalid maps per core %q", path, value)
			}
			cfg.MapsPerCore = boolPtr(b)
			found = true
		case "btf path":
			cfg.BTFPath = stringPtr(value)
			found = true
		case "lifetime":
			n, err := strconv.Atoi(value)
			if err != nil {
				return cachestatConfigFile{}, false, fmt.Errorf("%s: invalid lifetime %q: %w", path, value, err)
			}
			cfg.Lifetime = intPtr(n)
			found = true
		}
	}

	if err := scanner.Err(); err != nil {
		return cachestatConfigFile{}, false, err
	}

	return cfg, found, nil
}

func (c *cachestatConfigFile) apply(other cachestatConfigFile) {
	if other.UpdateEvery != nil {
		c.UpdateEvery = other.UpdateEvery
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
