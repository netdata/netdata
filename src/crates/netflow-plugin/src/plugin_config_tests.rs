use super::*;
use clap::Parser;
use tempfile::tempdir;

#[test]
fn dynamic_bmp_decode_error_threshold_defaults_to_8() {
    let cfg = PluginConfig::default();
    assert_eq!(
        cfg.enrichment
            .routing_dynamic
            .bmp
            .max_consecutive_decode_errors,
        8
    );
}

#[test]
fn validate_rejects_zero_bmp_decode_error_threshold_when_enabled() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.routing_dynamic.bmp.enabled = true;
    cfg.enrichment
        .routing_dynamic
        .bmp
        .max_consecutive_decode_errors = 0;

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("max_consecutive_decode_errors must be greater than 0")
    );
}

#[test]
fn validate_rejects_enabled_bioris_without_instances() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.routing_dynamic.bioris.enabled = true;

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("ris_instances must contain at least one")
    );
}

#[test]
fn validate_accepts_enabled_bioris_with_instance() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.routing_dynamic.bioris.enabled = true;
    cfg.enrichment.routing_dynamic.bioris.ris_instances.push(
        RoutingDynamicBiorisRisInstanceConfig {
            grpc_addr: "127.0.0.1:50051".to_string(),
            grpc_secure: false,
            vrf_id: 0,
            vrf: String::new(),
        },
    );

    cfg.validate().expect("configuration should be valid");
}

#[test]
fn validate_rejects_bioris_instance_without_port_or_scheme() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.routing_dynamic.bioris.enabled = true;
    cfg.enrichment.routing_dynamic.bioris.ris_instances.push(
        RoutingDynamicBiorisRisInstanceConfig {
            grpc_addr: "ris.example.internal".to_string(),
            grpc_secure: false,
            vrf_id: 0,
            vrf: String::new(),
        },
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("grpc_addr must include host:port or an explicit URI scheme")
    );
}

#[test]
fn validate_rejects_zero_bioris_timeouts_when_enabled() {
    for field in ["timeout", "refresh", "refresh_timeout"] {
        let mut cfg = PluginConfig::default();
        cfg.enrichment.routing_dynamic.bioris.enabled = true;
        cfg.enrichment.routing_dynamic.bioris.ris_instances.push(
            RoutingDynamicBiorisRisInstanceConfig {
                grpc_addr: "127.0.0.1:50051".to_string(),
                grpc_secure: false,
                vrf_id: 0,
                vrf: String::new(),
            },
        );
        match field {
            "timeout" => cfg.enrichment.routing_dynamic.bioris.timeout = Duration::ZERO,
            "refresh" => cfg.enrichment.routing_dynamic.bioris.refresh = Duration::ZERO,
            "refresh_timeout" => {
                cfg.enrichment.routing_dynamic.bioris.refresh_timeout = Duration::ZERO;
            }
            _ => unreachable!(),
        }

        let err = cfg.validate().expect_err("expected validation error");
        assert!(
            err.to_string().contains(&format!(
                "enrichment.routing_dynamic.bioris.{field} must be greater than 0"
            )),
            "unexpected error for {field}: {err:#}"
        );
    }
}

#[test]
fn validate_rejects_network_source_verify_false() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.network_sources.insert(
        "source".to_string(),
        RemoteNetworkSourceConfig {
            url: "https://example.com/source.json".to_string(),
            tls: RemoteNetworkSourceTlsConfig {
                enable: true,
                verify: false,
                skip_verify: false,
                ..Default::default()
            },
            ..Default::default()
        },
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(err.to_string().contains("tls.verify must remain true"));
}

#[test]
fn validate_rejects_network_source_skip_verify() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.network_sources.insert(
        "source".to_string(),
        RemoteNetworkSourceConfig {
            url: "https://example.com/source.json".to_string(),
            tls: RemoteNetworkSourceTlsConfig {
                enable: true,
                verify: true,
                skip_verify: true,
                ..Default::default()
            },
            ..Default::default()
        },
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(err.to_string().contains("tls.skip_verify is not supported"));
}

