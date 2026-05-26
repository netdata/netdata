use super::*;

pub(crate) fn decode_asn_record(record: &AsnLookupRecord) -> Option<u32> {
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

pub(crate) fn decode_asn_name(record: &AsnLookupRecord) -> Option<String> {
    record
        .autonomous_system_organization
        .as_deref()
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .map(str::to_string)
}

pub(crate) fn decode_ip_class(netdata: &NetdataLookupRecord) -> Option<String> {
    let ip_class = netdata.ip_class.trim();
    (!ip_class.is_empty()).then(|| ip_class.to_string())
}

pub(crate) fn parse_asn_text(value: &str) -> Option<u32> {
    if let Some(rest) = value
        .strip_prefix("AS")
        .or_else(|| value.strip_prefix("as"))
    {
        return rest.parse::<u32>().ok();
    }
    value.parse::<u32>().ok()
}

pub(crate) fn apply_geo_record(out: &mut NetworkAttributes, record: &GeoLookupRecord) {
    if let Some(ip_class) = decode_ip_class(&record.netdata) {
        out.ip_class = ip_class;
    }
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
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(str::to_string)
    {
        out.state = state;
    }
    if let Some(location) = &record.location {
        if let Some(latitude) = normalized_coordinate(location.latitude, -90.0, 90.0) {
            out.latitude = latitude;
        }
        if let Some(longitude) = normalized_coordinate(location.longitude, -180.0, 180.0) {
            out.longitude = longitude;
        }
    }
}

pub(crate) fn country_code(value: &CountryValue) -> Option<String> {
    match value {
        CountryValue::Structured { iso_code } => iso_code.clone(),
        CountryValue::Plain(code) => Some(code.clone()),
    }
}

pub(crate) fn city_name(value: &CityValue) -> Option<String> {
    match value {
        CityValue::Structured { names } => names.get("en").cloned(),
        CityValue::Plain(name) => Some(name.clone()),
    }
}

pub(crate) fn normalized_coordinate(value: Option<f64>, min: f64, max: f64) -> Option<String> {
    let value = value?;
    if !value.is_finite() || !(min..=max).contains(&value) {
        return None;
    }
    Some(format!("{value:.6}"))
}
