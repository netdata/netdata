use super::*;

fn cloned_attr_field(
    attrs: Option<&NetworkAttributes>,
    select: impl FnOnce(&NetworkAttributes) -> &String,
) -> String {
    attrs.map(|attrs| select(attrs).clone()).unwrap_or_default()
}

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
    fields.insert(keys.as_name, effective_as_name(attrs, resolved_asn));
    fields.insert(keys.net_name, cloned_attr_field(attrs, |attrs| &attrs.name));
    fields.insert(keys.net_role, cloned_attr_field(attrs, |attrs| &attrs.role));
    fields.insert(keys.net_site, cloned_attr_field(attrs, |attrs| &attrs.site));
    fields.insert(
        keys.net_region,
        cloned_attr_field(attrs, |attrs| &attrs.region),
    );
    fields.insert(
        keys.net_tenant,
        cloned_attr_field(attrs, |attrs| &attrs.tenant),
    );
    fields.insert(
        keys.country,
        cloned_attr_field(attrs, |attrs| &attrs.country),
    );
    fields.insert(keys.geo_city, cloned_attr_field(attrs, |attrs| &attrs.city));
    fields.insert(
        keys.geo_state,
        cloned_attr_field(attrs, |attrs| &attrs.state),
    );
    fields.insert(
        keys.geo_latitude,
        cloned_attr_field(attrs, |attrs| &attrs.latitude),
    );
    fields.insert(
        keys.geo_longitude,
        cloned_attr_field(attrs, |attrs| &attrs.longitude),
    );
}

pub(crate) fn write_network_attributes_record_src(
    rec: &mut FlowRecord,
    attrs: Option<&NetworkAttributes>,
) {
    rec.src_as_name = effective_as_name(attrs, rec.src_as);
    rec.src_net_name = cloned_attr_field(attrs, |attrs| &attrs.name);
    rec.src_net_role = cloned_attr_field(attrs, |attrs| &attrs.role);
    rec.src_net_site = cloned_attr_field(attrs, |attrs| &attrs.site);
    rec.src_net_region = cloned_attr_field(attrs, |attrs| &attrs.region);
    rec.src_net_tenant = cloned_attr_field(attrs, |attrs| &attrs.tenant);
    rec.src_country = cloned_attr_field(attrs, |attrs| &attrs.country);
    rec.src_geo_city = cloned_attr_field(attrs, |attrs| &attrs.city);
    rec.src_geo_state = cloned_attr_field(attrs, |attrs| &attrs.state);
    rec.src_geo_latitude = cloned_attr_field(attrs, |attrs| &attrs.latitude);
    rec.src_geo_longitude = cloned_attr_field(attrs, |attrs| &attrs.longitude);
}

pub(crate) fn write_network_attributes_record_dst(
    rec: &mut FlowRecord,
    attrs: Option<&NetworkAttributes>,
) {
    rec.dst_as_name = effective_as_name(attrs, rec.dst_as);
    rec.dst_net_name = cloned_attr_field(attrs, |attrs| &attrs.name);
    rec.dst_net_role = cloned_attr_field(attrs, |attrs| &attrs.role);
    rec.dst_net_site = cloned_attr_field(attrs, |attrs| &attrs.site);
    rec.dst_net_region = cloned_attr_field(attrs, |attrs| &attrs.region);
    rec.dst_net_tenant = cloned_attr_field(attrs, |attrs| &attrs.tenant);
    rec.dst_country = cloned_attr_field(attrs, |attrs| &attrs.country);
    rec.dst_geo_city = cloned_attr_field(attrs, |attrs| &attrs.city);
    rec.dst_geo_state = cloned_attr_field(attrs, |attrs| &attrs.state);
    rec.dst_geo_latitude = cloned_attr_field(attrs, |attrs| &attrs.latitude);
    rec.dst_geo_longitude = cloned_attr_field(attrs, |attrs| &attrs.longitude);
}
