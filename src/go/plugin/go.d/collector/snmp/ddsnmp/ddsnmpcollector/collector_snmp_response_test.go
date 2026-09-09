// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

type responseClient struct {
	gosnmp.Handler
	packet  *gosnmp.SnmpPacket
	version gosnmp.SnmpVersion
}

func (c responseClient) Get([]string) (*gosnmp.SnmpPacket, error) { return c.packet, nil }
func (c responseClient) MaxOids() int                             { return 10 }
func (c responseClient) Version() gosnmp.SnmpVersion              { return c.version }

type indexedMissingClient struct {
	gosnmp.Handler
	requests [][]string
}

func (c *indexedMissingClient) MaxOids() int                { return 10 }
func (c *indexedMissingClient) Version() gosnmp.SnmpVersion { return gosnmp.Version1 }
func (c *indexedMissingClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	c.requests = append(c.requests, slices.Clone(oids))
	packet := &gosnmp.SnmpPacket{}
	for i, oid := range oids {
		packet.Variables = append(packet.Variables, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Null})
		if oid != "1.2.4" && packet.Error == gosnmp.NoError {
			packet.Error, packet.ErrorIndex = gosnmp.NoSuchName, uint8(i+1)
		}
	}
	if packet.Error == gosnmp.NoError {
		packet.Variables = []gosnmp.SnmpPDU{{Name: "1.2.4", Type: gosnmp.OctetString, Value: []byte("serial")}}
	}
	return packet, nil
}

func TestMetadataGETRecoversIndexedV1MissingOIDs(t *testing.T) {
	client := &indexedMissingClient{}
	oids := []string{"1.2.3", "1.2.4", "1.2.5"}
	missing := map[string]bool{}
	stats := &ddsnmp.CollectionStats{}
	recorder := &SourceRecorder{ContextID: 7}
	evidence := &negativeEvidence{}
	observed := &diagnosticClient{Handler: recorder.Wrap(client), negative: evidence, failures: &ddsnmp.CollectionFailures{}}
	for _, oid := range oids {
		require.False(t, isMissingOID(observed, missing, oid))
	}
	pdus, err := getSNMPValues(observed, oids, missing, stats)
	require.NoError(t, err)
	require.Equal(t, []string{"1.2.3", "1.2.4", "1.2.5"}, oids, "do not mutate callers' OID lists")
	require.Equal(t, [][]string{{"1.2.3", "1.2.4", "1.2.5"}, {"1.2.4", "1.2.5"}, {"1.2.4"}}, client.requests)
	require.Equal(t, map[string]bool{"1.2.3": true, "1.2.5": true}, missing)
	require.Equal(t, []byte("serial"), pdus["1.2.4"].Value)
	require.EqualValues(t, 3, stats.SNMP.GetRequests)
	require.EqualValues(t, 6, stats.SNMP.GetOIDs)
	require.EqualValues(t, 2, stats.Errors.MissingOIDs)
	require.Zero(t, stats.Errors.SNMP)
	require.EqualValues(t, 1, evidence.causes["1.2.3"].Operation)
	require.EqualValues(t, 2, evidence.causes["1.2.5"].Operation)
	require.EqualValues(t, 7, evidence.causes["1.2.5"].ContextID)
}
func TestMetadataGETRejectsPacketError(t *testing.T) {
	_, err := getSNMPValues(responseClient{packet: &gosnmp.SnmpPacket{Error: gosnmp.GenErr}}, []string{"1.2.3"}, map[string]bool{}, &ddsnmp.CollectionStats{})
	require.Error(t, err)
}

func TestMetadataGETV1AllMissing(t *testing.T) {
	client := &indexedMissingClient{}
	missing := map[string]bool{}
	stats := &ddsnmp.CollectionStats{}
	pdus, err := getSNMPValues(client, []string{"1.2.3", "1.2.5"}, missing, stats)
	require.NoError(t, err)
	require.Empty(t, pdus)
	require.Len(t, missing, 2)
	require.EqualValues(t, 2, stats.SNMP.GetRequests)
	require.Zero(t, stats.Errors.SNMP)
}

func TestMetadataGETRejectsUnrecoverableMissingResponse(t *testing.T) {
	for name, client := range map[string]responseClient{
		"v1 zero index":      {version: gosnmp.Version1, packet: &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName}},
		"v1 outside batch":   {version: gosnmp.Version1, packet: &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, ErrorIndex: 2}},
		"v2c missing status": {version: gosnmp.Version2c, packet: &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, ErrorIndex: 1}},
		"nil packet":         {},
	} {
		t.Run(name, func(t *testing.T) {
			missing := map[string]bool{}
			stats := &ddsnmp.CollectionStats{}
			_, err := getSNMPValues(client, []string{"1.2.3"}, missing, stats)
			require.Error(t, err)
			require.Empty(t, missing)
			require.EqualValues(t, 1, stats.SNMP.GetRequests)
			require.EqualValues(t, 1, stats.Errors.SNMP)
		})
	}
}
