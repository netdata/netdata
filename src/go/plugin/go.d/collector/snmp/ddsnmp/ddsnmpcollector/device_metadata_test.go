// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"
	"io"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

func TestMetadataOnlySourceReportsEnrichmentFailure(t *testing.T) {
	profile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{Metadata: ddprofiledefinition.MetadataConfig{"device": {Fields: map[string]ddprofiledefinition.MetadataField{
		"vendor": {Value: "fixture"}, "serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: "1.2.3", Format: "hex"}},
	}}}}}
	for _, client := range []responseClient{
		{},
		{err: &gosnmp.StatusError{Status: gosnmp.GenErr}},
		{packet: &gosnmp.Message{VarBinds: []gosnmp.VarBind{{Name: "1.2.3", Value: gosnmp.IntegerValue(1)}}}},
	} {
		_, err := CollectDeviceMetadata(context.Background(), client, gosnmp.Version1, 10, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
		require.Error(t, err, "a static field must not hide failed dynamic enrichment")
	}
	packet := &gosnmp.Message{VarBinds: []gosnmp.VarBind{{Name: "1.2.3", Value: gosnmp.NoSuchObjectValue()}}}
	meta, err := CollectDeviceMetadata(context.Background(), responseClient{packet: packet}, gosnmp.Version1, 10, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
	require.NoError(t, err)
	require.Equal(t, "fixture", meta["vendor"].Value)
	packet.VarBinds = []gosnmp.VarBind{{Name: "1.2.3", Value: gosnmp.OctetStringValue([]byte("serial"))}}
	meta, err = CollectDeviceMetadata(context.Background(), responseClient{packet: packet}, gosnmp.Version1, 10, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
	require.NoError(t, err)
	require.Equal(t, "73657269616c", meta["serial_number"].Value, "missing OIDs are retried by the next attempt")
}

func TestMetadataOnlyFallbackClearsOnlyResolvedFieldErrors(t *testing.T) {
	for _, unrelatedFailure := range []bool{false, true} {
		fields := map[string]ddprofiledefinition.MetadataField{
			"serial_number": {Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.2.3", Format: "hex"}, {OID: "1.2.4"}}},
		}
		if unrelatedFailure {
			fields["model"] = ddprofiledefinition.MetadataField{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.2.3", Format: "hex"}}
		}
		profile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{Metadata: ddprofiledefinition.MetadataConfig{"device": {Fields: fields}}}}
		packet := &gosnmp.Message{VarBinds: []gosnmp.VarBind{{Name: "1.2.3", Value: gosnmp.IntegerValue(42)}, {Name: "1.2.4", Value: gosnmp.OctetStringValue([]byte("fallback"))}}}
		meta, err := CollectDeviceMetadata(context.Background(), responseClient{packet: packet}, gosnmp.Version1, 10, []*ddsnmp.Profile{profile}, "", logger.NewWithWriter(io.Discard))
		if unrelatedFailure {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, "fallback", meta["serial_number"].Value)
		}
	}
}
