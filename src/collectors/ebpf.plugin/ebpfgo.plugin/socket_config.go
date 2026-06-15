package main

const socketLegacyConfigFile = "ebpf.d/network.conf"

func loadSocketConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(socketLegacyConfigFile)
}
