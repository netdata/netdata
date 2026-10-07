// SPDX-License-Identifier: GPL-3.0-or-later

package cachestat

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type snapshot struct {
	Accessed, BufferDirty, Added, AccountDirtied uint64
}

type nativeRuntime interface {
	Snapshot() (snapshot, error)
	Close()
}

type instruments struct {
	accessed, bufferDirty, added, accountDirtied metrix.SnapshotCounter
}

func newInstruments(store metrix.CollectorStore) instruments {
	m := store.Write().SnapshotMeter("cachestat")
	return instruments{
		accessed:       m.Counter("mark_page_accessed_total"),
		bufferDirty:    m.Counter("mark_buffer_dirty_total"),
		added:          m.Counter("add_to_page_cache_lru_total"),
		accountDirtied: m.Counter("account_page_dirtied_total"),
	}
}

func (m instruments) observe(s snapshot) {
	m.accessed.ObserveTotal(float64(s.Accessed))
	m.bufferDirty.ObserveTotal(float64(s.BufferDirty))
	m.added.ObserveTotal(float64(s.Added))
	m.accountDirtied.ObserveTotal(float64(s.AccountDirtied))
}
