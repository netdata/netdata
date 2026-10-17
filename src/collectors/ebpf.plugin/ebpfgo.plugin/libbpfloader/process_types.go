package libbpfloader

type ProcessSnapshot struct {
	Exits     uint64
	TaskClose uint64
	Forks     uint64
	Clones    uint64
	Errors    uint64
}

const ProcessAppCommLen = 96

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
