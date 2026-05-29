// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cato_networks/catofunc"
)

func TestTopologyFunction(t *testing.T) {
	tests := map[string]struct {
		setup func(*testing.T, *Collector)
		check func(*testing.T, *Collector)
	}{
		"returns current topology": {
			setup: func(t *testing.T, c *Collector) {
				c.client = newFixtureAPIClient()
				c.now = fixedCatoTestNow
				initCollector(t, c)
				collectOnce(t, c)
			},
			check: func(t *testing.T, c *Collector) {
				resp := c.funcRouter.Handle(context.Background(), catofunc.TopologyMethodID, nil)
				require.Equal(t, 200, resp.Status)
				require.Equal(t, "topology", resp.ResponseType)
				data, ok := resp.Data.(*topologyv1.Data)
				require.True(t, ok)
				validateCatoTopologyV1Data(t, data)
				require.Equal(t, topologyv1.SchemaVersion, data.SchemaVersion)
				require.Equal(t, topologySource, data.Producer.Source)
				require.Greater(t, data.Actors.Rows, 0)
				require.Greater(t, data.Links.Rows, 0)
			},
		},
		"requires job selection": {
			check: func(t *testing.T, _ *Collector) {
				cfg := catofunc.Methods(defaultUpdateEvery)[0]
				require.False(t, cfg.AgentWide)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.AccountID = "12345"
			c.APIKey = "secret"
			if tc.setup != nil {
				tc.setup(t, c)
			}
			tc.check(t, c)
		})
	}
}

func TestBuildTopology(t *testing.T) {
	tests := map[string]struct {
		build func(*testing.T) *topologyv1.Data
		check func(*testing.T, *topologyv1.Data)
	}{
		"omits unavailable tunnel metrics": {
			build: func(t *testing.T) *topologyv1.Data {
				return mustBuildTopology(t, "12345", map[string]*siteState{
					"1001": {
						ID:                 "1001",
						Name:               "Paris Office",
						ConnectivityStatus: "connected",
						PopName:            "POP-Paris",
						Interfaces:         make(map[string]*interfaceState),
					},
				}, []string{"1001"}, fixedCatoTestNow())
			},
			check: func(t *testing.T, data *topologyv1.Data) {
				rows := topologyTableRows(t, data.Links, data.Dictionaries)
				require.Len(t, rows, 1)
				require.Equal(t, catofunc.LinkTypeTunnel, rows[0]["type"])
				require.Nil(t, rows[0]["bytes_upstream_max"])
				require.Nil(t, rows[0]["bytes_downstream_max"])
				require.Nil(t, rows[0]["lost_upstream_percent"])
				require.Nil(t, rows[0]["rtt_ms"])
			},
		},
		"preserves empty BGP peer IP as an empty attribute": {
			build: func(t *testing.T) *topologyv1.Data {
				site := &siteState{
					ID:   "1001",
					Name: "Paris Office",
					BGPPeers: []bgpPeerState{
						{
							RemoteASN:  "64512",
							BGPSession: "Established",
						},
					},
				}
				return mustBuildTopology(t, "12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
			},
			check: func(t *testing.T, data *topologyv1.Data) {
				actors := topologyTableRows(t, data.Actors, data.Dictionaries)
				peerActor := requireTopologyRow(t, actors, "type", catofunc.ActorTypeBGPPeer)
				require.Empty(t, peerActor["remote_ip"])

				links := topologyTableRows(t, data.Links, data.Dictionaries)
				bgpLink := requireTopologyRow(t, links, "type", catofunc.LinkTypeBGP)
				require.Equal(t, peerActor["_row"], bgpLink["dst_actor"])
			},
		},
		"deduplicates BGP peers": {
			build: func(t *testing.T) *topologyv1.Data {
				site := &siteState{
					ID:   "1001",
					Name: "Paris Office",
					BGPPeers: []bgpPeerState{
						{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
						{RemoteIP: "192.0.2.10", RemoteASN: "64512", BGPSession: "established"},
					},
				}
				return mustBuildTopology(t, "12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
			},
			check: func(t *testing.T, data *topologyv1.Data) {
				actors := topologyTableRows(t, data.Actors, data.Dictionaries)
				links := topologyTableRows(t, data.Links, data.Dictionaries)
				require.Equal(t, 1, countTopologyRows(actors, "type", catofunc.ActorTypeBGPPeer))
				require.Equal(t, 1, countTopologyRows(links, "type", catofunc.LinkTypeBGP))
			},
		},
		"site topology tables are deterministic": {
			build: func(t *testing.T) *topologyv1.Data {
				site := &siteState{
					ID:   "1001",
					Name: "Paris Office",
					Interfaces: map[string]*interfaceState{
						"z": {Name: "WAN 2"},
						"a": {Name: "WAN 1"},
					},
					Devices: []deviceState{
						{ID: "z", Name: "Socket 2"},
						{ID: "a", Name: "Socket 1"},
					},
				}
				return mustBuildTopology(t, "12345", map[string]*siteState{site.ID: site}, []string{site.ID}, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC))
			},
			check: func(t *testing.T, data *topologyv1.Data) {
				require.NotNil(t, data.Tables)
				interfaces := topologyTableRows(t, data.Tables.Actor[catofunc.ActorTableInterfaces].Table, data.Dictionaries)
				devices := topologyTableRows(t, data.Tables.Actor[catofunc.ActorTableDevices].Table, data.Dictionaries)
				require.Equal(t, "WAN 1", interfaces[0]["name"])
				require.Equal(t, "WAN 2", interfaces[1]["name"])
				require.Equal(t, "Socket 1", devices[0]["name"])
				require.Equal(t, "Socket 2", devices[1]["name"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data := tc.build(t)
			validateCatoTopologyV1Data(t, data)
			tc.check(t, data)
		})
	}
}

func mustBuildTopology(t *testing.T, accountID string, sites map[string]*siteState, order []string, collectedAt time.Time) *topologyv1.Data {
	t.Helper()

	data, err := buildTopology(accountID, sites, order, collectedAt)
	require.NoError(t, err)
	return data
}

func validateCatoTopologyV1Data(t *testing.T, data *topologyv1.Data) {
	t.Helper()

	payload, err := json.Marshal(data)
	require.NoError(t, err)

	var decodedData map[string]any
	require.NoError(t, json.Unmarshal(payload, &decodedData))
	require.NoError(t, topologyv1.ValidateDecodedData(decodedData))

	schemaBytes, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "plugins.d", "FUNCTION_TOPOLOGY_SCHEMA.json")))
	require.NoError(t, err)

	var schemaDoc any
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaDoc))

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", schemaDoc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	response := map[string]any{
		"status": 200,
		"type":   topologyv1.ResponseType,
		"data":   decodedData,
	}
	require.NoError(t, schema.Validate(response))
}

