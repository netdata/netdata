// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// AcquisitionObserver receives one terminal report for every selected profile.
// The report and its Routes remain valid after the call; ProfileMetrics is
// borrowed only for the duration of the call.
type AcquisitionObserver interface {
	ObserveProfile(AcquisitionProfileReport, *ddsnmp.ProfileMetrics)
}

type AcquisitionObserverFunc func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics)

func (fn AcquisitionObserverFunc) ObserveProfile(report AcquisitionProfileReport, metrics *ddsnmp.ProfileMetrics) {
	fn(report, metrics)
}

type AcquisitionProfileOutcome uint8

const (
	AcquisitionProfileOutcomeUnknown AcquisitionProfileOutcome = iota
	AcquisitionProfileOutcomeSuccess
	AcquisitionProfileOutcomePartial
	AcquisitionProfileOutcomeFailed
)

type AcquisitionFailurePhase uint8

const (
	AcquisitionFailurePhaseNone AcquisitionFailurePhase = iota
	AcquisitionFailurePhasePrepare
	AcquisitionFailurePhaseTables
)

type AcquisitionRouteKind uint8

const (
	AcquisitionRouteKindUnknown AcquisitionRouteKind = iota
	AcquisitionRouteKindProfileTagScalar
	AcquisitionRouteKindMetadataScalar
	AcquisitionRouteKindMetricScalar
	AcquisitionRouteKindMetricTable
	AcquisitionRouteKindTopologyScalar
	AcquisitionRouteKindTopologyTable
	AcquisitionRouteKindBGPScalar
	AcquisitionRouteKindBGPTable
)

type AcquisitionRouteSource uint8

const (
	AcquisitionRouteSourceNone AcquisitionRouteSource = iota
	AcquisitionRouteSourceGET
	AcquisitionRouteSourceWalk
	AcquisitionRouteSourceCache
)

type AcquisitionRouteOutcome uint8

const (
	AcquisitionRouteOutcomeNotObserved AcquisitionRouteOutcome = iota
	AcquisitionRouteOutcomeMissing
	AcquisitionRouteOutcomeFailed
	AcquisitionRouteOutcomeEmpty
	AcquisitionRouteOutcomeValues
	AcquisitionRouteOutcomeRejected
	AcquisitionRouteOutcomePartial
)

type AcquisitionFailureClass uint8

const (
	AcquisitionFailureClassNone AcquisitionFailureClass = iota
	AcquisitionFailureClassTransport
	AcquisitionFailureClassProcessing
	AcquisitionFailureClassDependency
)

type AcquisitionProfileIdentity struct {
	Ordinal     uint32
	RouteDigest [32]byte
}

type AcquisitionProfileReport struct {
	Identity     AcquisitionProfileIdentity
	Outcome      AcquisitionProfileOutcome
	FailurePhase AcquisitionFailurePhase
	Stats        ddsnmp.CollectionStats
	Routes       []AcquisitionRouteReport
}

// AcquisitionRouteReport contains only the bounded terminal state of one
// configured route. It deliberately contains no profile path, packet, decoded
// value, or error text.
type AcquisitionRouteReport struct {
	Ordinal      uint32
	Kind         AcquisitionRouteKind
	RootOID      string
	Source       AcquisitionRouteSource
	Outcome      AcquisitionRouteOutcome
	FailureClass AcquisitionFailureClass
	Rows         uint64
	Values       uint64
	Rejected     uint64
}

func (r AcquisitionProfileReport) String() string {
	return fmt.Sprintf("profile ordinal=%d digest=%x outcome=%d phase=%d routes=%d",
		r.Identity.Ordinal, r.Identity.RouteDigest, r.Outcome, r.FailurePhase, len(r.Routes))
}

type acquisitionProfilePlan struct {
	identity AcquisitionProfileIdentity
}

type acquisitionProfileCollection struct {
	plan                 acquisitionProfilePlan
	routes               []AcquisitionRouteReport
	metricScalarRoutes   []int
	metricTableRoutes    []int
	topologyScalarRoutes []int
	topologyTableRoutes  []int
	bgpRoutes            []int
	tableBindings        []acquisitionTableBinding
	globalTags           *acquisitionGlobalTagObserver
	metadata             []*acquisitionMetadataObserver
	metadataCursor       int
}

