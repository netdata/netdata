use super::*;
use crate::plugin_config::{
    AsnProviderConfig, GeoIpConfig, NetProviderConfig, NetworkAttributesConfig,
    NetworkAttributesValue, RoutingDynamicBmpConfig, RoutingDynamicConfig, StaticExporterConfig,
    StaticInterfaceConfig, StaticMetadataConfig, StaticRoutingConfig, StaticRoutingEntryConfig,
    StaticRoutingLargeCommunityConfig,
};
use std::io::Write;
use tempfile::tempdir;

#[test]
fn enricher_is_disabled_when_configuration_is_empty() {
    let cfg = EnrichmentConfig::default();
    let enricher = FlowEnricher::from_config(&cfg).expect("build enricher");
    assert!(enricher.is_none());
}

#[test]
fn static_sampling_override_uses_most_specific_prefix() {
    let cfg = EnrichmentConfig {
        default_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([(
            "192.0.2.0/24".to_string(),
            100_u64,
        )]))),
        override_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([
            ("192.0.2.0/24".to_string(), 500_u64),
            ("192.0.2.128/25".to_string(), 1000_u64),
        ]))),
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.142", 10, 20, 0, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("SAMPLING_RATE").map(String::as_str),
        Some("1000")
    );
}

#[test]
fn static_metadata_populates_exporter_and_interface_fields() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));

    assert_eq!(
        fields.get("EXPORTER_NAME").map(String::as_str),
        Some("edge-router")
    );
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("blue")
    );
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("eu")
    );
    assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("Gi10"));
    assert_eq!(fields.get("OUT_IF_NAME").map(String::as_str), Some("Gi20"));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("transit-a")
    );
    assert_eq!(
        fields.get("OUT_IF_CONNECTIVITY").map(String::as_str),
        Some("peering")
    );
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
    assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
    assert_eq!(fields.get("IN_IF_SPEED").map(String::as_str), Some("1000"));
    assert_eq!(
        fields.get("OUT_IF_SPEED").map(String::as_str),
        Some("10000")
    );
}

#[test]
fn metadata_classification_has_priority_over_classifiers() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        exporter_classifiers: vec![
            r#"ClassifyRegion("override-region") && ClassifyTenant("override-tenant")"#.to_string(),
        ],
        interface_classifiers: vec![
            r#"ClassifyProvider("override-provider") && ClassifyExternal() && SetName("ethX")"#
                .to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));

    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("eu")
    );
    assert_eq!(
        fields.get("EXPORTER_TENANT").map(String::as_str),
        Some("tenant-a")
    );
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("transit-a")
    );
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
    assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("Gi10"));
}

#[test]
fn exporter_classifier_cache_hit_is_used_before_re_evaluation() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"ClassifyRegion("live")"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("live")
    );

    let key = ExporterInfo {
        ip: "192.0.2.10".to_string(),
        name: "edge-router".to_string(),
    };
    {
        let mut cache = enricher
            .exporter_classifier_cache
            .lock()
            .expect("lock exporter cache");
        let entry = cache.entries.get_mut(&key).expect("cache entry");
        entry.value.region = "cached".to_string();
    }

    let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut next_fields));
    assert_eq!(
        next_fields.get("EXPORTER_REGION").map(String::as_str),
        Some("cached")
    );
}

#[test]
fn exporter_classifier_cache_entry_expires_by_ttl() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"ClassifyRegion("live")"#.to_string()],
        classifier_cache_duration: Duration::from_millis(1),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("live")
    );

    let key = ExporterInfo {
        ip: "192.0.2.10".to_string(),
        name: "edge-router".to_string(),
    };
    {
        let mut cache = enricher
            .exporter_classifier_cache
            .lock()
            .expect("lock exporter cache");
        let entry = cache.entries.get_mut(&key).expect("cache entry");
        entry.value.region = "stale-cache".to_string();
    }

    std::thread::sleep(Duration::from_millis(20));

    let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut next_fields));
    assert_eq!(
        next_fields.get("EXPORTER_REGION").map(String::as_str),
        Some("live")
    );
}

#[test]
fn exporter_classifier_assigns_region() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name startsWith "edge" && ClassifyRegion("EU West")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("euwest")
    );
}

#[test]
fn exporter_classifier_format_uses_exporter_name() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyTenant(Format("tenant-%s", Exporter.Name))"#.to_string()
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_TENANT").map(String::as_str),
        Some("tenant-edge-router")
    );
}

#[test]
fn exporter_classifier_matches_operator_assigns_group() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name matches "^edge-.*" && Classify("europe")"#.to_string()
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("EXPORTER_NAME", "edge-router".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("europe")
    );
}

#[test]
fn exporter_classifier_regex_with_character_class_extracts_group() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyRegex(Exporter.Name, "^(\\w+).r", "europe-$1")"#.to_string()
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("europe-edge")
    );
}

#[test]
fn exporter_classifier_multiline_expression_works() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            "Exporter.Name matches \"^edge-.*\" &&\nClassify(\"europe\")".to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("europe")
    );
}

#[test]
fn exporter_classifier_false_rule_is_noop() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"false"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("EXPORTER_GROUP").map(String::as_str), Some(""));
    assert_eq!(fields.get("EXPORTER_REGION").map(String::as_str), Some(""));
}

#[test]
fn exporter_classifier_fills_from_multiple_rules_until_complete() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name startsWith "hello" && ClassifyRegion("europe")"#.to_string(),
            r#"Exporter.Name startsWith "edge" && ClassifyRegion("asia")"#.to_string(),
            r#"ClassifySite("unknown") && ClassifyTenant("alfred")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("asia")
    );
    assert_eq!(
        fields.get("EXPORTER_SITE").map(String::as_str),
        Some("unknown")
    );
    assert_eq!(
        fields.get("EXPORTER_TENANT").map(String::as_str),
        Some("alfred")
    );
}

#[test]
fn exporter_classifier_reject_drops_flow() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"Reject()"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(!enricher.enrich_fields(&mut fields));
}

#[test]
fn exporter_classifier_runtime_error_stops_following_rules() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyTenant("alfred")"#.to_string(),
            r#"Exporter.Name > "hello""#.to_string(),
            r#"ClassifySite("should-not-apply")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_TENANT").map(String::as_str),
        Some("alfred")
    );
    assert_eq!(fields.get("EXPORTER_SITE").map(String::as_str), Some(""));
}

#[test]
fn exporter_classifier_invalid_regex_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyRegex(Exporter.Name, "^(ebp+.r", "europe-$1")"#.to_string()
        ],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_dynamic_regex_expression_is_allowed_at_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyRegex("something", Exporter.Name + "^(ebp+.r", "europe-$1")"#.to_string(),
        ],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_ok());
}

