#[derive(Debug, Default)]
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
        exporters.finalize();
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
    pub(crate) asn_name: String,
    pub(crate) ip_class: String,
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
            asn_name: String::new(),
            ip_class: String::new(),
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
            && self.asn_name.is_empty()
            && self.ip_class.is_empty()
    }

    fn merge_from(&mut self, overlay: &Self) {
        if overlay.asn != 0 {
            self.asn = overlay.asn;
            self.asn_name = overlay.asn_name.clone();
        }
        if overlay.asn == 0 && !overlay.asn_name.is_empty() {
            self.asn_name = overlay.asn_name.clone();
        }
        if !overlay.ip_class.is_empty() {
            self.ip_class = overlay.ip_class.clone();
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
    out.finalize();
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
    autonomous_system_organization: Option<String>,
    #[serde(default)]
    asn: Option<String>,
    #[serde(default)]
    netdata: NetdataLookupRecord,
}

#[derive(Debug, Default, Deserialize)]
struct NetdataLookupRecord {
    #[serde(default)]
    ip_class: String,
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
            if let Ok(record) = db.lookup::<AsnLookupRecord>(address) {
                if let Some(asn) = decode_asn_record(&record) {
                    out.asn = asn;
                }
                if let Some(asn_name) = decode_asn_name(&record) {
                    out.asn_name = asn_name;
                }
                if let Some(ip_class) = decode_ip_class(&record) {
                    out.ip_class = ip_class;
                }
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

fn decode_asn_name(record: &AsnLookupRecord) -> Option<String> {
    record
        .autonomous_system_organization
        .as_deref()
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .map(str::to_string)
}

fn decode_ip_class(record: &AsnLookupRecord) -> Option<String> {
    let ip_class = record.netdata.ip_class.trim();
    (!ip_class.is_empty()).then(|| ip_class.to_string())
}

fn format_as_name(asn: u32, name: &str) -> String {
    let name = name.trim();
    if name.is_empty() {
        format!("AS{asn}")
    } else {
        format!("AS{asn} {name}")
    }
}

fn effective_as_name(attrs: Option<&NetworkAttributes>, resolved_asn: u32) -> String {
    let Some(attrs) = attrs else {
        return if resolved_asn == 0 {
            format_as_name(0, UNKNOWN_ASN_LABEL)
        } else {
            format_as_name(resolved_asn, "")
        };
    };

    if resolved_asn == 0 && attrs.ip_class == "private" {
        format_as_name(0, PRIVATE_IP_ADDRESS_SPACE_LABEL)
    } else if resolved_asn == 0 {
        format_as_name(0, UNKNOWN_ASN_LABEL)
    } else {
        format_as_name(resolved_asn, &attrs.asn_name)
    }
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
        prefixes.finalize();

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

/// IP prefix map optimized for longest-prefix-match lookups.
///
/// Entries are separated by address family (IPv4 vs IPv6) and sorted by prefix length
/// descending. Lookup returns the first match, which is the longest prefix match.
/// This avoids scanning entries of the wrong address family and terminates early.
#[derive(Debug, Clone, Default)]
struct PrefixMap<T> {
    // Sorted by prefix_len descending after finalize() for longest-match-first.
    v4_entries: Vec<PrefixMapEntry<T>>,
    v6_entries: Vec<PrefixMapEntry<T>>,
}

impl<T> PrefixMap<T> {
    fn insert(&mut self, prefix: IpNet, value: T) {
        let entry = PrefixMapEntry { prefix, value };
        match prefix {
            IpNet::V4(_) => self.v4_entries.push(entry),
            IpNet::V6(_) => self.v6_entries.push(entry),
        }
    }

    /// Sort entries by prefix length descending. Must be called after all inserts.
    fn finalize(&mut self) {
        self.v4_entries
            .sort_by(|a, b| b.prefix.prefix_len().cmp(&a.prefix.prefix_len()));
        self.v6_entries
            .sort_by(|a, b| b.prefix.prefix_len().cmp(&a.prefix.prefix_len()));
    }

    fn is_empty(&self) -> bool {
        self.v4_entries.is_empty() && self.v6_entries.is_empty()
    }

    /// Longest-prefix-match: returns the value associated with the most specific prefix.
    /// O(n) worst case but terminates on first match due to descending sort order.
    fn lookup(&self, address: IpAddr) -> Option<&T> {
        let entries = match address {
            IpAddr::V4(_) => &self.v4_entries,
            IpAddr::V6(_) => &self.v6_entries,
        };
        for entry in entries {
            if entry.prefix.contains(&address) {
                return Some(&entry.value);
            }
        }
        None
    }

    /// Iterate all matching entries in ascending prefix length order (least specific first).
    /// Used by resolve_network_attributes where all matching prefixes must be merged.
    fn matching_entries_ascending(
        &self,
        address: IpAddr,
    ) -> impl Iterator<Item = &PrefixMapEntry<T>> {
        let entries = match address {
            IpAddr::V4(_) => &self.v4_entries,
            IpAddr::V6(_) => &self.v6_entries,
        };
        // Entries are sorted descending, so reverse iteration gives ascending order.
        entries
            .iter()
            .rev()
            .filter(move |entry| entry.prefix.contains(&address))
    }
}

fn apply_network_asn_override(current_asn: u32, network_asn: u32) -> u32 {
    if network_asn != 0 {
        network_asn
    } else {
        current_asn
    }
}

/// Pre-defined static keys for SRC/DST network attribute fields.
#[cfg(test)]
struct SideKeys {
    as_name: &'static str,
    net_name: &'static str,
    net_role: &'static str,
    net_site: &'static str,
    net_region: &'static str,
    net_tenant: &'static str,
    country: &'static str,
    geo_city: &'static str,
    geo_state: &'static str,
}

#[cfg(test)]
const SRC_KEYS: SideKeys = SideKeys {
    as_name: "SRC_AS_NAME",
    net_name: "SRC_NET_NAME",
    net_role: "SRC_NET_ROLE",
    net_site: "SRC_NET_SITE",
    net_region: "SRC_NET_REGION",
    net_tenant: "SRC_NET_TENANT",
    country: "SRC_COUNTRY",
    geo_city: "SRC_GEO_CITY",
    geo_state: "SRC_GEO_STATE",
};

#[cfg(test)]
const DST_KEYS: SideKeys = SideKeys {
    as_name: "DST_AS_NAME",
    net_name: "DST_NET_NAME",
    net_role: "DST_NET_ROLE",
    net_site: "DST_NET_SITE",
    net_region: "DST_NET_REGION",
    net_tenant: "DST_NET_TENANT",
    country: "DST_COUNTRY",
    geo_city: "DST_GEO_CITY",
    geo_state: "DST_GEO_STATE",
};

#[cfg(test)]
fn write_network_attributes(
    fields: &mut FlowFields,
    keys: &SideKeys,
    attrs: Option<&NetworkAttributes>,
    resolved_asn: u32,
) {
    let attrs = attrs.cloned().unwrap_or_default();
    fields.insert(keys.as_name, effective_as_name(Some(&attrs), resolved_asn));
    fields.insert(keys.net_name, attrs.name);
    fields.insert(keys.net_role, attrs.role);
    fields.insert(keys.net_site, attrs.site);
    fields.insert(keys.net_region, attrs.region);
    fields.insert(keys.net_tenant, attrs.tenant);
    fields.insert(keys.country, attrs.country);
    fields.insert(keys.geo_city, attrs.city);
    fields.insert(keys.geo_state, attrs.state);
}

// ---------------------------------------------------------------------------
// FlowRecord-native helpers for enrich_record
// ---------------------------------------------------------------------------

fn write_network_attributes_record_src(rec: &mut FlowRecord, attrs: Option<&NetworkAttributes>) {
    let attrs = attrs.cloned().unwrap_or_default();
    rec.src_as_name = effective_as_name(Some(&attrs), rec.src_as);
    rec.src_net_name = attrs.name;
    rec.src_net_role = attrs.role;
    rec.src_net_site = attrs.site;
    rec.src_net_region = attrs.region;
    rec.src_net_tenant = attrs.tenant;
    rec.src_country = attrs.country;
    rec.src_geo_city = attrs.city;
    rec.src_geo_state = attrs.state;
}

fn write_network_attributes_record_dst(rec: &mut FlowRecord, attrs: Option<&NetworkAttributes>) {
    let attrs = attrs.cloned().unwrap_or_default();
    rec.dst_as_name = effective_as_name(Some(&attrs), rec.dst_as);
    rec.dst_net_name = attrs.name;
    rec.dst_net_role = attrs.role;
    rec.dst_net_site = attrs.site;
    rec.dst_net_region = attrs.region;
    rec.dst_net_tenant = attrs.tenant;
    rec.dst_country = attrs.country;
    rec.dst_geo_city = attrs.city;
    rec.dst_geo_state = attrs.state;
}

/// Append u32 values as CSV to a String field.
fn append_u32_csv(target: &mut String, values: &[u32]) {
    if values.is_empty() {
        return;
    }
    for v in values {
        if !target.is_empty() {
            target.push(',');
        }
        // itoa is available in Cargo.toml
        let mut buf = itoa::Buffer::new();
        target.push_str(buf.format(*v));
    }
}

/// Append large communities as CSV to a String field.
fn append_large_communities_csv(target: &mut String, values: &[StaticRoutingLargeCommunity]) {
    for lc in values {
        if !target.is_empty() {
            target.push(',');
        }
        target.push_str(&lc.format());
    }
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

    out.finalize();
    Ok(out)
}

fn parse_prefix(prefix: &str) -> Result<IpNet> {
    IpNet::from_str(prefix).with_context(|| format!("invalid prefix '{prefix}'"))
}

#[cfg(test)]
fn parse_exporter_ip(fields: &FlowFields) -> Option<IpAddr> {
    fields
        .get("EXPORTER_IP")
        .and_then(|value| value.parse::<IpAddr>().ok())
}

#[cfg(test)]
fn parse_u16_field(fields: &FlowFields, key: &str) -> u16 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0)
}

#[cfg(test)]
fn parse_u8_field(fields: &FlowFields, key: &str) -> u8 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u8>().ok())
        .unwrap_or(0)
}

#[cfg(test)]
fn parse_u32_field(fields: &FlowFields, key: &str) -> u32 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u32>().ok())
        .unwrap_or(0)
}

#[cfg(test)]
fn parse_u64_field(fields: &FlowFields, key: &str) -> u64 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

#[cfg(test)]
fn parse_ip_field(fields: &FlowFields, key: &str) -> Option<IpAddr> {
    fields
        .get(key)
        .and_then(|value| value.parse::<IpAddr>().ok())
}

#[cfg(test)]
fn append_u32_list_field(fields: &mut FlowFields, key: &'static str, values: &[u32]) {
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

#[cfg(test)]
fn append_large_communities_field(
    fields: &mut FlowFields,
    key: &'static str,
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

#[cfg(test)]
fn append_csv_field(fields: &mut FlowFields, key: &'static str, suffix: &str) {
    if suffix.is_empty() {
        return;
    }

    let entry = fields.entry(key).or_default();
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