type acquisitionTableBinding struct {
	routeIndex  int
	request     *tableCollectionRequest
	observation *acquisitionTableObservation
}

type acquisitionTableObservation struct {
	rows     uint64
	values   uint64
	rejected uint64
}

type acquisitionScalarObserver struct {
	collection   *acquisitionProfileCollection
	routeIndexes []int
}

type acquisitionGlobalTagObserver struct {
	collection   *acquisitionProfileCollection
	configs      []ddprofiledefinition.GlobalMetricTagConfig
	routeIndexes []int
}

type acquisitionMetadataObserver struct {
	collection *acquisitionProfileCollection
	routes     map[string]acquisitionMetadataRoute
}

type acquisitionMetadataRoute struct {
	routeIndex int
	oids       []string
}

func (o *acquisitionScalarObserver) start(
	configs []ddprofiledefinition.MetricsConfig,
	missingOIDs map[string]bool,
) {
	if o == nil {
		return
	}
	for i, cfg := range configs {
		if !cfg.IsScalar() {
			continue
		}
		route := o.route(i)
		if route == nil {
			continue
		}
		if missingOIDs[trimOID(cfg.Symbol.OID)] {
			route.Outcome = AcquisitionRouteOutcomeMissing
			continue
		}
		route.Source = AcquisitionRouteSourceGET
	}
}

func (o *acquisitionScalarObserver) failUnfinished(class AcquisitionFailureClass) {
	if o == nil {
		return
	}
	for i := range o.routeIndexes {
		route := o.route(i)
		if route == nil || route.Outcome != AcquisitionRouteOutcomeNotObserved || route.Source == AcquisitionRouteSourceNone {
			continue
		}
		route.Outcome = AcquisitionRouteOutcomeFailed
		route.FailureClass = class
	}
}

func (o *acquisitionScalarObserver) rejected(index int) {
	if route := o.route(index); route != nil {
		route.Outcome = AcquisitionRouteOutcomeRejected
		route.FailureClass = AcquisitionFailureClassProcessing
		route.Rejected = 1
	}
}

func (o *acquisitionScalarObserver) value(index int) {
	if route := o.route(index); route != nil {
		route.Outcome = AcquisitionRouteOutcomeValues
		route.Values = 1
	}
}

func (o *acquisitionScalarObserver) empty(index int) {
	if route := o.route(index); route != nil && route.Outcome == AcquisitionRouteOutcomeNotObserved {
		route.Outcome = AcquisitionRouteOutcomeEmpty
	}
}

func (o *acquisitionScalarObserver) route(configIndex int) *AcquisitionRouteReport {
	if o == nil || configIndex < 0 || configIndex >= len(o.routeIndexes) {
		return nil
	}
	return o.collection.route(o.routeIndexes[configIndex])
}

func (o *acquisitionGlobalTagObserver) start(missingOIDs map[string]bool) {
	if o == nil {
		return
	}
	for i, cfg := range o.configs {
		route := o.route(i)
		if route == nil {
			continue
		}
		if missingOIDs[trimOID(cfg.Symbol.OID)] {
			route.Outcome = AcquisitionRouteOutcomeMissing
			continue
		}
		route.Source = AcquisitionRouteSourceGET
	}
}

func (o *acquisitionGlobalTagObserver) failUnfinished(class AcquisitionFailureClass) {
	if o == nil {
		return
	}
	for i := range o.routeIndexes {
		route := o.route(i)
		if route == nil || route.Outcome != AcquisitionRouteOutcomeNotObserved || route.Source == AcquisitionRouteSourceNone {
			continue
		}
		route.Outcome = AcquisitionRouteOutcomeFailed
		route.FailureClass = class
	}
}

func (o *acquisitionGlobalTagObserver) value(configIndex int) {
	setAcquisitionScalarRouteValue(o.route(configIndex))
}

