package transportconfig

type TypedConfig struct {
	SupportedProfiles             uint32
	PreferredProfiles             uint32
	MaxRequestPayloadBytes        uint32
	MaxRequestBatchItems          uint32
	MaxResponsePayloadBytes       uint32
	CallTimeoutMs                 uint32
	AuthToken                     uint64
	MaxLogicalLookupItems         uint32
	MaxLogicalLookupSubcalls      uint32
	MaxLogicalLookupResponseBytes uint32
}

func responseBatchItems(config TypedConfig) uint32 {
	return config.MaxRequestBatchItems
}
