//go:build cgo
// +build cgo

package pmi

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/pmi/contexts"
	pmiproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/pmi"
)

// Collector implements the WebSphere PMI module using the ibm.d framework.
type Collector struct {
	framework.Collector

	Config `yaml:",inline" json:",inline"`

	once sync.Once

	client *pmiproto.Client

	identity common.Identity

	appSelector     matcher.Matcher
	poolSelector    matcher.Matcher
	jmsSelector     matcher.Matcher
	servletSelector matcher.Matcher
	ejbSelector     matcher.Matcher
}

func (c *Collector) initOnce() {
	c.once.Do(func() {})
}

// CollectOnce performs a single PMI collection iteration.
func (c *Collector) CollectOnce() error {
	c.initOnce()
	if c.client == nil {
		return errors.New("pmi client not initialised")
	}

	ctx := context.Background()
	snapshot, err := c.client.Fetch(ctx)
	if err != nil {
		return err
	}

	agg := newAggregator(c.Config)
	agg.identity = c.identity

	agg.processSnapshot(snapshot, c.applySelectors())
	c.Debugf("PMI aggregator counts: threadPools=%d transactions=%d jdbcPools=%d jcaPools=%d jmsQueues=%d jmsTopics=%d webApps=%d sessions=%d dynamicCaches=%d urls=%d securityAuth=%d", len(agg.threadPools), len(agg.transactions), len(agg.jdbcPools), len(agg.jcaPools), len(agg.jmsQueues), len(agg.jmsTopics), len(agg.webApps), len(agg.sessions), len(agg.dynamicCaches), len(agg.urls), len(agg.securityAuth))
	agg.exportMetrics(c.State)

	if labels := c.identity.Labels(); len(labels) > 0 {
		c.SetGlobalLabels(labels)
	}

	return nil
}

var _ framework.CollectorImpl = (*Collector)(nil)

type selectorBundle struct {
	app     matcher.Matcher
	pool    matcher.Matcher
	jms     matcher.Matcher
	servlet matcher.Matcher
	ejb     matcher.Matcher
}

func (c *Collector) applySelectors() selectorBundle {
	return selectorBundle{
		app:     c.appSelector,
		pool:    c.poolSelector,
		jms:     c.jmsSelector,
		servlet: c.servletSelector,
		ejb:     c.ejbSelector,
	}
}

type aggregator struct {
	cfg Config

	identity common.Identity

	system        jvmSystemMetrics
	threadPools   map[string]threadPoolMetrics
	transactions  map[string]*transactionMetrics
	jdbcPools     map[string]*jdbcPoolMetrics
	webApps       map[string]*webAppMetrics
	sessions      map[string]*sessionMetrics
	dynamicCaches map[string]*dynamicCacheMetrics
	urls          map[string]*urlMetrics
	securityAuth  map[string]*securityAuthMetrics
	orb           map[string]*orbMetrics
	systemData    systemDataMetrics
	securityAuthz map[string]*securityAuthorizationMetrics
	haManager     map[string]*haManagerMetrics
	alarmManagers map[string]*alarmManagerMetrics
	schedulers    map[string]*schedulerMetrics
	objectPools   map[string]*objectPoolMetrics
	enterpriseEJB map[string]*enterpriseBeanMetrics
	webServices   map[string]*webServiceMetrics
	webGateway    map[string]*webServiceGatewayMetrics
	pmiModules    map[string]*pmiWebServiceModuleMetrics
	extensionReg  extensionRegistryMetrics
	jcaPools      map[string]*jcaPoolMetrics
	jmsQueues     map[string]*jmsQueueMetrics
	jmsTopics     map[string]*jmsTopicMetrics
	jmsStores     map[string]*jmsStoreSectionMetrics
	portletApps   map[string]*portletAppMetrics
	portlets      map[string]*portletMetrics

	coverage *statCoverage
}

type statCoverage struct {
	all     map[string]struct{}
	handled map[string]struct{}
}

func newStatCoverage() *statCoverage {
	return &statCoverage{
		all:     make(map[string]struct{}),
		handled: make(map[string]struct{}),
	}
}

func (c *statCoverage) Reset() {
	if c == nil {
		return
	}
	c.all = make(map[string]struct{})
	c.handled = make(map[string]struct{})
}

func (c *statCoverage) Seed(snapshot *pmiproto.Snapshot) {
	if c == nil || snapshot == nil {
		return
	}
	for i := range snapshot.Nodes {
		node := &snapshot.Nodes[i]
		for j := range node.Servers {
			server := &node.Servers[j]
			for k := range server.Stats {
				c.walk(&server.Stats[k])
			}
		}
	}
	for i := range snapshot.Stats {
		c.walk(&snapshot.Stats[i])
	}
}

func (c *statCoverage) walk(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	path := statPath(stat)
	c.all[path] = struct{}{}
	for i := range stat.SubStats {
		c.walk(&stat.SubStats[i])
	}
}

func (c *statCoverage) Handle(stat *pmiproto.Stat) {
	if c == nil || stat == nil {
		return
	}
	path := statPath(stat)
	c.handled[path] = struct{}{}
}