#[test]
fn validate_rejects_network_source_explicit_insecure_tls_opt_out() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.network_sources.insert(
        "source".to_string(),
        RemoteNetworkSourceConfig {
            url: "https://example.com/source.json".to_string(),
            tls: RemoteNetworkSourceTlsConfig {
                enable: true,
                verify: false,
                skip_verify: true,
                ..Default::default()
            },
            ..Default::default()
        },
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(err.to_string().contains("tls.verify must remain true"));
}

#[test]
fn validate_rejects_static_network_coordinate_without_pair() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.networks.insert(
        "198.51.100.0/24".to_string(),
        NetworkAttributesValue::Attributes(NetworkAttributesConfig {
            city: "Paris".to_string(),
            latitude: Some(48.8566),
            ..Default::default()
        }),
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("must set both latitude and longitude")
    );
}

#[test]
fn validate_rejects_static_network_coordinate_out_of_range() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.networks.insert(
        "198.51.100.0/24".to_string(),
        NetworkAttributesValue::Attributes(NetworkAttributesConfig {
            city: "Paris".to_string(),
            latitude: Some(120.0),
            longitude: Some(2.3522),
            ..Default::default()
        }),
    );

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains(".latitude must be a finite value between -90 and 90")
    );
}

#[test]
fn validate_accepts_static_network_coordinates() {
    let mut cfg = PluginConfig::default();
    cfg.enrichment.networks.insert(
        "198.51.100.0/24".to_string(),
        NetworkAttributesValue::Attributes(NetworkAttributesConfig {
            city: "Paris".to_string(),
            latitude: Some(48.8566),
            longitude: Some(2.3522),
            ..Default::default()
        }),
    );

    cfg.validate().expect("configuration should be valid");
}

#[test]
fn validate_rejects_zero_query_max_groups() {
    let mut cfg = PluginConfig::default();
    cfg.journal.query_max_groups = 0;

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("journal.query_max_groups must be greater than 0")
    );
}

#[test]
fn plugin_enabled_defaults_to_true() {
    let cfg = PluginConfig::default();
    assert!(cfg.enabled);
}

#[test]
fn deserialized_empty_enrichment_section_keeps_provider_defaults() {
    let cfg: EnrichmentConfig = serde_yaml::from_str("{}").expect("parse enrichment config");

    assert_eq!(
        cfg.asn_providers,
        vec![
            AsnProviderConfig::Flow,
            AsnProviderConfig::Routing,
            AsnProviderConfig::Geoip,
        ]
    );
    assert_eq!(
        cfg.net_providers,
        vec![NetProviderConfig::Flow, NetProviderConfig::Routing]
    );
}

#[test]
fn yaml_can_disable_plugin() {
    let yaml = r#"
enabled: false
listener:
  listen: "127.0.0.1:2055"
  max_packet_size: 9216
  sync_every_entries: 1024
  sync_interval: 1s
protocols:
  v5: true
  v7: true
  v9: true
  ipfix: true
  sflow: true
  decapsulation_mode: none
  timestamp_source: input
journal:
  journal_dir: flows
  query_max_groups: 50000
"#;

    let cfg: PluginConfig = serde_yaml::from_str(yaml).expect("yaml should parse");
    assert!(!cfg.enabled);
}

#[test]
fn stock_netflow_yaml_parses_and_validates() {
    let yaml = include_str!("../configs/netflow.yaml");
    let cfg: PluginConfig = serde_yaml::from_str(yaml).expect("stock netflow.yaml should parse");

    cfg.validate().expect("stock netflow.yaml should validate");
}

#[test]
fn journal_tier_retention_uses_built_in_tier_defaults() {
    let cfg = PluginConfig::default();

    let raw = cfg.journal.retention_for_tier(TierKind::Raw);
    let minute_1 = cfg.journal.retention_for_tier(TierKind::Minute1);
    let minute_5 = cfg.journal.retention_for_tier(TierKind::Minute5);
    let hour_1 = cfg.journal.retention_for_tier(TierKind::Hour1);

    for retention in [raw, minute_1, minute_5, hour_1] {
        assert_eq!(
            retention.size_of_journal_files.unwrap().as_u64(),
            ByteSize::gb(10).as_u64()
        );
        assert_eq!(
            retention.duration_of_journal_files.unwrap(),
            Duration::from_secs(7 * 24 * 60 * 60)
        );
    }
}

