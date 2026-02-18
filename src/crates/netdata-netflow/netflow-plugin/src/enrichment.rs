use crate::plugin_config::{
    AsnProviderConfig, EnrichmentConfig, GeoIpConfig, NetProviderConfig, NetworkAttributesConfig,
    NetworkAttributesValue, SamplingRateSetting, StaticExporterConfig, StaticInterfaceConfig,
    StaticRoutingConfig, StaticRoutingEntryConfig, StaticRoutingLargeCommunityConfig,
};
use anyhow::{Context, Result};
use ipnet::IpNet;
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

#[derive(Debug, Default)]
struct DynamicRoutingState {
    entries: Vec<DynamicRoutingPrefixEntry>,
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
struct DynamicRoutingPrefixEntry {
    prefix: IpNet,
    routes: Vec<DynamicRoutingRoute>,
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
        if let Some(prefix_entry) = state
            .entries
            .iter_mut()
            .find(|prefix_entry| prefix_entry.prefix == update.prefix)
        {
            if let Some(route) = prefix_entry
                .routes
                .iter_mut()
                .find(|route| route.peer == update.peer && route.route_key == update.route_key)
            {
                route.next_hop = update.next_hop;
                route.entry = entry;
                return;
            }
            prefix_entry.routes.push(DynamicRoutingRoute {
                peer: update.peer,
                route_key: update.route_key,
                next_hop: update.next_hop,
                entry,
            });
            return;
        }

        state.entries.push(DynamicRoutingPrefixEntry {
            prefix: update.prefix,
            routes: vec![DynamicRoutingRoute {
                peer: update.peer,
                route_key: update.route_key,
                next_hop: update.next_hop,
                entry,
            }],
        });
    }

    pub(crate) fn withdraw(&self, peer: &DynamicRoutingPeerKey, prefix: IpNet, route_key: &str) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        if let Some(prefix_entry) = state
            .entries
            .iter_mut()
            .find(|prefix_entry| prefix_entry.prefix == prefix)
        {
            prefix_entry
                .routes
                .retain(|route| !(&route.peer == peer && route.route_key == route_key));
        }
        state
            .entries
            .retain(|prefix_entry| !prefix_entry.routes.is_empty());
    }

    pub(crate) fn clear_peer(&self, peer: &DynamicRoutingPeerKey) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        for prefix_entry in &mut state.entries {
            prefix_entry.routes.retain(|route| &route.peer != peer);
        }
        state
            .entries
            .retain(|prefix_entry| !prefix_entry.routes.is_empty());
    }

    pub(crate) fn clear_session(&self, exporter: SocketAddr, session_id: u64) {
        let Ok(mut state) = self.state.write() else {
            return;
        };
        for prefix_entry in &mut state.entries {
            prefix_entry.routes.retain(|route| {
                route.peer.exporter != exporter || route.peer.session_id != session_id
            });
        }
        state
            .entries
            .retain(|prefix_entry| !prefix_entry.routes.is_empty());
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

        let mut best: Option<&DynamicRoutingPrefixEntry> = None;
        let mut best_len = 0_u8;
        for prefix_entry in &state.entries {
            if prefix_entry.routes.is_empty() || !prefix_entry.prefix.contains(&address) {
                continue;
            }
            let prefix_len = prefix_entry.prefix.prefix_len();
            if best.is_some() && prefix_len <= best_len {
                continue;
            }
            best = Some(prefix_entry);
            best_len = prefix_len;
        }

        let routes = best.map(|entry| &entry.routes)?;
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
        state.entries.iter().map(|entry| entry.routes.len()).sum()
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

    pub(crate) fn enrich_fields(&mut self, fields: &mut BTreeMap<String, String>) -> bool {
        let Some(exporter_ip) = parse_exporter_ip(fields) else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let in_if = parse_u32_field(fields, "IN_IF");
        let out_if = parse_u32_field(fields, "OUT_IF");

        let mut exporter_name = String::new();
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

        // Akvorado parity: reject records lacking interface identity.
        if in_if == 0 && out_if == 0 {
            return false;
        }
        // Akvorado parity: metadata lookup must yield an exporter name.
        if exporter_name.is_empty() {
            return false;
        }

        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            fields.insert("SAMPLING_RATE".to_string(), sampling_rate.to_string());
        }
        if parse_u64_field(fields, "SAMPLING_RATE") == 0 {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                fields.insert("SAMPLING_RATE".to_string(), sampling_rate.to_string());
            } else {
                // Akvorado parity: sampling rate is required after overrides/defaults.
                return false;
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

        fields.insert("SRC_MASK".to_string(), source_mask.to_string());
        fields.insert("DST_MASK".to_string(), dest_mask.to_string());
        fields.insert("SRC_AS".to_string(), source_as.to_string());
        fields.insert("DST_AS".to_string(), dest_as.to_string());
        fields.insert(
            "NEXT_HOP".to_string(),
            next_hop.map(|addr| addr.to_string()).unwrap_or_default(),
        );
        write_network_attributes(fields, "SRC", source_network.as_ref());
        write_network_attributes(fields, "DST", dest_network.as_ref());

        if let Some(dest_routing) = dest_routing {
            append_u32_list_field(fields, "DST_AS_PATH", &dest_routing.as_path);
            append_u32_list_field(fields, "DST_COMMUNITIES", &dest_routing.communities);
            append_large_communities_field(
                fields,
                "DST_LARGE_COMMUNITIES",
                &dest_routing.large_communities,
            );
        }

        fields.insert("EXPORTER_NAME".to_string(), exporter_name);
        fields.insert("EXPORTER_GROUP".to_string(), exporter_classification.group);
        fields.insert("EXPORTER_ROLE".to_string(), exporter_classification.role);
        fields.insert("EXPORTER_SITE".to_string(), exporter_classification.site);
        fields.insert(
            "EXPORTER_REGION".to_string(),
            exporter_classification.region,
        );
        fields.insert(
            "EXPORTER_TENANT".to_string(),
            exporter_classification.tenant,
        );

        fields.insert("IN_IF_NAME".to_string(), in_classification.name);
        fields.insert(
            "IN_IF_DESCRIPTION".to_string(),
            in_classification.description,
        );
        fields.insert("IN_IF_SPEED".to_string(), in_interface.speed.to_string());
        fields.insert("IN_IF_PROVIDER".to_string(), in_classification.provider);
        fields.insert(
            "IN_IF_CONNECTIVITY".to_string(),
            in_classification.connectivity,
        );
        fields.insert(
            "IN_IF_BOUNDARY".to_string(),
            in_classification.boundary.to_string(),
        );

        fields.insert("OUT_IF_NAME".to_string(), out_classification.name);
        fields.insert(
            "OUT_IF_DESCRIPTION".to_string(),
            out_classification.description,
        );
        fields.insert("OUT_IF_SPEED".to_string(), out_interface.speed.to_string());
        fields.insert("OUT_IF_PROVIDER".to_string(), out_classification.provider);
        fields.insert(
            "OUT_IF_CONNECTIVITY".to_string(),
            out_classification.connectivity,
        );
        fields.insert(
            "OUT_IF_BOUNDARY".to_string(),
            out_classification.boundary.to_string(),
        );

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

        let mut candidates: Vec<(u8, u8, NetworkAttributes)> = Vec::new();
        if let Some(runtime) = &self.network_sources_runtime
            && let Ok(records) = runtime.records.read()
        {
            for record in records.iter() {
                if record.prefix.contains(&address) {
                    candidates.push((record.prefix.prefix_len(), 0, record.attrs.clone()));
                }
            }
        }
        for entry in &self.networks.entries {
            if entry.prefix.contains(&address) {
                candidates.push((entry.prefix.prefix_len(), 1, entry.value.clone()));
            }
        }
        candidates.sort_by_key(|(prefix_len, source_priority, _)| (*prefix_len, *source_priority));
        for (_, _, attrs) in candidates {
            resolved.merge_from(&attrs);
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

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
struct ExporterInfo {
    ip: String,
    name: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
struct InterfaceInfo {
    index: u32,
    name: String,
    description: String,
    speed: u64,
    vlan: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct ExporterAndInterfaceInfo {
    exporter: ExporterInfo,
    interface: InterfaceInfo,
}

#[derive(Debug, Clone, Default)]
struct ExporterClassification {
    group: String,
    role: String,
    site: String,
    region: String,
    tenant: String,
    reject: bool,
}

impl ExporterClassification {
    fn is_empty(&self) -> bool {
        self.group.is_empty()
            && self.role.is_empty()
            && self.site.is_empty()
            && self.region.is_empty()
            && self.tenant.is_empty()
            && !self.reject
    }

    fn is_complete(&self) -> bool {
        !self.group.is_empty()
            && !self.role.is_empty()
            && !self.site.is_empty()
            && !self.region.is_empty()
            && !self.tenant.is_empty()
    }
}

#[derive(Debug, Clone, Default)]
struct InterfaceClassification {
    connectivity: String,
    provider: String,
    boundary: u8,
    reject: bool,
    name: String,
    description: String,
}

impl InterfaceClassification {
    fn is_empty(&self) -> bool {
        self.connectivity.is_empty()
            && self.provider.is_empty()
            && self.boundary == 0
            && !self.reject
            && self.name.is_empty()
            && self.description.is_empty()
    }
}

#[derive(Debug, Clone)]
struct ClassifierRule {
    expression: BoolExpr,
}

impl ClassifierRule {
    fn parse(rule: &str) -> Result<Self> {
        let expression = parse_boolean_expr(rule)?;
        Ok(Self { expression })
    }

    fn evaluate_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        self.expression.eval_exporter(exporter, classification)
    }

    fn evaluate_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        self.expression
            .eval_interface(exporter, interface, classification)
    }
}

#[derive(Debug, Clone)]
enum BoolExpr {
    Term(RuleTerm),
    And(Box<BoolExpr>, Box<BoolExpr>),
    Or(Box<BoolExpr>, Box<BoolExpr>),
    Not(Box<BoolExpr>),
}

impl BoolExpr {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => term.eval_exporter(exporter, classification),
            BoolExpr::And(left, right) => {
                if !left.eval_exporter(exporter, classification)? {
                    return Ok(false);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_exporter(exporter, classification)? {
                    return Ok(true);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Not(inner) => Ok(!inner.eval_exporter(exporter, classification)?),
        }
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => term.eval_interface(exporter, interface, classification),
            BoolExpr::And(left, right) => {
                if !left.eval_interface(exporter, interface, classification)? {
                    return Ok(false);
                }
                right.eval_interface(exporter, interface, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_interface(exporter, interface, classification)? {
                    return Ok(true);
                }
                right.eval_interface(exporter, interface, classification)
            }
            BoolExpr::Not(inner) => {
                Ok(!inner.eval_interface(exporter, interface, classification)?)
            }
        }
    }
}

#[derive(Debug, Clone)]
enum RuleTerm {
    Condition(ConditionExpr),
    Action(ActionExpr),
}

impl RuleTerm {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => condition.eval_exporter(exporter, classification),
            RuleTerm::Action(action) => action.eval_exporter(exporter, classification),
        }
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => {
                condition.eval_interface(exporter, interface, classification)
            }
            RuleTerm::Action(action) => action.eval_interface(exporter, interface, classification),
        }
    }
}

#[derive(Debug, Clone)]
enum ConditionExpr {
    Literal(bool),
    Equals(ValueExpr, ValueExpr),
    NotEquals(ValueExpr, ValueExpr),
    Greater(ValueExpr, ValueExpr),
    GreaterOrEqual(ValueExpr, ValueExpr),
    Less(ValueExpr, ValueExpr),
    LessOrEqual(ValueExpr, ValueExpr),
    In(ValueExpr, ValueExpr),
    Contains(ValueExpr, ValueExpr),
    StartsWith(ValueExpr, ValueExpr),
    EndsWith(ValueExpr, ValueExpr),
    Matches(ValueExpr, ValueExpr),
}

impl ConditionExpr {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &ExporterClassification,
    ) -> Result<bool> {
        self.eval_with_context(Some(exporter), None, Some(classification), None)
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &InterfaceClassification,
    ) -> Result<bool> {
        self.eval_with_context(Some(exporter), Some(interface), None, Some(classification))
    }

    fn eval_with_context(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<bool> {
        match self {
            ConditionExpr::Literal(value) => Ok(*value),
            ConditionExpr::Equals(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value() == right.to_string_value())
            }
            ConditionExpr::NotEquals(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value() != right.to_string_value())
            }
            ConditionExpr::Greater(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '>'"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '>'"))?;
                Ok(left_num > right_num)
            }
            ConditionExpr::GreaterOrEqual(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '>='"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '>='"))?;
                Ok(left_num >= right_num)
            }
            ConditionExpr::Less(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '<'"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '<'"))?;
                Ok(left_num < right_num)
            }
            ConditionExpr::LessOrEqual(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '<='"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '<='"))?;
                Ok(left_num <= right_num)
            }
            ConditionExpr::In(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left = left.to_string_value();
                let members = right
                    .as_list()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not a list for 'in'"))?;
                Ok(members
                    .iter()
                    .any(|candidate| candidate.to_string_value() == left))
            }
            ConditionExpr::Contains(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().contains(&right.to_string_value()))
            }
            ConditionExpr::StartsWith(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().starts_with(&right.to_string_value()))
            }
            ConditionExpr::EndsWith(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().ends_with(&right.to_string_value()))
            }
            ConditionExpr::Matches(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let pattern = right.to_string_value();
                let regex = Regex::new(&pattern)
                    .with_context(|| format!("invalid regex '{pattern}' for 'matches'"))?;
                Ok(regex.is_match(&left.to_string_value()))
            }
        }
    }
}