#[test]
fn exporter_classifier_invalid_argument_type_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"Classify(1)"#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_unquoted_identifier_argument_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"Classify(hello)"#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_non_matching_regex_does_not_set_value() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"ClassifyRegex(Exporter.Name, "^(ebp+).r", "europe-$1")"#.to_string()
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("EXPORTER_GROUP").map(String::as_str), Some(""));
}

#[test]
fn exporter_classifier_selective_reject_does_not_drop_flow() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"Exporter.Name startsWith "nothing" && Reject()"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
}

#[test]
fn exporter_classifier_syntax_error_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"Classify("europe""#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_non_boolean_expression_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#""hello""#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_unknown_action_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"ClassifyStuff("blip")"#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn exporter_classifier_or_supports_fallback_action() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "core" && ClassifyRegion("europe") || ClassifyRegion("fallback")"#
                    .to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("EXPORTER_NAME", "edge-router".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_REGION").map(String::as_str),
        Some("fallback")
    );
}

#[test]
fn exporter_classifier_not_operator_applies_negated_condition() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"!(Exporter.Name startsWith "core") && ClassifySite("branch")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_SITE").map(String::as_str),
        Some("branch")
    );
}

#[test]
fn exporter_classifier_keyword_boolean_operators_work() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"not Exporter.Name startsWith "core" and Exporter.Name startsWith "edge" and ClassifySite("branch")"#
                    .to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("EXPORTER_NAME", "edge-router".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_SITE").map(String::as_str),
        Some("branch")
    );
}

#[test]
fn exporter_classifier_in_operator_works_for_strings() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name in ["edge-router", "core-router"] && ClassifyGroup("metro")"#
                .to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("metro")
    );
}

#[test]
fn exporter_classifier_not_equals_operator_works() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name != "edge-router" && ClassifyRegion("other")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("EXPORTER_REGION").map(String::as_str), Some(""));
}

#[test]
fn exporter_classifier_contains_operator_works() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![
            r#"Exporter.Name contains "router" && ClassifyGroup("metro")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("EXPORTER_NAME", "edge-router".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_GROUP").map(String::as_str),
        Some("metro")
    );
}

#[test]
fn exporter_classifier_and_has_higher_precedence_than_or() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "edge" || Exporter.Name startsWith "core" && ClassifySite("branch")"#.to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("EXPORTER_SITE").map(String::as_str), Some(""));
}

#[test]
fn exporter_classifier_parentheses_override_boolean_precedence() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"(Exporter.Name startsWith "edge" || Exporter.Name startsWith "core") && ClassifySite("branch")"#.to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_SITE").map(String::as_str),
        Some("branch")
    );
}

#[test]
fn interface_classifier_sets_provider_and_renames_with_format() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"Interface.Index == 10 && ClassifyProvider("Transit-101") && SetName("eth10")"#
                .to_string(),
            r#"Interface.VLAN > 200 && SetName(Format("%s.%d", Interface.Name, Interface.VLAN))"#
                .to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("transit-101")
    );
    assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("eth10"));
    assert_eq!(
        fields.get("OUT_IF_NAME").map(String::as_str),
        Some("Gi20.300")
    );
}

#[test]
fn interface_classifier_cache_hit_is_used_before_re_evaluation() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"ClassifyProvider("live")"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("live")
    );

    let key = ExporterAndInterfaceInfo {
        exporter: ExporterInfo {
            ip: "192.0.2.10".to_string(),
            name: "edge-router".to_string(),
        },
        interface: InterfaceInfo {
            index: 10,
            name: "Gi10".to_string(),
            description: "10th interface".to_string(),
            speed: 1000,
            vlan: 10,
        },
    };
    {
        let mut cache = enricher
            .interface_classifier_cache
            .lock()
            .expect("lock interface cache");
        let entry = cache.entries.get_mut(&key).expect("cache entry");
        entry.value.provider = "cached".to_string();
    }

    let mut next_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut next_fields));
    assert_eq!(
        next_fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("cached")
    );
}

#[test]
fn exporter_classifier_cache_prunes_expired_entries_on_insert() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_exporter_classification(),
        exporter_classifiers: vec![r#"ClassifyRegion("live")"#.to_string()],
        classifier_cache_duration: Duration::from_millis(1),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut first_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut first_fields));

    std::thread::sleep(Duration::from_millis(20));

    let mut second_fields = base_fields("192.0.2.11", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut second_fields));

    let cache = enricher
        .exporter_classifier_cache
        .lock()
        .expect("lock exporter cache");
    assert_eq!(cache.entries.len(), 1);
    assert!(
        !cache.entries.contains_key(&ExporterInfo {
            ip: "192.0.2.10".to_string(),
            name: "edge-router".to_string(),
        }),
        "expired exporter cache entry should be pruned on the next insert"
    );
}

#[test]
fn interface_classifier_cache_prunes_expired_entries_on_insert() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"ClassifyProvider("live")"#.to_string()],
        classifier_cache_duration: Duration::from_millis(1),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut first_fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut first_fields));

    std::thread::sleep(Duration::from_millis(20));

    let mut second_fields = base_fields("192.0.2.10", 30, 40, 100, 10, 300);
    assert!(enricher.enrich_fields(&mut second_fields));

    let cache = enricher
        .interface_classifier_cache
        .lock()
        .expect("lock interface cache");
    assert_eq!(
        cache.entries.len(),
        2,
        "the fresh flow should repopulate only the current interface keys"
    );
    assert!(
        !cache.entries.contains_key(&ExporterAndInterfaceInfo {
            exporter: ExporterInfo {
                ip: "192.0.2.10".to_string(),
                name: "edge-router".to_string(),
            },
            interface: InterfaceInfo {
                index: 10,
                name: "Gi10".to_string(),
                description: "10th interface".to_string(),
                speed: 1000,
                vlan: 10,
            },
        }),
        "stale interface cache entries should be pruned before new ones are inserted"
    );
    assert!(
        cache.entries.contains_key(&ExporterAndInterfaceInfo {
            exporter: ExporterInfo {
                ip: "192.0.2.10".to_string(),
                name: "edge-router".to_string(),
            },
            interface: InterfaceInfo {
                index: 30,
                name: "Default0".to_string(),
                description: "Default interface".to_string(),
                speed: 1000,
                vlan: 10,
            },
        }),
        "new default-interface cache entries should remain after pruning"
    );
}

#[test]
fn interface_classifier_classify_provider_with_format_works() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"ClassifyProvider(Format("II-%s", Interface.Name))"#.to_string()
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("ii-gi10")
    );
    assert_eq!(
        fields.get("OUT_IF_PROVIDER").map(String::as_str),
        Some("ii-gi20")
    );
}

