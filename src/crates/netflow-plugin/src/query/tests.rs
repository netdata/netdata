use super::planner::{grouped_query_can_use_projected_scan, resolve_effective_group_by};
use super::scan::cursor_prefilter_pairs;
use super::{
    FlowsRequest, SortBy, build_aggregated_flows, build_facets, build_grouped_flows,
    dimensions_from_fields, metrics_from_fields, requires_raw_tier_for_fields,
};
use crate::rollup::build_rollup_key;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::time::Instant;

#[test]
fn rollup_dimensions_exclude_only_metrics_and_internal() {
    let mut fields: crate::decoder::FlowFields = BTreeMap::new();
    fields.insert("_BOOT_ID", "boot".to_string());
    fields.insert("_SOURCE_REALTIME_TIMESTAMP", "1".to_string());
    fields.insert("V9_IN_BYTES", "10".to_string());
    fields.insert("SRC_ADDR", "10.0.0.1".to_string());
    fields.insert("DST_ADDR", "10.0.0.2".to_string());
    fields.insert("PROTOCOL", "6".to_string());
    fields.insert("BYTES", "10".to_string());

    let dims = dimensions_from_fields(&fields);
    assert!(dims.contains_key("SRC_ADDR"));
    assert!(dims.contains_key("DST_ADDR"));
    assert!(dims.contains_key("PROTOCOL"));
    assert!(!dims.contains_key("V9_IN_BYTES"));
    assert!(!dims.contains_key("BYTES"));
    assert!(!dims.contains_key("_BOOT_ID"));
    assert!(!dims.contains_key("_SOURCE_REALTIME_TIMESTAMP"));

    let key = build_rollup_key(&dims);
    assert!(!key.0.iter().any(|(k, _)| *k == "SRC_ADDR"));
    assert!(!key.0.iter().any(|(k, _)| *k == "DST_ADDR"));
}

#[test]
fn metrics_default_flow_count_is_zero() {
    let fields = BTreeMap::new();
    let metrics = metrics_from_fields(&fields);
    assert_eq!(metrics.bytes, 0);
    assert_eq!(metrics.packets, 0);
}

#[test]
fn aggregated_flow_does_not_expose_top_level_endpoints() {
    let records = vec![
        super::QueryFlowRecord {
            timestamp_usec: 100,
            fields: BTreeMap::from([
                ("FLOW_VERSION".to_string(), "v5".to_string()),
                ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                ("SRC_ADDR".to_string(), "10.0.0.1".to_string()),
                ("DST_ADDR".to_string(), "10.0.0.2".to_string()),
                ("PROTOCOL".to_string(), "6".to_string()),
                ("BYTES".to_string(), "100".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 200,
            fields: BTreeMap::from([
                ("FLOW_VERSION".to_string(), "v5".to_string()),
                ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                ("SRC_ADDR".to_string(), "10.0.0.3".to_string()),
                ("DST_ADDR".to_string(), "10.0.0.4".to_string()),
                ("PROTOCOL".to_string(), "6".to_string()),
                ("BYTES".to_string(), "50".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
    ];

    let result = build_aggregated_flows(&records);
    assert_eq!(result.flows.len(), 1);
    let flow = &result.flows[0];
    assert_eq!(flow["metrics"]["bytes"], 150);
    assert!(flow.get("src").is_none());
    assert!(flow.get("dst").is_none());
}

#[test]
fn raw_tier_is_required_for_ip_or_port_fields() {
    let group_by = vec!["SRC_ADDR".to_string(), "PROTOCOL".to_string()];
    let selections = HashMap::from([("DST_PORT".to_string(), vec!["443".to_string()])]);
    assert!(requires_raw_tier_for_fields(&group_by, &selections, ""));
}

#[test]
fn raw_tier_is_required_for_city_fields() {
    let group_by = vec!["SRC_GEO_CITY".to_string(), "DST_GEO_STATE".to_string()];
    let selections = HashMap::new();
    assert!(requires_raw_tier_for_fields(&group_by, &selections, ""));
}

#[test]
fn grouped_flows_add_other_bucket_when_truncated() {
    let mut records = Vec::new();
    for idx in 0..3 {
        records.push(super::QueryFlowRecord {
            timestamp_usec: 100 + idx,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                ("BYTES".to_string(), format!("{}", 100 - (idx * 10))),
                ("PACKETS".to_string(), format!("{}", 10 + idx)),
            ]),
        });
    }

    let result = build_grouped_flows(&records, &["BYTES".to_string()], SortBy::Packets, 2);
    assert!(result.truncated);
    assert_eq!(result.other_count, 1);
    assert_eq!(result.flows.len(), 3);
    assert_eq!(result.flows[2]["key"]["_bucket"], "__other__");
}

#[test]
fn grouped_other_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
    let records = vec![
        super::QueryFlowRecord {
            timestamp_usec: 100,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Alpha".to_string()),
                ("BYTES".to_string(), "300".to_string()),
                ("PACKETS".to_string(), "3".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 101,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Beta".to_string()),
                ("BYTES".to_string(), "200".to_string()),
                ("PACKETS".to_string(), "2".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 102,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Gamma".to_string()),
                ("BYTES".to_string(), "100".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
    ];

    let result = build_grouped_flows(
        &records,
        &["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()],
        SortBy::Bytes,
        1,
    );

    assert!(result.truncated);
    assert_eq!(result.other_count, 2);
    assert_eq!(result.flows.len(), 2);
    assert_eq!(result.flows[1]["key"]["_bucket"], "__other__");
    assert_eq!(result.flows[1]["key"]["PROTOCOL"], "6");
    assert_eq!(result.flows[1]["key"]["SRC_AS_NAME"], "Other (2)");
}

#[test]
fn facets_include_ip_and_port_fields_when_present() {
    let records = vec![super::QueryFlowRecord {
        timestamp_usec: 100,
        fields: BTreeMap::from([
            ("SRC_ADDR".to_string(), "10.0.0.1".to_string()),
            ("DST_PORT".to_string(), "443".to_string()),
            ("PROTOCOL".to_string(), "6".to_string()),
            ("BYTES".to_string(), "10".to_string()),
            ("PACKETS".to_string(), "1".to_string()),
        ]),
    }];

    let facets = build_facets(
        &records,
        SortBy::Bytes,
        &["PROTOCOL".to_string()],
        &FlowsRequest::default(),
    );

    let fields = facets["fields"].as_array().expect("fields array");
    assert!(fields.iter().any(|entry| entry["field"] == "PROTOCOL"));
    assert!(fields.iter().any(|entry| entry["field"] == "SRC_ADDR"));
    assert!(fields.iter().any(|entry| entry["field"] == "DST_PORT"));
}

#[test]
fn default_group_by_uses_selected_tuple_defaults() {
    let request = FlowsRequest::default();
    let group_by = resolve_effective_group_by(&request);
    assert_eq!(
        group_by,
        vec![
            "SRC_AS_NAME".to_string(),
            "PROTOCOL".to_string(),
            "DST_AS_NAME".to_string()
        ]
    );
}

#[test]
fn country_map_view_replaces_user_group_by_with_country_pair() {
    let request = FlowsRequest {
        view: super::ViewMode::CountryMap,
        group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
        ..FlowsRequest::default()
    };
    let group_by = resolve_effective_group_by(&request);
    assert_eq!(
        group_by,
        vec!["SRC_COUNTRY".to_string(), "DST_COUNTRY".to_string()]
    );
}

#[test]
fn state_map_view_replaces_user_group_by_with_state_pairs() {
    let request = FlowsRequest {
        view: super::ViewMode::StateMap,
        group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
        ..FlowsRequest::default()
    };
    let group_by = resolve_effective_group_by(&request);
    assert_eq!(
        group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string()
        ]
    );
}

#[test]
fn city_map_view_replaces_user_group_by_with_city_and_coordinate_tuple() {
    let request = FlowsRequest {
        view: super::ViewMode::CityMap,
        group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
        ..FlowsRequest::default()
    };
    let group_by = resolve_effective_group_by(&request);
    assert_eq!(
        group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "SRC_GEO_CITY".to_string(),
            "SRC_GEO_LATITUDE".to_string(),
            "SRC_GEO_LONGITUDE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string(),
            "DST_GEO_CITY".to_string(),
            "DST_GEO_LATITUDE".to_string(),
            "DST_GEO_LONGITUDE".to_string(),
        ]
    );
}

#[test]
fn supported_group_by_fields_exclude_metrics_and_include_defaults() {
    let fields = super::supported_group_by_fields();
    assert!(fields.iter().any(|field| field == "SRC_AS"));
    assert!(fields.iter().any(|field| field == "DST_AS"));
    assert!(fields.iter().any(|field| field == "SRC_AS_NAME"));
    assert!(fields.iter().any(|field| field == "DST_AS_NAME"));
    assert!(fields.iter().any(|field| field == "PROTOCOL"));
    assert!(fields.iter().any(|field| field == "ICMPV4"));
    assert!(fields.iter().any(|field| field == "ICMPV6"));
    assert!(!fields.iter().any(|field| field == "SRC_GEO_LATITUDE"));
    assert!(!fields.iter().any(|field| field == "DST_GEO_LONGITUDE"));
    assert!(!fields.iter().any(|field| field == "BYTES"));
    assert!(!fields.iter().any(|field| field == "PACKETS"));
    assert!(!fields.iter().any(|field| field == "RAW_BYTES"));
    assert!(!fields.iter().any(|field| field == "RAW_PACKETS"));
}

#[test]
fn normalized_group_by_is_capped_to_ten_fields() {
    let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"table-sankey",
                "group_by":["FLOW_VERSION","EXPORTER_IP","EXPORTER_PORT","EXPORTER_NAME","PROTOCOL","SRC_ADDR","DST_ADDR","SRC_PORT","DST_PORT","SRC_AS_NAME","DST_AS_NAME","SRC_COUNTRY"],
                "sort_by":"bytes",
                "top_n":"25"
            }"#,
        )
        .expect("request should deserialize");

    assert_eq!(
        request.normalized_group_by(),
        vec![
            "FLOW_VERSION".to_string(),
            "EXPORTER_IP".to_string(),
            "EXPORTER_PORT".to_string(),
            "EXPORTER_NAME".to_string(),
            "PROTOCOL".to_string(),
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "SRC_PORT".to_string(),
            "DST_PORT".to_string(),
            "SRC_AS_NAME".to_string(),
        ]
    );
}

#[test]
fn request_deserialization_defaults_missing_view_group_by_sort_by_and_top_n() {
    let request = serde_json::from_str::<FlowsRequest>(r#"{}"#)
        .expect("missing selectors should fall back to request defaults");

    assert_eq!(request.view, super::ViewMode::TableSankey);
    assert_eq!(
        request.group_by,
        vec![
            "SRC_AS_NAME".to_string(),
            "PROTOCOL".to_string(),
            "DST_AS_NAME".to_string()
        ]
    );
    assert_eq!(request.sort_by, SortBy::Bytes);
    assert_eq!(request.top_n, super::TopN::N25);

    let invalid_view = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"bogus","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"25"}"#,
    )
    .expect_err("invalid view should fail");
    assert!(
        invalid_view.to_string().contains("unknown variant `bogus`"),
        "unexpected error: {invalid_view}"
    );

    let invalid_group_by = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","group_by":["BYTES"],"sort_by":"bytes","top_n":"25"}"#,
    )
    .expect_err("metric fields should not be groupable");
    assert!(
        invalid_group_by
            .to_string()
            .contains("unsupported group_by field `BYTES`"),
        "unexpected error: {invalid_group_by}"
    );

    let removed_timestamp_group_by = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","group_by":["FLOW_END_USEC"],"sort_by":"bytes","top_n":"25"}"#,
    )
    .expect_err("removed internal timestamp fields should not be groupable");
    assert!(
        removed_timestamp_group_by
            .to_string()
            .contains("unsupported group_by field `FLOW_END_USEC`"),
        "unexpected error: {removed_timestamp_group_by}"
    );

    let invalid_top_n = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"42"}"#,
    )
    .expect_err("invalid top_n should fail");
    assert!(
        invalid_top_n.to_string().contains("unsupported top_n `42`"),
        "unexpected error: {invalid_top_n}"
    );
}

#[test]
fn request_deserialization_accepts_numeric_top_n_values() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":100}"#,
    )
    .expect("numeric top_n should match documented JSON examples");

    assert_eq!(request.top_n, super::TopN::N100);

    let invalid_top_n = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":42}"#,
    )
    .expect_err("unsupported numeric top_n should fail");
    assert!(
        invalid_top_n.to_string().contains("unsupported top_n `42`"),
        "unexpected error: {invalid_top_n}"
    );
}

