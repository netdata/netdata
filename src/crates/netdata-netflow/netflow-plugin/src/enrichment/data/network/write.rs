use super::*;

#[cfg(test)]
pub(crate) struct SideKeys {
    pub(crate) as_name: &'static str,
    pub(crate) net_name: &'static str,
    pub(crate) net_role: &'static str,
    pub(crate) net_site: &'static str,
    pub(crate) net_region: &'static str,
    pub(crate) net_tenant: &'static str,
    pub(crate) country: &'static str,
    pub(crate) geo_city: &'static str,
    pub(crate) geo_state: &'static str,
    pub(crate) geo_latitude: &'static str,
    pub(crate) geo_longitude: &'static str,
}

#[cfg(test)]
pub(crate) const SRC_KEYS: SideKeys = SideKeys {
    as_name: "SRC_AS_NAME",
    net_name: "SRC_NET_NAME",
    net_role: "SRC_NET_ROLE",
    net_site: "SRC_NET_SITE",
    net_region: "SRC_NET_REGION",
    net_tenant: "SRC_NET_TENANT",
    country: "SRC_COUNTRY",
    geo_city: "SRC_GEO_CITY",
    geo_state: "SRC_GEO_STATE",
    geo_latitude: "SRC_GEO_LATITUDE",
    geo_longitude: "SRC_GEO_LONGITUDE",
};

#[cfg(test)]
pub(crate) const DST_KEYS: SideKeys = SideKeys {
    as_name: "DST_AS_NAME",
    net_name: "DST_NET_NAME",
    net_role: "DST_NET_ROLE",
    net_site: "DST_NET_SITE",
    net_region: "DST_NET_REGION",
    net_tenant: "DST_NET_TENANT",
    country: "DST_COUNTRY",
    geo_city: "DST_GEO_CITY",
    geo_state: "DST_GEO_STATE",
    geo_latitude: "DST_GEO_LATITUDE",
    geo_longitude: "DST_GEO_LONGITUDE",
};

#[cfg(test)]
pub(crate) fn write_network_attributes(
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
    fields.insert(keys.geo_latitude, attrs.latitude);
    fields.insert(keys.geo_longitude, attrs.longitude);
}

pub(crate) fn write_network_attributes_record_src(
    rec: &mut FlowRecord,
    attrs: Option<&NetworkAttributes>,
) {
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
    rec.src_geo_latitude = attrs.latitude;
    rec.src_geo_longitude = attrs.longitude;
}

pub(crate) fn write_network_attributes_record_dst(
    rec: &mut FlowRecord,
    attrs: Option<&NetworkAttributes>,
) {
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
    rec.dst_geo_latitude = attrs.latitude;
    rec.dst_geo_longitude = attrs.longitude;
}