#[test]
fn interface_classifier_reject_drops_flow() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"Reject()"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(!enricher.enrich_fields(&mut fields));
}

#[test]
fn interface_classifier_false_rule_is_noop() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"false"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("IN_IF_PROVIDER").map(String::as_str), Some(""));
    assert_eq!(fields.get("IN_IF_BOUNDARY"), None);
}

#[test]
fn interface_classifier_in_operator_works_with_numeric_values() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"Interface.Index in [9, 10, 11] && ClassifyProvider("edge-range")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("edge-range")
    );
    assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
}

#[test]
fn interface_classifier_sets_name_and_description() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"Interface.Index == 10 && SetName("eth10")"#.to_string(),
            r#"Interface.Index == 20 && SetDescription("uplink")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("eth10"));
    assert_eq!(
        fields.get("OUT_IF_DESCRIPTION").map(String::as_str),
        Some("uplink")
    );
}

#[test]
fn interface_classifier_unquoted_identifier_argument_is_rejected_during_config_parse() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"ClassifyProvider(foo)"#.to_string()],
        ..Default::default()
    };
    assert!(FlowEnricher::from_config(&cfg).is_err());
}

#[test]
fn interface_classifier_vlan_equality_applies_only_to_matching_direction() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![r#"Interface.VLAN == 100 && ClassifyExternal()"#.to_string()],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 200);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
    assert_eq!(fields.get("OUT_IF_BOUNDARY"), None);
}

#[test]
fn interface_classifier_first_write_wins_for_provider_and_boundary() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"ClassifyInternal()"#.to_string(),
            r#"ClassifyExternal()"#.to_string(),
            r#"ClassifyProvider("telia")"#.to_string(),
            r#"ClassifyProvider("cogent")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("2"));
    assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("telia")
    );
    assert_eq!(
        fields.get("OUT_IF_PROVIDER").map(String::as_str),
        Some("telia")
    );
}

#[test]
fn interface_classifier_regex_and_boundary_rules_match_akvorado_expectations() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"ClassifyProvider("Othello")"#.to_string(),
                r#"ClassifyConnectivityRegex(Interface.Description, "^(1\\d*)th interface$", "P$1") && ClassifyExternal()"#
                    .to_string(),
                r#"ClassifyInternal() && ClassifyConnectivity("core")"#.to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("othello")
    );
    assert_eq!(
        fields.get("OUT_IF_PROVIDER").map(String::as_str),
        Some("othello")
    );
    assert_eq!(
        fields.get("IN_IF_CONNECTIVITY").map(String::as_str),
        Some("p10")
    );
    assert_eq!(
        fields.get("OUT_IF_CONNECTIVITY").map(String::as_str),
        Some("core")
    );
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
    assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
}

#[test]
fn interface_classifier_or_with_actions_respects_short_circuit() {
    let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"(Interface.VLAN == 100 && ClassifyProvider("TransitA")) || ClassifyProvider("TransitB")"#
                    .to_string(),
            ],
            ..Default::default()
        };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("transita")
    );
    assert_eq!(
        fields.get("OUT_IF_PROVIDER").map(String::as_str),
        Some("transitb")
    );
}

#[test]
fn interface_classifier_supports_le_ge_and_lt_operators() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"Interface.VLAN <= 100 && ClassifyInternal()"#.to_string(),
            r#"Interface.VLAN >= 300 && ClassifyExternal()"#.to_string(),
            r#"Interface.VLAN < 200 && ClassifyProvider("low")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("2"));
    assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("1"));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("low")
    );
    assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
}

#[test]
fn interface_classifier_ends_with_operator_works() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_without_interface_classification(),
        interface_classifiers: vec![
            r#"Interface.Name endsWith "10" && ClassifyProvider("suffix")"#.to_string(),
        ],
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 100, 300);

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("IN_IF_PROVIDER").map(String::as_str),
        Some("suffix")
    );
    assert_eq!(fields.get("OUT_IF_PROVIDER").map(String::as_str), Some(""));
}

#[test]
fn geoip_only_enrichment_keeps_flow_without_static_metadata() {
    let cfg = EnrichmentConfig {
        geoip: GeoIpConfig {
            asn_database: vec!["/path/that/does/not/exist/asn.mmdb".to_string()],
            geo_database: vec!["/path/that/does/not/exist/geo.mmdb".to_string()],
            optional: true,
        },
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 0, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_NAME").map(String::as_str),
        Some("192.0.2.10")
    );
    assert_eq!(fields.get("SAMPLING_RATE").map(String::as_str), Some("0"));
}

#[test]
fn static_metadata_without_sampling_keeps_flow() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 0, 10, 300);
    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("EXPORTER_NAME").map(String::as_str),
        Some("edge-router")
    );
    assert_eq!(fields.get("SAMPLING_RATE").map(String::as_str), Some("0"));
}

#[test]
fn asn_provider_order_matches_akvorado_behavior() {
    let mut enricher = test_enricher_for_provider_order();
    let cases = [
        (
            vec![AsnProviderConfig::Flow],
            12_322_u32,
            0_u32,
            24_u8,
            12_322_u32,
        ),
        (
            vec![AsnProviderConfig::FlowExceptPrivate],
            65_536_u32,
            0_u32,
            24_u8,
            0_u32,
        ),
        (
            vec![
                AsnProviderConfig::FlowExceptPrivate,
                AsnProviderConfig::Flow,
            ],
            65_536_u32,
            0_u32,
            24_u8,
            65_536_u32,
        ),
        (
            vec![AsnProviderConfig::FlowExceptDefaultRoute],
            12_322_u32,
            0_u32,
            0_u8,
            0_u32,
        ),
        (
            vec![AsnProviderConfig::Routing],
            12_322_u32,
            1_299_u32,
            24_u8,
            1_299_u32,
        ),
        (
            vec![AsnProviderConfig::Geoip, AsnProviderConfig::Routing],
            12_322_u32,
            65_300_u32,
            24_u8,
            0_u32,
        ),
    ];

    for (providers, flow_as, routing_as, flow_mask, expected) in cases {
        enricher.asn_providers = providers;
        assert_eq!(
            enricher.get_as_number(flow_as, routing_as, flow_mask),
            expected
        );
    }
}

#[test]
fn net_mask_provider_order_matches_akvorado_behavior() {
    let mut enricher = test_enricher_for_provider_order();
    let cases = [
        (vec![NetProviderConfig::Flow], 12_u8, 24_u8, 12_u8),
        (vec![NetProviderConfig::Routing], 12_u8, 24_u8, 24_u8),
        (
            vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
            12_u8,
            24_u8,
            24_u8,
        ),
        (
            vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
            12_u8,
            24_u8,
            12_u8,
        ),
        (
            vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
            12_u8,
            0_u8,
            12_u8,
        ),
    ];

    for (providers, flow_mask, routing_mask, expected) in cases {
        enricher.net_providers = providers;
        assert_eq!(enricher.get_net_mask(flow_mask, routing_mask), expected);
    }
}

