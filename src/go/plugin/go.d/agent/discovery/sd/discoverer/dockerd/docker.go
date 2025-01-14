// SPDX-License-Identifier: GPL-3.0-or-later

package dockerd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/dockerhost"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	docker "github.com/docker/docker/client"
	"github.com/gohugoio/hashstructure"
)

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	tags, err := model.ParseTags(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("parse tags: %v", err)
	}

	d := &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", "docker"),
		),
		cfgSource: cfg.Source,
		newDockerClient: func(addr string) (dockerClient, error) {
			return docker.NewClientWithOpts(docker.WithHost(addr))
		},
		addr:           docker.DefaultDockerHost,
		listInterval:   time.Second * 60,
		timeout:        time.Second * 2,
		seenTggSources: make(map[string]bool),
		started:        make(chan struct{}),
	}

	if addr := dockerhost.FromEnv(); addr != "" && d.addr == docker.DefaultDockerHost {
		d.Infof("using docker host from environment: %s ", addr)
		d.addr = addr
	}

	d.Tags().Merge(tags)

	if cfg.Timeout.Duration().Seconds() != 0 {
		d.timeout = cfg.Timeout.Duration()
	}
	if cfg.Address != "" {
		d.addr = cfg.Address
	}

	return d, nil
}

type Config struct {
	Source string

	Tags    string           `yaml:"tags"`
	Address string           `yaml:"address"`
	Timeout confopt.Duration `yaml:"timeout"`
}

type (
	Discoverer struct {
		*logger.Logger
		model.Base

		dockerClient    dockerClient
		newDockerClient func(addr string) (dockerClient, error)
		addr            string

		cfgSource string

		listInterval   time.Duration
		timeout        time.Duration
		seenTggSources map[string]bool // [targetGroup.Source]

		started chan struct{}
	}
	dockerClient interface {
		NegotiateAPIVersion(context.Context)
		ContainerList(context.Context, typesContainer.ListOptions) ([]types.Container, error)
		Close() error
	}
)

func (d *Discoverer) String() string {
	return "sd:docker"
}

func (d *Discoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer func() { d.cleanup(); d.Info("instance is stopped") }()

	close(d.started)

	if d.dockerClient == nil {
		client, err := d.newDockerClient(d.addr)
		if err != nil {
			d.Errorf("error on creating docker client: %v", err)
			return
		}
		d.dockerClient = client
	}

	d.dockerClient.NegotiateAPIVersion(ctx)

	if err := d.listContainers(ctx, in); err != nil {
		d.Error(err)
		return
	}

	tk := time.NewTicker(d.listInterval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			if err := d.listContainers(ctx, in); err != nil {
				d.Warning(err)
			}
		}
	}
}

func (d *Discoverer) listContainers(ctx context.Context, in chan<- []model.TargetGroup) error {
	listCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	containers, err := d.dockerClient.ContainerList(listCtx, typesContainer.ListOptions{})
	if err != nil {
		return err
	}

	var tggs []model.TargetGroup
	seen := make(map[string]bool)

	for _, cntr := range containers {
		if tgg := d.buildTargetGroup(cntr); tgg != nil {
			tggs = append(tggs, tgg)
			seen[tgg.Source()] = true
		}
	}

	for src := range d.seenTggSources {
		if !seen[src] {
			tggs = append(tggs, &targetGroup{source: src})
		}
	}
	d.seenTggSources = seen

	select {
	case <-ctx.Done():
	case in <- tggs:
	}

	return nil
}

func (d *Discoverer) buildTargetGroup(cntr types.Container) model.TargetGroup {
	if len(cntr.Names) == 0 || cntr.NetworkSettings == nil || len(cntr.NetworkSettings.Networks) == 0 {
		return nil
	}

	tgg := &targetGroup{
		source: cntrSource(cntr),
	}
	if d.cfgSource != "" {
		tgg.source += fmt.Sprintf(",%s", d.cfgSource)
	}

	for netDriver, network := range cntr.NetworkSettings.Networks {
		// container with network mode host will be discovered by local-listeners
		for _, port := range cntr.Ports {
			tgt := &target{
				ID:            cntr.ID,
				Name:          strings.TrimPrefix(cntr.Names[0], "/"),
				Image:         cntr.Image,
				Command:       cntr.Command,
				Labels:        mapAny(cntr.Labels),
				PrivatePort:   strconv.Itoa(int(port.PrivatePort)),
				PublicPort:    strconv.Itoa(int(port.PublicPort)),
				PublicPortIP:  port.IP,
				PortProtocol:  port.Type,
				NetworkMode:   cntr.HostConfig.NetworkMode,
				NetworkDriver: netDriver,
				IPAddress:     network.IPAddress,
			}
			tgt.Address = net.JoinHostPort(tgt.IPAddress, tgt.PrivatePort)

			hash, err := calcHash(tgt)
			if err != nil {
				continue
			}

			tgt.hash = hash
			tgt.Tags().Merge(d.Tags())

			tgg.targets = append(tgg.targets, tgt)
		}
	}

	return tgg
}

func (d *Discoverer) cleanup() {
	if d.dockerClient != nil {
		_ = d.dockerClient.Close()
	}
}

func cntrSource(cntr types.Container) string {
	name := strings.TrimPrefix(cntr.Names[0], "/")
	return fmt.Sprintf("discoverer=docker,container=%s,image=%s", name, cntr.Image)
}

func calcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}

func mapAny(src map[string]string) map[string]any {
	if src == nil {
		return nil
	}
	m := make(map[string]any, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}
