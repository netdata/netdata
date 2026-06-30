use crate::api::{FLOWS_FUNCTION_NAME, FlowsFunctionResponse, NetflowFlowsHandler};
use crate::{facet_runtime, ingest, plugin_config, query};
use anyhow::{Context, Result, bail};
use rt::ProgressState;
use std::ffi::{OsStr, OsString};
use std::io::{self, Read, Write};
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tokio_util::sync::CancellationToken;

const USAGE: &str = "usage: netflow-plugin --test flows:netflow --dir <flows-dir> [--timeout <seconds>] [--no-persist] < payload.json";
const DEFAULT_TIMEOUT_SECONDS: u64 = 30;
const DISABLED_TIMEOUT_SECONDS: u64 = 100 * 365 * 24 * 60 * 60;
const MAX_REQUEST_BYTES: u64 = 16 * 1024 * 1024;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct TestCommand {
    pub(crate) function_name: String,
    pub(crate) backend_dir: PathBuf,
    pub(crate) timeout_seconds: u64,
    pub(crate) no_persist: bool,
}

impl TestCommand {
    pub(crate) fn parse_from_env_args() -> std::result::Result<Option<Self>, String> {
        parse_from_os(std::env::args_os().skip(1))
    }
}

pub(crate) async fn run(command: TestCommand) -> Result<()> {
    let request_bytes = read_request_payload_from_stdin(io::stdin().lock())?;
    let response = execute(command, &request_bytes).await?;
    let stdout = io::stdout();
    let mut handle = stdout.lock();
    write_json_response(&response, &mut handle)?;
    ensure_success_status(response.status())
}

pub(crate) async fn execute(
    command: TestCommand,
    request_bytes: &[u8],
) -> Result<FlowsFunctionResponse> {
    if command.function_name != FLOWS_FUNCTION_NAME {
        bail!(
            "unsupported netflow test function `{}` (expected `{}`)",
            command.function_name,
            FLOWS_FUNCTION_NAME
        );
    }

    let effective_timeout = effective_timeout_seconds(command.timeout_seconds);
    let cancellation = CancellationToken::new();
    match tokio::time::timeout(
        Duration::from_secs(effective_timeout),
        execute_inner(command, request_bytes, cancellation.clone()),
    )
    .await
    {
        Ok(result) => result,
        Err(_) => {
            cancellation.cancel();
            bail!(
                "netflow test function timed out after {} seconds",
                effective_timeout
            );
        }
    }
}

async fn execute_inner(
    command: TestCommand,
    request_bytes: &[u8],
    cancellation: CancellationToken,
) -> Result<FlowsFunctionResponse> {
    let request = serde_json::from_slice::<query::FlowsRequest>(request_bytes)
        .context("failed to parse request payload from stdin")?;

    let config = plugin_config::PluginConfig::for_test_backend_dir(&command.backend_dir)?;
    ensure_tier_directories_exist(&config)?;

    let facet_runtime = if command.no_persist {
        facet_runtime::FacetRuntime::new_read_only(&config.journal.base_dir())
    } else {
        facet_runtime::FacetRuntime::new(&config.journal.base_dir())
    };
    let facet_runtime = Arc::new(facet_runtime);
    let (query_service, _notify_rx) =
        query::FlowQueryService::new_with_facet_runtime(&config, Arc::clone(&facet_runtime))
            .await?;
    let query_service = Arc::new(query_service);
    query_service.initialize_facets().await?;

    let handler = NetflowFlowsHandler::new(
        Arc::new(ingest::IngestMetrics::default()),
        Arc::clone(&query_service),
    );
    let execution = if request.is_autocomplete_mode() {
        None
    } else {
        Some(query::QueryExecutionContext::new(
            ProgressState::default(),
            cancellation.clone(),
        ))
    };

    handler
        .handle_request_with_execution(execution, request)
        .await
        .map_err(Into::into)
}

