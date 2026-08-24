package main

const (
	fdDefaultUpdateEvery  = 10
	fdDefaultBTFPath      = "/sys/kernel/btf"
	fdDefaultObjectFlavor = "buffer"
	fdLegacyConfigFile    = "ebpf.d/fd.conf"
)

func loadFDConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(fdLegacyConfigFile)
}
