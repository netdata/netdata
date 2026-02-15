//! Bridge from `opentelemetry_proto` types to pdata Arrow RecordBatches.
//!
//! Since both crates generate prost types from the same proto definitions,
//! we bridge via proto bytes: encode ours → decode as pdata's.
//!
//! # Pre-computed attribute hashes (`_nd_kv_hash`)
//!
//! The downstream indexer (`wal-explore`) builds an inverted bitmap index
//! keyed by `"key=value"` strings. Without optimization, it must format
//! every attribute as `"key=value"`, compute `xxhash64` over it, and look
//! the result up in a string interner — for every attribute on every log
//! record. Since most `key=value` pairs repeat across records (e.g.,
//! `service.name=myapp`), the formatting and hashing work is redundant.
//!
//! To avoid this, the **producer** (this module) pre-computes
//! `xxhash64("key=value")` for every attribute while the typed proto
//! values are still in hand, and stores the hashes as a synthetic
//! `_nd_kv_hash` attribute. The **consumer** (`wal-explore`) then uses
//! these hashes for hash-only lookups in its string interner — on a cache
//! hit (the hash is already interned), it skips string formatting entirely.
//!
//! ## Wire format
//!
//! `_nd_kv_hash` is a `BytesValue` containing N concatenated little-endian
//! `u64` hashes, one per preceding attribute, in the same order. It is
//! always the **last** attribute on each log record. OTAP preserves
//! attribute order through encode/decode, so position `i` in the hash
//! bytes corresponds to attribute row `i` in the consumer's Arrow batch.
//!
//! ## Hash contract (must match on both sides)
//!
//! The hash is `xxhash64(key_bytes + b"=" + formatted_value_bytes)` with
//! seed 0. Value formatting:
//!
//! | Type     | Format                              | Example              |
//! |----------|-------------------------------------|----------------------|
//! | String   | raw UTF-8 bytes, no quotes          | `hello`              |
//! | Int      | decimal via `Display`                | `123`                |
//! | Double   | decimal via `Display`                | `1.5`                |
//! | Bool     | `true` / `false`                     | `true`               |
//! | Bytes    | lowercase hex, no prefix             | `deadbeef`           |
//! | None     | empty (zero bytes after `=`)         |                      |
//!
//! This contract is implemented by [`hash_value_display`] (producer) and
//! [`wal_explore::arrow_columns::AttrsColumns::append_value`] (consumer).
//! If either side changes formatting, the hashes will silently mismatch
//! and the consumer will fall back to the slow path (correct but slower).

use std::hash::Hasher;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value};
use otap_df_pdata::encode::encode_logs_otap_batch;
use otap_df_pdata::proto::opentelemetry::arrow::v1::ArrowPayloadType;
use otap_df_pdata::proto::opentelemetry::logs::v1::LogsData;
use prost::Message;
use twox_hash::XxHash64;