#[test]
fn next_hop_provider_order_matches_akvorado_behavior() {
    let mut enricher = test_enricher_for_provider_order();
    let nh1: IpAddr = "2001:db8::1".parse().expect("parse nh1");
    let nh2: IpAddr = "2001:db8::2".parse().expect("parse nh2");
    let cases = [
        (
            vec![NetProviderConfig::Flow],
            Some(nh1),
            Some(nh2),
            Some(nh1),
        ),
        (
            vec![NetProviderConfig::Routing],
            Some(nh1),
            Some(nh2),
            Some(nh2),
        ),
        (
            vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
            Some(nh1),
            Some(nh2),
            Some(nh2),
        ),
        (
            vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
            Some(nh1),
            Some(nh2),
            Some(nh1),
        ),
        (
            vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
            None,
            None,
            None,
        ),
    ];

    for (providers, flow_nh, routing_nh, expected) in cases {
        enricher.net_providers = providers;
        assert_eq!(enricher.get_next_hop(flow_nh, routing_nh), expected);
    }
}

#[test]
fn static_routing_enrichment_updates_as_mask_nexthop_and_appends_arrays() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
        asn_providers: vec![AsnProviderConfig::Routing, AsnProviderConfig::Flow],
        net_providers: vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
        routing_static: StaticRoutingConfig {
            prefixes: BTreeMap::from([
                (
                    "10.10.0.0/16".to_string(),
                    StaticRoutingEntryConfig {
                        asn: 64_500,
                        net_mask: Some(16),
                        ..Default::default()
                    },
                ),
                (
                    "198.51.100.0/24".to_string(),
                    StaticRoutingEntryConfig {
                        asn: 64_600,
                        as_path: vec![64_550, 64_600],
                        communities: vec![123_456, 654_321],
                        large_communities: vec![StaticRoutingLargeCommunityConfig {
                            asn: 64_600,
                            local_data1: 7,
                            local_data2: 8,
                        }],
                        next_hop: "203.0.113.9".to_string(),
                        net_mask: Some(24),
                    },
                ),
            ]),
        },
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("SRC_ADDR", "10.10.20.30".to_string());
    fields.insert("DST_ADDR", "198.51.100.42".to_string());
    fields.insert("SRC_AS", "65100".to_string());
    fields.insert("DST_AS", "65200".to_string());
    fields.insert("SRC_MASK", "30".to_string());
    fields.insert("DST_MASK", "31".to_string());
    fields.insert("NEXT_HOP", String::new());
    fields.insert("DST_AS_PATH", "65000".to_string());
    fields.insert("DST_COMMUNITIES", "111".to_string());
    fields.insert("DST_LARGE_COMMUNITIES", "1:1:1".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64500"));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64600"));
    assert_eq!(fields.get("SRC_MASK").map(String::as_str), Some("16"));
    assert_eq!(fields.get("DST_MASK").map(String::as_str), Some("24"));
    assert_eq!(
        fields.get("NEXT_HOP").map(String::as_str),
        Some("203.0.113.9")
    );
    assert_eq!(
        fields.get("DST_AS_PATH").map(String::as_str),
        Some("65000,64550,64600")
    );
    assert_eq!(
        fields.get("DST_COMMUNITIES").map(String::as_str),
        Some("111,123456,654321")
    );
    assert_eq!(
        fields.get("DST_LARGE_COMMUNITIES").map(String::as_str),
        Some("1:1:1,64600:7:8")
    );
}

#[test]
fn dynamic_routing_runtime_prefers_exact_next_hop() {
    let runtime = DynamicRoutingRuntime::default();
    let peer_a = DynamicRoutingPeerKey {
        exporter: "192.0.2.10:10179".parse().expect("parse exporter A"),
        session_id: 1,
        peer_id: "peer-a".to_string(),
    };
    let peer_b = DynamicRoutingPeerKey {
        exporter: "192.0.2.10:10179".parse().expect("parse exporter B"),
        session_id: 1,
        peer_id: "peer-b".to_string(),
    };
    let prefix = parse_prefix("198.51.100.0/24").expect("parse prefix");
    let nh_a: IpAddr = "203.0.113.1".parse().expect("parse nh a");
    let nh_b: IpAddr = "203.0.113.2".parse().expect("parse nh b");

    runtime.upsert(DynamicRoutingUpdate {
        peer: peer_a.clone(),
        prefix,
        route_key: "route-a".to_string(),
        next_hop: Some(nh_a),
        asn: 64_500,
        as_path: vec![64_500],
        communities: vec![],
        large_communities: vec![],
    });
    runtime.upsert(DynamicRoutingUpdate {
        peer: peer_b,
        prefix,
        route_key: "route-b".to_string(),
        next_hop: Some(nh_b),
        asn: 64_600,
        as_path: vec![64_600],
        communities: vec![],
        large_communities: vec![],
    });

    let selected = runtime
        .lookup(
            "198.51.100.42".parse().expect("parse ip"),
            Some(nh_b),
            Some("192.0.2.10".parse().expect("parse exporter ip")),
        )
        .expect("route with matching next-hop");
    assert_eq!(selected.asn, 64_600);

    runtime.clear_peer(&peer_a);
    assert_eq!(runtime.route_count(), 1);
}

