// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/syslog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslogTarget(t *testing.T) {
	for name, tt := range map[string]struct {
		input string
		want  syslog.Config
		err   bool
	}{
		"local":            {input: "netdata", want: syslog.Config{Prefix: "netdata"}},
		"priority":         {input: "daemon.notice/netdata", want: syslog.Config{Prefix: "netdata", Facility: "daemon", Level: "notice"}},
		"remote":           {input: "@logs.example.org/netdata", want: syslog.Config{Prefix: "netdata", Host: "logs.example.org"}},
		"remote port":      {input: "daemon.notice@logs.example.org:514/netdata", want: syslog.Config{Prefix: "netdata", Facility: "daemon", Level: "notice", Host: "logs.example.org", Port: new(field.Integer(514))}},
		"IPv6":             {input: "@[2001:db8::1]/netdata", want: syslog.Config{Prefix: "netdata", Host: "2001:db8::1"}},
		"IPv6 port":        {input: "@[2001:db8::1]:514/netdata", want: syslog.Config{Prefix: "netdata", Host: "2001:db8::1", Port: new(field.Integer(514))}},
		"empty priority":   {input: "/netdata", want: syslog.Config{Prefix: "netdata"}},
		"missing prefix":   {input: "daemon.notice/", err: true},
		"extra separator":  {input: "daemon.notice/net/data", err: true},
		"missing level":    {input: "daemon/netdata", err: true},
		"missing facility": {input: ".notice/netdata", err: true},
		"empty IPv6":       {input: "@[]/netdata", err: true},
		"priority no host": {input: "daemon.notice@[]/netdata", err: true},
		"empty IPv6 port":  {input: "@[]:514/netdata", err: true},
		"empty host port":  {input: "@:514/netdata", err: true},
		"missing host":     {input: "@/netdata", err: true},
		"missing port":     {input: "@host:/netdata", err: true},
		"unbracketed IPv6": {input: "@2001:db8::1/netdata", err: true},
		"invalid port":     {input: "@host:synthetic-private-value/netdata", err: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := syslogTarget(tt.input)
			if tt.err {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
