//go:build !linux || !cgo

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

type SharedDnsMemoryPublisher struct{}

func NewSharedDnsMemoryPublisher(_ uint32) (*SharedDnsMemoryPublisher, error) {
	return nil, ErrDisabled
}

func (p *SharedDnsMemoryPublisher) Publish(_ libbpfloader.DNSSnapshot, _ []libbpfloader.DNSFlowRecord) {
	// No shared-memory segment is available on non-Linux or non-CGo builds.
}
func (p *SharedDnsMemoryPublisher) Close() {
	// No shared-memory segment to release on non-Linux or non-CGo builds.
}