func (c *statCoverage) Missing() []string {
	if c == nil {
		return nil
	}
	missing := make([]string, 0)
	for path := range c.all {
		if _, ok := c.handled[path]; !ok {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	return missing
}

func statPath(stat *pmiproto.Stat) string {
	if stat == nil {
		return ""
	}
	path := strings.TrimSpace(stat.Path)
	if path == "" {
		path = strings.TrimSpace(stat.Name)
	}
	return path
}

func (a *aggregator) markNestedStats(stats ...*pmiproto.Stat) {
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		a.coverage.Handle(stat)
		for i := range stat.SubStats {
			a.markNestedStats(&stat.SubStats[i])
		}
	}
}

func (a *aggregator) markNestedStatsSlice(stats []pmiproto.Stat) {
	for i := range stats {
		a.markNestedStats(&stats[i])
	}
}

type jvmSystemMetrics struct {
	cpuUtilization int64
	heapUsed       int64
	heapFree       int64
	heapCommitted  int64
	heapMax        int64
	uptimeSeconds  int64
	gcCollections  int64
	gcTimeMs       int64
	threadDaemon   int64
	threadOther    int64
	threadPeak     int64
}

type threadPoolMetrics struct {
	active int64
	size   int64
}

type transactionMetrics struct {
	node   string
	server string

	globalBegun      int64
	globalCommitted  int64
	globalRolledBack int64
	globalTimeout    int64
	globalInvolved   int64
	optimizations    int64

	localBegun      int64
	localCommitted  int64
	localRolledBack int64
	localTimeout    int64

	activeGlobal int64
	activeLocal  int64

	globalTotalMs            int64
	globalPrepareMs          int64
	globalCommitMs           int64
	globalBeforeCompletionMs int64
	localTotalMs             int64
	localCommitMs            int64
	localBeforeCompletionMs  int64
}

type jdbcPoolMetrics struct {
	node     string
	server   string
	name     string
	provider string

	percentUsed    int64
	percentMaxed   int64
	waitingThreads int64

	managedConnections int64
	connectionHandles  int64

	createCount      int64
	closeCount       int64
	allocateCount    int64
	returnCount      int64
	faultCount       int64
	prepDiscardCount int64

	useTimeMs  int64
	waitTimeMs int64
	jdbcTimeMs int64
}

type webAppMetrics struct {
	node   string
	server string
	name   string

	loadedServlets int64
	reloads        int64
}

type sessionMetrics struct {
	node   string
	server string
	app    string

	active int64
	live   int64

	createCount           int64
	invalidateCount       int64
	timeoutInvalidations  int64
	affinityBreaks        int64
	cacheDiscards         int64
	noRoomCount           int64
	activateNonExistCount int64
}

type dynamicCacheMetrics struct {
	node   string
	server string
	cache  string

	maxEntries int64
	entries    int64
}

type systemDataMetrics struct {
	cpuUsageSinceLast int64
	freeMemoryBytes   int64
}

type urlMetrics struct {
	node   string
	server string
	url    string

	requestCount    int64
	serviceTimeMs   int64
	asyncResponseMs int64
}

type portletAppMetrics struct {
	node           string
	server         string
	loadedPortlets int64
}

type portletMetrics struct {
	node   string
	server string
	name   string

	requestCount    int64
	concurrent      int64
	errors          int64
	renderTimeMs    int64
	actionTimeMs    int64
	processEventMs  int64
	serveResourceMs int64
}

type securityAuthMetrics struct {
	node   string
	server string

	webAuth            int64
	taiRequests        int64
	identityAssertions int64
	basicAuth          int64
	tokenAuth          int64
	jaasIdentity       int64
	jaasBasic          int64
	jaasToken          int64
	rmiAuth            int64
}

type orbMetrics struct {
	node   string
	server string

	concurrentRequests int64
	requestCount       int64
}

type securityAuthorizationMetrics struct {
	node   string
	server string

	webMs   int64
	ejbMs   int64
	adminMs int64
	cwwjaMs int64
}

type haManagerMetrics struct {
	node   string
	server string

	localGroups         int64
	bBoardSubjects      int64
	bBoardSubscriptions int64
	localSubjects       int64
	localSubscriptions  int64
	groupStateRebuildMs int64
	bBoardRebuildMs     int64
}

type alarmManagerMetrics struct {
	node   string
	server string
	name   string

	created   int64
	cancelled int64
	fired     int64
}

type schedulerMetrics struct {
	node     string
	server   string
	name     string
	finished int64
	failures int64
	polls    int64
}

type objectPoolMetrics struct {
	node      string
	server    string
	name      string
	created   int64
	allocated int64
	returned  int64
	idle      int64
}

type enterpriseBeanMetrics struct {
	node   string
	server string
	name   string

	createCount      int64
	removeCount      int64
	activateCount    int64
	passivateCount   int64
	instantiateCount int64
	storeCount       int64
	loadCount        int64

	messageCount      int64
	messageBackoutCnt int64

	readyCount             int64
	liveCount              int64
	pooledCount            int64
	activeMethodCount      int64
	passiveCount           int64
	serverSessionPoolUsage int64
	methodReadyCount       int64
	asyncQueueSize         int64

	activationTimeMs     int64
	passivationTimeMs    int64
	createTimeMs         int64
	removeTimeMs         int64
	loadTimeMs           int64
	storeTimeMs          int64
	methodResponseTimeMs int64
	waitTimeMs           int64
	asyncWaitTimeMs      int64
	readLockTimeMs       int64
	writeLockTimeMs      int64
}

type webServiceMetrics struct {
	node    string
	server  string
	service string

	loaded int64
}

type webServiceGatewayMetrics struct {
	node   string
	server string
	name   string

	syncRequests   int64
	syncResponses  int64
	asyncRequests  int64
	asyncResponses int64
}

type pmiWebServiceModuleMetrics struct {
	node   string
	server string
	name   string

	loaded int64
}

type extensionRegistryMetrics struct {
	node   string
	server string

	requests      int64
	hits          int64
	displacements int64
	hitRate       int64
}

type jcaPoolMetrics struct {
	node     string
	server   string
	provider string
	name     string

	createCount   int64
	closeCount    int64
	allocateCount int64
	freedCount    int64
	faultCount    int64

	managedConnections int64
	connectionHandles  int64

	percentUsed  int64
	percentMaxed int64

	waitingThreads int64
}

type jmsQueueMetrics struct {
	node   string
	server string
	engine string
	name   string

	totalProduced                 int64
	bestEffortProduced            int64
	expressProduced               int64
	reliableNonPersistentProduced int64
	reliablePersistentProduced    int64
	assuredPersistentProduced     int64

	totalConsumed                 int64
	bestEffortConsumed            int64
	expressConsumed               int64
	reliableNonPersistentConsumed int64
	reliablePersistentConsumed    int64
	assuredPersistentConsumed     int64

	reportEnabledExpired int64

	localProducerAttaches int64
	localProducerCount    int64
	localConsumerAttaches int64
	localConsumerCount    int64

	availableMessages   int64
	unavailableMessages int64
	oldestMessageAgeMs  int64

	aggregateWaitMs int64
	localWaitMs     int64
}

type jmsTopicMetrics struct {
	node   string
	server string
	engine string
	name   string

	assuredHits    int64
	bestEffortHits int64
	expressHits    int64

	assuredPublished    int64
	bestEffortPublished int64
	expressPublished    int64

	durableLocalSubscriptions int64
	incompletePublications    int64
	localOldestPublicationMs  int64
	localPublisherAttaches    int64
	localSubscriberAttaches   int64
}

type jmsStoreSectionMetrics struct {
	node    string
	server  string
	engine  string
	section string

	cacheAddStored             int64
	cacheAddNotStored          int64
	cacheCurrentStoredCount    int64
	cacheCurrentStoredBytes    int64
	cacheCurrentNotStoredCount int64
	cacheCurrentNotStoredBytes int64
	cacheDiscardCount          int64
	cacheDiscardBytes          int64

	datastoreInsertBatches int64
	datastoreUpdateBatches int64
	datastoreDeleteBatches int64
	datastoreInsertCount   int64
	datastoreUpdateCount   int64
	datastoreDeleteCount   int64
	datastoreOpenCount     int64
	datastoreAbortCount    int64
	datastoreTransactionMs int64

	expiryIndexItemCount int64

	globalTxnStart   int64
	globalTxnCommit  int64
	globalTxnAbort   int64
	globalTxnInDoubt int64
	localTxnStart    int64
	localTxnCommit   int64
	localTxnAbort    int64
}

func newAggregator(cfg Config) *aggregator {
	return &aggregator{
		cfg:           cfg,
		threadPools:   make(map[string]threadPoolMetrics),
		transactions:  make(map[string]*transactionMetrics),
		jdbcPools:     make(map[string]*jdbcPoolMetrics),
		webApps:       make(map[string]*webAppMetrics),
		sessions:      make(map[string]*sessionMetrics),
		dynamicCaches: make(map[string]*dynamicCacheMetrics),
		urls:          make(map[string]*urlMetrics),
		securityAuth:  make(map[string]*securityAuthMetrics),
		orb:           make(map[string]*orbMetrics),
		securityAuthz: make(map[string]*securityAuthorizationMetrics),
		haManager:     make(map[string]*haManagerMetrics),
		alarmManagers: make(map[string]*alarmManagerMetrics),
		schedulers:    make(map[string]*schedulerMetrics),
		objectPools:   make(map[string]*objectPoolMetrics),
		enterpriseEJB: make(map[string]*enterpriseBeanMetrics),
		webServices:   make(map[string]*webServiceMetrics),
		webGateway:    make(map[string]*webServiceGatewayMetrics),
		pmiModules:    make(map[string]*pmiWebServiceModuleMetrics),
		jcaPools:      make(map[string]*jcaPoolMetrics),
		jmsQueues:     make(map[string]*jmsQueueMetrics),
		jmsTopics:     make(map[string]*jmsTopicMetrics),
		jmsStores:     make(map[string]*jmsStoreSectionMetrics),
		portletApps:   make(map[string]*portletAppMetrics),
		portlets:      make(map[string]*portletMetrics),
		coverage:      newStatCoverage(),
	}
}

func (a *aggregator) processSnapshot(snapshot *pmiproto.Snapshot, selectors selectorBundle) {
	if a.coverage == nil {
		a.coverage = newStatCoverage()
	} else {
		a.coverage.Reset()
	}
	a.coverage.Seed(snapshot)

	for _, node := range snapshot.Nodes {
		for _, server := range node.Servers {
			a.processStats(node.Name, server.Name, server.Stats, selectors)
		}
	}
	if len(snapshot.Stats) > 0 {
		a.processStats("", "", snapshot.Stats, selectors)
	}
}

func (a *aggregator) processStats(node, server string, stats []pmiproto.Stat, selectors selectorBundle) {
	for i := range stats {
		stat := &stats[i]
		handled := false
		switch stat.Name {
		case "JVM Runtime":
			a.processJVMRuntime(stat)
			handled = true
		case "JVM Runtime MBean":
			a.processJVMRuntime(stat)
			handled = true
		case "JVM Thread":
			a.processJVMThreads(stat)
			handled = true
		case "JVM Thread MBean":
			a.processJVMThreads(stat)
			handled = true
		case "JVM Memory":
			a.processJVMMemory(stat)
			handled = true
		case "JVM Memory MBean":
			a.processJVMMemory(stat)
			handled = true
		case "JVM GC":
			a.processJVMGC(stat)
			handled = true
		case "Thread Pools":
			for j := range stat.SubStats {
				a.processThreadPool(&stat.SubStats[j])
			}
			handled = true
		case "Transaction Manager":
			a.processTransactionManager(node, server, stat)
			handled = true
		case "JDBC Connection Pools":
			if !a.collectJDBCMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				provider := &stat.SubStats[j]
				a.coverage.Handle(provider)
				for k := range provider.SubStats {
					a.processJDBCPool(node, server, provider.Name, &provider.SubStats[k], selectors)
				}
			}
			handled = true
		case "JCA Connection Pools":
			if !a.collectJCAMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				provider := &stat.SubStats[j]
				a.coverage.Handle(provider)
				for k := range provider.SubStats {
					a.processJCAPool(node, server, provider.Name, &provider.SubStats[k], selectors)
				}
			}
			handled = true
		case "Web Applications":
			if !a.collectWebAppMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				a.processWebApplication(node, server, &stat.SubStats[j], selectors)
			}
			handled = true
		case "Portlet Application":
			a.processPortletApplication(node, server, stat)
			handled = true
		case "Portlets":
			for j := range stat.SubStats {
				a.processPortlet(node, server, &stat.SubStats[j])
			}
			handled = true
		case "WIM Group Management":
			a.processPortlet(node, server, stat)
			handled = true
		case "WIM User Management":
			a.processPortlet(node, server, stat)
			handled = true
		case "URLs":
			if !a.collectServletMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				a.processURLMetric(node, server, &stat.SubStats[j], selectors)
			}
			handled = true
		case "Servlet Session Manager":
			if !a.collectSessionMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				a.processSessionManager(node, server, &stat.SubStats[j], selectors)
			}
			handled = true
		case "Dynamic Caching":
			if !a.collectDynamicCacheMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			for j := range stat.SubStats {
				a.processDynamicCache(node, server, &stat.SubStats[j])
			}
			handled = true
		case "System Data":
			a.processSystemData(stat)
			handled = true
		case "Security Authentication":
			a.processSecurityAuthentication(node, server, stat)
			handled = true
		case "ORB":
			a.processORB(node, server, stat)
			handled = true
		case "Object Request Broker":
			a.processORB(node, server, stat)
			handled = true
		case "Security Authorization":
			a.processSecurityAuthorization(node, server, stat)
			handled = true
		case "HAManager":
			a.processHAManager(node, server, stat)
			handled = true
		case "Alarm Manager":
			a.processAlarmManager(node, server, stat)
			handled = true
		case "Schedulers":
			a.processSchedulers(node, server, stat)
			handled = true
		case "Object Pool":
			a.processObjectPool(node, server, stat)
			handled = true
		case "Enterprise Beans":
			a.processEnterpriseBeans(node, server, stat, selectors)
			handled = true
		case "Web services":
			a.processWebServices(node, server, stat)
			handled = true
		case "Web services Gateway":
			a.processWebServicesGateway(node, server, stat)
			handled = true
		case "pmiWebServiceModule":
			a.processPMIWebServiceModule(node, server, stat)
			handled = true
		case "ExtensionRegistryStats.name":
			a.processExtensionRegistry(node, server, stat)
			handled = true
		case "SIB Service":
			if !a.collectJMSMetricsEnabled() {
				a.coverage.Handle(stat)
				continue
			}
			a.processSIBService(node, server, stat, selectors)
			handled = true
		}

		if handled {
			a.coverage.Handle(stat)
			continue
		}
		if len(stat.SubStats) > 0 {
			a.processStats(node, server, stat.SubStats, selectors)
			a.coverage.Handle(stat)
		}
	}
}

func (a *aggregator) processJVMRuntime(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	for _, cs := range stat.CountStatistics {
		switch strings.ToLower(cs.Name) {
		case "freememory":
			if v, ok := parseCount(cs.Count); ok {
				a.system.heapFree = convertUnits(v, cs.Unit, unitBytes)
			}
		case "usedmemory":
			if v, ok := parseCount(cs.Count); ok {
				a.system.heapUsed = convertUnits(v, cs.Unit, unitBytes)
			}
		case "heap":
			if v, ok := parseCount(cs.Count); ok {
				a.system.heapCommitted = convertUnits(v, cs.Unit, unitBytes)
			}
		case "uptime":
			if v, ok := parseCount(cs.Count); ok {
				a.system.uptimeSeconds = convertUnits(v, cs.Unit, unitSeconds)
			}
		case "processcpuusage":
			if v, ok := parseFloat(cs.Count); ok {
				a.system.cpuUtilization = int64(math.Round(v * 1000))
			}
		}
	}

	for _, ds := range stat.DoubleStatistics {
		switch strings.ToLower(ds.Name) {
		case "processcpuusage":
			if v, ok := parseFloat(ds.Double); ok {
				a.system.cpuUtilization = int64(math.Round(v * 1000))
			}
		}
	}

	for _, ts := range stat.TimeStatistics {
		if strings.EqualFold(ts.Name, "CPUUsage") {
			if v, err := strconv.ParseFloat(ts.Mean, 64); err == nil {
				a.system.cpuUtilization = int64(math.Round(v * 1000))
			}
		}
	}
}

