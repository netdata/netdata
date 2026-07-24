package main

const dnsLegacyConfigFile = "ebpf.d/dns.conf"

func loadDNSConfigFiles() (pluginConfigFile, bool, error) {
	return loadCollectorConfigFiles(dnsLegacyConfigFile)
}