#[test]
fn dynamic_routing_enrichment_overrides_static_when_enabled() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
        asn_providers: vec![AsnProviderConfig::Routing, AsnProviderConfig::Flow],
        net_providers: vec![NetProviderConfig::Routing, NetProviderConfig::Flow],
        routing_dynamic: RoutingDynamicConfig {
            bmp: RoutingDynamicBmpConfig {
                enabled: true,
                ..Default::default()
            },
            ..Default::default()
        },
        routing_static: StaticRoutingConfig {
            prefixes: BTreeMap::from([(
                "198.51.100.0/24".to_string(),
                StaticRoutingEntryConfig {
                    asn: 64_601,
                    net_mask: Some(24),
                    next_hop: "203.0.113.10".to_string(),
                    ..Default::default()
                },
            )]),
        },
        ..Default::default()
    };
    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let runtime = enricher
        .dynamic_routing_runtime()
        .expect("dynamic routing runtime");
    let peer = DynamicRoutingPeerKey {
        exporter: "192.0.2.10:10179".parse().expect("parse exporter"),
        session_id: 1,
        peer_id: "peer-1".to_string(),
    };
    runtime.upsert(DynamicRoutingUpdate {
        peer,
        prefix: parse_prefix("198.51.100.0/24").expect("prefix"),
        route_key: "route-1".to_string(),
        next_hop: Some("203.0.113.9".parse().expect("next hop")),
        asn: 64_700,
        as_path: vec![64_690, 64_700],
        communities: vec![100],
        large_communities: vec![(64_700, 1, 2)],
    });

    let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
    fields.insert("SRC_ADDR", "10.10.20.30".to_string());
    fields.insert("DST_ADDR", "198.51.100.42".to_string());
    fields.insert("SRC_AS", "65100".to_string());
    fields.insert("DST_AS", "65200".to_string());
    fields.insert("SRC_MASK", "30".to_string());
    fields.insert("DST_MASK", "31".to_string());
    fields.insert("NEXT_HOP", "203.0.113.9".to_string());
    fields.insert("DST_AS_PATH", String::new());
    fields.insert("DST_COMMUNITIES", String::new());
    fields.insert("DST_LARGE_COMMUNITIES", String::new());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64700"));
    assert_eq!(
        fields.get("NEXT_HOP").map(String::as_str),
        Some("203.0.113.9")
    );
    assert_eq!(
        fields.get("DST_AS_PATH").map(String::as_str),
        Some("64690,64700")
    );
    assert_eq!(
        fields.get("DST_COMMUNITIES").map(String::as_str),
        Some("100")
    );
    assert_eq!(
        fields.get("DST_LARGE_COMMUNITIES").map(String::as_str),
        Some("64700:1:2")
    );
}

#[test]
fn network_enrichment_populates_network_dimensions_and_asn_fallback() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
        networks: BTreeMap::from([(
            "198.51.100.0/24".to_string(),
            NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                name: "edge-net".to_string(),
                role: "customer".to_string(),
                site: "par1".to_string(),
                region: "eu-west".to_string(),
                country: "FR".to_string(),
                state: "Ile-de-France".to_string(),
                city: "Paris".to_string(),
                latitude: Some(48.8566),
                longitude: Some(2.3522),
                tenant: "tenant-a".to_string(),
                asn: 64_500,
            }),
        )]),
        ..Default::default()
    };

    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
    fields.insert("SRC_ADDR", "198.51.100.10".to_string());
    fields.insert("DST_ADDR", "198.51.100.20".to_string());
    fields.insert("SRC_AS", "0".to_string());
    fields.insert("DST_AS", "0".to_string());
    fields.insert("SRC_MASK", "24".to_string());
    fields.insert("DST_MASK", "24".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64500"));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64500"));
    assert_eq!(
        fields.get("SRC_NET_NAME").map(String::as_str),
        Some("edge-net")
    );
    assert_eq!(
        fields.get("DST_NET_NAME").map(String::as_str),
        Some("edge-net")
    );
    assert_eq!(
        fields.get("SRC_NET_ROLE").map(String::as_str),
        Some("customer")
    );
    assert_eq!(
        fields.get("DST_NET_ROLE").map(String::as_str),
        Some("customer")
    );
    assert_eq!(fields.get("SRC_NET_SITE").map(String::as_str), Some("par1"));
    assert_eq!(fields.get("DST_NET_SITE").map(String::as_str), Some("par1"));
    assert_eq!(
        fields.get("SRC_NET_REGION").map(String::as_str),
        Some("eu-west")
    );
    assert_eq!(
        fields.get("DST_NET_REGION").map(String::as_str),
        Some("eu-west")
    );
    assert_eq!(
        fields.get("SRC_NET_TENANT").map(String::as_str),
        Some("tenant-a")
    );
    assert_eq!(
        fields.get("DST_NET_TENANT").map(String::as_str),
        Some("tenant-a")
    );
    assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("FR"));
    assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("FR"));
    assert_eq!(
        fields.get("SRC_GEO_CITY").map(String::as_str),
        Some("Paris")
    );
    assert_eq!(
        fields.get("DST_GEO_CITY").map(String::as_str),
        Some("Paris")
    );
    assert_eq!(
        fields.get("SRC_GEO_STATE").map(String::as_str),
        Some("Ile-de-France")
    );
    assert_eq!(
        fields.get("DST_GEO_STATE").map(String::as_str),
        Some("Ile-de-France")
    );
    assert_eq!(
        fields.get("SRC_GEO_LATITUDE").map(String::as_str),
        Some("48.856600")
    );
    assert_eq!(
        fields.get("DST_GEO_LATITUDE").map(String::as_str),
        Some("48.856600")
    );
    assert_eq!(
        fields.get("SRC_GEO_LONGITUDE").map(String::as_str),
        Some("2.352200")
    );
    assert_eq!(
        fields.get("DST_GEO_LONGITUDE").map(String::as_str),
        Some("2.352200")
    );
}

#[test]
fn network_enrichment_prefers_network_asn_when_present() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
        networks: BTreeMap::from([(
            "198.51.100.0/24".to_string(),
            NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                asn: 64_500,
                ..Default::default()
            }),
        )]),
        ..Default::default()
    };

    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
    fields.insert("SRC_ADDR", "198.51.100.10".to_string());
    fields.insert("DST_ADDR", "198.51.100.20".to_string());
    fields.insert("SRC_AS", "65001".to_string());
    fields.insert("DST_AS", "0".to_string());
    fields.insert("SRC_MASK", "24".to_string());
    fields.insert("DST_MASK", "24".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64500"));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64500"));
}