#[derive(Debug, Clone)]
enum ActionExpr {
    Reject,
    ClassifyExporter(ExporterTarget, ValueExpr),
    ClassifyExporterRegex(ExporterTarget, ValueExpr, ValueExpr, ValueExpr),
    ClassifyInterface(InterfaceTarget, ValueExpr),
    ClassifyInterfaceRegex(InterfaceTarget, ValueExpr, ValueExpr, ValueExpr),
    SetName(ValueExpr),
    SetDescription(ValueExpr),
    ClassifyExternal,
    ClassifyInternal,
}

impl ActionExpr {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            ActionExpr::Reject => {
                classification.reject = true;
                Ok(false)
            }
            ActionExpr::ClassifyExporter(target, value) => {
                let value = value.resolve(Some(exporter), None, Some(classification), None)?;
                let slot = classification.exporter_target_mut(target);
                if slot.is_empty() {
                    *slot = normalize_classifier_value(&value.to_string_value());
                }
                Ok(true)
            }
            ActionExpr::ClassifyExporterRegex(target, input, pattern, template) => {
                let input = input
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();
                let pattern = pattern
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();
                let template = template
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();

                let slot = classification.exporter_target_mut(target);
                if slot.is_empty() {
                    if let Some(mapped) = apply_regex_template(&input, &pattern, &template)? {
                        *slot = normalize_classifier_value(&mapped);
                        return Ok(true);
                    }
                    return Ok(false);
                }
                Ok(true)
            }
            ActionExpr::ClassifyInterface(_, _)
            | ActionExpr::ClassifyInterfaceRegex(_, _, _, _)
            | ActionExpr::SetName(_)
            | ActionExpr::SetDescription(_)
            | ActionExpr::ClassifyExternal
            | ActionExpr::ClassifyInternal => anyhow::bail!("interface action in exporter rule"),
        }
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            ActionExpr::Reject => {
                classification.reject = true;
                Ok(false)
            }
            ActionExpr::ClassifyInterface(target, value) => {
                let value =
                    value.resolve(Some(exporter), Some(interface), None, Some(classification))?;
                let slot = classification.interface_target_mut(target);
                if slot.is_empty() {
                    *slot = normalize_classifier_value(&value.to_string_value());
                }
                Ok(true)
            }
            ActionExpr::ClassifyInterfaceRegex(target, input, pattern, template) => {
                let input = input
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();
                let pattern = pattern
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();
                let template = template
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();

                let slot = classification.interface_target_mut(target);
                if slot.is_empty() {
                    if let Some(mapped) = apply_regex_template(&input, &pattern, &template)? {
                        *slot = normalize_classifier_value(&mapped);
                        return Ok(true);
                    }
                    return Ok(false);
                }
                Ok(true)
            }
            ActionExpr::SetName(value) => {
                if classification.name.is_empty() {
                    classification.name = value
                        .resolve(Some(exporter), Some(interface), None, Some(classification))?
                        .to_string_value();
                }
                Ok(true)
            }
            ActionExpr::SetDescription(value) => {
                if classification.description.is_empty() {
                    classification.description = value
                        .resolve(Some(exporter), Some(interface), None, Some(classification))?
                        .to_string_value();
                }
                Ok(true)
            }
            ActionExpr::ClassifyExternal => {
                if classification.boundary == 0 {
                    classification.boundary = 1;
                }
                Ok(true)
            }
            ActionExpr::ClassifyInternal => {
                if classification.boundary == 0 {
                    classification.boundary = 2;
                }
                Ok(true)
            }
            ActionExpr::ClassifyExporter(_, _) | ActionExpr::ClassifyExporterRegex(_, _, _, _) => {
                anyhow::bail!("exporter action in interface rule")
            }
        }
    }
}

#[derive(Debug, Clone, Copy)]
enum ExporterTarget {
    Group,
    Role,
    Site,
    Region,
    Tenant,
}

#[derive(Debug, Clone, Copy)]
enum InterfaceTarget {
    Provider,
    Connectivity,
}

impl ExporterClassification {
    fn exporter_target_mut(&mut self, target: &ExporterTarget) -> &mut String {
        match target {
            ExporterTarget::Group => &mut self.group,
            ExporterTarget::Role => &mut self.role,
            ExporterTarget::Site => &mut self.site,
            ExporterTarget::Region => &mut self.region,
            ExporterTarget::Tenant => &mut self.tenant,
        }
    }
}

impl InterfaceClassification {
    fn interface_target_mut(&mut self, target: &InterfaceTarget) -> &mut String {
        match target {
            InterfaceTarget::Provider => &mut self.provider,
            InterfaceTarget::Connectivity => &mut self.connectivity,
        }
    }
}

#[derive(Debug, Clone)]
enum ValueExpr {
    StringLiteral(String),
    NumberLiteral(i64),
    Field(FieldExpr),
    List(Vec<ValueExpr>),
    Concat(Vec<ValueExpr>),
    Format {
        pattern: Box<ValueExpr>,
        args: Vec<ValueExpr>,
    },
}

impl ValueExpr {
    fn resolve(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<ResolvedValue> {
        match self {
            ValueExpr::StringLiteral(value) => Ok(ResolvedValue::String(value.clone())),
            ValueExpr::NumberLiteral(value) => Ok(ResolvedValue::Number(*value)),
            ValueExpr::Field(field) => field.resolve(
                exporter,
                interface,
                exporter_classification,
                interface_classification,
            ),
            ValueExpr::List(items) => {
                let mut resolved = Vec::with_capacity(items.len());
                for item in items {
                    resolved.push(item.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?);
                }
                Ok(ResolvedValue::List(resolved))
            }
            ValueExpr::Concat(parts) => {
                let mut output = String::new();
                for part in parts {
                    let value = part.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?;
                    output.push_str(&value.to_string_value());
                }
                Ok(ResolvedValue::String(output))
            }
            ValueExpr::Format { pattern, args } => {
                let pattern = pattern
                    .resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?
                    .to_string_value();
                let mut resolved_args = Vec::with_capacity(args.len());
                for arg in args {
                    resolved_args.push(arg.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?);
                }
                Ok(ResolvedValue::String(format_with_percent_placeholders(
                    &pattern,
                    &resolved_args,
                )))
            }
        }
    }

    fn is_string_expression(&self) -> bool {
        match self {
            ValueExpr::StringLiteral(_) => true,
            ValueExpr::NumberLiteral(_) => false,
            ValueExpr::Field(field) => field.is_string_field(),
            ValueExpr::List(_) => false,
            ValueExpr::Concat(parts) => parts.iter().all(ValueExpr::is_string_expression),
            ValueExpr::Format { .. } => true,
        }
    }
}

#[derive(Debug, Clone)]
enum FieldExpr {
    ExporterIp,
    ExporterName,
    InterfaceIndex,
    InterfaceName,
    InterfaceDescription,
    InterfaceSpeed,
    InterfaceVlan,
    CurrentExporterGroup,
    CurrentExporterRole,
    CurrentExporterSite,
    CurrentExporterRegion,
    CurrentExporterTenant,
    CurrentInterfaceConnectivity,
    CurrentInterfaceProvider,
    CurrentInterfaceBoundary,
    CurrentInterfaceName,
    CurrentInterfaceDescription,
}

impl FieldExpr {
    fn parse(input: &str) -> Option<Self> {
        match input {
            "Exporter.IP" => Some(Self::ExporterIp),
            "Exporter.Name" => Some(Self::ExporterName),
            "Interface.Index" => Some(Self::InterfaceIndex),
            "Interface.Name" => Some(Self::InterfaceName),
            "Interface.Description" => Some(Self::InterfaceDescription),
            "Interface.Speed" => Some(Self::InterfaceSpeed),
            "Interface.VLAN" => Some(Self::InterfaceVlan),
            "CurrentClassification.Group" => Some(Self::CurrentExporterGroup),
            "CurrentClassification.Role" => Some(Self::CurrentExporterRole),
            "CurrentClassification.Site" => Some(Self::CurrentExporterSite),
            "CurrentClassification.Region" => Some(Self::CurrentExporterRegion),
            "CurrentClassification.Tenant" => Some(Self::CurrentExporterTenant),
            "CurrentClassification.Connectivity" => Some(Self::CurrentInterfaceConnectivity),
            "CurrentClassification.Provider" => Some(Self::CurrentInterfaceProvider),
            "CurrentClassification.Boundary" => Some(Self::CurrentInterfaceBoundary),
            "CurrentClassification.Name" => Some(Self::CurrentInterfaceName),
            "CurrentClassification.Description" => Some(Self::CurrentInterfaceDescription),
            _ => None,
        }
    }

    fn resolve(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<ResolvedValue> {
        match self {
            FieldExpr::ExporterIp => Ok(ResolvedValue::String(
                exporter.map(|exp| exp.ip.clone()).unwrap_or_default(),
            )),
            FieldExpr::ExporterName => Ok(ResolvedValue::String(
                exporter.map(|exp| exp.name.clone()).unwrap_or_default(),
            )),
            FieldExpr::InterfaceIndex => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.index as i64).unwrap_or(0),
            )),
            FieldExpr::InterfaceName => Ok(ResolvedValue::String(
                interface
                    .map(|iface| iface.name.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::InterfaceDescription => Ok(ResolvedValue::String(
                interface
                    .map(|iface| iface.description.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::InterfaceSpeed => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.speed as i64).unwrap_or(0),
            )),
            FieldExpr::InterfaceVlan => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.vlan as i64).unwrap_or(0),
            )),
            FieldExpr::CurrentExporterGroup => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.group.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterRole => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.role.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterSite => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.site.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterRegion => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.region.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterTenant => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.tenant.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceConnectivity => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.connectivity.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceProvider => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.provider.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceBoundary => Ok(ResolvedValue::Number(
                interface_classification
                    .map(|classification| classification.boundary as i64)
                    .unwrap_or(0),
            )),
            FieldExpr::CurrentInterfaceName => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.name.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceDescription => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.description.clone())
                    .unwrap_or_default(),
            )),
        }
    }

    fn is_string_field(&self) -> bool {
        !matches!(
            self,
            FieldExpr::InterfaceIndex
                | FieldExpr::InterfaceSpeed
                | FieldExpr::InterfaceVlan
                | FieldExpr::CurrentInterfaceBoundary
        )
    }
}

