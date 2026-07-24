//go:build !linux || !cgo

package main

type SharedPidMemoryPublisher struct{}

func NewSharedPidMemoryPublisher(total uint32, _ uint32) (*SharedPidMemoryPublisher, error) {
	_ = total
	return nil, ErrDisabled
}

func (p *SharedPidMemoryPublisher) Publish(entries []ebpfPidStat, _ uint32) error {
	_ = entries
	return ErrDisabled
}

func (p *SharedPidMemoryPublisher) Close() {
	// No shared memory or semaphore resources are opened on non-Unix builds.
	// The publisher is intentionally a no-op here.
}
