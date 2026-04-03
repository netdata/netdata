use super::*;

pub(crate) fn decoder_state_bincode_options() -> impl Options {
    bincode::DefaultOptions::new()
        .with_fixint_encoding()
        .with_little_endian()
}

pub(crate) fn xxhash64(data: &[u8]) -> u64 {
    let mut hasher = XxHash64::default();
    hasher.write(data);
    hasher.finish()
}

pub(crate) fn encode_persisted_namespace_file(
    file: &PersistedDecoderNamespaceFile,
) -> Result<Vec<u8>, String> {
    let payload = decoder_state_bincode_options()
        .serialize(file)
        .map_err(|err| format!("failed to encode decoder namespace state: {err}"))?;
    let payload_hash = xxhash64(&payload);
    let payload_len = payload.len() as u64;

    let mut out = Vec::with_capacity(DECODER_STATE_HEADER_LEN + payload.len());
    out.extend_from_slice(DECODER_STATE_MAGIC);
    out.extend_from_slice(&DECODER_STATE_SCHEMA_VERSION.to_le_bytes());
    out.extend_from_slice(&payload_hash.to_le_bytes());
    out.extend_from_slice(&payload_len.to_le_bytes());
    out.extend_from_slice(&payload);
    Ok(out)
}

pub(crate) fn decode_persisted_namespace_file(
    data: &[u8],
) -> Result<PersistedDecoderNamespaceFile, String> {
    if data.len() < DECODER_STATE_HEADER_LEN {
        return Err("truncated decoder namespace state header".to_string());
    }
    if &data[..4] != DECODER_STATE_MAGIC {
        return Err("invalid decoder namespace state magic".to_string());
    }

    let version = u32::from_le_bytes(data[4..8].try_into().unwrap());
    if version != DECODER_STATE_SCHEMA_VERSION {
        return Err(format!(
            "unsupported decoder namespace schema version {} (expected {})",
            version, DECODER_STATE_SCHEMA_VERSION
        ));
    }

    let expected_hash = u64::from_le_bytes(data[8..16].try_into().unwrap());
    let payload_len = u64::from_le_bytes(data[16..24].try_into().unwrap());
    let payload_len = usize::try_from(payload_len)
        .map_err(|_| "decoder namespace payload length overflows usize".to_string())?;
    let payload = &data[DECODER_STATE_HEADER_LEN..];
    if payload.len() != payload_len {
        return Err(format!(
            "decoder namespace payload length mismatch (header {}, actual {})",
            payload_len,
            payload.len()
        ));
    }

    let actual_hash = xxhash64(payload);
    if actual_hash != expected_hash {
        return Err(format!(
            "decoder namespace payload hash mismatch (expected {}, got {})",
            expected_hash, actual_hash
        ));
    }

    decoder_state_bincode_options()
        .deserialize(payload)
        .map_err(|err| format!("failed to decode decoder namespace state: {err}"))
}