#[derive(Debug, Clone)]
enum ResolvedValue {
    String(String),
    Number(i64),
    List(Vec<ResolvedValue>),
}

impl ResolvedValue {
    fn to_string_value(&self) -> String {
        match self {
            ResolvedValue::String(value) => value.clone(),
            ResolvedValue::Number(value) => value.to_string(),
            ResolvedValue::List(values) => values
                .iter()
                .map(ResolvedValue::to_string_value)
                .collect::<Vec<_>>()
                .join(","),
        }
    }

    fn to_i64(&self) -> Option<i64> {
        match self {
            ResolvedValue::String(value) => value.parse::<i64>().ok(),
            ResolvedValue::Number(value) => Some(*value),
            ResolvedValue::List(_) => None,
        }
    }

    fn as_list(&self) -> Option<&[ResolvedValue]> {
        match self {
            ResolvedValue::List(values) => Some(values.as_slice()),
            _ => None,
        }
    }
}

fn parse_boolean_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier rule");
    }
    parse_or_expr(input)
}

fn parse_or_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "||");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "or")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_and_expr(
            iter.next()
                .expect("split_top_level for '||' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::Or(
                Box::new(left),
                Box::new(parse_and_expr(part.trim())?),
            ))
        });
    }
    parse_and_expr(input)
}

fn parse_and_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "&&");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "and")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_unary_expr(
            iter.next()
                .expect("split_top_level for '&&' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::And(
                Box::new(left),
                Box::new(parse_unary_expr(part.trim())?),
            ))
        });
    }
    parse_unary_expr(input)
}

fn parse_unary_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier expression");
    }

    if let Some(rest) = input.strip_prefix('!') {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }
    if let Some(rest) = strip_keyword_prefix(input, "not") {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }

    if let Some(inner) = strip_outer_parentheses(input) {
        return parse_boolean_expr(inner);
    }

    Ok(BoolExpr::Term(parse_rule_term(input)?))
}

fn parse_rule_term(term: &str) -> Result<RuleTerm> {
    let term = term.trim();
    if term.is_empty() {
        anyhow::bail!("empty rule term");
    }

    if term == "true" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(true)));
    }
    if term == "false" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(false)));
    }

    if let Some((name, args)) = parse_function_call(term) {
        let action = parse_action(&name, &args)?;
        return Ok(RuleTerm::Action(action));
    }

    if let Some((left, right)) = split_once_top_level(term, " startsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::StartsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " matches ") {
        let left = parse_value_expr(left.trim())?;
        let right = parse_value_expr(right.trim())?;
        validate_literal_regex_value(&right, "matches")?;
        return Ok(RuleTerm::Condition(ConditionExpr::Matches(left, right)));
    }
    if let Some((left, right)) = split_once_top_level(term, " contains ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Contains(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " endsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::EndsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " != ") {
        return Ok(RuleTerm::Condition(ConditionExpr::NotEquals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " >= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::GreaterOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " <= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::LessOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level_keyword(term, "in") {
        return Ok(RuleTerm::Condition(ConditionExpr::In(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " == ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Equals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " > ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Greater(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " < ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Less(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }

    anyhow::bail!("unsupported rule term: {term}")
}

fn parse_action(name: &str, args: &[String]) -> Result<ActionExpr> {
    match name {
        "Reject" => {
            if !args.is_empty() {
                anyhow::bail!("Reject() does not accept arguments");
            }
            Ok(ActionExpr::Reject)
        }
        "Classify" | "ClassifyGroup" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Group,
            one_string_arg(name, args)?,
        )),
        "ClassifyRole" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Role,
            one_string_arg(name, args)?,
        )),
        "ClassifySite" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Site,
            one_string_arg(name, args)?,
        )),
        "ClassifyRegion" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Region,
            one_string_arg(name, args)?,
        )),
        "ClassifyTenant" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Tenant,
            one_string_arg(name, args)?,
        )),
        "ClassifyRegex" | "ClassifyGroupRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Group,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyRoleRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Role,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifySiteRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Site,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyRegionRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Region,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyTenantRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Tenant,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyProvider" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Provider,
            one_string_arg(name, args)?,
        )),
        "ClassifyConnectivity" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Connectivity,
            one_string_arg(name, args)?,
        )),
        "ClassifyProviderRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Provider,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyConnectivityRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Connectivity,
                arg1,
                arg2,
                arg3,
            ))
        }
        "SetName" => Ok(ActionExpr::SetName(one_string_arg(name, args)?)),
        "SetDescription" => Ok(ActionExpr::SetDescription(one_string_arg(name, args)?)),
        "ClassifyExternal" => {
            if !args.is_empty() {
                anyhow::bail!("ClassifyExternal() does not accept arguments");
            }
            Ok(ActionExpr::ClassifyExternal)
        }
        "ClassifyInternal" => {
            if !args.is_empty() {
                anyhow::bail!("ClassifyInternal() does not accept arguments");
            }
            Ok(ActionExpr::ClassifyInternal)
        }
        _ => anyhow::bail!("unsupported classifier action '{name}'"),
    }
}

fn one_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    if args.len() != 1 {
        anyhow::bail!("{name}() expects exactly 1 argument");
    }
    parse_value_expr(args[0].trim())
}

fn three_args(name: &str, args: &[String]) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    if args.len() != 3 {
        anyhow::bail!("{name}() expects exactly 3 arguments");
    }
    Ok((
        parse_value_expr(args[0].trim())?,
        parse_value_expr(args[1].trim())?,
        parse_value_expr(args[2].trim())?,
    ))
}

fn one_string_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    let value = one_arg(name, args)?;
    if !value.is_string_expression() {
        anyhow::bail!("{name}() expects a string argument");
    }
    Ok(value)
}

fn three_string_args(name: &str, args: &[String]) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    let (arg1, arg2, arg3) = three_args(name, args)?;
    if !arg1.is_string_expression() || !arg2.is_string_expression() || !arg3.is_string_expression()
    {
        anyhow::bail!("{name}() expects string arguments");
    }
    validate_literal_regex_value(&arg2, name)?;
    Ok((arg1, arg2, arg3))
}

fn validate_literal_regex_value(value: &ValueExpr, context: &str) -> Result<()> {
    if let ValueExpr::StringLiteral(pattern) = value {
        Regex::new(pattern)
            .with_context(|| format!("invalid regex '{pattern}' in {context} expression"))?;
    }
    Ok(())
}

fn parse_value_expr(input: &str) -> Result<ValueExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty expression");
    }

    let plus_parts = split_top_level(input, "+");
    if plus_parts.len() > 1 {
        let mut parts = Vec::with_capacity(plus_parts.len());
        for part in plus_parts {
            parts.push(parse_value_expr(part.trim())?);
        }
        return Ok(ValueExpr::Concat(parts));
    }

    if let Some(string) = parse_quoted_string(input) {
        return Ok(ValueExpr::StringLiteral(string));
    }
    if let Ok(number) = input.parse::<i64>() {
        return Ok(ValueExpr::NumberLiteral(number));
    }
    if let Some(items) = parse_array_literal(input)? {
        return Ok(ValueExpr::List(items));
    }
    if let Some(field) = FieldExpr::parse(input) {
        return Ok(ValueExpr::Field(field));
    }
    if let Some((name, args)) = parse_function_call(input)
        && name == "Format"
    {
        if args.is_empty() {
            anyhow::bail!("Format() expects at least one argument");
        }
        let pattern = parse_value_expr(args[0].trim())?;
        let mut fmt_args = Vec::new();
        for arg in args.iter().skip(1) {
            fmt_args.push(parse_value_expr(arg.trim())?);
        }
        return Ok(ValueExpr::Format {
            pattern: Box::new(pattern),
            args: fmt_args,
        });
    }

    anyhow::bail!("unsupported value expression: {input}")
}

fn parse_array_literal(input: &str) -> Result<Option<Vec<ValueExpr>>> {
    let input = input.trim();
    if !input.starts_with('[') || !input.ends_with(']') {
        return Ok(None);
    }

    if !is_wrapped_by_top_level_delimiters(input, '[', ']') {
        return Ok(None);
    }

    let inner = input[1..input.len() - 1].trim();
    if inner.is_empty() {
        return Ok(Some(Vec::new()));
    }

    let mut values = Vec::new();
    for item in split_top_level(inner, ",") {
        values.push(parse_value_expr(item.trim())?);
    }
    Ok(Some(values))
}

fn parse_function_call(input: &str) -> Option<(String, Vec<String>)> {
    let input = input.trim();
    if !input.ends_with(')') {
        return None;
    }
    let open = input.find('(')?;
    let name = input[..open].trim();
    if name.is_empty() {
        return None;
    }
    let args_raw = &input[open + 1..input.len() - 1];
    let args = if args_raw.trim().is_empty() {
        Vec::new()
    } else {
        split_top_level(args_raw, ",")
    };
    Some((name.to_string(), args))
}

fn parse_quoted_string(input: &str) -> Option<String> {
    if input.len() < 2 || !input.starts_with('"') || !input.ends_with('"') {
        return None;
    }
    serde_json::from_str::<String>(input).ok()
}

fn strip_outer_parentheses(input: &str) -> Option<&str> {
    let input = input.trim();
    if !is_wrapped_by_top_level_delimiters(input, '(', ')') {
        return None;
    }
    Some(input[1..input.len() - 1].trim())
}

fn split_top_level(input: &str, sep: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            parts.push(input[start..i].trim().to_string());
            i += sep.len();
            start = i;
            continue;
        }
        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

fn split_once_top_level<'a>(input: &'a str, sep: &str) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            let left = &input[..i];
            let right = &input[i + sep.len()..];
            return Some((left, right));
        }
        i += 1;
    }

    None
}

fn split_top_level_keyword(input: &str, keyword: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            parts.push(input[start..i].trim().to_string());
            i += keyword.len();
            start = i;
            continue;
        }

        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

fn split_once_top_level_keyword<'a>(input: &'a str, keyword: &str) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            let left = &input[..i];
            let right = &input[i + keyword.len()..];
            return Some((left, right));
        }
        i += 1;
    }
    None
}

fn strip_keyword_prefix<'a>(input: &'a str, keyword: &str) -> Option<&'a str> {
    let input = input.trim_start();
    let keyword_bytes = keyword.as_bytes();
    if !input.as_bytes().starts_with(keyword_bytes) {
        return None;
    }
    if !is_keyword_boundary(input, 0, keyword.len()) {
        return None;
    }
    Some(input[keyword.len()..].trim_start())
}

