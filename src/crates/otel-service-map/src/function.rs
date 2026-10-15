//! The `topology:tracing` Function.
//!
//! A raw handler rather than a typed `HandlerAdapter` one: topology errors
//! answer `{"status", "errorMessage"}` with statuses the adapter cannot map
//! (413 for an answer over the byte budget).

use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use bridge::function::{FunctionContext, RawFunctionHandler};
use netdata_plugin_types::{FunctionDeclaration, FunctionResult, HttpAccess};
use serde::Deserialize;
use serde_json::{Value, json};

use crate::payload;
use crate::service::ServiceMap;

pub const FUNCTION_NAME: &str = "topology:tracing";
const HELP: &str = "Service map from OpenTelemetry traces: instrumented services, the services and uninstrumented \
                    peers they call, and request, error and latency statistics per call direction.";
const DEFAULT_WINDOW_S: u64 = 15 * 60;
const MAX_WINDOW_S: u64 = 31 * 24 * 3600;
const MAX_FUTURE_S: i64 = 24 * 3600;
/// Far below the 16 MiB IPC message limit and the agent's 100 MiB plugin
/// result limit, which stops the whole plugin.
const MAX_ANSWER_BYTES: usize = 8 * 1024 * 1024;
const DEFAULT_TENANT: &str = "default";

pub struct TopologyFunction {
    map: Arc<ServiceMap>,
}

impl TopologyFunction {
    pub fn new(map: Arc<ServiceMap>) -> Self {
        Self { map }
    }
}

/// The POST body. The generic topology UI also sends `selections`,
/// `timeout`, `last` and more, so unknown fields are ignored.
#[derive(Debug, Default, Deserialize)]
struct Request {
    info: Option<Value>,
    after: Option<i64>,
    before: Option<i64>,
    tenant: Option<String>,
}

#[derive(Debug, PartialEq)]
enum Parsed {
    Info,
    Window {
        tenant: String,
        after_s: u64,
        before_s: u64,
    },
}

fn parse(args: &[String], payload: Option<&[u8]>, now_s: u64) -> Result<Parsed, String> {
    let mut request = match payload {
        Some(body) if !body.iter().all(u8::is_ascii_whitespace) => {
            serde_json::from_slice::<Request>(body)
                .map_err(|e| format!("invalid request body: {e}"))?
        }
        _ => Request::default(),
    };
    // GET callers pass `info` and `key:value` words instead of a body; the
    // UI's info call also carries a window, which info ignores.
    for arg in args {
        match arg.split_once(':') {
            None if arg == "info" => request.info = Some(Value::Bool(true)),
            Some(("after", v)) if request.after.is_none() => request.after = v.parse().ok(),
            Some(("before", v)) if request.before.is_none() => request.before = v.parse().ok(),
            Some(("tenant", v)) if request.tenant.is_none() => request.tenant = Some(v.to_string()),
            _ => {}
        }
    }
    if request
        .info
        .as_ref()
        .is_some_and(|v| !matches!(v, Value::Bool(false) | Value::Null))
    {
        return Ok(Parsed::Info);
    }

    // Netdata time conventions: absent or 0 means "now" for `before` and
    // "15 minutes before `before`" for `after`; a negative value is relative
    // (to now for `before`, to `before` for `after`).
    let now = now_s as i64;
    let out_of_range = || {
        "after/before are out of range: use unix seconds or negative relative seconds".to_string()
    };
    let before = match request.before {
        None | Some(0) => Some(now),
        Some(b) if b < 0 => now.checked_add(b),
        Some(b) => Some(b),
    }
    .ok_or_else(out_of_range)?;
    let after = match request.after {
        None | Some(0) => before.checked_sub(DEFAULT_WINDOW_S as i64),
        Some(a) if a < 0 => before.checked_add(a),
        Some(a) => Some(a),
    }
    .ok_or_else(out_of_range)?;
    // A millisecond timestamp passed as seconds lands thousands of years ahead.
    if before > now.saturating_add(MAX_FUTURE_S) {
        return Err(format!(
            "before ({before}) is more than a day in the future; times are unix seconds"
        ));
    }
    if after < 0 || before <= after {
        return Err(format!("after ({after}) must be before before ({before})"));
    }
    if (before - after) as u64 > MAX_WINDOW_S {
        return Err(format!(
            "the window is longer than {} days",
            MAX_WINDOW_S / 86_400
        ));
    }
    let tenant = request
        .tenant
        .filter(|t| !t.is_empty())
        .unwrap_or_else(|| DEFAULT_TENANT.to_string());
    Ok(Parsed::Window {
        tenant,
        after_s: after as u64,
        before_s: before as u64,
    })
}

fn error(transaction: String, status: u32, message: &str) -> FunctionResult {
    FunctionResult {
        transaction,
        status,
        format: "application/json".to_string(),
        expires: 0,
        payload: serde_json::to_vec(&json!({"status": status, "errorMessage": message}))
            .unwrap_or_default(),
    }
}

fn ok(transaction: String, body: &Value) -> FunctionResult {
    let payload = serde_json::to_vec(body).unwrap_or_default();
    if payload.len() > MAX_ANSWER_BYTES {
        return error(
            transaction,
            413,
            &format!(
                "the service map is {} bytes, over the {} byte limit; ask for a shorter window",
                payload.len(),
                MAX_ANSWER_BYTES
            ),
        );
    }
    let expires = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_secs() + 2);
    FunctionResult {
        transaction,
        status: 200,
        format: "application/json".to_string(),
        expires,
        payload,
    }
}

