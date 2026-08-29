// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type tableRouteState uint8

const (
	tableRoutePending tableRouteState = iota
	tableRouteNeedsFresh
	tableRouteCached
	tableRouteFresh
	tableRouteFailed
)

// tableCollectionSession resolves every table route before any scope emits
// metrics. A route therefore has one authoritative cached, fresh, or failed
// outcome for the entire device collection.
type tableCollectionSession struct {
	collector *tableCollector
	identity  *tableIdentity
	scopes    []*tableCollectionScope
	routes    map[string]*tableCollectionRoute
	graph     tableRouteGraph
	freshData map[string]map[string]gosnmp.SnmpPDU
	resolved  bool
}

type tableCollectionScope struct {
	profileSource  string
	mode           tableSymbolMode
	stats          *ddsnmp.CollectionStats
	requests       []*tableCollectionRequest
	tableNameToOID map[string]string
	acquisition    map[*tableCollectionRequest]*acquisitionTableObservation
}

type tableCollectionRequest struct {
	scope  *tableCollectionScope
	config ddprofiledefinition.MetricsConfig
	route  *tableCollectionRoute

	missing          bool
	cacheEligible    bool
	cacheHit         bool
	sharesRouteCache bool
	candidateMetrics []ddsnmp.Metric
	candidateStats   ddsnmp.CollectionStats
}

type tableCollectionRoute struct {
	oid                    string
	requests               []*tableCollectionRequest
	state                  tableRouteState
	pdus                   map[string]gosnmp.SnmpPDU
	err                    error
	acquisitionValueCounts map[string]uint64
}

type freshRouteWork struct {
	route   *tableCollectionRoute
	trigger *tableCollectionRequest
}

type tableRouteGraph struct {
	// Forward edges stay request-scoped because only rows eligible for that
	// request activate its dependencies. Reverse edges are route-scoped because
	// any current dependency outcome invalidates every cached tag snapshot.
	dependenciesByRequest map[*tableCollectionRequest][]*tableCollectionRoute
	dependentsByRoute     map[*tableCollectionRoute]map[string]*tableCollectionRoute
}

func (g *tableRouteGraph) addDependency(req *tableCollectionRequest, dependency *tableCollectionRoute) {
	if g.dependenciesByRequest == nil {
		g.dependenciesByRequest = make(map[*tableCollectionRequest][]*tableCollectionRoute)
	}
	g.dependenciesByRequest[req] = append(g.dependenciesByRequest[req], dependency)

	if g.dependentsByRoute == nil {
		g.dependentsByRoute = make(map[*tableCollectionRoute]map[string]*tableCollectionRoute)
	}
	if g.dependentsByRoute[dependency] == nil {
		g.dependentsByRoute[dependency] = make(map[string]*tableCollectionRoute)
	}
	g.dependentsByRoute[dependency][req.route.oid] = req.route
}

func (g *tableRouteGraph) dependencies(req *tableCollectionRequest) []*tableCollectionRoute {
	return g.dependenciesByRequest[req]
}

func (g *tableRouteGraph) dependents(route *tableCollectionRoute) []*tableCollectionRoute {
	byOID := g.dependentsByRoute[route]
	if len(byOID) == 0 {
		return nil
	}
	keys := make([]string, 0, len(byOID))
	for oid := range byOID {
		keys = append(keys, oid)
	}
	sort.Strings(keys)

	result := make([]*tableCollectionRoute, 0, len(keys))
	for _, oid := range keys {
		result = append(result, byOID[oid])
	}
	return result
}

func newTableCollectionSession(collector *tableCollector, identity *tableIdentity) *tableCollectionSession {
	if identity == nil {
		identity = emptyTableIdentity
	}
	return &tableCollectionSession{
		collector: collector,
		identity:  identity,
		routes:    make(map[string]*tableCollectionRoute),
		freshData: make(map[string]map[string]gosnmp.SnmpPDU),
	}
}

func (s *tableCollectionSession) addScope(
	prof *ddsnmp.Profile,
	mode tableSymbolMode,
	stats *ddsnmp.CollectionStats,
) *tableCollectionScope {
	return s.addObservedScope(prof, mode, stats, nil)
}

func (s *tableCollectionSession) addObservedScope(
	prof *ddsnmp.Profile,
	mode tableSymbolMode,
	stats *ddsnmp.CollectionStats,
	acquisition *acquisitionTopologyTableScope,
) *tableCollectionScope {
	scope := &tableCollectionScope{
		profileSource:  prof.SourceFile,
		mode:           mode,
		stats:          stats,
		tableNameToOID: make(map[string]string),
	}
	s.scopes = append(s.scopes, scope)

	if prof.Definition == nil {
		return scope
	}
	for configIndex, cfg := range prof.Definition.Metrics {
		if cfg.IsScalar() || cfg.Table.OID == "" {
			continue
		}
		if cfg.Table.Name != "" {
			scope.tableNameToOID[cfg.Table.Name] = cfg.Table.OID
		}
		request := &tableCollectionRequest{
			scope:         scope,
			config:        cfg,
			cacheEligible: mode == tableSymbolModeValue,
		}
		if observation := acquisition.bind(configIndex, request); observation != nil {
			if scope.acquisition == nil {
				scope.acquisition = make(map[*tableCollectionRequest]*acquisitionTableObservation)
			}
			scope.acquisition[request] = observation
		}
		scope.requests = append(scope.requests, request)
	}
	return scope
}