func (a *aggregator) processJVMMemory(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	for _, rs := range stat.RangeStatistics {
		switch strings.ToLower(rs.Name) {
		case "heapbytesused":
			if v, ok := parseFloat(rs.Current); ok {
				a.system.heapUsed = int64(v)
			}
		case "heapbytesmax":
			if v, ok := parseFloat(rs.Current); ok {
				a.system.heapMax = int64(v)
			}
		case "heapbytescommitted":
			if v, ok := parseFloat(rs.Current); ok {
				a.system.heapCommitted = int64(v)
			}
		}
	}
}

func (a *aggregator) processJVMThreads(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	var total, daemon int64
	for _, rs := range stat.RangeStatistics {
		switch strings.ToLower(rs.Name) {
		case "livethreadcount":
			if v, ok := parseFloat(rs.Current); ok {
				total = int64(v)
			}
		case "daemonthreadcount":
			if v, ok := parseFloat(rs.Current); ok {
				daemon = int64(v)
			}
		case "peakthreadcount":
			if v, ok := parseFloat(rs.Current); ok {
				a.system.threadPeak = int64(v)
			}
		}
	}
	if daemon < 0 {
		daemon = 0
	}
	a.system.threadDaemon = daemon
	if total < daemon {
		total = daemon
	}
	a.system.threadOther = total - daemon
}

func (a *aggregator) processJVMGC(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	for _, cs := range stat.CountStatistics {
		switch strings.ToLower(cs.Name) {
		case "collectioncount":
			if v, ok := parseCount(cs.Count); ok {
				a.system.gcCollections = v
			}
		case "collectiontime":
			if v, ok := parseCount(cs.Count); ok {
				a.system.gcTimeMs = convertUnits(v, cs.Unit, unitMilliseconds)
			}
		}
	}
}

func (a *aggregator) processThreadPool(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	metrics := a.threadPools[stat.Name]
	for _, rs := range stat.RangeStatistics {
		switch strings.ToLower(rs.Name) {
		case "currentthreadsbusy", "currentthreadsbusycount":
			if v, ok := parseFloat(rs.Current); ok {
				metrics.active = int64(v)
			}
		case "currentthreadspoolsize", "currentthreadspoolsizecount":
			if v, ok := parseFloat(rs.Current); ok {
				metrics.size = int64(v)
			}
		}
	}
	a.threadPools[stat.Name] = metrics
}

func (a *aggregator) processTransactionManager(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	key := common.InstanceKey(node, server, "transaction_manager")
	metrics := a.transactions[key]
	if metrics == nil {
		metrics = &transactionMetrics{node: node, server: server}
		a.transactions[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "globalbeguncount":
			metrics.globalBegun = value
		case "globalcommittedcount", "committedcount":
			metrics.globalCommitted = value
		case "globalrolledbackcount", "rolledbackcount":
			metrics.globalRolledBack = value
		case "globaltimeoutcount":
			metrics.globalTimeout = value
		case "globalinvolvedcount":
			metrics.globalInvolved = value
		case "optimizationcount":
			metrics.optimizations = value
		case "localbeguncount":
			metrics.localBegun = value
		case "localcommittedcount":
			metrics.localCommitted = value
		case "localrolledbackcount":
			metrics.localRolledBack = value
		case "localtimeoutcount":
			metrics.localTimeout = value
		case "activecount":
			metrics.activeGlobal = value
		case "localactivecount":
			metrics.activeLocal = value
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "globaltrantime":
			metrics.globalTotalMs = ms
		case "globalpreparetime":
			metrics.globalPrepareMs = ms
		case "globalcommittime":
			metrics.globalCommitMs = ms
		case "globalbeforecompletiontime":
			metrics.globalBeforeCompletionMs = ms
		case "localtrantime":
			metrics.localTotalMs = ms
		case "localcommittime":
			metrics.localCommitMs = ms
		case "localbeforecompletiontime":
			metrics.localBeforeCompletionMs = ms
		}
	}
}

func (a *aggregator) processJDBCPool(node, server, provider string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	poolName := stat.Name
	if poolName == "" {
		poolName = provider
	}
	if poolName == "" {
		return
	}
	target := poolName
	if provider != "" {
		target = provider + "/" + poolName
	}
	if selectors.pool != nil && !selectors.pool.MatchString(target) {
		return
	}

	key := common.InstanceKey(node, server, poolName)
	metrics := a.jdbcPools[key]
	if metrics == nil {
		if a.cfg.MaxJDBCPools > 0 && len(a.jdbcPools) >= a.cfg.MaxJDBCPools {
			return
		}
		metrics = &jdbcPoolMetrics{node: node, server: server, name: poolName, provider: provider}
		a.jdbcPools[key] = metrics
	}

	for _, rs := range stat.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		switch strings.ToLower(rs.Name) {
		case "waitingthreadcount":
			metrics.waitingThreads = int64(math.Round(value))
		case "percentused":
			metrics.percentUsed = common.FormatPercent(value / 100.0)
		case "percentmaxed":
			metrics.percentMaxed = common.FormatPercent(value / 100.0)
		}
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "createcount":
			metrics.createCount = value
		case "closecount":
			metrics.closeCount = value
		case "allocatecount":
			metrics.allocateCount = value
		case "returncount", "freedcount":
			metrics.returnCount = value
		case "faultcount":
			metrics.faultCount = value
		case "managedconnectioncount":
			metrics.managedConnections = value
		case "connectionhandlecount":
			metrics.connectionHandles = value
		case "prepstmtcachediscardcount":
			metrics.prepDiscardCount = value
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "usetime":
			metrics.useTimeMs = ms
		case "waittime":
			metrics.waitTimeMs = ms
		case "jdbctime":
			metrics.jdbcTimeMs = ms
		}
	}
}

func (a *aggregator) processJCAPool(node, server, provider string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	poolName := stat.Name
	if poolName == "" {
		poolName = provider
	}
	if poolName == "" {
		return
	}

	target := poolName
	if provider != "" {
		target = provider + "/" + poolName
	}
	if selectors.pool != nil && !selectors.pool.MatchString(target) {
		return
	}

	key := common.InstanceKey(node, server, provider, poolName)
	metrics := a.jcaPools[key]
	if metrics == nil {
		if a.cfg.MaxJCAPools > 0 && len(a.jcaPools) >= a.cfg.MaxJCAPools {
			return
		}
		metrics = &jcaPoolMetrics{node: node, server: server, provider: provider, name: poolName}
		a.jcaPools[key] = metrics
	}

	for _, rs := range stat.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		name := strings.ToLower(rs.Name)
		switch name {
		case "percentused":
			metrics.percentUsed = common.FormatPercent(value / 100.0)
		case "percentmaxed":
			metrics.percentMaxed = common.FormatPercent(value / 100.0)
		case "waitingthreadcount":
			metrics.waitingThreads = int64(math.Round(value))
		}
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		name := strings.ToLower(cs.Name)
		switch name {
		case "createcount":
			metrics.createCount = value
		case "closecount":
			metrics.closeCount = value
		case "allocatecount":
			metrics.allocateCount = value
		case "freedcount":
			metrics.freedCount = value
		case "faultcount":
			metrics.faultCount = value
		case "managedconnectioncount":
			metrics.managedConnections = value
		case "connectionhandlecount":
			metrics.connectionHandles = value
		}
	}
}

func (a *aggregator) processWebApplication(node, server string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	name := stat.Name
	if name == "" {
		name = "unknown"
	}
	if selectors.app != nil && !selectors.app.MatchString(name) {
		return
	}

	key := common.InstanceKey(node, server, name)
	metrics := a.webApps[key]
	if metrics == nil {
		if a.cfg.MaxApplications > 0 && len(a.webApps) >= a.cfg.MaxApplications {
			return
		}
		metrics = &webAppMetrics{node: node, server: server, name: name}
		a.webApps[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "loadedservletcount":
			metrics.loadedServlets = value
		case "reloadcount":
			metrics.reloads = value
		}
	}

	if len(stat.SubStats) > 0 {
		a.processStats(node, server, stat.SubStats, selectors)
	}

	a.markNestedStatsSlice(stat.SubStats)
}

func (a *aggregator) processPortletApplication(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	key := common.InstanceKey(node, server, "portlet_application")
	metrics := a.portletApps[key]
	if metrics == nil {
		metrics = &portletAppMetrics{node: node, server: server}
		a.portletApps[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "number of loaded portlets":
			metrics.loadedPortlets = value
		}
	}
}

func (a *aggregator) processPortlet(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	name := stat.Name
	if name == "" {
		name = "unknown"
	}

	key := common.InstanceKey(node, server, name)
	metrics := a.portlets[key]
	if metrics == nil {
		metrics = &portletMetrics{node: node, server: server, name: name}
		a.portlets[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "number of portlet requests":
			metrics.requestCount = value
		case "number of portlet errors":
			metrics.errors = value
		}
	}

	for _, rs := range stat.RangeStatistics {
		val, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		switch strings.ToLower(rs.Name) {
		case "number of concurrent portlet requests":
			metrics.concurrent = int64(math.Round(val))
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "response time of portlet render":
			metrics.renderTimeMs = ms
		case "response time of portlet action":
			metrics.actionTimeMs = ms
		case "response time of a portlet processevent request":
			metrics.processEventMs = ms
		case "response time of a portlet serveresource request":
			metrics.serveResourceMs = ms
		}
	}
}

func (a *aggregator) processSessionManager(node, server string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	appName := stat.Name
	if appName == "" {
		appName = "unknown"
	}
	if selectors.app != nil && !selectors.app.MatchString(appName) {
		return
	}

	key := common.InstanceKey(node, server, appName, "sessions")
	metrics := a.sessions[key]
	if metrics == nil {
		if a.cfg.MaxApplications > 0 && len(a.sessions) >= a.cfg.MaxApplications {
			return
		}
		metrics = &sessionMetrics{node: node, server: server, app: appName}
		a.sessions[key] = metrics
	}

	for _, rs := range stat.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		switch strings.ToLower(rs.Name) {
		case "activecount":
			metrics.active = int64(math.Round(value))
		case "livecount":
			metrics.live = int64(math.Round(value))
		}
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "createcount":
			metrics.createCount = value
		case "invalidatecount":
			metrics.invalidateCount = value
		case "timeoutinvalidationcount":
			metrics.timeoutInvalidations = value
		case "affinitybreakcount":
			metrics.affinityBreaks = value
		case "cachediscardcount":
			metrics.cacheDiscards = value
		case "noroomfornewsessioncount":
			metrics.noRoomCount = value
		case "activatenonexistsessioncount":
			metrics.activateNonExistCount = value
		}
	}
}