#[test]
fn request_deserialization_accepts_relative_time_bounds() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"table-sankey","after":-3600,"before":0,"group_by":["PROTOCOL"],"sort_by":"bytes","top_n":100}"#,
    )
    .expect("documented relative time bounds should parse");

    assert_eq!(request.after, Some(-3600));
    assert_eq!(request.before, Some(0));
    assert_eq!(request.top_n, super::TopN::N100);
}

#[test]
fn resolve_time_bounds_treats_negative_after_as_relative_to_before() {
    let request = FlowsRequest {
        after: Some(-300),
        before: Some(1_000),
        ..Default::default()
    };

    assert_eq!(super::resolve_time_bounds(&request), (700, 1_000));
}

#[test]
fn resolve_time_bounds_preserves_order_after_clamping() {
    let request = FlowsRequest {
        after: Some(i64::MAX - 10),
        before: Some(i64::MAX),
        ..Default::default()
    };

    assert_eq!(
        super::resolve_time_bounds(&request),
        (u32::MAX - 1, u32::MAX)
    );
}

#[test]
fn request_deserialization_accepts_state_and_city_map_views() {
    let state = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"state-map","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"25"}"#,
    )
    .expect("state-map should deserialize");
    assert_eq!(state.view, super::ViewMode::StateMap);

    let city = serde_json::from_str::<FlowsRequest>(
        r#"{"view":"city-map","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"25"}"#,
    )
    .expect("city-map should deserialize");
    assert_eq!(city.view, super::ViewMode::CityMap);
}

#[test]
fn request_deserialization_accepts_scalar_and_array_selections() {
    let scalar = serde_json::from_str::<FlowsRequest>(
        r#"{
                "view":"table-sankey",
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25",
                "selections":{"FLOW_VERSION":"v5","DST_PORT":["443","8443"]}
            }"#,
    )
    .expect("scalar and array selections should deserialize");

    assert_eq!(
        scalar.selections.get("FLOW_VERSION"),
        Some(&vec!["v5".to_string()])
    );
    assert_eq!(
        scalar.selections.get("DST_PORT"),
        Some(&vec!["443".to_string(), "8443".to_string()])
    );
}

#[test]
fn request_deserialization_rejects_removed_internal_timestamp_selection_field() {
    let error = serde_json::from_str::<FlowsRequest>(
        r#"{
                "view":"table-sankey",
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25",
                "selections":{"FLOW_END_USEC":"90000000"}
            }"#,
    )
    .expect_err("removed internal timestamp field should not be selectable");

    assert!(
        error
            .to_string()
            .contains("unsupported selection field `FLOW_END_USEC`"),
        "unexpected error: {error}"
    );
}

#[test]
fn request_deserialization_accepts_object_based_selections() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{
                "view":"table-sankey",
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25",
                "selections":{
                    "FLOW_VERSION":{"id":"v5","name":"NetFlow v5"},
                    "PROTOCOL":[{"id":"6"},{"value":"17"}]
                }
            }"#,
    )
    .expect("object selections should deserialize");

    assert_eq!(
        request.selections.get("FLOW_VERSION"),
        Some(&vec!["v5".to_string()])
    );
    assert_eq!(
        request.selections.get("PROTOCOL"),
        Some(&vec!["6".to_string(), "17".to_string()])
    );
}

