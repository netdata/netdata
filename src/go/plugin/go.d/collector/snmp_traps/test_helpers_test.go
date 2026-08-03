// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net/netip"
	"sync"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment/netdataadapter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/traptest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const testEngineIDHex = "80001f888077dfe44faa700258"

type v3SecuredTrapSpec struct {
	user        string
	engineIDHex string
	authProto   string
	privProto   string
	authKey     string
	privKey     string
	trapOID     string
	extra       []gosnmp.SnmpPDU
}

func (s v3SecuredTrapSpec) traptest() traptest.V3Spec {
	return traptest.V3Spec{
		User: s.user, EngineIDHex: s.engineIDHex, AuthProto: s.authProto, PrivProto: s.privProto,
		AuthKey: s.authKey, PrivKey: s.privKey, TrapOID: s.trapOID, Extra: s.extra,
	}
}

func buildV2cTrap(t testing.TB, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	return traptest.BuildV2cTrap(t, community, trapOID, extra...)
}

func buildV2cPDU(t testing.TB, pduType gosnmp.PDUType, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	return traptest.BuildV2cPDU(t, pduType, community, trapOID, extra...)
}

func buildV3TrapWithEngineID(t testing.TB, user, engineIDHex, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	return traptest.BuildV3TrapWithEngineID(t, user, engineIDHex, trapOID, extra...)
}

func buildV3Trap(t testing.TB, user, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	return traptest.BuildV3Trap(t, user, trapOID, extra...)
}

func buildV3SecuredTrap(t testing.TB, spec v3SecuredTrapSpec) []byte {
	return traptest.BuildV3SecuredTrap(t, spec.traptest())
}

func buildV3SecuredTrapWithFlags(t testing.TB, spec v3SecuredTrapSpec, flags gosnmp.SnmpV3MsgFlags) []byte {
	return traptest.BuildV3SecuredTrapWithFlags(t, spec.traptest(), flags)
}

func readSinglePcapUDPPacket(t *testing.T, fixture string) traptest.UDPPacket {
	t.Helper()

	packets := traptest.ReadPcapUDPPackets(t, fixture)
	if len(packets) != 1 {
		t.Fatalf("expected one packet in %s, got %d", fixture, len(packets))
	}
	return packets[0]
}

func readColdStartUDPPacket(t *testing.T) traptest.UDPPacket {
	t.Helper()
	return readSinglePcapUDPPacket(t, "testdata/v2c_coldstart.pcap.hex")
}

func testColdStartTrap(category, severity, description string) *TrapDef {
	return &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    category,
		Severity:    severity,
		Description: description,
	}
}

func setSingleTestTrap(t *testing.T, trap *TrapDef) {
	t.Helper()
	setTestProfileIndex(t, map[string]*TrapDef{trap.OID: trap})
}

func newTestV2Collector(jobName string, writer output.Writer, prefixes []netip.Prefix, communities []string) *Collector {
	return newTestV2CollectorWithPolicy(jobName, writer, receiver.PolicyConfig{
		Versions:        []string{"v2c"},
		Communities:     communities,
		SourceAllowlist: prefixes,
	})
}

func newTestV2CollectorWithPolicy(jobName string, writer output.Writer, policy receiver.PolicyConfig) *Collector {
	c := &Collector{
		Config:       Config{Name: jobName},
		trapWriter:   writer,
		journalHost:  newTestJournalHostProvider(),
		profileIndex: currentTestProfileIndex,
	}
	c.receiver = newTestReceiver(c, policy)
	return c
}

func newTestReceiver(c *Collector, cfg receiver.PolicyConfig) *receiver.Receiver {
	return receiver.New(receiver.NewPolicy(cfg), func(event receiver.Event) {
		c.handleReceiverEvent(c.trapMetrics(), event)
	})
}

func bindTestReceiver(t *testing.T, c *Collector) *receiver.Receiver {
	t.Helper()
	recv := newTestReceiver(c, receiver.PolicyConfig{
		Listen: receiver.ListenConfig{Endpoints: []receiver.Endpoint{{
			Protocol: "udp",
			Address:  "127.0.0.1",
			Port:     freeUDPPort(t),
		}}},
	})
	if err := recv.Bind(); err != nil {
		t.Fatalf("bind test receiver: %v", err)
	}
	t.Cleanup(recv.Close)
	return recv
}

func newDefaultTestV2Collector(writer output.Writer) *Collector {
	return newTestV2Collector("test", writer, nil, []string{"public"})
}

func newTestSNMPTrapsCollector() *Collector {
	c := New(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle(), newTestReverseDNSResolver())
	if currentTestCatalogManager != nil {
		c.profileCatalog = currentTestCatalogManager
	}
	return c
}

func newTestReverseDNSResolver() *reversedns.Resolver {
	return reversedns.New(reversedns.Config{})
}

type testTrapTopologyEnricher func(ip, trapIfIndex string) *snmptopology.TrapTopologyEnrichment

