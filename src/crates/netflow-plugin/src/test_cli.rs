use crate::api::{FLOWS_FUNCTION_NAME, FlowsFunctionResponse, NetflowFlowsHandler};
use crate::{facet_runtime, ingest, plugin_config, query};
use anyhow::{Context, Result, bail};
use rt::ProgressState;
use std::fs;
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;
use tokio_util::sync::CancellationToken;

const USAGE: &str = "usage: netflow-plugin --test flows:netflow --dir <flows-dir> --request <payload.json> [--timeout <seconds>] [--no-persist]";
const DEFAULT_TIMEOUT_SECONDS: u64 = 30;
const DISABLED_TIMEOUT_SECONDS: u64 = 100 * 365 * 24 * 60 * 60;
const MAX_REQUEST_BYTES: u64 = 16 * 1024 * 1024;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct TestCommand {
    pub(crate) function_name: String,
    pub(crate) backend_dir: PathBuf,
    pub(crate) request_path: PathBuf,
    pub(crate) timeout_seconds: u64,
    pub(crate) no_persist: bool,
}

impl TestCommand {
    pub(crate) fn parse_from_env_args() -> std::result::Result<Option<Self>, String> {
        parse_from(std::env::args().skip(1))
    }
}

pub(crate) async fn run(command: TestCommand) -> Result<()> {
    let response = execute(command).await?;
    let stdout = io::stdout();
    let mut handle = stdout.lock();
    write_json_response(&response, &mut handle)?;
    ensure_success_status(response.status())
}