#[test]
fn request_deserialization_hoists_required_controls_from_selections() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{
                "after":1773734839,
                "before":1773735439,
                "query":"",
                "selections":{
                    "view":"table-sankey",
                    "group_by":["PROTOCOL","SRC_AS_NAME","DST_AS_NAME"],
                    "sort_by":"bytes",
                    "top_n":"25",
                    "IN_IF":["30","35"]
                },
                "timeout":120000,
                "last":200
            }"#,
    )
    .expect("request should deserialize");

    assert_eq!(request.view, super::ViewMode::TableSankey);
    assert_eq!(
        request.group_by,
        vec![
            "PROTOCOL".to_string(),
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string()
        ]
    );
    assert_eq!(request.sort_by, SortBy::Bytes);
    assert_eq!(request.top_n, super::TopN::N25);
    assert_eq!(
        request.selections,
        HashMap::from([(
            "IN_IF".to_string(),
            vec!["30".to_string(), "35".to_string()]
        )])
    );
}

#[test]
fn request_deserialization_prefers_top_level_controls_over_selections() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{
                "view":"timeseries",
                "group_by":["PROTOCOL"],
                "sort_by":"packets",
                "top_n":"50",
                "selections":{
                    "view":"table-sankey",
                    "group_by":["SRC_AS_NAME","DST_AS_NAME"],
                    "sort_by":"bytes",
                    "top_n":"25",
                    "IN_IF":["30","35"]
                }
            }"#,
    )
    .expect("request should deserialize");

    assert_eq!(request.view, super::ViewMode::TimeSeries);
    assert_eq!(request.group_by, vec!["PROTOCOL".to_string()]);
    assert_eq!(request.sort_by, SortBy::Packets);
    assert_eq!(request.top_n, super::TopN::N50);
    assert_eq!(
        request.selections,
        HashMap::from([(
            "IN_IF".to_string(),
            vec!["30".to_string(), "35".to_string()]
        )])
    );
}

#[test]
fn request_deserialization_supports_autocomplete_mode() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{
            "mode":"autocomplete",
            "field":"src_addr",
            "term":"10.0."
        }"#,
    )
    .expect("autocomplete request should deserialize");

    assert!(request.is_autocomplete_mode());
    assert_eq!(
        request.normalized_autocomplete_field().as_deref(),
        Some("SRC_ADDR")
    );
    assert_eq!(request.normalized_autocomplete_term(), "10.0.");
}

#[test]
fn request_deserialization_rejects_oversized_autocomplete_term() {
    let term = "x".repeat(257);
    let payload = format!(r#"{{"mode":"autocomplete","field":"src_as_name","term":"{term}"}}"#);
    let error = serde_json::from_str::<FlowsRequest>(&payload)
        .expect_err("oversized autocomplete term should be rejected");
    assert!(
        error.to_string().contains("autocomplete `term` exceeds"),
        "unexpected error: {error}"
    );
}

#[test]
fn request_deserialization_accepts_long_term_for_non_autocomplete_mode() {
    // The 256-byte cap is autocomplete-only. Regular flows/timeseries requests
    // may carry an ignored `term` of any length and must not be rejected.
    let term = "x".repeat(1024);
    let payload = format!(r#"{{"mode":"flows","view":"table-sankey","term":"{term}"}}"#);
    let request = serde_json::from_str::<FlowsRequest>(&payload)
        .expect("non-autocomplete request with long term must deserialize");
    assert!(!request.is_autocomplete_mode());
    assert_eq!(request.normalized_autocomplete_term().len(), 1024);
}

#[test]
fn request_deserialization_rejects_removed_internal_timestamp_autocomplete_field() {
    let error = serde_json::from_str::<FlowsRequest>(
        r#"{
            "mode":"autocomplete",
            "field":"flow_end_usec",
            "term":"9"
        }"#,
    )
    .expect_err("removed internal timestamp field should not be autocompletable");

    assert!(
        error
            .to_string()
            .contains("unsupported autocomplete field `FLOW_END_USEC`"),
        "unexpected error: {error}"
    );
}

#[test]
fn request_deserialization_accepts_and_normalizes_requested_facets() {
    let request = serde_json::from_str::<FlowsRequest>(
        r#"{
                "view":"table-sankey",
                "facets":["protocol","src_as_name","protocol"],
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25"
            }"#,
    )
    .expect("request should deserialize");

    assert_eq!(
        request.normalized_facets(),
        Some(vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()])
    );
}

#[test]
fn cursor_prefilter_includes_multi_value_selections_as_or_matches() {
    let selections = HashMap::from([
        ("FLOW_VERSION".to_string(), vec!["v5".to_string()]),
        (
            "PROTOCOL".to_string(),
            vec!["6".to_string(), "17".to_string()],
        ),
        ("DST_PORT".to_string(), vec!["".to_string()]),
    ]);

    let pairs = cursor_prefilter_pairs(&selections);

    assert_eq!(
        pairs,
        vec![
            ("FLOW_VERSION".to_string(), "v5".to_string()),
            ("PROTOCOL".to_string(), "17".to_string()),
            ("PROTOCOL".to_string(), "6".to_string()),
        ]
    );
}

#[test]
fn cursor_prefilter_skips_virtual_flow_fields() {
    let selections = HashMap::from([
        ("ICMPV4".to_string(), vec!["Echo Request".to_string()]),
        ("PROTOCOL".to_string(), vec!["1".to_string()]),
    ]);

    let pairs = cursor_prefilter_pairs(&selections);

    assert_eq!(pairs, vec![("PROTOCOL".to_string(), "1".to_string())]);
}

#[test]
fn cursor_prefilter_includes_stored_as_name_values() {
    let selections = HashMap::from([
        (
            "SRC_AS_NAME".to_string(),
            vec!["AS0 Private IP Address Space".to_string()],
        ),
        (
            "DST_AS_NAME".to_string(),
            vec!["AS0 Unknown ASN".to_string()],
        ),
        ("PROTOCOL".to_string(), vec!["6".to_string()]),
    ]);

    let pairs = cursor_prefilter_pairs(&selections);

    assert_eq!(
        pairs,
        vec![
            ("DST_AS_NAME".to_string(), "AS0 Unknown ASN".to_string()),
            ("PROTOCOL".to_string(), "6".to_string()),
            (
                "SRC_AS_NAME".to_string(),
                "AS0 Private IP Address Space".to_string(),
            ),
        ]
    );
}

#[test]
fn virtual_facet_dependencies_expand_to_stored_fields() {
    let mut fields = HashSet::from([
        "ICMPV4".to_string(),
        "ICMPV6".to_string(),
        "SRC_AS_NAME".to_string(),
    ]);

    super::expand_virtual_flow_field_dependencies(&mut fields);

    assert!(fields.contains("ICMPV4"));
    assert!(fields.contains("ICMPV6"));
    assert!(fields.contains("SRC_AS_NAME"));
    assert!(fields.contains("PROTOCOL"));
    assert!(fields.contains("ICMPV4_TYPE"));
    assert!(fields.contains("ICMPV4_CODE"));
    assert!(fields.contains("ICMPV6_TYPE"));
    assert!(fields.contains("ICMPV6_CODE"));
}

#[test]
fn labels_for_group_uses_stored_as_name_values() {
    let record = super::QueryFlowRecord::new(
        42,
        BTreeMap::from([
            ("SRC_AS".to_string(), "0".to_string()),
            (
                "SRC_AS_NAME".to_string(),
                "AS0 Private IP Address Space".to_string(),
            ),
            ("DST_AS".to_string(), "0".to_string()),
            ("DST_AS_NAME".to_string(), "AS0 Unknown ASN".to_string()),
        ]),
    );

    let labels = super::labels_for_group(
        &record,
        &["SRC_AS_NAME".to_string(), "DST_AS_NAME".to_string()],
    );

    assert_eq!(
        labels.get("SRC_AS_NAME").map(String::as_str),
        Some("AS0 Private IP Address Space")
    );
    assert_eq!(
        labels.get("DST_AS_NAME").map(String::as_str),
        Some("AS0 Unknown ASN")
    );
}

