#![allow(dead_code)]

use fake::rand::{seq::SliceRandom, thread_rng, Rng};
use fake::Fake;
use opentelemetry_proto::tonic::common::v1::{
    any_value, AnyValue, ArrayValue, KeyValue, KeyValueList,
};

use serde_json;

#[derive(Debug, Clone)]
struct AnyValueGenerator {
    max_depth: u8,
    current_depth: u8,
}

impl AnyValueGenerator {
    fn new() -> Self {
        Self {
            max_depth: 5,
            current_depth: 0,
        }
    }

    fn generate(&mut self) -> AnyValue {
        let value_types = if self.current_depth >= self.max_depth {
            &[
                Self::string_value,
                Self::bool_value,
                Self::int_value,
                Self::double_value,
                Self::bytes_value,
            ][..]
        } else {
            &[
                Self::string_value,
                Self::bool_value,
                Self::int_value,
                Self::double_value,
                Self::bytes_value,
                Self::array_value,
                Self::kvlist_value,
            ][..]
        };

        self.current_depth += 1;
        let av = value_types[thread_rng().gen_range(0..value_types.len())](self);
        self.current_depth -= 1;

        return AnyValue { value: Some(av) };
    }

    fn bool_value(&mut self) -> any_value::Value {
        any_value::Value::BoolValue(thread_rng().gen())
    }

    fn int_value(&mut self) -> any_value::Value {
        any_value::Value::IntValue(thread_rng().gen_range(-10..10))
    }

    fn double_value(&mut self) -> any_value::Value {
        any_value::Value::DoubleValue(thread_rng().gen_range(-100.0..100.0))
    }

    fn bytes_value(&mut self) -> any_value::Value {
        let length = thread_rng().gen_range(1..5);
        let bytes: Vec<u8> = (0..length).map(|_| thread_rng().gen::<u8>()).collect();

        any_value::Value::BytesValue(bytes)
    }

    fn string_value(&mut self) -> any_value::Value {
        any_value::Value::StringValue(format!("string-{}", (1..100).fake::<i32>()))
    }

    fn array_value(&mut self) -> any_value::Value {
        let length = thread_rng().gen_range(1..5);
        let values: Vec<AnyValue> = (0..length).map(|_| self.generate()).collect();

        any_value::Value::ArrayValue(ArrayValue { values })
    }

    fn kvlist_value(&mut self) -> any_value::Value {
        let length = thread_rng().gen_range(1..5);
        let mut nums: Vec<i32> = (0..length).map(|i| i as i32).collect();
        nums.shuffle(&mut thread_rng());

        let values: Vec<KeyValue> = nums
            .into_iter()
            .map(|n| KeyValue {
                key: format!("key-{}", n),
                value: Some(self.generate()),
            })
            .collect();

        any_value::Value::KvlistValue(KeyValueList { values })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_random_any_value() {
        let mut generator = AnyValueGenerator::new();

        for _ in 0..10 {
            let av = generator.generate();
            let j = serde_json::to_string_pretty(&av);

            debug_assert!(j.is_ok());

            println!("{}", j.unwrap());
        }
    }
}
