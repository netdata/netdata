// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnode supplies go.d acquisition adapters for configured virtual nodes.
package vnode

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// SNMP acquires identity independently of any metrics job.
type SNMP struct{}

func (SNMP) Acquire(ctx context.Context, config vnodes.SNMPConfig) (*vnodes.Metadata, error) {
	config = config.Defaults()
	client := &gosnmp.GoSNMP{Target: config.Address, Port: uint16(config.Port), Timeout: config.Timeout.Duration(), Retries: *config.Retries, MaxOids: 60, Context: ctx}
	if err := config.Config.Apply(client); err != nil {
		return nil, err
	}
	if err := client.Connect(); err != nil {
		return nil, errors.New("SNMP connection failed")
	}
	conn := client.Conn
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() { stop(); _ = conn.Close() }()
	return acquire(scalarClient{client}, config.Address)
}

type scalarClient struct{ *gosnmp.GoSNMP }

func (c scalarClient) MaxOids() int                { return c.GoSNMP.MaxOids }
func (c scalarClient) Version() gosnmp.SnmpVersion { return c.GoSNMP.Version }

func acquire(client snmputils.ScalarClient, address string) (*vnodes.Metadata, error) {
	si, err := snmputils.GetSysInfo(client)
	if err != nil {
		return nil, errors.New("SNMP system identity acquisition failed")
	}
	si.Name = strings.TrimSpace(si.Name)
	if !(si.Probe.SeenSysObjectID && si.SysObjectID != "" || si.Probe.SeenSysName && si.Name != "" || si.Probe.SeenSysDescr && strings.TrimSpace(si.Descr) != "") {
		return nil, errors.New("SNMP response contains no usable system identity")
	}
	if !si.Probe.SeenSysName {
		si.Name = ""
	}
	base, _ := ddsnmp.AcquireDeviceIdentity(si, nil, ddsnmp.DeviceIdentityOptions{Address: address})
	result := &vnodes.Metadata{Hostname: si.Name, Labels: base.Labels}
	if si.SysObjectID == "" {
		return result, nil
	}
	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{SysObjectID: si.SysObjectID, SysDescr: si.Descr}).Profiles()
	// Acquisition reports safe phase errors; raw returned values stay out of logs.
	meta, err := ddsnmpcollector.CollectDeviceMetadata(client, profiles, si.SysObjectID, logger.NewWithWriter(io.Discard))
	if err != nil {
		return result, errors.New("SNMP profile metadata acquisition failed")
	}
	result.Labels = ddsnmp.ResolveDeviceMetadata(base.Labels, meta, nil)
	return result, nil
}
