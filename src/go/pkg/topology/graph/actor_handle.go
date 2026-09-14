// SPDX-License-Identifier: GPL-3.0-or-later

package graph

import (
	"fmt"
	"math"
)

// ActorHandle is an opaque, generation-local actor identity used only while a
// topology graph is being built. It is deliberately unrelated to public actor
// IDs and must never be persisted or serialized.
type ActorHandle struct {
	generation *actorHandleGeneration
	ordinal    uint64
}

type actorHandleGeneration struct {
	marker byte
}

func (h ActorHandle) IsZero() bool {
	return h.generation == nil || h.ordinal == 0
}

// ActorHandleAllocator allocates handles within one graph generation.
type ActorHandleAllocator struct {
	generation *actorHandleGeneration
	next       uint64
}

func NewActorHandleAllocator() ActorHandleAllocator {
	return ActorHandleAllocator{generation: &actorHandleGeneration{}}
}

func (a *ActorHandleAllocator) Next() ActorHandle {
	if a.generation == nil {
		*a = NewActorHandleAllocator()
	}
	if a.next == math.MaxUint64 {
		panic("actor handle ordinal exhausted")
	}
	a.next++
	return ActorHandle{generation: a.generation, ordinal: a.next}
}

// Include continues an existing generation at or above handle's ordinal.
func (a *ActorHandleAllocator) Include(handle ActorHandle) error {
	if handle.IsZero() {
		return fmt.Errorf("cannot include a zero actor handle")
	}
	if a.generation == nil {
		a.generation = handle.generation
	} else if a.generation != handle.generation {
		return fmt.Errorf("actor handles belong to different generations")
	}
	if handle.ordinal > a.next {
		a.next = handle.ordinal
	}
	return nil
}
