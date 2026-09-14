// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

// InitializeActorHandles continues the imported graph generation and validates
// that every actor and link belongs to it.
func (d *Data) InitializeActorHandles() error {
	if d == nil {
		return fmt.Errorf("cannot initialize actor handles on nil topology data")
	}

	var allocator graph.ActorHandleAllocator
	seen := make(map[ActorHandle]struct{}, len(d.Actors))
	for i := range d.Actors {
		handle := d.Actors[i].ActorHandle
		if handle.IsZero() {
			return fmt.Errorf("actor %d has a zero handle", i)
		}
		if _, ok := seen[handle]; ok {
			return fmt.Errorf("actor %d has a duplicate handle", i)
		}
		seen[handle] = struct{}{}
		if err := allocator.Include(handle); err != nil {
			return fmt.Errorf("actor %d: %w", i, err)
		}
	}
	if len(d.Actors) == 0 {
		allocator = graph.NewActorHandleAllocator()
	}
	for i, link := range d.Links {
		if _, ok := seen[link.SrcActorHandle]; !ok {
			return fmt.Errorf("link %d references an unknown source actor handle", i)
		}
		if _, ok := seen[link.DstActorHandle]; !ok {
			return fmt.Errorf("link %d references an unknown destination actor handle", i)
		}
	}
	d.actorHandles = allocator
	return nil
}

func (d *Data) NextActorHandle() ActorHandle {
	if d == nil {
		return ActorHandle{}
	}
	return d.actorHandles.Next()
}

func (d *Data) ValidateActorHandles() error {
	if d == nil {
		return fmt.Errorf("cannot validate actor handles on nil topology data")
	}
	seen := make(map[ActorHandle]struct{}, len(d.Actors))
	var allocator graph.ActorHandleAllocator
	for i, actor := range d.Actors {
		if actor.ActorHandle.IsZero() {
			return fmt.Errorf("actor %d has a zero handle", i)
		}
		if _, ok := seen[actor.ActorHandle]; ok {
			return fmt.Errorf("actor %d has a duplicate handle", i)
		}
		if err := allocator.Include(actor.ActorHandle); err != nil {
			return fmt.Errorf("actor %d: %w", i, err)
		}
		seen[actor.ActorHandle] = struct{}{}
	}
	for i, link := range d.Links {
		if _, ok := seen[link.SrcActorHandle]; !ok {
			return fmt.Errorf("link %d references an unknown source actor handle", i)
		}
		if _, ok := seen[link.DstActorHandle]; !ok {
			return fmt.Errorf("link %d references an unknown destination actor handle", i)
		}
	}
	return nil
}
