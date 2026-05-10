package libbpfloader

type CachestatSnapshot struct {
	MarkPageAccessed   uint64
	MarkBufferDirty    uint64
	AddToPageCacheLru  uint64
	AccountPageDirtied uint64
}