#[test]
fn journal_tier_retention_uses_per_tier_values_when_present() {
    let mut cfg = PluginConfig::default();
    cfg.journal.tiers.raw = JournalTierRetentionConfig {
        size_of_journal_files: Some(ByteSize::gb(2)),
        duration_of_journal_files: Some(Duration::from_secs(2 * 24 * 60 * 60)),
    };

    let raw = cfg.journal.retention_for_tier(TierKind::Raw);
    assert_eq!(
        raw.size_of_journal_files.unwrap().as_u64(),
        ByteSize::gb(2).as_u64()
    );
    assert_eq!(
        raw.duration_of_journal_files.unwrap(),
        Duration::from_secs(2 * 24 * 60 * 60)
    );

    // Other tiers untouched -- still at the built-in defaults.
    let minute_1 = cfg.journal.retention_for_tier(TierKind::Minute1);
    assert_eq!(
        minute_1.size_of_journal_files.unwrap().as_u64(),
        ByteSize::gb(10).as_u64()
    );
    assert_eq!(
        minute_1.duration_of_journal_files.unwrap(),
        Duration::from_secs(7 * 24 * 60 * 60)
    );
}

#[test]
fn journal_cli_retention_aliases_apply_to_all_tiers() {
    let cfg = PluginConfig::try_parse_from([
        "netflow-plugin",
        "--netflow-retention-size-of-journal-files",
        "20GB",
        "--netflow-retention-duration-of-journal-files",
        "2d",
    ])
    .expect("CLI config should parse");

    for tier in [
        TierKind::Raw,
        TierKind::Minute1,
        TierKind::Minute5,
        TierKind::Hour1,
    ] {
        let retention = cfg.journal.retention_for_tier(tier);
        assert_eq!(
            retention.size_of_journal_files.unwrap().as_u64(),
            ByteSize::gb(20).as_u64(),
            "unexpected size retention for {tier:?}"
        );
        assert_eq!(
            retention.duration_of_journal_files.unwrap(),
            Duration::from_secs(2 * 24 * 60 * 60),
            "unexpected duration retention for {tier:?}"
        );
    }
}

#[test]
fn journal_rotation_size_derives_from_tier_size_budget() {
    let mut cfg = PluginConfig::default();
    cfg.journal.tiers.raw.size_of_journal_files = Some(ByteSize::gb(1));

    assert_eq!(
        cfg.journal.rotation_size_for_tier(TierKind::Raw),
        ByteSize::mb(50).as_u64()
    );

    cfg.journal.tiers.raw.size_of_journal_files = Some(ByteSize::gb(4));
    assert_eq!(
        cfg.journal.rotation_size_for_tier(TierKind::Raw),
        ByteSize::mb(200).as_u64()
    );

    cfg.journal.tiers.raw.size_of_journal_files = Some(ByteSize::gb(10));
    assert_eq!(
        cfg.journal.rotation_size_for_tier(TierKind::Raw),
        ByteSize::mb(200).as_u64()
    );
}

#[test]
fn journal_rotation_size_uses_100mb_for_time_only_retention() {
    let mut cfg = PluginConfig::default();
    cfg.journal.tiers.raw.size_of_journal_files = None;
    cfg.journal.tiers.raw.duration_of_journal_files = Some(Duration::from_secs(7 * 24 * 60 * 60));

    assert_eq!(
        cfg.journal.rotation_size_for_tier(TierKind::Raw),
        ByteSize::mb(100).as_u64()
    );
}

#[test]
fn journal_validation_rejects_tier_size_below_100mb() {
    let mut cfg = PluginConfig::default();
    cfg.journal.tiers.raw.size_of_journal_files = Some(ByteSize::mb(99));

    let err = cfg.validate().expect_err("expected validation error");
    assert!(
        err.to_string()
            .contains("journal.tiers.raw.size_of_journal_files must be at least 100MB"),
        "got: {err}"
    );
}

