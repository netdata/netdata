// SPDX-License-Identifier: GPL-3.0-or-later
package kafka

import (
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func checkFormConfig(t *testing.T, dst Config, wantErr string) {
	t.Helper()
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"target": dst}}
	data, err := marshalConfig(want)
	require.NoError(t, err)
	got, err := readConfig(strings.NewReader(string(data)))
	if wantErr != "" {
		require.ErrorContains(t, err, wantErr)
		assert.NotContains(t, err.Error(), "synthetic-private-value")
		assert.Equal(t, testutil.Document[Config]{}, got)
	} else {
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}
