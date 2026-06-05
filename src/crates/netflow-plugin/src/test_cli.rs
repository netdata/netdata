use crate::api::{FLOWS_FUNCTION_NAME, FlowsFunctionResponse, NetflowFlowsHandler};
use crate::{facet_runtime, ingest, plugin_config, query};
use anyhow::{Context, Result, bail};
use std::fs;
use std::io::{self, Write};
use std::path::PathBuf;
use std::sync::Arc;

const USAGE: &str = "usage: netflow-plugin --test flows:netflow --dir <flows-dir> --request <payload.json> [--no-persist]";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct TestCommand {
    pub(crate) function_name: String,
    pub(crate) backend_dir: PathBuf,
    pub(crate) request_path: PathBuf,
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
    write_json_response(&response, &mut handle)
}

pub(crate) async fn execute(command: TestCommand) -> Result<FlowsFunctionResponse> {
    if command.function_name != FLOWS_FUNCTION_NAME {
        bail!(
            "unsupported netflow test function `{}` (expected `{}`)",
            command.function_name,
            FLOWS_FUNCTION_NAME
        );
    }

    let request_bytes = fs::read(&command.request_path).with_context(|| {
        format!(
            "failed to read request payload {}",
            command.request_path.display()
        )
    })?;
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
    handler.handle_request(request).await.map_err(Into::into)
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
                set_once(&mut function_name, value.clone(), "--test")?;
            }
            "--dir" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --dir\n{USAGE}"))?;
                set_once(&mut backend_dir, PathBuf::from(value), "--dir")?;
            }
            "--request" => {
                idx += 1;
                let value = args
                    .get(idx)
                    .ok_or_else(|| format!("missing value for --request\n{USAGE}"))?;
                set_once(&mut request_path, PathBuf::from(value), "--request")?;
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
                    arg.trim_start_matches("--test=").to_string(),
                    "--test",
                )?;
            }
            _ if arg.starts_with("--dir=") => {
                set_once(
                    &mut backend_dir,
                    PathBuf::from(arg.trim_start_matches("--dir=")),
                    "--dir",
                )?;
            }
            _ if arg.starts_with("--request=") => {
                set_once(
                    &mut request_path,
                    PathBuf::from(arg.trim_start_matches("--request=")),
                    "--request",
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

fn required<T>(slot: Option<T>, option: &str) -> std::result::Result<T, String> {
    slot.ok_or_else(|| format!("missing required {option}\n{USAGE}"))
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
        assert!(command.no_persist);
    }

    #[test]
    fn parser_accepts_equals_form() {
        let command = parse(&[
            "--test=flows:netflow",
            "--dir=flows",
            "--request=payload.json",
        ])
        .unwrap()
        .expect("test command");

        assert_eq!(command.function_name, "flows:netflow");
        assert_eq!(command.backend_dir, Path::new("flows"));
        assert_eq!(command.request_path, Path::new("payload.json"));
        assert!(!command.no_persist);
    }

    #[test]
    fn parser_rejects_missing_required_options() {
        let err = parse(&["--test", "flows:netflow"])
            .expect_err("missing options should fail")
            .to_string();
        assert!(err.contains("missing required --dir"));
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
