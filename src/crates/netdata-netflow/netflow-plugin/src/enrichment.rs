#[cfg(test)]
use crate::decoder::FlowFields;
use crate::decoder::FlowRecord;
use crate::plugin_config::{
    AsnProviderConfig, EnrichmentConfig, GeoIpConfig, NetProviderConfig, NetworkAttributesConfig,
    NetworkAttributesValue, SamplingRateSetting, StaticExporterConfig, StaticInterfaceConfig,
    StaticRoutingConfig, StaticRoutingEntryConfig, StaticRoutingLargeCommunityConfig,
};
use anyhow::{Context, Result};
use ipnet::IpNet;
use ipnet_trie::IpnetTrie;
use maxminddb::Reader;
use regex::Regex;
use serde::Deserialize;
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::net::{IpAddr, SocketAddr};
use std::str::FromStr;
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

const GEOIP_RELOAD_CHECK_INTERVAL: Duration = Duration::from_secs(30);
pub(crate) const PRIVATE_IP_ADDRESS_SPACE_LABEL: &str = "Private IP Address Space";
pub(crate) const UNKNOWN_ASN_LABEL: &str = "Unknown ASN";

include!("enrichment_classifiers.rs");
include!("enrichment_data.rs");

#[derive(Debug)]
pub(crate) struct FlowEnricher {
    default_sampling_rate: PrefixMap<u64>,
    override_sampling_rate: PrefixMap<u64>,
    static_metadata: StaticMetadata,
    networks: PrefixMap<NetworkAttributes>,
    geoip: Option<GeoIpResolver>,
    network_sources_runtime: Option<NetworkSourcesRuntime>,
    exporter_classifiers: Vec<ClassifierRule>,
    interface_classifiers: Vec<ClassifierRule>,
    classifier_cache_duration: Duration,
    exporter_classifier_cache: Arc<Mutex<ExporterClassifierCache>>,
    interface_classifier_cache: Arc<Mutex<InterfaceClassifierCache>>,
    asn_providers: Vec<AsnProviderConfig>,
    net_providers: Vec<NetProviderConfig>,
    static_routing: StaticRouting,
    dynamic_routing: Option<DynamicRoutingRuntime>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct DynamicRoutingPeerKey {
    pub(crate) exporter: SocketAddr,
    pub(crate) session_id: u64,
    pub(crate) peer_id: String,
}

#[derive(Debug, Clone)]
pub(crate) struct DynamicRoutingUpdate {
    pub(crate) peer: DynamicRoutingPeerKey,
    pub(crate) prefix: IpNet,
    pub(crate) route_key: String,
    pub(crate) next_hop: Option<IpAddr>,
    pub(crate) asn: u32,
    pub(crate) as_path: Vec<u32>,
    pub(crate) communities: Vec<u32>,
    pub(crate) large_communities: Vec<(u32, u32, u32)>,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct DynamicRoutingRuntime {
    state: Arc<RwLock<DynamicRoutingState>>,
}

struct DynamicRoutingState {
    entries: IpnetTrie<Vec<DynamicRoutingRoute>>,
}

impl Default for DynamicRoutingState {
    fn default() -> Self {
        Self {
            entries: IpnetTrie::new(),
        }
    }
}

impl std::fmt::Debug for DynamicRoutingState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("DynamicRoutingState")
            .field("entries", &"<IpnetTrie>")
            .finish()
    }
}