/// Encode an `ExportLogsServiceRequest` (from opentelemetry-proto) into
/// Arrow IPC bytes via pdata.
///
/// Returns `(ipc_bytes, log_record_count)`.
pub fn encode_logs_arrow(req: &mut ExportLogsServiceRequest) -> Result<(Vec<u8>, usize), String> {
    // Step 0: Normalize, flatten, and hash all attributes.
    let mut log_count = 0usize;
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for lr in &mut sl.log_records {
                log_count += 1;
                otel_flatten::normalize::normalize_body(lr);
                prepare_log_attributes(lr);
            }
        }
    }

    // Step 1: Serialize our proto types to bytes.
    let mut proto_bytes = Vec::with_capacity(req.encoded_len());
    req.encode(&mut proto_bytes)
        .map_err(|e| format!("proto encode: {e}"))?;

    // Step 2: Deserialize as pdata's LogsData.
    // ExportLogsServiceRequest has a single field `resource_logs` at tag 1,
    // which is the same as LogsData. So we can decode directly.
    let logs_data =
        LogsData::decode(proto_bytes.as_slice()).map_err(|e| format!("proto decode: {e}"))?;

    // Step 3: Encode to Arrow RecordBatches via pdata.
    let otap_batch =
        encode_logs_otap_batch(&logs_data).map_err(|e| format!("arrow encode: {e:?}"))?;

    // Step 4: Serialize all RecordBatches into a single Arrow IPC stream.
    let mut ipc_buf = Vec::new();
    for pt in [
        ArrowPayloadType::Logs,
        ArrowPayloadType::LogAttrs,
        ArrowPayloadType::ResourceAttrs,
        ArrowPayloadType::ScopeAttrs,
    ] {
        if let Some(rb) = otap_batch.get(pt) {
            // Write a tag byte so the reader knows which payload type this is.
            ipc_buf.push(pt as u8);
            let len_pos = ipc_buf.len();
            ipc_buf.extend_from_slice(&[0u8; 4]); // placeholder for length

            let start = ipc_buf.len();
            let mut writer = arrow::ipc::writer::StreamWriter::try_new(&mut ipc_buf, &rb.schema())
                .map_err(|e| format!("ipc writer: {e}"))?;
            writer.write(rb).map_err(|e| format!("ipc write: {e}"))?;
            writer.finish().map_err(|e| format!("ipc finish: {e}"))?;
            drop(writer);

            let ipc_len = (ipc_buf.len() - start) as u32;
            ipc_buf[len_pos..len_pos + 4].copy_from_slice(&ipc_len.to_le_bytes());
        }
    }

    Ok((ipc_buf, log_count))
}

/// Flatten all nested attributes and the body, then append `_nd_kv_hash`
/// covering every attribute.
///
/// 1. Flatten any nested pre-existing attributes (KvlistValue, ArrayValue)
///    into dotted-key scalar attributes in-place.
/// 2. Flatten the body into `log.body.*` attributes.
/// 3. Hash ALL attributes and append `_nd_kv_hash`.
fn prepare_log_attributes(lr: &mut opentelemetry_proto::tonic::logs::v1::LogRecord) {
    // Step 1: Flatten any nested pre-existing attributes.
    let mut key_buf = String::with_capacity(128);
    let orig_attrs = std::mem::take(&mut lr.attributes);
    for kv in orig_attrs {
        if let Some(ref av) = kv.value {
            match &av.value {
                Some(Value::KvlistValue(_)) | Some(Value::ArrayValue(_)) => {
                    key_buf.clear();
                    key_buf.push_str(&kv.key);
                    flatten_any_value(av, &mut key_buf, &mut lr.attributes);
                }
                _ => lr.attributes.push(kv),
            }
        } else {
            lr.attributes.push(kv);
        }
    }

    // Step 2: Flatten the body into attributes.
    if let Some(body) = lr.body.take() {
        key_buf.clear();
        key_buf.push_str("log.body");
        flatten_any_value(&body, &mut key_buf, &mut lr.attributes);
    }

    // Step 3: Hash ALL attributes.
    append_hash_attributes(&mut lr.attributes, 0);
}

/// Compute XxHash64 (seed 0) of `key=value` for each attribute in
/// `attrs[start..]`, then append a single synthetic `_nd_kv_hash` attribute
/// as a `BytesValue` containing the concatenated little-endian u64 hashes.
///
/// Value formatting uses Rust's `Display` trait (no heap allocation):
/// - StringValue  → raw string bytes
/// - IntValue     → decimal (e.g. `123`)
/// - DoubleValue  → decimal (e.g. `1.5`)
/// - BoolValue    → `true` / `false`
/// - BytesValue   → hex-encoded bytes
/// - None         → empty
fn append_hash_attributes(attrs: &mut Vec<KeyValue>, flattened_start: usize) {
    let flattened = &attrs[flattened_start..];
    if flattened.is_empty() {
        return;
    }

    let mut hash_bytes = Vec::with_capacity(flattened.len() * 8);

    for kv in flattened {
        let mut h = XxHash64::default();
        h.write(kv.key.as_bytes());
        h.write(b"=");
        hash_value_display(&mut h, &kv.value);
        hash_bytes.extend_from_slice(&h.finish().to_le_bytes());
    }

    attrs.push(KeyValue {
        key: "_nd_kv_hash".to_owned(),
        value: Some(AnyValue {
            value: Some(Value::BytesValue(hash_bytes)),
        }),
    });
}

