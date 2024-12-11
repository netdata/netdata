// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logind

import (
	"strings"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.newLogindConn(c.Config)
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	mx := make(map[string]int64)

	// https://www.freedesktop.org/wiki/Software/systemd/logind/ (Session Objects)
	if err := c.collectSessions(mx); err != nil {
		return nil, err
	}
	// https://www.freedesktop.org/wiki/Software/systemd/logind/ (User Objects)
	if err := c.collectUsers(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectSessions(mx map[string]int64) error {
	sessions, err := c.conn.ListSessions()
	if err != nil {
		return err
	}

	mx["sessions_remote"] = 0
	mx["sessions_local"] = 0
	mx["sessions_type_graphical"] = 0
	mx["sessions_type_console"] = 0
	mx["sessions_type_other"] = 0
	mx["sessions_state_online"] = 0
	mx["sessions_state_active"] = 0
	mx["sessions_state_closing"] = 0

	for _, session := range sessions {
		props, err := c.conn.GetSessionProperties(session.Path)
		if err != nil {
			return err
		}

		if v, ok := props["Remote"]; ok && v.String() == "true" {
			mx["sessions_remote"]++
		} else {
			mx["sessions_local"]++
		}

		if v, ok := props["Type"]; ok {
			typ := strings.Trim(v.String(), "\"")
			switch typ {
			case "x11", "mir", "wayland":
				mx["sessions_type_graphical"]++
			case "tty":
				mx["sessions_type_console"]++
			case "unspecified":
				mx["sessions_type_other"]++
			default:
				c.Debugf("unknown session type '%s' for session '%s/%s'", typ, session.User, session.ID)
				mx["sessions_type_other"]++
			}
		}

		if v, ok := props["State"]; ok {
			state := strings.Trim(v.String(), "\"")
			switch state {
			case "online":
				mx["sessions_state_online"]++
			case "active":
				mx["sessions_state_active"]++
			case "closing":
				mx["sessions_state_closing"]++
			default:
				c.Debugf("unknown session state '%s' for session '%s/%s'", state, session.User, session.ID)
			}
		}
	}
	return nil
}

func (c *Collector) collectUsers(mx map[string]int64) error {
	users, err := c.conn.ListUsers()
	if err != nil {
		return err
	}

	// https://www.freedesktop.org/software/systemd/man/sd_uid_get_state.html
	mx["users_state_offline"] = 0
	mx["users_state_lingering"] = 0
	mx["users_state_online"] = 0
	mx["users_state_active"] = 0
	mx["users_state_closing"] = 0

	for _, user := range users {
		v, err := c.conn.GetUserProperty(user.Path, "State")
		if err != nil {
			return err
		}

		state := strings.Trim(v.String(), "\"")
		switch state {
		case "offline":
			mx["users_state_offline"]++
		case "lingering":
			mx["users_state_lingering"]++
		case "online":
			mx["users_state_online"]++
		case "active":
			mx["users_state_active"]++
		case "closing":
			mx["users_state_closing"]++
		default:
			c.Debugf("unknown user state '%s' for user '%s/%d'", state, user.Name, user.UID)
		}
	}
	return nil
}