func (a *aggregator) processDynamicCache(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	cacheName := stat.Name
	if cacheName == "" {
		cacheName = "default"
	}

	key := common.InstanceKey(node, server, cacheName)
	metrics := a.dynamicCaches[key]
	if metrics == nil {
		metrics = &dynamicCacheMetrics{node: node, server: server, cache: cacheName}
		a.dynamicCaches[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "maxinmemorycacheentrycount":
			metrics.maxEntries = value
		case "inmemorycacheentrycount":
			metrics.entries = value
		}
	}

	for i := range stat.SubStats {
		sub := &stat.SubStats[i]
		a.coverage.Handle(sub)
		for j := range sub.SubStats {
			a.coverage.Handle(&sub.SubStats[j])
		}
	}
}

func (a *aggregator) processSystemData(stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "cpuusagesincelastmeasurement":
			a.systemData.cpuUsageSinceLast = value
		case "freememory":
			a.systemData.freeMemoryBytes = convertUnits(value, cs.Unit, unitBytes)
		}
	}

	for _, rs := range stat.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		switch strings.ToLower(rs.Name) {
		case "freememory":
			a.systemData.freeMemoryBytes = convertUnits(int64(math.Round(value)), rs.Unit, unitBytes)
		}
	}
}

func (a *aggregator) processURLMetric(node, server string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	urlName := stat.Name
	if urlName == "" {
		urlName = "unknown"
	}
	if selectors.servlet != nil && !selectors.servlet.MatchString(urlName) {
		return
	}

	key := common.InstanceKey(node, server, urlName)
	metrics := a.urls[key]
	if metrics == nil {
		metrics = &urlMetrics{node: node, server: server, url: urlName}
		a.urls[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "urirequestcount":
			metrics.requestCount = value
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "uriservicetime":
			metrics.serviceTimeMs = ms
		case "url asynccontext response time":
			metrics.asyncResponseMs = ms
		}
	}
}

func (a *aggregator) processSecurityAuthentication(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	key := common.InstanceKey(node, server, "security_auth")
	metrics := a.securityAuth[key]
	if metrics == nil {
		metrics = &securityAuthMetrics{node: node, server: server}
		a.securityAuth[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "webauthenticationcount":
			metrics.webAuth = value
		case "tairequestcount":
			metrics.taiRequests = value
		case "identityassertioncount":
			metrics.identityAssertions = value
		case "basicauthenticationcount":
			metrics.basicAuth = value
		case "tokenauthenticationcount":
			metrics.tokenAuth = value
		case "jaasidentityassertioncount":
			metrics.jaasIdentity = value
		case "jaasbasicauthenticationcount":
			metrics.jaasBasic = value
		case "jaastokenauthenticationcount":
			metrics.jaasToken = value
		case "rmiauthenticationcount":
			metrics.rmiAuth = value
		}
	}
}

func (a *aggregator) processSecurityAuthorization(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	key := common.InstanceKey(node, server, "security_authz")
	metrics := a.securityAuthz[key]
	if metrics == nil {
		metrics = &securityAuthorizationMetrics{node: node, server: server}
		a.securityAuthz[key] = metrics
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "webauthorizationtime":
			metrics.webMs = ms
		case "ejbauthorizationtime":
			metrics.ejbMs = ms
		case "adminauthorizationtime":
			metrics.adminMs = ms
		case "cwwjaauthorizationtime":
			metrics.cwwjaMs = ms
		}
	}
}

func (a *aggregator) processORB(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	key := common.InstanceKey(node, server, "orb")
	metrics := a.orb[key]
	if metrics == nil {
		metrics = &orbMetrics{node: node, server: server}
		a.orb[key] = metrics
	}

	for _, rs := range stat.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		switch strings.ToLower(rs.Name) {
		case "concurrentrequestcount":
			metrics.concurrentRequests = int64(math.Round(value))
		}
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "requestcount":
			metrics.requestCount = value
		}
	}

	for i := range stat.SubStats {
		sub := &stat.SubStats[i]
		a.coverage.Handle(sub)
		if strings.EqualFold(sub.Name, "Interceptors") {
			for j := range sub.SubStats {
				interceptor := &sub.SubStats[j]
				a.coverage.Handle(interceptor)
				for k := range interceptor.CountStatistics {
					cs := interceptor.CountStatistics[k]
					if strings.EqualFold(cs.Name, "requestcount") {
						if value, ok := parseCount(cs.Count); ok {
							metrics.requestCount = value
						}
					}
				}
			}
		}
	}
}

func (a *aggregator) processHAManager(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	key := common.InstanceKey(node, server, "ha_manager")
	metrics := a.haManager[key]
	if metrics == nil {
		metrics = &haManagerMetrics{node: node, server: server}
		a.haManager[key] = metrics
	}

	updateCount := func(name string, value int64) {
		switch name {
		case "localgroupcount":
			metrics.localGroups = value
		case "bulletinboardsubjectcount":
			metrics.bBoardSubjects = value
		case "bulletinboardsubcriptioncount":
			metrics.bBoardSubscriptions = value
		case "localbulletinboardsubjectcount":
			metrics.localSubjects = value
		case "localbulletinboardsubcriptioncount":
			metrics.localSubscriptions = value
		}
	}

	for _, rs := range stat.RangeStatistics {
		if val, ok := parseFloat(rs.Current); ok {
			updateCount(strings.ToLower(rs.Name), int64(math.Round(val)))
		}
	}

	for _, br := range stat.BoundedRangeStatistics {
		if val, ok := parseFloat(br.Current); ok {
			updateCount(strings.ToLower(br.Name), int64(math.Round(val)))
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "groupstaterebuildtime":
			metrics.groupStateRebuildMs = ms
		case "bulletinboardrebuildtime":
			metrics.bBoardRebuildMs = ms
		}
	}

	for i := range stat.SubStats {
		a.coverage.Handle(&stat.SubStats[i])
	}
}

func (a *aggregator) processAlarmManager(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		manager := &stat.SubStats[i]
		a.coverage.Handle(manager)

		name := manager.Name
		if name == "" {
			name = "default"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.alarmManagers[key]
		if metrics == nil {
			metrics = &alarmManagerMetrics{node: node, server: server, name: name}
			a.alarmManagers[key] = metrics
		}
		for _, cs := range manager.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "alarmscreatedcount":
				metrics.created = value
			case "alarmscancelledcount":
				metrics.cancelled = value
			case "alarmsfiredcount":
				metrics.fired = value
			}
		}
	}
}

func (a *aggregator) processSchedulers(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		scheduler := &stat.SubStats[i]
		a.coverage.Handle(scheduler)

		name := scheduler.Name
		if name == "" {
			name = "default"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.schedulers[key]
		if metrics == nil {
			metrics = &schedulerMetrics{node: node, server: server, name: name}
			a.schedulers[key] = metrics
		}
		for _, cs := range scheduler.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "taskfinishcount":
				metrics.finished = value
			case "taskfailurecount":
				metrics.failures = value
			case "pollcount":
				metrics.polls = value
			}
		}
	}
}

func (a *aggregator) processSIBService(node, server string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		sub := &stat.SubStats[i]
		a.coverage.Handle(sub)
		if !strings.EqualFold(sub.Name, "SIB Messaging Engines") {
			continue
		}
		for j := range sub.SubStats {
			engine := &sub.SubStats[j]
			a.coverage.Handle(engine)
			engineName := engine.Name
			if engineName == "" {
				engineName = "engine"
			}
			a.processSIBMessagingEngine(node, server, engineName, engine, selectors)
		}
	}

	for i := range stat.SubStats {
		s := &stat.SubStats[i]
		for j := range s.SubStats {
			child := &s.SubStats[j]
			a.coverage.Handle(child)
			for k := range child.SubStats {
				a.coverage.Handle(&child.SubStats[k])
			}
		}
	}
}

func (a *aggregator) processSIBMessagingEngine(node, server, engine string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		sub := &stat.SubStats[i]
		a.coverage.Handle(sub)
		switch sub.Name {
		case "Destinations":
			for j := range sub.SubStats {
				destGroup := &sub.SubStats[j]
				a.coverage.Handle(destGroup)
				switch destGroup.Name {
				case "Queues":
					for k := range destGroup.SubStats {
						queue := &destGroup.SubStats[k]
						a.coverage.Handle(queue)
						a.processSIBQueue(node, server, engine, queue, selectors)
					}
				case "Topicspaces":
					for k := range destGroup.SubStats {
						topic := &destGroup.SubStats[k]
						a.coverage.Handle(topic)
						a.processSIBTopicSpace(node, server, engine, topic, selectors)
					}
				}
			}
		case "MessageStoreStats.group":
			for j := range sub.SubStats {
				section := &sub.SubStats[j]
				a.coverage.Handle(section)
				sectionName := section.Name
				if sectionName == "" {
					continue
				}
				a.processSIBMessageStoreSection(node, server, engine, sectionName, section)
			}
		}
	}
}