func (f testTrapTopologyEnricher) EnrichmentForSource(ip, trapIfIndex string) *snmptopology.TrapTopologyEnrichment {
	return f(ip, trapIfIndex)
}

func newTestTrapEnrichmentCollector(topologyEnricher testTrapTopologyEnricher, dns ...enrichment.ReverseDNS) (*Collector, *ddsnmp.DeviceStore) {
	store := ddsnmp.NewDeviceStore()
	var reverseDNS enrichment.ReverseDNS
	if len(dns) > 0 {
		reverseDNS = dns[0]
	}
	return &Collector{enricher: newTestTrapEnricher(store, topologyEnricher, reverseDNS)}, store
}

func newTestTrapEnricher(store *ddsnmp.DeviceStore, topologyEnricher testTrapTopologyEnricher, dns enrichment.ReverseDNS) *enrichment.Enricher {
	var topology enrichment.TopologyLookup
	if topologyEnricher != nil {
		topology = func(sourceIP, trapIfIndex string) enrichment.TopologyResult {
			return netdataadapter.ProjectTopologyResult(topologyEnricher.EnrichmentForSource(sourceIP, trapIfIndex))
		}
	}
	return enrichment.New(
		netdataadapter.RegistryLookup(store),
		topology,
		dns,
	)
}

type testReverseDNS struct {
	mu        sync.Mutex
	results   map[netip.Addr]reversedns.Result
	lookups   []netip.Addr
	schedules []netip.Addr
}

func newTestReverseDNS(results map[string]string) *testReverseDNS {
	dns := &testReverseDNS{results: make(map[netip.Addr]reversedns.Result, len(results))}
	for raw, name := range results {
		addr := netip.MustParseAddr(raw).Unmap()
		state := reversedns.StateNegative
		if name != "" {
			state = reversedns.StatePositive
		}
		dns.results[addr] = reversedns.Result{State: state, Name: name}
	}
	return dns
}

func (d *testReverseDNS) Lookup(addr netip.Addr) reversedns.Result {
	addr = addr.Unmap()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lookups = append(d.lookups, addr)
	if result, ok := d.results[addr]; ok {
		return result
	}
	return reversedns.Result{State: reversedns.StateMiss}
}

func (d *testReverseDNS) Schedule(addr netip.Addr) reversedns.ScheduleState {
	addr = addr.Unmap()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.schedules = append(d.schedules, addr)
	if !addr.IsValid() {
		return reversedns.ScheduleInvalid
	}
	if result, ok := d.results[addr]; ok {
		if result.State == reversedns.StatePositive {
			return reversedns.SchedulePositive
		}
		if result.State == reversedns.StateNegative {
			return reversedns.ScheduleNegative
		}
	}
	return reversedns.ScheduleScheduled
}

func (d *testReverseDNS) callCounts() (lookups, schedules int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.lookups), len(d.schedules)
}

func withCleanJobMetrics(t *testing.T, jobName string) *perJobMetrics {
	t.Helper()
	removeJobMetrics(jobName)
	t.Cleanup(func() { removeJobMetrics(jobName) })
	return getJobMetrics(jobName)
}

func collectJobMetricsForTest(t *testing.T, jobName string) metrix.CollectorStore {
	t.Helper()
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}
	managed.CycleController().BeginCycle()
	collectMetrics(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}
	return store
}

func newDedupTestV2Collector(t *testing.T, jobName string, writer output.Writer) (*Collector, *perJobMetrics) {
	t.Helper()

	metrics := withCleanJobMetrics(t, jobName)
	metrics.setDedupEnabled(true)
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})
	c.Dedup = DedupConfig{Enabled: true}
	c.metrics = metrics
	c.deduper = newTrapDeduper(jobName, c.Dedup, writer, metrics, "", c.monotonicUsec)
	return c, metrics
}

func testNoAuthV3User(engineID string) USMUserConfig {
	return USMUserConfig{
		Username:  "testuser",
		EngineID:  engineID,
		AuthProto: "none",
		PrivProto: "none",
	}
}

func newTestV3Collector(
	t *testing.T,
	jobName string,
	writer output.Writer,
	users []USMUserConfig,
	engineIDs []string,
) *Collector {
	t.Helper()
	c := &Collector{
		Config:      Config{Name: jobName},
		trapWriter:  writer,
		journalHost: newTestJournalHostProvider(),
	}
	recv := newTestReceiver(c, receiver.PolicyConfig{
		Versions:          []string{"v3"},
		USMUsers:          toReceiverUSMUsers(users),
		EngineIDWhitelist: engineIDs,
	})
	if err := recv.PrepareV3(t.TempDir(), jobName); err != nil {
		t.Fatalf("prepare test v3 receiver: %v", err)
	}
	t.Cleanup(recv.RollbackPreparedState)
	c.receiver = recv
	return c
}
