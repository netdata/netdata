// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/types"
)

type Client interface {
	Version() string
	PerformanceMetrics([]types.PerfQuerySpec) ([]performance.EntityMetric, error)
}

func New(client Client) *Scraper {
	v := &Scraper{Client: client}
	v.calcMaxQuery()
	return v
}

type Scraper struct {
	*logger.Logger
	Client
	maxQuery int
}

// Default settings for vCenter 6.5 and above is 256, prior versions of vCenter have this set to 64.
func (s *Scraper) calcMaxQuery() {
	major, minor, err := parseVersion(s.Version())
	if err != nil || major < 6 || minor == 0 {
		s.maxQuery = 64
		return
	}
	s.maxQuery = 256
}

func (s *Scraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	t := time.Now()
	pqs := newHostsPerfQuerySpecs(hosts)
	ms := s.scrapeMetrics(pqs)
	s.Debugf("scraping : scraped metrics for %d/%d hosts, process took %s",
		len(ms),
		len(hosts),
		time.Since(t),
	)
	return ms
}

func (s *Scraper) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	t := time.Now()
	pqs := newVMsPerfQuerySpecs(vms)
	ms := s.scrapeMetrics(pqs)
	s.Debugf("scraping : scraped metrics for %d/%d vms, process took %s",
		len(ms),
		len(vms),
		time.Since(t),
	)
	return ms
}

func (s *Scraper) scrapeMetrics(pqs []types.PerfQuerySpec) []performance.EntityMetric {
	tc := newThrottledCaller(5)
	var ms []performance.EntityMetric
	lock := &sync.Mutex{}

	chunks := chunkify(pqs, s.maxQuery)
	for _, chunk := range chunks {
		pqs := chunk
		job := func() {
			s.scrape(&ms, lock, pqs)
		}
		tc.call(job)
	}
	tc.wait()

	return ms
}

func (s *Scraper) scrape(metrics *[]performance.EntityMetric, lock *sync.Mutex, pqs []types.PerfQuerySpec) {
	m, err := s.PerformanceMetrics(pqs)
	if err != nil {
		s.Error(err)
		return
	}

	lock.Lock()
	*metrics = append(*metrics, m...)
	lock.Unlock()
}

func chunkify(pqs []types.PerfQuerySpec, chunkSize int) (chunks [][]types.PerfQuerySpec) {
	for i := 0; i < len(pqs); i += chunkSize {
		end := i + chunkSize
		if end > len(pqs) {
			end = len(pqs)
		}
		chunks = append(chunks, pqs[i:end])
	}
	return chunks
}

const (
	pqsMaxSample  = 1
	pqsIntervalID = 20
	pqsFormat     = "normal"
)

func newHostsPerfQuerySpecs(hosts rs.Hosts) []types.PerfQuerySpec {
	pqs := make([]types.PerfQuerySpec, 0, len(hosts))
	for _, host := range hosts {
		pq := types.PerfQuerySpec{
			Entity:     host.Ref,
			MaxSample:  pqsMaxSample,
			MetricId:   host.MetricList,
			IntervalId: pqsIntervalID,
			Format:     pqsFormat,
		}
		pqs = append(pqs, pq)
	}
	return pqs
}

func newVMsPerfQuerySpecs(vms rs.VMs) []types.PerfQuerySpec {
	pqs := make([]types.PerfQuerySpec, 0, len(vms))
	for _, vm := range vms {
		pq := types.PerfQuerySpec{
			Entity:     vm.Ref,
			MaxSample:  pqsMaxSample,
			MetricId:   vm.MetricList,
			IntervalId: pqsIntervalID,
			Format:     pqsFormat,
		}
		pqs = append(pqs, pq)
	}
	return pqs
}

func parseVersion(version string) (major, minor int, err error) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("unparsable version string : %s", version)
	}
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, err
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, err
	}
	return major, minor, nil
}
