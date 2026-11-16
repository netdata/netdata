package libbpfloader

// ProcessSnapshot holds the cumulative global counters of tbl_total_stats
// (enum process_counters in netdata/kernel-collector includes/netdata_process.h).
type ProcessSnapshot struct {
	Exits      uint64
	TaskClose  uint64
	Process    uint64
	ProcessErr uint64
	Thread     uint64
	ThreadErr  uint64
}

const ProcessAppCommLen = 96

// ProcessAppSnapshot holds one TGID's cumulative per-PID counters.
type ProcessAppSnapshot struct {
	Pid       uint32
	Ppid      uint32
	Comm      [ProcessAppCommLen]byte
	Ct        uint64
	Exits     uint32
	TaskClose uint32
	Forks     uint32
	Clones    uint32
	Errors    uint32
}
