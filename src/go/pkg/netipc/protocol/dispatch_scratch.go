package protocol

import "sync"

// The request view and builder a lookup dispatch hands to its handler
// escape through that call. Pooled scratch keeps them, and the cgroups
// payload-exceeded table, off the per-request heap; each dispatch owns its
// entry until it returns.

type cgroupsLookupScratch struct {
	view    CgroupsLookupRequestView
	builder CgroupsLookupBuilder
	suffix  []uint32
}

var cgroupsLookupScratchPool = sync.Pool{New: func() any { return new(cgroupsLookupScratch) }}

// release drops the references to the caller's buffers, keeps the table.
func (s *cgroupsLookupScratch) release() {
	s.view = CgroupsLookupRequestView{}
	s.builder = CgroupsLookupBuilder{}
	cgroupsLookupScratchPool.Put(s)
}

type appsLookupScratch struct {
	view    AppsLookupRequestView
	builder AppsLookupBuilder
}

var appsLookupScratchPool = sync.Pool{New: func() any { return new(appsLookupScratch) }}

func (s *appsLookupScratch) release() {
	s.view = AppsLookupRequestView{}
	s.builder = AppsLookupBuilder{}
	appsLookupScratchPool.Put(s)
}
