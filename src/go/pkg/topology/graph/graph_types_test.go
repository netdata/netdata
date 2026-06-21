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
			Layer:      "3",
			Protocol:   "ospf",
			LinkType:   "ospf_adjacency",
			Direction:  "bidirectional",
			State:      "up",
			SrcActorID: "actor-a",
			DstActorID: "actor-b",
			Src: LinkEndpoint{
				Match:         Match{IPAddresses: []string{"10.0.0.1"}},
				IfIndex:       1,
				IfName:        "eth0",
				IfDescr:       "uplink",
				IfAlias:       "core",
				PortID:        "1",
				PortName:      "eth0",
				BridgePort:    "11",
				SysName:       "router-a",
				ManagementIP:  "10.0.0.1",
				DisplayName:   "router-a:eth0",
				DisplaySource: "interface",
				AdminStatus:   "up",
				OperStatus:    "up",
			},
			Dst: LinkEndpoint{
				Match:    Match{IPAddresses: []string{"10.0.0.2"}},
				IfName:   "eth1",
				PortName: "eth1",
			},
			DiscoveredAt: &tm,
			LastSeen:     &tm,
			Display:      &LinkDisplay{Name: "router-a:eth0 -> router-b:eth1", SrcPortName: "eth0", DstPortName: "eth1"},
			L2:           &LinkL2{BridgeDomain: "bd-a", Designated: true, PairID: "pair-a", PairPass: "default", PairConsistent: true},
			Inference:    &LinkInference{Inference: "probable", Confidence: "low", AttachmentMode: "probable_direct"},
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
				"src":{
					"match":{"ip_addresses":["10.0.0.1"]},
					"if_index":1,
					"if_name":"eth0",
					"if_descr":"uplink",
					"if_alias":"core",
					"port_id":"1",
					"port_name":"eth0",
					"bridge_port":"11",
					"sys_name":"router-a",
					"management_ip":"10.0.0.1",
					"display_name":"router-a:eth0",
					"display_source":"interface",
					"if_admin_status":"up",
					"if_oper_status":"up"
				},
				"dst":{"match":{"ip_addresses":["10.0.0.2"]},"if_name":"eth1","port_name":"eth1"},
				"discovered_at":"2026-06-20T08:09:10Z",
				"last_seen":"2026-06-20T08:09:10Z",
				"display":{"name":"router-a:eth0 -> router-b:eth1","src_port_name":"eth0","dst_port_name":"eth1"},
				"l2":{"bridge_domain":"bd-a","designated":true,"pair_id":"pair-a","pair_pass":"default","pair_consistent":true},
				"inference":{"inference":"probable","confidence":"low","attachment_mode":"probable_direct"}
			}]
		}`, string(bs))
}
