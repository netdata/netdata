// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// usmUserLevel is one configured USM user, reduced to what the security-level check needs.
type usmUserLevel struct {
	username string
	// engineID is the raw configured engine ID; empty for a dynamic-discovery user, which matches any engine.
	engineID string
	// unauthenticated is true when the user has no authentication protocol configured.
	unauthenticated bool
}

func newUSMUserLevels(users []USMUser) ([]usmUserLevel, error) {
	levels := make([]usmUserLevel, 0, len(users))
	for _, u := range users {
		var engineID []byte
		if u.EngineID != "" {
			var err error
			if engineID, err = parseEngineIDHex(u.EngineID); err != nil {
				return nil, err
			}
		}
		level := snmputils.SNMPv3SecurityLevel(
			snmpV3AuthProto(strings.ToLower(u.AuthProto)),
			snmpV3PrivProto(strings.ToLower(u.PrivProto)),
		)
		levels = append(levels, usmUserLevel{
			username:        u.Username,
			engineID:        string(engineID),
			unauthenticated: level == gosnmp.NoAuthNoPriv,
		})
	}
	return levels, nil
}

// securityLevelAllowed reports whether a decoded SNMPv3 message meets the security level of its user.
//
// The gosnmp table decode authenticates only what the message's own flags request, so a message that skips
// authentication decodes successfully even when it names a user configured with an authentication protocol.
// An authenticated message has passed digest verification during decode. An unauthenticated one is allowed
// only when a configured entry for its user and engine ID is itself unauthenticated. All users are also
// registered under the local engine ID for INFORMs, so any entry of the user matches that engine. Privacy
// is not required.
func (r *Receiver) securityLevelAllowed(pkt *gosnmp.SnmpPacket) bool {
	if pkt.MsgFlags&gosnmp.AuthNoPriv != 0 {
		return true
	}
	usm, ok := pkt.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return false
	}
	localEngine := r.localEngineID != nil && r.localEngineID.equalRaw(usm.AuthoritativeEngineID)
	for _, u := range r.usmUserLevels {
		if !u.unauthenticated || u.username != usm.UserName {
			continue
		}
		if localEngine || u.engineID == "" || u.engineID == usm.AuthoritativeEngineID {
			return true
		}
	}
	return false
}
