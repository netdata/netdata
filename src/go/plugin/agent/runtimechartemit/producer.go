// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

type runtimeProducer interface {
	Name() string
	Tick() error
}

type runtimeProducerFunc struct {
	name string
	tick func() error
}

func (p runtimeProducerFunc) Name() string { return p.name }

func (p runtimeProducerFunc) Tick() error {
	if p.tick == nil {
		return nil
	}
	return p.tick()
}
