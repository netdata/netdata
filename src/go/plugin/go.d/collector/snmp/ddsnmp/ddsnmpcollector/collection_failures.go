// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// CollectionFailures returns the most recent attempt's compact diagnostics.
// Like Collect, it is called by the collector owner, never concurrently with it.
func (c *Collector) CollectionFailures() ddsnmp.CollectionFailures { return c.failures }

type diagnosticClient struct {
	gosnmp.Handler
	failures *ddsnmp.CollectionFailures
}

func (c *diagnosticClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	packet, err := c.Handler.Get(oids)
	c.failures.GET.Record(snmputils.ClassifyGetFailure(packet, err))
	return packet, err
}

func (c *diagnosticClient) WalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	values, err := c.Handler.WalkAll(oid)
	c.recordWalk(err)
	return values, err
}

func (c *diagnosticClient) BulkWalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	values, err := c.Handler.BulkWalkAll(oid)
	c.recordWalk(err)
	return values, err
}

func (c *diagnosticClient) recordWalk(err error) {
	if err != nil {
		failure := snmputils.ClassifyFailure(err)
		failure.Operation = "walk"
		c.failures.WALK.Record(failure)
	}
}

func recordCollectionFailure(dst *ddsnmp.FailureCount, err error, operation, reason string) {
	if err == nil {
		return
	}
	f := snmputils.ClassifyFailure(err)
	f.Operation = operation
	if reason != "" {
		f.Reason = reason
	}
	dst.Record(f)
}
