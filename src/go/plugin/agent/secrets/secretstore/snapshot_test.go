// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testPublishedStore struct{}

func (testPublishedStore) Resolve(_ context.Context, _ ResolveRequest) (string, error) {
	return "", nil
}

func TestSnapshotLookupStore(t *testing.T) {
	tests := map[string]struct {
		snapshot *Snapshot
		id       string
		wantOK   bool
	}{
		"nil snapshot": {
			snapshot: nil,
			id:       "s1",
			wantOK:   false,
		},
		"empty stores map": {
			snapshot: &Snapshot{stores: map[string]publishedRecord{}},
			id:       "s1",
			wantOK:   false,
		},
		"not found": {
			snapshot: &Snapshot{
				stores: map[string]publishedRecord{
					"s1": {},
				},
			},
			id:     "s2",
			wantOK: false,
		},
		"found": {
			snapshot: &Snapshot{
				stores: map[string]publishedRecord{
					"s1": {published: testPublishedStore{}},
				},
			},
			id:     "s1",
			wantOK: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store, ok := tc.snapshot.lookupStore(tc.id)
			if !assert.Equal(t, tc.wantOK, ok, "lookupStore(%q) ok mismatch", tc.id) {
				return
			}
			if ok {
				assert.NotNil(t, store.published)
			}
		})
	}
}
