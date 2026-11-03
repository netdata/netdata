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

// v3SecurityAllowed reports whether a decoded SNMPv3 message passed USM and meets the security level of its user.
//
// The gosnmp table decode authenticates only what the message's own flags request, and only for the User-based
// Security Model: a message claiming another model decodes with its auth flags unverified, so it is rejected.
// A USM message with auth flags has passed digest verification during decode.
//
// A configured engine_id does not restrict which engines a user's messages come from: the table is keyed by
// username, keys are localized to the message's engine, and engine authorization belongs to the whitelist or the
// dynamic registry. An unauthenticated message is decided by the most specific entries of its user: entries bound
// to its engine ID; otherwise entries without engine ID, or any entry when it targets the local engine, under which
// all users are registered for INFORMs; otherwise it is allowed only when every entry of its user is no-auth.
// Privacy is not required.
func (r *Receiver) v3SecurityAllowed(pkt *gosnmp.SnmpPacket) bool {
	if pkt.SecurityModel != gosnmp.UserSecurityModel {
		return false
	}
	if pkt.MsgFlags&gosnmp.AuthNoPriv != 0 {
		return true
	}
	usm, ok := pkt.SecurityParameters.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return false
	}
	localEngine := r.localEngineID != nil && r.localEngineID.equalRaw(usm.AuthoritativeEngineID)

	var found, boundToEngine, boundUnauthenticated, generalUnauthenticated bool
	allUnauthenticated := true
	for _, u := range r.usmUserLevels {
		if u.username != usm.UserName {
			continue
		}
		found = true
		allUnauthenticated = allUnauthenticated && u.unauthenticated
		switch {
		case u.engineID != "" && u.engineID == usm.AuthoritativeEngineID:
			boundToEngine = true
			boundUnauthenticated = boundUnauthenticated || u.unauthenticated
		case u.engineID == "" || localEngine:
			generalUnauthenticated = generalUnauthenticated || u.unauthenticated
		}
	}
	if boundToEngine {
		return boundUnauthenticated
	}
	return generalUnauthenticated || (found && allUnauthenticated)
}