func (s *tableCollectionSession) resolve() {
	if s.resolved {
		return
	}
	s.resolved = true

	s.buildRoutes()
	s.buildRouteDependencies()
	routes := s.sortedRoutes()
	var freshQueue []freshRouteWork
	for _, route := range routes {
		if routeNeedsFresh(route) {
			s.requireFresh(route, freshTriggerForRoute(route), &freshQueue)
		}
	}
	s.resolveFreshQueue(&freshQueue)
	for _, route := range routes {
		if route.state != tableRoutePending {
			continue
		}
		if trigger := s.stageCached(route); trigger != nil {
			s.requireFresh(route, trigger, &freshQueue)
			s.resolveFreshQueue(&freshQueue)
		} else {
			route.state = tableRouteCached
		}
	}
	s.discardSettledRouteCaches(routes)
	s.finalizeStats()
}

func (s *tableCollectionSession) discardSettledRouteCaches(routes []*tableCollectionRoute) {
	firstDiscard := -1
	for i, route := range routes {
		if route.state == tableRouteFresh || route.state == tableRouteFailed {
			firstDiscard = i
			break
		}
	}
	if firstDiscard < 0 || !s.collector.tableCache.enabled() {
		return
	}

	tableOIDs := make([]string, 0, len(routes)-firstDiscard)
	for _, route := range routes[firstDiscard:] {
		if route.state == tableRouteFresh || route.state == tableRouteFailed {
			tableOIDs = append(tableOIDs, route.oid)
		}
	}
	// A previous snapshot is valid only when the route stayed cached. Fresh
	// results repopulate their routes later if their scope consumes them.
	s.collector.tableCache.discardTables(tableOIDs)
}

func (s *tableCollectionSession) buildRoutes() {
	canonicalByName := s.identity.canonicalByName

	for _, scope := range s.scopes {
		for name, tableOID := range scope.tableNameToOID {
			if canonicalOID, ok := canonicalByName[name]; ok && isDescendantTableOID(tableOID, canonicalOID) {
				scope.tableNameToOID[name] = canonicalOID
			}
		}

		for _, req := range scope.requests {
			routeOID := req.config.Table.OID
			if len(req.config.Symbols) == 0 {
				if canonicalOID, ok := canonicalByName[req.config.Table.Name]; ok &&
					isDescendantTableOID(routeOID, canonicalOID) {
					routeOID = canonicalOID
					req.sharesRouteCache = true
				}
			}

			if s.collector.missingOIDs[trimOID(req.config.Table.OID)] {
				req.missing = true
				req.scope.stats.Errors.MissingOIDs++
				continue
			}

			route := s.routes[routeOID]
			if route == nil {
				route = &tableCollectionRoute{oid: routeOID}
				s.routes[routeOID] = route
			}
			req.route = route
			if req.cacheEligible && !req.sharesRouteCache {
				req.cacheHit = s.collector.tableCache.isConfigCached(req.config)
			}
			route.requests = append(route.requests, req)
		}
	}
}

func (s *tableCollectionSession) buildRouteDependencies() {
	for _, route := range s.sortedRoutes() {
		for _, req := range route.requests {
			for _, dependencyOID := range extractTableDependencies(req.config, req.scope.tableNameToOID) {
				dependency := s.routes[dependencyOID]
				if dependency == nil || dependency == route {
					continue
				}
				s.graph.addDependency(req, dependency)
			}
		}
	}
}

func isDescendantTableOID(tableOID, canonicalOID string) bool {
	tableOID = trimOID(tableOID)
	canonicalOID = trimOID(canonicalOID)
	return tableOID == canonicalOID || strings.HasPrefix(tableOID, canonicalOID+".")
}

func (s *tableCollectionSession) sortedRoutes() []*tableCollectionRoute {
	keys := make([]string, 0, len(s.routes))
	for tableOID := range s.routes {
		keys = append(keys, tableOID)
	}
	sort.Strings(keys)

	routes := make([]*tableCollectionRoute, 0, len(keys))
	for _, tableOID := range keys {
		routes = append(routes, s.routes[tableOID])
	}
	return routes
}