func (o *acquisitionGlobalTagObserver) empty(configIndex int) {
	setAcquisitionScalarRouteEmpty(o.route(configIndex))
}

func (o *acquisitionGlobalTagObserver) rejected(configIndex int) {
	setAcquisitionScalarRouteRejected(o.route(configIndex))
}

func (o *acquisitionGlobalTagObserver) route(configIndex int) *AcquisitionRouteReport {
	if o == nil || configIndex < 0 || configIndex >= len(o.routeIndexes) {
		return nil
	}
	return o.collection.route(o.routeIndexes[configIndex])
}

func (o *acquisitionMetadataObserver) start(missingOIDs map[string]bool) {
	if o == nil {
		return
	}
	for _, binding := range o.routes {
		route := o.collection.route(binding.routeIndex)
		if route == nil {
			continue
		}
		if allAcquisitionOIDsMissing(binding.oids, missingOIDs) {
			route.Outcome = AcquisitionRouteOutcomeMissing
			continue
		}
		route.Source = AcquisitionRouteSourceGET
	}
}

func (o *acquisitionMetadataObserver) failUnfinished(class AcquisitionFailureClass) {
	if o == nil {
		return
	}
	for _, binding := range o.routes {
		route := o.collection.route(binding.routeIndex)
		if route == nil || route.Outcome != AcquisitionRouteOutcomeNotObserved || route.Source == AcquisitionRouteSourceNone {
			continue
		}
		route.Outcome = AcquisitionRouteOutcomeFailed
		route.FailureClass = class
	}
}

func (o *acquisitionMetadataObserver) value(fieldName string) {
	setAcquisitionScalarRouteValue(o.route(fieldName))
}

func (o *acquisitionMetadataObserver) empty(fieldName string) {
	setAcquisitionScalarRouteEmpty(o.route(fieldName))
}

func (o *acquisitionMetadataObserver) rejected(fieldName string) {
	setAcquisitionScalarRouteRejected(o.route(fieldName))
}

func (o *acquisitionMetadataObserver) route(fieldName string) *AcquisitionRouteReport {
	if o == nil {
		return nil
	}
	binding, ok := o.routes[fieldName]
	if !ok {
		return nil
	}
	return o.collection.route(binding.routeIndex)
}

func setAcquisitionScalarRouteValue(route *AcquisitionRouteReport) {
	if route == nil || route.Outcome == AcquisitionRouteOutcomeMissing || route.Outcome == AcquisitionRouteOutcomeFailed {
		return
	}
	route.Values = 1
	if route.Rejected > 0 {
		route.Outcome = AcquisitionRouteOutcomePartial
		route.FailureClass = AcquisitionFailureClassProcessing
	} else {
		route.Outcome = AcquisitionRouteOutcomeValues
	}
}

func setAcquisitionScalarRouteEmpty(route *AcquisitionRouteReport) {
	if route == nil || route.Outcome != AcquisitionRouteOutcomeNotObserved {
		return
	}
	route.Outcome = AcquisitionRouteOutcomeEmpty
}

func setAcquisitionScalarRouteRejected(route *AcquisitionRouteReport) {
	if route == nil || route.Outcome == AcquisitionRouteOutcomeMissing || route.Outcome == AcquisitionRouteOutcomeFailed {
		return
	}
	route.Rejected++
	if route.Values > 0 {
		route.Outcome = AcquisitionRouteOutcomePartial
	} else {
		route.Outcome = AcquisitionRouteOutcomeRejected
	}
	route.FailureClass = AcquisitionFailureClassProcessing
}

func allAcquisitionOIDsMissing(oids []string, missingOIDs map[string]bool) bool {
	if len(oids) == 0 {
		return false
	}
	for _, oid := range oids {
		if !missingOIDs[oid] {
			return false
		}
	}
	return true
}

func buildAcquisitionProfilePlan(profile *ddsnmp.Profile, ordinal uint32) acquisitionProfilePlan {
	return acquisitionProfilePlan{identity: AcquisitionProfileIdentity{
		Ordinal:     ordinal,
		RouteDigest: acquisitionProfileRouteDigest(profile),
	}}
}

