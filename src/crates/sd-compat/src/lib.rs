// String tokenizer and parser for field names with compact encoding
//
// Tokenizes strings into words (lowercase, UPPERCASE, Capitalized) and separators (. _ -)
// Parses tokens into fields: Lowercase, Uppercase, LowerCamel, UpperCamel, Empty
// Encodes token stream into compact lossless representation
//
// Example: "log.body.HostName" → encoded: "3RAAO" (5 chars: 2-char checksum + 3-char structure)

mod encoder;
mod parser;
mod tokenizer;

use arrayvec::ArrayString;
use encoder::encode;

/// Returns the fully encoded key: `ND` + structure encoding + `_` + normalized key.
///
/// The result combines:
/// - `ND` prefix (Netdata namespace identifier)
/// - The compact structure encoding with run-length compression
/// - An underscore separator
/// - The original key converted to uppercase with dots and hyphens replaced by underscores,
///   and common prefixes shortened:
///   - `resource.attributes.` → `RA_`
///   - `log.attributes.` → `LA_`
///   - `log.body.` → `LB_`
///
/// Run-length compression replaces 3+ consecutive identical characters with count + character.
/// If the key contains camel case, a 2-character checksum is prepended to the encoding.
///
/// # MD5 Fallback
///
/// Falls back to `ND_<32-hex-chars>` format when:
/// - Input contains invalid characters (anything except a-z, A-Z, 0-9, '.', '-', '_')
/// - Encoded result would exceed 64 bytes (systemd's field name limit)
///
/// The result is guaranteed to be systemd-compatible and ≤ 64 bytes.
///
/// # Examples
///
/// ```
/// use sd_compat::remap;
///
/// // Simple lowercase field
/// assert_eq!(remap(b"hello").as_str(), "NDE_HELLO");
///
/// // With dot separators (2 A's — no compression)
/// assert_eq!(remap(b"log.body.hostname").as_str(), "NDAAE_LB_HOSTNAME");
///
/// // Many nested levels — structure compression (9 A's → 9A, then E for end)
/// assert_eq!(remap(b"my.very.deeply.nested.field.that.ends.in.the.abyss").as_str(), "ND9AE_MY_VERY_DEEPLY_NESTED_FIELD_THAT_ENDS_IN_THE_ABYSS");
///
/// // With camel case (2-char checksum + structure)
/// assert_eq!(remap(b"log.body.HostName").as_str(), "ND3RAAO_LB_HOSTNAME");
///
/// // With hyphens
/// assert_eq!(remap(b"hello-world").as_str(), "NDCE_HELLO_WORLD");
///
/// // With resource.attributes prefix — compression (3 A's → 3A)
/// assert_eq!(remap(b"resource.attributes.host.name").as_str(), "ND3AE_RA_HOST_NAME");
///
/// // With invalid characters (space) — falls back to MD5
/// let md5_result = remap(b"field name");
/// assert!(md5_result.starts_with("ND_"));
/// assert_eq!(md5_result.len(), 35); // ND_ + 32 hex chars
///
/// // Non-UTF8 — falls back to MD5
/// let non_utf8 = b"\xFF\xFE invalid";
/// let result = remap(non_utf8);
/// assert!(result.starts_with("ND_"));
/// assert_eq!(result.len(), 35);
///
/// // Long names that would exceed 64 bytes — falls back to MD5
/// let long_name = b"very.long.deeply.nested.field.name.that.would.definitely.exceed.the.systemd.limit";
/// let result = remap(long_name);
/// assert!(result.starts_with("ND_"));
/// assert!(result.len() <= 64);
/// ```
pub fn remap(key: &[u8]) -> ArrayString<64> {
    // Detect common prefix on the raw key, keep only the suffix for normalization.
    let (short_prefix, suffix) = if let Some(rest) = key.strip_prefix(b"resource.attributes.") {
        ("RA_", rest)
    } else if let Some(rest) = key.strip_prefix(b"log.attributes.") {
        ("LA_", rest)
    } else if let Some(rest) = key.strip_prefix(b"log.body.") {
        ("LB_", rest)
    } else {
        ("", key)
    };

    let mut remapped_key = ArrayString::<64>::new();
    remapped_key.push_str("ND");

    // If encoding contains non-valid bytes, or the length of the remapped
    // key would be larger than systemd's 64-byte limit, fallback to using
    // an md5 hash.
    if !encode(key, &mut remapped_key)
        || remapped_key.len() + 1 + short_prefix.len() + suffix.len() > 64
    {
        use std::fmt::Write;
        let mut result = ArrayString::<64>::new();
        write!(result, "ND_{:X}", md5::compute(key)).expect("MD5 fits in 64 bytes");
        return result;
    }

    remapped_key.push('_');
    remapped_key.push_str(short_prefix);

    // Normalize and push only the suffix: uppercase, dots and hyphens become underscores.
    for &b in suffix {
        remapped_key.push(match b {
            b'.' | b'-' => b'_',
            _ => b.to_ascii_uppercase(),
        } as char);
    }

    remapped_key
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- basic remapping ---

    #[test]
    fn remap_simple_lowercase() {
        assert_eq!(remap(b"hello").as_str(), "NDE_HELLO");
    }

    #[test]
    fn remap_dot_separated() {
        assert_eq!(remap(b"foo.bar").as_str(), "NDAE_FOO_BAR");
    }

    #[test]
    fn remap_hyphen_replaced() {
        assert_eq!(remap(b"hello-world").as_str(), "NDCE_HELLO_WORLD");
    }

    #[test]
    fn remap_camel_case_has_checksum() {
        assert_eq!(remap(b"log.body.HostName").as_str(), "ND3RAAO_LB_HOSTNAME");
    }

    // --- prefix shortening ---

    #[test]
    fn remap_resource_attributes_prefix() {
        assert_eq!(
            remap(b"resource.attributes.host.name").as_str(),
            "ND3AE_RA_HOST_NAME"
        );
    }

    #[test]
    fn remap_log_attributes_prefix() {
        assert_eq!(remap(b"log.attributes.logtag").as_str(), "NDAAE_LA_LOGTAG");
    }

    #[test]
    fn remap_log_body_prefix() {
        assert_eq!(remap(b"log.body.hostname").as_str(), "NDAAE_LB_HOSTNAME");
    }

    // --- MD5 fallback ---

    #[test]
    fn remap_invalid_chars_falls_back_to_md5() {
        let result = remap(b"field name");
        assert!(result.starts_with("ND_"));
        assert_eq!(result.len(), 35); // ND_ + 32 hex chars
    }

    #[test]
    fn remap_non_utf8_falls_back_to_md5() {
        let result = remap(b"\xFF\xFE invalid");
        assert!(result.starts_with("ND_"));
        assert_eq!(result.len(), 35);
    }

    #[test]
    fn remap_exceeding_64_bytes_falls_back_to_md5() {
        let result = remap(
            b"very.long.deeply.nested.field.name.that.would.definitely.exceed.the.systemd.limit",
        );
        assert!(result.starts_with("ND_"));
        assert!(result.len() <= 64);
    }

    // --- length guarantee ---

    #[test]
    fn remap_result_never_exceeds_64_bytes() {
        // 64 bytes of valid input — should be right at the limit
        let key = b"a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z.aa.bb.cc";
        let result = remap(key);
        assert!(result.len() <= 64, "len={}: {}", result.len(), result);
    }

    // --- production keys (from real OTel logs) ---

    #[test]
    fn remap_production_keys() {
        let cases: &[(&[u8], &str)] = &[
            (b"HTTPSConnection", "NDVWSO_HTTPSCONNECTION"),
            (b"OAuth2Token", "NDNRSNSO_OAUTH2TOKEN"),
            (b"foo.bar", "NDAE_FOO_BAR"),
            (b"fooBar", "ND9HJ_FOOBAR"),
            (b"foo_bar", "NDBE_FOO_BAR"),
            (b"log.attributes.log.file.path", "ND4AE_LA_LOG_FILE_PATH"),
            (b"log.attributes.log.iostream", "ND3AE_LA_LOG_IOSTREAM"),
            (b"log.attributes.logtag", "NDAAE_LA_LOGTAG"),
            (b"log.body.ClientAddr", "ND72AAO_LB_CLIENTADDR"),
            (b"log.body.ClientHost", "NDK5AAO_LB_CLIENTHOST"),
            (b"log.body.ClientPort", "NDLSAAO_LB_CLIENTPORT"),
            (b"log.body.ClientUsername", "ND56AAO_LB_CLIENTUSERNAME"),
            (
                b"log.body.DownstreamContentSize",
                "NDILAAO_LB_DOWNSTREAMCONTENTSIZE",
            ),
            (b"log.body.DownstreamStatus", "NDL5AAO_LB_DOWNSTREAMSTATUS"),
            (b"log.body.Duration", "NDSNAAO_LB_DURATION"),
            (b"log.body.GzipRatio", "NDPPAAO_LB_GZIPRATIO"),
            (b"log.body.HOSTNAME", "NDAAT_LB_HOSTNAME"),
            (
                b"log.body.OriginContentSize",
                "ND1UAAO_LB_ORIGINCONTENTSIZE",
            ),
            (b"log.body.OriginDuration", "NDGJAAO_LB_ORIGINDURATION"),
            (b"log.body.OriginStatus", "NDMOAAO_LB_ORIGINSTATUS"),
            (b"log.body.Overhead", "NDSMAAO_LB_OVERHEAD"),
            (b"log.body.RequestAddr", "NDYLAAO_LB_REQUESTADDR"),
            (
                b"log.body.RequestContentSize",
                "NDI9AAO_LB_REQUESTCONTENTSIZE",
            ),
            (b"log.body.RequestCount", "NDWNAAO_LB_REQUESTCOUNT"),
            (b"log.body.RequestHost", "NDFFAAO_LB_REQUESTHOST"),
            (b"log.body.RequestMethod", "NDQLAAO_LB_REQUESTMETHOD"),
            (b"log.body.RequestPath", "ND6AAAO_LB_REQUESTPATH"),
            (b"log.body.RequestPort", "NDM7AAO_LB_REQUESTPORT"),
            (b"log.body.RequestProtocol", "ND6EAAO_LB_REQUESTPROTOCOL"),
            (b"log.body.RequestScheme", "NDEUAAO_LB_REQUESTSCHEME"),
            (b"log.body.RetryAttempts", "ND00AAO_LB_RETRYATTEMPTS"),
            (b"log.body.RouterName", "ND63AAO_LB_ROUTERNAME"),
            (b"log.body.ServiceAddr", "ND7PAAO_LB_SERVICEADDR"),
            (b"log.body.ServiceName", "NDK5AAO_LB_SERVICENAME"),
            (b"log.body.ServiceURL", "NDWQAANT_LB_SERVICEURL"),
            (b"log.body.StartLocal", "NDA4AAO_LB_STARTLOCAL"),
            (b"log.body.StartUTC", "NDOLAANT_LB_STARTUTC"),
            (b"log.body.TR", "NDAAT_LB_TR"),
            (b"log.body.Ta", "NDYWAAO_LB_TA"),
            (b"log.body.Tc", "ND8GAAO_LB_TC"),
            (b"log.body.Th", "NDPCAAO_LB_TH"),
            (b"log.body.Ti", "NDAJAAO_LB_TI"),
            (b"log.body.Tr", "NDVQAAO_LB_TR"),
            (b"log.body.Tw", "NDDPAAO_LB_TW"),
            (b"log.body.account", "NDAAE_LB_ACCOUNT"),
            (b"log.body.actconn", "NDAAE_LB_ACTCONN"),
            (b"log.body.action", "NDAAE_LB_ACTION"),
            (b"log.body.agent", "NDAAE_LB_AGENT"),
            (
                b"log.body.agents_missing_from_emqx",
                "NDAA3BE_LB_AGENTS_MISSING_FROM_EMQX",
            ),
            (
                b"log.body.agents_unreachable_postgres_disconnected",
                "NDAA3BE_LB_AGENTS_UNREACHABLE_POSTGRES_DISCONNECTED",
            ),
            (b"log.body.apiVersion", "ND2UAAJ_LB_APIVERSION"),
            (b"log.body.arn", "NDAAE_LB_ARN"),
            (b"log.body.authority", "NDAAE_LB_AUTHORITY"),
            (b"log.body.backend_name", "NDAABE_LB_BACKEND_NAME"),
            (b"log.body.backend_queue", "NDAABE_LB_BACKEND_QUEUE"),
            (b"log.body.beconn", "NDAAE_LB_BECONN"),
            (b"log.body.bootstrap", "NDAAE_LB_BOOTSTRAP"),
            (b"log.body.bytes_read", "NDAABE_LB_BYTES_READ"),
            (b"log.body.caller", "NDAAE_LB_CALLER"),
            (b"log.body.cf_conn_ip", "NDAABBE_LB_CF_CONN_IP"),
            (b"log.body.client_ip", "NDAABE_LB_CLIENT_IP"),
            (b"log.body.client_port", "NDAABE_LB_CLIENT_PORT"),
            (
                b"log.body.config_file_source",
                "NDAABBE_LB_CONFIG_FILE_SOURCE",
            ),
            (b"log.body.count", "NDAAE_LB_COUNT"),
            (b"log.body.date_time", "NDAABE_LB_DATE_TIME"),
            (b"log.body.duration", "NDAAE_LB_DURATION"),
            (
                b"log.body.emqx_client_id_count",
                "NDAA3BE_LB_EMQX_CLIENT_ID_COUNT",
            ),
            (b"log.body.entryPointName", "NDQDAAJ_LB_ENTRYPOINTNAME"),
            (b"log.body.err", "NDAAE_LB_ERR"),
            (b"log.body.error", "NDAAE_LB_ERROR"),
            (b"log.body.eventTime", "NDBJAAJ_LB_EVENTTIME"),
            (b"log.body.failed", "NDAAE_LB_FAILED"),
            (b"log.body.feconn", "NDAAE_LB_FECONN"),
            (b"log.body.fields.component", "ND3AE_LB_FIELDS_COMPONENT"),
            (b"log.body.fields.data", "ND3AE_LB_FIELDS_DATA"),
            (b"log.body.fields.error", "ND3AE_LB_FIELDS_ERROR"),
            (b"log.body.fields.interval", "ND3AE_LB_FIELDS_INTERVAL"),
            (b"log.body.fields.metrics", "ND3AE_LB_FIELDS_METRICS"),
            (
                b"log.body.fields.otelcol.component.id",
                "ND5AE_LB_FIELDS_OTELCOL_COMPONENT_ID",
            ),
            (
                b"log.body.fields.otelcol.component.kind",
                "ND5AE_LB_FIELDS_OTELCOL_COMPONENT_KIND",
            ),
            (
                b"log.body.fields.otelcol.signal",
                "ND4AE_LB_FIELDS_OTELCOL_SIGNAL",
            ),
            (b"log.body.fields.path", "ND3AE_LB_FIELDS_PATH"),
            (b"log.body.fields.resource", "ND3AE_LB_FIELDS_RESOURCE"),
            (
                b"log.body.fields.resource.service.instance.id",
                "ND6AE_LB_FIELDS_RESOURCE_SERVICE_INSTANCE_ID",
            ),
            (
                b"log.body.fields.resource.service.name",
                "ND5AE_LB_FIELDS_RESOURCE_SERVICE_NAME",
            ),
            (
                b"log.body.fields.resource.service.version",
                "ND5AE_LB_FIELDS_RESOURCE_SERVICE_VERSION",
            ),
            (b"log.body.fields_raw", "NDAABE_LB_FIELDS_RAW"),
            (b"log.body.file", "NDAAE_LB_FILE"),
            (b"log.body.firstTimestamp", "ND3KAAJ_LB_FIRSTTIMESTAMP"),
            (b"log.body.forwarded-for", "NDAACE_LB_FORWARDED_FOR"),
            (b"log.body.frontend_name", "NDAABE_LB_FRONTEND_NAME"),
            (b"log.body.host", "NDAAE_LB_HOST"),
            (b"log.body.http_method", "NDAABE_LB_HTTP_METHOD"),
            (b"log.body.http_query", "NDAABE_LB_HTTP_QUERY"),
            (b"log.body.http_uri", "NDAABE_LB_HTTP_URI"),
            (b"log.body.http_version", "NDAABE_LB_HTTP_VERSION"),
            (b"log.body.id", "NDAAE_LB_ID"),
            (
                b"log.body.involvedObject.apiVersion",
                "NDJRAAFJ_LB_INVOLVEDOBJECT_APIVERSION",
            ),
            (
                b"log.body.involvedObject.fieldPath",
                "NDWCAAFJ_LB_INVOLVEDOBJECT_FIELDPATH",
            ),
            (
                b"log.body.involvedObject.kind",
                "NDBFAAFE_LB_INVOLVEDOBJECT_KIND",
            ),
            (
                b"log.body.involvedObject.name",
                "NDVXAAFE_LB_INVOLVEDOBJECT_NAME",
            ),
            (
                b"log.body.involvedObject.namespace",
                "ND9BAAFE_LB_INVOLVEDOBJECT_NAMESPACE",
            ),
            (
                b"log.body.involvedObject.resourceVersion",
                "NDCVAAFJ_LB_INVOLVEDOBJECT_RESOURCEVERSION",
            ),
            (
                b"log.body.involvedObject.uid",
                "NDPUAAFE_LB_INVOLVEDOBJECT_UID",
            ),
            (b"log.body.job_type", "NDAABE_LB_JOB_TYPE"),
            (b"log.body.kind", "NDAAE_LB_KIND"),
            (b"log.body.lastTimestamp", "NDO1AAJ_LB_LASTTIMESTAMP"),
            (b"log.body.level", "NDAAE_LB_LEVEL"),
            (b"log.body.line", "NDAAE_LB_LINE"),
            (b"log.body.local_addr.IP", "NDAABAT_LB_LOCAL_ADDR_IP"),
            (b"log.body.local_addr.Port", "NDNIAABAO_LB_LOCAL_ADDR_PORT"),
            (b"log.body.local_addr.Zone", "NDNGAABAO_LB_LOCAL_ADDR_ZONE"),
            (b"log.body.logger", "NDAAE_LB_LOGGER"),
            (b"log.body.message", "NDAAE_LB_MESSAGE"),
            (
                b"log.body.metadata.creationTimestamp",
                "NDN83AJ_LB_METADATA_CREATIONTIMESTAMP",
            ),
            (b"log.body.metadata.name", "ND3AE_LB_METADATA_NAME"),
            (
                b"log.body.metadata.namespace",
                "ND3AE_LB_METADATA_NAMESPACE",
            ),
            (
                b"log.body.metadata.resourceVersion",
                "NDZJ3AJ_LB_METADATA_RESOURCEVERSION",
            ),
            (b"log.body.metadata.uid", "ND3AE_LB_METADATA_UID"),
            (b"log.body.method", "NDAAE_LB_METHOD"),
            (b"log.body.msg", "NDAAE_LB_MSG"),
            (b"log.body.path", "NDAAE_LB_PATH"),
            (b"log.body.progress", "NDAAE_LB_PROGRESS"),
            (b"log.body.protocol", "NDAAE_LB_PROTOCOL"),
            (b"log.body.reason", "NDAAE_LB_REASON"),
            (b"log.body.record-id", "NDAACE_LB_RECORD_ID"),
            (b"log.body.record-type", "NDAACE_LB_RECORD_TYPE"),
            (b"log.body.referer", "NDAAE_LB_REFERER"),
            (b"log.body.referrer", "NDAAE_LB_REFERRER"),
            (b"log.body.region", "NDAAE_LB_REGION"),
            (
                b"log.body.remote_addr.ForceQuery",
                "NDYBAABAO_LB_REMOTE_ADDR_FORCEQUERY",
            ),
            (
                b"log.body.remote_addr.Fragment",
                "ND19AABAO_LB_REMOTE_ADDR_FRAGMENT",
            ),
            (
                b"log.body.remote_addr.Host",
                "ND7WAABAO_LB_REMOTE_ADDR_HOST",
            ),
            (
                b"log.body.remote_addr.OmitHost",
                "NDJ1AABAO_LB_REMOTE_ADDR_OMITHOST",
            ),
            (
                b"log.body.remote_addr.Opaque",
                "NDZMAABAO_LB_REMOTE_ADDR_OPAQUE",
            ),
            (
                b"log.body.remote_addr.Path",
                "NDL9AABAO_LB_REMOTE_ADDR_PATH",
            ),
            (
                b"log.body.remote_addr.RawFragment",
                "NDVKAABAO_LB_REMOTE_ADDR_RAWFRAGMENT",
            ),
            (
                b"log.body.remote_addr.RawPath",
                "NDSLAABAO_LB_REMOTE_ADDR_RAWPATH",
            ),
            (
                b"log.body.remote_addr.RawQuery",
                "NDI4AABAO_LB_REMOTE_ADDR_RAWQUERY",
            ),
            (
                b"log.body.remote_addr.Scheme",
                "ND6LAABAO_LB_REMOTE_ADDR_SCHEME",
            ),
            (
                b"log.body.remote_addr.User",
                "NDPJAABAO_LB_REMOTE_ADDR_USER",
            ),
            (
                b"log.body.reportingComponent",
                "ND16AAJ_LB_REPORTINGCOMPONENT",
            ),
            (
                b"log.body.reportingInstance",
                "ND59AAJ_LB_REPORTINGINSTANCE",
            ),
            (b"log.body.request-id", "NDAACE_LB_REQUEST_ID"),
            (b"log.body.request.host", "ND3AE_LB_REQUEST_HOST"),
            (b"log.body.request.method", "ND3AE_LB_REQUEST_METHOD"),
            (b"log.body.request.proto", "ND3AE_LB_REQUEST_PROTO"),
            (b"log.body.request.remote_ip", "ND3ABE_LB_REQUEST_REMOTE_IP"),
            (
                b"log.body.request.remote_port",
                "ND3ABE_LB_REQUEST_REMOTE_PORT",
            ),
            (b"log.body.request.uri", "ND3AE_LB_REQUEST_URI"),
            (
                b"log.body.request_User-Agent",
                "NDI9AABMO_LB_REQUEST_USER_AGENT",
            ),
            (b"log.body.response-code", "NDAACE_LB_RESPONSE_CODE"),
            (
                b"log.body.response-code-details",
                "NDAACCE_LB_RESPONSE_CODE_DETAILS",
            ),
            (b"log.body.retries", "NDAAE_LB_RETRIES"),
            (b"log.body.server_name", "NDAABE_LB_SERVER_NAME"),
            (b"log.body.service", "NDAAE_LB_SERVICE"),
            (
                b"log.body.serviceURL.ForceQuery",
                "NDCRAADPO_LB_SERVICEURL_FORCEQUERY",
            ),
            (
                b"log.body.serviceURL.Fragment",
                "ND7HAADPO_LB_SERVICEURL_FRAGMENT",
            ),
            (b"log.body.serviceURL.Host", "ND32AADPO_LB_SERVICEURL_HOST"),
            (
                b"log.body.serviceURL.OmitHost",
                "ND4RAADPO_LB_SERVICEURL_OMITHOST",
            ),
            (
                b"log.body.serviceURL.Opaque",
                "NDL3AADPO_LB_SERVICEURL_OPAQUE",
            ),
            (b"log.body.serviceURL.Path", "ND00AADPO_LB_SERVICEURL_PATH"),
            (
                b"log.body.serviceURL.RawFragment",
                "NDWDAADPO_LB_SERVICEURL_RAWFRAGMENT",
            ),
            (
                b"log.body.serviceURL.RawPath",
                "ND34AADPO_LB_SERVICEURL_RAWPATH",
            ),
            (
                b"log.body.serviceURL.RawQuery",
                "NDOGAADPO_LB_SERVICEURL_RAWQUERY",
            ),
            (
                b"log.body.serviceURL.Scheme",
                "NDR5AADPO_LB_SERVICEURL_SCHEME",
            ),
            (b"log.body.serviceURL.User", "NDRYAADPO_LB_SERVICEURL_USER"),
            (b"log.body.session_id", "NDAABE_LB_SESSION_ID"),
            (b"log.body.severity", "NDAAE_LB_SEVERITY"),
            (b"log.body.size", "NDAAE_LB_SIZE"),
            (b"log.body.source.component", "ND3AE_LB_SOURCE_COMPONENT"),
            (b"log.body.source.host", "ND3AE_LB_SOURCE_HOST"),
            (b"log.body.srv_queue", "NDAABE_LB_SRV_QUEUE"),
            (b"log.body.srvconn", "NDAAE_LB_SRVCONN"),
            (b"log.body.ssl_ciphers", "NDAABE_LB_SSL_CIPHERS"),
            (b"log.body.ssl_version", "NDAABE_LB_SSL_VERSION"),
            (b"log.body.status", "NDAAE_LB_STATUS"),
            (b"log.body.status_code", "NDAABE_LB_STATUS_CODE"),
            (b"log.body.successful", "NDAAE_LB_SUCCESSFUL"),
            (b"log.body.svc", "NDAAE_LB_SVC"),
            (b"log.body.termination_state", "NDAABE_LB_TERMINATION_STATE"),
            (b"log.body.time", "NDAAE_LB_TIME"),
            (b"log.body.timestamp", "NDAAE_LB_TIMESTAMP"),
            (b"log.body.topic", "NDAAE_LB_TOPIC"),
            (b"log.body.ts", "NDAAE_LB_TS"),
            (b"log.body.type", "NDAAE_LB_TYPE"),
            (b"log.body.unique_id", "NDAABE_LB_UNIQUE_ID"),
            (
                b"log.body.unreachable_agents_cleaned",
                "NDAABBE_LB_UNREACHABLE_AGENTS_CLEANED",
            ),
            (b"log.body.upstream-cluster", "NDAACE_LB_UPSTREAM_CLUSTER"),
            (b"log.body.user-agent", "NDAACE_LB_USER_AGENT"),
            (b"log.body.user_id", "NDAABE_LB_USER_ID"),
            (b"log.body.version", "NDAAE_LB_VERSION"),
            (
                b"log.dropped_attributes_count",
                "NDABBE_LOG_DROPPED_ATTRIBUTES_COUNT",
            ),
            (b"log.event_name", "NDABE_LOG_EVENT_NAME"),
            (b"log.flags", "NDAE_LOG_FLAGS"),
            (
                b"log.observed_time_unix_nano",
                "NDA3BE_LOG_OBSERVED_TIME_UNIX_NANO",
            ),
            (b"log.severity_number", "NDABE_LOG_SEVERITY_NUMBER"),
            (b"log.severity_text", "NDABE_LOG_SEVERITY_TEXT"),
            (b"log.time_unix_nano", "NDABBE_LOG_TIME_UNIX_NANO"),
            (
                b"my.deeply.nested.key.checks.run.length.encoding.works.well",
                "ND9AE_MY_DEEPLY_NESTED_KEY_CHECKS_RUN_LENGTH_ENCODING_WORKS_WELL",
            ),
            (b"resource.attributes.app", "NDAAE_RA_APP"),
            (b"resource.attributes.component", "NDAAE_RA_COMPONENT"),
            (b"resource.attributes.host.name", "ND3AE_RA_HOST_NAME"),
            (
                b"resource.attributes.k8s.container.name",
                "NDGKAAFAE_RA_K8S_CONTAINER_NAME",
            ),
            (
                b"resource.attributes.k8s.container.restart_count",
                "NDMIAAFABE_RA_K8S_CONTAINER_RESTART_COUNT",
            ),
            (
                b"resource.attributes.k8s.cronjob.name",
                "ND6OAAFAE_RA_K8S_CRONJOB_NAME",
            ),
            (
                b"resource.attributes.k8s.daemonset.name",
                "NDRJAAFAE_RA_K8S_DAEMONSET_NAME",
            ),
            (
                b"resource.attributes.k8s.deployment.name",
                "NDK6AAFAE_RA_K8S_DEPLOYMENT_NAME",
            ),
            (
                b"resource.attributes.k8s.job.name",
                "NDNSAAFAE_RA_K8S_JOB_NAME",
            ),
            (
                b"resource.attributes.k8s.namespace.name",
                "ND3LAAFAE_RA_K8S_NAMESPACE_NAME",
            ),
            (
                b"resource.attributes.k8s.node.name",
                "NDNXAAFAE_RA_K8S_NODE_NAME",
            ),
            (
                b"resource.attributes.k8s.pod.name",
                "NDZ7AAFAE_RA_K8S_POD_NAME",
            ),
            (
                b"resource.attributes.k8s.pod.start_time",
                "NDYNAAFABE_RA_K8S_POD_START_TIME",
            ),
            (
                b"resource.attributes.k8s.pod.uid",
                "NDOCAAFAE_RA_K8S_POD_UID",
            ),
            (b"resource.attributes.os.type", "ND3AE_RA_OS_TYPE"),
            (
                b"resource.attributes.service.instance.environment.region.zone",
                "ND6AE_RA_SERVICE_INSTANCE_ENVIRONMENT_REGION_ZONE",
            ),
            (b"resource.schema_url", "NDABE_RESOURCE_SCHEMA_URL"),
        ];

        for (key, expected) in cases {
            assert_eq!(
                remap(key).as_str(),
                *expected,
                "key: {:?}",
                std::str::from_utf8(key).unwrap()
            );
        }
    }
}
