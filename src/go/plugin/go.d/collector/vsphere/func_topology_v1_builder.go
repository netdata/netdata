// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

type vsphereActorDetail struct {
	objectType          string
	parentObjectType    string
	parentID            string
	moid                string
	name                string
	datacenter          string
	cluster             string
	host                string
	resourcePool        string
	connectionState     string
	powerState          string
	overallStatus       string
	accessible          any
	inMaintenanceMode   any
	maintenanceMode     string
	datastoreType       string
	networkType         string
	ipPoolName          string
	storageDRSEnabled   any
	multipleHostAccess  any
	snapshotCount       any
	snapshotChainDepth  any
	configuredVCPUs     any
	configuredMemoryMiB any
	capacityBytes       any
	freeSpaceBytes      any
	uncommittedBytes    any
	cpuCapacityMHz      any
	memoryCapacityMiB   any
	cpuLimitMHz         any
	memoryLimitMiB      any
	cpuReservation      any
	memReservation      any
	networkHosts        any
	networkVMs          any
	drsEnabled          any
	haEnabled           any
	vsanEnabled         any
	consolidationNeeded any
	toolsRunningStatus  string
	toolsVersionStatus  string
}

type vsphereTopologyBuilder struct {
	strings          *topologyv1.StringDictionary
	actorIndexes     map[string]int
	actorObjectTypes map[string]string
	actors           *topologyv1.TableBuilder
	links            *topologyv1.TableBuilder
	evidence         map[string]*topologyv1.TableBuilder
	details          *topologyv1.TableBuilder
	labels           *topologyv1.TableBuilder
}

func newVSphereTopologyBuilder() *vsphereTopologyBuilder {
	return &vsphereTopologyBuilder{
		strings:          topologyv1.NewStringDictionary(),
		actorIndexes:     make(map[string]int),
		actorObjectTypes: make(map[string]string),
		actors:           topologyv1.NewTableBuilder(vsphereTopologyActorColumns()...),
		links:            topologyv1.NewTableBuilder(vsphereTopologyLinkColumns()...),
		evidence: map[string]*topologyv1.TableBuilder{
			vsphereOwnershipEvidenceType:     topologyv1.NewTableBuilder(vsphereTopologyEvidenceColumns()...),
			vsphereRunsOnEvidenceType:        topologyv1.NewTableBuilder(vsphereTopologyEvidenceColumns()...),
			vsphereNetworkConnectionEvidence: topologyv1.NewTableBuilder(vsphereTopologyEvidenceColumns()...),
		},
		details: topologyv1.NewTableBuilder(vsphereTopologyDetailColumns()...),
		labels:  topologyv1.NewTableBuilder(vsphereTopologyLabelColumns()...),
	}
}

func (b *vsphereTopologyBuilder) addActor(actorType, moid, name, parentID string, detail vsphereActorDetail, labels map[string]string) int {
	actorType = cleanTopologyString(actorType)
	moid = cleanTopologyString(moid)
	if actorType == "" || moid == "" {
		return -1
	}
	if row, ok := b.actorIndexes[moid]; ok {
		return row
	}

	objectType := cleanTopologyString(detail.objectType)
	if objectType == "" {
		objectType = strings.TrimPrefix(actorType, "vsphere_")
	}
	name = cleanTopologyString(name)
	if name == "" {
		name = moid
	}

	parentID = cleanTopologyString(parentID)
	parentObjectType := ""
	if _, ok := b.actorIndexes[parentID]; !ok {
		parentID = ""
	} else {
		parentObjectType = b.actorObjectTypes[parentID]
	}

	row := b.actors.Add(
		b.stringRef(actorType),
		b.stringRef(objectType),
		b.stringRef(moid),
		b.stringRef(name),
		b.nullableStringRef(parentObjectType),
		b.nullableStringRef(parentID),
		b.nullableStringRef(detail.overallStatus),
		b.nullableStringRef(detail.powerState),
	)
	b.actorIndexes[moid] = row
	b.actorObjectTypes[moid] = objectType

	detail.objectType = objectType
	detail.parentObjectType = parentObjectType
	detail.parentID = parentID
	detail.moid = moid
	detail.name = name
	b.addActorDetail(row, detail)
	b.addActorLabels(row, detail, labels)

	return row
}