pub(crate) fn write_json_response(
    response: &FlowsFunctionResponse,
    writer: &mut impl Write,
) -> Result<()> {
    serde_json::to_writer(&mut *writer, response).context("failed to write JSON response")?;
    writer
        .write_all(b"\n")
        .context("failed to finish JSON response")?;
    writer.flush().context("failed to flush JSON response")
}

fn ensure_tier_directories_exist(config: &plugin_config::PluginConfig) -> Result<()> {
    for tier_dir in config.journal.all_tier_dirs() {
        if !tier_dir.is_dir() {
            bail!(
                "netflow backend tier directory {} does not exist",
                tier_dir.display()
            );
        }
    }
    Ok(())
}

fn effective_timeout_seconds(timeout_seconds: u64) -> u64 {
    if timeout_seconds == 0 {
        DISABLED_TIMEOUT_SECONDS
    } else {
        timeout_seconds
    }
}

fn ensure_success_status(status: u32) -> Result<()> {
    if !(200..300).contains(&status) {
        bail!("netflow test function returned status {status}");
    }

    Ok(())
}

fn read_request_payload_from_stdin(reader: impl Read) -> Result<Vec<u8>> {
    let mut request_bytes = Vec::new();
    reader
        .take(MAX_REQUEST_BYTES + 1)
        .read_to_end(&mut request_bytes)
        .context("failed to read request payload from stdin")?;

    if request_bytes.is_empty() {
        bail!("request payload from stdin is empty");
    }

    if request_bytes.len() as u64 > MAX_REQUEST_BYTES {
        bail!(
            "request payload from stdin is too large: more than {} bytes",
            MAX_REQUEST_BYTES
        );
    }

    Ok(request_bytes)
}

#[cfg(test)]
fn parse_from(
    args: impl IntoIterator<Item = String>,
) -> std::result::Result<Option<TestCommand>, String> {
    parse_from_os(args.into_iter().map(OsString::from))
}

fn parse_from_os(
    args: impl IntoIterator<Item = OsString>,
) -> std::result::Result<Option<TestCommand>, String> {
    let args = args.into_iter().collect::<Vec<_>>();
    if !args
        .iter()
        .any(|arg| arg == OsStr::new("--test") || strip_os_prefix(arg, "--test=").is_some())
    {
        return Ok(None);
    }

    let mut function_name = None;
    let mut backend_dir = None;
    let mut timeout_seconds = None;
    let mut no_persist = false;
    let mut idx = 0;

    while idx < args.len() {
        let arg = &args[idx];
        if arg == OsStr::new("--test") {
            idx += 1;
            let value = args
                .get(idx)
                .ok_or_else(|| format!("missing value for --test\n{USAGE}"))?;
            set_once(
                &mut function_name,
                required_os_string_value(value, "--test")?,
                "--test",
            )?;
        } else if arg == OsStr::new("--dir") {
            idx += 1;
            let value = args
                .get(idx)
                .ok_or_else(|| format!("missing value for --dir\n{USAGE}"))?;
            set_once(
                &mut backend_dir,
                PathBuf::from(required_os_string_value(value, "--dir")?),
                "--dir",
            )?;
        } else if arg == OsStr::new("--request") {
            return Err(format!(
                "--request is no longer supported; pass the request payload on stdin\n{USAGE}"
            ));
        } else if arg == OsStr::new("--timeout") {
            idx += 1;
            let value = args
                .get(idx)
                .ok_or_else(|| format!("missing value for --timeout\n{USAGE}"))?;
            set_once(
                &mut timeout_seconds,
                parse_timeout_seconds(&required_os_string_value(value, "--timeout")?)?,
                "--timeout",
            )?;
        } else if arg == OsStr::new("--no-persist") {
            if no_persist {
                return Err(format!("duplicate --no-persist\n{USAGE}"));
            }
            no_persist = true;
        } else if let Some(value) = strip_os_prefix(arg, "--test=") {
            set_once(
                &mut function_name,
                required_os_string_value(&value, "--test")?,
                "--test",
            )?;
        } else if let Some(value) = strip_os_prefix(arg, "--dir=") {
            set_once(
                &mut backend_dir,
                PathBuf::from(required_os_string_value(&value, "--dir")?),
                "--dir",
            )?;
        } else if strip_os_prefix(arg, "--request=").is_some() {
            return Err(format!(
                "--request is no longer supported; pass the request payload on stdin\n{USAGE}"
            ));
        } else if let Some(value) = strip_os_prefix(arg, "--timeout=") {
            set_once(
                &mut timeout_seconds,
                parse_timeout_seconds(&required_os_string_value(&value, "--timeout")?)?,
                "--timeout",
            )?;
        } else if arg == OsStr::new("-h") || arg == OsStr::new("--help") {
            return Err(USAGE.to_string());
        } else {
            return Err(format!(
                "unsupported netflow test option `{}`\n{USAGE}",
                arg.to_string_lossy()
            ));
        }
        idx += 1;
    }

    Ok(Some(TestCommand {
        function_name: required(function_name, "--test")?,
        backend_dir: required(backend_dir, "--dir")?,
        timeout_seconds: timeout_seconds.unwrap_or(DEFAULT_TIMEOUT_SECONDS),
        no_persist,
    }))
}

