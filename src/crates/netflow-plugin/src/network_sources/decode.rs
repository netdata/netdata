use super::transform::run_transform;
use super::types::{AsnValue, CompiledTransform, RemoteRecord};
use super::*;

pub(super) fn decode_remote_records(
    payload: Value,
    transform: &CompiledTransform,
) -> Result<Vec<NetworkSourceRecord>> {
    let items = run_transform(payload, transform)?;

    let mut out = Vec::with_capacity(items.len());
    for item in items {
        let remote: RemoteRecord =
            serde_json::from_value(item).context("cannot map remote network source row")?;
        let prefix = parse_remote_prefix(&remote.prefix)?;
        let attrs = NetworkAttributes {
            name: remote.name,
            role: remote.role,
            site: remote.site,
            region: remote.region,
            country: remote.country,
            state: remote.state,
            city: remote.city,
            latitude: String::new(),
            longitude: String::new(),
            tenant: remote.tenant,
            asn: decode_remote_asn(remote.asn),
            asn_name: remote.asn_name.trim().to_string(),
            ip_class: String::new(),
        };
        out.push(NetworkSourceRecord { prefix, attrs });
    }

    if out.is_empty() {
        anyhow::bail!("empty result");
    }

    Ok(out)
}

fn parse_remote_prefix(value: &str) -> Result<IpNet> {
    IpNet::from_str(value.trim())
        .with_context(|| format!("invalid remote network prefix '{}'", value))
}

fn decode_remote_asn(value: AsnValue) -> u32 {
    match value {
        AsnValue::Empty => 0,
        AsnValue::Number(v) => v,
        AsnValue::Text(text) => text
            .strip_prefix("AS")
            .or_else(|| text.strip_prefix("as"))
            .unwrap_or(&text)
            .trim()
            .parse::<u32>()
            .unwrap_or(0),
    }
}
