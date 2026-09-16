// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

type Discoverer struct {
	*logger.Logger
	model.Base

	source     string
	config     parsedConfig
	hash       string
	statusPath string
	status     *discoveryStatus

	probe func(context.Context, endpointCandidate) error
	now   func() time.Time
}

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	parsed, err := cfg.validateAndParse()
	if err != nil {
		return nil, err
	}
	hash, err := discoveryConfigHash(cfg.Source, parsed)
	if err != nil {
		return nil, err
	}
	return &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", "redfish"),
		),
		source: cfg.Source, config: parsed, hash: hash, statusPath: statusFileName(hash),
		status: newDiscoveryStatus(hash),
		probe:  probeCandidate, now: time.Now,
	}, nil
}

func (d *Discoverer) String() string { return "sd:redfish" }

func (d *Discoverer) Discover(ctx context.Context, output chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer d.Info("instance is stopped")

	filename := d.statusPath
	status, err := loadStatus(filename, d.hash)
	if err != nil {
		d.Warningf("failed to load Redfish discovery status: %v", err)
	}
	d.status = status
	d.scan(ctx, output)
	if d.config.scanOnce || ctx.Err() != nil {
		if err := saveStatus(filename, d.status); err != nil {
			d.Warningf("failed to save Redfish discovery status: %v", err)
		}
		return
	}
	ticker := time.NewTicker(d.config.rescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := saveStatus(filename, d.status); err != nil {
				d.Warningf("failed to save Redfish discovery status: %v", err)
			}
			return
		case <-ticker.C:
			d.scan(ctx, output)
		}
	}
}

func (d *Discoverer) scan(ctx context.Context, output chan<- []model.TargetGroup) {
	now := d.now()
	authorized := make(map[string]struct{}, len(d.status.Endpoints))
	workerCount := 0
	completed := d.visitEffectiveCandidates(func(candidate endpointCandidate) bool {
		if ctx.Err() != nil {
			return false
		}
		if workerCount < d.config.maxConcurrentScans {
			workerCount++
		}
		if _, tracked := d.status.Endpoints[candidate.url]; tracked {
			authorized[candidate.url] = struct{}{}
		}
		return true
	})
	if !completed || ctx.Err() != nil {
		return
	}
	for origin := range d.status.Endpoints {
		if _, ok := authorized[origin]; !ok {
			delete(d.status.Endpoints, origin)
		}
	}

	group := &targetGroup{
		provider: "sd:redfishdiscoverer",
		source:   "discoverer=redfish",
	}
	if d.source != "" {
		group.source += "," + d.source
	}
	jobs := make(chan endpointCandidate)
	var wait sync.WaitGroup
	var statusMu sync.Mutex
	for range workerCount {
		wait.Go(func() {
			for candidate := range jobs {
				err := d.probe(ctx, candidate)
				if ctx.Err() != nil || errors.Is(err, context.Canceled) {
					continue
				}
				statusMu.Lock()
				_, existed := d.status.Endpoints[candidate.url]
				if err != nil {
					delete(d.status.Endpoints, candidate.url)
				} else {
					d.status.Endpoints[candidate.url] = statusEntry{ValidatedAt: now}
				}
				statusMu.Unlock()
				if err != nil {
					if existed {
						d.Warningf("lost previously discovered Redfish endpoint %s: %v", candidate.url, err)
					} else {
						d.Debugf("candidate %s did not identify as Redfish: %v", candidate.url, err)
					}
					continue
				}
				group.add(newTarget(candidate))
				d.Infof("successfully discovered Redfish endpoint %s", candidate.url)
			}
		})
	}

	d.visitEffectiveCandidates(func(candidate endpointCandidate) bool {
		if ctx.Err() != nil {
			return false
		}
		statusMu.Lock()
		entry, cached := d.status.Endpoints[candidate.url]
		statusMu.Unlock()
		valid := cached && (d.config.deviceCacheTTL == 0 ||
			now.Before(entry.ValidatedAt.Add(d.config.deviceCacheTTL)))
		if valid {
			group.add(newTarget(candidate))
			return true
		}
		select {
		case jobs <- candidate:
			return true
		case <-ctx.Done():
			return false
		}
	})
	close(jobs)
	wait.Wait()
	group.sort()
	if err := saveStatus(d.statusPath, d.status); err != nil {
		d.Warningf("failed to save Redfish discovery status: %v", err)
	}
	if ctx.Err() != nil {
		return
	}
	select {
	case <-ctx.Done():
	case output <- []model.TargetGroup{group}:
	}
}

func (d *Discoverer) effectiveCandidates() []endpointCandidate {
	var result []endpointCandidate
	d.visitEffectiveCandidates(func(candidate endpointCandidate) bool {
		result = append(result, candidate)
		return true
	})
	sort.Slice(result, func(i, j int) bool { return result[i].url < result[j].url })
	return result
}

func (d *Discoverer) visitEffectiveCandidates(yield func(endpointCandidate) bool) bool {
	claimed := make(map[netip.Addr]struct{})
	for _, network := range d.config.networks {
		for _, address := range network.addresses {
			if _, ok := claimed[address]; ok {
				continue
			}
			claimed[address] = struct{}{}
			for _, port := range network.ports {
				candidate, err := makeCandidate(address, port, network.profile)
				if err == nil && !yield(candidate) {
					return false
				}
			}
		}
	}
	return true
}

func discoveryConfigHash(source string, config parsedConfig) (string, error) {
	type safeProfile struct {
		Name          string `json:"name"`
		Scheme        string `json:"scheme"`
		Timeout       any    `json:"timeout,omitempty"`
		TLSCA         any    `json:"tls_ca,omitempty"`
		TLSSkipVerify any    `json:"tls_skip_verify,omitempty"`
		ProxyURL      any    `json:"proxy_url,omitempty"`
	}
	type safeNetwork struct {
		Prefix  string `json:"prefix"`
		Ports   []int  `json:"ports"`
		Profile string `json:"profile"`
	}
	value := struct {
		Source   string        `json:"source"`
		Profiles []safeProfile `json:"profiles"`
		Networks []safeNetwork `json:"networks"`
	}{Source: source}
	names := make([]string, 0, len(config.profiles))
	for name := range config.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := config.profiles[name]
		value.Profiles = append(value.Profiles, safeProfile{
			Name: profile.Name, Scheme: profile.Scheme,
			Timeout:       profile.JobConfig["timeout"],
			TLSCA:         profile.JobConfig["tls_ca"],
			TLSSkipVerify: profile.JobConfig["tls_skip_verify"],
			ProxyURL:      cacheSafeProxyURL(profile.ProbeConfig["proxy_url"]),
		})
	}
	for _, network := range config.networks {
		value.Networks = append(value.Networks, safeNetwork{
			Prefix: network.normalized, Ports: network.ports, Profile: network.profile.Name,
		})
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Redfish discovery cache identity: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func cacheSafeProxyURL(value any) any {
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "${") {
		return "secret-reference-configured"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "explicit-proxy-configured"
	}
	parsed.User = nil
	return parsed.String()
}