func routeNeedsFresh(route *tableCollectionRoute) bool {
	hasCacheOwner := false
	for _, req := range route.requests {
		if !req.cacheEligible {
			return true
		}
		if req.sharesRouteCache {
			continue
		}
		hasCacheOwner = true
		if !req.cacheHit {
			return true
		}
	}
	return !hasCacheOwner
}

func freshTriggerForRoute(route *tableCollectionRoute) *tableCollectionRequest {
	var cacheIneligible *tableCollectionRequest
	var cacheOwner *tableCollectionRequest
	for _, req := range route.requests {
		if !req.cacheEligible {
			if cacheIneligible == nil {
				cacheIneligible = req
			}
			continue
		}
		if req.sharesRouteCache {
			continue
		}
		if cacheOwner == nil {
			cacheOwner = req
		}
		if !req.cacheHit {
			if cacheIneligible == nil {
				return req
			}
		}
	}
	if cacheIneligible != nil {
		return cacheIneligible
	}
	if cacheOwner != nil {
		return cacheOwner
	}
	return route.requests[0]
}

func (s *tableCollectionSession) stageCached(route *tableCollectionRoute) *tableCollectionRequest {
	for _, req := range route.requests {
		if !req.cacheEligible || len(req.config.Symbols) == 0 {
			continue
		}

		started := time.Now()
		req.candidateMetrics = s.collector.tryCollectFromCache(
			req.config,
			req.scope.profileSource,
			req.scope.mode,
			&req.candidateStats,
			req.scope.acquisition[req],
		)
		req.candidateStats.Timing.Table += time.Since(started)
		if req.candidateMetrics == nil {
			return req
		}
	}
	return nil
}

func (s *tableCollectionSession) requireFresh(
	route *tableCollectionRoute,
	trigger *tableCollectionRequest,
	queue *[]freshRouteWork,
) {
	switch route.state {
	case tableRouteNeedsFresh, tableRouteFresh, tableRouteFailed:
		return
	}

	route.state = tableRouteNeedsFresh
	if trigger == nil {
		trigger = freshTriggerForRoute(route)
	}
	*queue = append(*queue, freshRouteWork{route: route, trigger: trigger})
}

func (s *tableCollectionSession) resolveFreshQueue(queue *[]freshRouteWork) {
	for len(*queue) > 0 {
		work := (*queue)[0]
		*queue = (*queue)[1:]
		route := work.route
		if route.state != tableRouteNeedsFresh {
			continue
		}
		// Cached dependents contain tags derived from this route. Settle them to
		// current data before any scope can emit the staged cache candidates.
		for _, dependent := range s.graph.dependents(route) {
			s.requireFresh(dependent, freshTriggerForRoute(dependent), queue)
		}
		s.resolveFreshRoute(route, work.trigger)
		if route.state != tableRouteFresh {
			continue
		}

		for _, req := range route.requests {
			if len(req.config.Symbols) == 0 || !tableHasEligibleRows(req.config, route.pdus) {
				continue
			}
			for _, dependency := range s.graph.dependencies(req) {
				s.requireFresh(dependency, freshTriggerForRoute(dependency), queue)
			}
		}
	}
}

func (s *tableCollectionSession) resolveFreshRoute(route *tableCollectionRoute, trigger *tableCollectionRequest) {
	started := time.Now()
	pdus, err := walkTableWithStats(s.collector, route.oid, trigger.scope.stats)
	trigger.scope.stats.Timing.Table += time.Since(started)
	if err != nil {
		route.state = tableRouteFailed
		route.err = fmt.Errorf("failed to walk table OID '%s': %w", route.oid, err)
		return
	}

	route.state = tableRouteFresh
	route.pdus = pdus
	s.freshData[route.oid] = pdus
}

func tableHasEligibleRows(cfg ddprofiledefinition.MetricsConfig, pdus map[string]gosnmp.SnmpPDU) bool {
	columnOIDs := buildColumnOIDs(cfg)
	for oid := range pdus {
		for columnOID := range columnOIDs {
			if strings.HasPrefix(oid, columnOID+".") {
				return true
			}
		}
	}
	return false
}

func (s *tableCollectionSession) finalizeStats() {
	for _, scope := range s.scopes {
		for _, req := range scope.requests {
			if req.missing || req.route == nil || !req.cacheEligible {
				continue
			}

			mergeTableCollectionStats(req.scope.stats, &req.candidateStats, req.route.state == tableRouteCached)
			if req.route.state == tableRouteCached {
				req.scope.stats.TableCache.Hits++
				req.scope.stats.SNMP.TablesCached++
			} else {
				req.scope.stats.TableCache.Misses++
			}
		}
	}
}

