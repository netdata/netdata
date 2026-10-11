// SPDX-License-Identifier: GPL-3.0-or-later

package privileged

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIdentityParsers(t *testing.T) {
	stat := "42 (name with ) inside) S " + strings.Repeat("0 ", 18) + "12345 0"
	start, err := startTime(stat)
	require.NoError(t, err)
	require.EqualValues(t, 12345, start)
	_, err = startTime("42 broken")
	require.Error(t, err)
	uid, gid, err := credentials("Uid:\t1001\t1001\t1001\t1001\nGid:\t1002\t1002\t1002\t1002\n")
	require.NoError(t, err)
	require.EqualValues(t, 1001, uid)
	require.EqualValues(t, 1002, gid)
	for _, status := range []string{"Uid: 0 1001 0 1001\nGid: 1 1 1 1", "Uid: 1 1 1 1", "Uid: 1 1 1 1\nGid: 1 2 1 1"} {
		_, _, err = credentials(status)
		require.Error(t, err)
	}
}

func TestApplicationNames(t *testing.T) {
	tests := []struct {
		args   []string
		name   string
		helper bool
	}{
		{[]string{"java", "-jar", "/opt/apps/orders.jar"}, "orders.jar", false},
		{[]string{"java", "-cp", "/x", "org.example.Main"}, "org.example.Main", false},
		{[]string{"java", "-Dservice.name=payment;secret=oops", "-jar", "app.jar"}, "payment_secret_oops", false},
		{[]string{"java", "-Dservice.name=other", "-Dspring.application.name=orders", "Main"}, "orders", false},
		{[]string{"java", "-cp", "/helper", "NetdataAttach"}, "NetdataAttach", true},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			name, helper := application(tt.args)
			require.Equal(t, tt.name, name)
			require.Equal(t, tt.helper, helper)
		})
	}
	require.Len(t, displayName(strings.Repeat("a", 200)), 128)
}
