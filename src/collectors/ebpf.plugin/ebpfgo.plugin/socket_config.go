package main

import "path/filepath"

const (
	socketLegacyConfigFile = "ebpf.d/network.conf"
	socketLegacyConfigName = "network.conf"
)

// loadSocketConfigFiles merges the plugin-wide ebpf.d.conf with the
// socket-specific network.conf override, preserving the same load order
// as the old C plugin: stock global → stock network → user global → user network.
func loadSocketConfigFiles() (pluginConfigFile, bool, error) {
	userRoot, stockRoot := pluginConfigRoots()

	var merged pluginConfigFile
	found := false
	for _, path := range []string{
		filepath.Join(stockRoot, pluginPrimaryConfigFile),
		filepath.Join(stockRoot, socketLegacyConfigFile),
		filepath.Join(userRoot, pluginPrimaryConfigFile),
		filepath.Join(userRoot, socketLegacyConfigFile),
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
