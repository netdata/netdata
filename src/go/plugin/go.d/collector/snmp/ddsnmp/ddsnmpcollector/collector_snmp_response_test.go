// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

type responseClient struct {
	gosnmp.Handler
	packet *gosnmp.SnmpPacket
}

func (c responseClient) Get([]string) (*gosnmp.SnmpPacket, error) { return c.packet, nil }
func (c responseClient) MaxOids() int                             { return 10 }
func TestMetadataGETRejectsPacketError(t *testing.T) {
	_, err := getSNMPValues(responseClient{packet: &gosnmp.SnmpPacket{Error: gosnmp.GenErr}}, []string{"1.2.3"}, map[string]bool{}, &ddsnmp.CollectionStats{})
	require.Error(t, err)
}