func (b *vsphereTopologyBuilder) addLink(srcID, dstID, linkType, relationship string) int {
	srcID = cleanTopologyString(srcID)
	dstID = cleanTopologyString(dstID)
	linkType = cleanTopologyString(linkType)
	relationship = cleanTopologyString(relationship)
	if relationship == "" {
		relationship = linkType
	}

	src, ok := b.actorIndexes[srcID]
	if !ok {
		return -1
	}
	dst, ok := b.actorIndexes[dstID]
	if !ok {
		return -1
	}
	evidenceType := vsphereEvidenceTypeForLink(linkType)
	evidence := b.evidence[evidenceType]
	if evidence == nil {
		return -1
	}

	link := b.links.Add(
		src,
		dst,
		b.stringRef(linkType),
		b.stringRef(relationship),
		1,
	)
	evidence.Add(
		link,
		b.stringRef(b.actorObjectTypes[srcID]),
		b.stringRef(srcID),
		b.stringRef(b.actorObjectTypes[dstID]),
		b.stringRef(dstID),
		b.stringRef(relationship),
	)

	return link
}

func (b *vsphereTopologyBuilder) data(agentID string, collectedAt time.Time, resourceStats map[string]any) (topologyv1.Data, error) {
	actorTable, err := b.actors.Table()
	if err != nil {
		return topologyv1.Data{}, fmt.Errorf("build vSphere topology actors table: %w", err)
	}
	linkTable, err := b.links.Table()
	if err != nil {
		return topologyv1.Data{}, fmt.Errorf("build vSphere topology links table: %w", err)
	}
	detailTable, err := b.details.Table()
	if err != nil {
		return topologyv1.Data{}, fmt.Errorf("build vSphere topology detail table: %w", err)
	}
	labelTable, err := b.labels.Table()
	if err != nil {
		return topologyv1.Data{}, fmt.Errorf("build vSphere topology labels table: %w", err)
	}

	evidence, evidenceRows, err := b.evidenceMap()
	if err != nil {
		return topologyv1.Data{}, err
	}
	if agentID = cleanTopologyString(agentID); agentID == "" {
		agentID = "unknown"
	}
	if collectedAt.IsZero() {
		collectedAt = time.Now().UTC()
	}

	stats := make(map[string]any, len(resourceStats)+5)
	maps.Copy(stats, resourceStats)
	stats["actor_rows"] = actorTable.Rows
	stats["link_rows"] = linkTable.Rows
	stats["evidence_rows"] = evidenceRows
	stats["detail_rows"] = detailTable.Rows
	stats["label_rows"] = labelTable.Rows

	return topologyv1.Data{
		SchemaVersion: topologyv1.SchemaVersion,
		Producer: topologyv1.Producer{
			Source:       vsphereTopologySource,
			Instance:     agentID,
			Plugin:       "go.d/vsphere",
			Capabilities: []string{"inventory", "topology-v1"},
		},
		CollectedAt: collectedAt,
		View: &topologyv1.View{
			ID:             vsphereTopologyView,
			Scope:          "vsphere_object",
			Mode:           "detailed",
			SupportedModes: []string{"detailed"},
			GroupBy:        []string{"object_type"},
		},
		Dictionaries: topologyv1.Dictionaries{
			"strings": b.strings.Values(),
		},
		Types:        vsphereTopologyTypes(),
		Presentation: vsphereTopologyPresentation(),
		Actors:       actorTable,
		Links:        linkTable,
		Evidence:     evidence,
		Tables: &topologyv1.DetailTables{
			Actor: map[string]topologyv1.DetailTable{
				vsphereTopologyDetailTable: {
					Type:  vsphereTopologyDetailTable,
					Table: detailTable,
				},
				vsphereTopologyLabelsTable: {
					Type:  vsphereTopologyLabelsTable,
					Table: labelTable,
				},
			},
		},
		Stats: stats,
	}, nil
}

func (b *vsphereTopologyBuilder) evidenceMap() (topologyv1.EvidenceMap, int, error) {
	sections := topologyv1.EvidenceMap{}
	totalRows := 0
	for _, evidenceType := range []string{vsphereOwnershipEvidenceType, vsphereRunsOnEvidenceType, vsphereNetworkConnectionEvidence} {
		table, err := b.evidence[evidenceType].Table()
		if err != nil {
			return nil, 0, fmt.Errorf("build vSphere topology %s table: %w", evidenceType, err)
		}
		sections[evidenceType] = topologyv1.EvidenceSection{
			Type:  evidenceType,
			Table: table,
		}
		totalRows += table.Rows
	}
	return sections, totalRows, nil
}