#[test]
fn journal_validation_allows_time_only_retention_when_size_is_disabled() {
    let mut cfg = PluginConfig::default();
    for tier_cfg in [
        &mut cfg.journal.tiers.raw,
        &mut cfg.journal.tiers.minute_1,
        &mut cfg.journal.tiers.minute_5,
        &mut cfg.journal.tiers.hour_1,
    ] {
        tier_cfg.size_of_journal_files = None;
        tier_cfg.duration_of_journal_files = Some(Duration::from_secs(7 * 24 * 60 * 60));
    }

    cfg.validate().expect("time-only retention should be valid");
}

#[test]
fn journal_tier_retention_null_disables_size_limit_for_that_tier_only() {
    let yaml = r#"
journal_dir: flows
query_max_groups: 50000
tiers:
  raw:
    size_of_journal_files: null
    duration_of_journal_files: 24h
"#;

    let cfg: JournalConfig = serde_yaml::from_str(yaml).expect("journal config should parse");
    let raw = cfg.retention_for_tier(TierKind::Raw);
    let minute_1 = cfg.retention_for_tier(TierKind::Minute1);

    assert_eq!(raw.size_of_journal_files, None);
    assert_eq!(
        raw.duration_of_journal_files,
        Some(Duration::from_secs(24 * 60 * 60))
    );
    // Other tiers still at the built-in 10GB / 7d defaults.
    assert_eq!(minute_1.size_of_journal_files, Some(ByteSize::gb(10)));
    assert_eq!(
        minute_1.duration_of_journal_files,
        Some(Duration::from_secs(7 * 24 * 60 * 60))
    );
}