func (a *aggregator) processSIBQueue(node, server, engine string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	queueName := stat.Name
	if queueName == "" {
		queueName = "queue"
	}
	if selectors.jms != nil {
		candidate := engine + "/" + queueName
		if !selectors.jms.MatchString(candidate) && !selectors.jms.MatchString(queueName) {
			return
		}
	}
	key := common.InstanceKey(node, server, engine, queueName)
	metrics := a.jmsQueues[key]
	if metrics == nil {
		if a.cfg.MaxJMSDestinations > 0 && len(a.jmsQueues) >= a.cfg.MaxJMSDestinations {
			return
		}
		metrics = &jmsQueueMetrics{node: node, server: server, engine: engine, name: queueName}
		a.jmsQueues[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		name := strings.ToLower(cs.Name)
		switch name {
		case "queuestats.totalmessagesproducedcount":
			metrics.totalProduced = value
		case "queuestats.besteffortnonpersistentmessagesproducedcount":
			metrics.bestEffortProduced = value
		case "queuestats.expressnonpersistentmessagesproducedcount":
			metrics.expressProduced = value
		case "queuestats.reliablenonpersistentmessagesproducedcount":
			metrics.reliableNonPersistentProduced = value
		case "queuestats.reliablepersistentmessagesproducedcount":
			metrics.reliablePersistentProduced = value
		case "queuestats.assuredpersistentmessagesproducedcount":
			metrics.assuredPersistentProduced = value
		case "queuestats.totalmessagesconsumedcount":
			metrics.totalConsumed = value
		case "queuestats.besteffortnonpersistentmessagesconsumedcount":
			metrics.bestEffortConsumed = value
		case "queuestats.expressnonpersistentmessagesconsumedcount":
			metrics.expressConsumed = value
		case "queuestats.reliablenonpersistentmessagesconsumedcount":
			metrics.reliableNonPersistentConsumed = value
		case "queuestats.reliablepersistentmessagesconsumedcount":
			metrics.reliablePersistentConsumed = value
		case "queuestats.assuredpersistentmessagesconsumedcount":
			metrics.assuredPersistentConsumed = value
		case "queuestats.localproducerattachescount":
			metrics.localProducerAttaches = value
		case "queuestats.localproducercount":
			metrics.localProducerCount = value
		case "queuestats.localconsumerattachescount":
			metrics.localConsumerAttaches = value
		case "queuestats.localconsumercount":
			metrics.localConsumerCount = value
		case "queuestats.availablemessagecount":
			metrics.availableMessages = value
		case "queuestats.unavailablemessagecount":
			metrics.unavailableMessages = value
		case "queuestats.localoldestmessageage":
			metrics.oldestMessageAgeMs = convertUnits(value, cs.Unit, unitMilliseconds)
		case "queuestats.reportenabledmessagesexpiredcount":
			metrics.reportEnabledExpired = value
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		name := strings.ToLower(ts.Name)
		switch name {
		case "queuestats.aggregatemessagewaittime":
			metrics.aggregateWaitMs = ms
		case "queuestats.localmessagewaittime":
			metrics.localWaitMs = ms
		}
	}
}

func (a *aggregator) processSIBTopicSpace(node, server, engine string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	topicName := stat.Name
	if topicName == "" {
		topicName = "topicspace"
	}
	if selectors.jms != nil {
		candidate := engine + "/" + topicName
		if !selectors.jms.MatchString(candidate) && !selectors.jms.MatchString(topicName) {
			return
		}
	}
	key := common.InstanceKey(node, server, engine, topicName)
	metrics := a.jmsTopics[key]
	if metrics == nil {
		if a.cfg.MaxJMSDestinations > 0 && len(a.jmsTopics) >= a.cfg.MaxJMSDestinations {
			return
		}
		metrics = &jmsTopicMetrics{node: node, server: server, engine: engine, name: topicName}
		a.jmsTopics[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		name := strings.ToLower(cs.Name)
		switch name {
		case "topicspacestats.assuredpersistentlocalsubscriptionhitcount":
			metrics.assuredHits = value
		case "topicspacestats.besteffortnonpersistentlocalsubscriptionhitcount":
			metrics.bestEffortHits = value
		case "topicspacestats.expressnonpersistentlocalsubscriptionhitcount":
			metrics.expressHits = value
		case "topicspacestats.assuredpersistentmessagespublishedcount":
			metrics.assuredPublished = value
		case "topicspacestats.besteffortnonpersistentmessagespublishedcount":
			metrics.bestEffortPublished = value
		case "topicspacestats.expressnonpersistentmessagespublishedcount":
			metrics.expressPublished = value
		case "topicspacestats.durablelocalsubscriptioncount":
			metrics.durableLocalSubscriptions = value
		case "topicspacestats.incompletepublicationcount":
			metrics.incompletePublications = value
		case "topicspacestats.localoldestpublicationage":
			metrics.localOldestPublicationMs = convertUnits(value, cs.Unit, unitMilliseconds)
		case "topicspacestats.localpublisherattachescount":
			metrics.localPublisherAttaches = value
		case "topicspacestats.localsubscriberattachescount":
			metrics.localSubscriberAttaches = value
		}
	}

	a.markNestedStatsSlice(stat.SubStats)
}

func (a *aggregator) processSIBMessageStoreSection(node, server, engine, section string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	lowerSection := strings.ToLower(section)
	sectionLabel := section
	if strings.HasPrefix(lowerSection, "messagestorestats.") {
		sectionLabel = section[len("MessageStoreStats."):]
		lowerSection = strings.ToLower(sectionLabel)
	}
	key := common.InstanceKey(node, server, engine, sectionLabel)
	metrics := a.jmsStores[key]
	if metrics == nil {
		metrics = &jmsStoreSectionMetrics{node: node, server: server, engine: engine, section: sectionLabel}
		a.jmsStores[key] = metrics
	}

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		name := strings.ToLower(cs.Name)
		switch lowerSection {
		case "cache":
			switch name {
			case "messagestorestats.cacheaddstoredcount":
				metrics.cacheAddStored = value
			case "messagestorestats.cacheaddnotstoredcount":
				metrics.cacheAddNotStored = value
			case "messagestorestats.cachecurrentstoredcount":
				metrics.cacheCurrentStoredCount = value
			case "messagestorestats.cachecurrentstoredbytecount":
				metrics.cacheCurrentStoredBytes = value
			case "messagestorestats.cachecurrentnotstoredcount":
				metrics.cacheCurrentNotStoredCount = value
			case "messagestorestats.cachecurrentnotstoredbytecount":
				metrics.cacheCurrentNotStoredBytes = value
			case "messagestorestats.cachenotstoreddiscardcount":
				metrics.cacheDiscardCount = value
			case "messagestorestats.cachenotstoreddiscardbytecount":
				metrics.cacheDiscardBytes = value
			}
		case "datastore":
			switch name {
			case "messagestorestats.iteminsertbatchcount":
				metrics.datastoreInsertBatches = value
			case "messagestorestats.itemupdatebatchcount":
				metrics.datastoreUpdateBatches = value
			case "messagestorestats.itemdeletebatchcount":
				metrics.datastoreDeleteBatches = value
			case "messagestorestats.jdbciteminsertcount":
				metrics.datastoreInsertCount = value
			case "messagestorestats.jdbcitemupdatecount":
				metrics.datastoreUpdateCount = value
			case "messagestorestats.jdbcitemdeletecount":
				metrics.datastoreDeleteCount = value
			case "messagestorestats.jdbcopencount":
				metrics.datastoreOpenCount = value
			case "messagestorestats.jdbctransactionabortcount":
				metrics.datastoreAbortCount = value
			}
		case "expiry":
			switch name {
			case "messagestorestats.expiryindexitemcount":
				metrics.expiryIndexItemCount = value
			}
		case "transactions":
			switch name {
			case "messagestorestats.globaltransactionstartcount":
				metrics.globalTxnStart = value
			case "messagestorestats.globaltransactioncommitcount":
				metrics.globalTxnCommit = value
			case "messagestorestats.globaltransactionabortcount":
				metrics.globalTxnAbort = value
			case "messagestorestats.globaltransactionindoubtcount":
				metrics.globalTxnInDoubt = value
			case "messagestorestats.localtransactionstartcount":
				metrics.localTxnStart = value
			case "messagestorestats.localtransactioncommitcount":
				metrics.localTxnCommit = value
			case "messagestorestats.localtransactionabortcount":
				metrics.localTxnAbort = value
			}
		}
	}

	for _, ts := range stat.TimeStatistics {
		val, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(val)), ts.Unit, unitMilliseconds)
		name := strings.ToLower(ts.Name)
		if lowerSection == "datastore" && name == "messagestorestats.jdbctransactiontime" {
			metrics.datastoreTransactionMs = ms
		}
	}
}

func (a *aggregator) processObjectPool(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		pool := &stat.SubStats[i]
		a.coverage.Handle(pool)

		name := pool.Name
		if name == "" {
			name = "default"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.objectPools[key]
		if metrics == nil {
			metrics = &objectPoolMetrics{node: node, server: server, name: name}
			a.objectPools[key] = metrics
		}

		for _, cs := range pool.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "objectscreatedcount":
				metrics.created = value
			}
		}

		for _, rs := range pool.BoundedRangeStatistics {
			value, ok := parseFloat(rs.Current)
			if !ok {
				continue
			}
			rounded := int64(math.Round(value))
			switch strings.ToLower(rs.Name) {
			case "objectsallocatedcount":
				metrics.allocated = rounded
			case "objectsreturnedcount":
				metrics.returned = rounded
			case "idleobjectssize":
				metrics.idle = rounded
			}
		}

		for _, rs := range pool.RangeStatistics {
			value, ok := parseFloat(rs.Current)
			if !ok {
				continue
			}
			rounded := int64(math.Round(value))
			switch strings.ToLower(rs.Name) {
			case "objectsallocatedcount":
				metrics.allocated = rounded
			case "objectsreturnedcount":
				metrics.returned = rounded
			case "idleobjectssize":
				metrics.idle = rounded
			}
		}
	}
}

func (a *aggregator) processEnterpriseBeans(node, server string, stat *pmiproto.Stat, selectors selectorBundle) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	if a.cfg.CollectEJBMetrics != nil && !*a.cfg.CollectEJBMetrics {
		return
	}

	for i := range stat.SubStats {
		category := &stat.SubStats[i]
		a.coverage.Handle(category)

		if len(category.SubStats) == 0 {
			a.processEnterpriseBeanInstance(node, server, category.Name, category, selectors)
			continue
		}

		for j := range category.SubStats {
			bean := &category.SubStats[j]
			a.processEnterpriseBeanInstance(node, server, bean.Name, bean, selectors)
			a.markNestedStatsSlice(bean.SubStats)
		}
	}
}

func (a *aggregator) processEnterpriseBeanInstance(node, server, rawName string, bean *pmiproto.Stat, selectors selectorBundle) {
	if bean == nil {
		return
	}
	a.coverage.Handle(bean)

	name := rawName
	if name == "" {
		name = bean.Name
	}
	if name == "" {
		name = "unknown"
	}

	if selectors.ejb != nil && !selectors.ejb.MatchString(name) {
		return
	}

	key := common.InstanceKey(node, server, name)
	metrics := a.enterpriseEJB[key]
	if metrics == nil {
		if a.cfg.MaxEJBs > 0 && len(a.enterpriseEJB) >= a.cfg.MaxEJBs {
			return
		}
		metrics = &enterpriseBeanMetrics{node: node, server: server, name: name}
		a.enterpriseEJB[key] = metrics
	}

	for _, cs := range bean.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "createcount":
			metrics.createCount = value
		case "removecount":
			metrics.removeCount = value
		case "activatecount":
			metrics.activateCount = value
		case "passivatecount":
			metrics.passivateCount = value
		case "instantiatecount":
			metrics.instantiateCount = value
		case "storecount":
			metrics.storeCount = value
		case "loadcount":
			metrics.loadCount = value
		case "messagecount":
			metrics.messageCount = value
		case "messagebackoutcount":
			metrics.messageBackoutCnt = value
		}
	}

	for _, rs := range bean.RangeStatistics {
		value, ok := parseFloat(rs.Current)
		if !ok {
			continue
		}
		rounded := int64(math.Round(value))
		switch strings.ToLower(rs.Name) {
		case "readycount":
			metrics.readyCount = rounded
		case "livecount":
			metrics.liveCount = rounded
		case "pooledcount":
			metrics.pooledCount = rounded
		case "activemethodcount":
			metrics.activeMethodCount = rounded
		case "passivecount":
			metrics.passiveCount = rounded
		case "serversessionpoolusage":
			metrics.serverSessionPoolUsage = rounded
		case "methodreadycount":
			metrics.methodReadyCount = rounded
		case "asyncqsize":
			metrics.asyncQueueSize = rounded
		}
	}

	for _, ts := range bean.TimeStatistics {
		value, ok := parseFloat(ts.TotalTime)
		if !ok {
			continue
		}
		ms := convertUnits(int64(math.Round(value)), ts.Unit, unitMilliseconds)
		switch strings.ToLower(ts.Name) {
		case "activationtime":
			metrics.activationTimeMs = ms
		case "passivationtime":
			metrics.passivationTimeMs = ms
		case "createtime":
			metrics.createTimeMs = ms
		case "removetime":
			metrics.removeTimeMs = ms
		case "loadtime":
			metrics.loadTimeMs = ms
		case "storetime":
			metrics.storeTimeMs = ms
		case "methodresponsetime":
			metrics.methodResponseTimeMs = ms
		case "waittime":
			metrics.waitTimeMs = ms
		case "asyncwaittime":
			metrics.asyncWaitTimeMs = ms
		case "readlocktime":
			metrics.readLockTimeMs = ms
		case "writelocktime":
			metrics.writeLockTimeMs = ms
		}
	}

	a.markNestedStatsSlice(bean.SubStats)
}

