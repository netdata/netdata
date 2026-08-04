// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gosnmp/gosnmp"
	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment/netdataadapter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profiletest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/traptest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const (
	testEngineIDHex = "80001f888077dfe44faa700258"
)

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

func readColdStartUDPPacket(t *testing.T) traptest.UDPPacket {
	t.Helper()
	packets := traptest.ReadPcapUDPPackets(t, filepath.Join("..", "..", "testdata", "v2c_coldstart.pcap.hex"))
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}
	return packets[0]
}

func testColdStartTrap(category, severity, description string) *catalog.TrapDef {
	return &catalog.TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    category,
		Severity:    severity,
		Description: description,
	}
}

func setSingleTestTrap(t *testing.T, trap *catalog.TrapDef) *catalog.Epoch {
	return setTestProfileIndex(t, map[string]*catalog.TrapDef{trap.OID: trap})
}

func setTestProfileIndex(t *testing.T, traps map[string]*catalog.TrapDef) *catalog.Epoch {
	t.Helper()
	paths := profiletest.CatalogPaths(t)
	if len(traps) > 0 {
		defs := make([]*catalog.TrapDef, 0, len(traps))
		for _, trap := range traps {
			defs = append(defs, trap)
		}
		writeProfileYAML(t, paths.UserDirs[0], "packet-tests.yaml", map[string]any{"traps": defs})
	}
	lease, err := catalog.NewManager(paths).Acquire()
	if err != nil {
		t.Fatalf("build test profile index: %v", err)
	}
	t.Cleanup(lease.Close)
	return lease.Epoch()
}

func newTestV2Job(jobName string, writer output.Writer, prefixes []netip.Prefix, communities []string, indexes ...*catalog.Epoch) *Job {
	return newTestV2JobWithPolicy(jobName, writer, receiver.PolicyConfig{
		Versions:        []string{"v2c"},
		Communities:     communities,
		SourceAllowlist: prefixes,
	}, indexes...)
}

func newTestV2JobWithPolicy(jobName string, writer output.Writer, cfg receiver.PolicyConfig, indexes ...*catalog.Epoch) *Job {
	registry := telemetry.NewRegistry()
	var profileIndex *catalog.Epoch
	if len(indexes) > 0 {
		profileIndex = indexes[0]
	}
	j := newTestJob(
		NewPolicy(PolicyConfig{JobName: jobName, Receiver: receiver.NewPolicy(cfg)}),
		registry,
		newTestTrapEnricher(ddsnmp.NewDeviceStore(), nil, nil),
	)
	j.writer = writer
	j.journalHost = newTestJournalHostProvider()
	j.profileIndex = profileIndex
	j.telemetry = registry.Attach(jobName, telemetry.Options{})
	j.receiver = newTestReceiver(j, cfg)
	return j
}

func newTestJob(policy Policy, registry *telemetry.Registry, trapEnricher *enrichment.Enricher) *Job {
	if registry == nil {
		registry = telemetry.NewRegistry()
	}
	if trapEnricher == nil {
		trapEnricher = enrichment.New(nil, nil, nil)
	}
	provider := newTestJournalHostProvider()
	return New(policy, Dependencies{
		Catalog:         catalog.NewManager(catalog.Paths{}),
		HostIdentity:    staticHostIdentity{provider: provider},
		Enricher:        trapEnricher,
		Telemetry:       registry,
		JournalActivity: testJournalActivity{},
	})
}

type staticHostIdentity struct {
	provider hostidentity.Provider
}

func (s staticHostIdentity) FreshJournal() (hostidentity.Provider, error) {
	return s.provider, nil
}

func (s staticHostIdentity) CachedFallback() (hostidentity.Provider, error) {
	return s.provider, nil
}

type testJournalActivity struct{}

func (testJournalActivity) Acquire() JournalActivityLease { return testJournalActivityLease{} }

type testJournalActivityLease struct{}

func (testJournalActivityLease) Close() {}

func newTestReceiver(j *Job, cfg receiver.PolicyConfig) *receiver.Receiver {
	return receiver.New(receiver.NewPolicy(cfg), func(event receiver.Event) {
		j.handleReceiverEvent(j.telemetry, event)
	})
}

func bindTestReceiver(t *testing.T, j *Job) *receiver.Receiver {
	t.Helper()
	recv := newTestReceiver(j, receiver.PolicyConfig{
		Listen: receiver.ListenConfig{Endpoints: []receiver.Endpoint{{
			Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t),
		}}},
	})
	if _, err := recv.Bind(); err != nil {
		t.Fatalf("bind test receiver: %v", err)
	}
	t.Cleanup(recv.Close)
	return recv
}

func newDefaultTestV2Job(writer output.Writer, indexes ...*catalog.Epoch) *Job {
	return newTestV2Job("test", writer, nil, []string{"public"}, indexes...)
}

func testDatagram(data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr) receiver.Datagram {
	return receiver.Datagram{Data: data, PeerIP: peerIP, Conn: conn, Peer: peer}
}

func newTestJobTelemetry(t testing.TB, jobName string, dedupEnabled bool) *telemetry.Job {
	t.Helper()
	registry := telemetry.NewRegistry()
	job := registry.Attach(jobName, telemetry.Options{DedupEnabled: dedupEnabled})
	t.Cleanup(job.Detach)
	return job
}

func collectJobMetricsForTest(t testing.TB, job *telemetry.Job) metrix.CollectorStore {
	t.Helper()
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("metric store does not expose cycle control")
	}
	managed.CycleController().BeginCycle()
	job.Collect(store)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}
	return store
}

