//! Router-level tests over a stub (empty) source supplier: routes,
//! status codes, content types, error mapping, and the empty-corpus
//! response bodies. Real-data end-to-end runs against a live agent +
//! Grafana (SOW step 5); everything wire-shaped is already pinned by
//! the parser/json/pb golden tests.

use axum::body::Body;
use axum::http::{Request, StatusCode};
use prost::Message;
use sfsq::traces::TraceSource;
use tower::ServiceExt;

use super::{SourceSupplier, router};
use crate::pb;

struct Empty;

#[async_trait::async_trait]
impl SourceSupplier for Empty {
    async fn snapshot(&self) -> Result<Vec<TraceSource>, String> {
        Ok(Vec::new())
    }
    async fn snapshot_pair(&self) -> Result<(Vec<TraceSource>, Vec<TraceSource>), String> {
        Ok((Vec::new(), Vec::new()))
    }
}

struct Failing;

#[async_trait::async_trait]
impl SourceSupplier for Failing {
    async fn snapshot(&self) -> Result<Vec<TraceSource>, String> {
        Err("registry unavailable".to_string())
    }
    async fn snapshot_pair(&self) -> Result<(Vec<TraceSource>, Vec<TraceSource>), String> {
        Err("registry unavailable".to_string())
    }
}

async fn get(app: &axum::Router, uri: &str) -> (StatusCode, axum::http::HeaderMap, Vec<u8>) {
    let resp = app
        .clone()
        .oneshot(Request::builder().uri(uri).body(Body::empty()).unwrap())
        .await
        .unwrap();
    let status = resp.status();
    let headers = resp.headers().clone();
    let body = axum::body::to_bytes(resp.into_body(), 1 << 20).await.unwrap();
    (status, headers, body.to_vec())
}

