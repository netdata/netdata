package main

const dnslLegacyConfigFile = "ebpf.d/dns.conf"

func loadDNSConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(dnslLegacyConfigFile)
}