#[test]
fn auto_detect_geoip_databases_uses_absolute_journal_parent() {
    let dir = tempdir().expect("create tempdir");
    let cache_dir = dir.path();
    let intel_dir = cache_dir.join(TOPOLOGY_IP_INTEL_DIR);
    fs::create_dir_all(&intel_dir).expect("create intel dir");
    fs::write(intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"asn").expect("write asn db");
    fs::write(intel_dir.join(TOPOLOGY_IP_GEO_MMDB), b"geo").expect("write geo db");

    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = cache_dir.join("flows").to_string_lossy().to_string();

    cfg.auto_detect_geoip_databases();

    assert_eq!(
        cfg.enrichment.geoip.asn_database,
        vec![
            intel_dir
                .join(TOPOLOGY_IP_ASN_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert_eq!(
        cfg.enrichment.geoip.geo_database,
        vec![
            intel_dir
                .join(TOPOLOGY_IP_GEO_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert!(cfg.enrichment.geoip.optional);
}

#[test]
fn auto_detect_geoip_databases_uses_netdata_cache_dir_for_relative_journal_dir() {
    let dir = tempdir().expect("create tempdir");
    let cache_dir = dir.path();
    let stock_data_dir = dir.path().join("share");
    let intel_dir = cache_dir.join(TOPOLOGY_IP_INTEL_DIR);
    fs::create_dir_all(&intel_dir).expect("create intel dir");
    fs::create_dir_all(&stock_data_dir).expect("create stock data dir");
    fs::write(intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"asn").expect("write asn db");

    let mut cfg = PluginConfig::default();
    cfg._netdata_env.cache_dir = Some(cache_dir.to_path_buf());
    cfg._netdata_env.stock_data_dir = Some(stock_data_dir);
    cfg.journal.journal_dir = "flows".to_string();

    cfg.auto_detect_geoip_databases();

    assert_eq!(
        cfg.enrichment.geoip.asn_database,
        vec![
            intel_dir
                .join(TOPOLOGY_IP_ASN_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert!(cfg.enrichment.geoip.geo_database.is_empty());
    assert!(cfg.enrichment.geoip.optional);
}

#[test]
fn auto_detect_geoip_databases_does_not_override_explicit_config() {
    let dir = tempdir().expect("create tempdir");
    let cache_dir = dir.path();
    let intel_dir = cache_dir.join(TOPOLOGY_IP_INTEL_DIR);
    fs::create_dir_all(&intel_dir).expect("create intel dir");
    fs::write(intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"asn").expect("write asn db");
    fs::write(intel_dir.join(TOPOLOGY_IP_GEO_MMDB), b"geo").expect("write geo db");

    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = cache_dir.join("flows").to_string_lossy().to_string();
    cfg.enrichment.geoip.asn_database = vec!["/custom/asn.mmdb".to_string()];
    cfg.enrichment.geoip.geo_database = vec!["/custom/geo.mmdb".to_string()];

    cfg.auto_detect_geoip_databases();

    assert_eq!(cfg.enrichment.geoip.asn_database, vec!["/custom/asn.mmdb"]);
    assert_eq!(cfg.enrichment.geoip.geo_database, vec!["/custom/geo.mmdb"]);
}

#[test]
fn auto_detect_geoip_databases_falls_back_to_stock_data_dir() {
    let dir = tempdir().expect("create tempdir");
    let cache_dir = dir.path().join("cache");
    let stock_data_dir = dir.path().join("share");
    fs::create_dir_all(&cache_dir).expect("create cache dir");
    let intel_dir = stock_data_dir.join(TOPOLOGY_IP_INTEL_DIR);
    fs::create_dir_all(&intel_dir).expect("create intel dir");
    fs::write(intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"asn").expect("write asn db");
    fs::write(intel_dir.join(TOPOLOGY_IP_GEO_MMDB), b"geo").expect("write geo db");

    let mut cfg = PluginConfig::default();
    cfg._netdata_env.cache_dir = Some(cache_dir);
    cfg._netdata_env.stock_data_dir = Some(stock_data_dir.clone());
    cfg.journal.journal_dir = "flows".to_string();

    cfg.auto_detect_geoip_databases();

    assert_eq!(
        cfg.enrichment.geoip.asn_database,
        vec![
            intel_dir
                .join(TOPOLOGY_IP_ASN_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert_eq!(
        cfg.enrichment.geoip.geo_database,
        vec![
            intel_dir
                .join(TOPOLOGY_IP_GEO_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert!(cfg.enrichment.geoip.optional);
}

#[test]
fn auto_detect_geoip_databases_prefers_cache_over_stock_data_dir() {
    let dir = tempdir().expect("create tempdir");
    let cache_dir = dir.path().join("cache");
    let stock_data_dir = dir.path().join("share");
    let cache_intel_dir = cache_dir.join(TOPOLOGY_IP_INTEL_DIR);
    let stock_intel_dir = stock_data_dir.join(TOPOLOGY_IP_INTEL_DIR);
    fs::create_dir_all(&cache_intel_dir).expect("create cache intel dir");
    fs::create_dir_all(&stock_intel_dir).expect("create stock intel dir");

    fs::write(cache_intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"cache-asn")
        .expect("write cache asn db");
    fs::write(cache_intel_dir.join(TOPOLOGY_IP_GEO_MMDB), b"cache-geo")
        .expect("write cache geo db");
    fs::write(stock_intel_dir.join(TOPOLOGY_IP_ASN_MMDB), b"stock-asn")
        .expect("write stock asn db");
    fs::write(stock_intel_dir.join(TOPOLOGY_IP_GEO_MMDB), b"stock-geo")
        .expect("write stock geo db");

    let mut cfg = PluginConfig::default();
    cfg._netdata_env.cache_dir = Some(cache_dir.clone());
    cfg._netdata_env.stock_data_dir = Some(stock_data_dir.clone());
    cfg.journal.journal_dir = "flows".to_string();

    cfg.auto_detect_geoip_databases();

    assert_eq!(
        cfg.enrichment.geoip.asn_database,
        vec![
            cache_intel_dir
                .join(TOPOLOGY_IP_ASN_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
    assert_eq!(
        cfg.enrichment.geoip.geo_database,
        vec![
            cache_intel_dir
                .join(TOPOLOGY_IP_GEO_MMDB)
                .to_string_lossy()
                .to_string()
        ]
    );
}
