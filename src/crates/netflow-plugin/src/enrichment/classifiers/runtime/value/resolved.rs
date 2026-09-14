use std::borrow::Cow;

#[derive(Debug, Clone)]
pub(crate) enum ResolvedValue {
    String(String),
    Number(i64),
    List(Vec<ResolvedValue>),
}

impl ResolvedValue {
    pub(crate) fn as_str(&self) -> Option<&str> {
        match self {
            ResolvedValue::String(value) => Some(value.as_str()),
            _ => None,
        }
    }

    pub(crate) fn as_cow_str(&self) -> Cow<'_, str> {
        match self {
            ResolvedValue::String(value) => Cow::Borrowed(value.as_str()),
            ResolvedValue::Number(value) => Cow::Owned(value.to_string()),
            ResolvedValue::List(values) => {
                let mut result = String::new();

                for (index, value) in values.iter().enumerate() {
                    if index > 0 {
                        result.push(',');
                    }
                    let value = value.as_cow_str();
                    result.push_str(value.as_ref());
                }

                Cow::Owned(result)
            }
        }
    }

    pub(crate) fn to_string_value(&self) -> String {
        self.as_cow_str().into_owned()
    }

    pub(crate) fn to_i64(&self) -> Option<i64> {
        match self {
            ResolvedValue::String(value) => value.parse::<i64>().ok(),
            ResolvedValue::Number(value) => Some(*value),
            ResolvedValue::List(_) => None,
        }
    }

    pub(crate) fn as_list(&self) -> Option<&[ResolvedValue]> {
        match self {
            ResolvedValue::List(values) => Some(values.as_slice()),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn list_to_string_value_joins_items_in_order() {
        let value = ResolvedValue::List(vec![
            ResolvedValue::String("alpha".to_string()),
            ResolvedValue::Number(42),
            ResolvedValue::String("beta".to_string()),
        ]);

        assert_eq!(value.to_string_value(), "alpha,42,beta");
    }
}
