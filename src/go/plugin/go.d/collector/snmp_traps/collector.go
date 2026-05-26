// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"runtime"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"golang.org/x/sys/unix"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("snmp_traps", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			Versions: []string{"v1", "v2c"},
		},
		store: store,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	listener     *Listener
	trapWriter   TrapWriter
	journalDir   string
	store        metrix.CollectorStore
	jobName      string
	vnode        string
	profileGen   uint64
	profileIndex *ProfileIndex
	versions     map[SnmpVersion]struct{}
	communities  map[string]struct{}
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) SetJobName(name string) {
	c.jobName = name
}

func (c *Collector) Init(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return &dyncfgCodedError{err: errors.New("SNMP trap journal backend requires Linux"), code: 422}
	}

	if err := validateJobName(c.jobName); err != nil {
		return &dyncfgCodedError{err: err, code: 422}
	}

	if err := validateEndpoints(c.Listen.Endpoints); err != nil {
		return &dyncfgCodedError{err: err, code: 422}
	}

	versions, err := validateVersions(c.Versions)
	if err != nil {
		return &dyncfgCodedError{err: err, code: 422}
	}

	retCfg, err := parseRetentionConfig(c.Retention)
	if err != nil {
		return &dyncfgCodedError{err: err, code: 422}
	}

	// The go.d framework calls Init sequentially per job; this guard keeps
	// repeated Init calls idempotent in tests and defensive call paths.
	if c.listener != nil {
		return nil
	}

	idx, gen, err := AcquireProfileCache()
	if err != nil {
		return &dyncfgCodedError{err: err, code: 422}
	}

	dir := journalRoot(c.jobName)
	journalCfg := retCfg.makeJournalConfig()
	journalWriter, err := NewJournalWriter(dir, journalCfg)
	if err != nil {
		ReleaseProfileCache(gen)
		return &dyncfgCodedError{err: err, code: 422}
	}

	listener, err := newListener(c.jobName, c.Listen.Endpoints)
	if err != nil {
		ReleaseProfileCache(gen)
		journalWriter.Close()
		return &dyncfgCodedError{err: err, code: 422}
	}

	tw := newJournalTrapWriter(journalWriter, defaultQueueCapacity)

	c.profileIndex = idx
	c.profileGen = gen
	c.Versions = versions
	c.vnode = c.Vnode
	c.versions = versionSet(versions)
	c.communities = communitySet(c.Communities)
	c.listener = listener
	c.trapWriter = tw
	c.journalDir = journalWriter.JournalDirectory()

	c.listener.start(c.handlePacket)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.listener != nil {
		c.listener.close()
		c.listener = nil
	}
	if c.trapWriter != nil {
		c.trapWriter.Close()
		c.trapWriter = nil
	}
	if c.profileIndex != nil {
		ReleaseProfileCache(c.profileGen)
		c.profileIndex = nil
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return "" }

func (c *Collector) collect(ctx context.Context) error {
	if c.listener == nil {
		return errors.New("listener not started")
	}
	return nil
}

func (c *Collector) handlePacket(data []byte, peerIP net.IP) {
	pdu, err := DecodeTrap(data, peerIP)
	if err != nil {
		return
	}
	if !c.versionAllowed(pdu.Version) || !c.communityAllowed(pdu.Community) {
		return
	}

	td := c.profileIndex.Lookup(pdu.OID)
	entry := trapEntryFromPDU(c.jobName, c.vnode, pdu, td, time.Now().UnixMicro(), monotonicUsec())
	if err := c.trapWriter.Write(entry); err != nil {
		return
	}
}

func (c *Collector) versionAllowed(version SnmpVersion) bool {
	if len(c.versions) == 0 {
		return true
	}
	_, ok := c.versions[version]
	return ok
}

func (c *Collector) communityAllowed(community string) bool {
	if len(c.communities) == 0 {
		return true
	}
	_, ok := c.communities[community]
	return ok
}

func versionSet(versions []string) map[SnmpVersion]struct{} {
	set := make(map[SnmpVersion]struct{}, len(versions))
	for _, version := range versions {
		set[SnmpVersion(version)] = struct{}{}
	}
	return set
}

func communitySet(communities []string) map[string]struct{} {
	set := make(map[string]struct{}, len(communities))
	for _, community := range communities {
		set[community] = struct{}{}
	}
	return set
}

func monotonicUsec() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return time.Now().UnixMicro()
	}
	return ts.Sec*1_000_000 + ts.Nsec/1_000
}

// dyncfgCodedError implements the CodedError contract for DynCfg surfacing.
type dyncfgCodedError struct {
	err  error
	code int
}

func (e *dyncfgCodedError) Error() string { return e.err.Error() }
func (e *dyncfgCodedError) Unwrap() error { return e.err }
func (e *dyncfgCodedError) Code() int     { return e.code }
