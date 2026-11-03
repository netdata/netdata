// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// CollectionFailures returns the most recent attempt's compact diagnostics.
// Like Collect, it is called by the collector owner, never concurrently with it.
func (c *Collector) CollectionFailures() ddsnmp.CollectionFailures { return c.failures }

// Cached metadata and profiles without recurring work do not establish that a
// device responded in this poll. Keep response evidence separate from validity:
// an error response or an incomplete walk still came from the device.
type snmpRequestState struct {
	responded bool
	err       error
}

func (s *snmpRequestState) observe(responded bool, err error) {
	if s == nil {
		return
	}
	s.responded = s.responded || responded
	if !responded && err == nil {
		err = errors.New("SNMP GET returned nil response")
	}
	if s.err == nil {
		s.err = err
	}
}

type diagnosticClient struct {
	gosnmp.Handler
	failures *ddsnmp.CollectionFailures
	negative *negativeEvidence
	requests *snmpRequestState
}

func (c *diagnosticClient) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	packet, err := c.Handler.Get(oids)
	c.requests.observe(packet != nil, err)
	c.failures.GET.Record(snmputils.ClassifyGetFailure(packet, err))
	return packet, err
}

func (c *diagnosticClient) WalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	values, err := c.Handler.WalkAll(oid)
	c.requests.observe(err == nil || len(values) > 0, err)
	c.recordWalk(err)
	return values, err
}

func (c *diagnosticClient) BulkWalkAll(oid string) ([]gosnmp.SnmpPDU, error) {
	values, err := c.Handler.BulkWalkAll(oid)
	c.requests.observe(err == nil || len(values) > 0, err)
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
	if reason != "" && f.Reason == "unknown" {
		f.Reason = reason
	}
	dst.Record(f)
}

func (c *diagnosticClient) SourceRecorder() *SourceRecorder { return sourceRecorder(c.Handler) }