func newAcquisitionProfileCollection(
	plan acquisitionProfilePlan,
	profile *ddsnmp.Profile,
	sysObjectID string,
) *acquisitionProfileCollection {
	collection := &acquisitionProfileCollection{plan: plan}
	if profile == nil || profile.Definition == nil {
		return collection
	}
	def := profile.Definition
	collection.metricScalarRoutes, collection.metricTableRoutes = collection.addMetricRoutes(
		def.Metrics,
		AcquisitionRouteKindMetricScalar,
		AcquisitionRouteKindMetricTable,
	)

	topologyMetrics := make([]ddprofiledefinition.MetricsConfig, 0, len(def.Topology))
	for _, cfg := range def.Topology {
		topologyMetrics = append(topologyMetrics, cfg.MetricsConfig)
	}
	collection.topologyScalarRoutes, collection.topologyTableRoutes = collection.addMetricRoutes(
		topologyMetrics,
		AcquisitionRouteKindTopologyScalar,
		AcquisitionRouteKindTopologyTable,
	)

	collection.bgpRoutes = make([]int, len(def.BGP))
	for i := range collection.bgpRoutes {
		collection.bgpRoutes[i] = -1
	}
	for i, cfg := range def.BGP {
		if cfg.Table.OID == "" {
			collection.bgpRoutes[i] = collection.addRoute(AcquisitionRouteKindBGPScalar, firstBGPRouteOID(cfg))
		}
	}
	for i, cfg := range def.BGP {
		if cfg.Table.OID != "" {
			collection.bgpRoutes[i] = collection.addRoute(AcquisitionRouteKindBGPTable, trimOID(cfg.Table.OID))
		}
	}
	collection.prepareProfileInputRoutes(profile, sysObjectID)
	return collection
}

func (c *acquisitionProfileCollection) prepareProfileInputRoutes(profile *ddsnmp.Profile, sysObjectID string) {
	if c == nil || profile == nil || profile.Definition == nil {
		return
	}
	def := profile.Definition
	c.globalTags = c.newGlobalTagObserver(def.MetricTags)
	if sysObjectID != "" {
		for _, entry := range def.SysobjectIDMetadata {
			if ddprofiledefinition.SelectorOidMatches(sysObjectID, entry.SysobjectID) {
				c.metadata = append(c.metadata, c.newMetadataObserver(entry.Metadata))
			}
		}
	}
	if cfg, ok := def.Metadata[ddprofiledefinition.MetadataDeviceResource]; ok {
		c.metadata = append(c.metadata, c.newMetadataObserver(cfg.Fields))
	}
}

func (c *acquisitionProfileCollection) addMetricRoutes(
	configs []ddprofiledefinition.MetricsConfig,
	scalarKind, tableKind AcquisitionRouteKind,
) (scalarRoutes, tableRoutes []int) {
	scalarRoutes = make([]int, len(configs))
	for i := range scalarRoutes {
		scalarRoutes[i] = -1
	}
	for i, cfg := range configs {
		if cfg.IsScalar() {
			scalarRoutes[i] = c.addRoute(scalarKind, trimOID(cfg.Symbol.OID))
		}
	}
	for _, cfg := range configs {
		if cfg.IsColumn() {
			tableRoutes = append(tableRoutes, c.addRoute(tableKind, trimOID(cfg.Table.OID)))
		}
	}
	return scalarRoutes, tableRoutes
}

func (c *acquisitionProfileCollection) addRoute(kind AcquisitionRouteKind, rootOID string) int {
	index := len(c.routes)
	c.routes = append(c.routes, AcquisitionRouteReport{
		Ordinal: uint32(index),
		Kind:    kind,
		RootOID: rootOID,
		Outcome: AcquisitionRouteOutcomeNotObserved,
	})
	return index
}

func (c *acquisitionProfileCollection) route(index int) *AcquisitionRouteReport {
	if c == nil || index < 0 || index >= len(c.routes) {
		return nil
	}
	return &c.routes[index]
}

