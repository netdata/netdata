// SPDX-License-Identifier: GPL-3.0-or-later

package vnode

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/snmpauth"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, reply func(*gosnmp.SnmpPacket) *gosnmp.SnmpPacket) (vnodes.SNMPConfig, <-chan struct{}) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	seen := make(chan struct{}, 1)
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			decoder := &gosnmp.GoSNMP{}
			p, err := decoder.SnmpDecodePacket(buf[:n])
			if err != nil {
				return
			}
			select {
			case seen <- struct{}{}:
			default:
			}
			if response := reply(p); response != nil {
				response.Version = p.Version
				response.Community = p.Community
				response.PDUType = gosnmp.GetResponse
				response.RequestID = p.RequestID
				raw, err := response.MarshalMsg()
				if err != nil {
					return
				}
				_, _ = conn.WriteTo(raw, addr)
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	_, port, _ := net.SplitHostPort(conn.LocalAddr().String())
	portNum, _ := strconv.Atoi(port)
	retries := 0
	return vnodes.SNMPConfig{Config: snmpauth.Config{Credentials: &snmpauth.Community{Community: "fixture-secret"}}, Address: "127.0.0.1", Port: portNum, Retries: &retries, Timeout: confopt.Duration(time.Second)}, seen
}
func TestAcquireRealUDP(t *testing.T) {
	c, _ := serve(t, func(p *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
		result := &gosnmp.SnmpPacket{}
		for _, request := range p.Variables {
			v := gosnmp.SnmpPDU{Name: request.Name, Type: gosnmp.NoSuchObject}
			if request.Name == "."+snmputils.OidSysDescr {
				v.Type = gosnmp.OctetString
				v.Value = []byte("fixture device")
			}
			result.Variables = append(result.Variables, v)
		}
		return result
	})
	m, err := (SNMP{}).Acquire(context.Background(), c)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Empty(t, m.Hostname, "missing sysName must not become the legacy unknown placeholder")
	require.Equal(t, "fixture device", m.Labels["description"])
}
func TestAcquireRejectsEmptyAndErrorResponses(t *testing.T) {
	for _, status := range []gosnmp.SNMPError{gosnmp.NoError, gosnmp.GenErr} {
		t.Run(status.String(), func(t *testing.T) {
			c, _ := serve(t, func(*gosnmp.SnmpPacket) *gosnmp.SnmpPacket { return &gosnmp.SnmpPacket{Error: status} })
			m, err := (SNMP{}).Acquire(context.Background(), c)
			require.Nil(t, m)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "fixture-secret")
		})
	}
}
func TestCancellationClosesBlockedUDPRead(t *testing.T) {
	c, seen := serve(t, func(*gosnmp.SnmpPacket) *gosnmp.SnmpPacket { return nil })
	c.Timeout = confopt.Duration(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = (SNMP{}).Acquire(ctx, c) }()
	select {
	case <-seen:
	case <-time.After(3 * time.Second):
		t.Fatal("no SNMP request")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt network read")
	}
}