func (a *aggregator) processWebServices(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		svc := &stat.SubStats[i]
		a.coverage.Handle(svc)
		name := svc.Name
		if name == "" {
			name = "default"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.webServices[key]
		if metrics == nil {
			metrics = &webServiceMetrics{node: node, server: server, service: name}
			a.webServices[key] = metrics
		}
		for _, cs := range svc.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "loadedwebservicecount":
				metrics.loaded = value
			}
		}

		for j := range svc.SubStats {
			a.coverage.Handle(&svc.SubStats[j])
		}
	}
}

func (a *aggregator) processWebServicesGateway(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		gateway := &stat.SubStats[i]
		a.coverage.Handle(gateway)
		name := gateway.Name
		if name == "" {
			name = "gateway"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.webGateway[key]
		if metrics == nil {
			metrics = &webServiceGatewayMetrics{node: node, server: server, name: name}
			a.webGateway[key] = metrics
		}
		for _, cs := range gateway.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "synchronousrequestcount":
				metrics.syncRequests = value
			case "synchronousresponsecount":
				metrics.syncResponses = value
			case "asynchronousrequestcount":
				metrics.asyncRequests = value
			case "asynchronousresponsecount":
				metrics.asyncResponses = value
			}
		}

		for j := range gateway.SubStats {
			a.coverage.Handle(&gateway.SubStats[j])
		}
	}
}

func (a *aggregator) processPMIWebServiceModule(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	for i := range stat.SubStats {
		module := &stat.SubStats[i]
		a.coverage.Handle(module)
		name := module.Name
		if name == "" {
			name = "module"
		}
		key := common.InstanceKey(node, server, name)
		metrics := a.pmiModules[key]
		if metrics == nil {
			metrics = &pmiWebServiceModuleMetrics{node: node, server: server, name: name}
			a.pmiModules[key] = metrics
		}
		for _, cs := range module.CountStatistics {
			value, ok := parseCount(cs.Count)
			if !ok {
				continue
			}
			switch strings.ToLower(cs.Name) {
			case "servicesloaded":
				metrics.loaded = value
			}
		}

		for j := range module.SubStats {
			a.coverage.Handle(&module.SubStats[j])
		}
	}
}

func (a *aggregator) processExtensionRegistry(node, server string, stat *pmiproto.Stat) {
	if stat == nil {
		return
	}
	a.coverage.Handle(stat)
	metrics := a.extensionReg
	metrics.node = node
	metrics.server = server

	for _, cs := range stat.CountStatistics {
		value, ok := parseCount(cs.Count)
		if !ok {
			continue
		}
		switch strings.ToLower(cs.Name) {
		case "requestcount":
			metrics.requests = value
		case "hitcount":
			metrics.hits = value
		case "displacementcount":
			metrics.displacements = value
		}
	}

	for _, ds := range stat.DoubleStatistics {
		value, ok := parseFloat(ds.Double)
		if !ok {
			continue
		}
		if strings.EqualFold(ds.Name, "HitRate") {
			metrics.hitRate = int64(math.Round(value * 1000))
		}
	}

	a.extensionReg = metrics
}