func (c *acquisitionProfileCollection) bgpRoute(configIndex int) *AcquisitionRouteReport {
	if c == nil || configIndex < 0 || configIndex >= len(c.bgpRoutes) {
		return nil
	}
	return c.route(c.bgpRoutes[configIndex])
}

func (c *acquisitionProfileCollection) metricScalarObserver() *acquisitionScalarObserver {
	if c == nil {
		return nil
	}
	return &acquisitionScalarObserver{collection: c, routeIndexes: c.metricScalarRoutes}
}

func (c *acquisitionProfileCollection) topologyScalarObserver() *acquisitionScalarObserver {
	if c == nil {
		return nil
	}
	return &acquisitionScalarObserver{collection: c, routeIndexes: c.topologyScalarRoutes}
}

func (c *acquisitionProfileCollection) globalTagObserver(
	configs []ddprofiledefinition.GlobalMetricTagConfig,
) *acquisitionGlobalTagObserver {
	if c == nil {
		return nil
	}
	if c.globalTags == nil {
		c.globalTags = c.newGlobalTagObserver(configs)
	}
	return c.globalTags
}

func (c *acquisitionProfileCollection) newGlobalTagObserver(
	configs []ddprofiledefinition.GlobalMetricTagConfig,
) *acquisitionGlobalTagObserver {
	if len(configs) == 0 {
		return nil
	}
	observer := &acquisitionGlobalTagObserver{
		collection:   c,
		configs:      configs,
		routeIndexes: make([]int, len(configs)),
	}
	for i := range observer.routeIndexes {
		observer.routeIndexes[i] = -1
	}
	for i, cfg := range configs {
		if oid := trimOID(cfg.Symbol.OID); oid != "" {
			observer.routeIndexes[i] = c.addRoute(AcquisitionRouteKindProfileTagScalar, oid)
		}
	}
	return observer
}

func (c *acquisitionProfileCollection) metadataObserver(
	fields map[string]ddprofiledefinition.MetadataField,
) *acquisitionMetadataObserver {
	if c == nil {
		return nil
	}
	if c.metadataCursor < len(c.metadata) {
		observer := c.metadata[c.metadataCursor]
		c.metadataCursor++
		return observer
	}
	return c.newMetadataObserver(fields)
}

func (c *acquisitionProfileCollection) newMetadataObserver(
	fields map[string]ddprofiledefinition.MetadataField,
) *acquisitionMetadataObserver {
	var observer *acquisitionMetadataObserver
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		oids := acquisitionMetadataFieldOIDs(fields[name])
		if len(oids) == 0 {
			continue
		}
		if observer == nil {
			observer = &acquisitionMetadataObserver{
				collection: c,
				routes:     make(map[string]acquisitionMetadataRoute),
			}
		}
		observer.routes[name] = acquisitionMetadataRoute{
			routeIndex: c.addRoute(AcquisitionRouteKindMetadataScalar, oids[0]),
			oids:       oids,
		}
	}
	return observer
}

func acquisitionMetadataFieldOIDs(field ddprofiledefinition.MetadataField) []string {
	if field.Value != "" {
		return nil
	}
	if oid := trimOID(field.Symbol.OID); oid != "" {
		return []string{oid}
	}
	oids := make([]string, 0, len(field.Symbols))
	for _, symbol := range field.Symbols {
		if oid := trimOID(symbol.OID); oid != "" {
			oids = append(oids, oid)
		}
	}
	return oids
}

func (c *acquisitionProfileCollection) profileInputRoutes() []AcquisitionRouteReport {
	if c == nil {
		return nil
	}
	routes := make([]AcquisitionRouteReport, 0)
	for _, route := range c.routes {
		if isAcquisitionProfileInputRoute(route.Kind) {
			routes = append(routes, route)
		}
	}
	return routes
}