#[test]
fn captured_facet_field_value_uses_stored_as_name_values() {
    let capture_positions = super::FastHashMap::from([
        ("SRC_AS_NAME".to_string(), 0usize),
        ("DST_AS_NAME".to_string(), 1usize),
    ]);
    let captured_values = vec![
        Some("AS0 Private IP Address Space".to_string()),
        Some("AS0 Unknown ASN".to_string()),
    ];

    assert_eq!(
        super::captured_facet_field_value("SRC_AS_NAME", &capture_positions, &captured_values)
            .as_deref(),
        Some("AS0 Private IP Address Space")
    );
    assert_eq!(
        super::captured_facet_field_value("DST_AS_NAME", &capture_positions, &captured_values)
            .as_deref(),
        Some("AS0 Unknown ASN")
    );
}

#[test]
fn grouped_projected_scan_falls_back_for_virtual_fields() {
    let request = super::FlowsRequest {
        group_by: vec!["ICMPV4".to_string()],
        ..super::FlowsRequest::default()
    };
    assert!(!grouped_query_can_use_projected_scan(&request));

    let request = super::FlowsRequest {
        selections: HashMap::from([("ICMPV4".to_string(), vec!["Echo Request".to_string()])]),
        ..super::FlowsRequest::default()
    };
    assert!(!grouped_query_can_use_projected_scan(&request));
}

#[test]
fn query_record_populates_virtual_icmp_fields() {
    let record = super::QueryFlowRecord::new(
        42,
        BTreeMap::from([
            ("PROTOCOL".to_string(), "1".to_string()),
            ("ICMPV4_TYPE".to_string(), "8".to_string()),
            ("ICMPV4_CODE".to_string(), "0".to_string()),
        ]),
    );

    assert_eq!(
        record.fields.get("ICMPV4").map(String::as_str),
        Some("Echo Request")
    );
    assert!(!record.fields.contains_key("ICMPV6"));

    let record = super::QueryFlowRecord::new(
        42,
        BTreeMap::from([
            ("PROTOCOL".to_string(), "58".to_string()),
            ("ICMPV6_TYPE".to_string(), "160".to_string()),
            ("ICMPV6_CODE".to_string(), "1".to_string()),
        ]),
    );

    assert_eq!(
        record.fields.get("ICMPV6").map(String::as_str),
        Some("160/1")
    );
}

#[test]
fn facet_vocabularies_ignore_active_filters_and_do_not_return_metrics() {
    let requested_fields = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
    let selections = HashMap::from([
        ("PROTOCOL".to_string(), vec!["6".to_string()]),
        (
            "SRC_AS_NAME".to_string(),
            vec!["AS15169 GOOGLE".to_string()],
        ),
    ]);
    let facets = super::build_facet_vocabulary_payload(
        &requested_fields,
        &selections,
        &BTreeMap::from([
            (
                "PROTOCOL".to_string(),
                crate::facet_runtime::FacetPublishedField {
                    total_values: 2,
                    autocomplete: false,
                    values: vec!["6".to_string(), "17".to_string()],
                },
            ),
            (
                "SRC_AS_NAME".to_string(),
                crate::facet_runtime::FacetPublishedField {
                    total_values: 2,
                    autocomplete: false,
                    values: vec!["AS15169 GOOGLE".to_string(), "AS4333 NETDATA".to_string()],
                },
            ),
        ]),
    );

    let fields = facets["fields"].as_array().expect("fields array");
    let protocol = fields
        .iter()
        .find(|entry| entry["field"] == "PROTOCOL")
        .expect("protocol facet");
    let src_as = fields
        .iter()
        .find(|entry| entry["field"] == "SRC_AS_NAME")
        .expect("src facet");

    assert_eq!(
        protocol["values"]
            .as_array()
            .expect("protocol values")
            .iter()
            .map(|entry| entry["value"].as_str().unwrap_or_default())
            .collect::<Vec<_>>(),
        vec!["6", "17"]
    );
    assert_eq!(
        src_as["values"]
            .as_array()
            .expect("src values")
            .iter()
            .map(|entry| entry["value"].as_str().unwrap_or_default())
            .collect::<Vec<_>>(),
        vec!["AS15169 GOOGLE", "AS4333 NETDATA"]
    );
    assert!(
        protocol["values"]
            .as_array()
            .expect("protocol values")
            .iter()
            .all(|entry| entry.get("metrics").is_none())
    );
    assert!(
        facets.get("excluded_fields").is_none(),
        "facet payload should not advertise exposed raw-only fields as excluded"
    );
}

#[test]
fn captured_facet_helpers_resolve_virtual_values_and_ignore_self_selection() {
    let capture_positions = super::FastHashMap::from([
        ("PROTOCOL".to_string(), 0usize),
        ("ICMPV4_TYPE".to_string(), 1usize),
        ("ICMPV4_CODE".to_string(), 2usize),
        ("SRC_AS_NAME".to_string(), 3usize),
    ]);
    let captured_values = vec![
        Some("1".to_string()),
        Some("3".to_string()),
        Some("1".to_string()),
        Some("AS4333 NETDATA".to_string()),
    ];

    let value = super::captured_facet_field_value("ICMPV4", &capture_positions, &captured_values)
        .expect("virtual icmpv4 value");
    assert_eq!(value.as_ref(), "Host Unreachable");

    let selections = HashMap::from([
        ("ICMPV4".to_string(), vec!["Echo Request".to_string()]),
        (
            "SRC_AS_NAME".to_string(),
            vec!["AS4333 NETDATA".to_string()],
        ),
    ]);
    assert!(super::captured_facet_matches_selections_except(
        Some("ICMPV4"),
        &selections,
        &capture_positions,
        &captured_values,
    ));
    assert!(!super::captured_facet_matches_selections_except(
        Some("SRC_AS_NAME"),
        &HashMap::from([("ICMPV4".to_string(), vec!["Echo Request".to_string()])]),
        &capture_positions,
        &captured_values,
    ));
}

