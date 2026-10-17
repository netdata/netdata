package main

const (
	processDefaultUpdateEvery  = 10
	processDefaultBTFPath      = "/sys/kernel/btf"
	processDefaultObjectFlavor = "buffer"
	processLegacyConfigFile    = "ebpf.d/process.conf"
)

func loadProcessConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(processLegacyConfigFile)
}
