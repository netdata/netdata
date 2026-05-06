use super::*;

pub(crate) type GeoIpDatabaseReader = Reader<Vec<u8>>;

#[derive(Debug)]
pub(crate) struct GeoIpResolver {
    pub(crate) asn_paths: Vec<String>,
    pub(crate) geo_paths: Vec<String>,
    pub(crate) optional: bool,
    pub(crate) asn_databases: Vec<GeoIpDatabaseReader>,
    pub(crate) geo_databases: Vec<GeoIpDatabaseReader>,
    pub(crate) signature: GeoIpDatabasesSignature,
    pub(crate) last_reload_check: Instant,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct GeoIpDatabasesSignature {
    pub(crate) asn: Vec<Option<GeoIpFileSignature>>,
    pub(crate) geo: Vec<Option<GeoIpFileSignature>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct GeoIpFileSignature {
    pub(crate) modified_usec: u64,
    pub(crate) size: u64,
}

#[derive(Debug, Deserialize)]
pub(crate) struct AsnLookupRecord {
    #[serde(default)]
    pub(crate) autonomous_system_number: Option<u32>,
    #[serde(default)]
    pub(crate) autonomous_system_organization: Option<String>,
    #[serde(default)]
    pub(crate) asn: Option<String>,
    #[serde(default)]
    pub(crate) netdata: NetdataLookupRecord,
}

#[derive(Debug, Default, Deserialize)]
pub(crate) struct NetdataLookupRecord {
    #[serde(default)]
    pub(crate) ip_class: String,
}

#[derive(Debug, Deserialize)]
pub(crate) struct GeoLookupRecord {
    #[serde(default)]
    pub(crate) country: Option<CountryValue>,
    #[serde(default)]
    pub(crate) city: Option<CityValue>,
    #[serde(default)]
    pub(crate) location: Option<LocationValue>,
    #[serde(default)]
    pub(crate) subdivisions: Vec<SubdivisionValue>,
    #[serde(default)]
    pub(crate) region: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub(crate) enum CountryValue {
    Structured {
        #[serde(default)]
        iso_code: Option<String>,
    },
    Plain(String),
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub(crate) enum CityValue {
    Structured {
        #[serde(default)]
        names: HashMap<String, String>,
    },
    Plain(String),
}

#[derive(Debug, Deserialize)]
pub(crate) struct LocationValue {
    #[serde(default)]
    pub(crate) latitude: Option<f64>,
    #[serde(default)]
    pub(crate) longitude: Option<f64>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct SubdivisionValue {
    #[serde(default)]
    pub(crate) iso_code: Option<String>,
}
