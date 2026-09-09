package main

const socketLegacyConfigFile = "ebpf.d/socket.conf"

func loadSocketConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(socketLegacyConfigFile)
}