pub(crate) async fn execute(command: TestCommand) -> Result<FlowsFunctionResponse> {
    if command.function_name != FLOWS_FUNCTION_NAME {
        bail!(
            "unsupported netflow test function `{}` (expected `{}`)",
            command.function_name,
            FLOWS_FUNCTION_NAME
        );
    }

    let request_bytes = read_request_payload(&command.request_path)?;
    let request =
        serde_json::from_slice::<query::FlowsRequest>(&request_bytes).with_context(|| {
            format!(
                "failed to parse request payload {}",
                command.request_path.display()
            )
        })?;

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
    let cancellation = CancellationToken::new();
    let execution = if request.is_autocomplete_mode() {
        None
    } else {
        Some(query::QueryExecutionContext::new(
            ProgressState::default(),
            cancellation.clone(),
        ))
    };

    match tokio::time::timeout(
        Duration::from_secs(effective_timeout_seconds(command.timeout_seconds)),
        handler.handle_request_with_execution(execution, request),
    )
    .await
    {
        Ok(result) => result.map_err(Into::into),
        Err(_) => {
            cancellation.cancel();
            bail!(
                "netflow test function timed out after {} seconds",
                effective_timeout_seconds(command.timeout_seconds)
            );
        }
    }
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

fn read_request_payload(path: &Path) -> Result<Vec<u8>> {
    let metadata = fs::symlink_metadata(path)
        .with_context(|| format!("failed to inspect request payload {}", path.display()))?;
    validate_request_payload_metadata(path, &metadata)?;

    let file = open_request_payload_file(path)?;
    let metadata = file
        .metadata()
        .with_context(|| format!("failed to inspect request payload {}", path.display()))?;
    validate_request_payload_metadata(path, &metadata)?;

    let mut request_bytes = Vec::with_capacity(metadata.len() as usize);
    file.take(MAX_REQUEST_BYTES + 1)
        .read_to_end(&mut request_bytes)
        .with_context(|| format!("failed to read request payload {}", path.display()))?;

    if request_bytes.is_empty() {
        bail!("request payload {} is empty", path.display());
    }

    if request_bytes.len() as u64 > MAX_REQUEST_BYTES {
        bail!(
            "request payload {} is too large: more than {} bytes",
            path.display(),
            MAX_REQUEST_BYTES
        );
    }

    Ok(request_bytes)
}

fn open_request_payload_file(path: &Path) -> Result<fs::File> {
    #[cfg(not(unix))]
    {
        bail!("netflow test request payloads require Unix safe-open flags");
    }

    #[cfg(unix)]
    {
        let mut options = fs::OpenOptions::new();
        options.read(true);
        use std::os::unix::fs::OpenOptionsExt;

        // O_NOFOLLOW rejects a final symlink race. O_NONBLOCK avoids blocking if the path races to a FIFO before
        // fstat() rejects it as non-regular.
        options.custom_flags(libc::O_NOFOLLOW | libc::O_NONBLOCK);

        options
            .open(path)
            .with_context(|| format!("failed to open request payload {}", path.display()))
    }
}

fn validate_request_payload_metadata(path: &Path, metadata: &fs::Metadata) -> Result<()> {
    if !metadata.is_file() {
        bail!("request payload {} is not a regular file", path.display());
    }

    if metadata.len() == 0 {
        bail!("request payload {} is empty", path.display());
    }

    if metadata.len() > MAX_REQUEST_BYTES {
        bail!(
            "request payload {} is too large: {} bytes, max {} bytes",
            path.display(),
            metadata.len(),
            MAX_REQUEST_BYTES
        );
    }

    Ok(())
}

fn parse_from(
    args: impl IntoIterator<Item = String>,
) -> std::result::Result<Option<TestCommand>, String> {
    let args = args.into_iter().collect::<Vec<_>>();
    if !args
        .iter()
        .any(|arg| arg == "--test" || arg.starts_with("--test="))
    {
        return Ok(None);
    }

    let mut function_name = None;
    let mut backend_dir = None;
    let mut request_path = None;
    let mut timeout_seconds = None;
    let mut no_persist = false;
    let mut idx = 0;

    while idx < args.len() {
        let arg = &args[idx];
        match arg.as_str() {
            "--test" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --test\n{USAGE}"))?;
                set_once(
                    &mut function_name,
                    required_option_value(value, "--test")?.to_string(),
                    "--test",
                )?;
            }
            "--dir" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --dir\n{USAGE}"))?;
                set_once(
                    &mut backend_dir,
                    PathBuf::from(required_option_value(value, "--dir")?),
                    "--dir",
                )?;
            }
            "--request" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --request\n{USAGE}"))?;
                set_once(
                    &mut request_path,
                    PathBuf::from(required_option_value(value, "--request")?),
                    "--request",
                )?;
            }
            "--timeout" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --timeout\n{USAGE}"))?;
                set_once(
                    &mut timeout_seconds,
                    parse_timeout_seconds(value)?,
                    "--timeout",
                )?;
            }
            "--no-persist" => {
                if no_persist {
                    return Err(format!("duplicate --no-persist\n{USAGE}"));
                }
                no_persist = true;
            }
            _ if arg.starts_with("--test=") => {
                set_once(
                    &mut function_name,
                    required_option_value(arg.trim_start_matches("--test="), "--test")?.to_string(),
                    "--test",
                )?;
            }
            _ if arg.starts_with("--dir=") => {
                set_once(
                    &mut backend_dir,
                    PathBuf::from(required_option_value(
                        arg.trim_start_matches("--dir="),
                        "--dir",
                    )?),
                    "--dir",
                )?;
            }
            _ if arg.starts_with("--request=") => {
                set_once(
                    &mut request_path,
                    PathBuf::from(required_option_value(
                        arg.trim_start_matches("--request="),
                        "--request",
                    )?),
                    "--request",
                )?;
            }
            _ if arg.starts_with("--timeout=") => {
                set_once(
                    &mut timeout_seconds,
                    parse_timeout_seconds(arg.trim_start_matches("--timeout="))?,
                    "--timeout",
                )?;
            }
            "-h" | "--help" => {
                return Err(USAGE.to_string());
            }
            _ => {
                return Err(format!("unsupported netflow test option `{arg}`\n{USAGE}"));
            }
        }
        idx += 1;
    }

    Ok(Some(TestCommand {
        function_name: required(function_name, "--test")?,
        backend_dir: required(backend_dir, "--dir")?,
        request_path: required(request_path, "--request")?,
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

fn required_option_value<'a>(value: &'a str, option: &str) -> std::result::Result<&'a str, String> {
    if value.is_empty() {
        return Err(format!("missing value for {option}\n{USAGE}"));
    }

    Ok(value)
}

fn required<T>(slot: Option<T>, option: &str) -> std::result::Result<T, String> {
    slot.ok_or_else(|| format!("missing required {option}\n{USAGE}"))
}

fn parse_timeout_seconds(value: &str) -> std::result::Result<u64, String> {
    value
        .parse::<u64>()
        .map_err(|_| format!("invalid value for --timeout `{value}`; expected seconds\n{USAGE}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    fn parse(args: &[&str]) -> std::result::Result<Option<TestCommand>, String> {
        parse_from(args.iter().map(|arg| (*arg).to_string()))
    }

    #[test]
    fn parser_ignores_normal_plugin_arguments_when_test_is_absent() {
        assert_eq!(parse(&["--netflow-enabled", "false"]).unwrap(), None);
    }

    #[test]
    fn parser_accepts_file_based_test_command() {
        let command = parse(&[
            "--test",
            "flows:netflow",
            "--dir",
            "flows",
            "--request",
            "payload.json",
            "--no-persist",
        ])
        .unwrap()
        .expect("test command");

        assert_eq!(command.function_name, "flows:netflow");
        assert_eq!(command.backend_dir, Path::new("flows"));
        assert_eq!(command.request_path, Path::new("payload.json"));
        assert_eq!(command.timeout_seconds, DEFAULT_TIMEOUT_SECONDS);
        assert!(command.no_persist);
    }

    #[test]
    fn parser_accepts_equals_form() {
        let command = parse(&[
            "--test=flows:netflow",
            "--dir=flows",
            "--request=payload.json",
            "--timeout=0",
        ])
        .unwrap()
        .expect("test command");

        assert_eq!(command.function_name, "flows:netflow");
        assert_eq!(command.backend_dir, Path::new("flows"));
        assert_eq!(command.request_path, Path::new("payload.json"));
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
            "--request",
            "payload.json",
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
            ["--test=", "--dir=flows", "--request=payload.json"].as_slice(),
            ["--test", "", "--dir=flows", "--request=payload.json"].as_slice(),
            ["--test=flows:netflow", "--dir=", "--request=payload.json"].as_slice(),
            [
                "--test=flows:netflow",
                "--dir",
                "",
                "--request=payload.json",
            ]
            .as_slice(),
            ["--test=flows:netflow", "--dir=flows", "--request="].as_slice(),
            ["--test=flows:netflow", "--dir=flows", "--request", ""].as_slice(),
        ] {
            let err = parse(args).expect_err("empty required option should fail");
            assert!(err.contains("missing value for --"));
        }
    }

    #[test]
    fn parser_rejects_unknown_test_options() {
        let err = parse(&[
            "--test",
            "flows:netflow",
            "--dir",
            "flows",
            "--request",
            "payload.json",
            "--unknown",
        ])
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
            "--request",
            "payload.json",
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
            "--request",
            "payload.json",
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
    fn read_request_payload_rejects_empty_file() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let path = tmp.path().join("empty.json");
        fs::write(&path, []).expect("write empty request");

        let err = read_request_payload(&path).expect_err("empty request should fail");
        assert!(err.to_string().contains("is empty"));
    }

    #[test]
    fn read_request_payload_rejects_oversized_file() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let path = tmp.path().join("oversized.json");
        let file = fs::File::create(&path).expect("create oversized request");
        file.set_len(MAX_REQUEST_BYTES + 1)
            .expect("size oversized request");

        let err = read_request_payload(&path).expect_err("oversized request should fail");
        assert!(err.to_string().contains("is too large"));
    }

    #[cfg(unix)]
    #[test]
    fn read_request_payload_rejects_fifo() {
        use std::ffi::CString;
        use std::os::unix::ffi::OsStrExt;

        let tmp = tempfile::tempdir().expect("create temp dir");
        let path = tmp.path().join("request.fifo");
        let c_path = CString::new(path.as_os_str().as_bytes()).expect("fifo path has no nul byte");
        let rc = unsafe { libc::mkfifo(c_path.as_ptr(), 0o600) };
        assert_eq!(rc, 0, "mkfifo {} failed", path.display());

        let err = read_request_payload(&path).expect_err("fifo request should fail");
        assert!(err.to_string().contains("not a regular file"));
    }

    #[cfg(unix)]
    #[test]
    fn read_request_payload_rejects_symlink() {
        use std::os::unix::fs::symlink;

        let tmp = tempfile::tempdir().expect("create temp dir");
        let target_path = tmp.path().join("target.json");
        fs::write(&target_path, "{}").expect("write request target");
        let symlink_path = tmp.path().join("request.json");
        symlink(&target_path, &symlink_path).expect("create request symlink");

        let err = read_request_payload(&symlink_path).expect_err("symlink request should fail");
        assert!(err.to_string().contains("not a regular file"));
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn execute_empty_backend_writes_raw_json_without_persisting_facets() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let backend_dir = tmp.path().join("flows");
        for tier in ["raw", "1m", "5m", "1h"] {
            fs::create_dir_all(backend_dir.join(tier)).expect("create tier dir");
        }

        let request_path = tmp.path().join("flows-request.json");
        fs::write(
            &request_path,
            r#"{"after":1,"before":2,"group_by":["PROTOCOL"],"top_n":"100"}"#,
        )
        .expect("write request payload");

        let response = execute(TestCommand {
            function_name: FLOWS_FUNCTION_NAME.to_string(),
            backend_dir: backend_dir.clone(),
            request_path,
            timeout_seconds: DEFAULT_TIMEOUT_SECONDS,
            no_persist: true,
        })
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
}
