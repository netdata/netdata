use super::super::super::*;
use super::normalize_group_by_values;

pub(crate) fn take_selection_view(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<ViewMode, String>> {
    take_single_selection_value(selections, "VIEW")
        .map(|value| value.and_then(|value| parse_enum_selection("view", &value)))
}

pub(crate) fn take_selection_sort_by(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<SortBy, String>> {
    take_single_selection_value(selections, "SORT_BY")
        .map(|value| value.and_then(|value| parse_enum_selection("sort_by", &value)))
}

pub(crate) fn take_selection_top_n(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<TopN, String>> {
    take_single_selection_value(selections, "TOP_N")
        .map(|value| value.and_then(|value| parse_enum_selection("top_n", &value)))
}

pub(crate) fn take_selection_group_by(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<Vec<String>, String>> {
    selections.remove("GROUP_BY").map(normalize_group_by_values)
}

pub(crate) fn take_single_selection_value(
    selections: &mut HashMap<String, Vec<String>>,
    key: &str,
) -> Option<std::result::Result<String, String>> {
    selections
        .remove(key)
        .map(|values| match values.as_slice() {
            [] => Err(format!("selection `{key}` is empty")),
            [value] => Ok(value.clone()),
            _ => Err(format!("selection `{key}` must contain exactly one value")),
        })
}

pub(crate) fn parse_enum_selection<T>(field: &str, value: &str) -> std::result::Result<T, String>
where
    T: for<'de> Deserialize<'de>,
{
    serde_json::from_value::<T>(Value::String(value.to_string()))
        .map_err(|err| format!("invalid {field}: {err}"))
}
