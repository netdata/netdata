// SPDX-License-Identifier: GPL-3.0-or-later

package graph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGraphJSONShape(t *testing.T) {
	tm := time.Date(2026, time.June, 20, 8, 9, 10, 0, time.UTC)
	payload := Graph{
		SchemaVersion: "2.0",
		Source:        "snmp",
		Layer:         "3",
		AgentID:       "agent-id",
		CollectedAt:   tm,
		View:          "summary",
		Actors: []Actor{{
			ActorID:   "actor-a",
			ActorType: "router",
			Layer:     "3",
			Source:    "snmp",
			Match: Match{
				ChassisIDs:         []string{"chassis-a"},
				MacAddresses:       []string{"00:11:22:33:44:55"},
				IPAddresses:        []string{"10.0.0.1"},
				Hostnames:          []string{"router-a"},
				DNSNames:           []string{"router-a.example.net"},
				SysObjectID:        "1.3.6.1.4.1.8072.3.2.10",
				SysName:            "router-a",
				NetdataNodeID:      "node-id",
				NetdataMachineGUID: "machine-guid",
				CloudInstanceID:    "instance-id",
				CloudAccountID:     "account-id",
				ContainerIDs:       []string{"container-id"},
				PodNames:           []string{"pod-a"},
				NamespaceIDs:       []string{"namespace-a"},
			},
			ParentMatch: &Match{SysName: "parent"},
			Attributes:  map[string]any{"role": "core"},
			Labels:      map[string]string{"site": "lab"},
			Tables:      map[string][]map[string]any{"ports": {{"if_name": "eth0"}}},
		}},
		Links: []Link{{
			Layer:        "3",
			Protocol:     "ospf",
			LinkType:     "ospf_adjacency",
			Direction:    "bidirectional",
			State:        "up",
			SrcActorID:   "actor-a",
			DstActorID:   "actor-b",
			Src:          LinkEndpoint{Match: Match{IPAddresses: []string{"10.0.0.1"}}, Attributes: map[string]any{"if_name": "eth0"}},
			Dst:          LinkEndpoint{Match: Match{IPAddresses: []string{"10.0.0.2"}}, Attributes: map[string]any{"if_name": "eth1"}},
			DiscoveredAt: &tm,
			LastSeen:     &tm,
			Metrics:      map[string]any{"cost": float64(10)},
		}},
	}

	bs, err := json.Marshal(payload)
	require.NoError(t, err)

	require.JSONEq(t, `{
		"schema_version":"2.0",
		"source":"snmp",
		"layer":"3",
		"agent_id":"agent-id",
		"collected_at":"2026-06-20T08:09:10Z",
		"view":"summary",
		"actors":[{
			"actor_id":"actor-a",
			"actor_type":"router",
			"layer":"3",
			"source":"snmp",
			"match":{
				"chassis_ids":["chassis-a"],
				"mac_addresses":["00:11:22:33:44:55"],
				"ip_addresses":["10.0.0.1"],
				"hostnames":["router-a"],
				"dns_names":["router-a.example.net"],
				"sys_object_id":"1.3.6.1.4.1.8072.3.2.10",
				"sys_name":"router-a",
				"netdata_node_id":"node-id",
				"netdata_machine_guid":"machine-guid",
				"cloud_instance_id":"instance-id",
				"cloud_account_id":"account-id",
				"container_ids":["container-id"],
				"pod_names":["pod-a"],
				"namespace_ids":["namespace-a"]
			},
			"parent_match":{"sys_name":"parent"},
			"attributes":{"role":"core"},
			"labels":{"site":"lab"},
			"tables":{"ports":[{"if_name":"eth0"}]}
		}],
		"links":[{
			"layer":"3",
			"protocol":"ospf",
			"link_type":"ospf_adjacency",
			"direction":"bidirectional",
			"state":"up",
			"src_actor_id":"actor-a",
			"dst_actor_id":"actor-b",
			"src":{"match":{"ip_addresses":["10.0.0.1"]},"attributes":{"if_name":"eth0"}},
			"dst":{"match":{"ip_addresses":["10.0.0.2"]},"attributes":{"if_name":"eth1"}},
			"discovered_at":"2026-06-20T08:09:10Z",
			"last_seen":"2026-06-20T08:09:10Z",
			"metrics":{"cost":10}
		}]
	}`, string(bs))
}
