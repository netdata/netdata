package main

// Keep the Go shared-memory schema aligned with the C collector layout so the
// process snapshot can grow without another contract rewrite.
const EBPF_MAX_COMPARE_NAME = 95
const TASK_COMM_LEN = 16

type netdataCachestat struct {
	AddToPageCacheLru  uint32
	MarkPageAccessed   uint32
	AccountPageDirtied uint32
	MarkBufferDirty    uint32
}

type netdataPublishCachestat struct {
	Ct      uint64
	Ratio   int64
	Dirty   int64
	Hit     int64
	Miss    int64
	Current netdataCachestat
	Prev    netdataCachestat
}

type netdataPublishDCStatPid struct {
	CacheAccess uint64
	FileSystem  uint64
	NotFound    uint64
}

type netdataPublishDCStat struct {
	Ct          uint64
	Ratio       int64
	CacheAccess int64
	Curr        netdataPublishDCStatPid
	Prev        netdataPublishDCStatPid
}

type netdataPublishFDStat struct {
	Ct        uint64
	OpenCall  uint32
	CloseCall uint32
	OpenErr   uint32
	CloseErr  uint32
}

type ebpfProcessStat struct {
	Ct            uint64
	Uid           uint32
	Gid           uint32
	Name          [TASK_COMM_LEN]byte
	Tgid          uint32
	Pid           uint32
	ExitCall      uint32
	ReleaseCall   uint32
	CreateProcess uint32
	CreateThread  uint32
	TaskErr       uint32
}

type netdataPublishSHM struct {
	Ct  uint64
	Get uint32
	At  uint32
	Dt  uint32
	Ctl uint32
}

type netdataPublishSwap struct {
	Ct    uint64
	Read  uint32
	Write uint32
}

type ebpfSocketPublishApps struct {
	BytesSent           uint64
	BytesReceived       uint64
	CallTCPSent         uint64
	CallTCPReceived     uint64
	Retransmit          uint64
	CallUDPSent         uint64
	CallUDPReceived     uint64
	CallClose           uint64
	CallTCPV4Connection uint64
	CallTCPV6Connection uint64
}

type netdataPublishVFS struct {
	Ct          uint64
	WriteCall   uint32
	WritevCall  uint32
	ReadCall    uint32
	ReadvCall   uint32
	UnlinkCall  uint32
	FsyncCall   uint32
	OpenCall    uint32
	CreateCall  uint32
	WriteBytes  uint64
	WritevBytes uint64
	ReadvBytes  uint64
	ReadBytes   uint64
	WriteErr    uint32
	WritevErr   uint32
	ReadErr     uint32
	ReadvErr    uint32
	UnlinkErr   uint32
	FsyncErr    uint32
	OpenErr     uint32
	CreateErr   uint32
}

type ebpfPidStat struct {
	pid       uint32
	comm      [EBPF_MAX_COMPARE_NAME + 1]byte
	ppid      uint32
	cachestat netdataPublishCachestat
	dc        netdataPublishDCStat
	fd        netdataPublishFDStat
	process   ebpfProcessStat
	shm       netdataPublishSHM
	swap      netdataPublishSwap
	socket    ebpfSocketPublishApps
	vfs       netdataPublishVFS
}
