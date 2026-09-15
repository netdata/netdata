package libbpfloader

type CachestatSnapshot struct {
	MarkPageAccessed   uint64
	MarkBufferDirty    uint64
	AddToPageCacheLru  uint64
	AccountPageDirtied uint64
}

const CachestatAppCommLen = 96

type CachestatAppSnapshot struct {
	Pid                uint32
	Ppid               uint32
	Comm               [CachestatAppCommLen]byte
	Ct                 uint64
	AddToPageCacheLru  uint32
	MarkPageAccessed   uint32
	AccountPageDirtied uint32
	MarkBufferDirty    uint32
}
