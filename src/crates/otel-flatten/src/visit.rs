use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, InstrumentationScope, KeyValue, any_value::Value},
    logs::v1::LogRecord,
    resource::v1::Resource,
};

#[derive(Debug)]
pub enum LeafValue<'a> {
    Str(&'a str),
    I64(i64),
    U64(u64),
    F64(f64),
    Bool(bool),
    Bytes(&'a [u8]),
    Null,
}

pub struct FlatVisitor {
    key_buf: String,
}

impl FlatVisitor {
    pub fn new() -> Self {
        Self {
            key_buf: String::with_capacity(256),
        }
    }

    pub fn visit_resource(&mut self, resource: &Resource, cb: &mut impl FnMut(&str, LeafValue)) {
        self.visit_key_value_list(&resource.attributes, "resource.attributes", cb);
    }

    pub fn visit_scope(
        &mut self,
        scope: &InstrumentationScope,
        cb: &mut impl FnMut(&str, LeafValue),
    ) {
        if !scope.name.is_empty() {
            cb("scope.name", LeafValue::Str(&scope.name));
        }
        if !scope.version.is_empty() {
            cb("scope.version", LeafValue::Str(&scope.version));
        }
        self.visit_key_value_list(&scope.attributes, "scope.attributes", cb);
    }

    pub fn visit_log_record(&mut self, lr: &LogRecord, cb: &mut impl FnMut(&str, LeafValue)) {
        cb("log.time_unix_nano", LeafValue::U64(lr.time_unix_nano));
        cb(
            "log.observed_time_unix_nano",
            LeafValue::U64(lr.observed_time_unix_nano),
        );
        cb(
            "log.severity_number",
            LeafValue::I64(lr.severity_number as i64),
        );
        if !lr.severity_text.is_empty() {
            cb("log.severity_text", LeafValue::Str(&lr.severity_text));
        }

        if let Some(body) = &lr.body {
            self.key_buf.clear();
            self.key_buf.push_str("log.body");
            self.visit_any_value(body, cb);
            self.key_buf.clear();
        }

        if !lr.event_name.is_empty() {
            cb("log.event_name", LeafValue::Str(&lr.event_name));
        }

        self.visit_key_value_list(&lr.attributes, "log.attributes", cb);

        cb(
            "log.dropped_attributes_count",
            LeafValue::U64(lr.dropped_attributes_count as u64),
        );
        cb("log.flags", LeafValue::U64(lr.flags as u64));
    }

    fn visit_key_value_list(
        &mut self,
        kvl: &[KeyValue],
        prefix: &str,
        cb: &mut impl FnMut(&str, LeafValue),
    ) {
        for kv in kvl {
            self.key_buf.clear();
            self.key_buf.push_str(prefix);
            self.push_segment(&kv.key);

            if let Some(value) = &kv.value {
                self.visit_any_value(value, cb);
            } else {
                cb(&self.key_buf, LeafValue::Null);
            }
        }
        self.key_buf.clear();
    }

    fn visit_any_value(&mut self, av: &AnyValue, cb: &mut impl FnMut(&str, LeafValue)) {
        match &av.value {
            Some(Value::StringValue(s)) => cb(&self.key_buf, LeafValue::Str(s)),
            Some(Value::IntValue(i)) => cb(&self.key_buf, LeafValue::I64(*i)),
            Some(Value::DoubleValue(d)) => cb(&self.key_buf, LeafValue::F64(*d)),
            Some(Value::BoolValue(b)) => cb(&self.key_buf, LeafValue::Bool(*b)),
            Some(Value::BytesValue(bytes)) => cb(&self.key_buf, LeafValue::Bytes(bytes)),
            Some(Value::ArrayValue(arr)) => {
                for item in &arr.values {
                    self.visit_any_value(item, cb);
                }
            }
            Some(Value::KvlistValue(kvl)) => {
                let base = self.key_buf.len();
                for kv in &kvl.values {
                    self.key_buf.truncate(base);
                    self.push_segment(&kv.key);
                    if let Some(value) = &kv.value {
                        self.visit_any_value(value, cb);
                    } else {
                        cb(&self.key_buf, LeafValue::Null);
                    }
                }
                self.key_buf.truncate(base);
            }
            None => cb(&self.key_buf, LeafValue::Null),
        }
    }

    fn push_segment(&mut self, segment: &str) {
        self.key_buf.push('.');
        self.key_buf.push_str(segment);
    }
}
