use serde_json::{Map as JsonMap, Value as JsonValue};

use opentelemetry_proto::tonic::{
    collector::logs::v1::ExportLogsServiceRequest, logs::v1::LogRecord,
};

use crate::{
    json_from_any_value, json_from_instrumentation_scope, json_from_key_value_list,
    json_from_resource,
};

pub fn json_from_log_record(jm: &mut JsonMap<String, JsonValue>, log_record: &LogRecord) {
    // Add log record fields with "log." prefix
    jm.insert(
        "log.time_unix_nano".to_string(),
        JsonValue::Number(serde_json::Number::from(log_record.time_unix_nano)),
    );
    jm.insert(
        "log.observed_time_unix_nano".to_string(),
        JsonValue::Number(serde_json::Number::from(log_record.observed_time_unix_nano)),
    );
    jm.insert(
        "log.severity_number".to_string(),
        JsonValue::Number(serde_json::Number::from(log_record.severity_number)),
    );
    jm.insert(
        "log.severity_text".to_string(),
        JsonValue::String(log_record.severity_text.clone()),
    );

    // Add body if present
    if let Some(body) = &log_record.body {
        let body_json = json_from_any_value(body);

        match &body_json {
            // If body is a string, try to parse it as JSON first
            JsonValue::String(s) => {
                // Try to parse the string as JSON
                if let Ok(parsed) = serde_json::from_str::<JsonValue>(s) {
                    // Successfully parsed JSON - check if it's an object
                    if let JsonValue::Object(_) = parsed {
                        // Flatten the parsed JSON object
                        let mut temp_map = JsonMap::new();
                        temp_map.insert("body".to_string(), parsed);

                        let flattened_body = flatten_serde_json::flatten(&temp_map);
                        for (key, value) in flattened_body {
                            jm.insert(format!("log.{}", key), value);
                        }
                    } else {
                        // Parsed JSON but not an object (array, primitive, etc.)
                        // Store the original string
                        jm.insert("log.body".to_string(), body_json);
                    }
                } else {
                    // Not valid JSON, store as-is
                    jm.insert("log.body".to_string(), body_json);
                }
            }
            // If body is a number or bool, add it directly
            JsonValue::Number(_) | JsonValue::Bool(_) => {
                jm.insert("log.body".to_string(), body_json);
            }
            // If body is structured, flatten it
            JsonValue::Object(_) => {
                let mut temp_map = JsonMap::new();
                temp_map.insert("body".to_string(), body_json);

                let flattened_body = flatten_serde_json::flatten(&temp_map);
                for (key, value) in flattened_body {
                    jm.insert(format!("log.{}", key), value);
                }
            }
            _ => {
                // Arrays, null, etc. - add as-is
                jm.insert("log.body".to_string(), body_json);
            }
        }
    }

    // Add event name
    jm.insert(
        "log.event_name".to_string(),
        JsonValue::String(log_record.event_name.clone()),
    );

    // Add log record attributes
    let log_attrs = json_from_key_value_list(&log_record.attributes);
    for (key, value) in log_attrs {
        jm.insert(format!("log.attributes.{}", key), value);
    }

    jm.insert(
        "log.dropped_attributes_count".to_string(),
        JsonValue::Number(serde_json::Number::from(
            log_record.dropped_attributes_count,
        )),
    );
    jm.insert(
        "log.flags".to_string(),
        JsonValue::Number(serde_json::Number::from(log_record.flags)),
    );

    // Add trace_id and span_id as hex strings
    // if !log_record.trace_id.is_empty() {
    //     jm.insert(
    //         "log.trace_id".to_string(),
    //         JsonValue::String(hex::encode(&log_record.trace_id)),
    //     );
    // }
    // if !log_record.span_id.is_empty() {
    //     jm.insert(
    //         "log.span_id".to_string(),
    //         JsonValue::String(hex::encode(&log_record.span_id)),
    //     );
    // }
}

// TODO: this does not belong here, it should be part of the service.
#[tracing::instrument(skip_all)]
pub fn json_from_export_logs_service_request(request: &ExportLogsServiceRequest) -> JsonValue {
    let mut items = Vec::new();

    for resource_logs in &request.resource_logs {
        for scope_logs in &resource_logs.scope_logs {
            for log_record in &scope_logs.log_records {
                let mut jm = JsonMap::new();

                // Add resource information
                if let Some(resource) = resource_logs.resource.as_ref() {
                    json_from_resource(&mut jm, resource);
                }

                // Add resource schema URL
                if !resource_logs.schema_url.is_empty() {
                    jm.insert(
                        "resource.schema_url".to_string(),
                        JsonValue::String(resource_logs.schema_url.clone()),
                    );
                }

                // Add scope information
                if let Some(scope) = scope_logs.scope.as_ref() {
                    json_from_instrumentation_scope(&mut jm, scope);
                }

                // Add scope schema URL
                if !scope_logs.schema_url.is_empty() {
                    jm.insert(
                        "scope.schema_url".to_string(),
                        JsonValue::String(scope_logs.schema_url.clone()),
                    );
                }

                // Add log record information
                json_from_log_record(&mut jm, log_record);

                items.push(JsonValue::Object(jm));
            }
        }
    }

    JsonValue::Array(items)
}
