// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type DeviceInput struct {
	Hostname    string
	SysObjectID string
	SysName     string
	SysDescr    string
	SysContact  string
	SysLocation string
	Vendor      string
	Model       string
	VnodeGUID   string
	VnodeLabels map[string]string
}

func (d DeviceInput) ConnectionInfo() ddsnmp.DeviceConnectionInfo {
	return ddsnmp.DeviceConnectionInfo{
		Hostname:    d.Hostname,
		SysObjectID: d.SysObjectID,
		SysName:     d.SysName,
		SysDescr:    d.SysDescr,
		SysContact:  d.SysContact,
		SysLocation: d.SysLocation,
		Vendor:      d.Vendor,
		Model:       d.Model,
		VnodeGUID:   d.VnodeGUID,
		VnodeLabels: d.VnodeLabels,
	}
}

type CaptureState uint8

const (
	CaptureUnknown CaptureState = iota
	CaptureAvailable
	CaptureUnavailable
)

type CaptureReason uint8

const (
	CaptureReasonNone CaptureReason = iota
	CaptureReasonProjectionError
	CaptureReasonProjectionPanic
)

type AcquisitionProfileValues struct {
	Metadata        map[string]ddsnmp.MetaTag
	Tags            map[string]string
	TopologyMetrics []AcquisitionMetricValue
	BGPRows         []AcquisitionBGPRowValue
	BGPFailed       bool
}

type AcquisitionMetricValue struct {
	RowIndex     string
	Field        string
	RouteOrdinal uint32
	RowOrdinal   uint32
	ValueOrdinal uint32
	Kind         ddsnmp.TopologyKind
	Tags         map[string]string
}

type AcquisitionBGPRowValue struct {
	RouteOrdinal uint32
	RowOrdinal   uint32
	ValueOrdinal uint32

	OriginProfileID string
	Table           string
	RowKey          string
	StructuralID    string
	Kind            ddprofiledefinition.BGPRowKind

	RoutingInstance string
	Neighbor        string
	RemoteAS        string
	LocalAddress    string
	LocalAS         string
	LocalIdentifier string
	PeerIdentifier  string
	PeerType        string
	BGPVersion      string
	Description     string

	AdminHas       bool
	AdminEnabled   bool
	StateHas       bool
	State          ddprofiledefinition.BGPPeerState
	StateRaw       string
	EstablishedHas bool
	Established    int64
	UpdateAgeHas   bool
	UpdateAge      int64
	Tags           map[string]string
}