func (b *vsphereTopologyBuilder) addActorDetail(actor int, detail vsphereActorDetail) {
	b.details.Add(
		actor,
		b.stringRef(detail.objectType),
		b.stringRef(detail.moid),
		b.stringRef(detail.name),
		b.nullableStringRef(detail.parentObjectType),
		b.nullableStringRef(detail.parentID),
		b.nullableStringRef(detail.datacenter),
		b.nullableStringRef(detail.cluster),
		b.nullableStringRef(detail.host),
		b.nullableStringRef(detail.resourcePool),
		b.nullableStringRef(detail.connectionState),
		b.nullableStringRef(detail.powerState),
		b.nullableStringRef(detail.overallStatus),
		nullableBool(detail.accessible),
		nullableBool(detail.inMaintenanceMode),
		b.nullableStringRef(detail.maintenanceMode),
		b.nullableStringRef(detail.datastoreType),
		b.nullableStringRef(detail.networkType),
		b.nullableStringRef(detail.ipPoolName),
		nullableBool(detail.storageDRSEnabled),
		nullableBool(detail.multipleHostAccess),
		nullableUint(detail.snapshotCount),
		nullableUint(detail.snapshotChainDepth),
		nullableUint(detail.configuredVCPUs),
		nullableUint(detail.configuredMemoryMiB),
		nullableUint(detail.capacityBytes),
		nullableUint(detail.freeSpaceBytes),
		nullableUint(detail.uncommittedBytes),
		nullableInt(detail.cpuCapacityMHz),
		nullableInt(detail.memoryCapacityMiB),
		nullableInt(detail.cpuLimitMHz),
		nullableInt(detail.memoryLimitMiB),
		nullableInt(detail.cpuReservation),
		nullableInt(detail.memReservation),
		nullableUint(detail.networkHosts),
		nullableUint(detail.networkVMs),
		nullableBool(detail.drsEnabled),
		nullableBool(detail.haEnabled),
		nullableBool(detail.vsanEnabled),
		nullableBool(detail.consolidationNeeded),
		b.nullableStringRef(detail.toolsRunningStatus),
		b.nullableStringRef(detail.toolsVersionStatus),
	)
}