func (a *aggregator) exportMetrics(state *framework.CollectorState) {
	contexts.System.CPU.Set(state, contexts.EmptyLabels{}, contexts.SystemCPUValues{
		Utilization: a.system.cpuUtilization,
	})

	used := a.system.heapUsed
	free := a.system.heapFree
	if free == 0 && a.system.heapCommitted > 0 && used > 0 {
		if calculated := a.system.heapCommitted - used; calculated > 0 {
			free = calculated
		}
	}
	contexts.JVM.HeapUsage.Set(state, contexts.EmptyLabels{}, contexts.JVMHeapUsageValues{
		Used: used,
		Free: free,
	})
	if a.system.heapCommitted > 0 {
		contexts.JVM.HeapCommitted.Set(state, contexts.EmptyLabels{}, contexts.JVMHeapCommittedValues{
			Committed: a.system.heapCommitted,
		})
	}
	if a.system.heapMax > 0 {
		contexts.JVM.HeapMax.Set(state, contexts.EmptyLabels{}, contexts.JVMHeapMaxValues{
			Limit: a.system.heapMax,
		})
	}
	if a.system.uptimeSeconds > 0 {
		contexts.JVM.Uptime.Set(state, contexts.EmptyLabels{}, contexts.JVMUptimeValues{
			Uptime: a.system.uptimeSeconds,
		})
	}
	contexts.JVM.CPU.Set(state, contexts.EmptyLabels{}, contexts.JVMCPUValues{
		Usage: a.system.cpuUtilization,
	})
	if a.system.gcCollections > 0 {
		contexts.JVM.GCCollections.Set(state, contexts.EmptyLabels{}, contexts.JVMGCCollectionsValues{
			Collections: a.system.gcCollections,
		})
	}
	if a.system.gcTimeMs > 0 {
		contexts.JVM.GCTime.Set(state, contexts.EmptyLabels{}, contexts.JVMGCTimeValues{
			Total: a.system.gcTimeMs,
		})
	}
	contexts.JVM.Threads.Set(state, contexts.EmptyLabels{}, contexts.JVMThreadsValues{
		Daemon: a.system.threadDaemon,
		Other:  a.system.threadOther,
	})
	if a.system.threadPeak > 0 {
		contexts.JVM.ThreadPeak.Set(state, contexts.EmptyLabels{}, contexts.JVMThreadPeakValues{
			Peak: a.system.threadPeak,
		})
	}

	for name, tp := range a.threadPools {
		labels := contexts.ThreadPoolLabels{Name: name}
		contexts.ThreadPool.Usage.Set(state, labels, contexts.ThreadPoolUsageValues{
			Active: tp.active,
			Size:   tp.size,
		})
	}

	for _, tx := range a.transactions {
		labels := contexts.TransactionManagerLabels{Node: tx.node, Server: tx.server}
		contexts.TransactionManager.Counts.Set(state, labels, contexts.TransactionManagerCountsValues{
			Global_begun:       tx.globalBegun,
			Global_committed:   tx.globalCommitted,
			Global_rolled_back: tx.globalRolledBack,
			Global_timeout:     tx.globalTimeout,
			Global_involved:    tx.globalInvolved,
			Optimizations:      tx.optimizations,
			Local_begun:        tx.localBegun,
			Local_committed:    tx.localCommitted,
			Local_rolled_back:  tx.localRolledBack,
			Local_timeout:      tx.localTimeout,
		})
		contexts.TransactionManager.Active.Set(state, labels, contexts.TransactionManagerActiveValues{
			Global: tx.activeGlobal,
			Local:  tx.activeLocal,
		})
		contexts.TransactionManager.Time.Set(state, labels, contexts.TransactionManagerTimeValues{
			Global_total:             tx.globalTotalMs,
			Global_prepare:           tx.globalPrepareMs,
			Global_commit:            tx.globalCommitMs,
			Global_before_completion: tx.globalBeforeCompletionMs,
			Local_total:              tx.localTotalMs,
			Local_commit:             tx.localCommitMs,
			Local_before_completion:  tx.localBeforeCompletionMs,
		})
	}

	for _, pool := range a.jdbcPools {
		labels := contexts.JDBCPoolLabels{Node: pool.node, Server: pool.server, Pool: pool.name}
		contexts.JDBCPool.Usage.Set(state, labels, contexts.JDBCPoolUsageValues{
			Percent_used:  pool.percentUsed,
			Percent_maxed: pool.percentMaxed,
		})
		contexts.JDBCPool.Waiting.Set(state, labels, contexts.JDBCPoolWaitingValues{
			Waiting_threads: pool.waitingThreads,
		})
		contexts.JDBCPool.Connections.Set(state, labels, contexts.JDBCPoolConnectionsValues{
			Managed: pool.managedConnections,
			Handles: pool.connectionHandles,
		})
		contexts.JDBCPool.Operations.Set(state, labels, contexts.JDBCPoolOperationsValues{
			Created:                 pool.createCount,
			Closed:                  pool.closeCount,
			Allocated:               pool.allocateCount,
			Returned:                pool.returnCount,
			Faults:                  pool.faultCount,
			Prep_stmt_cache_discard: pool.prepDiscardCount,
		})
		contexts.JDBCPool.Time.Set(state, labels, contexts.JDBCPoolTimeValues{
			Use:  pool.useTimeMs,
			Wait: pool.waitTimeMs,
			Jdbc: pool.jdbcTimeMs,
		})
	}

	for _, dc := range a.dynamicCaches {
		labels := contexts.DynamicCacheLabels{Node: dc.node, Server: dc.server, Cache: dc.cache}
		contexts.DynamicCache.InMemory.Set(state, labels, contexts.DynamicCacheInMemoryValues{
			Entries: dc.entries,
		})
		contexts.DynamicCache.Capacity.Set(state, labels, contexts.DynamicCacheCapacityValues{
			Max_entries: dc.maxEntries,
		})
	}

	for _, url := range a.urls {
		labels := contexts.URLLabels{Node: url.node, Server: url.server, Url: url.url}
		contexts.URL.Requests.Set(state, labels, contexts.URLRequestsValues{
			Requests: url.requestCount,
		})
		contexts.URL.Time.Set(state, labels, contexts.URLTimeValues{
			Service: url.serviceTimeMs,
			Async:   url.asyncResponseMs,
		})
	}

	for _, auth := range a.securityAuth {
		labels := contexts.SecurityAuthLabels{Node: auth.node, Server: auth.server}
		contexts.SecurityAuth.Counts.Set(state, labels, contexts.SecurityAuthCountsValues{
			Web:           auth.webAuth,
			Tai:           auth.taiRequests,
			Identity:      auth.identityAssertions,
			Basic:         auth.basicAuth,
			Token:         auth.tokenAuth,
			Jaas_identity: auth.jaasIdentity,
			Jaas_basic:    auth.jaasBasic,
			Jaas_token:    auth.jaasToken,
			Rmi:           auth.rmiAuth,
		})
	}

	for _, orb := range a.orb {
		labels := contexts.ORBLabels{Node: orb.node, Server: orb.server}
		contexts.ORB.Concurrent.Set(state, labels, contexts.ORBConcurrentValues{
			Concurrent_requests: orb.concurrentRequests,
		})
		contexts.ORB.Requests.Set(state, labels, contexts.ORBRequestsValues{
			Requests: orb.requestCount,
		})
	}

	for _, app := range a.webApps {
		labels := contexts.WebAppLabels{Node: app.node, Server: app.server, App: app.name}
		contexts.WebApp.Load.Set(state, labels, contexts.WebAppLoadValues{
			Loaded_servlets: app.loadedServlets,
			Reloads:         app.reloads,
		})
	}

	for _, sess := range a.sessions {
		labels := contexts.SessionManagerLabels{Node: sess.node, Server: sess.server, App: sess.app}
		contexts.SessionManager.Active.Set(state, labels, contexts.SessionManagerActiveValues{
			Active: sess.active,
			Live:   sess.live,
		})
		contexts.SessionManager.Events.Set(state, labels, contexts.SessionManagerEventsValues{
			Created:               sess.createCount,
			Invalidated:           sess.invalidateCount,
			Timeout_invalidations: sess.timeoutInvalidations,
			Affinity_breaks:       sess.affinityBreaks,
			Cache_discards:        sess.cacheDiscards,
			No_room:               sess.noRoomCount,
			Activate_non_exist:    sess.activateNonExistCount,
		})
	}

	for _, dc := range a.dynamicCaches {
		labels := contexts.DynamicCacheLabels{Node: dc.node, Server: dc.server, Cache: dc.cache}
		contexts.DynamicCache.InMemory.Set(state, labels, contexts.DynamicCacheInMemoryValues{
			Entries: dc.entries,
		})
		contexts.DynamicCache.Capacity.Set(state, labels, contexts.DynamicCacheCapacityValues{
			Max_entries: dc.maxEntries,
		})
	}

	for _, url := range a.urls {
		labels := contexts.URLLabels{Node: url.node, Server: url.server, Url: url.url}
		contexts.URL.Requests.Set(state, labels, contexts.URLRequestsValues{
			Requests: url.requestCount,
		})
		contexts.URL.Time.Set(state, labels, contexts.URLTimeValues{
			Service: url.serviceTimeMs,
			Async:   url.asyncResponseMs,
		})
	}

	for _, auth := range a.securityAuth {
		labels := contexts.SecurityAuthLabels{Node: auth.node, Server: auth.server}
		contexts.SecurityAuth.Counts.Set(state, labels, contexts.SecurityAuthCountsValues{
			Web:           auth.webAuth,
			Tai:           auth.taiRequests,
			Identity:      auth.identityAssertions,
			Basic:         auth.basicAuth,
			Token:         auth.tokenAuth,
			Jaas_identity: auth.jaasIdentity,
			Jaas_basic:    auth.jaasBasic,
			Jaas_token:    auth.jaasToken,
			Rmi:           auth.rmiAuth,
		})
	}

	for _, authz := range a.securityAuthz {
		labels := contexts.SecurityAuthzLabels{Node: authz.node, Server: authz.server}
		contexts.SecurityAuthz.Time.Set(state, labels, contexts.SecurityAuthzTimeValues{
			Web:   authz.webMs,
			Ejb:   authz.ejbMs,
			Admin: authz.adminMs,
			Cwwja: authz.cwwjaMs,
		})
	}

	for _, ha := range a.haManager {
		labels := contexts.HAManagerLabels{Node: ha.node, Server: ha.server}
		contexts.HAManager.Groups.Set(state, labels, contexts.HAManagerGroupsValues{
			Local: ha.localGroups,
		})
		contexts.HAManager.BulletinBoard.Set(state, labels, contexts.HAManagerBulletinBoardValues{
			Subjects:            ha.bBoardSubjects,
			Subscriptions:       ha.bBoardSubscriptions,
			Local_subjects:      ha.localSubjects,
			Local_subscriptions: ha.localSubscriptions,
		})
		contexts.HAManager.RebuildTime.Set(state, labels, contexts.HAManagerRebuildTimeValues{
			Group_state:    ha.groupStateRebuildMs,
			Bulletin_board: ha.bBoardRebuildMs,
		})
	}

	for _, alarm := range a.alarmManagers {
		labels := contexts.AlarmManagerLabels{Node: alarm.node, Server: alarm.server, Manager: alarm.name}
		contexts.AlarmManager.Events.Set(state, labels, contexts.AlarmManagerEventsValues{
			Created:   alarm.created,
			Cancelled: alarm.cancelled,
			Fired:     alarm.fired,
		})
	}

	for _, scheduler := range a.schedulers {
		labels := contexts.SchedulersLabels{Node: scheduler.node, Server: scheduler.server, Scheduler: scheduler.name}
		contexts.Schedulers.Activity.Set(state, labels, contexts.SchedulersActivityValues{
			Finished: scheduler.finished,
			Failures: scheduler.failures,
			Polls:    scheduler.polls,
		})
	}

	for _, pool := range a.objectPools {
		labels := contexts.ObjectPoolLabels{Node: pool.node, Server: pool.server, Pool: pool.name}
		contexts.ObjectPool.Operations.Set(state, labels, contexts.ObjectPoolOperationsValues{
			Created: pool.created,
		})
		contexts.ObjectPool.Size.Set(state, labels, contexts.ObjectPoolSizeValues{
			Allocated: pool.allocated,
			Returned:  pool.returned,
			Idle:      pool.idle,
		})
	}

	for _, pool := range a.jcaPools {
		labels := contexts.JCAPoolLabels{Node: pool.node, Server: pool.server, Provider: pool.provider, Pool: pool.name}
		contexts.JCAPool.Operations.Set(state, labels, contexts.JCAPoolOperationsValues{
			Create:   pool.createCount,
			Close:    pool.closeCount,
			Allocate: pool.allocateCount,
			Freed:    pool.freedCount,
			Faults:   pool.faultCount,
		})
		contexts.JCAPool.Managed.Set(state, labels, contexts.JCAPoolManagedValues{
			Managed_connections: pool.managedConnections,
			Connection_handles:  pool.connectionHandles,
		})
		contexts.JCAPool.Utilization.Set(state, labels, contexts.JCAPoolUtilizationValues{
			Percent_used:  pool.percentUsed,
			Percent_maxed: pool.percentMaxed,
		})
		contexts.JCAPool.Waiting.Set(state, labels, contexts.JCAPoolWaitingValues{
			Waiting_threads: pool.waitingThreads,
		})
	}

	for _, queue := range a.jmsQueues {
		labels := contexts.JMSQueueLabels{Node: queue.node, Server: queue.server, Engine: queue.engine, Destination: queue.name}
		contexts.JMSQueue.MessagesProduced.Set(state, labels, contexts.JMSQueueMessagesProducedValues{
			Total:                  queue.totalProduced,
			Best_effort:            queue.bestEffortProduced,
			Express:                queue.expressProduced,
			Reliable_nonpersistent: queue.reliableNonPersistentProduced,
			Reliable_persistent:    queue.reliablePersistentProduced,
			Assured_persistent:     queue.assuredPersistentProduced,
		})
		contexts.JMSQueue.MessagesConsumed.Set(state, labels, contexts.JMSQueueMessagesConsumedValues{
			Total:                  queue.totalConsumed,
			Best_effort:            queue.bestEffortConsumed,
			Express:                queue.expressConsumed,
			Reliable_nonpersistent: queue.reliableNonPersistentConsumed,
			Reliable_persistent:    queue.reliablePersistentConsumed,
			Assured_persistent:     queue.assuredPersistentConsumed,
			Expired:                queue.reportEnabledExpired,
		})
		contexts.JMSQueue.Clients.Set(state, labels, contexts.JMSQueueClientsValues{
			Local_producers:         queue.localProducerCount,
			Local_producer_attaches: queue.localProducerAttaches,
			Local_consumers:         queue.localConsumerCount,
			Local_consumer_attaches: queue.localConsumerAttaches,
		})
		contexts.JMSQueue.Storage.Set(state, labels, contexts.JMSQueueStorageValues{
			Available:   queue.availableMessages,
			Unavailable: queue.unavailableMessages,
			Oldest_age:  queue.oldestMessageAgeMs,
		})
		contexts.JMSQueue.WaitTime.Set(state, labels, contexts.JMSQueueWaitTimeValues{
			Aggregate: queue.aggregateWaitMs,
			Local:     queue.localWaitMs,
		})
	}

	for _, topic := range a.jmsTopics {
		labels := contexts.JMSTopicLabels{Node: topic.node, Server: topic.server, Engine: topic.engine, Destination: topic.name}
		contexts.JMSTopic.Publications.Set(state, labels, contexts.JMSTopicPublicationsValues{
			Assured:     topic.assuredPublished,
			Best_effort: topic.bestEffortPublished,
			Express:     topic.expressPublished,
		})
		contexts.JMSTopic.SubscriptionHits.Set(state, labels, contexts.JMSTopicSubscriptionHitsValues{
			Assured:     topic.assuredHits,
			Best_effort: topic.bestEffortHits,
			Express:     topic.expressHits,
		})
		contexts.JMSTopic.Subscriptions.Set(state, labels, contexts.JMSTopicSubscriptionsValues{
			Durable_local: topic.durableLocalSubscriptions,
		})
		contexts.JMSTopic.Events.Set(state, labels, contexts.JMSTopicEventsValues{
			Incomplete_publications: topic.incompletePublications,
			Publisher_attaches:      topic.localPublisherAttaches,
			Subscriber_attaches:     topic.localSubscriberAttaches,
		})
		contexts.JMSTopic.Age.Set(state, labels, contexts.JMSTopicAgeValues{
			Local_oldest: topic.localOldestPublicationMs,
		})
	}

	for _, store := range a.jmsStores {
		labels := contexts.JMSStoreLabels{Node: store.node, Server: store.server, Engine: store.engine, Section: store.section}
		switch strings.ToLower(store.section) {
		case "cache":
			contexts.JMSStore.Cache.Set(state, labels, contexts.JMSStoreCacheValues{
				Add_stored:         store.cacheAddStored,
				Add_not_stored:     store.cacheAddNotStored,
				Stored_current:     store.cacheCurrentStoredCount,
				Stored_bytes:       store.cacheCurrentStoredBytes,
				Not_stored_current: store.cacheCurrentNotStoredCount,
				Not_stored_bytes:   store.cacheCurrentNotStoredBytes,
				Discard_count:      store.cacheDiscardCount,
				Discard_bytes:      store.cacheDiscardBytes,
			})
		case "datastore":
			contexts.JMSStore.Datastore.Set(state, labels, contexts.JMSStoreDatastoreValues{
				Insert_batches: store.datastoreInsertBatches,
				Update_batches: store.datastoreUpdateBatches,
				Delete_batches: store.datastoreDeleteBatches,
				Insert_count:   store.datastoreInsertCount,
				Update_count:   store.datastoreUpdateCount,
				Delete_count:   store.datastoreDeleteCount,
				Open_count:     store.datastoreOpenCount,
				Abort_count:    store.datastoreAbortCount,
				Transaction_ms: store.datastoreTransactionMs,
			})
		case "transactions":
			contexts.JMSStore.Transactions.Set(state, labels, contexts.JMSStoreTransactionsValues{
				Global_start:   store.globalTxnStart,
				Global_commit:  store.globalTxnCommit,
				Global_abort:   store.globalTxnAbort,
				Global_indoubt: store.globalTxnInDoubt,
				Local_start:    store.localTxnStart,
				Local_commit:   store.localTxnCommit,
				Local_abort:    store.localTxnAbort,
			})
		case "expiry":
			contexts.JMSStore.Expiry.Set(state, labels, contexts.JMSStoreExpiryValues{
				Index_items: store.expiryIndexItemCount,
			})
		}
	}

	for _, app := range a.portletApps {
		labels := contexts.PortletApplicationLabels{Node: app.node, Server: app.server}
		contexts.PortletApplication.Loaded.Set(state, labels, contexts.PortletApplicationLoadedValues{
			Loaded: app.loadedPortlets,
		})
	}

	for _, portlet := range a.portlets {
		labels := contexts.PortletLabels{Node: portlet.node, Server: portlet.server, Portlet: portlet.name}
		contexts.Portlet.Requests.Set(state, labels, contexts.PortletRequestsValues{
			Requests: portlet.requestCount,
		})
		contexts.Portlet.Concurrent.Set(state, labels, contexts.PortletConcurrentValues{
			Concurrent: portlet.concurrent,
		})
		contexts.Portlet.Errors.Set(state, labels, contexts.PortletErrorsValues{
			Errors: portlet.errors,
		})
		contexts.Portlet.ResponseTime.Set(state, labels, contexts.PortletResponseTimeValues{
			Render:         portlet.renderTimeMs,
			Action:         portlet.actionTimeMs,
			Process_event:  portlet.processEventMs,
			Serve_resource: portlet.serveResourceMs,
		})
	}

	for _, bean := range a.enterpriseEJB {
		labels := contexts.EnterpriseBeansLabels{Node: bean.node, Server: bean.server, Bean: bean.name}
		contexts.EnterpriseBeans.Operations.Set(state, labels, contexts.EnterpriseBeansOperationsValues{
			Create:      bean.createCount,
			Remove:      bean.removeCount,
			Activate:    bean.activateCount,
			Passivate:   bean.passivateCount,
			Instantiate: bean.instantiateCount,
			Store:       bean.storeCount,
			Load:        bean.loadCount,
		})
		contexts.EnterpriseBeans.Messages.Set(state, labels, contexts.EnterpriseBeansMessagesValues{
			Received: bean.messageCount,
			Backout:  bean.messageBackoutCnt,
		})
		contexts.EnterpriseBeans.Pool.Set(state, labels, contexts.EnterpriseBeansPoolValues{
			Ready:               bean.readyCount,
			Live:                bean.liveCount,
			Pooled:              bean.pooledCount,
			Active_method:       bean.activeMethodCount,
			Passive:             bean.passiveCount,
			Server_session_pool: bean.serverSessionPoolUsage,
			Method_ready:        bean.methodReadyCount,
			Async_queue:         bean.asyncQueueSize,
		})
		contexts.EnterpriseBeans.Time.Set(state, labels, contexts.EnterpriseBeansTimeValues{
			Activation:      bean.activationTimeMs,
			Passivation:     bean.passivationTimeMs,
			Create:          bean.createTimeMs,
			Remove:          bean.removeTimeMs,
			Load:            bean.loadTimeMs,
			Store:           bean.storeTimeMs,
			Method_response: bean.methodResponseTimeMs,
			Wait:            bean.waitTimeMs,
			Async_wait:      bean.asyncWaitTimeMs,
			Read_lock:       bean.readLockTimeMs,
			Write_lock:      bean.writeLockTimeMs,
		})
	}

	for _, svc := range a.webServices {
		labels := contexts.WebServicesLabels{Node: svc.node, Server: svc.server, Service: svc.service}
		contexts.WebServices.Loaded.Set(state, labels, contexts.WebServicesLoadedValues{
			Loaded: svc.loaded,
		})
	}

	for _, gw := range a.webGateway {
		labels := contexts.WebServicesGatewayLabels{Node: gw.node, Server: gw.server, Gateway: gw.name}
		contexts.WebServicesGateway.Requests.Set(state, labels, contexts.WebServicesGatewayRequestsValues{
			Synchronous:            gw.syncRequests,
			Synchronous_responses:  gw.syncResponses,
			Asynchronous:           gw.asyncRequests,
			Asynchronous_responses: gw.asyncResponses,
		})
	}

	for _, module := range a.pmiModules {
		labels := contexts.PMIWebServiceModuleLabels{Node: module.node, Server: module.server, Module: module.name}
		contexts.PMIWebServiceModule.Services.Set(state, labels, contexts.PMIWebServiceModuleServicesValues{
			Loaded: module.loaded,
		})
	}

	if a.extensionReg.node != "" || a.extensionReg.requests != 0 || a.extensionReg.hitRate != 0 {
		labels := contexts.ExtensionRegistryLabels{Node: a.extensionReg.node, Server: a.extensionReg.server}
		contexts.ExtensionRegistry.Requests.Set(state, labels, contexts.ExtensionRegistryRequestsValues{
			Requests:      a.extensionReg.requests,
			Hits:          a.extensionReg.hits,
			Displacements: a.extensionReg.displacements,
		})
		contexts.ExtensionRegistry.HitRate.Set(state, labels, contexts.ExtensionRegistryHitRateValues{
			Hit_rate: a.extensionReg.hitRate,
		})
	}

	contexts.SystemData.Usage.Set(state, contexts.EmptyLabels{}, contexts.SystemDataUsageValues{
		Cpu_since_last: a.systemData.cpuUsageSinceLast,
		Free_memory:    a.systemData.freeMemoryBytes,
	})
}