#[tokio::test]
async fn routes_and_empty_bodies() {
    let app = router(std::sync::Arc::new(Empty));

    let (status, _, body) = get(&app, "/api/echo").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body, b"echo");

    // Search: absent q = match-all; empty corpus = jsonpb empty.
    let (status, headers, body) = get(&app, "/api/search").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "application/json");
    assert!(!headers.contains_key("x-netdata-partial"));
    assert_eq!(body, br#"{"metrics":{}}"#);

    // With the full parameter set the plugin sends.
    let (status, _, body) =
        get(&app, "/api/search?q=%7B%7D&limit=20&spss=3&start=100&end=200").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body, br#"{"metrics":{}}"#);

    // Tags v2 (scope param tolerated and ignored). Even an empty
    // corpus advertises the full intrinsic set (engine decision 18B:
    // intrinsics are capabilities, not data properties).
    let (status, _, body) = get(&app, "/api/v2/search/tags?scope=none&limit=100").await;
    assert_eq!(status, StatusCode::OK);
    let body = String::from_utf8(body).unwrap();
    assert!(body.starts_with(r#"{"scopes":[{"name":"intrinsic","tags":["#), "{body}");
    assert!(body.contains(r#""traceDuration""#), "{body}");

    // Tag values v2: scoped tag with dots; the proxy's extra `tag=`
    // param is tolerated.
    let (status, _, body) =
        get(&app, "/api/v2/search/tag/resource.service.name/values?tag=service.name").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(body, br#"{"metrics":{}}"#);

    // Unscoped and intrinsic tags resolve too.
    let (status, _, _) = get(&app, "/api/v2/search/tag/.foo/values").await;
    assert_eq!(status, StatusCode::OK);
    let (status, _, _) = get(&app, "/api/v2/search/tag/name/values").await;
    assert_eq!(status, StatusCode::OK);
    let (status, _, _) = get(&app, "/api/v2/search/tag/status/values").await;
    assert_eq!(status, StatusCode::OK);

    // v2 by-id: missing trace = 200 with an empty protobuf trace.
    let (status, headers, body) =
        get(&app, "/api/v2/traces/0af7651916cd43dd8448eb211c80319c").await;
    assert_eq!(status, StatusCode::OK);
    assert_eq!(headers["content-type"], "application/protobuf");
    let resp = pb::TraceByIdResponse::decode(body.as_slice()).unwrap();
    assert_eq!(resp.status, pb::PartialStatus::Complete as i32);
    assert!(resp.trace.unwrap().resource_spans.is_empty());

    // v1 by-id: missing trace = 404.
    let (status, _, _) = get(&app, "/api/traces/0af7651916cd43dd8448eb211c80319c").await;
    assert_eq!(status, StatusCode::NOT_FOUND);

    // Unrouted paths (incl. the unimplemented metrics namespace).
    let (status, _, _) = get(&app, "/api/metrics/query_range?q=x").await;
    assert_eq!(status, StatusCode::NOT_FOUND);
    let (status, _, _) = get(&app, "/api/v2/search/tag/name/nonsense").await;
    assert_eq!(status, StatusCode::NOT_FOUND);
}

#[tokio::test]
async fn request_errors_are_400() {
    let app = router(std::sync::Arc::new(Empty));

    // Out-of-grammar TraceQL (%7B%7D%20%7C%20count() = "{} | count()").
    let (status, _, body) = get(&app, "/api/search?q=%7B%7D%20%7C%20count()").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert!(String::from_utf8_lossy(&body).contains("unsupported TraceQL"));

    // Engine request validation surfaces with its own text.
    let (status, _, body) = get(&app, "/api/search?limit=0").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert!(String::from_utf8_lossy(&body).contains("zero limit"));
    let (status, _, _) = get(&app, "/api/search?limit=nope").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    let (status, _, body) = get(&app, "/api/search?start=200&end=100").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert!(String::from_utf8_lossy(&body).contains("start must be before end"));

    // Malformed by-id path ids.
    let (status, _, _) = get(&app, "/api/v2/traces/xyz").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    let (status, _, _) = get(&app, "/api/v2/traces/00000000000000000000000000000000").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);

    // A virtual intrinsic has no value dictionary (engine error text).
    let (status, _, body) = get(&app, "/api/v2/search/tag/trace:id/values").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
    assert!(String::from_utf8_lossy(&body).contains("virtual"));

    // An unknown scope word is a bad tag.
    let (status, _, _) = get(&app, "/api/v2/search/tag/span:bogus/values").await;
    assert_eq!(status, StatusCode::BAD_REQUEST);
}

#[test]
fn unscoped_tag_values_merge_is_sorted_capped_and_exact() {
    use sfsq::traces::{QueryStatus, TagValue, TagValuesData};
    let val = |s: &str| TagValue { value: s.into(), kind: None };
    let a = TagValuesData {
        values: vec![val("b"), val("z")],
        truncated: false,
        status: QueryStatus::Complete,
    };
    let b = TagValuesData {
        values: vec![val("a"), val("b")],
        truncated: false,
        status: QueryStatus::Complete,
    };
    // Sorted union {a, b, z} capped at 2 → {a, b}, truncated becomes
    // true because the union overflowed even though neither scope did.
    let merged = super::merge_tag_values(a, b, Some(2));
    assert_eq!(
        merged.values.iter().map(|v| v.value.as_str()).collect::<Vec<_>>(),
        vec!["a", "b"]
    );
    assert!(merged.truncated);

    // No cap: full sorted dedup, not truncated.
    let a = TagValuesData {
        values: vec![val("b"), val("z")],
        truncated: false,
        status: QueryStatus::Complete,
    };
    let b = TagValuesData {
        values: vec![val("a"), val("b")],
        truncated: false,
        status: QueryStatus::Complete,
    };
    let merged = super::merge_tag_values(a, b, None);
    assert_eq!(
        merged.values.iter().map(|v| v.value.as_str()).collect::<Vec<_>>(),
        vec!["a", "b", "z"]
    );
    assert!(!merged.truncated);
}

#[tokio::test]
async fn supplier_failure_is_500() {
    let app = router(std::sync::Arc::new(Failing));
    for uri in [
        "/api/search",
        "/api/v2/traces/0af7651916cd43dd8448eb211c80319c",
        "/api/v2/search/tags",
        "/api/v2/search/tag/name/values",
    ] {
        let (status, _, _) = get(&app, uri).await;
        assert_eq!(status, StatusCode::INTERNAL_SERVER_ERROR, "{uri}");
    }
}
