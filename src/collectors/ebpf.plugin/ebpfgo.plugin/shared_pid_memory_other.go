//go:build !unix || !cgo

package main

type SharedPidMemoryPublisher struct{}

func NewSharedPidMemoryPublisher(total uint32) (*SharedPidMemoryPublisher, error) {
	_ = total
	return nil, ErrDisabled
}

func (p *SharedPidMemoryPublisher) Publish(entries []ebpfPidStat) error {
	_ = entries
	return ErrDisabled
}

func (p *SharedPidMemoryPublisher) Close() {
	// No shared memory or semaphore resources are opened on non-Unix builds.
	// The publisher is intentionally a no-op here.
}
