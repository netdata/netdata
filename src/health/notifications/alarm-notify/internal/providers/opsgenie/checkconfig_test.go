// SPDX-License-Identifier: GPL-3.0-or-later
package opsgenie

import (
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/require"
)

func checkFormConfig(t *testing.T, dst Config, wantErr string) {
	t.Helper()
	want := testutil.Document[Config]{Version: 1, Destinations: map[string]Config{"target": dst}}
	data, err := marshalConfig(want)
	require.NoError(t, err)
	testutil.CheckConfig(t, data, want, wantErr, readConfig)
}
