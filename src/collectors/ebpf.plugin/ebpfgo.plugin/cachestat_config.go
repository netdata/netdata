package main

import "path/filepath"

const (
	cachestatDefaultUpdateEvery  = 10
	cachestatDefaultBTFPath      = "/sys/kernel/btf"
	cachestatDefaultObjectFlavor = "buffer"
	cachestatLegacyConfigFile    = "ebpf.d/cachestat.conf"
	cachestatLegacyConfigName    = "cachestat.conf"
)

// loadCachestatConfigFiles merges the plugin-wide ebpf.d.conf with the
// cachestat-specific cachestat.conf override, preserving the same load order
// as the old C plugin: stock global → stock cachestat → user global → user cachestat.
func loadCachestatConfigFiles() (pluginConfigFile, bool, error) {
	userRoot, stockRoot := pluginConfigRoots()

	var merged pluginConfigFile
	found := false
	for _, path := range []string{
		filepath.Join(stockRoot, pluginPrimaryConfigFile),
		filepath.Join(stockRoot, cachestatLegacyConfigFile),
		filepath.Join(userRoot, pluginPrimaryConfigFile),
		filepath.Join(userRoot, cachestatLegacyConfigFile),
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
