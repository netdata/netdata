use super::*;

fn machine() -> Uuid {
    Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}

fn boot() -> Uuid {
    Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}

fn sample_date() -> NaiveDate {
    NaiveDate::from_ymd_opt(2026, 4, 17).unwrap()
}

fn tenant() -> TenantId {
    TenantId::from("tenant1")
}

#[test]
fn sfst_key_and_date_roundtrip() {
    let id = FileId::new(machine(), boot(), 0, 42, 0);
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
    let id = FileId::new(machine(), boot(), 0, 42, 0);
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
        machine(),
        boot(),
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
