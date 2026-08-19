package main

const (
	cachestatDefaultUpdateEvery  = 10
	cachestatDefaultBTFPath      = "/sys/kernel/btf"
	cachestatDefaultObjectFlavor = "buffer"
	cachestatLegacyConfigFile    = "ebpf.d/cachestat.conf"
)

func loadCachestatConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(cachestatLegacyConfigFile)
}
