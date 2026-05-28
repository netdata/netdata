// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"maps"
	"time"
)

type publishedRecord struct {
	published PublishedStore
}

type Snapshot struct {
	generation  uint64
	publishedAt time.Time
	stores      map[string]publishedRecord
}

func (s *Snapshot) Generation() uint64 {
	if s == nil {
		return 0
	}
	return s.generation
}

func (s *Snapshot) PublishedAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.publishedAt
}

func (s *Snapshot) lookupStore(key string) (publishedRecord, bool) {
	if s == nil {
		return publishedRecord{}, false
	}
	store, ok := s.stores[key]
	if !ok {
		return publishedRecord{}, false
	}
	return store, true
}

func cloneSnapshot(s *Snapshot) *Snapshot {
	if s == nil {
		return &Snapshot{stores: map[string]publishedRecord{}}
	}
	return &Snapshot{
		generation:  s.generation,
		publishedAt: s.publishedAt,
		stores:      clonePublishedRecords(s.stores),
	}
}

func clonePublishedRecords(in map[string]publishedRecord) map[string]publishedRecord {
	if len(in) == 0 {
		return map[string]publishedRecord{}
	}
	out := make(map[string]publishedRecord, len(in))
	maps.Copy(out, in)
	return out
}
