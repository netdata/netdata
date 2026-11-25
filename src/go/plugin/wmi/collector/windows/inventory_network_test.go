// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultInterface(t *testing.T) {
	route := map[string]any{"InterfaceIndex": uint32(math.MaxUint32)}
	adapter := map[string]any{"InterfaceIndex": "4294967295", "NetConnectionID": " Ethernet 2 "}
	ipConfig := func(addresses ...string) map[string]any {
		return map[string]any{"InterfaceIndex": uint32(math.MaxUint32), "IPAddress": addresses}
	}
	adapters := fixtureRows(adapter)
	config := fixtureRows(ipConfig("fe80::1", "192.0.2.7", "192.0.2.7"))
	queryErr := errors.New("configuration unavailable")
	tests := map[string]struct {
		routes   queryResult
		adapters queryResult
		configs  queryResult
		want     [2]string
		err      error
	}{
		"distinct default routes on the same interface": {
			routes:   fixtureRows(route, map[string]any{"InterfaceIndex": "4294967295"}),
			adapters: adapters,
			configs:  config,
			want:     [2]string{"Ethernet 2", "192.0.2.7"},
		},
		"different default interfaces": {
			routes: fixtureRows(route, map[string]any{"InterfaceIndex": uint32(12)}),
		},
		"missing route index": {
			routes: fixtureRows(route, map[string]any{}),
		},
		"invalid route index": {
			routes: fixtureRows(map[string]any{"InterfaceIndex": uint64(math.MaxUint32) + 1}),
		},
		"no default route": {},
		"wrong adapter": {
			routes:   fixtureRows(route),
			adapters: fixtureRows(map[string]any{"InterfaceIndex": uint32(12), "NetConnectionID": "Ethernet"}),
			configs:  config,
		},
		"ambiguous adapter": {
			routes:   fixtureRows(route),
			adapters: fixtureRows(adapter, adapter),
			configs:  config,
		},
		"multiple usable IPv4 addresses": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  fixtureRows(ipConfig("192.0.2.7", "192.0.2.8")),
		},
		"no usable IPv4 address": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs: fixtureRows(
				ipConfig("fe80::1", "127.0.0.1", "169.254.1.1", "0.0.0.0", "224.0.0.1", "255.255.255.255"),
			),
		},
		"malformed address": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs:  fixtureRows(ipConfig("192.0.2.7", "malformed")),
		},
		"query error": {
			routes:   fixtureRows(route),
			adapters: adapters,
			configs: queryResult{
				err: queryErr,
			},
			err: queryErr,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			session := &fakeSession{
				results: map[string]queryResult{
					queryDefaultRoutes: tt.routes,
					fmt.Sprintf(queryAdapterNameFormat, uint64(math.MaxUint32)): tt.adapters,
					fmt.Sprintf(queryAdapterIPsFormat, uint64(math.MaxUint32)):  tt.configs,
				},
			}
			name, ip, err := defaultInterface(t.Context(), session)
			require.ErrorIs(t, err, tt.err)
			assert.Equal(t, tt.want, [2]string{name, ip})
		})
	}
}
