//! Bridge from `opentelemetry_proto` types to pdata Arrow RecordBatches.
//!
//! Since both crates generate prost types from the same proto definitions,
//! we bridge via proto bytes: encode ours → decode as pdata's.
//!
//! # Pre-computed attribute hashes (`_nd_kv_hash`)
//!
//! The downstream indexer (`sfst_indexer`) builds an inverted bitmap index
//! keyed by `"key=value"` strings. Without optimization, it must format
//! every attribute as `"key=value"`, compute `xxhash64` over it, and look
//! the result up in a string interner — for every attribute on every log
//! record. Since most `key=value` pairs repeat across records (e.g.,
//! `service.name=myapp`), the formatting and hashing work is redundant.
//!
//! To avoid this, the **producer** (this module) pre-computes
//! `xxhash64("key=value")` for every attribute while the typed proto
//! values are still in hand, and stores the hashes as a synthetic
//! `_nd_kv_hash` attribute. The **consumer** (`sfst_indexer`) then uses
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
//! This contract is implemented by `hash_value_display` below (producer)
//! and the attribute-column value formatting inside `wal-otap` (consumer).
//! If either side changes formatting, the hashes will silently mismatch
//! and the consumer will fall back to the slow path (correct but slower).

use std::hash::Hasher;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value::Value};
use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use otap_df_pdata::encode::encode_logs_otap_batch;
use otap_df_pdata::proto::opentelemetry::arrow::v1::ArrowPayloadType;
use otap_df_pdata::proto::opentelemetry::logs::v1::LogsData;
use prost::Message;
use twox_hash::XxHash64;