#[test]
fn open_tier_facet_vocabularies_are_non_contextual_for_stored_as_name_values() {
    let mut store = crate::tiering::TierFlowIndexStore::default();
    let timestamp_usec = 120_000_000;

    let mut first = crate::decoder::FlowRecord::default();
    first.protocol = 6;
    first.src_as_name = "AS15169 GOOGLE".to_string();
    first.bytes = 100;
    first.packets = 1;

    let mut second = crate::decoder::FlowRecord::default();
    second.protocol = 17;
    second.src_as_name = "AS15169 GOOGLE".to_string();
    second.bytes = 200;
    second.packets = 2;

    let mut third = crate::decoder::FlowRecord::default();
    third.protocol = 6;
    third.src_as = 0;
    third.src_as_name = "AS0 Unknown ASN".to_string();
    third.bytes = 300;
    third.packets = 3;

    let rows = vec![
        crate::tiering::OpenTierRow {
            timestamp_usec,
            flow_ref: store
                .get_or_insert_record_flow(timestamp_usec, &first)
                .expect("intern first open row"),
            metrics: crate::tiering::FlowMetrics::from_record(&first),
        },
        crate::tiering::OpenTierRow {
            timestamp_usec: timestamp_usec + 1,
            flow_ref: store
                .get_or_insert_record_flow(timestamp_usec + 1, &second)
                .expect("intern second open row"),
            metrics: crate::tiering::FlowMetrics::from_record(&second),
        },
        crate::tiering::OpenTierRow {
            timestamp_usec: timestamp_usec + 2,
            flow_ref: store
                .get_or_insert_record_flow(timestamp_usec + 2, &third)
                .expect("intern third open row"),
            metrics: crate::tiering::FlowMetrics::from_record(&third),
        },
    ];

    let requested_fields = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
    let requested_set = requested_fields.iter().cloned().collect::<HashSet<_>>();
    let selections = HashMap::from([
        ("PROTOCOL".to_string(), vec!["6".to_string()]),
        (
            "SRC_AS_NAME".to_string(),
            vec!["AS15169 GOOGLE".to_string()],
        ),
    ]);
    let mut by_field = BTreeMap::new();

    super::accumulate_open_tier_facet_vocabulary(&rows, &store, &requested_fields, &mut by_field);

    let facets = super::build_facet_vocabulary_payload(
        &requested_fields,
        &selections,
        &super::finalize_facet_vocabulary(by_field, &requested_set)
            .into_iter()
            .map(|(field, values)| {
                let total_values = values.len();
                (
                    field,
                    crate::facet_runtime::FacetPublishedField {
                        total_values,
                        autocomplete: false,
                        values,
                    },
                )
            })
            .collect(),
    );

    let fields = facets["fields"].as_array().expect("fields array");
    let protocol = fields
        .iter()
        .find(|entry| entry["field"] == "PROTOCOL")
        .expect("protocol facet");
    let src_as = fields
        .iter()
        .find(|entry| entry["field"] == "SRC_AS_NAME")
        .expect("src facet");

    assert_eq!(
        protocol["values"]
            .as_array()
            .expect("protocol values")
            .iter()
            .map(|entry| entry["value"].as_str().unwrap_or_default())
            .collect::<Vec<_>>(),
        vec!["6", "17"]
    );
    let mut src_values = src_as["values"]
        .as_array()
        .expect("src values")
        .iter()
        .map(|entry| entry["value"].as_str().unwrap_or_default())
        .collect::<Vec<_>>();
    src_values.sort_unstable();
    assert_eq!(src_values, vec!["AS0 Unknown ASN", "AS15169 GOOGLE"]);
}

