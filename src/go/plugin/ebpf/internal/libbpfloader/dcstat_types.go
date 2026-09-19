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
	Pid  uint32
	Ppid uint32
	Comm [DCStatAppCommLen]byte
	// Ct is the raw BPF creation/event timestamp.  It is NOT a usable freshness
	// signal and the shared-memory store deliberately ignores it: only the buffer
	// and arena objects stamp it per event, the CO-RE base object never writes it
	// (it stays 0), and the legacy object writes it once at map-entry creation.
	// It is kept because it mirrors the C snapshot layout, where the per-CPU merge
	// needs it.  Consumers must use the store's synthetic token instead — see
	// UpdateDCStatApps.
	Ct          uint64
	CacheAccess uint64
	FileSystem  uint64
	NotFound    uint64
}
