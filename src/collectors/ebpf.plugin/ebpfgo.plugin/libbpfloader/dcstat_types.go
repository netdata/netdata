package libbpfloader

// DCStatSnapshot holds the three global directory-cache counters read from the
// dcstat_global BPF map.  Reference counts lookup_fast calls, Slow counts
// d_lookup calls (the slow path), Miss counts d_lookup calls that found nothing.
type DCStatSnapshot struct {
	Reference uint64
	Slow      uint64
	Miss      uint64
}

const DCStatAppCommLen = 96

type DCStatAppSnapshot struct {
	Pid         uint32
	Ppid        uint32
	Comm        [DCStatAppCommLen]byte
	Ct          uint64
	CacheAccess uint64
	FileSystem  uint64
	NotFound    uint64
}