/// Write the Display-formatted representation of an `AnyValue` directly into
/// a hasher, without heap allocation.
fn hash_value_display(h: &mut XxHash64, av: &Option<AnyValue>) {
    use std::io::Write as _;
    match av.as_ref().and_then(|v| v.value.as_ref()) {
        Some(Value::StringValue(s)) => {
            h.write(s.as_bytes());
        }
        Some(Value::BoolValue(b)) => {
            h.write(if *b { b"true" } else { b"false" });
        }
        Some(Value::IntValue(i)) => {
            let mut buf = [0u8; 20];
            let mut cursor = std::io::Cursor::new(&mut buf[..]);
            write!(cursor, "{i}").unwrap();
            let n = cursor.position() as usize;
            h.write(&buf[..n]);
        }
        Some(Value::DoubleValue(f)) => {
            let mut buf = [0u8; 32];
            let mut cursor = std::io::Cursor::new(&mut buf[..]);
            write!(cursor, "{f}").unwrap();
            let n = cursor.position() as usize;
            h.write(&buf[..n]);
        }
        Some(Value::BytesValue(b)) => {
            // Hex-encode each byte directly into the hasher.
            const HEX: &[u8; 16] = b"0123456789abcdef";
            for &byte in b.iter() {
                h.write(&[HEX[(byte >> 4) as usize], HEX[(byte & 0xf) as usize]]);
            }
        }
        // ArrayValue / KvlistValue should not appear after flattening.
        Some(Value::ArrayValue(_)) | Some(Value::KvlistValue(_)) | None => {}
    }
}

fn flatten_any_value(av: &AnyValue, key_buf: &mut String, out: &mut Vec<KeyValue>) {
    match &av.value {
        Some(Value::StringValue(_))
        | Some(Value::IntValue(_))
        | Some(Value::DoubleValue(_))
        | Some(Value::BoolValue(_))
        | Some(Value::BytesValue(_)) => {
            out.push(KeyValue {
                key: key_buf.clone(),
                value: Some(av.clone()),
            });
        }
        Some(Value::KvlistValue(kvl)) => {
            let base = key_buf.len();
            for kv in &kvl.values {
                key_buf.truncate(base);
                key_buf.push('.');
                key_buf.push_str(&kv.key);
                if let Some(value) = &kv.value {
                    flatten_any_value(value, key_buf, out);
                } else {
                    out.push(KeyValue {
                        key: key_buf.clone(),
                        value: None,
                    });
                }
            }
            key_buf.truncate(base);
        }
        Some(Value::ArrayValue(arr)) => {
            // Flatten array elements with index suffixes.
            let base = key_buf.len();
            for (i, item) in arr.values.iter().enumerate() {
                key_buf.truncate(base);
                key_buf.push('.');
                // Inline integer formatting without allocating.
                let mut itoa_buf = [0u8; 20];
                let n = {
                    use std::io::Write;
                    let mut cursor = std::io::Cursor::new(&mut itoa_buf[..]);
                    write!(cursor, "{i}").unwrap();
                    cursor.position() as usize
                };
                key_buf.push_str(std::str::from_utf8(&itoa_buf[..n]).unwrap());
                flatten_any_value(item, key_buf, out);
            }
            key_buf.truncate(base);
        }
        None => {}
    }
}