#[derive(Debug, Clone)]
pub(crate) struct NetworkSourceRecord {
    pub(crate) prefix: IpNet,
    pub(crate) attrs: NetworkAttributes,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct NetworkSourcesRuntime {
    records: Arc<RwLock<Vec<NetworkSourceRecord>>>,
}

#[derive(Debug, Clone)]
struct DynamicRoutingRoute {
    peer: DynamicRoutingPeerKey,
    route_key: String,
    next_hop: Option<IpAddr>,
    entry: StaticRoutingEntry,
}

#[derive(Debug, Clone)]
struct TimedClassifierEntry<T> {
    value: T,
    last_access: Instant,
}

type ExporterClassifierCache = HashMap<ExporterInfo, TimedClassifierEntry<ExporterClassification>>;
type InterfaceClassifierCache =
    HashMap<ExporterAndInterfaceInfo, TimedClassifierEntry<InterfaceClassification>>;

impl DynamicRoutingRuntime {
    pub(crate) fn upsert(&self, update: DynamicRoutingUpdate) {
        let entry = StaticRoutingEntry {
            asn: update.asn,
            as_path: update.as_path,
            communities: update.communities,
            large_communities: update
                .large_communities
                .into_iter()
                .map(
                    |(asn, local_data1, local_data2)| StaticRoutingLargeCommunity {
                        asn,
                        local_data1,
                        local_data2,
                    },
                )
                .collect(),
            net_mask: update.prefix.prefix_len(),
            next_hop: update.next_hop,
        };

        let Ok(mut state) = self.state.write() else {
            return;
        };
        // Remove existing routes for this prefix (if any), modify, re-insert.
        let mut routes = state.entries.remove(update.prefix).unwrap_or_default();
        if let Some(route) = routes
            .iter_mut()
            .find(|route| route.peer == update.peer && route.route_key == update.route_key)
        {
            route.next_hop = update.next_hop;
            route.entry = entry;
        } else {
            routes.push(DynamicRoutingRoute {
                peer: update.peer,
                route_key: update.route_key,
                next_hop: update.next_hop,
                entry,
            });
        }
        state.entries.insert(update.prefix, routes);
    }

    pub(crate) fn withdraw(&self, peer: &DynamicRoutingPeerKey, prefix: IpNet, route_key: &str) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        if let Some(mut routes) = state.entries.remove(prefix) {
            routes.retain(|route| !(&route.peer == peer && route.route_key == route_key));
            if !routes.is_empty() {
                state.entries.insert(prefix, routes);
            }
        }
    }

    pub(crate) fn clear_peer(&self, peer: &DynamicRoutingPeerKey) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        // Collect affected prefixes (can't mutate trie while iterating).
        let affected: Vec<IpNet> = state
            .entries
            .iter()
            .filter(|(_, routes)| routes.iter().any(|r| &r.peer == peer))
            .map(|(prefix, _)| prefix)
            .collect();
        for prefix in affected {
            if let Some(mut routes) = state.entries.remove(prefix) {
                routes.retain(|r| &r.peer != peer);
                if !routes.is_empty() {
                    state.entries.insert(prefix, routes);
                }
            }
        }
    }

    pub(crate) fn clear_session(&self, exporter: SocketAddr, session_id: u64) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        let affected: Vec<IpNet> = state
            .entries
            .iter()
            .filter(|(_, routes)| {
                routes
                    .iter()
                    .any(|r| r.peer.exporter == exporter && r.peer.session_id == session_id)
            })
            .map(|(prefix, _)| prefix)
            .collect();
        for prefix in affected {
            if let Some(mut routes) = state.entries.remove(prefix) {
                routes.retain(|r| r.peer.exporter != exporter || r.peer.session_id != session_id);
                if !routes.is_empty() {
                    state.entries.insert(prefix, routes);
                }
            }
        }
    }

    fn lookup(
        &self,
        address: IpAddr,
        preferred_next_hop: Option<IpAddr>,
        preferred_exporter: Option<IpAddr>,
    ) -> Option<StaticRoutingEntry> {
        let Ok(state) = self.state.read() else {
            return None;
        };

        // O(prefix_length) trie lookup instead of O(n) linear scan.
        let host_prefix = IpNet::from(address);
        let (_, routes) = state.entries.longest_match(&host_prefix)?;
        if routes.is_empty() {
            return None;
        }

        if let Some(exporter_ip) = preferred_exporter {
            if let Some(next_hop) = preferred_next_hop
                && let Some(route) = routes.iter().find(|route| {
                    route.peer.exporter.ip() == exporter_ip && route.next_hop == Some(next_hop)
                })
            {
                return Some(route.entry.clone());
            }
            if let Some(route) = routes
                .iter()
                .find(|route| route.peer.exporter.ip() == exporter_ip)
            {
                return Some(route.entry.clone());
            }
        }
        if let Some(next_hop) = preferred_next_hop
            && let Some(route) = routes.iter().find(|route| route.next_hop == Some(next_hop))
        {
            return Some(route.entry.clone());
        }
        routes.first().map(|route| route.entry.clone())
    }

    #[cfg(test)]
    fn route_count(&self) -> usize {
        let Ok(state) = self.state.read() else {
            return 0;
        };
        state.entries.iter().map(|(_, routes)| routes.len()).sum()
    }
}