#[test]
fn network_enrichment_merges_supernet_and_subnet_attributes() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
        networks: BTreeMap::from([
            (
                "198.51.0.0/16".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    region: "eu-west".to_string(),
                    country: "FR".to_string(),
                    tenant: "tenant-a".to_string(),
                    asn: 64_501,
                    ..Default::default()
                }),
            ),
            (
                "198.51.100.0/24".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    name: "edge-net".to_string(),
                    role: "customer".to_string(),
                    site: "par1".to_string(),
                    ..Default::default()
                }),
            ),
        ]),
        ..Default::default()
    };

    let mut enricher = FlowEnricher::from_config(&cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let mut fields = base_fields("192.0.2.10", 10, 20, 1000, 10, 20);
    fields.insert("SRC_ADDR", "198.51.100.10".to_string());
    fields.insert("DST_ADDR", "198.51.100.20".to_string());
    fields.insert("SRC_AS", "0".to_string());
    fields.insert("DST_AS", "0".to_string());
    fields.insert("SRC_MASK", "24".to_string());
    fields.insert("DST_MASK", "24".to_string());

    assert!(enricher.enrich_fields(&mut fields));
    assert_eq!(
        fields.get("SRC_NET_NAME").map(String::as_str),
        Some("edge-net")
    );
    assert_eq!(
        fields.get("DST_NET_NAME").map(String::as_str),
        Some("edge-net")
    );
    assert_eq!(
        fields.get("SRC_NET_ROLE").map(String::as_str),
        Some("customer")
    );
    assert_eq!(
        fields.get("DST_NET_ROLE").map(String::as_str),
        Some("customer")
    );
    assert_eq!(fields.get("SRC_NET_SITE").map(String::as_str), Some("par1"));
    assert_eq!(fields.get("DST_NET_SITE").map(String::as_str), Some("par1"));
    assert_eq!(
        fields.get("SRC_NET_REGION").map(String::as_str),
        Some("eu-west")
    );
    assert_eq!(
        fields.get("DST_NET_REGION").map(String::as_str),
        Some("eu-west")
    );
    assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("FR"));
    assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("FR"));
    assert_eq!(
        fields.get("SRC_NET_TENANT").map(String::as_str),
        Some("tenant-a")
    );
    assert_eq!(
        fields.get("DST_NET_TENANT").map(String::as_str),
        Some("tenant-a")
    );
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64501"));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64501"));
}

#[test]
fn network_attributes_asn_override_clears_stale_asn_name() {
    let mut base = NetworkAttributes {
        asn: 64_496,
        asn_name: "Legacy Transit".to_string(),
        ..NetworkAttributes::default()
    };
    let overlay = NetworkAttributes {
        asn: 64_500,
        asn_name: String::new(),
        ..NetworkAttributes::default()
    };

    base.merge_from(&overlay);

    assert_eq!(base.asn, 64_500);
    assert!(
        base.asn_name.is_empty(),
        "ASN override without a replacement name must clear the previous name"
    );
}

#[test]
fn optional_geoip_missing_databases_are_accepted() {
    let cfg = GeoIpConfig {
        asn_database: vec!["/path/that/does/not/exist/asn.mmdb".to_string()],
        geo_database: vec!["/path/that/does/not/exist/geo.mmdb".to_string()],
        optional: true,
    };
    let resolver = GeoIpResolver::from_config(&cfg).expect("optional geoip config");
    assert!(resolver.is_some());
    let resolver = resolver.expect("resolver should exist");
    assert!(resolver.asn_databases.is_empty());
    assert!(resolver.geo_databases.is_empty());
}

#[test]
fn geoip_signature_changes_when_database_file_changes() {
    let dir = tempdir().expect("create tempdir");
    let db_path = dir.path().join("asn.mmdb");
    {
        let mut file = std::fs::File::create(&db_path).expect("create file");
        file.write_all(b"v1").expect("write v1");
        file.sync_all().expect("sync v1");
    }
    let first = read_geoip_file_signature(db_path.to_str().expect("utf-8 path"), false)
        .expect("signature v1")
        .expect("signature should exist");

    std::thread::sleep(Duration::from_millis(2));
    {
        let mut file = std::fs::OpenOptions::new()
            .write(true)
            .truncate(true)
            .open(&db_path)
            .expect("open file");
        file.write_all(b"v2-longer").expect("write v2");
        file.sync_all().expect("sync v2");
    }
    let second = read_geoip_file_signature(db_path.to_str().expect("utf-8 path"), false)
        .expect("signature v2")
        .expect("signature should exist");

    assert_ne!(first, second);
}

#[test]
fn parse_asn_text_supports_prefixed_and_numeric_values() {
    assert_eq!(parse_asn_text("AS64512"), Some(64_512));
    assert_eq!(parse_asn_text("as64513"), Some(64_513));
    assert_eq!(parse_asn_text("64514"), Some(64_514));
    assert_eq!(parse_asn_text("ASX"), None);
}

#[test]
fn decode_asn_name_returns_non_empty_organization() {
    let record = AsnLookupRecord {
        autonomous_system_number: Some(64_512),
        autonomous_system_organization: Some("Example Transit".to_string()),
        asn: None,
        netdata: NetdataLookupRecord::default(),
    };

    assert_eq!(decode_asn_name(&record).as_deref(), Some("Example Transit"));
}

#[test]
fn decode_ip_class_returns_non_empty_value() {
    let record = AsnLookupRecord {
        autonomous_system_number: None,
        autonomous_system_organization: None,
        asn: None,
        netdata: NetdataLookupRecord {
            ip_class: "private".to_string(),
        },
    };

    assert_eq!(decode_ip_class(&record).as_deref(), Some("private"));
}

#[test]
fn apply_geo_record_normalizes_coordinates_and_rejects_invalid_values() {
    let mut attrs = NetworkAttributes::default();
    let record = GeoLookupRecord {
        country: None,
        city: None,
        location: Some(LocationValue {
            latitude: Some(48.8566),
            longitude: Some(2.3522),
        }),
        subdivisions: Vec::new(),
        region: None,
    };

    apply_geo_record(&mut attrs, &record);

    assert_eq!(attrs.latitude, "48.856600");
    assert_eq!(attrs.longitude, "2.352200");

    let invalid = GeoLookupRecord {
        country: None,
        city: None,
        location: Some(LocationValue {
            latitude: Some(120.0),
            longitude: Some(f64::NAN),
        }),
        subdivisions: Vec::new(),
        region: None,
    };
    apply_geo_record(&mut attrs, &invalid);

    assert_eq!(attrs.latitude, "48.856600");
    assert_eq!(attrs.longitude, "2.352200");
}

#[test]
fn effective_as_name_uses_private_space_label_only_for_private_zero_asn() {
    let attrs = NetworkAttributes {
        ip_class: "private".to_string(),
        ..Default::default()
    };

    assert_eq!(
        effective_as_name(Some(&attrs), 0),
        "AS0 Private IP Address Space"
    );
    assert_eq!(effective_as_name(Some(&attrs), 64_500), "AS64500");
    assert_eq!(effective_as_name(None, 0), "AS0 Unknown ASN");
}

#[test]
fn effective_as_name_uses_unknown_label_for_non_private_zero_asn() {
    let attrs = NetworkAttributes::default();

    assert_eq!(effective_as_name(Some(&attrs), 0), "AS0 Unknown ASN");
}

#[test]
fn write_network_attributes_uses_private_space_label_for_zero_private_asn() {
    let attrs = NetworkAttributes {
        ip_class: "private".to_string(),
        ..Default::default()
    };
    let mut fields = FlowFields::new();

    write_network_attributes(&mut fields, &SRC_KEYS, Some(&attrs), 0);

    assert_eq!(
        fields.get("SRC_AS_NAME").map(String::as_str),
        Some("AS0 Private IP Address Space")
    );
}

