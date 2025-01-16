use opentelemetry_proto::tonic::common::v1::{any_value, AnyValue, KeyValue};

use std::fmt::Write;

pub trait OtelFormat {
    fn otel_format(&self, out: &mut String) -> std::fmt::Result;
}

mod value {
    use super::*;

    impl OtelFormat for any_value::Value {
        fn otel_format(&self, out: &mut String) -> std::fmt::Result {
            match self {
                any_value::Value::StringValue(s) => {
                    out.push_str(s);
                }
                any_value::Value::BoolValue(b) => {
                    write!(out, "{}", b)?;
                }
                any_value::Value::IntValue(i) => {
                    write!(out, "{}", i)?;
                }
                any_value::Value::DoubleValue(d) => {
                    write!(out, "{}", d)?;
                }
                any_value::Value::BytesValue(bytes) => {
                    write!(out, "0x")?;
                    for byte in bytes {
                        write!(out, "{:02x}", byte)?;
                    }
                }
                any_value::Value::ArrayValue(arr) => {
                    out.push('[');
                    for (i, value) in arr.values.iter().enumerate() {
                        if i > 0 {
                            out.push_str(", ");
                        }
                        value.otel_format(out)?;
                    }
                    out.push(']');
                }
                any_value::Value::KvlistValue(kvlist) => {
                    out.push('{');
                    for (i, kv) in kvlist.values.iter().enumerate() {
                        if i > 0 {
                            out.push_str(", ");
                        }
                        kv.otel_format(out)?;
                    }
                    out.push('}');
                }
            }

            Ok(())
        }
    }
}

mod common {
    use super::*;

    impl OtelFormat for AnyValue {
        fn otel_format(&self, out: &mut String) -> std::fmt::Result {
            if let Some(value) = &self.value {
                value.otel_format(out)?;
            }
            Ok(())
        }
    }

    impl OtelFormat for KeyValue {
        fn otel_format(&self, out: &mut String) -> std::fmt::Result {
            out.push_str(&self.key);
            out.push(':');
            if let Some(value) = &self.value {
                out.push(' ');
                value.otel_format(out)?;
            }
            Ok(())
        }
    }
}
