use super::*;
use uuid::Uuid;

fn machine() -> file_registry::MachineId { file_registry::MachineId::new(Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)).unwrap() }

fn instance() -> file_registry::InstanceId { file_registry::InstanceId::new(Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)).unwrap() }

fn ident() -> file_registry::Identity { file_registry::Identity::new(machine(), instance()) }

fn sample_date() -> NaiveDate {
    NaiveDate::from_ymd_opt(2026, 4, 17).unwrap()
}

fn tenant() -> TenantId {
    TenantId::from("tenant1")
}

#[test]
fn sfst_key_and_date_roundtrip() {
    let id = FileId::new(ident(), 0, 42, 0);
    let key = sfst("logs", &tenant(), sample_date(), id);
    assert!(key.starts_with("v2/logs/tenants/tenant1/sfst/2026-04-17/"));
    assert!(key.ends_with(".sfst"));
    assert_eq!(parse_sfst_date(&key), Some(sample_date()));
}

#[test]
fn sfst_prefix_has_trailing_slash() {
    assert_eq!(
        sfst_prefix("logs", &tenant(), sample_date()),
        "v2/logs/tenants/tenant1/sfst/2026-04-17/",
    );
}

#[test]
fn signal_segment_scopes_the_key() {
    // The signal segment is the top-level discriminator: two signals never
    // share a prefix, so per-signal LIST/lifecycle/IAM stays clean.
    let id = FileId::new(ident(), 0, 42, 0);
    assert!(sfst("logs", &tenant(), sample_date(), id).starts_with("v2/logs/"));
    assert!(sfst("traces", &tenant(), sample_date(), id).starts_with("v2/traces/"));
    assert!(sfst_prefix("traces", &tenant(), sample_date()).starts_with("v2/traces/"));
}

#[test]
fn catalog_key_is_versioned_signal_catalog_date_tenant() {
    let key = catalog(
        "logs",
        sample_date(),
        &tenant(),
        ident(),
        100,
        1_700_000_000,
        1_700_003_600,
    );
    assert!(key.starts_with("v2/logs/catalog/2026-04-17/tenant1/"));
    assert!(key.ends_with(".catalog"));
}

#[test]
fn parse_sfst_date_happy_path() {
    let key = "v2/logs/tenants/tenant1/sfst/2026-04-17/abc123.sfst";
    assert_eq!(parse_sfst_date(key), Some(sample_date()));
}

#[test]
fn parse_sfst_date_rejects_unknown_shapes() {
    assert!(parse_sfst_date("").is_none());
    // Missing v2/ root.
    assert!(parse_sfst_date("logs/tenants/tenant1/sfst/2026-04-17/x").is_none());
    // Wrong version (the former generation).
    assert!(parse_sfst_date("v1/logs/tenants/tenant1/sfst/2026-04-17/x").is_none());
    // Missing tenants umbrella (signal present, but no `tenants` segment).
    assert!(parse_sfst_date("v2/logs/tenant1/sfst/2026-04-17/x").is_none());
    // Old segment-less v1 layout (pre-signal-segment): rejected at the
    // version check now that the schema is v2.
    assert!(parse_sfst_date("v1/tenants/tenant1/sfst/2026-04-17/x.sfst").is_none());
    // Catalog key shape (not an SFST key).
    assert!(parse_sfst_date("v2/logs/catalog/2026-04-17/tenant1/x").is_none());
    // Truncated.
    assert!(parse_sfst_date("v2/logs/tenants/tenant1/sfst").is_none());
    // Date doesn't parse.
    assert!(parse_sfst_date("v2/logs/tenants/tenant1/sfst/not-a-date/x").is_none());
}

#[test]
fn parse_catalog_key_shape_table() {
    let valid = catalog("logs", sample_date(), &tenant(), ident(), 42, 100, 200);
    // (description, key, should_parse)
    let cases: &[(&str, String, bool)] = &[
        ("valid", valid.clone(), true),
        ("wrong extension (.sfst)", valid.replace(".catalog", ".sfst"), false),
        ("no extension", valid.replace(".catalog", ""), false),
        (
            "too few segments",
            "v2/logs/catalog/2026-04-17/tenant1".to_owned(),
            false,
        ),
        (
            "too many segments",
            format!("{valid}/extra"),
            false,
        ),
        (
            "bad tenant charset",
            valid.replace("/tenant1/", "/bad!tenant/"),
            false,
        ),
        ("traversal (..) tenant", valid.replace("/tenant1/", "/../"), false),
        ("dot (.) tenant", valid.replace("/tenant1/", "/./"), false),
        // The auth-off "default" tenant is a legitimate stored segment (this is
        // the auth-off restore blackout regression: it MUST parse).
        (
            "default tenant",
            catalog("logs", sample_date(), &TenantId::default_tenant(), ident(), 42, 100, 200),
            true,
        ),
        ("bad date", valid.replace("2026-04-17", "2026-13-99"), false),
        ("wrong schema version", valid.replace("v2/", "v1/"), false),
        (
            "wrong signal segment",
            catalog("traces", sample_date(), &tenant(), ident(), 42, 100, 200),
            false,
        ),
        (
            "an SFST key",
            sfst("logs", &tenant(), sample_date(), FileId::new(ident(), 0, 5, 0)),
            false,
        ),
    ];
    for (desc, key, expected) in cases {
        assert_eq!(
            parse_catalog_key(key, "logs").is_some(),
            *expected,
            "parse_catalog_key({desc}): {key}"
        );
    }
    // The valid one recovers the fields.
    let p = parse_catalog_key(&valid, "logs").unwrap();
    assert_eq!((p.tenant_id, p.identity, p.max_seq), (tenant(), ident(), 42));
}

#[test]
fn parse_sfst_key_shape_table() {
    let id = FileId::new(ident(), 0, 5, 7);
    let valid = sfst("logs", &tenant(), sample_date(), id);
    let cases: &[(&str, String, bool)] = &[
        ("valid", valid.clone(), true),
        ("wrong extension (.catalog)", valid.replace(".sfst", ".catalog"), false),
        ("no extension", valid.replace(".sfst", ""), false),
        (
            "too few segments",
            "v2/logs/tenants/tenant1/sfst/2026-04-17".to_owned(),
            false,
        ),
        ("bad tenant charset", valid.replace("/tenant1/", "/bad!tenant/"), false),
        ("traversal (..) tenant", valid.replace("/tenant1/", "/../"), false),
        (
            "default tenant",
            sfst("logs", &TenantId::default_tenant(), sample_date(), id),
            true,
        ),
        ("bad date", valid.replace("2026-04-17", "not-a-date"), false),
        ("wrong schema version", valid.replace("v2/", "v1/"), false),
        ("wrong signal segment", valid.replace("/logs/", "/traces/"), false),
        (
            "a catalog key",
            catalog("logs", sample_date(), &tenant(), ident(), 42, 100, 200),
            false,
        ),
    ];
    for (desc, key, expected) in cases {
        assert_eq!(
            parse_sfst_key(key, "logs").is_some(),
            *expected,
            "parse_sfst_key({desc}): {key}"
        );
    }
    let (parsed_id, parsed_tenant) = parse_sfst_key(&valid, "logs").unwrap();
    assert_eq!((parsed_id, parsed_tenant), (id, tenant()));
    // Nil-identity filenames are rejected by otel_catalog::parse_stem /
    // FileId::parse (covered by those crates' own tests), so a nil-UUID key
    // fails here too — not re-tested in this shape table.
}