fn is_keyword_boundary(input: &str, start: usize, len: usize) -> bool {
    let before_ok = if start == 0 {
        true
    } else {
        input[..start]
            .chars()
            .next_back()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    let after_index = start + len;
    let after_ok = if after_index >= input.len() {
        true
    } else {
        input[after_index..]
            .chars()
            .next()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    before_ok && after_ok
}

fn is_identifier_char(ch: char) -> bool {
    ch.is_ascii_alphanumeric() || ch == '_' || ch == '.'
}

fn is_wrapped_by_top_level_delimiters(input: &str, open: char, close: char) -> bool {
    let input = input.trim();
    if !input.starts_with(open) || !input.ends_with(close) {
        return false;
    }

    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let chars: Vec<(usize, char)> = input.char_indices().collect();

    for (idx, ch) in chars.iter().copied() {
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        let current_depth = match open {
            '(' => paren_depth,
            '[' => bracket_depth,
            '{' => brace_depth,
            _ => return false,
        };
        if current_depth == 0 && idx < input.len() - ch.len_utf8() {
            return false;
        }
        if paren_depth < 0 || bracket_depth < 0 || brace_depth < 0 {
            return false;
        }
    }

    paren_depth == 0 && bracket_depth == 0 && brace_depth == 0 && !in_string && !escaped
}

fn normalize_classifier_value(input: &str) -> String {
    input
        .to_ascii_lowercase()
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric() || *ch == '.' || *ch == '+' || *ch == '-')
        .collect()
}

fn apply_regex_template(input: &str, pattern: &str, template: &str) -> Result<Option<String>> {
    let regex = Regex::new(pattern).with_context(|| format!("invalid regex '{pattern}'"))?;
    if let Some(captures) = regex.captures(input) {
        let mut output = String::new();
        captures.expand(template, &mut output);
        return Ok(Some(output));
    }
    Ok(None)
}

fn format_with_percent_placeholders(pattern: &str, args: &[ResolvedValue]) -> String {
    let mut out = String::new();
    let chars: Vec<char> = pattern.chars().collect();
    let mut idx = 0_usize;
    let mut arg_idx = 0_usize;

    while idx < chars.len() {
        let ch = chars[idx];
        if ch == '%' && idx + 1 < chars.len() {
            let spec = chars[idx + 1];
            match spec {
                's' | 'v' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_string_value());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                'd' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_i64().unwrap_or(0).to_string());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                '%' => {
                    out.push('%');
                    idx += 2;
                    continue;
                }
                _ => {}
            }
        }
        out.push(ch);
        idx += 1;
    }

    out
}

#[derive(Debug, Clone, Default)]
struct StaticMetadata {
    exporters: PrefixMap<StaticExporter>,
}

impl StaticMetadata {
    fn from_config(config: &EnrichmentConfig) -> Result<Self> {
        let mut exporters = PrefixMap::default();
        for (prefix, cfg) in &config.metadata_static.exporters {
            let parsed_prefix = parse_prefix(prefix)
                .with_context(|| format!("invalid metadata exporter prefix '{prefix}'"))?;
            exporters.insert(parsed_prefix, StaticExporter::from_config(cfg));
        }
        Ok(Self { exporters })
    }

    fn is_empty(&self) -> bool {
        self.exporters.is_empty()
    }

    fn lookup(&self, exporter_ip: IpAddr, if_index: u32) -> Option<StaticMetadataLookup<'_>> {
        let exporter = self.exporters.lookup(exporter_ip)?;
        let interface = exporter.lookup_interface(if_index)?;
        Some(StaticMetadataLookup {
            exporter,
            interface,
        })
    }
}

struct StaticMetadataLookup<'a> {
    exporter: &'a StaticExporter,
    interface: &'a StaticInterface,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct NetworkAttributes {
    pub(crate) name: String,
    pub(crate) role: String,
    pub(crate) site: String,
    pub(crate) region: String,
    pub(crate) country: String,
    pub(crate) state: String,
    pub(crate) city: String,
    pub(crate) tenant: String,
    pub(crate) asn: u32,
}

impl NetworkAttributes {
    fn from_config(config: &NetworkAttributesConfig) -> Self {
        Self {
            name: config.name.clone(),
            role: config.role.clone(),
            site: config.site.clone(),
            region: config.region.clone(),
            country: config.country.clone(),
            state: config.state.clone(),
            city: config.city.clone(),
            tenant: config.tenant.clone(),
            asn: config.asn,
        }
    }

    fn is_empty(&self) -> bool {
        self.name.is_empty()
            && self.role.is_empty()
            && self.site.is_empty()
            && self.region.is_empty()
            && self.country.is_empty()
            && self.state.is_empty()
            && self.city.is_empty()
            && self.tenant.is_empty()
            && self.asn == 0
    }

    fn merge_from(&mut self, overlay: &Self) {
        if overlay.asn != 0 {
            self.asn = overlay.asn;
        }
        if !overlay.name.is_empty() {
            self.name = overlay.name.clone();
        }
        if !overlay.role.is_empty() {
            self.role = overlay.role.clone();
        }
        if !overlay.site.is_empty() {
            self.site = overlay.site.clone();
        }
        if !overlay.region.is_empty() {
            self.region = overlay.region.clone();
        }
        if !overlay.country.is_empty() {
            self.country = overlay.country.clone();
        }
        if !overlay.state.is_empty() {
            self.state = overlay.state.clone();
        }
        if !overlay.city.is_empty() {
            self.city = overlay.city.clone();
        }
        if !overlay.tenant.is_empty() {
            self.tenant = overlay.tenant.clone();
        }
    }
}

fn build_network_attributes_map(
    entries: &BTreeMap<String, NetworkAttributesValue>,
) -> Result<PrefixMap<NetworkAttributes>> {
    let mut out = PrefixMap::default();
    for (prefix, value) in entries {
        let parsed_prefix = parse_prefix(prefix)
            .with_context(|| format!("invalid enrichment.networks prefix '{prefix}'"))?;
        let attrs = match value {
            NetworkAttributesValue::Name(name) => NetworkAttributes {
                name: name.clone(),
                ..Default::default()
            },
            NetworkAttributesValue::Attributes(config) => NetworkAttributes::from_config(config),
        };
        out.insert(parsed_prefix, attrs);
    }
    Ok(out)
}

