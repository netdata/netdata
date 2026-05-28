use super::display::field_display_name;
use super::labels::field_value_labels;
use serde_json::{Map, Value, json};

const FULL_WIDTH_FIELDS: &[&str] = &["SRC_AS_NAME", "DST_AS_NAME"];
const HIDDEN_GROUP_FIELDS: &[&str] = &[
    "SRC_GEO_LATITUDE",
    "SRC_GEO_LONGITUDE",
    "DST_GEO_LATITUDE",
    "DST_GEO_LONGITUDE",
];

pub(crate) fn build_table_columns(group_by: &[String]) -> Value {
    let mut columns = Map::new();

    for (index, field) in group_by.iter().enumerate() {
        columns.insert(field.clone(), build_group_column(field, index));
    }

    let base_index = group_by.len();
    columns.insert(
        "bytes".to_string(),
        json!({
            "index": base_index,
            "name": "Bytes",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "value_options": {
                "transform": "number",
                "units": "B",
                "decimal_points": 2,
            },
        }),
    );
    columns.insert(
        "packets".to_string(),
        json!({
            "index": base_index + 1,
            "name": "Packets",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "value_options": {
                "transform": "number",
                "units": "packets",
                "decimal_points": 0,
            },
        }),
    );

    Value::Object(columns)
}

pub(crate) fn build_timeseries_columns(group_by: &[String]) -> Value {
    let mut columns = Map::new();
    for (index, field) in group_by.iter().enumerate() {
        columns.insert(field.clone(), build_group_column(field, index));
    }
    Value::Object(columns)
}

fn build_group_column(field: &str, index: usize) -> Value {
    let mut value_options = Map::new();
    if let Some(labels) = field_value_labels(field) {
        value_options.insert("labels".to_string(), Value::Object(labels));
    }

    let mut column = Map::new();
    column.insert("index".to_string(), json!(index));
    column.insert("name".to_string(), json!(field_display_name(field)));
    column.insert("type".to_string(), json!("string"));
    column.insert("visualization".to_string(), json!("value"));
    column.insert("sort".to_string(), json!("ascending"));
    column.insert("sortable".to_string(), json!(true));
    column.insert("filter".to_string(), json!("multiselect"));
    if FULL_WIDTH_FIELDS.contains(&field) {
        column.insert("full_width".to_string(), json!(true));
    }
    if HIDDEN_GROUP_FIELDS.contains(&field) {
        column.insert("visible".to_string(), json!(false));
    }
    if !value_options.is_empty() {
        column.insert("value_options".to_string(), Value::Object(value_options));
    }

    Value::Object(column)
}