func (c *acquisitionProfileCollection) restoreProfileInputRoutes(cached []AcquisitionRouteReport) {
	if c == nil {
		return
	}
	for _, route := range cached {
		current := c.route(int(route.Ordinal))
		if current == nil || current.Kind != route.Kind || current.RootOID != route.RootOID ||
			!isAcquisitionProfileInputRoute(route.Kind) {
			continue
		}
		*current = route
		if current.Outcome != AcquisitionRouteOutcomeNotObserved {
			current.Source = AcquisitionRouteSourceCache
		}
	}
}

func isAcquisitionProfileInputRoute(kind AcquisitionRouteKind) bool {
	return kind == AcquisitionRouteKindProfileTagScalar || kind == AcquisitionRouteKindMetadataScalar
}

func (c *acquisitionProfileCollection) bindTableRoutes(scope *tableCollectionScope, routeIndexes []int) {
	if c == nil || scope == nil {
		return
	}
	for i, request := range scope.requests {
		if i >= len(routeIndexes) {
			break
		}
		if scope.acquisition == nil {
			scope.acquisition = make(map[*tableCollectionRequest]*acquisitionTableObservation, len(scope.requests))
		}
		observation := &acquisitionTableObservation{}
		scope.acquisition[request] = observation
		c.tableBindings = append(c.tableBindings, acquisitionTableBinding{
			routeIndex:  routeIndexes[i],
			request:     request,
			observation: observation,
		})
	}
}

func (c *acquisitionProfileCollection) syncTableRoutes() {
	if c == nil {
		return
	}
	for _, binding := range c.tableBindings {
		route := c.route(binding.routeIndex)
		request := binding.request
		observation := binding.observation
		if route == nil || request == nil || observation == nil {
			continue
		}
		switch {
		case request.missing:
			route.Outcome = AcquisitionRouteOutcomeMissing
		case request.route == nil:
			route.Outcome = AcquisitionRouteOutcomeNotObserved
		case request.route.state == tableRouteFailed:
			route.Source = AcquisitionRouteSourceWalk
			route.Outcome = AcquisitionRouteOutcomeFailed
			route.FailureClass = AcquisitionFailureClassTransport
		case request.route.state == tableRouteCached:
			route.Source = AcquisitionRouteSourceCache
			setAcquisitionRouteCounts(route, uint64(request.candidateStats.Metrics.Rows), uint64(len(request.candidateMetrics)),
				uint64(request.candidateStats.Errors.Processing.Table))
		case request.route.state == tableRouteFresh:
			route.Source = AcquisitionRouteSourceWalk
			setAcquisitionRouteCounts(route, observation.rows, observation.values, observation.rejected)
		}
	}
}

func setAcquisitionRouteCounts(route *AcquisitionRouteReport, rows, values, rejected uint64) {
	route.Rows = rows
	route.Values = values
	route.Rejected = rejected
	switch {
	case rejected > 0 && values > 0:
		route.Outcome = AcquisitionRouteOutcomePartial
		route.FailureClass = AcquisitionFailureClassProcessing
	case rejected > 0:
		route.Outcome = AcquisitionRouteOutcomeRejected
		route.FailureClass = AcquisitionFailureClassProcessing
	case values > 0:
		route.Outcome = AcquisitionRouteOutcomeValues
	default:
		route.Outcome = AcquisitionRouteOutcomeEmpty
	}
}

func (c *acquisitionProfileCollection) report(
	outcome AcquisitionProfileOutcome,
	phase AcquisitionFailurePhase,
	metrics *ddsnmp.ProfileMetrics,
) AcquisitionProfileReport {
	report := AcquisitionProfileReport{
		Identity:     c.plan.identity,
		Outcome:      outcome,
		FailurePhase: phase,
		Routes:       c.routes,
	}
	if metrics != nil {
		report.Stats = metrics.Stats
	}
	if outcome == AcquisitionProfileOutcomeSuccess && routesHaveFailures(report.Routes) {
		report.Outcome = AcquisitionProfileOutcomePartial
	}
	return report
}

func routesHaveFailures(routes []AcquisitionRouteReport) bool {
	for _, route := range routes {
		switch route.Outcome {
		case AcquisitionRouteOutcomeFailed, AcquisitionRouteOutcomeRejected, AcquisitionRouteOutcomePartial:
			return true
		}
	}
	return false
}