#[test]
fn open_tier_timeseries_helpers_match_materialized_record_path() {
    let mut store = crate::tiering::TierFlowIndexStore::default();
    let group_by = vec!["PROTOCOL".to_string()];

    let mut first = crate::decoder::FlowRecord::default();
    first.protocol = 6;
    first.bytes = 10;
    first.packets = 1;
    first.set_sampling_rate(100);

    let mut second = crate::decoder::FlowRecord::default();
    second.protocol = 17;
    second.bytes = 20;
    second.packets = 2;
    second.set_sampling_rate(1);

    let rows = vec![
        crate::tiering::OpenTierRow {
            timestamp_usec: 1_000_000,
            flow_ref: store
                .get_or_insert_record_flow(1_000_000, &first)
                .expect("intern first flow"),
            metrics: crate::tiering::FlowMetrics::from_record(&first),
        },
        crate::tiering::OpenTierRow {
            timestamp_usec: 2_000_000,
            flow_ref: store
                .get_or_insert_record_flow(2_000_000, &second)
                .expect("intern second flow"),
            metrics: crate::tiering::FlowMetrics::from_record(&second),
        },
    ];

    let records = rows
        .iter()
        .map(|row| {
            let mut fields = store
                .materialize_fields(row.flow_ref)
                .expect("materialize rollup fields");
            row.metrics.write_fields(&mut fields);
            super::QueryFlowRecord {
                timestamp_usec: row.timestamp_usec,
                fields: fields
                    .into_iter()
                    .map(|(key, value)| (key.to_string(), value))
                    .collect(),
            }
        })
        .collect::<Vec<_>>();

    let mut baseline_aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
    let mut baseline_overflow = super::GroupOverflow::default();
    for record in &records {
        super::accumulate_grouped_record(
            record,
            super::sampled_metrics_from_fields(&record.fields),
            &group_by,
            &mut baseline_aggregates,
            &mut baseline_overflow,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }

    let mut projected_aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
    let mut projected_overflow = super::GroupOverflow::default();
    for row in &rows {
        super::accumulate_open_tier_timeseries_grouped_record(
            row,
            &store,
            &group_by,
            &mut projected_aggregates,
            &mut projected_overflow,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }

    let baseline_ranked = super::rank_aggregates(
        baseline_aggregates,
        baseline_overflow.aggregate,
        SortBy::Bytes,
        10,
    );
    let projected_ranked = super::rank_aggregates(
        projected_aggregates,
        projected_overflow.aggregate,
        SortBy::Bytes,
        10,
    );

    assert_eq!(baseline_ranked.rows.len(), projected_ranked.rows.len());
    assert_eq!(
        baseline_ranked.rows[0].labels,
        projected_ranked.rows[0].labels
    );
    assert_eq!(
        baseline_ranked.rows[0].metrics.bytes,
        projected_ranked.rows[0].metrics.bytes
    );
    assert_eq!(
        baseline_ranked.rows[0].metrics.packets,
        projected_ranked.rows[0].metrics.packets
    );
    assert_eq!(
        baseline_ranked.rows[1].labels,
        projected_ranked.rows[1].labels
    );
    assert_eq!(
        baseline_ranked.rows[1].metrics.bytes,
        projected_ranked.rows[1].metrics.bytes
    );
    assert_eq!(
        baseline_ranked.rows[1].metrics.packets,
        projected_ranked.rows[1].metrics.packets
    );

    let layout = super::init_timeseries_layout(0, 60);
    let baseline_top_keys: HashMap<super::GroupKey, usize> = baseline_ranked
        .rows
        .iter()
        .enumerate()
        .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
        .collect();
    let projected_top_keys: HashMap<super::GroupKey, usize> = projected_ranked
        .rows
        .iter()
        .enumerate()
        .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
        .collect();

    let mut baseline_buckets = vec![vec![0_u64; baseline_ranked.rows.len()]; layout.bucket_count];
    for record in &records {
        let key = super::group_key_from_labels(&super::labels_for_group(record, &group_by));
        if let Some(index) = baseline_top_keys.get(&key).copied() {
            super::accumulate_series_bucket(
                &mut baseline_buckets,
                record.timestamp_usec,
                layout.after,
                layout.before,
                layout.bucket_seconds,
                index,
                super::sampled_metric_value(SortBy::Bytes, &record.fields),
            );
        }
    }

    let mut projected_buckets = vec![vec![0_u64; projected_ranked.rows.len()]; layout.bucket_count];
    for row in &rows {
        let key =
            super::group_key_from_labels(&super::open_tier_row_labels(row, &store, &group_by));
        if let Some(index) = projected_top_keys.get(&key).copied() {
            super::accumulate_series_bucket(
                &mut projected_buckets,
                row.timestamp_usec,
                layout.after,
                layout.before,
                layout.bucket_seconds,
                index,
                super::sampled_metric_value_from_open_tier_row(SortBy::Bytes, row, &store),
            );
        }
    }

    assert_eq!(baseline_buckets, projected_buckets);
}

#[test]
fn grouped_accumulator_routes_new_groups_to_overflow_after_cap() {
    let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
    for idx in 0..super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS {
        aggregates.insert(
            super::GroupKey(vec![("PROTOCOL".to_string(), idx.to_string())]),
            super::AggregatedFlow::default(),
        );
    }

    let record = super::QueryFlowRecord {
        timestamp_usec: 100,
        fields: BTreeMap::from([
            ("PROTOCOL".to_string(), "overflow-key".to_string()),
            ("BYTES".to_string(), "123".to_string()),
            ("PACKETS".to_string(), "1".to_string()),
        ]),
    };
    let metrics = metrics_from_fields(&record.fields);
    let mut overflow = super::GroupOverflow::default();
    super::accumulate_grouped_record(
        &record,
        metrics,
        &["PROTOCOL".to_string()],
        &mut aggregates,
        &mut overflow,
        super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
    );

    assert_eq!(
        aggregates.len(),
        super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS
    );
    assert_eq!(overflow.dropped_records, 1);
    let overflow_row = overflow.aggregate.expect("overflow row");
    assert_eq!(
        overflow_row.labels.get("_bucket"),
        Some(&"__overflow__".to_string())
    );
    assert_eq!(overflow_row.metrics.bytes, 123);
}

#[test]
fn grouped_overflow_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
    let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
    let records = vec![
        super::QueryFlowRecord {
            timestamp_usec: 1,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Alpha".to_string()),
                ("BYTES".to_string(), "100".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 2,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Beta".to_string()),
                ("BYTES".to_string(), "90".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 3,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), "Gamma".to_string()),
                ("BYTES".to_string(), "80".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        },
    ];

    let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
    let mut overflow = super::GroupOverflow::default();
    for record in &records {
        super::accumulate_grouped_record(
            record,
            metrics_from_fields(&record.fields),
            &group_by,
            &mut aggregates,
            &mut overflow,
            1,
        );
    }

    let flow = super::flow_value_from_aggregate(overflow.aggregate.expect("overflow row"));
    assert_eq!(flow["key"]["_bucket"], "__overflow__");
    assert_eq!(flow["key"]["PROTOCOL"], "6");
    assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
}

#[test]
fn compact_grouped_accumulator_routes_new_groups_to_overflow_after_cap() {
    let group_by = vec!["PROTOCOL".to_string()];
    let mut aggregates =
        super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

    for idx in 0..super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS {
        let record = super::QueryFlowRecord {
            timestamp_usec: idx as u64 + 1,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), idx.to_string()),
                ("BYTES".to_string(), "1".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        super::accumulate_compact_grouped_record(
            &record,
            super::RecordHandle::JournalRealtime {
                tier: super::TierKind::Raw,
                timestamp_usec: record.timestamp_usec,
            },
            metrics_from_fields(&record.fields),
            &group_by,
            &mut aggregates,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        )
        .expect("accumulate compact record");
    }

    let overflow_record = super::QueryFlowRecord {
        timestamp_usec: 999_999,
        fields: BTreeMap::from([
            ("PROTOCOL".to_string(), "overflow-key".to_string()),
            ("BYTES".to_string(), "123".to_string()),
            ("PACKETS".to_string(), "1".to_string()),
        ]),
    };
    super::accumulate_compact_grouped_record(
        &overflow_record,
        super::RecordHandle::JournalRealtime {
            tier: super::TierKind::Raw,
            timestamp_usec: overflow_record.timestamp_usec,
        },
        metrics_from_fields(&overflow_record.fields),
        &group_by,
        &mut aggregates,
        super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
    )
    .expect("accumulate compact overflow record");

    assert_eq!(
        aggregates.grouped_total(),
        super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS
    );
    assert_eq!(aggregates.overflow.dropped_records, 1);
    let overflow = aggregates
        .overflow
        .aggregate
        .expect("compact overflow aggregate");
    assert_eq!(overflow.metrics.bytes, 123);
}

#[test]
fn compact_other_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
    let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
    let mut aggregates =
        super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

    for (idx, src_as, bytes) in [
        (1_u64, "Alpha", "300"),
        (2, "Beta", "200"),
        (3, "Gamma", "100"),
    ] {
        let record = super::QueryFlowRecord {
            timestamp_usec: idx,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), src_as.to_string()),
                ("BYTES".to_string(), bytes.to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        super::accumulate_compact_grouped_record(
            &record,
            super::RecordHandle::JournalRealtime {
                tier: super::TierKind::Raw,
                timestamp_usec: record.timestamp_usec,
            },
            metrics_from_fields(&record.fields),
            &group_by,
            &mut aggregates,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        )
        .expect("accumulate compact record");
    }

    let super::CompactGroupAccumulator {
        index,
        rows,
        overflow,
        ..
    } = aggregates;
    let ranked = super::rank_compact_aggregates(
        rows,
        overflow.aggregate,
        SortBy::Bytes,
        1,
        &group_by,
        &index,
    )
    .expect("rank compact rows");
    let other = ranked.other.expect("other bucket");
    let flow = super::flow_value_from_aggregate(
        super::synthetic_aggregate_from_compact(other).expect("materialize compact other"),
    );
    assert_eq!(flow["key"]["_bucket"], "__other__");
    assert_eq!(flow["key"]["PROTOCOL"], "6");
    assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
}

#[test]
fn compact_overflow_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
    let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
    let mut aggregates =
        super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

    for (idx, src_as, bytes) in [
        (1_u64, "Alpha", "100"),
        (2, "Beta", "90"),
        (3, "Gamma", "80"),
    ] {
        let record = super::QueryFlowRecord {
            timestamp_usec: idx,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("SRC_AS_NAME".to_string(), src_as.to_string()),
                ("BYTES".to_string(), bytes.to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        super::accumulate_compact_grouped_record(
            &record,
            super::RecordHandle::JournalRealtime {
                tier: super::TierKind::Raw,
                timestamp_usec: record.timestamp_usec,
            },
            metrics_from_fields(&record.fields),
            &group_by,
            &mut aggregates,
            1,
        )
        .expect("accumulate compact record");
    }

    let overflow = aggregates
        .overflow
        .aggregate
        .expect("compact overflow aggregate");
    let flow = super::flow_value_from_aggregate(
        super::synthetic_aggregate_from_compact(overflow).expect("materialize compact overflow"),
    );
    assert_eq!(flow["key"]["_bucket"], "__overflow__");
    assert_eq!(flow["key"]["PROTOCOL"], "6");
    assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
}

#[test]
fn facet_accumulator_reports_overflow_when_value_cap_is_reached() {
    let mut field_acc = super::FacetFieldAccumulator::default();
    for idx in 0..super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD {
        field_acc
            .values
            .insert(idx.to_string(), super::QueryFlowMetrics::default());
    }
    let mut by_field: BTreeMap<String, super::FacetFieldAccumulator> =
        BTreeMap::from([("PROTOCOL".to_string(), field_acc)]);

    let record = super::QueryFlowRecord {
        timestamp_usec: 100,
        fields: BTreeMap::from([
            ("PROTOCOL".to_string(), "new".to_string()),
            ("BYTES".to_string(), "9".to_string()),
            ("PACKETS".to_string(), "1".to_string()),
        ]),
    };
    let metrics = metrics_from_fields(&record.fields);
    super::accumulate_facet_record(
        &record,
        metrics,
        &mut by_field,
        super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    );

    let field = by_field.get("PROTOCOL").expect("field");
    assert_eq!(
        field.values.len(),
        super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD
    );
    assert_eq!(field.overflow_records, 1);

    let facets = super::build_facets_from_accumulator(
        by_field,
        SortBy::Bytes,
        &["PROTOCOL".to_string()],
        &HashMap::new(),
        super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    );
    assert_eq!(facets["overflowed_fields"], 1);
    assert_eq!(facets["overflowed_records"], 1);
    let fields = facets["fields"].as_array().expect("fields array");
    let protocol = fields
        .iter()
        .find(|entry| entry["field"] == "PROTOCOL")
        .expect("PROTOCOL facet");
    assert_eq!(protocol["overflowed"], true);
    assert_eq!(protocol["overflow_records"], 1);
}

#[test]
fn metrics_chart_uses_only_discovered_top_n_groups() {
    let records = vec![
        super::QueryFlowRecord {
            timestamp_usec: 1_000_000,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("BYTES".to_string(), "100".to_string()),
                ("PACKETS".to_string(), "10".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 2_000_000,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "6".to_string()),
                ("BYTES".to_string(), "50".to_string()),
                ("PACKETS".to_string(), "5".to_string()),
            ]),
        },
        super::QueryFlowRecord {
            timestamp_usec: 3_000_000,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "17".to_string()),
                ("BYTES".to_string(), "40".to_string()),
                ("PACKETS".to_string(), "4".to_string()),
            ]),
        },
    ];

    let group_by = vec!["PROTOCOL".to_string()];
    let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
    let mut overflow = super::GroupOverflow::default();
    for record in &records {
        super::accumulate_grouped_record(
            record,
            super::sampled_metrics_from_fields(&record.fields),
            &group_by,
            &mut aggregates,
            &mut overflow,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }

    let ranked = super::rank_aggregates(aggregates, overflow.aggregate, SortBy::Bytes, 1);
    assert_eq!(ranked.rows.len(), 1);
    assert_eq!(
        ranked.rows[0].labels.get("PROTOCOL"),
        Some(&"6".to_string())
    );

    let layout = super::init_timeseries_layout(0, 60);
    let mut series_buckets = vec![vec![0_u64; ranked.rows.len()]; layout.bucket_count];
    let top_keys: HashMap<super::GroupKey, usize> = ranked
        .rows
        .iter()
        .enumerate()
        .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
        .collect();

    for record in &records {
        let key = super::group_key_from_labels(&super::labels_for_group(record, &group_by));
        if let Some(index) = top_keys.get(&key).copied() {
            super::accumulate_series_bucket(
                &mut series_buckets,
                record.timestamp_usec,
                layout.after,
                layout.before,
                layout.bucket_seconds,
                index,
                super::sampled_metric_value(SortBy::Bytes, &record.fields),
            );
        }
    }

    let chart = super::metrics_chart_from_top_groups(
        layout.after,
        layout.before,
        layout.bucket_seconds,
        SortBy::Bytes,
        &ranked.rows,
        &series_buckets,
    );

    assert_eq!(chart["result"]["labels"][1], "Protocol=TCP");
    assert_eq!(chart["view"]["chart_type"], "stacked");
    assert_eq!(
        chart["view"]["dimensions"]["ids"]
            .as_array()
            .map(|v| v.len()),
        Some(1)
    );

    let total: f64 = chart["result"]["data"]
        .as_array()
        .expect("chart data")
        .iter()
        .map(|row| row[1][0].as_f64().unwrap_or(0.0))
        .sum();
    assert_eq!(total, 2.5);
}

