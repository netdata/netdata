// SPDX-License-Identifier: GPL-3.0-or-later

// Package vnode supplies go.d acquisition adapters for configured virtual nodes.
package vnode

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/netdata/gosnmp"
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
	clientConfig := gosnmp.Config{
		Target:  config.Address,
		Port:    uint16(config.Port),
		Timeout: config.Timeout.Duration(),
		Retries: *config.Retries,
		MaxOids: gosnmp.MaxOids,
	}
	if err := config.Apply(&clientConfig); err != nil {
		return nil, err
	}
	// Its errors name the configuration field, never a value.
	client, err := gosnmp.NewClient(clientConfig)
	if err != nil {
		return nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, errors.New("SNMP connection failed")
	}
	defer func() { _ = client.Close() }()
	return acquire(ctx, client, clientConfig.Version, clientConfig.MaxOids, config.Address)
}

// acquire reads the identity through a client configured with version and maxOIDs.
func acquire(ctx context.Context, client snmputils.ScalarClient, version gosnmp.Version, maxOIDs int, address string) (*vnodes.Metadata, error) {
	si, err := snmputils.GetSysInfo(ctx, client, version, maxOIDs)
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
	base, _ := ddsnmp.AcquireDeviceIdentity(ctx, si, nil, ddsnmp.DeviceIdentityOptions{Address: address})
	result := &vnodes.Metadata{Hostname: si.Name, Labels: base.Labels}
	if si.SysObjectID == "" {
		return result, nil
	}
	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{SysObjectID: si.SysObjectID, SysDescr: si.Descr}).Profiles()
	// Acquisition reports safe phase errors; raw returned values stay out of logs.
	meta, err := ddsnmpcollector.CollectDeviceMetadata(ctx, client, version, maxOIDs, profiles, si.SysObjectID, logger.NewWithWriter(io.Discard))
	if err != nil {
		return result, errors.New("SNMP profile metadata acquisition failed")
	}
	result.Labels = ddsnmp.ResolveDeviceMetadata(base.Labels, meta, nil)
	return result, nil
}
