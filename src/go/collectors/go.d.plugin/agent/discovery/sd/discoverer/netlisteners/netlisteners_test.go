// SPDX-License-Identifier: GPL-3.0-or-later

package netlisteners

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"
)

func TestDiscoverer_Discover(t *testing.T) {
	tests := map[string]discoverySim{
		"add listeners": {
			listenersCli: func(cli listenersCli, interval, expiry time.Duration) {
				cli.addListener("UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP6|::|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP6|2001:DB8::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP|127.0.0.1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP|0.0.0.0|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP|192.0.2.1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1")
				cli.addListener("TCP6|::|80|/usr/sbin/apache2 -k start")
				cli.addListener("TCP|0.0.0.0|80|/usr/sbin/apache2 -k start")
				time.Sleep(interval * 2)
			},
			wantGroups: []model.TargetGroup{&targetGroup{
				provider: "sd:net_listeners",
				source:   "discoverer=net_listeners,host=localhost",
				targets: []model.Target{
					withHash(&target{
						Protocol:  "TCP",
						IPAddress: "127.0.0.1",
						Port:      "80",
						Address:   "127.0.0.1:80",
						Comm:      "apache2",
						Cmdline:   "/usr/sbin/apache2 -k start",
					}),
					withHash(&target{
						Protocol:  "TCP",
						IPAddress: "127.0.0.1",
						Port:      "8125",
						Address:   "127.0.0.1:8125",
						Comm:      "netdata",
						Cmdline:   "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&target{
						Protocol:  "UDP",
						IPAddress: "127.0.0.1",
						Port:      "53768",
						Address:   "127.0.0.1:53768",
						Comm:      "go.d.plugin",
						Cmdline:   "/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1",
					}),
					withHash(&target{
						Protocol:  "UDP6",
						IPAddress: "::1",
						Port:      "8125",
						Address:   "[::1]:8125",
						Comm:      "netdata",
						Cmdline:   "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
				},
			}},
		},
		"remove listeners; not expired": {
			listenersCli: func(cli listenersCli, interval, expiry time.Duration) {
				cli.addListener("UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP|127.0.0.1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1")
				time.Sleep(interval * 2)
				cli.removeListener("UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.removeListener("UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1")
				time.Sleep(interval * 2)
			},
			wantGroups: []model.TargetGroup{&targetGroup{
				provider: "sd:net_listeners",
				source:   "discoverer=net_listeners,host=localhost",
				targets: []model.Target{
					withHash(&target{
						Protocol:  "UDP6",
						IPAddress: "::1",
						Port:      "8125",
						Address:   "[::1]:8125",
						Comm:      "netdata",
						Cmdline:   "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&target{
						Protocol:  "TCP",
						IPAddress: "127.0.0.1",
						Port:      "8125",
						Address:   "127.0.0.1:8125",
						Comm:      "netdata",
						Cmdline:   "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
					withHash(&target{
						Protocol:  "UDP",
						IPAddress: "127.0.0.1",
						Port:      "53768",
						Address:   "127.0.0.1:53768",
						Comm:      "go.d.plugin",
						Cmdline:   "/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1",
					}),
				},
			}},
		},
		"remove listeners; expired": {
			listenersCli: func(cli listenersCli, interval, expiry time.Duration) {
				cli.addListener("UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("TCP|127.0.0.1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.addListener("UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1")
				time.Sleep(interval * 2)
				cli.removeListener("UDP6|::1|8125|/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D")
				cli.removeListener("UDP|127.0.0.1|53768|/opt/netdata/usr/libexec/netdata/plugins.d/go.d.plugin 1")
				time.Sleep(expiry * 2)
			},
			wantGroups: []model.TargetGroup{&targetGroup{
				provider: "sd:net_listeners",
				source:   "discoverer=net_listeners,host=localhost",
				targets: []model.Target{
					withHash(&target{
						Protocol:  "TCP",
						IPAddress: "127.0.0.1",
						Port:      "8125",
						Address:   "127.0.0.1:8125",
						Comm:      "netdata",
						Cmdline:   "/opt/netdata/usr/sbin/netdata -P /run/netdata/netdata.pid -D",
					}),
				},
			}},
		},
	}

	for name, sim := range tests {
		t.Run(name, func(t *testing.T) {
			sim.run(t)
		})
	}
}

func withHash(l *target) *target {
	l.hash, _ = calcHash(l)
	tags, _ := model.ParseTags("netlisteners")
	l.Tags().Merge(tags)
	return l
}