/// Encode logs from a vector of `ResourceLogs` into Arrow IPC bytes via pdata.
///
/// Returns `(ipc_bytes, log_record_count)`.
pub fn encode(mut resource_logs: Vec<ResourceLogs>) -> Result<(Vec<u8>, usize), String> {
    // Step 0: Normalize, flatten, and hash all attributes.
    let mut log_count = 0usize;
    for rl in &mut resource_logs {
        for sl in &mut rl.scope_logs {
            for lr in &mut sl.log_records {
                log_count += 1;
                otel_normalize::normalize_body(lr);
                prepare_log_attributes(lr);
            }
        }
    }

    // Steps 1-2: Proto bytes round-trip to bridge type systems.
    //
    // `opentelemetry-proto` (our gRPC service) and `otap-df-pdata` (Arrow
    // encoding) both generate prost types from the same .proto definitions,
    // but they are distinct Rust types. The only way to cross from one to
    // the other is to serialize and deserialize via proto bytes.
    //
    // This could be eliminated if `otap-df-pdata` either re-exported the
    // `opentelemetry-proto` types instead of generating its own, or
    // implemented `LogsDataView` (from `otap-df-pdata-views`) for them —
    // `encode_logs_otap_batch` is already generic over that trait. The
    // upstream project is aware of the duplication:
    //   - https://github.com/open-telemetry/otel-arrow/issues/1848
    //   - https://github.com/open-telemetry/otel-arrow/issues/1340
    let req = ExportLogsServiceRequest { resource_logs };
    let mut proto_bytes = Vec::with_capacity(req.encoded_len());
    req.encode(&mut proto_bytes)
        .map_err(|e| format!("proto encode: {e}"))?;

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
        hash_value_display(&mut h, kv.value.as_ref());
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
fn hash_value_display(h: &mut XxHash64, av: Option<&AnyValue>) {
    use std::io::Write as _;
    match av.and_then(|v| v.value.as_ref()) {
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

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{ArrayValue, KeyValueList};
    use opentelemetry_proto::tonic::logs::v1::LogRecord;

    fn s(v: &str) -> AnyValue {
        AnyValue {
            value: Some(Value::StringValue(v.into())),
        }
    }

    fn int(v: i64) -> AnyValue {
        AnyValue {
            value: Some(Value::IntValue(v)),
        }
    }

    fn arr(items: Vec<AnyValue>) -> AnyValue {
        AnyValue {
            value: Some(Value::ArrayValue(ArrayValue { values: items })),
        }
    }

    fn obj(pairs: Vec<(&str, AnyValue)>) -> AnyValue {
        AnyValue {
            value: Some(Value::KvlistValue(KeyValueList {
                values: pairs
                    .into_iter()
                    .map(|(k, v)| KeyValue {
                        key: k.into(),
                        value: Some(v),
                    })
                    .collect(),
            })),
        }
    }

    fn record(attrs: Vec<(&str, AnyValue)>) -> LogRecord {
        LogRecord {
            attributes: attrs
                .into_iter()
                .map(|(k, v)| KeyValue {
                    key: k.into(),
                    value: Some(v),
                })
                .collect(),
            ..Default::default()
        }
    }

    /// The flattened `(key, value)` pairs, excluding the synthetic hash
    /// attribute. Scalars are rendered with the same conventions as the
    /// hash contract.
    fn flat_pairs(lr: &LogRecord) -> Vec<(String, String)> {
        lr.attributes
            .iter()
            .filter(|kv| kv.key != "_nd_kv_hash")
            .map(|kv| {
                let v = match kv.value.as_ref().and_then(|a| a.value.as_ref()) {
                    Some(Value::StringValue(v)) => v.clone(),
                    Some(Value::IntValue(v)) => v.to_string(),
                    Some(Value::DoubleValue(v)) => v.to_string(),
                    Some(Value::BoolValue(v)) => v.to_string(),
                    other => format!("<{other:?}>"),
                };
                (kv.key.clone(), v)
            })
            .collect()
    }

    fn pair(k: &str, v: &str) -> (String, String) {
        (k.into(), v.into())
    }

    #[test]
    fn scalar_array_collapses_to_repeated_bare_key() {
        let mut lr = record(vec![("tags", arr(vec![s("a"), s("b")]))]);
        prepare_log_attributes(&mut lr);
        assert_eq!(flat_pairs(&lr), vec![pair("tags", "a"), pair("tags", "b")]);
    }

    #[test]
    fn array_of_objects_keeps_positions() {
        let mut lr = record(vec![(
            "endpoints",
            arr(vec![
                obj(vec![("host", s("a")), ("port", int(1))]),
                obj(vec![("host", s("b"))]),
            ]),
        )]);
        prepare_log_attributes(&mut lr);
        assert_eq!(
            flat_pairs(&lr),
            vec![
                pair("endpoints.0.host", "a"),
                pair("endpoints.0.port", "1"),
                pair("endpoints.1.host", "b"),
            ]
        );
    }

    #[test]
    fn mixed_array_numbers_sparsely() {
        let mut lr = record(vec![("key", arr(vec![obj(vec![("x", int(1))]), s("a")]))]);
        prepare_log_attributes(&mut lr);
        assert_eq!(
            flat_pairs(&lr),
            vec![pair("key.0.x", "1"), pair("key", "a")]
        );
    }

    #[test]
    fn nested_scalar_arrays_collapse_inner_positions() {
        let mut lr = record(vec![(
            "key",
            arr(vec![arr(vec![s("a"), s("b")]), arr(vec![s("c")])]),
        )]);
        prepare_log_attributes(&mut lr);
        assert_eq!(
            flat_pairs(&lr),
            vec![pair("key.0", "a"), pair("key.0", "b"), pair("key.1", "c")]
        );
    }

    #[test]
    fn duplicate_scalar_siblings_dedupe() {
        let mut lr = record(vec![("tags", arr(vec![s("a"), s("a"), s("b")]))]);
        prepare_log_attributes(&mut lr);
        assert_eq!(flat_pairs(&lr), vec![pair("tags", "a"), pair("tags", "b")]);

        // Dedup skips only verified-equal elements. Int(1) and String("1")
        // hash identically (same formatted value) but are different
        // elements, so both are emitted — never drop on a hash hit alone.
        // The redundancy is harmless: both become the pair `n=1` downstream
        // and collapse to one interned id.
        let mut lr = record(vec![("n", arr(vec![int(1), s("1")]))]);
        prepare_log_attributes(&mut lr);
        assert_eq!(flat_pairs(&lr), vec![pair("n", "1"), pair("n", "1")]);
    }

    #[test]
    fn empty_array_flattens_to_nothing() {
        let mut lr = record(vec![("empty", arr(vec![]))]);
        prepare_log_attributes(&mut lr);
        assert!(flat_pairs(&lr).is_empty());
        // No attributes → no hash attribute either.
        assert!(lr.attributes.is_empty());
    }

    #[test]
    fn body_scalar_array_collapses_under_log_body() {
        let mut lr = record(vec![]);
        lr.body = Some(obj(vec![("domains", arr(vec![s("a.com"), s("b.com")]))]));
        prepare_log_attributes(&mut lr);
        assert_eq!(
            flat_pairs(&lr),
            vec![
                pair("log.body.domains", "a.com"),
                pair("log.body.domains", "b.com"),
            ]
        );
    }

    /// One hash per flattened attribute, repeated keys included — the
    /// consumer indexes the hash blob positionally.
    #[test]
    fn hash_attribute_counts_repeated_keys() {
        let mut lr = record(vec![("tags", arr(vec![s("a"), s("b")])), ("plain", s("x"))]);
        prepare_log_attributes(&mut lr);
        let hash = lr.attributes.last().unwrap();
        assert_eq!(hash.key, "_nd_kv_hash");
        let Some(Value::BytesValue(bytes)) = hash.value.as_ref().and_then(|a| a.value.as_ref())
        else {
            panic!("hash attribute is not BytesValue");
        };
        // 3 flattened pairs (tags=a, tags=b, plain=x) → 3 × 8 hash bytes.
        assert_eq!(bytes.len(), 3 * 8);
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
            // Per-element rule:
            //
            // - A **scalar** element is emitted under the **bare** array key
            //   (no positional suffix), so the array becomes one
            //   multi-valued field: `all_domains: ["a","b"]` →
            //   `all_domains=a`, `all_domains=b`. Element order carries no
            //   indexable meaning, and positional keys exploded the field
            //   table (one field per position, scattered across cardinality
            //   tiers) while making "any element equals X" filters
            //   impossible to express.
            // - A **structured** element (kvlist / nested array) keeps its
            //   positional segment: the index is what associates the
            //   sub-fields of one element (`endpoints.0.host` /
            //   `endpoints.0.port`) and allows per-element conjunction.
            //
            // Mixed arrays therefore number sparsely: `[{x:1}, "a"]` →
            // `key.0.x=1` + `key=a`.
            //
            // Identical scalar siblings are deduplicated — equal elements
            // produce the identical `key=value` pair downstream, so
            // duplicates only bloat the stream row. The formatted value's
            // xxhash64 is the fast path; equality of the elements is the
            // truth (the same hash-then-verify discipline as the indexer's
            // interner). On a hash hit with a *different* element — a
            // 64-bit collision, or a differently-typed pair that formats
            // identically (`Int(1)` vs `String("1")`) — the element is
            // emitted rather than dropped: a redundant pair is harmless
            // (it collapses to the same interned id downstream), whereas
            // a dropped value would be silent data loss.
            let base = key_buf.len();
            let mut seen: std::collections::HashMap<u64, &AnyValue> =
                std::collections::HashMap::new();
            for (i, item) in arr.values.iter().enumerate() {
                match &item.value {
                    Some(Value::KvlistValue(_)) | Some(Value::ArrayValue(_)) => {
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
                    Some(_) => {
                        let mut h = XxHash64::default();
                        hash_value_display(&mut h, Some(item));
                        // The first occurrence stays the hash's
                        // representative; only a verified-equal element is
                        // skipped.
                        let dup = match seen.entry(h.finish()) {
                            std::collections::hash_map::Entry::Occupied(e) => *e.get() == item,
                            std::collections::hash_map::Entry::Vacant(e) => {
                                e.insert(item);
                                false
                            }
                        };
                        if !dup {
                            key_buf.truncate(base);
                            out.push(KeyValue {
                                key: key_buf.clone(),
                                value: Some(item.clone()),
                            });
                        }
                    }
                    // A valueless element flattens to nothing (same as the
                    // scalar arm below for `None`).
                    None => {}
                }
            }
            key_buf.truncate(base);
        }
        None => {}
    }
}
