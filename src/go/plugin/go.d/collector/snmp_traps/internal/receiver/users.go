// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"strings"

	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// receiverUsers registers the configured USM users with the engines whose
// messages they may decode, as RFC 3414's user table does: a message is
// decoded only by the users registered for its user name and engine ID, tried
// in order, and the receiver's minimum level makes a user with an
// authentication protocol accept only authenticated messages.
//
// The lists keep, per user name, the pairs a job accepts and its level rule:
//   - an engine with users of the name bound to it (their engine_id): those
//     users first, then the name's other authenticated users; an
//     unauthenticated message is then decided by the bound users. Without bound
//     users, the name's authenticated users; and when the name has none, its
//     unauthenticated users. Static jobs register this for each whitelisted
//     engine, dynamic jobs for each configured engine_id.
//   - the local engine ID (informs): the users bound to it as above, otherwise
//     every user of the name.
//   - dynamic jobs learn, for any other engine, the name's authenticated users,
//     and its unauthenticated users when one of the name without engine_id is
//     unauthenticated or every user of the name is.
//
// A user configured identically for two engines appears once in a list.
func receiverUsers(users []USMUser, whitelist [][]byte, localEngineID []byte, dynamic bool) ([]gosnmp.EngineUser, []gosnmp.User, error) {
	type configured struct {
		user     gosnmp.User
		engineID []byte
		auth     bool
	}
	var (
		names  []string
		byName = make(map[string][]configured)
	)
	for _, u := range users {
		var engineID []byte
		if u.EngineID != "" {
			var err error
			if engineID, err = parseEngineIDHex(u.EngineID); err != nil {
				return nil, nil, err
			}
		}
		user := gosnmp.User{
			Name:           u.Username,
			AuthProtocol:   snmputils.ParseSNMPv3AuthProtocol(strings.ToLower(u.AuthProto)),
			AuthPassphrase: u.AuthKey,
			PrivProtocol:   snmputils.ParseSNMPv3PrivProtocol(strings.ToLower(u.PrivProto)),
			PrivPassphrase: u.PrivKey,
		}
		if _, ok := byName[u.Username]; !ok {
			names = append(names, u.Username)
		}
		byName[u.Username] = append(byName[u.Username], configured{
			user:     user,
			engineID: engineID,
			auth:     user.AuthProtocol != gosnmp.NoAuth,
		})
	}

	appendUnique := func(list []gosnmp.User, user gosnmp.User) []gosnmp.User {
		for _, have := range list {
			if have == user {
				return list
			}
		}
		return append(list, user)
	}
	// engineList is the list of one name for an engine other than the local one.
	engineList := func(name string, engineID []byte) []gosnmp.User {
		var list []gosnmp.User
		for _, c := range byName[name] {
			if string(c.engineID) == string(engineID) {
				list = appendUnique(list, c.user)
			}
		}
		for _, c := range byName[name] {
			if c.auth {
				list = appendUnique(list, c.user)
			}
		}
		if len(list) == 0 {
			for _, c := range byName[name] {
				list = appendUnique(list, c.user)
			}
		}
		return list
	}
	localList := func(name string) []gosnmp.User {
		var list []gosnmp.User
		bound := false
		for _, c := range byName[name] {
			if string(c.engineID) == string(localEngineID) {
				list, bound = appendUnique(list, c.user), true
			}
		}
		for _, c := range byName[name] {
			if !bound || c.auth {
				list = appendUnique(list, c.user)
			}
		}
		return list
	}

	var engines [][]byte
	seenEngine := make(map[string]bool)
	addEngine := func(id []byte) {
		if len(id) > 0 && !seenEngine[string(id)] && string(id) != string(localEngineID) {
			seenEngine[string(id)] = true
			engines = append(engines, id)
		}
	}
	if dynamic {
		for _, name := range names {
			for _, c := range byName[name] {
				addEngine(c.engineID)
			}
		}
	} else {
		for _, id := range whitelist {
			addEngine(id)
		}
	}

	var registered []gosnmp.EngineUser
	for _, engineID := range engines {
		for _, name := range names {
			for _, user := range engineList(name, engineID) {
				registered = append(registered, gosnmp.EngineUser{EngineID: engineID, User: user})
			}
		}
	}
	if len(localEngineID) > 0 {
		for _, name := range names {
			for _, user := range localList(name) {
				registered = append(registered, gosnmp.EngineUser{EngineID: localEngineID, User: user})
			}
		}
	}

	var learn []gosnmp.User
	if dynamic {
		for _, name := range names {
			plain := true
			for _, c := range byName[name] {
				plain = plain && !c.auth
			}
			for _, c := range byName[name] {
				plain = plain || len(c.engineID) == 0 && !c.auth
			}
			for _, c := range byName[name] {
				if c.auth || plain {
					learn = appendUnique(learn, c.user)
				}
			}
		}
	}
	return registered, learn, nil
}

func buildEngineIDWhitelist(ids []string) ([][]byte, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	whitelist := make([][]byte, 0, len(ids))
	for _, id := range ids {
		raw, err := parseEngineIDHex(id)
		if err != nil {
			return nil, err
		}
		whitelist = append(whitelist, raw)
	}
	return whitelist, nil
}
