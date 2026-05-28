use super::super::super::*;
use super::{
    FacetSelection, GroupBySelection, facet_selection_to_values, group_by_selection_to_values,
};

pub(crate) fn deserialize_selections<'de, D>(
    deserializer: D,
) -> std::result::Result<HashMap<String, Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let raw = Option::<HashMap<String, Value>>::deserialize(deserializer)?.unwrap_or_default();
    let mut selections = HashMap::with_capacity(raw.len());

    for (field, value) in raw {
        let values = selection_values_from_json(value).map_err(D::Error::custom)?;
        if values.is_empty() {
            continue;
        }
        selections.insert(field.to_ascii_uppercase(), values);
    }

    Ok(selections)
}

pub(crate) fn deserialize_optional_group_by<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let selection = Option::<GroupBySelection>::deserialize(deserializer)?;
    selection
        .map(group_by_selection_to_values)
        .transpose()
        .map_err(D::Error::custom)
}

pub(crate) fn deserialize_optional_facet_fields<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let selection = Option::<FacetSelection>::deserialize(deserializer)?;
    selection
        .map(facet_selection_to_values)
        .transpose()
        .map_err(D::Error::custom)
}

pub(crate) fn selection_values_from_json(value: Value) -> std::result::Result<Vec<String>, String> {
    match value {
        Value::Null => Ok(Vec::new()),
        Value::String(value) => Ok(non_empty_selection(value).into_iter().collect()),
        Value::Number(value) => Ok(vec![value.to_string()]),
        Value::Bool(value) => Ok(vec![value.to_string()]),
        Value::Array(values) => {
            let mut out = Vec::new();
            let mut seen = HashSet::new();
            for value in values {
                if let Some(value) = selection_scalar_from_json(value)? {
                    if seen.insert(value.clone()) {
                        out.push(value);
                    }
                }
            }
            Ok(out)
        }
        Value::Object(map) => {
            selection_scalar_from_object(&map).map(|value| value.into_iter().collect())
        }
    }
}

pub(crate) fn selection_scalar_from_json(
    value: Value,
) -> std::result::Result<Option<String>, String> {
    match value {
        Value::Null => Ok(None),
        Value::String(value) => Ok(non_empty_selection(value)),
        Value::Number(value) => Ok(Some(value.to_string())),
        Value::Bool(value) => Ok(Some(value.to_string())),
        Value::Object(map) => selection_scalar_from_object(&map),
        Value::Array(_) => Err("nested arrays are not supported in selections".to_string()),
    }
}

pub(crate) fn selection_scalar_from_object(
    map: &Map<String, Value>,
) -> std::result::Result<Option<String>, String> {
    for key in ["id", "value", "name"] {
        if let Some(value) = map.get(key) {
            return selection_scalar_from_json(value.clone());
        }
    }

    Err("selection objects must contain `id`, `value`, or `name`".to_string())
}

pub(crate) fn non_empty_selection(value: String) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}
