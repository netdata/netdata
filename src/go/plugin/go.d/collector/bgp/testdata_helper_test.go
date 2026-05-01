// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := loadSharedTestData(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to load BGP test data: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func loadSharedTestData() error {
	read := func(path string) ([]byte, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return data, nil
	}

	var err error

	if dataConfigJSON, err = read("testdata/config.json"); err != nil {
		return err
	}
	if dataConfigYAML, err = read("testdata/config.yaml"); err != nil {
		return err
	}
	if dataFRRIPv4Summary, err = read("testdata/frr/show_bgp_vrf_all_ipv4_summary.json"); err != nil {
		return err
	}
	if dataFRRIPv4SummaryDeep, err = read("testdata/frr/show_bgp_vrf_all_ipv4_summary_deep_prefixes.json"); err != nil {
		return err
	}
	if dataFRRIPv4SummaryPfxSnt, err = read("testdata/frr/show_bgp_vrf_all_ipv4_summary_pfxsnt.json"); err != nil {
		return err
	}
	if dataFRRIPv6Summary, err = read("testdata/frr/show_bgp_vrf_all_ipv6_summary.json"); err != nil {
		return err
	}
	if dataFRREVPNSummary, err = read("testdata/frr/show_bgp_vrf_all_l2vpn_evpn_summary.json"); err != nil {
		return err
	}
	if dataFRREVPNVNI, err = read("testdata/frr/show_evpn_vni.json"); err != nil {
		return err
	}
	if dataFRRNeighbors, err = read("testdata/frr/show_bgp_vrf_all_neighbors.json"); err != nil {
		return err
	}
	if dataFRRNeighborsEnriched, err = read("testdata/frr/show_bgp_vrf_all_neighbors_enriched.json"); err != nil {
		return err
	}
	if dataFRRPeerRoutesDefault, err = read("testdata/frr/show_bgp_ipv4_unicast_neighbor_192.168.0.2_routes.json"); err != nil {
		return err
	}
	if dataFRRPeerRoutesRed, err = read("testdata/frr/show_bgp_vrf_red_ipv4_unicast_neighbor_192.168.1.2_routes.json"); err != nil {
		return err
	}
	if dataFRRPeerAdvDefault, err = read("testdata/frr/show_bgp_ipv4_unicast_neighbor_192.168.0.2_advertised_routes.json"); err != nil {
		return err
	}
	if dataFRRPeerAdvRed, err = read("testdata/frr/show_bgp_vrf_red_ipv4_unicast_neighbor_192.168.1.2_advertised_routes.json"); err != nil {
		return err
	}
	if dataFRRNeighborsRich, err = read("testdata/frr/show_bgp_vrf_all_neighbors_rich.json"); err != nil {
		return err
	}
	if dataFRRIPv4SummaryDualstack, err = read("testdata/frr/show_bgp_vrf_all_ipv4_summary_dualstack.json"); err != nil {
		return err
	}
	if dataFRRIPv6SummaryDualstack, err = read("testdata/frr/show_bgp_vrf_all_ipv6_summary_dualstack.json"); err != nil {
		return err
	}
	if dataFRRNeighborsDualstack, err = read("testdata/frr/show_bgp_vrf_all_neighbors_dualstack.json"); err != nil {
		return err
	}
	if dataFRRRPKICacheServer, err = read("testdata/frr/show_rpki_cache_server.json"); err != nil {
		return err
	}
	if dataFRRRPKICacheConnection, err = read("testdata/frr/show_rpki_cache_connection.json"); err != nil {
		return err
	}
	if dataFRRRPKICacheConnectionDisconnected, err = read("testdata/frr/show_rpki_cache_connection_disconnected.json"); err != nil {
		return err
	}
	if dataFRRRPKIPrefixCount, err = read("testdata/frr/show_rpki_prefix_count.json"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllAccessDenied, err = read("testdata/bird/show_protocols_all_access_denied.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllMultichannel, err = read("testdata/bird/show_protocols_all_multichannel.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllLegacy, err = read("testdata/bird/show_protocols_all_legacy.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllBird3, err = read("testdata/bird/show_protocols_all_bird3.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllAdvanced, err = read("testdata/bird/show_protocols_all_advanced_families.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllMPLSVPNAliases, err = read("testdata/bird/show_protocols_all_mpls_vpn_aliases.txt"); err != nil {
		return err
	}
	if dataBIRDProtocolsAllRPKI, err = read("testdata/bird/show_protocols_all_rpki.txt"); err != nil {
		return err
	}
	if dataOpenBGPDNeighbors, err = read("testdata/openbgpd/show_neighbor.json"); err != nil {
		return err
	}
	if dataOpenBGPDNeighborsMultifamily, err = read("testdata/openbgpd/show_neighbor_multifamily.json"); err != nil {
		return err
	}
	if dataOpenBGPDRIB, err = read("testdata/openbgpd/rib.json"); err != nil {
		return err
	}
	if dataOpenBGPDNeighborsLiveActive, err = read("testdata/openbgpd/show_neighbor_live_active.json"); err != nil {
		return err
	}
	if dataOpenBGPDRIBLiveActive, err = read("testdata/openbgpd/rib_live_active.json"); err != nil {
		return err
	}

	return nil
}
