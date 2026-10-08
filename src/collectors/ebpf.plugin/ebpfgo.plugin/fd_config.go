package main

const (
	fdDefaultUpdateEvery  = 10
	fdDefaultBTFPath      = "/sys/kernel/btf"
	fdDefaultBTFFile      = "vmlinux"
	fdDefaultObjectFlavor = "buffer"
	fdLegacyConfigFile    = "ebpf.d/fd.conf"
)

func loadFDConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(fdLegacyConfigFile)
}
