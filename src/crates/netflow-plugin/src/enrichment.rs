#[cfg(test)]
use crate::flow::FlowFields;
use crate::flow::FlowRecord;
use crate::network_sources::NetworkSourcesRuntime;
use crate::plugin_config::{
    AsnProviderConfig, EnrichmentConfig, GeoIpConfig, NetProviderConfig, NetworkAttributesConfig,
    NetworkAttributesValue, SamplingRateSetting, StaticExporterConfig, StaticInterfaceConfig,
    StaticRoutingConfig, StaticRoutingEntryConfig, StaticRoutingLargeCommunityConfig,
};
use crate::routing::DynamicRoutingRuntime;
#[cfg(test)]
use crate::routing::{DynamicRoutingPeerKey, DynamicRoutingUpdate};
use anyhow::{Context, Result};
use ipnet::IpNet;
use maxminddb::{Mmap, Reader};
use regex::Regex;
use serde::Deserialize;
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::net::IpAddr;
use std::str::FromStr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, UNIX_EPOCH};

mod apply;
mod classifiers;
mod classify;
mod data;
mod init;
mod resolve;

pub(crate) use classifiers::*;
pub(crate) use data::*;

const GEOIP_RELOAD_CHECK_INTERVAL: Duration = Duration::from_secs(30);
pub(crate) const PRIVATE_IP_ADDRESS_SPACE_LABEL: &str = "Private IP Address Space";
pub(crate) const UNKNOWN_ASN_LABEL: &str = "Unknown ASN";
const CLASSIFIER_CACHE_PRUNE_INTERVAL: Duration = Duration::from_secs(30);

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

#[derive(Debug, Clone)]
struct TimedClassifierEntry<T> {
    value: T,
    last_access: Instant,
}

#[derive(Debug)]
struct TimedClassifierCache<K, V> {
    entries: HashMap<K, TimedClassifierEntry<V>>,
    last_prune: Instant,
}

impl<K, V> Default for TimedClassifierCache<K, V> {
    fn default() -> Self {
        Self {
            entries: HashMap::new(),
            last_prune: Instant::now(),
        }
    }
}

type ExporterClassifierCache = TimedClassifierCache<ExporterInfo, ExporterClassification>;
type InterfaceClassifierCache =
    TimedClassifierCache<ExporterAndInterfaceInfo, InterfaceClassification>;

#[cfg(test)]
mod tests;