impl NetworkSourcesRuntime {
    pub(crate) fn replace_records(&self, records: Vec<NetworkSourceRecord>) {
        if let Ok(mut guard) = self.records.write() {
            *guard = records;
        }
    }
}

impl FlowEnricher {
    pub(crate) fn from_config(config: &EnrichmentConfig) -> Result<Option<Self>> {
        let default_sampling_rate = build_sampling_map(
            config.default_sampling_rate.as_ref(),
            "enrichment.default_sampling_rate",
        )?;
        let override_sampling_rate = build_sampling_map(
            config.override_sampling_rate.as_ref(),
            "enrichment.override_sampling_rate",
        )?;
        let static_metadata = StaticMetadata::from_config(config)?;
        let networks = build_network_attributes_map(&config.networks)?;
        let geoip = GeoIpResolver::from_config(&config.geoip)?;
        let network_sources_runtime = if config.network_sources.is_empty() {
            None
        } else {
            Some(NetworkSourcesRuntime::default())
        };

        let exporter_classifiers = config
            .exporter_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.exporter_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let interface_classifiers = config
            .interface_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.interface_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let static_routing = StaticRouting::from_config(&config.routing_static)?;
        let dynamic_routing =
            if config.routing_dynamic.bmp.enabled || config.routing_dynamic.bioris.enabled {
                Some(DynamicRoutingRuntime::default())
            } else {
                None
            };

        if default_sampling_rate.is_empty()
            && override_sampling_rate.is_empty()
            && static_metadata.is_empty()
            && networks.is_empty()
            && geoip.is_none()
            && network_sources_runtime.is_none()
            && exporter_classifiers.is_empty()
            && interface_classifiers.is_empty()
            && static_routing.is_empty()
            && dynamic_routing.is_none()
        {
            return Ok(None);
        }

        Ok(Some(Self {
            default_sampling_rate,
            override_sampling_rate,
            static_metadata,
            networks,
            geoip,
            network_sources_runtime,
            exporter_classifiers,
            interface_classifiers,
            classifier_cache_duration: config.classifier_cache_duration,
            exporter_classifier_cache: Arc::new(Mutex::new(HashMap::new())),
            interface_classifier_cache: Arc::new(Mutex::new(HashMap::new())),
            asn_providers: config.asn_providers.clone(),
            net_providers: config.net_providers.clone(),
            static_routing,
            dynamic_routing,
        }))
    }

    pub(crate) fn dynamic_routing_runtime(&self) -> Option<DynamicRoutingRuntime> {
        self.dynamic_routing.clone()
    }

    pub(crate) fn network_sources_runtime(&self) -> Option<NetworkSourcesRuntime> {
        self.network_sources_runtime.clone()
    }

    pub(crate) fn refresh_runtime_state(&mut self) {
        if let Some(geoip) = &mut self.geoip {
            geoip.refresh_if_needed();
        }
    }

    #[cfg(test)]
    pub(crate) fn enrich_fields(&mut self, fields: &mut FlowFields) -> bool {
        let Some(exporter_ip) = parse_exporter_ip(fields) else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let in_if = parse_u32_field(fields, "IN_IF");
        let out_if = parse_u32_field(fields, "OUT_IF");

        let mut exporter_name = fields
            .get("EXPORTER_NAME")
            .cloned()
            .filter(|value| !value.is_empty())
            .unwrap_or_else(|| exporter_ip_str.clone());
        let mut exporter_classification = ExporterClassification::default();
        let mut in_interface = InterfaceInfo {
            index: in_if,
            vlan: parse_u16_field(fields, "SRC_VLAN"),
            ..Default::default()
        };
        let mut out_interface = InterfaceInfo {
            index: out_if,
            vlan: parse_u16_field(fields, "DST_VLAN"),
            ..Default::default()
        };
        let mut in_classification = InterfaceClassification::default();
        let mut out_classification = InterfaceClassification::default();

        if in_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, in_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            in_interface.name = lookup.interface.name.clone();
            in_interface.description = lookup.interface.description.clone();
            in_interface.speed = lookup.interface.speed;
            in_classification.provider = lookup.interface.provider.clone();
            in_classification.connectivity = lookup.interface.connectivity.clone();
            in_classification.boundary = lookup.interface.boundary;
        }