#[test]
fn write_network_attributes_uses_unknown_label_for_zero_non_private_asn() {
    let attrs = NetworkAttributes::default();
    let mut fields = FlowFields::new();

    write_network_attributes(&mut fields, &SRC_KEYS, Some(&attrs), 0);

    assert_eq!(
        fields.get("SRC_AS_NAME").map(String::as_str),
        Some("AS0 Unknown ASN")
    );
}

#[test]
fn write_network_attributes_record_src_uses_unknown_label_for_zero_non_private_asn() {
    let attrs = NetworkAttributes::default();
    let mut rec = FlowRecord {
        src_as: 0,
        ..FlowRecord::default()
    };

    write_network_attributes_record_src(&mut rec, Some(&attrs));

    assert_eq!(rec.src_as_name, "AS0 Unknown ASN");
}

#[test]
fn network_asn_override_prefers_network_asn_when_present() {
    assert_eq!(apply_network_asn_override(65_001, 64_500), 64_500);
    assert_eq!(apply_network_asn_override(65_001, 0), 65_001);
    assert_eq!(apply_network_asn_override(0, 64_500), 64_500);
}

#[test]
fn private_asn_classification_matches_akvorado_special_range_boundaries() {
    assert!(is_private_as(23_456));
    assert!(!is_private_as(64_495));
    assert!(is_private_as(64_496));
    assert!(is_private_as(64_511));
    assert!(is_private_as(64_512));
    assert!(is_private_as(65_534));
    assert!(is_private_as(65_535));
    assert!(is_private_as(65_536));
    assert!(is_private_as(65_551));
    assert!(!is_private_as(65_552));
    assert!(!is_private_as(4_199_999_999));
    assert!(is_private_as(4_200_000_000));
    assert!(is_private_as(4_294_967_294));
    assert!(is_private_as(4_294_967_295));
}

fn metadata_config_for_192() -> StaticMetadataConfig {
    StaticMetadataConfig {
        exporters: BTreeMap::from([(
            "192.0.2.0/24".to_string(),
            StaticExporterConfig {
                name: "edge-router".to_string(),
                region: "eu".to_string(),
                role: "peering".to_string(),
                tenant: "tenant-a".to_string(),
                site: "par".to_string(),
                group: "blue".to_string(),
                default: StaticInterfaceConfig {
                    name: "Default0".to_string(),
                    description: "Default interface".to_string(),
                    speed: 1000,
                    provider: String::new(),
                    connectivity: String::new(),
                    boundary: 0,
                },
                if_indexes: BTreeMap::from([
                    (
                        10_u32,
                        StaticInterfaceConfig {
                            name: "Gi10".to_string(),
                            description: "10th interface".to_string(),
                            speed: 1000,
                            provider: "transit-a".to_string(),
                            connectivity: "transit".to_string(),
                            boundary: 1,
                        },
                    ),
                    (
                        20_u32,
                        StaticInterfaceConfig {
                            name: "Gi20".to_string(),
                            description: "20th interface".to_string(),
                            speed: 10000,
                            provider: "ix".to_string(),
                            connectivity: "peering".to_string(),
                            boundary: 2,
                        },
                    ),
                ]),
                skip_missing_interfaces: false,
            },
        )]),
    }
}

fn metadata_config_without_exporter_classification() -> StaticMetadataConfig {
    let mut cfg = metadata_config_for_192();
    for exporter in cfg.exporters.values_mut() {
        exporter.region.clear();
        exporter.role.clear();
        exporter.site.clear();
        exporter.group.clear();
        exporter.tenant.clear();
    }
    cfg
}

fn metadata_config_without_interface_classification() -> StaticMetadataConfig {
    let mut cfg = metadata_config_without_exporter_classification();
    for exporter in cfg.exporters.values_mut() {
        for iface in exporter.if_indexes.values_mut() {
            iface.provider.clear();
            iface.connectivity.clear();
            iface.boundary = 0;
        }
    }
    cfg
}

fn metadata_config_with_zero_interface_state() -> StaticMetadataConfig {
    let mut cfg = metadata_config_without_interface_classification();
    for exporter in cfg.exporters.values_mut() {
        exporter.default.speed = 0;
        exporter.default.boundary = 0;
        for iface in exporter.if_indexes.values_mut() {
            iface.speed = 0;
            iface.boundary = 0;
        }
    }
    cfg
}

fn base_fields(
    exporter_ip: &str,
    in_if: u32,
    out_if: u32,
    sampling_rate: u64,
    src_vlan: u16,
    dst_vlan: u16,
) -> FlowFields {
    BTreeMap::from([
        ("EXPORTER_IP", exporter_ip.to_string()),
        ("IN_IF", in_if.to_string()),
        ("OUT_IF", out_if.to_string()),
        ("SAMPLING_RATE", sampling_rate.to_string()),
        ("SRC_VLAN", src_vlan.to_string()),
        ("DST_VLAN", dst_vlan.to_string()),
        ("EXPORTER_NAME", String::new()),
    ])
}

fn test_enricher_for_provider_order() -> FlowEnricher {
    FlowEnricher {
        default_sampling_rate: PrefixMap::default(),
        override_sampling_rate: PrefixMap::default(),
        static_metadata: StaticMetadata::default(),
        networks: PrefixMap::default(),
        geoip: None,
        network_sources_runtime: None,
        exporter_classifiers: Vec::new(),
        interface_classifiers: Vec::new(),
        classifier_cache_duration: Duration::from_secs(5 * 60),
        exporter_classifier_cache: Arc::new(Mutex::new(ExporterClassifierCache::default())),
        interface_classifier_cache: Arc::new(Mutex::new(InterfaceClassifierCache::default())),
        asn_providers: vec![
            AsnProviderConfig::Flow,
            AsnProviderConfig::Routing,
            AsnProviderConfig::Geoip,
        ],
        net_providers: vec![NetProviderConfig::Flow, NetProviderConfig::Routing],
        static_routing: StaticRouting::default(),
        dynamic_routing: None,
    }
}

/// Build a FlowRecord equivalent to the given base_fields.
fn base_record(
    exporter_ip: &str,
    in_if: u32,
    out_if: u32,
    sampling_rate: u64,
    src_vlan: u16,
    dst_vlan: u16,
) -> FlowRecord {
    let mut rec = FlowRecord {
        exporter_ip: exporter_ip.parse::<IpAddr>().ok(),
        in_if,
        out_if,
        ..Default::default()
    };
    rec.set_sampling_rate(sampling_rate);
    rec.set_src_vlan(src_vlan);
    rec.set_dst_vlan(dst_vlan);
    rec
}