fn set_once<T>(slot: &mut Option<T>, value: T, option: &str) -> std::result::Result<(), String> {
    if slot.is_some() {
        return Err(format!("duplicate {option}\n{USAGE}"));
    }
    *slot = Some(value);
    Ok(())
}

fn required_os_option_value<'a>(
    value: &'a OsStr,
    option: &str,
) -> std::result::Result<&'a OsStr, String> {
    if value.is_empty() {
        return Err(format!("missing value for {option}\n{USAGE}"));
    }

    Ok(value)
}

fn required_os_string_value(value: &OsStr, option: &str) -> std::result::Result<String, String> {
    let value = required_os_option_value(value, option)?;
    value
        .to_str()
        .map(ToString::to_string)
        .ok_or_else(|| format!("invalid value for {option}; expected UTF-8\n{USAGE}"))
}

fn required<T>(slot: Option<T>, option: &str) -> std::result::Result<T, String> {
    slot.ok_or_else(|| format!("missing required {option}\n{USAGE}"))
}

fn parse_timeout_seconds(value: &str) -> std::result::Result<u64, String> {
    value
        .parse::<u64>()
        .map_err(|_| format!("invalid value for --timeout `{value}`; expected seconds\n{USAGE}"))
}

#[cfg(unix)]
fn strip_os_prefix(value: &OsStr, prefix: &str) -> Option<OsString> {
    use std::os::unix::ffi::{OsStrExt, OsStringExt};

    value
        .as_bytes()
        .strip_prefix(prefix.as_bytes())
        .map(|suffix| OsString::from_vec(suffix.to_vec()))
}

