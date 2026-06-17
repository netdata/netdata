//go:build !linux || !cgo

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

type SharedDnsMemoryPublisher struct{}

func NewSharedDnsMemoryPublisher() (*SharedDnsMemoryPublisher, error) {
	return nil, ErrDisabled
}

func (p *SharedDnsMemoryPublisher) Publish(_ libbpfloader.DNSSnapshot, _ []libbpfloader.DNSFlowRecord) {
}
func (p *SharedDnsMemoryPublisher) Close() {}
