use super::*;

#[derive(Debug, Default)]
pub(crate) struct StaticMetadata {
    pub(crate) exporters: PrefixMap<StaticExporter>,
}

impl StaticMetadata {
    pub(crate) fn from_config(config: &EnrichmentConfig) -> Result<Self> {
        let mut exporters = PrefixMap::default();
        for (prefix, cfg) in &config.metadata_static.exporters {
            let parsed_prefix = parse_prefix(prefix)
                .with_context(|| format!("invalid metadata exporter prefix '{prefix}'"))?;
            exporters.insert(parsed_prefix, StaticExporter::from_config(cfg));
        }
        exporters.finalize();
        Ok(Self { exporters })
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.exporters.is_empty()
    }

    pub(crate) fn lookup(
        &self,
        exporter_ip: IpAddr,
        if_index: u32,
    ) -> Option<StaticMetadataLookup<'_>> {
        let exporter = self.exporters.lookup(exporter_ip)?;
        let interface = exporter.lookup_interface(if_index)?;
        Some(StaticMetadataLookup {
            exporter,
            interface,
        })
    }
}

pub(crate) struct StaticMetadataLookup<'a> {
    pub(crate) exporter: &'a StaticExporter,
    pub(crate) interface: &'a StaticInterface,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct StaticExporter {
    pub(crate) name: String,
    pub(crate) region: String,
    pub(crate) role: String,
    pub(crate) tenant: String,
    pub(crate) site: String,
    pub(crate) group: String,
    pub(crate) default_interface: StaticInterface,
    pub(crate) interfaces_by_index: HashMap<u32, StaticInterface>,
    pub(crate) skip_missing_interfaces: bool,
}

impl StaticExporter {
    pub(crate) fn from_config(config: &StaticExporterConfig) -> Self {
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

    pub(crate) fn lookup_interface(&self, if_index: u32) -> Option<&StaticInterface> {
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
pub(crate) struct StaticInterface {
    pub(crate) name: String,
    pub(crate) description: String,
    pub(crate) speed: u64,
    pub(crate) provider: String,
    pub(crate) connectivity: String,
    pub(crate) boundary: u8,
}

impl StaticInterface {
    pub(crate) fn from_config(config: &StaticInterfaceConfig) -> Self {
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
pub(crate) struct StaticRouting {
    pub(crate) prefixes: PrefixMap<StaticRoutingEntry>,
}

impl StaticRouting {
    pub(crate) fn from_config(config: &StaticRoutingConfig) -> Result<Self> {
        let mut prefixes = PrefixMap::default();
        for (prefix, entry_config) in &config.prefixes {
            let parsed_prefix = parse_prefix(prefix)
                .with_context(|| format!("invalid routing static prefix '{prefix}'"))?;
            prefixes.insert(
                parsed_prefix,
                StaticRoutingEntry::from_config(prefix, parsed_prefix, entry_config)?,
            );
        }
        prefixes.finalize();

        Ok(Self { prefixes })
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.prefixes.is_empty()
    }

    pub(crate) fn lookup(&self, address: IpAddr) -> Option<&StaticRoutingEntry> {
        self.prefixes.lookup(address)
    }
}

#[derive(Debug, Clone, Default)]
pub(crate) struct StaticRoutingEntry {
    pub(crate) asn: u32,
    pub(crate) as_path: Vec<u32>,
    pub(crate) communities: Vec<u32>,
    pub(crate) large_communities: Vec<StaticRoutingLargeCommunity>,
    pub(crate) net_mask: u8,
    pub(crate) next_hop: Option<IpAddr>,
}

impl StaticRoutingEntry {
    pub(crate) fn from_config(
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
pub(crate) struct StaticRoutingLargeCommunity {
    pub(crate) asn: u32,
    pub(crate) local_data1: u32,
    pub(crate) local_data2: u32,
}

impl StaticRoutingLargeCommunity {
    pub(crate) fn from_config(config: &StaticRoutingLargeCommunityConfig) -> Self {
        Self {
            asn: config.asn,
            local_data1: config.local_data1,
            local_data2: config.local_data2,
        }
    }
}
