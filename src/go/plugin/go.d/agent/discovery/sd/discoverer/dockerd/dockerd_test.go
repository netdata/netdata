// SPDX-License-Identifier: GPL-3.0-or-later

package dockerd

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/docker/docker/api/types"
	typesNetwork "github.com/docker/docker/api/types/network"
)

func TestDiscoverer_Discover(t *testing.T) {
	tests := map[string]struct {
		createSim func() *discoverySim
	}{
		"add containers": {
			createSim: func() *discoverySim {
				nginx1 := prepareNginxContainer("nginx1")
				nginx2 := prepareNginxContainer("nginx2")

				sim := &discoverySim{
					dockerCli: func(cli dockerCli, _ time.Duration) {
						cli.addContainer(nginx1)
						cli.addContainer(nginx2)
					},
					wantGroups: []model.TargetGroup{
						&targetGroup{
							source: cntrSource(nginx1),
							targets: []model.Target{
								withHash(&target{
									ID:            nginx1.ID,
									Name:          nginx1.Names[0][1:],
									Image:         nginx1.Image,
									Command:       nginx1.Command,
									Labels:        mapAny(nginx1.Labels),
									PrivatePort:   "80",
									PublicPort:    "8080",
									PublicPortIP:  "0.0.0.0",
									PortProtocol:  "tcp",
									NetworkMode:   "default",
									NetworkDriver: "bridge",
									IPAddress:     "192.0.2.0",
									Address:       "192.0.2.0:80",
								}),
							},
						},
						&targetGroup{
							source: cntrSource(nginx2),
							targets: []model.Target{
								withHash(&target{
									ID:            nginx2.ID,
									Name:          nginx2.Names[0][1:],
									Image:         nginx2.Image,
									Command:       nginx2.Command,
									Labels:        mapAny(nginx2.Labels),
									PrivatePort:   "80",
									PublicPort:    "8080",
									PublicPortIP:  "0.0.0.0",
									PortProtocol:  "tcp",
									NetworkMode:   "default",
									NetworkDriver: "bridge",
									IPAddress:     "192.0.2.0",
									Address:       "192.0.2.0:80",
								}),
							},
						},
					},
				}
				return sim
			},
		},
		"remove containers": {
			createSim: func() *discoverySim {
				nginx1 := prepareNginxContainer("nginx1")
				nginx2 := prepareNginxContainer("nginx2")

				sim := &discoverySim{
					dockerCli: func(cli dockerCli, interval time.Duration) {
						cli.addContainer(nginx1)
						cli.addContainer(nginx2)
						time.Sleep(interval * 2)
						cli.removeContainer(nginx1.ID)
					},
					wantGroups: []model.TargetGroup{
						&targetGroup{
							source:  cntrSource(nginx1),
							targets: nil,
						},
						&targetGroup{
							source: cntrSource(nginx2),
							targets: []model.Target{
								withHash(&target{
									ID:            nginx2.ID,
									Name:          nginx2.Names[0][1:],
									Image:         nginx2.Image,
									Command:       nginx2.Command,
									Labels:        mapAny(nginx2.Labels),
									PrivatePort:   "80",
									PublicPort:    "8080",
									PublicPortIP:  "0.0.0.0",
									PortProtocol:  "tcp",
									NetworkMode:   "default",
									NetworkDriver: "bridge",
									IPAddress:     "192.0.2.0",
									Address:       "192.0.2.0:80",
								}),
							},
						},
					},
				}
				return sim
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sim := test.createSim()
			sim.run(t)
		})
	}
}

func prepareNginxContainer(name string) types.Container {
	return types.Container{
		ID:      "id-" + name,
		Names:   []string{"/" + name},
		Image:   "nginx-image",
		ImageID: "nginx-image-id",
		Command: "nginx-command",
		Ports: []types.Port{
			{
				IP:          "0.0.0.0",
				PrivatePort: 80,
				PublicPort:  8080,
				Type:        "tcp",
			},
		},
		Labels: map[string]string{"key1": "value1"},
		HostConfig: struct {
			NetworkMode string            `json:",omitempty"`
			Annotations map[string]string `json:",omitempty"`
		}{
			NetworkMode: "default",
		},
		NetworkSettings: &types.SummaryNetworkSettings{
			Networks: map[string]*typesNetwork.EndpointSettings{
				"bridge": {IPAddress: "192.0.2.0"},
			},
		},
	}
}

func withHash(tgt *target) *target {
	tgt.hash, _ = calcHash(tgt)
	tags, _ := model.ParseTags("docker")
	tgt.Tags().Merge(tags)
	return tgt
}
