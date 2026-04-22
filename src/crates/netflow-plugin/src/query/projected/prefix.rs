use super::*;

#[inline(always)]
pub(crate) fn projected_prefix_value(bytes: &[u8]) -> u64 {
    if bytes.len() >= 8 {
        return u64::from_le_bytes(bytes[..8].try_into().unwrap());
    }

    let mut padded = [0_u8; 8];
    padded[..bytes.len()].copy_from_slice(bytes);
    u64::from_le_bytes(padded)
}

#[inline(always)]
pub(crate) fn projected_prefix_mask(bytes: &[u8]) -> u64 {
    let prefix_len = bytes.len().min(8);
    match prefix_len {
        0 => 0,
        8 => u64::MAX,
        _ => (1_u64 << (prefix_len * 8)) - 1,
    }
}

#[inline(always)]
pub(crate) fn projected_match_value<'a>(
    payload: &'a [u8],
    payload_prefix: u64,
    spec: &ProjectedFieldSpec,
) -> Option<&'a [u8]> {
    if payload.len() < spec.key.len() + 1 {
        return None;
    }
    if (payload_prefix & spec.mask) != spec.prefix {
        return None;
    }
    if payload[spec.key.len()] != b'=' || !payload.starts_with(spec.key.as_slice()) {
        return None;
    }

    Some(&payload[spec.key.len() + 1..])
}

#[inline(always)]
pub(crate) fn projected_match_value_prefix_only<'a>(
    payload: &'a [u8],
    payload_prefix: u64,
    spec: &ProjectedFieldSpec,
) -> Option<&'a [u8]> {
    if payload.len() < spec.key.len() + 1 {
        return None;
    }
    if (payload_prefix & spec.mask) != spec.prefix {
        return None;
    }
    if payload[spec.key.len()] != b'=' {
        return None;
    }

    Some(&payload[spec.key.len() + 1..])
}

pub(crate) fn projected_field_spec_index(specs: &mut Vec<ProjectedFieldSpec>, key: &[u8]) -> usize {
    if let Some(index) = specs.iter().position(|spec| spec.key.as_slice() == key) {
        return index;
    }

    let index = specs.len();
    specs.push(ProjectedFieldSpec {
        prefix: projected_prefix_value(key),
        mask: projected_prefix_mask(key),
        key: key.to_vec(),
        targets: ProjectedFieldTargets::default(),
    });
    index
}