#[async_trait]
impl RawFunctionHandler for TopologyFunction {
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult {
        let call = &ctx.function_call;
        let transaction = call.transaction.clone();
        let now = SystemTime::now();
        let now_s = now
            .duration_since(UNIX_EPOCH)
            .unwrap_or(Duration::ZERO)
            .as_secs();
        match parse(&call.args, call.payload.as_deref(), now_s) {
            Err(message) => error(transaction, 400, &message),
            Ok(Parsed::Info) => ok(transaction, &payload::info()),
            Ok(Parsed::Window {
                tenant,
                after_s,
                before_s,
            }) => {
                let view = self.map.view(&tenant, after_s, before_s).await;
                ok(
                    transaction,
                    &payload::build(&view, &tenant, after_s, before_s, now),
                )
            }
        }
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut d = FunctionDeclaration::new(FUNCTION_NAME, HELP);
        d.global = true;
        d.tags = Some("top".to_string());
        d.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        d
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const NOW: u64 = 1_000_000;

    fn window(tenant: &str, after_s: u64, before_s: u64) -> Parsed {
        Parsed::Window {
            tenant: tenant.to_string(),
            after_s,
            before_s,
        }
    }

    fn args(words: &[&str]) -> Vec<String> {
        words.iter().map(|w| w.to_string()).collect()
    }

    #[test]
    fn requests() {
        type Case<'a> = (&'a str, Vec<String>, Option<&'a str>, Result<Parsed, ()>);
        let cases: Vec<Case> = vec![
            (
                "defaults",
                vec![],
                None,
                Ok(window("default", NOW - 900, NOW)),
            ),
            (
                "blank body",
                vec![],
                Some("  "),
                Ok(window("default", NOW - 900, NOW)),
            ),
            (
                "ui body with extra fields",
                vec![],
                Some(
                    r#"{"after":999000,"before":999600,"selections":{},"timeout":120000,"last":200}"#,
                ),
                Ok(window("default", 999_000, 999_600)),
            ),
            (
                "relative",
                vec![],
                Some(r#"{"after":-600,"before":-60}"#),
                Ok(window("default", NOW - 660, NOW - 60)),
            ),
            (
                "tenant",
                vec![],
                Some(r#"{"tenant":"acme"}"#),
                Ok(window("acme", NOW - 900, NOW)),
            ),
            (
                "info args ignore the window",
                args(&["info", "after:-900", "before:0"]),
                None,
                Ok(Parsed::Info),
            ),
            (
                "info body",
                vec![],
                Some(r#"{"info":{}}"#),
                Ok(Parsed::Info),
            ),
            (
                "get args",
                args(&["after:999000", "before:999060", "tenant:t1"]),
                None,
                Ok(window("t1", 999_000, 999_060)),
            ),
            (
                "after not before before",
                vec![],
                Some(r#"{"after":999600,"before":999600}"#),
                Err(()),
            ),
            (
                "window too long",
                vec![],
                Some(r#"{"after":1,"before":2800000}"#),
                Err(()),
            ),
            ("bad body", vec![], Some("{"), Err(())),
            (
                "before far in the future",
                vec![],
                Some(r#"{"before":300000000000}"#),
                Err(()),
            ),
            (
                "before in milliseconds",
                args(&["before:1700000000000"]),
                None,
                Err(()),
            ),
            (
                "relative values that overflow",
                vec![],
                Some(r#"{"before":-9223372036854775807,"after":-9223372036854775807}"#),
                Err(()),
            ),
        ];
        for (name, a, body, expected) in cases {
            let got = parse(&a, body.map(str::as_bytes), NOW).map_err(|_| ());
            assert_eq!(got, expected, "{name}");
        }
    }

    #[test]
    fn errors_carry_status_and_message() {
        let r = error("t".into(), 400, "nope");
        assert_eq!(r.status, 400);
        let body: Value = serde_json::from_slice(&r.payload).unwrap();
        assert_eq!(body, json!({"status": 400, "errorMessage": "nope"}));
    }

    #[test]
    fn oversized_answers_are_413() {
        let big = json!({"x": "a".repeat(MAX_ANSWER_BYTES)});
        assert_eq!(ok("t".into(), &big).status, 413);
        assert_eq!(ok("t".into(), &json!({})).status, 200);
    }

    #[tokio::test]
    async fn answers_info_and_a_window() {
        let dir = tempfile::tempdir().unwrap();
        let map = Arc::new(ServiceMap::start(crate::service::Config::new(dir.path())).unwrap());
        let function = TopologyFunction::new(map.clone());
        assert_eq!(function.declaration().name, FUNCTION_NAME);
        for (args, payload) in [(args(&["info"]), None), (vec![], Some(b"{}".to_vec()))] {
            let (tx, _rx) = tokio::sync::mpsc::unbounded_channel();
            let ctx = Arc::new(FunctionContext {
                function_call: Box::new(netdata_plugin_protocol::FunctionCall {
                    transaction: "t".into(),
                    timeout: 10,
                    name: FUNCTION_NAME.into(),
                    args,
                    access: None,
                    source: None,
                    payload,
                }),
                cancellation_token: tokio_util::sync::CancellationToken::new(),
                outbound_tx: tx,
            });
            let result = function.handle_raw(ctx).await;
            assert_eq!(result.status, 200);
            let body: Value = serde_json::from_slice(&result.payload).unwrap();
            assert_eq!(body["type"], "topology");
        }
        map.shutdown().await;
    }
}