func firstBGPRouteOID(cfg ddprofiledefinition.BGPConfig) string {
	var oids []string
	forEachBGPValue(cfg, func(value ddprofiledefinition.BGPValueConfig) {
		if oid := trimOID(bgpValueSymbol(value).OID); oid != "" {
			oids = append(oids, oid)
		}
	})
	for _, tag := range cfg.MetricTags {
		if oid := trimOID(tag.Symbol.OID); oid != "" {
			oids = append(oids, oid)
		}
	}
	if len(oids) == 0 {
		return ""
	}
	sort.Strings(oids)
	return oids[0]
}

func acquisitionProfileRouteDigest(profile *ddsnmp.Profile) [32]byte {
	d := routeDigestWriter{hash: sha256.New()}
	d.add("netdata.snmp-acquisition-profile.v1")
	if profile == nil || profile.Definition == nil {
		return d.sum()
	}
	def := profile.Definition

	for _, cfg := range def.Metrics {
		d.addMetric("metric", cfg)
	}
	for _, cfg := range def.Topology {
		d.addMetric("topology:"+string(cfg.Kind), cfg.MetricsConfig)
	}
	for _, cfg := range def.MetricTags {
		d.add("global-tag")
		d.addMetricTag(cfg.MetricTagConfig)
	}
	d.addMetadata(def.Metadata)
	for _, entry := range def.SysobjectIDMetadata {
		d.add("sysobjectid-metadata")
		d.add(entry.SysobjectID)
		d.addMetadataFields(entry.Metadata)
	}
	for _, row := range def.BGP {
		d.add("bgp")
		d.add(string(row.Kind))
		d.add(row.Table.OID)
		ddprofiledefinition.ForEachBGPSignalValue(row, func(path string, value ddprofiledefinition.BGPValueConfig) {
			d.add(path)
			d.add(value.From)
			d.add(value.Table)
			d.add(value.OID)
			d.add(value.Symbol.OID)
			d.add(value.LookupSymbol.OID)
		})
		for _, tag := range row.MetricTags {
			d.addMetricTag(tag)
		}
	}
	return d.sum()
}

type routeDigestWriter struct {
	hash hash.Hash
	buf  [8]byte
}

func (d *routeDigestWriter) add(value string) {
	binary.LittleEndian.PutUint64(d.buf[:], uint64(len(value)))
	_, _ = d.hash.Write(d.buf[:])
	_, _ = d.hash.Write([]byte(value))
}

func (d *routeDigestWriter) addMetric(kind string, cfg ddprofiledefinition.MetricsConfig) {
	d.add(kind)
	d.add(cfg.Table.OID)
	d.add(cfg.Symbol.OID)
	for _, symbol := range cfg.Symbols {
		d.add(symbol.OID)
	}
	for _, tag := range cfg.MetricTags {
		d.addMetricTag(tag)
	}
}

func (d *routeDigestWriter) addMetricTag(tag ddprofiledefinition.MetricTagConfig) {
	d.add(tag.Table)
	d.add(tag.Column.OID)
	d.add(tag.Symbol.OID)
	d.add(tag.LookupSymbol.OID)
}

func (d *routeDigestWriter) addMetadata(metadata ddprofiledefinition.MetadataConfig) {
	resources := make([]string, 0, len(metadata))
	for resource := range metadata {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	for _, resource := range resources {
		d.add("metadata")
		d.add(resource)
		cfg := metadata[resource]
		d.addMetadataFields(cfg.Fields)
		for _, tag := range cfg.IDTags {
			d.addMetricTag(tag)
		}
	}
}

func (d *routeDigestWriter) addMetadataFields(fields map[string]ddprofiledefinition.MetadataField) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := fields[name]
		d.add(name)
		d.add(field.Symbol.OID)
		for _, symbol := range field.Symbols {
			d.add(symbol.OID)
		}
	}
}

func (d *routeDigestWriter) sum() (result [32]byte) {
	copy(result[:], d.hash.Sum(nil))
	return result
}
