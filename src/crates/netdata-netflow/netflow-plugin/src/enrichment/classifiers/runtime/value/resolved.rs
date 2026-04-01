#[derive(Debug, Clone)]
pub(crate) enum ResolvedValue {
    String(String),
    Number(i64),
    List(Vec<ResolvedValue>),
}

impl ResolvedValue {
    pub(crate) fn to_string_value(&self) -> String {
        match self {
            ResolvedValue::String(value) => value.clone(),
            ResolvedValue::Number(value) => value.to_string(),
            ResolvedValue::List(values) => values
                .iter()
                .map(ResolvedValue::to_string_value)
                .collect::<Vec<_>>()
                .join(","),
        }
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
