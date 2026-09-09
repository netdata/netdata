// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"io"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

func TestMetadataOnlySourceReportsEnrichmentFailure(t *testing.T) {
	profile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{Metadata: ddprofiledefinition.MetadataConfig{"device": {Fields: map[string]ddprofiledefinition.MetadataField{
		"vendor": {Value: "fixture"}, "serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: "1.2.3", Format: "hex"}},
	}}}}}
	for _, packet := range []*gosnmp.SnmpPacket{nil, {Error: gosnmp.GenErr}, {Variables: []gosnmp.SnmpPDU{{Name: "1.2.3", Type: gosnmp.Opaque, Value: struct{}{}}}}} {
		_, err := CollectDeviceMetadata(responseClient{packet: packet}, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
		require.Error(t, err, "a static field must not hide failed dynamic enrichment")
	}
	packet := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: "1.2.3", Type: gosnmp.NoSuchObject}}}
	meta, err := CollectDeviceMetadata(responseClient{packet: packet}, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
	require.NoError(t, err)
	require.Equal(t, "fixture", meta["vendor"].Value)
	packet.Variables = []gosnmp.SnmpPDU{{Name: "1.2.3", Type: gosnmp.OctetString, Value: []byte("serial")}}
	meta, err = CollectDeviceMetadata(responseClient{packet: packet}, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
	require.NoError(t, err)
	require.Equal(t, "73657269616c", meta["serial_number"].Value, "missing OIDs are retried by the next attempt")
}
