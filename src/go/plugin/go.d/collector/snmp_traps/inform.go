// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func sendInformResponse(conn *net.UDPConn, peer *net.UDPAddr, pkt *gosnmp.SnmpPacket, engineBoots *EngineBoots, localEngineID []byte) error {
	if pkt == nil || conn == nil || peer == nil {
		return nil
	}

	respPkt := *pkt
	respPkt.PDUType = gosnmp.GetResponse
	respPkt.Error = gosnmp.NoError
	respPkt.ErrorIndex = 0
	respPkt.Variables = append([]gosnmp.SnmpPDU(nil), pkt.Variables...)
	if pkt.SecurityParameters != nil {
		respPkt.SecurityParameters = pkt.SecurityParameters.Copy()
		if len(localEngineID) > 0 {
			if usp, ok := respPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
				usp.AuthoritativeEngineID = string(localEngineID)
			}
		}
		if engineBoots != nil {
			if usp, ok := respPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
				// RFC 3414 section 1.5.1 makes the receiver authoritative for
				// confirmed-class messages such as INFORM requests.
				v, engineTime := engineBoots.Snapshot()
				if v > 0 && v <= maxSnmpEngineBoots {
					usp.AuthoritativeEngineBoots = uint32(v)
					usp.AuthoritativeEngineTime = engineTime
				}
			}
		}
	}

	data, err := respPkt.MarshalMsg()
	if err != nil {
		return err
	}

	_, err = conn.WriteToUDP(data, peer)
	return err
}

func buildSnmpV3SecurityTable(users []USMUserConfig) (*gosnmp.SnmpV3SecurityParametersTable, error) {
	if len(users) == 0 {
		return nil, nil
	}

	tbl := gosnmp.NewSnmpV3SecurityParametersTable(trapDecodeLogger)

	for _, u := range users {
		authProto := snmpV3AuthProto(strings.ToLower(u.AuthProto))
		privProto := snmpV3PrivProto(strings.ToLower(u.PrivProto))
		engineID, err := parseEngineIDHex(u.EngineID)
		if err != nil {
			return nil, err
		}

		sp := &gosnmp.UsmSecurityParameters{
			UserName:                 u.Username,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: u.AuthKey,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        u.PrivKey,
			AuthoritativeEngineID:    string(engineID),
		}

		if err := tbl.Add(u.Username, sp); err != nil {
			return nil, err
		}
	}

	return tbl, nil
}

func snmpV3AuthProto(name string) gosnmp.SnmpV3AuthProtocol {
	return snmputils.ParseSNMPv3AuthProtocol(name)
}

func snmpV3PrivProto(name string) gosnmp.SnmpV3PrivProtocol {
	return snmputils.ParseSNMPv3PrivProtocol(name)
}

func buildEngineIDWhitelist(ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	whitelist := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		raw, err := parseEngineIDHex(id)
		if err != nil {
			return nil, err
		}
		whitelist[string(raw)] = struct{}{}
	}
	return whitelist, nil
}

func registerUSMUsersWithLocalEngineID(tbl *gosnmp.SnmpV3SecurityParametersTable, users []USMUserConfig, localEngineID []byte) error {
	if tbl == nil || len(localEngineID) == 0 {
		return nil
	}
	for _, u := range users {
		authProto := snmpV3AuthProto(strings.ToLower(u.AuthProto))
		privProto := snmpV3PrivProto(strings.ToLower(u.PrivProto))
		sp := &gosnmp.UsmSecurityParameters{
			UserName:                 u.Username,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: u.AuthKey,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        u.PrivKey,
			AuthoritativeEngineID:    string(localEngineID),
		}
		if err := tbl.Add(u.Username, sp); err != nil {
			return err
		}
	}
	return nil
}

func engineIDHexAllowed(engineIDHex string, whitelist map[string]struct{}) bool {
	if whitelist == nil {
		return true
	}
	raw, err := parseEngineIDHex(engineIDHex)
	if err != nil {
		return false
	}
	_, ok := whitelist[string(raw)]
	return ok
}

func isEngineIDAllowed(sp gosnmp.SnmpV3SecurityParameters, whitelist map[string]struct{}) bool {
	if sp == nil || whitelist == nil {
		return true
	}
	usp, ok := sp.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return true
	}
	_, ok = whitelist[usp.AuthoritativeEngineID]
	return ok
}