func topologyTableRows(t *testing.T, table topologyv1.Table, dictionaries topologyv1.Dictionaries) []map[string]any {
	t.Helper()

	rows := make([]map[string]any, table.Rows)
	for row := range rows {
		rows[row] = map[string]any{"_row": row}
	}

	for columnIndex, column := range table.Columns {
		values := topologyColumnValues(t, table.Rows, table.Values[columnIndex])
		for rowIndex, value := range values {
			if column.Type == "string_ref" {
				value = resolveTopologyStringRef(t, dictionaries, column.Dictionary, value)
			}
			rows[rowIndex][column.ID] = value
		}
	}
	return rows
}

func topologyColumnValues(t *testing.T, rows int, encoding topologyv1.ColumnEncoding) []any {
	t.Helper()

	switch value := encoding.(type) {
	case topologyv1.ConstEncoding:
		values := make([]any, rows)
		for i := range values {
			values[i] = value.Value
		}
		return values
	case topologyv1.ValuesEncoding:
		require.Len(t, value.Values, rows)
		return value.Values
	case topologyv1.DictEncoding:
		require.Len(t, value.Indexes, rows)
		values := make([]any, rows)
		for i, index := range value.Indexes {
			require.GreaterOrEqual(t, index, 0)
			require.Less(t, index, len(value.Values))
			values[i] = value.Values[index]
		}
		return values
	default:
		t.Fatalf("unsupported topology column encoding %T", encoding)
		return nil
	}
}

func resolveTopologyStringRef(t *testing.T, dictionaries topologyv1.Dictionaries, name string, value any) string {
	t.Helper()

	index, ok := value.(int)
	require.True(t, ok)
	values := dictionaries[name]
	require.Less(t, index, len(values))
	text, ok := values[index].(string)
	require.True(t, ok)
	return text
}

func requireTopologyRow(t *testing.T, rows []map[string]any, key string, value any) map[string]any {
	t.Helper()

	for _, row := range rows {
		if row[key] == value {
			return row
		}
	}
	t.Fatalf("topology row with %s=%v not found", key, value)
	return nil
}

func countTopologyRows(rows []map[string]any, key string, value any) int {
	var count int
	for _, row := range rows {
		if row[key] == value {
			count++
		}
	}
	return count
}