fn synthetic_rollup_record(group: usize, timestamp_usec: u64) -> super::QueryFlowRecord {
    let protocol = if group % 2 == 0 { "6" } else { "17" };
    let src_as = 64_512 + (group % 4_096);
    let dst_as = 65_000 + (group % 4_096);
    let in_if = 10 + (group % 128);
    let out_if = 20 + (group % 128);
    let exporter_id = group % 32;
    let site = format!("site-{}", group % 8);
    let region = format!("region-{}", group % 4);
    let tenant = format!("tenant-{}", group % 16);
    let src_country = if group % 3 == 0 { "US" } else { "DE" };
    let dst_country = if group % 5 == 0 { "GB" } else { "FR" };

    super::QueryFlowRecord {
        timestamp_usec,
        fields: BTreeMap::from([
            (
                "DIRECTION".to_string(),
                if group % 2 == 0 { "ingress" } else { "egress" }.to_string(),
            ),
            ("PROTOCOL".to_string(), protocol.to_string()),
            ("ETYPE".to_string(), "2048".to_string()),
            ("FORWARDING_STATUS".to_string(), "0".to_string()),
            ("FLOW_VERSION".to_string(), "ipfix".to_string()),
            ("IPTOS".to_string(), "0".to_string()),
            (
                "TCP_FLAGS".to_string(),
                if protocol == "6" { "24" } else { "0" }.to_string(),
            ),
            ("ICMPV4_TYPE".to_string(), "0".to_string()),
            ("ICMPV4_CODE".to_string(), "0".to_string()),
            ("ICMPV6_TYPE".to_string(), "0".to_string()),
            ("ICMPV6_CODE".to_string(), "0".to_string()),
            ("SRC_AS".to_string(), src_as.to_string()),
            ("DST_AS".to_string(), dst_as.to_string()),
            ("SRC_AS_NAME".to_string(), format!("Source AS {}", src_as)),
            (
                "DST_AS_NAME".to_string(),
                format!("Destination AS {}", dst_as),
            ),
            (
                "EXPORTER_IP".to_string(),
                format!("192.0.2.{}", exporter_id + 1),
            ),
            ("EXPORTER_PORT".to_string(), "2055".to_string()),
            (
                "EXPORTER_NAME".to_string(),
                format!("edge-router-{}", exporter_id),
            ),
            (
                "EXPORTER_GROUP".to_string(),
                format!("group-{}", exporter_id % 4),
            ),
            ("EXPORTER_ROLE".to_string(), "edge".to_string()),
            ("EXPORTER_SITE".to_string(), site.clone()),
            ("EXPORTER_REGION".to_string(), region.clone()),
            ("EXPORTER_TENANT".to_string(), tenant.clone()),
            ("IN_IF".to_string(), in_if.to_string()),
            ("OUT_IF".to_string(), out_if.to_string()),
            ("IN_IF_NAME".to_string(), format!("xe-0/0/{}", in_if % 16)),
            ("OUT_IF_NAME".to_string(), format!("xe-0/1/{}", out_if % 16)),
            (
                "IN_IF_DESCRIPTION".to_string(),
                format!("uplink-{}", in_if % 8),
            ),
            (
                "OUT_IF_DESCRIPTION".to_string(),
                format!("peer-{}", out_if % 8),
            ),
            ("IN_IF_SPEED".to_string(), "10000000000".to_string()),
            ("OUT_IF_SPEED".to_string(), "10000000000".to_string()),
            ("IN_IF_PROVIDER".to_string(), "isp-a".to_string()),
            ("OUT_IF_PROVIDER".to_string(), "isp-b".to_string()),
            ("IN_IF_CONNECTIVITY".to_string(), "transit".to_string()),
            ("OUT_IF_CONNECTIVITY".to_string(), "transit".to_string()),
            ("IN_IF_BOUNDARY".to_string(), "1".to_string()),
            ("OUT_IF_BOUNDARY".to_string(), "1".to_string()),
            (
                "SRC_NET_NAME".to_string(),
                format!("src-net-{}", group % 64),
            ),
            (
                "DST_NET_NAME".to_string(),
                format!("dst-net-{}", group % 64),
            ),
            ("SRC_NET_ROLE".to_string(), "application".to_string()),
            ("DST_NET_ROLE".to_string(), "service".to_string()),
            ("SRC_NET_SITE".to_string(), site.clone()),
            ("DST_NET_SITE".to_string(), site),
            ("SRC_NET_REGION".to_string(), region.clone()),
            ("DST_NET_REGION".to_string(), region),
            ("SRC_NET_TENANT".to_string(), tenant.clone()),
            ("DST_NET_TENANT".to_string(), tenant),
            ("SRC_COUNTRY".to_string(), src_country.to_string()),
            ("DST_COUNTRY".to_string(), dst_country.to_string()),
            ("SRC_GEO_CITY".to_string(), "Athens".to_string()),
            ("DST_GEO_CITY".to_string(), "Paris".to_string()),
            ("SRC_GEO_STATE".to_string(), "Attica".to_string()),
            ("DST_GEO_STATE".to_string(), "Ile-de-France".to_string()),
            ("NEXT_HOP".to_string(), "198.51.100.1".to_string()),
            ("SRC_VLAN".to_string(), "100".to_string()),
            ("DST_VLAN".to_string(), "200".to_string()),
            ("SAMPLING_RATE".to_string(), "1".to_string()),
            (
                "BYTES".to_string(),
                (1_500 + (group % 4_000) as u64).to_string(),
            ),
            (
                "PACKETS".to_string(),
                (10 + (group % 200) as u64).to_string(),
            ),
            (
                "RAW_BYTES".to_string(),
                (1_500 + (group % 4_000) as u64).to_string(),
            ),
            (
                "RAW_PACKETS".to_string(),
                (10 + (group % 200) as u64).to_string(),
            ),
        ]),
    }
}

