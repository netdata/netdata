// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_OpenTransport(t *testing.T) {
	tr, err := New(Config{
		Device:  1,
		Timeout: time.Second,
	}).newTransport()
	require.NoError(t, err)
	open, ok := tr.(*openTransport)
	require.True(t, ok)
	assert.Equal(t, client.InterfaceOpen, open.sdk.Interface)

	// The pinned SDK has no public accessor for its backend state. Read (never
	// mutate) the field that ConnectOpen requires.
	backend := reflect.ValueOf(open.sdk).Elem().FieldByName("openipmi")
	require.True(t, backend.IsValid())
	assert.False(t, backend.IsNil(), "the local SDK client lacks OpenIPMI state")

	assert.NoError(t, open.Close(t.Context()), "closing a client that never connected")
}

func TestReceiveTimeout(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	budget, cancelBudget := context.WithTimeout(context.Background(), time.Second)
	defer cancelBudget()

	tests := map[string]struct {
		ctx        context.Context
		configured time.Duration
		want       time.Duration
		wantAtMost bool // the result depends on elapsed time: 0 < got <= want
		wantErr    error
	}{
		"no deadline keeps the configured timeout": {
			ctx:        context.Background(),
			configured: time.Second,
			want:       time.Second,
		},
		"remaining budget caps the configured timeout": {
			ctx:        budget,
			configured: time.Hour,
			want:       time.Second,
			wantAtMost: true,
		},
		"configured timeout below the budget": {
			ctx:        budget,
			configured: time.Millisecond,
			want:       time.Millisecond,
		},
		"canceled": {
			ctx:        canceled,
			configured: time.Hour,
			wantErr:    context.Canceled,
		},
		"expired deadline": {
			ctx:        expired,
			configured: time.Hour,
			wantErr:    context.DeadlineExceeded,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := receiveTimeout(tc.ctx, tc.configured)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantAtMost {
				assert.Positive(t, got)
				assert.LessOrEqual(t, got, tc.want)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