        if out_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, out_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            out_interface.name = lookup.interface.name.clone();
            out_interface.description = lookup.interface.description.clone();
            out_interface.speed = lookup.interface.speed;
            out_classification.provider = lookup.interface.provider.clone();
            out_classification.connectivity = lookup.interface.connectivity.clone();
            out_classification.boundary = lookup.interface.boundary;
        }

        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            fields.insert("SAMPLING_RATE", sampling_rate.to_string());
        }
        if parse_u64_field(fields, "SAMPLING_RATE") == 0 {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                fields.insert("SAMPLING_RATE", sampling_rate.to_string());
            }
        }

        let exporter_info = ExporterInfo {
            ip: exporter_ip_str.clone(),
            name: exporter_name.clone(),
        };
        if !self.classify_exporter(&exporter_info, &mut exporter_classification) {
            return false;
        }

        if !self.classify_interface(&exporter_info, &out_interface, &mut out_classification) {
            return false;
        }
        if !self.classify_interface(&exporter_info, &in_interface, &mut in_classification) {
            return false;
        }

        let flow_next_hop = parse_ip_field(fields, "NEXT_HOP");
        let source_routing = parse_ip_field(fields, "SRC_ADDR")
            .and_then(|src_addr| self.lookup_routing(src_addr, None, Some(exporter_ip)));
        let dest_routing = parse_ip_field(fields, "DST_ADDR")
            .and_then(|dst_addr| self.lookup_routing(dst_addr, flow_next_hop, Some(exporter_ip)));

        let source_flow_mask = parse_u8_field(fields, "SRC_MASK");
        let dest_flow_mask = parse_u8_field(fields, "DST_MASK");
        let source_flow_as = parse_u32_field(fields, "SRC_AS");
        let dest_flow_as = parse_u32_field(fields, "DST_AS");
        let source_routing_as = source_routing.as_ref().map_or(0, |entry| entry.asn);
        let dest_routing_as = dest_routing.as_ref().map_or(0, |entry| entry.asn);
        let source_routing_mask = source_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let dest_routing_mask = dest_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let routing_next_hop = dest_routing.as_ref().and_then(|entry| entry.next_hop);

        let source_mask = self.get_net_mask(source_flow_mask, source_routing_mask);
        let dest_mask = self.get_net_mask(dest_flow_mask, dest_routing_mask);
        let source_network = parse_ip_field(fields, "SRC_ADDR")
            .and_then(|src_addr| self.resolve_network_attributes(src_addr));
        let dest_network = parse_ip_field(fields, "DST_ADDR")
            .and_then(|dst_addr| self.resolve_network_attributes(dst_addr));
        let source_as = apply_network_asn_override(
            self.get_as_number(source_flow_as, source_routing_as, source_mask),
            source_network.as_ref().map_or(0, |attrs| attrs.asn),
        );
        let dest_as = apply_network_asn_override(
            self.get_as_number(dest_flow_as, dest_routing_as, dest_mask),
            dest_network.as_ref().map_or(0, |attrs| attrs.asn),
        );
        let next_hop = self.get_next_hop(flow_next_hop, routing_next_hop);

        fields.insert("SRC_MASK", source_mask.to_string());
        fields.insert("DST_MASK", dest_mask.to_string());
        fields.insert("SRC_AS", source_as.to_string());
        fields.insert("DST_AS", dest_as.to_string());
        fields.insert(
            "NEXT_HOP",
            next_hop.map(|addr| addr.to_string()).unwrap_or_default(),
        );
        write_network_attributes(fields, &SRC_KEYS, source_network.as_ref(), source_as);
        write_network_attributes(fields, &DST_KEYS, dest_network.as_ref(), dest_as);

        if let Some(dest_routing) = dest_routing {
            append_u32_list_field(fields, "DST_AS_PATH", &dest_routing.as_path);
            append_u32_list_field(fields, "DST_COMMUNITIES", &dest_routing.communities);
            append_large_communities_field(
                fields,
                "DST_LARGE_COMMUNITIES",
                &dest_routing.large_communities,
            );
        }

        fields.insert("EXPORTER_NAME", exporter_name);
        fields.insert("EXPORTER_GROUP", exporter_classification.group);
        fields.insert("EXPORTER_ROLE", exporter_classification.role);
        fields.insert("EXPORTER_SITE", exporter_classification.site);
        fields.insert("EXPORTER_REGION", exporter_classification.region);
        fields.insert("EXPORTER_TENANT", exporter_classification.tenant);

        fields.insert("IN_IF_NAME", in_classification.name);
        fields.insert("IN_IF_DESCRIPTION", in_classification.description);
        if in_interface.speed > 0 {
            fields.insert("IN_IF_SPEED", in_interface.speed.to_string());
        } else {
            fields.remove("IN_IF_SPEED");
        }
        fields.insert("IN_IF_PROVIDER", in_classification.provider);
        fields.insert("IN_IF_CONNECTIVITY", in_classification.connectivity);
        if in_classification.boundary != 0 {
            fields.insert("IN_IF_BOUNDARY", in_classification.boundary.to_string());
        } else {
            fields.remove("IN_IF_BOUNDARY");
        }

        fields.insert("OUT_IF_NAME", out_classification.name);
        fields.insert("OUT_IF_DESCRIPTION", out_classification.description);
        if out_interface.speed > 0 {
            fields.insert("OUT_IF_SPEED", out_interface.speed.to_string());
        } else {
            fields.remove("OUT_IF_SPEED");
        }
        fields.insert("OUT_IF_PROVIDER", out_classification.provider);
        fields.insert("OUT_IF_CONNECTIVITY", out_classification.connectivity);
        if out_classification.boundary != 0 {
            fields.insert("OUT_IF_BOUNDARY", out_classification.boundary.to_string());
        } else {
            fields.remove("OUT_IF_BOUNDARY");
        }

        true
    }

    /// Enrich a FlowRecord in place. Same logic as enrich_fields but operates
    /// on native typed fields — no string parsing or formatting on the hot path.
    pub(crate) fn enrich_record(&mut self, rec: &mut FlowRecord) -> bool {
        let Some(exporter_ip) = rec.exporter_ip else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let in_if = rec.in_if;
        let out_if = rec.out_if;

        let mut exporter_name = if rec.exporter_name.is_empty() {
            exporter_ip_str.clone()
        } else {
            rec.exporter_name.clone()
        };
        let mut exporter_classification = ExporterClassification::default();
        let mut in_interface = InterfaceInfo {
            index: in_if,
            vlan: rec.src_vlan,
            ..Default::default()
        };
        let mut out_interface = InterfaceInfo {
            index: out_if,
            vlan: rec.dst_vlan,
            ..Default::default()
        };
        let mut in_classification = InterfaceClassification::default();
        let mut out_classification = InterfaceClassification::default();

        if in_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, in_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            in_interface.name = lookup.interface.name.clone();
            in_interface.description = lookup.interface.description.clone();
            in_interface.speed = lookup.interface.speed;
            in_classification.provider = lookup.interface.provider.clone();
            in_classification.connectivity = lookup.interface.connectivity.clone();
            in_classification.boundary = lookup.interface.boundary;
        }

        if out_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, out_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            out_interface.name = lookup.interface.name.clone();
            out_interface.description = lookup.interface.description.clone();
            out_interface.speed = lookup.interface.speed;
            out_classification.provider = lookup.interface.provider.clone();
            out_classification.connectivity = lookup.interface.connectivity.clone();
            out_classification.boundary = lookup.interface.boundary;
        }

        // Sampling rate overrides.
        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            rec.set_sampling_rate(sampling_rate);
        }
        if !rec.has_sampling_rate() {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                rec.set_sampling_rate(sampling_rate);
            }
        }

        let exporter_info = ExporterInfo {
            ip: exporter_ip_str.clone(),
            name: exporter_name.clone(),
        };
        if !self.classify_exporter(&exporter_info, &mut exporter_classification) {
            return false;
        }

        if !self.classify_interface(&exporter_info, &out_interface, &mut out_classification) {
            return false;
        }
        if !self.classify_interface(&exporter_info, &in_interface, &mut in_classification) {
            return false;
        }

        // Routing lookups — direct field access, no parsing.
        let flow_next_hop = rec.next_hop;
        let source_routing = rec
            .src_addr
            .and_then(|src_addr| self.lookup_routing(src_addr, None, Some(exporter_ip)));
        let dest_routing = rec
            .dst_addr
            .and_then(|dst_addr| self.lookup_routing(dst_addr, flow_next_hop, Some(exporter_ip)));

        let source_flow_mask = rec.src_mask;
        let dest_flow_mask = rec.dst_mask;
        let source_flow_as = rec.src_as;
        let dest_flow_as = rec.dst_as;
        let source_routing_as = source_routing.as_ref().map_or(0, |entry| entry.asn);
        let dest_routing_as = dest_routing.as_ref().map_or(0, |entry| entry.asn);
        let source_routing_mask = source_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let dest_routing_mask = dest_routing.as_ref().map_or(0, |entry| entry.net_mask);
        let routing_next_hop = dest_routing.as_ref().and_then(|entry| entry.next_hop);

        let source_mask = self.get_net_mask(source_flow_mask, source_routing_mask);
        let dest_mask = self.get_net_mask(dest_flow_mask, dest_routing_mask);
        let source_network = rec
            .src_addr
            .and_then(|src_addr| self.resolve_network_attributes(src_addr));
        let dest_network = rec
            .dst_addr
            .and_then(|dst_addr| self.resolve_network_attributes(dst_addr));
        let source_as = apply_network_asn_override(
            self.get_as_number(source_flow_as, source_routing_as, source_mask),
            source_network.as_ref().map_or(0, |attrs| attrs.asn),
        );
        let dest_as = apply_network_asn_override(
            self.get_as_number(dest_flow_as, dest_routing_as, dest_mask),
            dest_network.as_ref().map_or(0, |attrs| attrs.asn),
        );
        let next_hop = self.get_next_hop(flow_next_hop, routing_next_hop);

        // Write enriched values directly to record — no to_string().
        rec.src_mask = source_mask;
        rec.dst_mask = dest_mask;
        rec.src_as = source_as;
        rec.dst_as = dest_as;
        rec.next_hop = next_hop;

        // Network attributes — direct field assignment.
        write_network_attributes_record_src(rec, source_network.as_ref());
        write_network_attributes_record_dst(rec, dest_network.as_ref());

        // BGP routing info — CSV strings built once.
        if let Some(dest_routing) = dest_routing {
            append_u32_csv(&mut rec.dst_as_path, &dest_routing.as_path);
            append_u32_csv(&mut rec.dst_communities, &dest_routing.communities);
            append_large_communities_csv(
                &mut rec.dst_large_communities,
                &dest_routing.large_communities,
            );
        }

        // Exporter classification — move strings, no clone.
        rec.exporter_name = exporter_name;
        rec.exporter_group = exporter_classification.group;
        rec.exporter_role = exporter_classification.role;
        rec.exporter_site = exporter_classification.site;
        rec.exporter_region = exporter_classification.region;
        rec.exporter_tenant = exporter_classification.tenant;

        // Interface classification — move strings, no clone.
        rec.in_if_name = in_classification.name;
        rec.in_if_description = in_classification.description;
        if in_interface.speed > 0 {
            rec.set_in_if_speed(in_interface.speed);
        } else {
            rec.clear_in_if_speed();
        }
        rec.in_if_provider = in_classification.provider;
        rec.in_if_connectivity = in_classification.connectivity;
        if in_classification.boundary != 0 {
            rec.set_in_if_boundary(in_classification.boundary);
        } else {
            rec.clear_in_if_boundary();
        }

        rec.out_if_name = out_classification.name;
        rec.out_if_description = out_classification.description;
        if out_interface.speed > 0 {
            rec.set_out_if_speed(out_interface.speed);
        } else {
            rec.clear_out_if_speed();
        }
        rec.out_if_provider = out_classification.provider;
        rec.out_if_connectivity = out_classification.connectivity;
        if out_classification.boundary != 0 {
            rec.set_out_if_boundary(out_classification.boundary);
        } else {
            rec.clear_out_if_boundary();
        }

        true
    }

    fn lookup_routing(
        &self,
        address: IpAddr,
        preferred_next_hop: Option<IpAddr>,
        preferred_exporter: Option<IpAddr>,
    ) -> Option<StaticRoutingEntry> {
        if let Some(runtime) = &self.dynamic_routing
            && let Some(dynamic) = runtime.lookup(address, preferred_next_hop, preferred_exporter)
        {
            return Some(dynamic);
        }
        self.static_routing.lookup(address).cloned()
    }

    fn resolve_network_attributes(&self, address: IpAddr) -> Option<NetworkAttributes> {
        let mut resolved = self
            .geoip
            .as_ref()
            .and_then(|geoip| geoip.lookup(address))
            .unwrap_or_default();

        // Merge all matching prefixes in ascending prefix length order (least specific first,
        // so more specific prefixes override). Network sources (priority 0) are applied before
        // static config (priority 1) at the same prefix length.
        //
        // Runtime records are behind RwLock, so collect matching entries while the lock is held,
        // then merge outside the lock to minimize lock duration.
        let mut runtime_matches: Vec<(u8, NetworkAttributes)> = Vec::new();
        if let Some(runtime) = &self.network_sources_runtime
            && let Ok(records) = runtime.records.read()
        {
            for record in records.iter() {
                if record.prefix.contains(&address) {
                    runtime_matches.push((record.prefix.prefix_len(), record.attrs.clone()));
                }
            }
            runtime_matches.sort_by_key(|(prefix_len, _)| *prefix_len);
        }

        // Merge-walk: runtime (source_priority=0) then static (source_priority=1) at each level.
        let mut rt_iter = runtime_matches.iter().peekable();
        let mut static_iter = self.networks.matching_entries_ascending(address).peekable();

        loop {
            let rt_len = rt_iter.peek().map(|(len, _)| *len);
            let st_len = static_iter.peek().map(|e| e.prefix.prefix_len());

            match (rt_len, st_len) {
                (None, None) => break,
                (Some(_), None) => {
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                (None, Some(_)) => {
                    let entry = static_iter.next().unwrap();
                    resolved.merge_from(&entry.value);
                }
                (Some(r), Some(s)) if r < s => {
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                (Some(r), Some(s)) if r == s => {
                    // Same prefix length: runtime (priority 0) before static (priority 1).
                    let (_, attrs) = rt_iter.next().unwrap();
                    resolved.merge_from(attrs);
                }
                _ => {
                    let entry = static_iter.next().unwrap();
                    resolved.merge_from(&entry.value);
                }
            }
        }

        if resolved.is_empty() {
            None
        } else {
            Some(resolved)
        }
    }

    fn get_as_number(&self, flow_as: u32, routing_as: u32, flow_net_mask: u8) -> u32 {
        let mut asn = 0_u32;
        for provider in &self.asn_providers {
            if asn != 0 {
                break;
            }
            match provider {
                AsnProviderConfig::Geoip => {
                    // Akvorado parity: GeoIP is a terminal shortcut in provider chain.
                    return 0;
                }
                AsnProviderConfig::Flow => asn = flow_as,
                AsnProviderConfig::FlowExceptPrivate => {
                    asn = flow_as;
                    if is_private_as(asn) {
                        asn = 0;
                    }
                }
                AsnProviderConfig::FlowExceptDefaultRoute => {
                    asn = flow_as;
                    if flow_net_mask == 0 {
                        asn = 0;
                    }
                }
                AsnProviderConfig::Routing => asn = routing_as,
                AsnProviderConfig::RoutingExceptPrivate => {
                    asn = routing_as;
                    if is_private_as(asn) {
                        asn = 0;
                    }
                }
            }
        }
        asn
    }

    fn get_net_mask(&self, flow_mask: u8, routing_mask: u8) -> u8 {
        let mut mask = 0_u8;
        for provider in &self.net_providers {
            if mask != 0 {
                break;
            }
            match provider {
                NetProviderConfig::Flow => mask = flow_mask,
                NetProviderConfig::Routing => mask = routing_mask,
            }
        }
        mask
    }

    fn get_next_hop(
        &self,
        flow_next_hop: Option<IpAddr>,
        routing_next_hop: Option<IpAddr>,
    ) -> Option<IpAddr> {
        let mut next_hop: Option<IpAddr> = None;
        for provider in &self.net_providers {
            if let Some(value) = next_hop
                && !value.is_unspecified()
            {
                break;
            }
            match provider {
                NetProviderConfig::Flow => {
                    next_hop = flow_next_hop;
                }
                NetProviderConfig::Routing => {
                    next_hop = routing_next_hop;
                }
            }
        }
        next_hop.filter(|addr| !addr.is_unspecified())
    }

    fn get_cached_exporter_classification(
        &self,
        exporter: &ExporterInfo,
    ) -> Option<ExporterClassification> {
        let Ok(mut cache) = self.exporter_classifier_cache.lock() else {
            return None;
        };
        let now = Instant::now();
        if let Some(entry) = cache.get_mut(exporter) {
            if now.duration_since(entry.last_access) <= self.classifier_cache_duration {
                entry.last_access = now;
                return Some(entry.value.clone());
            }
        }
        cache.remove(exporter);
        None
    }

    fn set_cached_exporter_classification(
        &self,
        exporter: &ExporterInfo,
        classification: &ExporterClassification,
    ) {
        let Ok(mut cache) = self.exporter_classifier_cache.lock() else {
            return;
        };
        cache.insert(
            exporter.clone(),
            TimedClassifierEntry {
                value: classification.clone(),
                last_access: Instant::now(),
            },
        );
    }

    fn get_cached_interface_classification(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
    ) -> Option<InterfaceClassification> {
        let key = ExporterAndInterfaceInfo {
            exporter: exporter.clone(),
            interface: interface.clone(),
        };
        let Ok(mut cache) = self.interface_classifier_cache.lock() else {
            return None;
        };
        let now = Instant::now();
        if let Some(entry) = cache.get_mut(&key) {
            if now.duration_since(entry.last_access) <= self.classifier_cache_duration {
                entry.last_access = now;
                return Some(entry.value.clone());
            }
        }
        cache.remove(&key);
        None
    }

    fn set_cached_interface_classification(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &InterfaceClassification,
    ) {
        let key = ExporterAndInterfaceInfo {
            exporter: exporter.clone(),
            interface: interface.clone(),
        };
        let Ok(mut cache) = self.interface_classifier_cache.lock() else {
            return;
        };
        cache.insert(
            key,
            TimedClassifierEntry {
                value: classification.clone(),
                last_access: Instant::now(),
            },
        );
    }

    fn classify_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            return !classification.reject;
        }
        if self.exporter_classifiers.is_empty() {
            return true;
        }

        if let Some(cached) = self.get_cached_exporter_classification(exporter) {
            *classification = cached;
            return !classification.reject;
        }

        for rule in &self.exporter_classifiers {
            if rule.evaluate_exporter(exporter, classification).is_err() {
                break;
            }
            if classification.is_complete() {
                break;
            }
        }

        self.set_cached_exporter_classification(exporter, classification);
        !classification.reject
    }

    fn classify_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return !classification.reject;
        }

        if self.interface_classifiers.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return true;
        }

        if let Some(cached) = self.get_cached_interface_classification(exporter, interface) {
            *classification = cached;
            return !classification.reject;
        }

        for rule in &self.interface_classifiers {
            if rule
                .evaluate_interface(exporter, interface, classification)
                .is_err()
            {
                break;
            }
            if !classification.connectivity.is_empty()
                && !classification.provider.is_empty()
                && classification.boundary != 0
            {
                break;
            }
        }

        if classification.name.is_empty() {
            classification.name = interface.name.clone();
        }
        if classification.description.is_empty() {
            classification.description = interface.description.clone();
        }

        self.set_cached_interface_classification(exporter, interface, classification);
        !classification.reject
    }
}

#[cfg(test)]
#[path = "enrichment_tests.rs"]
mod tests;