#[test]
#[ignore]
fn bench_long_window_accumulation_cost_centers() {
    let group_by = vec![
        "PROTOCOL".to_string(),
        "SRC_AS_NAME".to_string(),
        "DST_AS_NAME".to_string(),
    ];
    let record_count = 200_000usize;
    let group_count = 20_000usize;
    let records = (0..record_count)
        .map(|idx| synthetic_rollup_record(idx % group_count, idx as u64 * 1_000_000))
        .collect::<Vec<_>>();

    let start = Instant::now();
    let mut compact_only =
        super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");
    for record in &records {
        super::accumulate_compact_grouped_record(
            record,
            super::RecordHandle::JournalRealtime {
                tier: super::TierKind::Raw,
                timestamp_usec: record.timestamp_usec,
            },
            metrics_from_fields(&record.fields),
            &group_by,
            &mut compact_only,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        )
        .expect("compact accumulation");
    }
    let compact_elapsed = start.elapsed();

    let start = Instant::now();
    let mut compact_with_facets =
        super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");
    let mut facet_values: BTreeMap<String, super::FacetFieldAccumulator> = BTreeMap::new();
    for record in &records {
        super::accumulate_record(
            record,
            super::RecordHandle::JournalRealtime {
                tier: super::TierKind::Raw,
                timestamp_usec: record.timestamp_usec,
            },
            &group_by,
            &mut compact_with_facets,
            &mut facet_values,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        )
        .expect("table accumulation with facets");
    }
    let with_facets_elapsed = start.elapsed();

    let start = Instant::now();
    let facets_payload = super::build_facets_from_accumulator(
        facet_values,
        SortBy::Bytes,
        &group_by,
        &HashMap::new(),
        super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    );
    let facet_finalize_elapsed = start.elapsed();

    let compact_ms = compact_elapsed.as_secs_f64() * 1_000.0;
    let with_facets_ms = with_facets_elapsed.as_secs_f64() * 1_000.0;
    let finalize_ms = facet_finalize_elapsed.as_secs_f64() * 1_000.0;
    let ratio = if compact_ms > 0.0 {
        with_facets_ms / compact_ms
    } else {
        0.0
    };

    eprintln!();
    eprintln!("=== Long Window Query Cost Centers ===");
    eprintln!("records:                 {}", record_count);
    eprintln!("distinct groups:         {}", group_count);
    eprintln!("group-by:                {:?}", group_by);
    eprintln!("compact grouping only:   {:.2} ms", compact_ms);
    eprintln!("grouping + facets scan:  {:.2} ms", with_facets_ms);
    eprintln!("facet finalize only:     {:.2} ms", finalize_ms);
    eprintln!("facet scan ratio:        {:.2}x", ratio);
    eprintln!(
        "facet fields returned:   {}",
        facets_payload["fields"]
            .as_array()
            .map(|rows| rows.len())
            .unwrap_or(0)
    );
    eprintln!();
}

#[test]
fn query_metrics_ignore_sampling_rate() {
    let fields = BTreeMap::from([
        ("BYTES".to_string(), "10".to_string()),
        ("PACKETS".to_string(), "2".to_string()),
        ("SAMPLING_RATE".to_string(), "100".to_string()),
    ]);

    let metrics = super::sampled_metrics_from_fields(&fields);
    assert_eq!(metrics.bytes, 10);
    assert_eq!(metrics.packets, 2);
}

#[test]
fn query_metrics_ignore_zero_sampling_rate() {
    let fields = BTreeMap::from([
        ("BYTES".to_string(), "10".to_string()),
        ("PACKETS".to_string(), "2".to_string()),
        ("SAMPLING_RATE".to_string(), "0".to_string()),
    ]);

    let metrics = super::sampled_metrics_from_fields(&fields);
    assert_eq!(metrics.bytes, 10);
    assert_eq!(metrics.packets, 2);
}

#[test]
fn chart_timestamp_uses_journal_time_not_flow_end() {
    let record = super::QueryFlowRecord {
        timestamp_usec: 30_000_000,
        fields: BTreeMap::from([("FLOW_END_USEC".to_string(), "90_000_000".to_string())]),
    };

    assert_eq!(super::chart_timestamp_usec(&record), 30_000_000);
}

#[test]
fn init_timeseries_layout_has_one_minute_floor_and_wall_clock_alignment() {
    let layout = super::init_timeseries_layout(65, 305);
    assert_eq!(layout.bucket_seconds, 60);
    assert_eq!(layout.after, 60);
    assert_eq!(layout.before, 360);
    assert_eq!(layout.bucket_count, 5);
}

#[test]
fn init_timeseries_layout_rounds_up_to_whole_minutes() {
    let layout = super::init_timeseries_layout(61, 7_261);
    assert_eq!(layout.bucket_seconds, 60);
    assert_eq!(layout.after, 60);
    assert_eq!(layout.before, 7_320);
    assert_eq!(layout.bucket_count, 121);
}

#[test]
fn select_timeseries_source_tier_prefers_highest_resolution_with_at_least_100_points() {
    assert_eq!(
        super::select_timeseries_source_tier(0, 24 * 60 * 60, false),
        super::TierKind::Minute5
    );
    assert_eq!(
        super::select_timeseries_source_tier(0, 7 * 24 * 60 * 60, false),
        super::TierKind::Hour1
    );
    assert_eq!(
        super::select_timeseries_source_tier(0, 3 * 60 * 60, false),
        super::TierKind::Minute1
    );
}

#[test]
fn timeseries_span_plan_uses_source_tier_and_finer_edges_only() {
    let spans = super::plan_query_tier_spans(
        14 * 3600 + 12 * 60 + 30,
        17 * 3600 + 12 * 60 + 30,
        super::timeseries_candidate_tiers(super::TierKind::Minute5),
        false,
    );
    assert_eq!(
        spans,
        vec![
            super::QueryTierSpan {
                tier: super::TierKind::Raw,
                after: 14 * 3600 + 12 * 60 + 30,
                before: 14 * 3600 + 13 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute1,
                after: 14 * 3600 + 13 * 60,
                before: 14 * 3600 + 15 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute5,
                after: 14 * 3600 + 15 * 60,
                before: 17 * 3600 + 10 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute1,
                after: 17 * 3600 + 10 * 60,
                before: 17 * 3600 + 12 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Raw,
                after: 17 * 3600 + 12 * 60,
                before: 17 * 3600 + 12 * 60 + 30,
            },
        ]
    );
}

#[test]
fn init_timeseries_layout_uses_source_tier_as_bucket_floor() {
    let layout = super::init_timeseries_layout_for_tier(0, 24 * 60 * 60, super::TierKind::Minute5);
    assert_eq!(layout.bucket_seconds, 300);
    assert_eq!(layout.after, 0);
    assert_eq!(layout.before, 24 * 60 * 60);
    assert_eq!(layout.bucket_count, 288);
}

#[test]
fn plan_query_tier_spans_stitches_aligned_ranges_greedily() {
    let spans = super::plan_query_tier_spans(
        14 * 3600 + 12 * 60 + 30,
        17 * 3600 + 12 * 60 + 30,
        &[
            super::TierKind::Hour1,
            super::TierKind::Minute5,
            super::TierKind::Minute1,
        ],
        false,
    );
    assert_eq!(
        spans,
        vec![
            super::QueryTierSpan {
                tier: super::TierKind::Raw,
                after: 14 * 3600 + 12 * 60 + 30,
                before: 14 * 3600 + 13 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute1,
                after: 14 * 3600 + 13 * 60,
                before: 14 * 3600 + 15 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute5,
                after: 14 * 3600 + 15 * 60,
                before: 15 * 3600,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Hour1,
                after: 15 * 3600,
                before: 17 * 3600,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute5,
                after: 17 * 3600,
                before: 17 * 3600 + 10 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Minute1,
                after: 17 * 3600 + 10 * 60,
                before: 17 * 3600 + 12 * 60,
            },
            super::QueryTierSpan {
                tier: super::TierKind::Raw,
                after: 17 * 3600 + 12 * 60,
                before: 17 * 3600 + 12 * 60 + 30,
            },
        ]
    );
}