#[cfg(not(unix))]
fn strip_os_prefix(value: &OsStr, prefix: &str) -> Option<OsString> {
    value.to_str()?.strip_prefix(prefix).map(OsString::from)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn parse(args: &[&str]) -> std::result::Result<Option<TestCommand>, String> {
        parse_from(args.iter().map(|arg| (*arg).to_string()))
    }

    #[test]
    fn parser_ignores_normal_plugin_arguments_when_test_is_absent() {
        assert_eq!(parse(&["--netflow-enabled", "false"]).unwrap(), None);
    }

    #[cfg(unix)]
    #[test]
    fn parser_ignores_non_utf8_normal_arguments_when_test_is_absent() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;

        let args = [
            OsString::from("--netflow-enabled"),
            OsString::from_vec(vec![0xff]),
        ];

        assert_eq!(parse_from_os(args).unwrap(), None);
    }

    #[cfg(unix)]
    #[test]
    fn parser_rejects_non_utf8_spaced_backend_dir() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;

        let err = parse_from_os([
            OsString::from("--test"),
            OsString::from("flows:netflow"),
            OsString::from("--dir"),
            OsString::from_vec(b"flows-\xff".to_vec()),
        ])
        .expect_err("non-UTF-8 backend directory should fail");

        assert!(err.contains("invalid value for --dir; expected UTF-8"));
    }

    #[cfg(unix)]
    #[test]
    fn parser_rejects_non_utf8_equals_backend_dir() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;

        let mut dir_arg = b"--dir=".to_vec();
        dir_arg.extend_from_slice(b"flows-\xff");

        let err = parse_from_os([
            OsString::from("--test=flows:netflow"),
            OsString::from_vec(dir_arg),
        ])
        .expect_err("non-UTF-8 backend directory should fail");

        assert!(err.contains("invalid value for --dir; expected UTF-8"));
    }

    #[test]
    fn parser_accepts_stdin_test_command() {
        let command = parse(&["--test", "flows:netflow", "--dir", "flows", "--no-persist"])
            .unwrap()
            .expect("test command");

        assert_eq!(command.function_name, "flows:netflow");
        assert_eq!(command.backend_dir, PathBuf::from("flows"));
        assert_eq!(command.timeout_seconds, DEFAULT_TIMEOUT_SECONDS);
        assert!(command.no_persist);
    }

    #[test]
    fn parser_accepts_equals_form() {
        let command = parse(&["--test=flows:netflow", "--dir=flows", "--timeout=0"])
            .unwrap()
            .expect("test command");

        assert_eq!(command.function_name, "flows:netflow");
        assert_eq!(command.backend_dir, PathBuf::from("flows"));
        assert_eq!(command.timeout_seconds, 0);
        assert_eq!(
            effective_timeout_seconds(command.timeout_seconds),
            DISABLED_TIMEOUT_SECONDS
        );
        assert!(!command.no_persist);
    }

    #[test]
    fn parser_accepts_spaced_timeout() {
        let command = parse(&[
            "--test",
            "flows:netflow",
            "--dir",
            "flows",
            "--timeout",
            "120",
        ])
        .unwrap()
        .expect("test command");

        assert_eq!(command.timeout_seconds, 120);
        assert_eq!(effective_timeout_seconds(command.timeout_seconds), 120);
    }

    #[test]
    fn parser_rejects_missing_required_options() {
        let err = parse(&["--test", "flows:netflow"])
            .expect_err("missing options should fail")
            .to_string();
        assert!(err.contains("missing required --dir"));
    }

    #[test]
    fn parser_rejects_empty_required_values() {
        for args in [
            ["--test=", "--dir=flows"].as_slice(),
            ["--test", "", "--dir=flows"].as_slice(),
            ["--test=flows:netflow", "--dir="].as_slice(),
            ["--test=flows:netflow", "--dir", ""].as_slice(),
        ] {
            let err = parse(args).expect_err("empty required option should fail");
            assert!(err.contains("missing value for --"));
        }
    }

    #[test]
    fn parser_rejects_request_option() {
        for args in [
            [
                "--test=flows:netflow",
                "--dir=flows",
                "--request=payload.json",
            ]
            .as_slice(),
            [
                "--test=flows:netflow",
                "--dir=flows",
                "--request",
                "payload.json",
            ]
            .as_slice(),
        ] {
            let err = parse(args).expect_err("--request should fail");
            assert!(err.contains("--request is no longer supported"));
        }
    }

    #[test]
    fn parser_rejects_unknown_test_options() {
        let err = parse(&["--test", "flows:netflow", "--dir", "flows", "--unknown"])
            .expect_err("unknown option should fail");

        assert!(err.contains("unsupported netflow test option"));
    }

    #[test]
    fn parser_rejects_invalid_timeout() {
        let err = parse(&[
            "--test",
            "flows:netflow",
            "--dir",
            "flows",
            "--timeout",
            "-1",
        ])
        .expect_err("invalid timeout should fail");

        assert!(err.contains("invalid value for --timeout"));
    }

    #[test]
    fn parser_rejects_duplicate_timeout() {
        let err = parse(&[
            "--test",
            "flows:netflow",
            "--dir",
            "flows",
            "--timeout",
            "30",
            "--timeout=60",
        ])
        .expect_err("duplicate timeout should fail");

        assert!(err.contains("duplicate --timeout"));
    }

    #[test]
    fn success_status_accepts_2xx() {
        ensure_success_status(200).expect("200 should pass");
        ensure_success_status(299).expect("299 should pass");
    }

    #[test]
    fn success_status_rejects_non_2xx() {
        let err = ensure_success_status(400).expect_err("400 should fail");
        assert!(err.to_string().contains("returned status 400"));
    }

    #[test]
    fn read_request_payload_from_stdin_rejects_empty_input() {
        let err = read_request_payload_from_stdin(io::Cursor::new(Vec::new()))
            .expect_err("empty request should fail");
        assert!(err.to_string().contains("is empty"));
    }

    #[test]
    fn read_request_payload_from_stdin_rejects_oversized_input() {
        let oversized = vec![b' '; (MAX_REQUEST_BYTES + 1) as usize];
        let err = read_request_payload_from_stdin(io::Cursor::new(oversized))
            .expect_err("oversized request should fail");
        assert!(err.to_string().contains("is too large"));
    }

    #[test]
    fn read_request_payload_from_stdin_reads_payload() {
        let request = read_request_payload_from_stdin(io::Cursor::new(br#"{"after":1}"#))
            .expect("read request payload");
        assert_eq!(request, br#"{"after":1}"#);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn execute_empty_backend_writes_raw_json_without_persisting_facets() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let backend_dir = tmp.path().join("flows");
        for tier in ["raw", "1m", "5m", "1h"] {
            fs::create_dir_all(backend_dir.join(tier)).expect("create tier dir");
        }

        let response = execute(
            TestCommand {
                function_name: FLOWS_FUNCTION_NAME.to_string(),
                backend_dir: backend_dir.clone(),
                timeout_seconds: DEFAULT_TIMEOUT_SECONDS,
                no_persist: true,
            },
            br#"{"after":1,"before":2,"group_by":["PROTOCOL"],"top_n":"100"}"#,
        )
        .await
        .expect("execute test CLI request");

        let mut stdout = Vec::new();
        write_json_response(&response, &mut stdout).expect("write JSON response");
        let output = String::from_utf8(stdout).expect("stdout should be UTF-8 JSON");

        assert!(
            output.trim_start().starts_with('{'),
            "test CLI stdout should start with JSON object, got {output:?}"
        );
        assert!(
            !output.starts_with("TRUST_DURATIONS"),
            "test CLI stdout must not include PLUGINSD protocol lines"
        );

        let value = serde_json::from_str::<serde_json::Value>(&output).expect("parse JSON output");
        assert_eq!(value["status"], 200);
        assert_eq!(value["type"], "flows");
        assert!(
            value["data"]["flows"].as_array().is_some_and(Vec::is_empty),
            "empty backend should return an empty flows array"
        );
        assert!(
            !backend_dir.join("facet-state.bin").exists(),
            "--no-persist must not write facet state under the fixture backend"
        );
    }

    #[cfg(unix)]
    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn execute_rejects_non_utf8_backend_dir() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;

        let err = execute(
            TestCommand {
                function_name: FLOWS_FUNCTION_NAME.to_string(),
                backend_dir: PathBuf::from(OsString::from_vec(b"flows-\xff".to_vec())),
                timeout_seconds: DEFAULT_TIMEOUT_SECONDS,
                no_persist: true,
            },
            br#"{"after":1,"before":2,"group_by":["PROTOCOL"],"top_n":"100"}"#,
        )
        .await
        .expect_err("non-UTF-8 backend directory should fail");

        assert!(err.to_string().contains("is not valid UTF-8"));
    }
}