func (a *aggregator) collectJDBCMetricsEnabled() bool {
	if a.cfg.CollectJDBCMetrics != nil {
		return *a.cfg.CollectJDBCMetrics
	}
	return true
}

func (a *aggregator) collectWebAppMetricsEnabled() bool {
	if a.cfg.CollectWebAppMetrics != nil {
		return *a.cfg.CollectWebAppMetrics
	}
	return true
}

func (a *aggregator) collectSessionMetricsEnabled() bool {
	if a.cfg.CollectSessionMetrics != nil {
		return *a.cfg.CollectSessionMetrics
	}
	return true
}

func (a *aggregator) collectDynamicCacheMetricsEnabled() bool {
	if a.cfg.CollectWebAppMetrics != nil {
		return *a.cfg.CollectWebAppMetrics
	}
	return true
}

func (a *aggregator) collectSystemDataEnabled() bool {
	return true
}

func (a *aggregator) collectServletMetricsEnabled() bool {
	if a.cfg.CollectServletMetrics != nil {
		return *a.cfg.CollectServletMetrics
	}
	return true
}

func (a *aggregator) collectJCAMetricsEnabled() bool {
	if a.cfg.CollectJCAMetrics != nil {
		return *a.cfg.CollectJCAMetrics
	}
	return true
}

func (a *aggregator) collectJMSMetricsEnabled() bool {
	if a.cfg.CollectJMSMetrics != nil {
		return *a.cfg.CollectJMSMetrics
	}
	return true
}

func parseCount(value string) (int64, bool) {
	if value == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return v, true
	}
	floatVal, ferr := strconv.ParseFloat(value, 64)
	if ferr != nil {
		return 0, false
	}
	return int64(math.Round(floatVal)), true
}

func parseFloat(value string) (float64, bool) {
	if value == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

const (
	unitBytes        = "BYTES"
	unitMilliseconds = "MILLISECONDS"
	unitSeconds      = "SECONDS"
)

func convertUnits(value int64, unit string, target string) int64 {
	unit = strings.TrimSpace(strings.ToUpper(unit))
	if unit == "" {
		unit = unitBytes
	}
	switch target {
	case unitBytes:
		return convertToBytes(value, unit)
	case unitMilliseconds:
		return convertToMilliseconds(value, unit)
	case unitSeconds:
		return convertToSeconds(value, unit)
	default:
		return value
	}
}

func (a *aggregator) coverageMissing() []string {
	if a.coverage == nil {
		return nil
	}
	return a.coverage.Missing()
}

func convertToBytes(value int64, unit string) int64 {
	switch unit {
	case "BYTE", "BYTES", "N/A", "NONE", "COUNT":
		return value
	case "KILOBYTE", "KB":
		return value * 1024
	case "MEGABYTE", "MB":
		return value * 1024 * 1024
	case "GIGABYTE", "GB":
		return value * 1024 * 1024 * 1024
	default:
		return value
	}
}

func convertToMilliseconds(value int64, unit string) int64 {
	switch unit {
	case "MILLISECOND", "MILLISECONDS":
		return value
	case "SECOND", "SECONDS":
		return value * 1000
	case "MICROSECOND":
		return value / 1000
	default:
		return value
	}
}

func convertToSeconds(value int64, unit string) int64 {
	switch unit {
	case "SECOND", "SECONDS":
		return value
	case "MILLISECOND", "MILLISECONDS":
		return value / 1000
	case "MICROSECOND":
		return value / 1_000_000
	case "MINUTE", "MINUTES":
		return value * 60
	case "HOUR", "HOURS":
		return value * 3600
	default:
		return value
	}
}
