// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"slices"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

type responseClient struct {
	snmputils.Client
	packet  *gosnmp.Message
	err     error
	version gosnmp.Version
}

func (c responseClient) Get(context.Context, []string) (*gosnmp.Message, error) {
	return c.packet, c.err
}

type indexedMissingClient struct {
	snmputils.Client
	requests [][]string
}

func (c *indexedMissingClient) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	c.requests = append(c.requests, slices.Clone(oids))
	for i, oid := range oids {
		if oid != "1.2.4" {
			return nil, &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: uint8(i + 1)}
		}
	}
	return &gosnmp.Message{VarBinds: []gosnmp.VarBind{{Name: "1.2.4", Value: gosnmp.OctetStringValue([]byte("serial"))}}}, nil
}

func TestMetadataGETRecoversIndexedV1MissingOIDs(t *testing.T) {
	client := &indexedMissingClient{}
	oids := []string{"1.2.3", "1.2.4", "1.2.5"}
	missing := map[string]bool{}
	stats := &ddsnmp.CollectionStats{}
	recorder := &SourceRecorder{ContextID: 7}
	evidence := &negativeEvidence{}
	observed := &diagnosticClient{Client: recorder.Wrap(client), negative: evidence, failures: &ddsnmp.CollectionFailures{}}
	for _, oid := range oids {
		require.False(t, isMissingOID(observed, missing, oid))
	}
	pdus, err := getSNMPValues(context.Background(), observed, requestConfig{version: gosnmp.Version1, maxOIDs: 10}, oids, missing, stats)
	require.NoError(t, err)
	require.Equal(t, []string{"1.2.3", "1.2.4", "1.2.5"}, oids, "do not mutate callers' OID lists")
	require.Equal(t, [][]string{{"1.2.3", "1.2.4", "1.2.5"}, {"1.2.4", "1.2.5"}, {"1.2.4"}}, client.requests)
	require.Equal(t, map[string]bool{"1.2.3": true, "1.2.5": true}, missing)
	require.Equal(t, gosnmp.OctetStringValue([]byte("serial")), pdus["1.2.4"].Value)
	require.EqualValues(t, 3, stats.SNMP.GetRequests)
	require.EqualValues(t, 6, stats.SNMP.GetOIDs)
	require.EqualValues(t, 2, stats.Errors.MissingOIDs)
	require.Zero(t, stats.Errors.SNMP)
	require.EqualValues(t, 1, evidence.causes["1.2.3"].Operation)
	require.EqualValues(t, 2, evidence.causes["1.2.5"].Operation)
	require.EqualValues(t, 7, evidence.causes["1.2.5"].ContextID)
}
func TestMetadataGETRejectsPacketError(t *testing.T) {
	_, err := getSNMPValues(context.Background(), responseClient{err: &gosnmp.StatusError{Status: gosnmp.GenErr}}, requestConfig{version: gosnmp.Version1, maxOIDs: 10}, []string{"1.2.3"}, map[string]bool{}, &ddsnmp.CollectionStats{})
	require.Error(t, err)
}

func TestMetadataGETV1AllMissing(t *testing.T) {
	client := &indexedMissingClient{}
	missing := map[string]bool{}
	stats := &ddsnmp.CollectionStats{}
	pdus, err := getSNMPValues(context.Background(), client, requestConfig{version: gosnmp.Version1, maxOIDs: 10}, []string{"1.2.3", "1.2.5"}, missing, stats)
	require.NoError(t, err)
	require.Empty(t, pdus)
	require.Len(t, missing, 2)
	require.EqualValues(t, 2, stats.SNMP.GetRequests)
	require.Zero(t, stats.Errors.SNMP)
}

func TestMetadataGETRejectsUnrecoverableMissingResponse(t *testing.T) {
	for name, client := range map[string]responseClient{
		"v1 zero index":      {version: gosnmp.Version1, err: &gosnmp.StatusError{Status: gosnmp.NoSuchName}},
		"v1 outside batch":   {version: gosnmp.Version1, err: &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: 2}},
		"v2c missing status": {version: gosnmp.Version2c, err: &gosnmp.StatusError{Status: gosnmp.NoSuchName, Index: 1}},
		"nil packet":         {},
	} {
		t.Run(name, func(t *testing.T) {
			missing := map[string]bool{}
			stats := &ddsnmp.CollectionStats{}
			_, err := getSNMPValues(context.Background(), client, requestConfig{version: client.version, maxOIDs: 10}, []string{"1.2.3"}, missing, stats)
			require.Error(t, err)
			require.Empty(t, missing)
			require.EqualValues(t, 1, stats.SNMP.GetRequests)
			require.EqualValues(t, 1, stats.Errors.SNMP)
		})
	}
}
