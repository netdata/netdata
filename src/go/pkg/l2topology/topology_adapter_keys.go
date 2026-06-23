// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/keyutil"
	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

const keySep = keyutil.Sep

const (
	adjacencyLabelPairID   = model.AdjacencyLabelPairID
	adjacencyLabelPairPass = model.AdjacencyLabelPairPass
)

const (
	lldpMatchPassDefault      = model.LLDPMatchPassDefault
	lldpMatchPassPortDesc     = model.LLDPMatchPassPortDesc
	lldpMatchPassSysName      = model.LLDPMatchPassSysName
	lldpMatchPassChassisPort  = model.LLDPMatchPassChassisPort
	lldpMatchPassChassisDescr = model.LLDPMatchPassChassisDescr
	lldpMatchPassChassis      = model.LLDPMatchPassChassis
	cdpMatchPassDefault       = model.CDPMatchPassDefault
)

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return keyutil.DeviceIfIndexKey(deviceID, ifIndex)
}
