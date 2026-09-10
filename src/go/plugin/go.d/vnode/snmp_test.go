// SPDX-License-Identifier: GPL-3.0-or-later

package vnode

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
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
				if !errors.Is(err, net.ErrClosed) {
					t.Errorf("SNMP fixture read: %v", err)
				}
				return
			}
			decoder := &gosnmp.GoSNMP{}
			p, err := decoder.SnmpDecodePacket(buf[:n])
			if err != nil {
				t.Errorf("SNMP fixture decode request: %v", err)
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
					t.Errorf("SNMP fixture encode response: %v", err)
					return
				}
				if _, err := conn.WriteTo(raw, addr); err != nil {
					if !errors.Is(err, net.ErrClosed) {
						t.Errorf("SNMP fixture write response: %v", err)
					}
					return
				}
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

func TestAcquireV1RecoversFromIndexedMissingSystemOIDs(t *testing.T) {
	c, _ := serve(t, func(p *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
		for i, v := range p.Variables {
			if v.Name != "."+snmputils.OidSysName {
				return &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName, ErrorIndex: uint8(i + 1), Variables: p.Variables}
			}
		}
		return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: "." + snmputils.OidSysName, Type: gosnmp.OctetString, Value: "v1-router"}}}
	})
	c.Version = "1"
	m, err := (SNMP{}).Acquire(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, "v1-router", m.Hostname)
}

func TestAcquireRealUDPProfileFailureRetainsSystemIdentity(t *testing.T) {
	var enrichmentRequests atomic.Int32
	c, _ := serve(t, func(p *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
		for _, request := range p.Variables {
			if !strings.HasPrefix(request.Name, ".1.3.6.1.2.1.1.") {
				enrichmentRequests.Add(1)
				return &gosnmp.SnmpPacket{Error: gosnmp.GenErr, Variables: p.Variables}
			}
		}
		r := &gosnmp.SnmpPacket{}
		for _, request := range p.Variables {
			v := gosnmp.SnmpPDU{Name: request.Name, Type: gosnmp.NoSuchObject}
			switch request.Name {
			case "." + snmputils.OidSysObject:
				v.Type, v.Value = gosnmp.ObjectIdentifier, ".1.3.6.1.4.1.6574.1"
			case "." + snmputils.OidSysName:
				v.Type, v.Value = gosnmp.OctetString, "storage"
			}
			r.Variables = append(r.Variables, v)
		}
		return r
	})
	m, err := (SNMP{}).Acquire(t.Context(), c)
	require.ErrorContains(t, err, "profile metadata acquisition failed")
	require.Equal(t, "storage", m.Hostname)
	require.NotEmpty(t, m.Labels["sys_object_id"])
	require.Positive(t, enrichmentRequests.Load())
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

func TestAcquireWhitespaceSystemName(t *testing.T) {
	for _, description := range []string{"", "usable description"} {
		t.Run(description, func(t *testing.T) {
			c, _ := serve(t, func(p *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
				result := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: "." + snmputils.OidSysName, Type: gosnmp.OctetString, Value: []byte("   ")}}}
				if description != "" {
					result.Variables = append(result.Variables, gosnmp.SnmpPDU{Name: "." + snmputils.OidSysDescr, Type: gosnmp.OctetString, Value: []byte(description)})
				}
				return result
			})
			m, err := (SNMP{}).Acquire(context.Background(), c)
			if description == "" {
				require.Error(t, err)
				require.Nil(t, m)
			} else {
				require.NoError(t, err)
				require.Empty(t, m.Hostname)
			}
		})
	}
}