func mergeTableCollectionStats(dst, src *ddsnmp.CollectionStats, includeRows bool) {
	dst.Timing.Table += src.Timing.Table
	dst.SNMP.GetRequests += src.SNMP.GetRequests
	dst.SNMP.GetOIDs += src.SNMP.GetOIDs
	dst.SNMP.WalkRequests += src.SNMP.WalkRequests
	dst.SNMP.WalkPDUs += src.SNMP.WalkPDUs
	dst.SNMP.TablesWalked += src.SNMP.TablesWalked
	dst.SNMP.TablesCached += src.SNMP.TablesCached
	if includeRows {
		dst.Metrics.Rows += src.Metrics.Rows
	}
	dst.Errors.SNMP += src.Errors.SNMP
	dst.Errors.Processing.Table += src.Errors.Processing.Table
	dst.Errors.MissingOIDs += src.Errors.MissingOIDs
}

func (s *tableCollectionSession) collectScope(scope *tableCollectionScope) ([]ddsnmp.Metric, error) {
	if !s.resolved {
		s.resolve()
	}
	if scope == nil {
		return nil, nil
	}

	collectionCtx := &tableCollectionContext{
		walkedData:     s.freshData,
		tableNameToOID: scope.tableNameToOID,
	}

	var metrics []ddsnmp.Metric
	var errs []error
	successful := false
	tablesSeen := make(map[string]bool)
	failedSeen := make(map[string]bool)

	for _, req := range scope.requests {
		acquisition := scope.acquisition[req]
		if req.missing || req.route == nil {
			continue
		}

		switch req.route.state {
		case tableRouteCached:
			tablesSeen[req.route.oid] = true
			if acquisition != nil {
				acquisition.processed = true
			}
			if len(req.config.Symbols) > 0 {
				successful = true
				metrics = append(metrics, req.candidateMetrics...)
			}
		case tableRouteFresh:
			tablesSeen[req.route.oid] = true
			// A current WALK is successful even when it is empty or auxiliary-only.
			successful = true
			if len(req.config.Symbols) == 0 {
				if acquisition != nil {
					acquisition.processed = true
					acquisition.values = req.route.acquisitionValuesWithin(req.config.Table.OID)
				}
				if req.cacheEligible && !req.sharesRouteCache {
					s.collector.tableCache.cacheMarker(req.config)
				}
				continue
			}
			ctx := &tableProcessingContext{
				config:         req.config,
				pdus:           req.route.pdus,
				walkedData:     s.freshData,
				tableNameToOID: scope.tableNameToOID,
				symbolMode:     scope.mode,
				cacheStructure: req.cacheEligible,
				acquisition:    acquisition,
			}
			if acquisition != nil {
				acquisition.processed = true
				ctx.rejected = &acquisition.rejected
				ctx.dependencyRejected = &acquisition.dependencyRejected
			}
			processingErrorsBefore := scope.stats.Errors.Processing.Table
			tableMetrics, err := s.collector.processTableData(ctx, collectionCtx, scope.stats)
			if acquisition != nil {
				acquisition.rows = uint64(len(ctx.rows))
				acquisition.values = uint64(len(tableMetrics))
				acquisition.rejected += uint64(scope.stats.Errors.Processing.Table - processingErrorsBefore)
			}
			scope.stats.Metrics.Rows += int64(len(ctx.rows))
			if err != nil {
				scope.stats.Errors.Processing.Table++
				errs = append(errs, fmt.Errorf("table '%s': %w", req.config.Table.Name, err))
				continue
			}
			metrics = append(metrics, tableMetrics...)
		case tableRouteFailed:
			if !failedSeen[req.route.oid] {
				failedSeen[req.route.oid] = true
				errs = append(errs, req.route.err)
			}
		}
	}
	scope.stats.Metrics.Tables = int64(len(tablesSeen))

	if len(errs) > 0 && !successful {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		s.collector.log.Limit(tableRowsPartialErrorLogKey+scope.profileSource, 1, tableRowsErrorLogEvery).
			Warningf("failed to walk some SNMP tables: %v", errors.Join(errs...))
	}
	return metrics, nil
}

func (r *tableCollectionRoute) acquisitionValuesWithin(rootOID string) uint64 {
	if r == nil {
		return 0
	}
	if r.acquisitionValueCounts == nil {
		r.acquisitionValueCounts = make(map[string]uint64)
		for _, req := range r.requests {
			if len(req.config.Symbols) != 0 || req.scope.acquisition[req] == nil {
				continue
			}
			if oid := trimOID(req.config.Table.OID); oid != "" {
				r.acquisitionValueCounts[oid] = 0
			}
		}
		for oid := range r.pdus {
			for oid = trimOID(oid); oid != ""; {
				if _, ok := r.acquisitionValueCounts[oid]; ok {
					r.acquisitionValueCounts[oid]++
				}
				pos := strings.LastIndexByte(oid, '.')
				if pos < 0 {
					break
				}
				oid = oid[:pos]
			}
		}
	}
	return r.acquisitionValueCounts[trimOID(rootOID)]
}
