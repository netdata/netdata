package libbpfloader

// FDSnapshot holds the four global file-descriptor counters read from the
// tbl_fd_global BPF map.  OpenCall/CloseCall count every traced call;
// OpenErr/CloseErr count the subset that returned an error.
type FDSnapshot struct {
	OpenCall  uint64
	OpenErr   uint64
	CloseCall uint64
	CloseErr  uint64
}

const FDAppCommLen = 96

type FDAppSnapshot struct {
	Pid  uint32
	Ppid uint32
	Comm [FDAppCommLen]byte
	// Ct is the raw BPF creation/event timestamp.  It is NOT a usable freshness
	// signal and the shared-memory store deliberately ignores it: only the buffer
	// and arena objects stamp it per event, the CO-RE base object writes it once
	// when the map entry is created and never again.  It is kept because it
	// mirrors the C snapshot layout, where the per-CPU merge needs it.  Consumers
	// must use the store's synthetic token instead — see UpdateFDApps.
	Ct        uint64
	OpenCall  uint32
	CloseCall uint32
	OpenErr   uint32
	CloseErr  uint32
}
