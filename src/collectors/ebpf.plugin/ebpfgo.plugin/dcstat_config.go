package main

const (
	dcstatDefaultUpdateEvery  = 10
	dcstatDefaultBTFPath      = "/sys/kernel/btf"
	dcstatDefaultObjectFlavor = "buffer"
	dcstatLegacyConfigFile    = "ebpf.d/dcstat.conf"
)

func loadDCStatConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(dcstatLegacyConfigFile)
}
