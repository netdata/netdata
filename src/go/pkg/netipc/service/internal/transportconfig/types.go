package transportconfig

type TypedConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	CallTimeoutMs           uint32
	AuthToken               uint64
}

func responseBatchItems(config TypedConfig) uint32 {
	return config.MaxRequestBatchItems
}
