// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"net"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func sendInformResponse(conn *net.UDPConn, peer *net.UDPAddr, pkt *gosnmp.SnmpPacket, engineBoots *engineBoots, localEngineID []byte) error {
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
				v, engineTime := engineBoots.snapshot()
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

func buildSnmpV3SecurityTable(users []USMUser, dynamic bool) (*gosnmp.SnmpV3SecurityParametersTable, error) {
	if len(users) == 0 {
		return nil, nil
	}
	tbl := gosnmp.NewSnmpV3SecurityParametersTable(trapDecodeLogger)

	for _, u := range users {
		var engineID []byte
		if u.EngineID != "" {
			var err error
			engineID, err = parseEngineIDHex(u.EngineID)
			if err != nil {
				return nil, err
			}
		} else if !dynamic {
			return nil, errors.New("engine_id is required for static v3 jobs")
		} else {
			continue
		}

		if err := tbl.Add(u.Username, newUSMSecurityParameters(u, engineID)); err != nil {
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

func newUSMSecurityParameters(user USMUser, engineID []byte) *gosnmp.UsmSecurityParameters {
	return &gosnmp.UsmSecurityParameters{
		UserName:                 user.Username,
		AuthenticationProtocol:   snmpV3AuthProto(strings.ToLower(user.AuthProto)),
		AuthenticationPassphrase: user.AuthKey,
		PrivacyProtocol:          snmpV3PrivProto(strings.ToLower(user.PrivProto)),
		PrivacyPassphrase:        user.PrivKey,
		AuthoritativeEngineID:    string(engineID),
	}
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

func registerUSMUsersWithLocalEngineID(tbl *gosnmp.SnmpV3SecurityParametersTable, users []USMUser, localEngineID []byte) error {
	if tbl == nil || len(localEngineID) == 0 {
		return nil
	}
	for _, u := range users {
		if err := tbl.Add(u.Username, newUSMSecurityParameters(u, localEngineID)); err != nil {
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

func sendDiscoveryReport(conn *net.UDPConn, peer *net.UDPAddr, engineBoots *engineBoots, localEngineID []byte, msgID uint32) error {
	if conn == nil || peer == nil {
		return nil
	}
	if len(localEngineID) == 0 {
		return nil
	}

	boots, engineTime := int64(0), uint32(0)
	if engineBoots != nil {
		boots, engineTime = engineBoots.snapshot()
	}

	reportPkt := gosnmp.SnmpPacket{
		Version:       gosnmp.Version3,
		MsgFlags:      gosnmp.NoAuthNoPriv,
		SecurityModel: gosnmp.UserSecurityModel,
		PDUType:       gosnmp.Report,
		MsgID:         msgID,
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  ".1.3.6.1.6.3.15.1.1.4.0",
				Type:  gosnmp.Counter32,
				Value: uint32(0),
			},
		},
	}

	reportPkt.SecurityParameters = &gosnmp.UsmSecurityParameters{
		AuthoritativeEngineID: string(localEngineID),
	}
	if boots > 0 && boots <= maxSnmpEngineBoots {
		if usp, ok := reportPkt.SecurityParameters.(*gosnmp.UsmSecurityParameters); ok {
			usp.AuthoritativeEngineBoots = uint32(boots)
			usp.AuthoritativeEngineTime = engineTime
		}
	}

	data, err := reportPkt.MarshalMsg()
	if err != nil {
		return err
	}

	_, err = conn.WriteToUDP(data, peer)
	return err
}