#[derive(Debug)]
struct GeoIpResolver {
    asn_paths: Vec<String>,
    geo_paths: Vec<String>,
    optional: bool,
    asn_databases: Vec<Reader<Vec<u8>>>,
    geo_databases: Vec<Reader<Vec<u8>>>,
    signature: GeoIpDatabasesSignature,
    last_reload_check: Instant,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct GeoIpDatabasesSignature {
    asn: Vec<Option<GeoIpFileSignature>>,
    geo: Vec<Option<GeoIpFileSignature>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct GeoIpFileSignature {
    modified_usec: u64,
    size: u64,
}

#[derive(Debug, Deserialize)]
struct AsnLookupRecord {
    #[serde(default)]
    autonomous_system_number: Option<u32>,
    #[serde(default)]
    asn: Option<String>,
}

#[derive(Debug, Deserialize)]
struct GeoLookupRecord {
    #[serde(default)]
    country: Option<CountryValue>,
    #[serde(default)]
    city: Option<CityValue>,
    #[serde(default)]
    subdivisions: Vec<SubdivisionValue>,
    #[serde(default)]
    region: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum CountryValue {
    Structured {
        #[serde(default)]
        iso_code: Option<String>,
    },
    Plain(String),
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum CityValue {
    Structured {
        #[serde(default)]
        names: HashMap<String, String>,
    },
    Plain(String),
}

#[derive(Debug, Deserialize)]
struct SubdivisionValue {
    #[serde(default)]
    iso_code: Option<String>,
}

impl GeoIpResolver {
    fn from_config(config: &GeoIpConfig) -> Result<Option<Self>> {
        if config.asn_database.is_empty() && config.geo_database.is_empty() {
            return Ok(None);
        }

        let asn_databases = load_geoip_readers(
            &config.asn_database,
            "enrichment.geoip.asn_database",
            config.optional,
        )?;
        let geo_databases = load_geoip_readers(
            &config.geo_database,
            "enrichment.geoip.geo_database",
            config.optional,
        )?;

        let signature =
            build_geoip_signature(&config.asn_database, &config.geo_database, config.optional)?;

        Ok(Some(Self {
            asn_paths: config.asn_database.clone(),
            geo_paths: config.geo_database.clone(),
            optional: config.optional,
            asn_databases,
            geo_databases,
            signature,
            last_reload_check: Instant::now(),
        }))
    }

    fn refresh_if_needed(&mut self) {
        if self.last_reload_check.elapsed() < GEOIP_RELOAD_CHECK_INTERVAL {
            return;
        }
        self.last_reload_check = Instant::now();

        let Ok(next_signature) =
            build_geoip_signature(&self.asn_paths, &self.geo_paths, self.optional)
        else {
            tracing::warn!("geoip: failed to check database signatures");
            return;
        };

        if next_signature == self.signature {
            return;
        }

        match (
            load_geoip_readers(
                &self.asn_paths,
                "enrichment.geoip.asn_database",
                self.optional,
            ),
            load_geoip_readers(
                &self.geo_paths,
                "enrichment.geoip.geo_database",
                self.optional,
            ),
        ) {
            (Ok(asn_databases), Ok(geo_databases)) => {
                self.asn_databases = asn_databases;
                self.geo_databases = geo_databases;
                self.signature = next_signature;
                tracing::info!(
                    "geoip: reloaded databases (asn={}, geo={})",
                    self.asn_databases.len(),
                    self.geo_databases.len()
                );
            }
            (Err(err), _) | (_, Err(err)) => {
                tracing::warn!("geoip: reload skipped, keeping previous databases: {}", err);
            }
        }
    }

    fn lookup(&self, address: IpAddr) -> Option<NetworkAttributes> {
        let mut out = NetworkAttributes::default();

        for db in &self.asn_databases {
            if let Ok(record) = db.lookup::<AsnLookupRecord>(address)
                && let Some(asn) = decode_asn_record(&record)
            {
                out.asn = asn;
            }
        }

        for db in &self.geo_databases {
            if let Ok(record) = db.lookup::<GeoLookupRecord>(address) {
                apply_geo_record(&mut out, &record);
            }
        }

        if out.is_empty() { None } else { Some(out) }
    }
}

fn load_geoip_readers(
    paths: &[String],
    field_name: &str,
    optional: bool,
) -> Result<Vec<Reader<Vec<u8>>>> {
    let mut readers = Vec::new();
    for path in paths {
        match Reader::open_readfile(path) {
            Ok(reader) => readers.push(reader),
            Err(err) if optional => {
                tracing::warn!(
                    "{}: failed to load optional database '{}': {}",
                    field_name,
                    path,
                    err
                );
            }
            Err(err) => {
                return Err(anyhow::anyhow!(
                    "{}: failed to load database '{}': {}",
                    field_name,
                    path,
                    err
                ));
            }
        }
    }
    Ok(readers)
}

fn build_geoip_signature(
    asn_paths: &[String],
    geo_paths: &[String],
    optional: bool,
) -> Result<GeoIpDatabasesSignature> {
    Ok(GeoIpDatabasesSignature {
        asn: asn_paths
            .iter()
            .map(|path| read_geoip_file_signature(path, optional))
            .collect::<Result<Vec<_>>>()?,
        geo: geo_paths
            .iter()
            .map(|path| read_geoip_file_signature(path, optional))
            .collect::<Result<Vec<_>>>()?,
    })
}

fn read_geoip_file_signature(path: &str, optional: bool) -> Result<Option<GeoIpFileSignature>> {
    let metadata = match fs::metadata(path) {
        Ok(metadata) => metadata,
        Err(_) if optional => {
            return Ok(None);
        }
        Err(err) => {
            return Err(anyhow::anyhow!(
                "geoip: failed to stat database '{}': {}",
                path,
                err
            ));
        }
    };

    let modified = metadata.modified().unwrap_or(SystemTime::UNIX_EPOCH);
    let modified_usec = modified
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_micros() as u64;
    Ok(Some(GeoIpFileSignature {
        modified_usec,
        size: metadata.len(),
    }))
}

fn decode_asn_record(record: &AsnLookupRecord) -> Option<u32> {
    if let Some(asn) = record.autonomous_system_number
        && asn != 0
    {
        return Some(asn);
    }
    record
        .asn
        .as_deref()
        .and_then(parse_asn_text)
        .filter(|asn| *asn != 0)
}

fn parse_asn_text(value: &str) -> Option<u32> {
    if let Some(rest) = value
        .strip_prefix("AS")
        .or_else(|| value.strip_prefix("as"))
    {
        return rest.parse::<u32>().ok();
    }
    value.parse::<u32>().ok()
}

fn apply_geo_record(out: &mut NetworkAttributes, record: &GeoLookupRecord) {
    if let Some(country) = &record.country
        && let Some(code) = country_code(country)
        && !code.is_empty()
    {
        out.country = code;
    }
    if let Some(city) = &record.city
        && let Some(name) = city_name(city)
        && !name.is_empty()
    {
        out.city = name;
    }
    if let Some(state) = record
        .subdivisions
        .first()
        .and_then(|s| s.iso_code.as_deref())
        .or(record.region.as_deref())
        .map(str::to_string)
        .filter(|v| !v.is_empty())
    {
        out.state = state;
    }
}

fn country_code(value: &CountryValue) -> Option<String> {
    match value {
        CountryValue::Structured { iso_code } => iso_code.clone(),
        CountryValue::Plain(code) => Some(code.clone()),
    }
}

fn city_name(value: &CityValue) -> Option<String> {
    match value {
        CityValue::Structured { names } => names.get("en").cloned(),
        CityValue::Plain(name) => Some(name.clone()),
    }
}

#[derive(Debug, Clone, Default)]
struct StaticExporter {
    name: String,
    region: String,
    role: String,
    tenant: String,
    site: String,
    group: String,
    default_interface: StaticInterface,
    interfaces_by_index: HashMap<u32, StaticInterface>,
    skip_missing_interfaces: bool,
}

impl StaticExporter {
    fn from_config(config: &StaticExporterConfig) -> Self {
        let mut interfaces_by_index = HashMap::new();
        for (if_index, interface) in &config.if_indexes {
            interfaces_by_index.insert(*if_index, StaticInterface::from_config(interface));
        }

        Self {
            name: config.name.clone(),
            region: config.region.clone(),
            role: config.role.clone(),
            tenant: config.tenant.clone(),
            site: config.site.clone(),
            group: config.group.clone(),
            default_interface: StaticInterface::from_config(&config.default),
            interfaces_by_index,
            skip_missing_interfaces: config.skip_missing_interfaces,
        }
    }

    fn lookup_interface(&self, if_index: u32) -> Option<&StaticInterface> {
        if let Some(interface) = self.interfaces_by_index.get(&if_index) {
            return Some(interface);
        }
        if self.skip_missing_interfaces {
            return None;
        }
        Some(&self.default_interface)
    }
}

#[derive(Debug, Clone, Default)]
struct StaticInterface {
    name: String,
    description: String,
    speed: u64,
    provider: String,
    connectivity: String,
    boundary: u8,
}

impl StaticInterface {
    fn from_config(config: &StaticInterfaceConfig) -> Self {
        Self {
            name: config.name.clone(),
            description: config.description.clone(),
            speed: config.speed,
            provider: config.provider.clone(),
            connectivity: config.connectivity.clone(),
            boundary: config.boundary,
        }
    }
}

#[derive(Debug, Clone, Default)]
struct StaticRouting {
    prefixes: PrefixMap<StaticRoutingEntry>,
}

impl StaticRouting {
    fn from_config(config: &StaticRoutingConfig) -> Result<Self> {
        let mut prefixes = PrefixMap::default();
        for (prefix, entry_config) in &config.prefixes {
            let parsed_prefix = parse_prefix(prefix)
                .with_context(|| format!("invalid routing static prefix '{prefix}'"))?;
            prefixes.insert(
                parsed_prefix,
                StaticRoutingEntry::from_config(prefix, parsed_prefix, entry_config)?,
            );
        }

        Ok(Self { prefixes })
    }

    fn is_empty(&self) -> bool {
        self.prefixes.is_empty()
    }

    fn lookup(&self, address: IpAddr) -> Option<&StaticRoutingEntry> {
        self.prefixes.lookup(address)
    }
}

#[derive(Debug, Clone, Default)]
struct StaticRoutingEntry {
    asn: u32,
    as_path: Vec<u32>,
    communities: Vec<u32>,
    large_communities: Vec<StaticRoutingLargeCommunity>,
    net_mask: u8,
    next_hop: Option<IpAddr>,
}

impl StaticRoutingEntry {
    fn from_config(
        prefix: &str,
        parsed_prefix: IpNet,
        config: &StaticRoutingEntryConfig,
    ) -> Result<Self> {
        let next_hop = if config.next_hop.is_empty() {
            None
        } else {
            Some(
                config
                    .next_hop
                    .parse::<IpAddr>()
                    .with_context(|| format!("invalid routing static next_hop in '{prefix}'"))?,
            )
        };

        Ok(Self {
            asn: config.asn,
            as_path: config.as_path.clone(),
            communities: config.communities.clone(),
            large_communities: config
                .large_communities
                .iter()
                .map(StaticRoutingLargeCommunity::from_config)
                .collect(),
            net_mask: config.net_mask.unwrap_or(parsed_prefix.prefix_len()),
            next_hop,
        })
    }
}

#[derive(Debug, Clone, Default)]
struct StaticRoutingLargeCommunity {
    asn: u32,
    local_data1: u32,
    local_data2: u32,
}

impl StaticRoutingLargeCommunity {
    fn from_config(config: &StaticRoutingLargeCommunityConfig) -> Self {
        Self {
            asn: config.asn,
            local_data1: config.local_data1,
            local_data2: config.local_data2,
        }
    }

    fn format(&self) -> String {
        format!("{}:{}:{}", self.asn, self.local_data1, self.local_data2)
    }
}

#[derive(Debug, Clone)]
struct PrefixMapEntry<T> {
    prefix: IpNet,
    value: T,
}

#[derive(Debug, Clone, Default)]
struct PrefixMap<T> {
    entries: Vec<PrefixMapEntry<T>>,
}

impl<T> PrefixMap<T> {
    fn insert(&mut self, prefix: IpNet, value: T) {
        self.entries.push(PrefixMapEntry { prefix, value });
    }

    fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    fn lookup(&self, address: IpAddr) -> Option<&T> {
        let mut best: Option<(&T, u8)> = None;

        for entry in &self.entries {
            if !entry.prefix.contains(&address) {
                continue;
            }

            let prefix_len = entry.prefix.prefix_len();
            if let Some((_, current_len)) = best
                && prefix_len <= current_len
            {
                continue;
            }

            best = Some((&entry.value, prefix_len));
        }

        best.map(|(value, _)| value)
    }
}

fn apply_network_asn_override(current_asn: u32, network_asn: u32) -> u32 {
    if current_asn == 0 {
        network_asn
    } else {
        current_asn
    }
}

fn write_network_attributes(
    fields: &mut BTreeMap<String, String>,
    side: &str,
    attrs: Option<&NetworkAttributes>,
) {
    let attrs = attrs.cloned().unwrap_or_default();
    fields.insert(format!("{side}_NET_NAME"), attrs.name);
    fields.insert(format!("{side}_NET_ROLE"), attrs.role);
    fields.insert(format!("{side}_NET_SITE"), attrs.site);
    fields.insert(format!("{side}_NET_REGION"), attrs.region);
    fields.insert(format!("{side}_NET_TENANT"), attrs.tenant);
    fields.insert(format!("{side}_COUNTRY"), attrs.country);
    fields.insert(format!("{side}_GEO_CITY"), attrs.city);
    fields.insert(format!("{side}_GEO_STATE"), attrs.state);
}

fn build_sampling_map(
    sampling: Option<&SamplingRateSetting>,
    field_name: &str,
) -> Result<PrefixMap<u64>> {
    let mut out = PrefixMap::default();
    let Some(sampling) = sampling else {
        return Ok(out);
    };

    match sampling {
        SamplingRateSetting::Single(rate) => {
            out.insert(parse_prefix("0.0.0.0/0")?, *rate);
            out.insert(parse_prefix("::/0")?, *rate);
        }
        SamplingRateSetting::PerPrefix(entries) => {
            for (prefix, rate) in entries {
                let parsed_prefix = parse_prefix(prefix)
                    .with_context(|| format!("{field_name}: invalid sampling prefix '{prefix}'"))?;
                out.insert(parsed_prefix, *rate);
            }
        }
    }

    Ok(out)
}

fn parse_prefix(prefix: &str) -> Result<IpNet> {
    IpNet::from_str(prefix).with_context(|| format!("invalid prefix '{prefix}'"))
}

fn parse_exporter_ip(fields: &BTreeMap<String, String>) -> Option<IpAddr> {
    fields
        .get("EXPORTER_IP")
        .and_then(|value| value.parse::<IpAddr>().ok())
}

fn parse_u16_field(fields: &BTreeMap<String, String>, key: &str) -> u16 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0)
}

fn parse_u8_field(fields: &BTreeMap<String, String>, key: &str) -> u8 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u8>().ok())
        .unwrap_or(0)
}

fn parse_u32_field(fields: &BTreeMap<String, String>, key: &str) -> u32 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u32>().ok())
        .unwrap_or(0)
}

fn parse_u64_field(fields: &BTreeMap<String, String>, key: &str) -> u64 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

fn parse_ip_field(fields: &BTreeMap<String, String>, key: &str) -> Option<IpAddr> {
    fields
        .get(key)
        .and_then(|value| value.parse::<IpAddr>().ok())
}

fn append_u32_list_field(fields: &mut BTreeMap<String, String>, key: &str, values: &[u32]) {
    if values.is_empty() {
        return;
    }
    let serialized = values
        .iter()
        .map(u32::to_string)
        .collect::<Vec<_>>()
        .join(",");
    append_csv_field(fields, key, &serialized);
}

fn append_large_communities_field(
    fields: &mut BTreeMap<String, String>,
    key: &str,
    values: &[StaticRoutingLargeCommunity],
) {
    if values.is_empty() {
        return;
    }
    let serialized = values
        .iter()
        .map(StaticRoutingLargeCommunity::format)
        .collect::<Vec<_>>()
        .join(",");
    append_csv_field(fields, key, &serialized);
}

fn append_csv_field(fields: &mut BTreeMap<String, String>, key: &str, suffix: &str) {
    if suffix.is_empty() {
        return;
    }

    let entry = fields.entry(key.to_string()).or_default();
    if entry.is_empty() {
        *entry = suffix.to_string();
    } else {
        entry.push(',');
        entry.push_str(suffix);
    }
}

