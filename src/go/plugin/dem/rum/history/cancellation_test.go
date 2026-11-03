// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScanCancellation(t *testing.T) {
	s, _ := newTestStore(t)
	for range 4 {
		appendEvent(t, s, EventRecord{
			Type:      "pageview",
			Site:      "shop",
			SessionID: "s",
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	err := s.scan(ctx, "shop", 0, time.Now().Unix()+1, func(EventRecord) { count++; cancel() })
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, count)
}

func TestCancelledDomainQueries(t *testing.T) {
	s, _ := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempted, err := s.AppendEvent(ctx, EventRecord{
		Type:      "activity",
		Site:      "shop",
		SessionID: "s",
	})
	assert.False(t, attempted)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.QuerySessions(ctx, "", "", 0, time.Now().Unix(), 0)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.QuerySessionEvents(ctx, "", "s")
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.QueryErrors(ctx, "", "", 0, time.Now().Unix())
	assert.ErrorIs(t, err, context.Canceled)
}