func assertJobMetric(t testing.TB, job *telemetry.Job, jobName, metric string, want float64) {
	t.Helper()
	store := collectJobMetricsForTest(t, job)
	got, ok := store.Read().Value(metric, metrix.Labels{"job_name": jobName})
	if !ok || got != want {
		t.Errorf("%s = %v/%v, want %v/true", metric, got, ok, want)
	}
}

func newDedupTestV2Job(t *testing.T, jobName string, writer output.Writer, indexes ...*catalog.Epoch) (*Job, *telemetry.Job) {
	t.Helper()
	j := newTestV2Job(jobName, writer, nil, []string{"public"}, indexes...)
	registry := telemetry.NewRegistry()
	j.telemetry.Detach()
	j.telemetry = registry.Attach(jobName, telemetry.Options{DedupEnabled: true})
	policy, err := dedup.Normalize(dedup.Config{Enabled: true})
	if err != nil {
		t.Fatalf("normalize test dedup policy: %v", err)
	}
	j.policy.dedup = policy
	j.dedupKeyVarbinds = policy.KeyVarbinds()
	j.deduper = newDeduper(jobName, policy, j.profileIndex, writer, j.telemetry, writeFailureJournal, func() int64 { return 0 })
	return j, j.telemetry
}

func testNoAuthV3User(engineID string) receiver.USMUser {
	return receiver.USMUser{Username: "testuser", EngineID: engineID, AuthProto: "none", PrivProto: "none"}
}

func newTestV3Job(t *testing.T, jobName string, writer output.Writer, users []receiver.USMUser, engineIDs []string) *Job {
	t.Helper()
	cfg := receiver.PolicyConfig{Versions: []string{"v3"}, USMUsers: users, EngineIDWhitelist: engineIDs}
	j := newTestV2JobWithPolicy(jobName, writer, cfg)
	if err := j.receiver.PrepareV3(t.TempDir(), jobName); err != nil {
		t.Fatalf("prepare test v3 receiver: %v", err)
	}
	t.Cleanup(j.receiver.RollbackPreparedState)
	return j
}

type testTrapTopologyEnricher func(ip, trapIfIndex string) *snmptopology.TrapTopologyEnrichment

func (f testTrapTopologyEnricher) EnrichmentForSource(ip, trapIfIndex string) *snmptopology.TrapTopologyEnrichment {
	return f(ip, trapIfIndex)
}

func newTestTrapEnricher(store *ddsnmp.DeviceStore, topologyEnricher testTrapTopologyEnricher, dns enrichment.ReverseDNS) *enrichment.Enricher {
	var topology enrichment.TopologyLookup
	if topologyEnricher != nil {
		topology = func(sourceIP, trapIfIndex string) enrichment.TopologyResult {
			return netdataadapter.ProjectTopologyResult(topologyEnricher.EnrichmentForSource(sourceIP, trapIfIndex))
		}
	}
	return enrichment.New(netdataadapter.RegistryLookup(store), topology, dns)
}

func newTestEnricherWithStore(topologyEnricher testTrapTopologyEnricher, dns ...enrichment.ReverseDNS) (*enrichment.Enricher, *ddsnmp.DeviceStore) {
	store := ddsnmp.NewDeviceStore()
	var reverseDNS enrichment.ReverseDNS
	if len(dns) > 0 {
		reverseDNS = dns[0]
	}
	return newTestTrapEnricher(store, topologyEnricher, reverseDNS), store
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

type staticJournalHostProvider struct {
	machineID sdkjournal.UUID
	bootID    sdkjournal.UUID
	nextMono  atomic.Uint64
}

func newTestJournalHostProvider() *staticJournalHostProvider {
	machineID, _ := sdkjournal.ParseUUID("00112233445566778899aabbccddeeff")
	bootID, _ := sdkjournal.ParseUUID("ffeeddccbbaa99887766554433221100")
	provider := &staticJournalHostProvider{machineID: machineID, bootID: bootID}
	provider.nextMono.Store(999)
	return provider
}

func (p *staticJournalHostProvider) MachineID() sdkjournal.UUID { return p.machineID }
func (p *staticJournalHostProvider) BootID() sdkjournal.UUID    { return p.bootID }
func (p *staticJournalHostProvider) MonotonicUsec() uint64      { return p.nextMono.Add(1) }

func writeProfileYAML(t testing.TB, dir, name string, value any) {
	t.Helper()
	var data []byte
	switch v := value.(type) {
	case string:
		data = []byte(v)
	default:
		var err error
		data, err = yaml.Marshal(value)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

func writeProfileCatalogue(t testing.TB, stockDir string, profiles map[string]any) {
	t.Helper()
	data, err := json.Marshal(profiles)
	if err != nil {
		t.Fatalf("marshal catalogue: %v", err)
	}
	var entries map[string]map[string]any
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("decode catalogue: %v", err)
	}
	for name, entry := range entries {
		file, _ := entry["file"].(string)
		profileData, err := os.ReadFile(filepath.Join(stockDir, file))
		if err != nil {
			t.Fatalf("read stock profile for catalogue entry %q: %v", name, err)
		}
		entry["sha256"] = fmt.Sprintf("%x", sha256.Sum256(profileData))
	}
	data, err = json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal catalogue checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(stockDir), "catalogue.json"), data, 0o600); err != nil {
		t.Fatalf("write catalogue: %v", err)
	}
}

func testBaseChartTemplate() string {
	data, err := os.ReadFile(filepath.Join("..", "..", "charts.yaml"))
	if err != nil {
		panic(err)
	}
	return string(data)
}

func freeUDPPort(t testing.TB) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("allocate UDP port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}
