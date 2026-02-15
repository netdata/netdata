mod flat;
pub mod normalize;
pub mod visit;

pub use flat::{Entry, Frame, Value};

use std::collections::HashMap;

use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use visit::FlatVisitor;

/// Flatten a `ResourceLogs` into a `Frame` of pre-flattened entries.
///
/// `ingestion_time_ns` is the last-resort fallback timestamp (nanoseconds since Unix epoch).
/// Each entry's `timestamp_ns` is resolved via: time_unix_nano → observed_time_unix_nano → ingestion_time_ns.
///
/// This runs two passes:
/// 1. Normalize: detect JSON strings in log bodies and convert them to KvlistValues.
/// 2. Flatten: walk the protobuf structure and yield `(key, value)` pairs at each leaf.
pub fn flatten_resource_logs(rl: &mut ResourceLogs, ingestion_time_ns: u64) -> Frame {
    // Pass 1: normalize JSON body strings.
    for sl in &mut rl.scope_logs {
        for lr in &mut sl.log_records {
            normalize::normalize_body(lr);
        }
    }

    // Pass 2: flatten into owned entries with interned keys.
    let mut visitor = FlatVisitor::new();
    let mut entries = Vec::new();
    let mut key_map: HashMap<String, u32> = HashMap::new();
    let mut key_list: Vec<String> = Vec::new();

    let intern_key = |key: &str, map: &mut HashMap<String, u32>, list: &mut Vec<String>| -> u32 {
        if let Some(&idx) = map.get(key) {
            idx
        } else {
            let idx = list.len() as u32;
            map.insert(key.to_owned(), idx);
            list.push(key.to_owned());
            idx
        }
    };

    for sl in &rl.scope_logs {
        for log_record in &sl.log_records {
            let timestamp_ns = if log_record.time_unix_nano != 0 {
                log_record.time_unix_nano
            } else if log_record.observed_time_unix_nano != 0 {
                log_record.observed_time_unix_nano
            } else {
                ingestion_time_ns
            };

            let mut fields: Vec<(u32, Value)> = Vec::new();

            let mut cb = |key: &str, value: visit::LeafValue| {
                let key_idx = intern_key(key, &mut key_map, &mut key_list);
                let owned = match value {
                    visit::LeafValue::Str(s) => Value::Str(s.to_owned()),
                    visit::LeafValue::I64(v) => Value::I64(v),
                    visit::LeafValue::U64(v) => Value::U64(v),
                    visit::LeafValue::F64(v) => Value::F64(v),
                    visit::LeafValue::Bool(v) => Value::Bool(v),
                    visit::LeafValue::Bytes(b) => Value::Bytes(b.to_vec()),
                    visit::LeafValue::Null => Value::Null,
                };
                fields.push((key_idx, owned));
            };

            if let Some(resource) = rl.resource.as_ref() {
                visitor.visit_resource(resource, &mut cb);
            }
            if let Some(scope) = sl.scope.as_ref() {
                visitor.visit_scope(scope, &mut cb);
            }
            visitor.visit_log_record(log_record, &mut cb);

            entries.push(Entry {
                timestamp_ns,
                kv_pairs: fields,
            });
        }
    }

    // Build keys_blob + keys_offsets from the interned keys.
    let mut keys_blob = Vec::new();
    let mut keys_offsets = Vec::with_capacity(key_list.len() + 1);
    for key in &key_list {
        keys_offsets.push(keys_blob.len() as u32);
        keys_blob.extend_from_slice(key.as_bytes());
    }
    keys_offsets.push(keys_blob.len() as u32); // sentinel

    Frame {
        keys_blob,
        keys_offsets,
        entries,
    }
}

/// Encode a `Frame` to bincode bytes.
pub fn encode_frame(frame: &Frame) -> Vec<u8> {
    bincode::serde::encode_to_vec(frame, bincode::config::standard()).unwrap()
}

/// Decode a `Frame` from bincode bytes.
pub fn decode_frame(data: &[u8]) -> Result<Frame, bincode::error::DecodeError> {
    let (frame, _) = bincode::serde::decode_from_slice(data, bincode::config::standard())?;
    Ok(frame)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn roundtrip() {
        let frame = Frame {
            keys_blob: b"hostservicemessage".to_vec(),
            keys_offsets: vec![0, 4, 11, 18],
            entries: vec![
                Entry {
                    timestamp_ns: 1_000_000,
                    kv_pairs: vec![
                        (0, Value::Str("myhost".into())),
                        (1, Value::Str("api".into())),
                        (2, Value::Str("hello world".into())),
                    ],
                },
                Entry {
                    timestamp_ns: 2_000_000,
                    kv_pairs: vec![
                        (0, Value::Str("myhost".into())),
                        (1, Value::Str("web".into())),
                        (2, Value::Null),
                    ],
                },
            ],
        };

        assert_eq!(frame.key(0), "host");
        assert_eq!(frame.key(1), "service");
        assert_eq!(frame.key(2), "message");

        let encoded = encode_frame(&frame);
        let decoded = decode_frame(&encoded).unwrap();

        assert_eq!(decoded.keys_blob, frame.keys_blob);
        assert_eq!(decoded.keys_offsets, frame.keys_offsets);
        assert_eq!(decoded.entries.len(), 2);
        assert_eq!(decoded.entries[0].timestamp_ns, 1_000_000);
        assert_eq!(decoded.entries[0].kv_pairs.len(), 3);
        assert_eq!(decoded.entries[0].kv_pairs[0].0, 0);
        assert_eq!(decoded.key(decoded.entries[0].kv_pairs[0].0), "host");
    }
}
