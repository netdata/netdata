use std::collections::HashMap;
use std::path::PathBuf;

use arrow::array::*;
use arrow::datatypes::*;
use arrow::record_batch::RecordBatch;
use serde_json::{Map, Value};

use crate::otap_frame::OtapFrame;

// --- Column readers ---

pub fn read_dict_utf8(col: &dyn Array, row: usize) -> Option<String> {
    if col.is_null(row) {
        return None;
    }
    if let Some(dict) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
        let vals = dict.values().as_any().downcast_ref::<StringArray>()?;
        let key = dict.keys().value(row) as usize;
        return Some(vals.value(key).to_string());
    }
    if let Some(dict) = col.as_any().downcast_ref::<DictionaryArray<UInt16Type>>() {
        let vals = dict.values().as_any().downcast_ref::<StringArray>()?;
        let key = dict.keys().value(row) as usize;
        return Some(vals.value(key).to_string());
    }
    if let Some(arr) = col.as_any().downcast_ref::<StringArray>() {
        return Some(arr.value(row).to_string());
    }
    None
}

pub fn read_dict_i64(col: &dyn Array, row: usize) -> Option<i64> {
    if col.is_null(row) {
        return None;
    }
    if let Some(dict) = col.as_any().downcast_ref::<DictionaryArray<UInt16Type>>() {
        let vals = dict.values().as_any().downcast_ref::<Int64Array>()?;
        let key = dict.keys().value(row) as usize;
        return Some(vals.value(key));
    }
    if let Some(dict) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
        let vals = dict.values().as_any().downcast_ref::<Int64Array>()?;
        let key = dict.keys().value(row) as usize;
        return Some(vals.value(key));
    }
    if let Some(arr) = col.as_any().downcast_ref::<Int64Array>() {
        return Some(arr.value(row));
    }
    None
}

fn read_dict_i32(col: &dyn Array, row: usize) -> Option<i32> {
    if col.is_null(row) {
        return None;
    }
    if let Some(dict) = col.as_any().downcast_ref::<DictionaryArray<UInt8Type>>() {
        let vals = dict.values().as_any().downcast_ref::<Int32Array>()?;
        let key = dict.keys().value(row) as usize;
        return Some(vals.value(key));
    }
    if let Some(arr) = col.as_any().downcast_ref::<Int32Array>() {
        return Some(arr.value(row));
    }
    None
}

pub fn read_u16(col: &dyn Array, row: usize) -> Option<u16> {
    if col.is_null(row) {
        return None;
    }
    col.as_any()
        .downcast_ref::<UInt16Array>()
        .map(|a| a.value(row))
}

pub fn read_u8(col: &dyn Array, row: usize) -> Option<u8> {
    if col.is_null(row) {
        return None;
    }
    col.as_any()
        .downcast_ref::<UInt8Array>()
        .map(|a| a.value(row))
}

pub fn read_f64(col: &dyn Array, row: usize) -> Option<f64> {
    if col.is_null(row) {
        return None;
    }
    col.as_any()
        .downcast_ref::<Float64Array>()
        .map(|a| a.value(row))
}

pub fn read_bool(col: &dyn Array, row: usize) -> Option<bool> {
    if col.is_null(row) {
        return None;
    }
    col.as_any()
        .downcast_ref::<BooleanArray>()
        .map(|a| a.value(row))
}

fn read_timestamp_ns(col: &dyn Array, row: usize) -> Option<i64> {
    if col.is_null(row) {
        return None;
    }
    col.as_any()
        .downcast_ref::<TimestampNanosecondArray>()
        .map(|a| a.value(row))
}

fn attr_value_json(rb: &RecordBatch, row: usize) -> Value {
    let type_val = rb
        .column_by_name("type")
        .and_then(|c| read_u8(c.as_ref(), row))
        .unwrap_or(0);

    match type_val {
        1 => rb
            .column_by_name("str")
            .and_then(|c| read_dict_utf8(c.as_ref(), row))
            .map(Value::String)
            .unwrap_or(Value::Null),
        2 => rb
            .column_by_name("int")
            .and_then(|c| read_dict_i64(c.as_ref(), row))
            .map(|v| Value::Number(v.into()))
            .unwrap_or(Value::Null),
        3 => rb
            .column_by_name("double")
            .and_then(|c| read_f64(c.as_ref(), row))
            .and_then(|v| serde_json::Number::from_f64(v).map(Value::Number))
            .unwrap_or(Value::Null),
        4 => rb
            .column_by_name("bool")
            .and_then(|c| read_bool(c.as_ref(), row))
            .map(Value::Bool)
            .unwrap_or(Value::Null),
        _ => Value::Null,
    }
}

// --- Index: group attribute rows by parent_id ---

#[derive(Default)]
struct AttrsIndex {
    map: HashMap<u16, Vec<(String, Value)>>,
}

impl AttrsIndex {
    fn build(rb: &RecordBatch) -> Self {
        let mut map: HashMap<u16, Vec<(String, Value)>> = HashMap::new();
        let Some(pid_col) = rb.column_by_name("parent_id") else {
            return Self { map };
        };
        let Some(key_col) = rb.column_by_name("key") else {
            return Self { map };
        };

        for row in 0..rb.num_rows() {
            let Some(pid) = read_u16(pid_col.as_ref(), row) else {
                continue;
            };
            let key = read_dict_utf8(key_col.as_ref(), row).unwrap_or_default();
            let val = attr_value_json(rb, row);
            map.entry(pid).or_default().push((key, val));
        }

        Self { map }
    }

    fn get(&self, parent_id: u16) -> &[(String, Value)] {
        self.map
            .get(&parent_id)
            .map(|v| v.as_slice())
            .unwrap_or(&[])
    }
}