fn is_private_as(asn: u32) -> bool {
    if asn == 0 || asn == 23_456 {
        return true;
    }
    (64_496..=65_551).contains(&asn) || asn >= 4_200_000_000
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::plugin_config::{
        AsnProviderConfig, GeoIpConfig, NetProviderConfig, NetworkAttributesConfig,
        NetworkAttributesValue, RoutingDynamicBmpConfig, RoutingDynamicConfig,
        StaticExporterConfig, StaticInterfaceConfig, StaticMetadataConfig, StaticRoutingConfig,
        StaticRoutingEntryConfig, StaticRoutingLargeCommunityConfig,
    };
    use std::io::Write;
    use tempfile::tempdir;

    #[test]
    fn enricher_is_disabled_when_configuration_is_empty() {
        let cfg = EnrichmentConfig::default();
        let enricher = FlowEnricher::from_config(&cfg).expect("build enricher");
        assert!(enricher.is_none());
    }

    #[test]
    fn static_sampling_override_uses_most_specific_prefix() {
        let cfg = EnrichmentConfig {
            default_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([(
                "192.0.2.0/24".to_string(),
                100_u64,
            )]))),
            override_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([
                ("192.0.2.0/24".to_string(), 500_u64),
                ("192.0.2.128/25".to_string(), 1000_u64),
            ]))),
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.142", 10, 20, 0, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("SAMPLING_RATE").map(String::as_str),
            Some("1000")
        );
    }

    #[test]
    fn static_metadata_populates_exporter_and_interface_fields() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));

        assert_eq!(
            fields.get("EXPORTER_NAME").map(String::as_str),
            Some("edge-router")
        );
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("blue")
        );
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("eu")
        );
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("Gi10"));
        assert_eq!(fields.get("OUT_IF_NAME").map(String::as_str), Some("Gi20"));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transit-a")
        );
        assert_eq!(
            fields.get("OUT_IF_CONNECTIVITY").map(String::as_str),
            Some("peering")
        );
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
        assert_eq!(fields.get("IN_IF_SPEED").map(String::as_str), Some("1000"));
        assert_eq!(
            fields.get("OUT_IF_SPEED").map(String::as_str),
            Some("10000")
        );
    }

    #[test]
    fn metadata_classification_has_priority_over_classifiers() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            exporter_classifiers: vec![
                r#"ClassifyRegion("override-region") && ClassifyTenant("override-tenant")"#
                    .to_string(),
            ],
            interface_classifiers: vec![
                r#"ClassifyProvider("override-provider") && ClassifyExternal() && SetName("ethX")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));

        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("eu")
        );
        assert_eq!(
            fields.get("EXPORTER_TENANT").map(String::as_str),
            Some("tenant-a")
        );
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transit-a")
        );
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("Gi10"));
    }

    #[test]
    fn exporter_classifier_cache_hit_is_used_before_re_evaluation() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"ClassifyRegion("live")"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("live")
        );

        let key = ExporterInfo {
            ip: "192.0.2.10".to_string(),
            name: "edge-router".to_string(),
        };
        {
            let mut cache = enricher
                .exporter_classifier_cache
                .lock()
                .expect("lock exporter cache");
            let entry = cache.get_mut(&key).expect("cache entry");
            entry.value.region = "cached".to_string();
        }

        let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut next_fields));
        assert_eq!(
            next_fields.get("EXPORTER_REGION").map(String::as_str),
            Some("cached")
        );
    }

    #[test]
    fn exporter_classifier_cache_entry_expires_by_ttl() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"ClassifyRegion("live")"#.to_string()],
            classifier_cache_duration: Duration::from_millis(1),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("live")
        );

        let key = ExporterInfo {
            ip: "192.0.2.10".to_string(),
            name: "edge-router".to_string(),
        };
        {
            let mut cache = enricher
                .exporter_classifier_cache
                .lock()
                .expect("lock exporter cache");
            let entry = cache.get_mut(&key).expect("cache entry");
            entry.value.region = "stale-cache".to_string();
        }

        std::thread::sleep(Duration::from_millis(20));

        let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut next_fields));
        assert_eq!(
            next_fields.get("EXPORTER_REGION").map(String::as_str),
            Some("live")
        );
    }

    #[test]
    fn exporter_classifier_assigns_region() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "edge" && ClassifyRegion("EU West")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("euwest")
        );
    }

    #[test]
    fn exporter_classifier_format_uses_exporter_name() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyTenant(Format("tenant-%s", Exporter.Name))"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_TENANT").map(String::as_str),
            Some("tenant-edge-router")
        );
    }

    #[test]
    fn exporter_classifier_matches_operator_assigns_group() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name matches "^edge-.*" && Classify("europe")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("EXPORTER_NAME".to_string(), "edge-router".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("europe")
        );
    }

    #[test]
    fn exporter_classifier_regex_with_character_class_extracts_group() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyRegex(Exporter.Name, "^(\\w+).r", "europe-$1")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("europe-edge")
        );
    }

    #[test]
    fn exporter_classifier_multiline_expression_works() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                "Exporter.Name matches \"^edge-.*\" &&\nClassify(\"europe\")".to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("europe")
        );
    }

    #[test]
    fn exporter_classifier_false_rule_is_noop() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"false"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("EXPORTER_GROUP").map(String::as_str), Some(""));
        assert_eq!(fields.get("EXPORTER_REGION").map(String::as_str), Some(""));
    }

    #[test]
    fn exporter_classifier_fills_from_multiple_rules_until_complete() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "hello" && ClassifyRegion("europe")"#.to_string(),
                r#"Exporter.Name startsWith "edge" && ClassifyRegion("asia")"#.to_string(),
                r#"ClassifySite("unknown") && ClassifyTenant("alfred")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("asia")
        );
        assert_eq!(
            fields.get("EXPORTER_SITE").map(String::as_str),
            Some("unknown")
        );
        assert_eq!(
            fields.get("EXPORTER_TENANT").map(String::as_str),
            Some("alfred")
        );
    }

    #[test]
    fn exporter_classifier_reject_drops_flow() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"Reject()"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn exporter_classifier_runtime_error_stops_following_rules() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyTenant("alfred")"#.to_string(),
                r#"Exporter.Name > "hello""#.to_string(),
                r#"ClassifySite("should-not-apply")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_TENANT").map(String::as_str),
            Some("alfred")
        );
        assert_eq!(fields.get("EXPORTER_SITE").map(String::as_str), Some(""));
    }

    #[test]
    fn exporter_classifier_invalid_regex_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyRegex(Exporter.Name, "^(ebp+.r", "europe-$1")"#.to_string(),
            ],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_dynamic_regex_expression_is_allowed_at_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyRegex("something", Exporter.Name + "^(ebp+.r", "europe-$1")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_ok());
    }

    #[test]
    fn exporter_classifier_invalid_argument_type_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"Classify(1)"#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_unquoted_identifier_argument_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"Classify(hello)"#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_non_matching_regex_does_not_set_value() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"ClassifyRegex(Exporter.Name, "^(ebp+).r", "europe-$1")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("EXPORTER_GROUP").map(String::as_str), Some(""));
    }

    #[test]
    fn exporter_classifier_selective_reject_does_not_drop_flow() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "nothing" && Reject()"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn exporter_classifier_syntax_error_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"Classify("europe""#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_non_boolean_expression_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#""hello""#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_unknown_action_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"ClassifyStuff("blip")"#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn exporter_classifier_or_supports_fallback_action() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "core" && ClassifyRegion("europe") || ClassifyRegion("fallback")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("EXPORTER_NAME".to_string(), "edge-router".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("fallback")
        );
    }

    #[test]
    fn exporter_classifier_not_operator_applies_negated_condition() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"!(Exporter.Name startsWith "core") && ClassifySite("branch")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_SITE").map(String::as_str),
            Some("branch")
        );
    }

    #[test]
    fn exporter_classifier_keyword_boolean_operators_work() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"not Exporter.Name startsWith "core" and Exporter.Name startsWith "edge" and ClassifySite("branch")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("EXPORTER_NAME".to_string(), "edge-router".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_SITE").map(String::as_str),
            Some("branch")
        );
    }

    #[test]
    fn exporter_classifier_in_operator_works_for_strings() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name in ["edge-router", "core-router"] && ClassifyGroup("metro")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("metro")
        );
    }

    #[test]
    fn exporter_classifier_not_equals_operator_works() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name != "edge-router" && ClassifyRegion("other")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("EXPORTER_REGION").map(String::as_str), Some(""));
    }

    #[test]
    fn exporter_classifier_contains_operator_works() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name contains "router" && ClassifyGroup("metro")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("EXPORTER_NAME".to_string(), "edge-router".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("metro")
        );
    }

    #[test]
    fn exporter_classifier_and_has_higher_precedence_than_or() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "edge" || Exporter.Name startsWith "core" && ClassifySite("branch")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("EXPORTER_SITE").map(String::as_str), Some(""));
    }

    #[test]
    fn exporter_classifier_parentheses_override_boolean_precedence() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"(Exporter.Name startsWith "edge" || Exporter.Name startsWith "core") && ClassifySite("branch")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_SITE").map(String::as_str),
            Some("branch")
        );
    }

    #[test]
    fn interface_classifier_sets_provider_and_renames_with_format() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.Index == 10 && ClassifyProvider("Transit-101") && SetName("eth10")"#
                    .to_string(),
                r#"Interface.VLAN > 200 && SetName(Format("%s.%d", Interface.Name, Interface.VLAN))"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transit-101")
        );
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("eth10"));
        assert_eq!(
            fields.get("OUT_IF_NAME").map(String::as_str),
            Some("Gi20.300")
        );
    }

    #[test]
    fn interface_classifier_cache_hit_is_used_before_re_evaluation() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![r#"ClassifyProvider("live")"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("live")
        );

        let key = ExporterAndInterfaceInfo {
            exporter: ExporterInfo {
                ip: "192.0.2.10".to_string(),
                name: "edge-router".to_string(),
            },
            interface: InterfaceInfo {
                index: 10,
                name: "Gi10".to_string(),
                description: "10th interface".to_string(),
                speed: 1000,
                vlan: 10,
            },
        };
        {
            let mut cache = enricher
                .interface_classifier_cache
                .lock()
                .expect("lock interface cache");
            let entry = cache.get_mut(&key).expect("cache entry");
            entry.value.provider = "cached".to_string();
        }

        let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(enricher.enrich_fields(&mut next_fields));
        assert_eq!(
            next_fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("cached")
        );
    }

    #[test]
    fn interface_classifier_classify_provider_with_format_works() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"ClassifyProvider(Format("II-%s", Interface.Name))"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("ii-gi10")
        );
        assert_eq!(
            fields.get("OUT_IF_PROVIDER").map(String::as_str),
            Some("ii-gi20")
        );
    }

    #[test]
    fn interface_classifier_reject_drops_flow() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![r#"Reject()"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn interface_classifier_false_rule_is_noop() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![r#"false"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("IN_IF_PROVIDER").map(String::as_str), Some(""));
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("0"));
    }

    #[test]
    fn interface_classifier_in_operator_works_with_numeric_values() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.Index in [9, 10, 11] && ClassifyProvider("edge-range")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("edge-range")
        );
        assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
    }

    #[test]
    fn interface_classifier_sets_name_and_description() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.Index == 10 && SetName("eth10")"#.to_string(),
                r#"Interface.Index == 20 && SetDescription("uplink")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("eth10"));
        assert_eq!(
            fields.get("OUT_IF_DESCRIPTION").map(String::as_str),
            Some("uplink")
        );
    }

    #[test]
    fn interface_classifier_unquoted_identifier_argument_is_rejected_during_config_parse() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![r#"ClassifyProvider(foo)"#.to_string()],
            ..Default::default()
        };
        assert!(FlowEnricher::from_config(&cfg).is_err());
    }

    #[test]
    fn interface_classifier_vlan_equality_applies_only_to_matching_direction() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.VLAN == 100 && ClassifyExternal()"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 200);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("0"));
    }

    #[test]
    fn interface_classifier_first_write_wins_for_provider_and_boundary() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"ClassifyInternal()"#.to_string(),
                r#"ClassifyExternal()"#.to_string(),
                r#"ClassifyProvider("telia")"#.to_string(),
                r#"ClassifyProvider("cogent")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("2"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("telia")
        );
        assert_eq!(
            fields.get("OUT_IF_PROVIDER").map(String::as_str),
            Some("telia")
        );
    }

    #[test]
    fn interface_classifier_regex_and_boundary_rules_match_akvorado_expectations() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"ClassifyProvider("Othello")"#.to_string(),
                r#"ClassifyConnectivityRegex(Interface.Description, "^(1\\d*)th interface$", "P$1") && ClassifyExternal()"#
                    .to_string(),
                r#"ClassifyInternal() && ClassifyConnectivity("core")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("othello")
        );
        assert_eq!(
            fields.get("OUT_IF_PROVIDER").map(String::as_str),
            Some("othello")
        );
        assert_eq!(
            fields.get("IN_IF_CONNECTIVITY").map(String::as_str),
            Some("p10")
        );
        assert_eq!(
            fields.get("OUT_IF_CONNECTIVITY").map(String::as_str),
            Some("core")
        );
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
    }

    #[test]
    fn interface_classifier_or_with_actions_respects_short_circuit() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"(Interface.VLAN == 100 && ClassifyProvider("TransitA")) || ClassifyProvider("TransitB")"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transita")
        );
        assert_eq!(
            fields.get("OUT_IF_PROVIDER").map(String::as_str),
            Some("transitb")
        );
    }

    #[test]
    fn interface_classifier_supports_le_ge_and_lt_operators() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.VLAN <= 100 && ClassifyInternal()"#.to_string(),
                r#"Interface.VLAN >= 300 && ClassifyExternal()"#.to_string(),
                r#"Interface.VLAN < 200 && ClassifyProvider("low")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("2"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("low")
        );
        assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
    }

    #[test]
    fn interface_classifier_ends_with_operator_works() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.Name endsWith "10" && ClassifyProvider("suffix")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("suffix")
        );
        assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
    }

    #[test]
    fn parity_drops_flow_when_metadata_is_missing() {
        let cfg = EnrichmentConfig {
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn parity_drops_flow_when_sampling_rate_is_missing() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 0, 10, 300);
        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn asn_provider_order_matches_akvorado_behavior() {
        let mut enricher = test_enricher_for_provider_order();
        let cases = [
            (
                vec![AsnProviderConfig::Flow],
                12_322_u32,
                0_u32,
                24_u8,
                12_322_u32,
            ),
            (
                vec![AsnProviderConfig::FlowExceptPrivate],
                65_536_u32,
                0_u32,
                24_u8,
                0_u32,
            ),
            (
                vec![
                    AsnProviderConfig::FlowExceptPrivate,
                    AsnProviderConfig::Flow,
                ],
                65_536_u32,
                0_u32,
                24_u8,
                65_536_u32,
            ),
            (
                vec![AsnProviderConfig::FlowExceptDefaultRoute],
                12_322_u32,
                0_u32,
                0_u8,
                0_u32,
            ),
            (
                vec![AsnProviderConfig::Routing],
                12_322_u32,
                1_299_u32,
                24_u8,
                1_299_u32,
            ),
            (
                vec![AsnProviderConfig::Geoip, AsnProviderConfig::Routing],
                12_322_u32,
                65_300_u32,
                24_u8,
                0_u32,
            ),
        ];

        for (providers, flow_as, routing_as, flow_mask, expected) in cases {
            enricher.asn_providers = providers;
            assert_eq!(
                enricher.get_as_number(flow_as, routing_as, flow_mask),
                expected
            );
        }
    }

    #[test]
    fn net_mask_provider_order_matches_akvorado_behavior() {
        let mut enricher = test_enricher_for_provider_order();
        let cases = [
            (vec![NetProviderConfig::Flow], 12_u8, 24_u8, 12_u8),
            (vec![NetProviderConfig::Routing], 12_u8, 24_u8, 24_u8),
            (
                vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
                12_u8,
                24_u8,
                24_u8,
            ),
            (
                vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
                12_u8,
                24_u8,
                12_u8,
            ),
            (
                vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
                12_u8,
                0_u8,
                12_u8,
            ),
        ];

        for (providers, flow_mask, routing_mask, expected) in cases {
            enricher.net_providers = providers;
            assert_eq!(enricher.get_net_mask(flow_mask, routing_mask), expected);
        }
    }

    #[test]
    fn next_hop_provider_order_matches_akvorado_behavior() {
        let mut enricher = test_enricher_for_provider_order();
        let nh1: IpAddr = "2001:db8::1".parse().expect("parse nh1");
        let nh2: IpAddr = "2001:db8::2".parse().expect("parse nh2");
        let cases = [
            (
                vec![NetProviderConfig::Flow],
                Some(nh1),
                Some(nh2),
                Some(nh1),
            ),
            (
                vec![NetProviderConfig::Routing],
                Some(nh1),
                Some(nh2),
                Some(nh2),
            ),
            (
                vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
                Some(nh1),
                Some(nh2),
                Some(nh2),
            ),
            (
                vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
                Some(nh1),
                Some(nh2),
                Some(nh1),
            ),
            (
                vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
                None,
                None,
                None,
            ),
        ];

        for (providers, flow_nh, routing_nh, expected) in cases {
            enricher.net_providers = providers;
            assert_eq!(enricher.get_next_hop(flow_nh, routing_nh), expected);
        }
    }

    #[test]
    fn static_routing_enrichment_updates_as_mask_nexthop_and_appends_arrays() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            asn_providers: vec![AsnProviderConfig::Routing, AsnProviderConfig::Flow],
            net_providers: vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
            routing_static: StaticRoutingConfig {
                prefixes: BTreeMap::from([
                    (
                        "10.10.0.0/16".to_string(),
                        StaticRoutingEntryConfig {
                            asn: 64_500,
                            net_mask: Some(16),
                            ..Default::default()
                        },
                    ),
                    (
                        "198.51.100.0/24".to_string(),
                        StaticRoutingEntryConfig {
                            asn: 64_600,
                            as_path: vec![64_550, 64_600],
                            communities: vec![123_456, 654_321],
                            large_communities: vec![StaticRoutingLargeCommunityConfig {
                                asn: 64_600,
                                local_data1: 7,
                                local_data2: 8,
                            }],
                            next_hop: "203.0.113.9".to_string(),
                            net_mask: Some(24),
                        },
                    ),
                ]),
            },
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("SRC_ADDR".to_string(), "10.10.20.30".to_string());
        fields.insert("DST_ADDR".to_string(), "198.51.100.42".to_string());
        fields.insert("SRC_AS".to_string(), "65100".to_string());
        fields.insert("DST_AS".to_string(), "65200".to_string());
        fields.insert("SRC_MASK".to_string(), "30".to_string());
        fields.insert("DST_MASK".to_string(), "31".to_string());
        fields.insert("NEXT_HOP".to_string(), String::new());
        fields.insert("DST_AS_PATH".to_string(), "65000".to_string());
        fields.insert("DST_COMMUNITIES".to_string(), "111".to_string());
        fields.insert("DST_LARGE_COMMUNITIES".to_string(), "1:1:1".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64500"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64600"));
        assert_eq!(fields.get("SRC_MASK").map(String::as_str), Some("16"));
        assert_eq!(fields.get("DST_MASK").map(String::as_str), Some("24"));
        assert_eq!(
            fields.get("NEXT_HOP").map(String::as_str),
            Some("203.0.113.9")
        );
        assert_eq!(
            fields.get("DST_AS_PATH").map(String::as_str),
            Some("65000,64550,64600")
        );
        assert_eq!(
            fields.get("DST_COMMUNITIES").map(String::as_str),
            Some("111,123456,654321")
        );
        assert_eq!(
            fields.get("DST_LARGE_COMMUNITIES").map(String::as_str),
            Some("1:1:1,64600:7:8")
        );
    }

    #[test]
    fn dynamic_routing_runtime_prefers_exact_next_hop() {
        let runtime = DynamicRoutingRuntime::default();
        let peer_a = DynamicRoutingPeerKey {
            exporter: "192.0.2.10:10179".parse().expect("parse exporter A"),
            session_id: 1,
            peer_id: "peer-a".to_string(),
        };
        let peer_b = DynamicRoutingPeerKey {
            exporter: "192.0.2.10:10179".parse().expect("parse exporter B"),
            session_id: 1,
            peer_id: "peer-b".to_string(),
        };
        let prefix = parse_prefix("198.51.100.0/24").expect("parse prefix");
        let nh_a: IpAddr = "203.0.113.1".parse().expect("parse nh a");
        let nh_b: IpAddr = "203.0.113.2".parse().expect("parse nh b");

        runtime.upsert(DynamicRoutingUpdate {
            peer: peer_a.clone(),
            prefix,
            route_key: "route-a".to_string(),
            next_hop: Some(nh_a),
            asn: 64_500,
            as_path: vec![64_500],
            communities: vec![],
            large_communities: vec![],
        });
        runtime.upsert(DynamicRoutingUpdate {
            peer: peer_b,
            prefix,
            route_key: "route-b".to_string(),
            next_hop: Some(nh_b),
            asn: 64_600,
            as_path: vec![64_600],
            communities: vec![],
            large_communities: vec![],
        });

        let selected = runtime
            .lookup(
                "198.51.100.42".parse().expect("parse ip"),
                Some(nh_b),
                Some("192.0.2.10".parse().expect("parse exporter ip")),
            )
            .expect("route with matching next-hop");
        assert_eq!(selected.asn, 64_600);

        runtime.clear_peer(&peer_a);
        assert_eq!(runtime.route_count(), 1);
    }

    #[test]
    fn dynamic_routing_enrichment_overrides_static_when_enabled() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            asn_providers: vec![AsnProviderConfig::Routing, AsnProviderConfig::Flow],
            net_providers: vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
            routing_dynamic: RoutingDynamicConfig {
                bmp: RoutingDynamicBmpConfig {
                    enabled: true,
                    ..Default::default()
                },
                ..Default::default()
            },
            routing_static: StaticRoutingConfig {
                prefixes: BTreeMap::from([(
                    "198.51.100.0/24".to_string(),
                    StaticRoutingEntryConfig {
                        asn: 64_601,
                        net_mask: Some(24),
                        next_hop: "203.0.113.10".to_string(),
                        ..Default::default()
                    },
                )]),
            },
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let runtime = enricher
            .dynamic_routing_runtime()
            .expect("dynamic routing runtime");
        let peer = DynamicRoutingPeerKey {
            exporter: "192.0.2.10:10179".parse().expect("parse exporter"),
            session_id: 1,
            peer_id: "peer-1".to_string(),
        };
        runtime.upsert(DynamicRoutingUpdate {
            peer,
            prefix: parse_prefix("198.51.100.0/24").expect("prefix"),
            route_key: "route-1".to_string(),
            next_hop: Some("203.0.113.9".parse().expect("next hop")),
            asn: 64_700,
            as_path: vec![64_690, 64_700],
            communities: vec![100],
            large_communities: vec![(64_700, 1, 2)],
        });

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        fields.insert("SRC_ADDR".to_string(), "10.10.20.30".to_string());
        fields.insert("DST_ADDR".to_string(), "198.51.100.42".to_string());
        fields.insert("SRC_AS".to_string(), "65100".to_string());
        fields.insert("DST_AS".to_string(), "65200".to_string());
        fields.insert("SRC_MASK".to_string(), "30".to_string());
        fields.insert("DST_MASK".to_string(), "31".to_string());
        fields.insert("NEXT_HOP".to_string(), "203.0.113.9".to_string());
        fields.insert("DST_AS_PATH".to_string(), String::new());
        fields.insert("DST_COMMUNITIES".to_string(), String::new());
        fields.insert("DST_LARGE_COMMUNITIES".to_string(), String::new());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64700"));
        assert_eq!(
            fields.get("NEXT_HOP").map(String::as_str),
            Some("203.0.113.9")
        );
        assert_eq!(
            fields.get("DST_AS_PATH").map(String::as_str),
            Some("64690,64700")
        );
        assert_eq!(
            fields.get("DST_COMMUNITIES").map(String::as_str),
            Some("100")
        );
        assert_eq!(
            fields.get("DST_LARGE_COMMUNITIES").map(String::as_str),
            Some("64700:1:2")
        );
    }

    #[test]
    fn network_enrichment_populates_network_dimensions_and_asn_fallback() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            networks: BTreeMap::from([(
                "198.51.100.0/24".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    name: "edge-net".to_string(),
                    role: "customer".to_string(),
                    site: "par1".to_string(),
                    region: "eu-west".to_string(),
                    country: "FR".to_string(),
                    state: "Ile-de-France".to_string(),
                    city: "Paris".to_string(),
                    tenant: "tenant-a".to_string(),
                    asn: 64_500,
                }),
            )]),
            ..Default::default()
        };

        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
        fields.insert("SRC_ADDR".to_string(), "198.51.100.10".to_string());
        fields.insert("DST_ADDR".to_string(), "198.51.100.20".to_string());
        fields.insert("SRC_AS".to_string(), "0".to_string());
        fields.insert("DST_AS".to_string(), "0".to_string());
        fields.insert("SRC_MASK".to_string(), "24".to_string());
        fields.insert("DST_MASK".to_string(), "24".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64500"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64500"));
        assert_eq!(
            fields.get("SRC_NET_NAME").map(String::as_str),
            Some("edge-net")
        );
        assert_eq!(
            fields.get("DST_NET_NAME").map(String::as_str),
            Some("edge-net")
        );
        assert_eq!(
            fields.get("SRC_NET_ROLE").map(String::as_str),
            Some("customer")
        );
        assert_eq!(
            fields.get("DST_NET_ROLE").map(String::as_str),
            Some("customer")
        );
        assert_eq!(fields.get("SRC_NET_SITE").map(String::as_str), Some("par1"));
        assert_eq!(fields.get("DST_NET_SITE").map(String::as_str), Some("par1"));
        assert_eq!(
            fields.get("SRC_NET_REGION").map(String::as_str),
            Some("eu-west")
        );
        assert_eq!(
            fields.get("DST_NET_REGION").map(String::as_str),
            Some("eu-west")
        );
        assert_eq!(
            fields.get("SRC_NET_TENANT").map(String::as_str),
            Some("tenant-a")
        );
        assert_eq!(
            fields.get("DST_NET_TENANT").map(String::as_str),
            Some("tenant-a")
        );
        assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("FR"));
        assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("FR"));
        assert_eq!(
            fields.get("SRC_GEO_CITY").map(String::as_str),
            Some("Paris")
        );
        assert_eq!(
            fields.get("DST_GEO_CITY").map(String::as_str),
            Some("Paris")
        );
        assert_eq!(
            fields.get("SRC_GEO_STATE").map(String::as_str),
            Some("Ile-de-France")
        );
        assert_eq!(
            fields.get("DST_GEO_STATE").map(String::as_str),
            Some("Ile-de-France")
        );
    }

    #[test]
    fn network_enrichment_does_not_override_non_zero_flow_asn() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            networks: BTreeMap::from([(
                "198.51.100.0/24".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    asn: 64_500,
                    ..Default::default()
                }),
            )]),
            ..Default::default()
        };

        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
        fields.insert("SRC_ADDR".to_string(), "198.51.100.10".to_string());
        fields.insert("DST_ADDR".to_string(), "198.51.100.20".to_string());
        fields.insert("SRC_AS".to_string(), "65001".to_string());
        fields.insert("DST_AS".to_string(), "0".to_string());
        fields.insert("SRC_MASK".to_string(), "24".to_string());
        fields.insert("DST_MASK".to_string(), "24".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("65001"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64500"));
    }

    #[test]
    fn network_enrichment_merges_supernet_and_subnet_attributes() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            networks: BTreeMap::from([
                (
                    "198.51.0.0/16".to_string(),
                    NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                        region: "eu-west".to_string(),
                        country: "FR".to_string(),
                        tenant: "tenant-a".to_string(),
                        asn: 64_501,
                        ..Default::default()
                    }),
                ),
                (
                    "198.51.100.0/24".to_string(),
                    NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                        name: "edge-net".to_string(),
                        role: "customer".to_string(),
                        site: "par1".to_string(),
                        ..Default::default()
                    }),
                ),
            ]),
            ..Default::default()
        };

        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
        fields.insert("SRC_ADDR".to_string(), "198.51.100.10".to_string());
        fields.insert("DST_ADDR".to_string(), "198.51.100.20".to_string());
        fields.insert("SRC_AS".to_string(), "0".to_string());
        fields.insert("DST_AS".to_string(), "0".to_string());
        fields.insert("SRC_MASK".to_string(), "24".to_string());
        fields.insert("DST_MASK".to_string(), "24".to_string());

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("SRC_NET_NAME").map(String::as_str),
            Some("edge-net")
        );
        assert_eq!(
            fields.get("DST_NET_NAME").map(String::as_str),
            Some("edge-net")
        );
        assert_eq!(
            fields.get("SRC_NET_ROLE").map(String::as_str),
            Some("customer")
        );
        assert_eq!(
            fields.get("DST_NET_ROLE").map(String::as_str),
            Some("customer")
        );
        assert_eq!(fields.get("SRC_NET_SITE").map(String::as_str), Some("par1"));
        assert_eq!(fields.get("DST_NET_SITE").map(String::as_str), Some("par1"));
        assert_eq!(
            fields.get("SRC_NET_REGION").map(String::as_str),
            Some("eu-west")
        );
        assert_eq!(
            fields.get("DST_NET_REGION").map(String::as_str),
            Some("eu-west")
        );
        assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("FR"));
        assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("FR"));
        assert_eq!(
            fields.get("SRC_NET_TENANT").map(String::as_str),
            Some("tenant-a")
        );
        assert_eq!(
            fields.get("DST_NET_TENANT").map(String::as_str),
            Some("tenant-a")
        );
        assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64501"));
        assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64501"));
    }

    #[test]
    fn optional_geoip_missing_databases_are_accepted() {
        let cfg = GeoIpConfig {
            asn_database: vec!["/path/that/does/not/exist/asn.mmdb".to_string()],
            geo_database: vec!["/path/that/does/not/exist/geo.mmdb".to_string()],
            optional: true,
        };
        let resolver = GeoIpResolver::from_config(&cfg).expect("optional geoip config");
        assert!(resolver.is_some());
        let resolver = resolver.expect("resolver should exist");
        assert!(resolver.asn_databases.is_empty());
        assert!(resolver.geo_databases.is_empty());
    }

    #[test]
    fn geoip_signature_changes_when_database_file_changes() {
        let dir = tempdir().expect("create tempdir");
        let db_path = dir.path().join("asn.mmdb");
        {
            let mut file = std::fs::File::create(&db_path).expect("create file");
            file.write_all(b"v1").expect("write v1");
            file.sync_all().expect("sync v1");
        }
        let first = read_geoip_file_signature(db_path.to_str().expect("utf-8 path"), false)
            .expect("signature v1")
            .expect("signature should exist");

        std::thread::sleep(Duration::from_millis(2));
        {
            let mut file = std::fs::OpenOptions::new()
                .write(true)
                .truncate(true)
                .open(&db_path)
                .expect("open file");
            file.write_all(b"v2-longer").expect("write v2");
            file.sync_all().expect("sync v2");
        }
        let second = read_geoip_file_signature(db_path.to_str().expect("utf-8 path"), false)
            .expect("signature v2")
            .expect("signature should exist");

        assert_ne!(first, second);
    }

    #[test]
    fn parse_asn_text_supports_prefixed_and_numeric_values() {
        assert_eq!(parse_asn_text("AS64512"), Some(64_512));
        assert_eq!(parse_asn_text("as64513"), Some(64_513));
        assert_eq!(parse_asn_text("64514"), Some(64_514));
        assert_eq!(parse_asn_text("ASX"), None);
    }

    fn metadata_config_for_192() -> StaticMetadataConfig {
        StaticMetadataConfig {
            exporters: BTreeMap::from([(
                "192.0.2.0/24".to_string(),
                StaticExporterConfig {
                    name: "edge-router".to_string(),
                    region: "eu".to_string(),
                    role: "peering".to_string(),
                    tenant: "tenant-a".to_string(),
                    site: "par".to_string(),
                    group: "blue".to_string(),
                    default: StaticInterfaceConfig {
                        name: "Default0".to_string(),
                        description: "Default interface".to_string(),
                        speed: 1000,
                        provider: String::new(),
                        connectivity: String::new(),
                        boundary: 0,
                    },
                    if_indexes: BTreeMap::from([
                        (
                            10_u32,
                            StaticInterfaceConfig {
                                name: "Gi10".to_string(),
                                description: "10th interface".to_string(),
                                speed: 1000,
                                provider: "transit-a".to_string(),
                                connectivity: "transit".to_string(),
                                boundary: 1,
                            },
                        ),
                        (
                            20_u32,
                            StaticInterfaceConfig {
                                name: "Gi20".to_string(),
                                description: "20th interface".to_string(),
                                speed: 10000,
                                provider: "ix".to_string(),
                                connectivity: "peering".to_string(),
                                boundary: 2,
                            },
                        ),
                    ]),
                    skip_missing_interfaces: false,
                },
            )]),
        }
    }

    fn metadata_config_without_exporter_classification() -> StaticMetadataConfig {
        let mut cfg = metadata_config_for_192();
        for exporter in cfg.exporters.values_mut() {
            exporter.region.clear();
            exporter.role.clear();
            exporter.site.clear();
            exporter.group.clear();
            exporter.tenant.clear();
        }
        cfg
    }

    fn metadata_config_without_interface_classification() -> StaticMetadataConfig {
        let mut cfg = metadata_config_without_exporter_classification();
        for exporter in cfg.exporters.values_mut() {
            for iface in exporter.if_indexes.values_mut() {
                iface.provider.clear();
                iface.connectivity.clear();
                iface.boundary = 0;
            }
        }
        cfg
    }

    fn base_fields(
        exporter_ip: &str,
        in_if: u32,
        out_if: u32,
        sampling_rate: u64,
        src_vlan: u16,
        dst_vlan: u16,
    ) -> BTreeMap<String, String> {
        BTreeMap::from([
            ("EXPORTER_IP".to_string(), exporter_ip.to_string()),
            ("IN_IF".to_string(), in_if.to_string()),
            ("OUT_IF".to_string(), out_if.to_string()),
            ("SAMPLING_RATE".to_string(), sampling_rate.to_string()),
            ("SRC_VLAN".to_string(), src_vlan.to_string()),
            ("DST_VLAN".to_string(), dst_vlan.to_string()),
            ("EXPORTER_NAME".to_string(), String::new()),
        ])
    }

    fn test_enricher_for_provider_order() -> FlowEnricher {
        FlowEnricher {
            default_sampling_rate: PrefixMap::default(),
            override_sampling_rate: PrefixMap::default(),
            static_metadata: StaticMetadata::default(),
            networks: PrefixMap::default(),
            geoip: None,
            network_sources_runtime: None,
            exporter_classifiers: Vec::new(),
            interface_classifiers: Vec::new(),
            classifier_cache_duration: Duration::from_secs(5 * 60),
            exporter_classifier_cache: Arc::new(Mutex::new(HashMap::new())),
            interface_classifier_cache: Arc::new(Mutex::new(HashMap::new())),
            asn_providers: vec![
                AsnProviderConfig::Flow,
                AsnProviderConfig::Routing,
                AsnProviderConfig::Geoip,
            ],
            net_providers: vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
            static_routing: StaticRouting::default(),
            dynamic_routing: None,
        }
    }
}
