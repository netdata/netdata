use super::*;

#[derive(Debug, Clone, Default)]
pub(crate) struct NetworkAttributes {
    pub(crate) name: String,
    pub(crate) role: String,
    pub(crate) site: String,
    pub(crate) region: String,
    pub(crate) country: String,
    pub(crate) state: String,
    pub(crate) city: String,
    pub(crate) latitude: String,
    pub(crate) longitude: String,
    pub(crate) tenant: String,
    pub(crate) asn: u32,
    pub(crate) asn_name: String,
    pub(crate) ip_class: String,
}

impl NetworkAttributes {
    pub(crate) fn from_config(config: &NetworkAttributesConfig) -> Self {
        Self {
            name: config.name.clone(),
            role: config.role.clone(),
            site: config.site.clone(),
            region: config.region.clone(),
            country: config.country.clone(),
            state: config.state.clone(),
            city: config.city.clone(),
            latitude: normalized_coordinate(config.latitude, -90.0, 90.0).unwrap_or_default(),
            longitude: normalized_coordinate(config.longitude, -180.0, 180.0).unwrap_or_default(),
            tenant: config.tenant.clone(),
            asn: config.asn,
            asn_name: String::new(),
            ip_class: String::new(),
        }
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.name.is_empty()
            && self.role.is_empty()
            && self.site.is_empty()
            && self.region.is_empty()
            && self.country.is_empty()
            && self.state.is_empty()
            && self.city.is_empty()
            && self.latitude.is_empty()
            && self.longitude.is_empty()
            && self.tenant.is_empty()
            && self.asn == 0
            && self.asn_name.is_empty()
            && self.ip_class.is_empty()
    }

    pub(crate) fn merge_from(&mut self, overlay: &Self) {
        if overlay.asn != 0 {
            self.asn = overlay.asn;
            self.asn_name.clone_from(&overlay.asn_name);
        }
        if overlay.asn == 0 && !overlay.asn_name.is_empty() {
            self.asn_name.clone_from(&overlay.asn_name);
        }
        if !overlay.ip_class.is_empty() {
            self.ip_class.clone_from(&overlay.ip_class);
        }
        if !overlay.name.is_empty() {
            self.name.clone_from(&overlay.name);
        }
        if !overlay.role.is_empty() {
            self.role.clone_from(&overlay.role);
        }
        if !overlay.site.is_empty() {
            self.site.clone_from(&overlay.site);
        }
        if !overlay.region.is_empty() {
            self.region.clone_from(&overlay.region);
        }
        if !overlay.country.is_empty() {
            self.country.clone_from(&overlay.country);
        }
        if !overlay.state.is_empty() {
            self.state.clone_from(&overlay.state);
        }
        if !overlay.city.is_empty() {
            self.city.clone_from(&overlay.city);
        }
        if !overlay.latitude.is_empty() {
            self.latitude.clone_from(&overlay.latitude);
        }
        if !overlay.longitude.is_empty() {
            self.longitude.clone_from(&overlay.longitude);
        }
        if !overlay.tenant.is_empty() {
            self.tenant.clone_from(&overlay.tenant);
        }
    }
}

pub(crate) fn build_network_attributes_map(
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