// --- Logs batch column accessors ---

struct LogsColumns<'a> {
    num_rows: usize,
    id: Option<&'a dyn Array>,
    resource_id: Option<&'a dyn Array>,
    scope_id: Option<&'a dyn Array>,
    scope_name: Option<&'a dyn Array>,
    scope_version: Option<&'a dyn Array>,
    time_unix_nano: Option<&'a dyn Array>,
    observed_time_unix_nano: Option<&'a dyn Array>,
    severity_number: Option<&'a dyn Array>,
    severity_text: Option<&'a dyn Array>,
    event_name: Option<&'a dyn Array>,
}

impl<'a> LogsColumns<'a> {
    fn from_batch(rb: &'a RecordBatch) -> Self {
        let resource_struct = rb
            .column_by_name("resource")
            .and_then(|c| c.as_any().downcast_ref::<StructArray>());
        let scope_struct = rb
            .column_by_name("scope")
            .and_then(|c| c.as_any().downcast_ref::<StructArray>());

        Self {
            num_rows: rb.num_rows(),
            id: rb.column_by_name("id").map(|c| c.as_ref()),
            resource_id: resource_struct
                .and_then(|s| s.column_by_name("id"))
                .map(|c| c.as_ref()),
            scope_id: scope_struct
                .and_then(|s| s.column_by_name("id"))
                .map(|c| c.as_ref()),
            scope_name: scope_struct
                .and_then(|s| s.column_by_name("name"))
                .map(|c| c.as_ref()),
            scope_version: scope_struct
                .and_then(|s| s.column_by_name("version"))
                .map(|c| c.as_ref()),
            time_unix_nano: rb.column_by_name("time_unix_nano").map(|c| c.as_ref()),
            observed_time_unix_nano: rb
                .column_by_name("observed_time_unix_nano")
                .map(|c| c.as_ref()),
            severity_number: rb.column_by_name("severity_number").map(|c| c.as_ref()),
            severity_text: rb.column_by_name("severity_text").map(|c| c.as_ref()),
            event_name: rb.column_by_name("event_name").map(|c| c.as_ref()),
        }
    }
}

fn log_entry_json(
    cols: &LogsColumns,
    row: usize,
    log_attrs: &AttrsIndex,
    resource_attrs: &AttrsIndex,
    scope_attrs: &AttrsIndex,
) -> Value {
    let mut obj = Map::new();

    // Timestamps
    if let Some(ts) = cols.time_unix_nano.and_then(|c| read_timestamp_ns(c, row)) {
        obj.insert("time_unix_nano".into(), ts.into());
    }
    if let Some(ts) = cols
        .observed_time_unix_nano
        .and_then(|c| read_timestamp_ns(c, row))
    {
        obj.insert("observed_time_unix_nano".into(), ts.into());
    }

    // Severity
    if let Some(sn) = cols.severity_number.and_then(|c| read_dict_i32(c, row)) {
        obj.insert("severity_number".into(), sn.into());
    }
    if let Some(st) = cols.severity_text.and_then(|c| read_dict_utf8(c, row)) {
        obj.insert("severity_text".into(), Value::String(st));
    }

    // Scope
    if let Some(name) = cols.scope_name.and_then(|c| read_dict_utf8(c, row)) {
        if !name.is_empty() {
            obj.insert("scope.name".into(), Value::String(name));
        }
    }
    if let Some(ver) = cols.scope_version.and_then(|c| read_dict_utf8(c, row)) {
        if !ver.is_empty() {
            obj.insert("scope.version".into(), Value::String(ver));
        }
    }

    // Event name
    if let Some(en) = cols.event_name.and_then(|c| read_dict_utf8(c, row)) {
        if !en.is_empty() {
            obj.insert("event_name".into(), Value::String(en));
        }
    }

    // Resource attributes
    if let Some(rid) = cols.resource_id.and_then(|c| read_u16(c, row)) {
        for (k, v) in resource_attrs.get(rid) {
            obj.insert(k.clone(), v.clone());
        }
    }

    // Scope attributes
    if let Some(sid) = cols.scope_id.and_then(|c| read_u16(c, row)) {
        for (k, v) in scope_attrs.get(sid) {
            obj.insert(k.clone(), v.clone());
        }
    }

    // Log attributes
    if let Some(lid) = cols.id.and_then(|c| read_u16(c, row)) {
        for (k, v) in log_attrs.get(lid) {
            obj.insert(k.clone(), v.clone());
        }
    }

    Value::Object(obj)
}

pub fn run(path: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let mut reader = wal::WalReader::open(path)?;
    let stdout = std::io::stdout();
    let mut out = std::io::BufWriter::new(stdout.lock());

    while let Some(wal_frame) = reader.next_frame()? {
        let otap_frame = OtapFrame::decode(wal_frame.data)?;

        let Some(logs_rb) = otap_frame.logs.as_ref() else {
            continue;
        };

        let resource_attrs_idx = otap_frame
            .resource_attrs
            .as_ref()
            .map(AttrsIndex::build)
            .unwrap_or_default();

        let scope_attrs_idx = otap_frame
            .scope_attrs
            .as_ref()
            .map(AttrsIndex::build)
            .unwrap_or_default();

        let log_attrs_idx = otap_frame
            .log_attrs
            .as_ref()
            .map(AttrsIndex::build)
            .unwrap_or_default();

        let cols = LogsColumns::from_batch(logs_rb);

        for row in 0..cols.num_rows {
            let entry = log_entry_json(
                &cols,
                row,
                &log_attrs_idx,
                &resource_attrs_idx,
                &scope_attrs_idx,
            );
            serde_json::to_writer(&mut out, &entry)?;
            use std::io::Write;
            out.write_all(b"\n")?;
        }
    }

    Ok(())
}