/// Compare enriched FlowFields from enrich_fields with to_fields() output
/// from an equivalently enriched FlowRecord.
fn assert_enrich_equivalence(
    cfg: &EnrichmentConfig,
    fields: &mut FlowFields,
    rec: &mut FlowRecord,
) {
    let mut enricher1 = FlowEnricher::from_config(cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");
    let mut enricher2 = FlowEnricher::from_config(cfg)
        .expect("build enricher")
        .expect("enricher must be enabled");

    let result_fields = enricher1.enrich_fields(fields);
    let result_record = enricher2.enrich_record(rec);

    assert_eq!(result_fields, result_record, "enrich return value mismatch");

    if !result_fields {
        return;
    }

    let rec_fields = rec.to_fields();

    // Compare all enrichment-written fields.
    let enrichment_keys = [
        "SAMPLING_RATE",
        "SRC_MASK",
        "DST_MASK",
        "SRC_AS",
        "DST_AS",
        "SRC_AS_NAME",
        "DST_AS_NAME",
        "NEXT_HOP",
        "SRC_NET_NAME",
        "SRC_NET_ROLE",
        "SRC_NET_SITE",
        "SRC_NET_REGION",
        "SRC_NET_TENANT",
        "SRC_COUNTRY",
        "SRC_GEO_CITY",
        "SRC_GEO_STATE",
        "SRC_GEO_LATITUDE",
        "SRC_GEO_LONGITUDE",
        "DST_NET_NAME",
        "DST_NET_ROLE",
        "DST_NET_SITE",
        "DST_NET_REGION",
        "DST_NET_TENANT",
        "DST_COUNTRY",
        "DST_GEO_CITY",
        "DST_GEO_STATE",
        "DST_GEO_LATITUDE",
        "DST_GEO_LONGITUDE",
        "DST_AS_PATH",
        "DST_COMMUNITIES",
        "DST_LARGE_COMMUNITIES",
        "EXPORTER_NAME",
        "EXPORTER_GROUP",
        "EXPORTER_ROLE",
        "EXPORTER_SITE",
        "EXPORTER_REGION",
        "EXPORTER_TENANT",
        "IN_IF_NAME",
        "IN_IF_DESCRIPTION",
        "IN_IF_SPEED",
        "IN_IF_PROVIDER",
        "IN_IF_CONNECTIVITY",
        "IN_IF_BOUNDARY",
        "OUT_IF_NAME",
        "OUT_IF_DESCRIPTION",
        "OUT_IF_SPEED",
        "OUT_IF_PROVIDER",
        "OUT_IF_CONNECTIVITY",
        "OUT_IF_BOUNDARY",
    ];

    for key in enrichment_keys {
        let expected = fields.get(key).map(String::as_str).unwrap_or("");
        let actual = rec_fields.get(key).map(String::as_str).unwrap_or("");
        assert_eq!(expected, actual, "mismatch for key '{key}'");
    }
}

#[test]
fn enrich_record_matches_enrich_fields_basic() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };

    let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
    let mut rec = base_record("192.0.2.10", 10, 20, 250, 10, 300);

    assert_enrich_equivalence(&cfg, &mut fields, &mut rec);
}

#[test]
fn zero_interface_speed_and_boundary_are_omitted_from_enrichment_output() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_with_zero_interface_state(),
        ..Default::default()
    };

    let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
    let mut rec = base_record("192.0.2.10", 10, 20, 250, 10, 300);

    assert_enrich_equivalence(&cfg, &mut fields, &mut rec);

    assert_eq!(fields.get("IN_IF_SPEED"), None);
    assert_eq!(fields.get("OUT_IF_SPEED"), None);
    assert_eq!(fields.get("IN_IF_BOUNDARY"), None);
    assert_eq!(fields.get("OUT_IF_BOUNDARY"), None);

    assert!(!rec.has_in_if_speed());
    assert!(!rec.has_out_if_speed());
    assert!(!rec.has_in_if_boundary());
    assert!(!rec.has_out_if_boundary());
    assert_eq!(rec.in_if_speed, 0);
    assert_eq!(rec.out_if_speed, 0);
    assert_eq!(rec.in_if_boundary, 0);
    assert_eq!(rec.out_if_boundary, 0);
}

#[test]
fn enrich_record_matches_with_sampling_override() {
    let cfg = EnrichmentConfig {
        default_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([(
            "192.0.2.0/24".to_string(),
            100_u64,
        )]))),
        override_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([
            ("192.0.2.0/24".to_string(), 500_u64),
            ("192.0.2.128/25".to_string(), 1000_u64),
        ]))),
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };

    let mut fields = base_fields("192.0.2.142", 10, 20, 0, 10, 300);
    let mut rec = base_record("192.0.2.142", 10, 20, 0, 10, 300);

    assert_enrich_equivalence(&cfg, &mut fields, &mut rec);
}

#[test]
fn enrich_record_matches_without_interface_indexes() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        ..Default::default()
    };

    let mut fields = base_fields("192.0.2.10", 0, 0, 250, 0, 0);
    let mut rec = base_record("192.0.2.10", 0, 0, 250, 0, 0);

    assert_enrich_equivalence(&cfg, &mut fields, &mut rec);
}

#[test]
fn enrich_record_matches_with_routing_and_network_attributes() {
    let cfg = EnrichmentConfig {
        metadata_static: metadata_config_for_192(),
        networks: BTreeMap::from([(
            "10.0.0.0/8".to_string(),
            NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                name: "internal".to_string(),
                role: "server".to_string(),
                site: "dc1".to_string(),
                region: "us-east".to_string(),
                tenant: "ops".to_string(),
                country: String::new(),
                state: String::new(),
                city: String::new(),
                latitude: None,
                longitude: None,
                asn: 0,
            }),
        )]),
        routing_static: StaticRoutingConfig {
            prefixes: BTreeMap::from([(
                "10.0.0.0/8".to_string(),
                StaticRoutingEntryConfig {
                    asn: 64512,
                    as_path: vec![64512, 15169],
                    communities: vec![100, 200],
                    large_communities: vec![StaticRoutingLargeCommunityConfig {
                        asn: 64512,
                        local_data1: 1,
                        local_data2: 2,
                    }],
                    next_hop: String::new(),
                    net_mask: None,
                },
            )]),
        },
        ..Default::default()
    };

    let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
    fields.insert("SRC_ADDR", "10.1.2.3".to_string());
    fields.insert("DST_ADDR", "10.4.5.6".to_string());

    let mut rec = base_record("192.0.2.10", 10, 20, 250, 10, 300);
    rec.src_addr = Some("10.1.2.3".parse().unwrap());
    rec.dst_addr = Some("10.4.5.6".parse().unwrap());

    assert_enrich_equivalence(&cfg, &mut fields, &mut rec);
}