func (b *vsphereTopologyBuilder) addActorLabels(actor int, detail vsphereActorDetail, labels map[string]string) {
	b.addActorLabel(actor, "name", detail.name, "vsphere", "identity", nil)
	b.addActorLabel(actor, "object_type", detail.objectType, "vsphere", "identity", nil)
	b.addActorLabel(actor, "vsphere_moid", detail.moid, "vsphere", "identity", nil)
	b.addActorLabel(actor, "datacenter", detail.datacenter, "vsphere", "hierarchy", nil)
	b.addActorLabel(actor, "cluster", detail.cluster, "vsphere", "hierarchy", nil)
	b.addActorLabel(actor, "host", detail.host, "vsphere", "hierarchy", nil)
	b.addActorLabel(actor, "resource_pool", detail.resourcePool, "vsphere", "hierarchy", nil)

	keys := make([]string, 0, len(labels))
	for key := range labels {
		key = cleanTopologyString(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.addActorLabel(actor, key, labels[key], "vsphere", "label", nil)
	}
}

func (b *vsphereTopologyBuilder) addActorLabel(actor int, key, value, source, kind string, valueIndex any) {
	key = cleanTopologyString(key)
	value = cleanTopologyString(value)
	if key == "" || value == "" {
		return
	}
	b.labels.Add(
		actor,
		b.stringRef(key),
		b.stringRef(value),
		b.nullableStringRef(source),
		b.nullableStringRef(kind),
		nullableUint(valueIndex),
	)
}

func (b *vsphereTopologyBuilder) stringRef(value string) int {
	return b.strings.Ref(cleanTopologyString(value))
}

func (b *vsphereTopologyBuilder) nullableStringRef(value string) any {
	value = cleanTopologyString(value)
	if value == "" {
		return nil
	}
	return b.strings.Ref(value)
}

func vsphereTopologyActorColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("object_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("identity")),
		topologyv1.NewColumn("vsphere_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("identity")),
		topologyv1.NewColumn("name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("parent_object_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("parent_identity")),
		topologyv1.NewColumn("parent_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("parent_identity")),
		topologyv1.NewColumn("overall_status", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("power_state", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
	}
}

func vsphereTopologyLinkColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("src_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("dst_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("relationship", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
	}
}

func vsphereTopologyEvidenceColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("source_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("source_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("target_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("target_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("relationship", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
	}
}

func vsphereTopologyDetailColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("object_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("vsphere_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("identity")),
		topologyv1.NewColumn("name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("parent_object_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("parent_identity")),
		topologyv1.NewColumn("parent_moid", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("parent_identity")),
		topologyv1.NewColumn("datacenter", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("cluster", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("host", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("resource_pool", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("connection_state", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("power_state", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("overall_status", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("accessible", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("in_maintenance_mode", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("maintenance_mode", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("datastore_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("network_type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("ip_pool_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("storage_drs_enabled", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("multiple_host_access", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("snapshot_count", "uint", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("snapshot_chain_depth", "uint", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("max")),
		topologyv1.NewColumn("configured_vcpus", "uint", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("configured_memory_mib", "uint", topologyv1.WithNullable(), topologyv1.WithUnit("MiB"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("capacity_bytes", "uint", topologyv1.WithNullable(), topologyv1.WithUnit("bytes"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("free_space_bytes", "uint", topologyv1.WithNullable(), topologyv1.WithUnit("bytes"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("uncommitted_bytes", "uint", topologyv1.WithNullable(), topologyv1.WithUnit("bytes"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("cpu_capacity_mhz", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MHz"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("memory_capacity_mib", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MiB"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("cpu_limit_mhz", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MHz"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("last")),
		topologyv1.NewColumn("memory_limit_mib", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MiB"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("last")),
		topologyv1.NewColumn("cpu_reservation_mhz", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MHz"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("last")),
		topologyv1.NewColumn("memory_reservation_mib", "int", topologyv1.WithNullable(), topologyv1.WithUnit("MiB"), topologyv1.WithRole("metric"), topologyv1.WithAggregation("last")),
		topologyv1.NewColumn("network_hosts", "uint", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("network_vms", "uint", topologyv1.WithNullable(), topologyv1.WithRole("metric"), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("drs_enabled", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("ha_enabled", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("vsan_enabled", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("consolidation_needed", "bool", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("tools_running_status", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("tools_version_status", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
	}
}

func vsphereTopologyLabelColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("key", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("value", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("kind", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		topologyv1.NewColumn("value_index", "uint", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
	}
}

func vsphereTopologyTypes() topologyv1.TypeRegistry {
	return topologyv1.TypeRegistry{
		ActorTypes: map[string]topologyv1.ActorType{
			"vsphere_datacenter":        vsphereActorType("Datacenter", "datacenter", "blue"),
			"vsphere_cluster":           vsphereActorType("Cluster", "cluster", "green"),
			"vsphere_host":              vsphereActorType("ESXi Host", "host", "orange"),
			"vsphere_vm":                vsphereActorType("VM", "vm", "purple"),
			"vsphere_datastore":         vsphereActorType("Datastore", "datastore", "cyan"),
			"vsphere_network":           vsphereActorType("Network", "network", "yellow"),
			"vsphere_datastore_cluster": vsphereActorType("Datastore Cluster", "datastore_cluster", "teal"),
			"vsphere_resource_pool":     vsphereActorType("Resource Pool", "resource_pool", "gray"),
		},
		LinkTypes: map[string]topologyv1.LinkType{
			vsphereTopologyOwnershipLink: vsphereLinkType(
				"hierarchical",
				"ownership",
				"ownership",
				"preserve",
				vsphereOwnershipEvidenceType,
				vsphereLinkPresentation("Contains", "gray", "forward", "closer"),
			),
			vsphereTopologyRunsOnLink: vsphereLinkType(
				"directed",
				"dependency",
				"normal",
				"preserve",
				vsphereRunsOnEvidenceType,
				vsphereLinkPresentation("Runs on", "blue", "forward", "normal"),
			),
			vsphereTopologyNetworkLink: vsphereLinkType(
				"undirected",
				"observation",
				"normal",
				"canonicalize_unordered",
				vsphereNetworkConnectionEvidence,
				vsphereLinkPresentation("Connected to", "green", "none", "farther"),
			),
		},
		EvidenceTypes: map[string]topologyv1.EvidenceType{
			vsphereOwnershipEvidenceType:     vsphereEvidenceType(vsphereTopologyOwnershipLink),
			vsphereRunsOnEvidenceType:        vsphereEvidenceType(vsphereTopologyRunsOnLink),
			vsphereNetworkConnectionEvidence: vsphereEvidenceType(vsphereTopologyNetworkLink),
		},
		TableTypes: map[string]topologyv1.TableType{
			vsphereTopologyDetailTable: vsphereDetailTableType(),
			vsphereTopologyLabelsTable: vsphereLabelsTableType(),
		},
	}
}

func vsphereActorType(label, icon, colorSlot string) topologyv1.ActorType {
	return topologyv1.ActorType{
		Layer:          vsphereTopologyLayer,
		Identity:       []string{"object_type", "vsphere_moid"},
		MergeIdentity:  []string{"object_type", "vsphere_moid"},
		ParentIdentity: []string{"parent_object_type", "parent_moid"},
		Search: &topologyv1.ActorSearchPolicy{
			Columns:   []string{"name", "vsphere_moid"},
			LabelKeys: []string{"datacenter", "cluster", "host", "resource_pool"},
		},
		Presentation: vsphereActorPresentation(label, icon, colorSlot),
	}
}

func vsphereLinkType(orientation, directionRole, semanticRole, aggregationDirection, evidenceType string, presentation *topologyv1.LinkPresentation) topologyv1.LinkType {
	return topologyv1.LinkType{
		Orientation:   orientation,
		DirectionRole: directionRole,
		SemanticRole:  semanticRole,
		Aggregation: topologyv1.LinkAggregation{
			Direction: aggregationDirection,
			Evidence:  "append",
			Metrics: map[string]string{
				"evidence_count": "sum",
			},
		},
		EvidenceTypes: []string{evidenceType},
		Presentation:  presentation,
	}
}

func vsphereEvidenceType(linkType string) topologyv1.EvidenceType {
	return topologyv1.EvidenceType{
		LinkType:     linkType,
		Role:         "relationship_evidence",
		Columns:      vsphereTopologyEvidenceColumns(),
		MatchColumns: []string{"source_type", "source_moid", "target_type", "target_moid", "relationship"},
	}
}

func vsphereDetailTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "last",
		Columns:     vsphereTopologyDetailColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:             "vSphere object details",
			Order:             1,
			DefaultVisibility: "expanded",
			Columns:           vsphereDetailModalColumns(),
		},
	}
}

func vsphereLabelsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_inventory",
		Owner:       "actor",
		Aggregation: "set",
		Columns:     vsphereTopologyLabelColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label: "Labels",
			Order: 0,
			Columns: []topologyv1.ModalColumn{
				vsphereModalDirectColumn("key", "Label", "key", "text"),
				vsphereModalDirectColumn("value", "Value", "value", "text"),
				vsphereModalDirectColumn("source", "Source", "source", "badge"),
				vsphereModalDirectColumn("kind", "Kind", "kind", "badge"),
			},
		},
	}
}

func vsphereEvidenceTypeForLink(linkType string) string {
	switch linkType {
	case vsphereTopologyOwnershipLink:
		return vsphereOwnershipEvidenceType
	case vsphereTopologyRunsOnLink:
		return vsphereRunsOnEvidenceType
	case vsphereTopologyNetworkLink:
		return vsphereNetworkConnectionEvidence
	default:
		return ""
	}
}

func firstExistingResourceID(exists func(string) bool, ids ...string) string {
	if exists == nil {
		return ""
	}
	for _, id := range ids {
		id = cleanTopologyString(id)
		if id != "" && exists(id) {
			return id
		}
	}
	return ""
}

func mergeStringMaps(maps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, values := range maps {
		for key, value := range values {
			key = cleanTopologyString(key)
			value = cleanTopologyString(value)
			if key == "" || value == "" {
				continue
			}
			merged[key] = value
		}
	}
	return merged
}

func sortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = cleanTopologyString(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(seen))
	for value := range seen {
		sorted = append(sorted, value)
	}
	sort.Strings(sorted)
	return sorted
}

func cleanTopologyString(value string) string {
	return strings.TrimSpace(value)
}

func nullableBool(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case bool:
		return v
	case *bool:
		if v == nil {
			return nil
		}
		return *v
	default:
		return nil
	}
}

func nullableUint(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case int:
		if v < 0 {
			return nil
		}
		return v
	case int16:
		if v < 0 {
			return nil
		}
		return int64(v)
	case int32:
		if v < 0 {
			return nil
		}
		return int64(v)
	case int64:
		if v < 0 {
			return nil
		}
		return v
	case uint:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return v
	default:
		return nil
	}
}

func nullableInt(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case int:
		return v
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint32:
		return int64(v)
	default:
		return nil
	}
}
