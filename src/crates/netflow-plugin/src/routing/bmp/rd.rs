use super::*;

pub(super) fn parse_configured_rds(values: &[RouteDistinguisherConfig]) -> Result<HashSet<u64>> {
    let mut accepted_rds = HashSet::new();
    for value in values {
        let parsed = match value {
            RouteDistinguisherConfig::Numeric(raw) => *raw,
            RouteDistinguisherConfig::Text(raw) => parse_rd_text(raw)?,
        };
        accepted_rds.insert(parsed);
    }
    Ok(accepted_rds)
}

pub(super) fn parse_rd_text(input: &str) -> Result<u64> {
    let parts: Vec<&str> = input.split(':').collect();
    match parts.len() {
        1 => Ok(parts[0]
            .parse::<u64>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as u64"))?),
        2 => parse_rd_parts(None, parts[0], parts[1], input),
        3 => {
            let rd_type = parts[0]
                .parse::<u8>()
                .with_context(|| format!("cannot parse route distinguisher type in '{input}'"))?;
            if rd_type > 2 {
                anyhow::bail!("route distinguisher type in '{input}' must be 0, 1, or 2");
            }
            parse_rd_parts(Some(rd_type), parts[1], parts[2], input)
        }
        _ => anyhow::bail!("cannot parse route distinguisher '{input}'"),
    }
}

fn parse_rd_parts(rd_type: Option<u8>, left: &str, right: &str, input: &str) -> Result<u64> {
    if rd_type == Some(1) || (rd_type.is_none() && left.contains('.')) {
        let ip = left
            .parse::<Ipv4Addr>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as IPv4:index"))?;
        let index = right
            .parse::<u16>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as IPv4:index"))?;
        let ip_u32 = u32::from(ip) as u64;
        return Ok(((1_u64) << 48) | (ip_u32 << 16) | u64::from(index));
    }

    let asn = left
        .parse::<u32>()
        .with_context(|| format!("cannot parse route distinguisher '{input}' as ASN:index"))?;
    let index_u32 = right
        .parse::<u32>()
        .with_context(|| format!("cannot parse route distinguisher '{input}' as ASN:index"))?;

    if rd_type == Some(0) && asn > u32::from(u16::MAX) {
        anyhow::bail!("cannot parse route distinguisher '{input}' as type-0 ASN2:index");
    }

    if asn <= u32::from(u16::MAX) && rd_type != Some(2) {
        return Ok(((0_u64) << 48) | ((asn as u64) << 32) | u64::from(index_u32));
    }

    let index_u16 = u16::try_from(index_u32).with_context(|| {
        format!("cannot parse route distinguisher '{input}' as type-2 ASN4:index")
    })?;
    Ok(((2_u64) << 48) | ((asn as u64) << 16) | u64::from(index_u16))
}
