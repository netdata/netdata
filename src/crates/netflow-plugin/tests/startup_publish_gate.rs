//! Startup publication contract, checked against the real binary over its
//! stdin/stdout pipes, the way the Agent runs it.
//!
//! The Agent counts FUNCTION, END and similar lines as successful data
//! collections, and it restarts a plugin that exits with an error after
//! producing any of them, forever (`src/plugins.d/plugins_d.c`). Only a plugin
//! that fails before publishing anything is disabled. So a startup failure,
//! such as a listen port already in use, must leave stdout free of every
//! keyword the Agent counts.
//!
//! Run with: `cargo test -p netflow-plugin --test startup_publish_gate`

use std::io::{BufRead, BufReader, Read};
use std::net::UdpSocket;
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::sync::mpsc;
use std::time::{Duration, Instant};

const STARTUP_TIMEOUT: Duration = Duration::from_secs(60);

/// The only lines a plugin may write before its startup has succeeded: neither
/// is counted as a data collection by the Agent.
const UNPUBLISHED_KEYWORDS: [&str; 2] = ["TRUST_DURATIONS", "PLUGIN_KEEPALIVE"];

fn spawn_plugin(listen: &str, journal_dir: &Path) -> Child {
    let mut command = Command::new(env!("CARGO_BIN_EXE_netflow-plugin"));
    // Without these the plugin reads its configuration from the command line,
    // not from the Agent's config directories.
    for (key, _) in std::env::vars_os() {
        if key.to_string_lossy().starts_with("NETDATA_") {
            command.env_remove(key);
        }
    }
    command
        .arg("--netflow-listen")
        .arg(listen)
        .arg("--netflow-journal-dir")
        .arg(journal_dir)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn netflow-plugin")
}

/// Forward stdout lines over a channel so the test can read with a deadline.
fn stdout_lines(child: &mut Child) -> mpsc::Receiver<String> {
    stdout_lines_with_disconnect(child, false)
}

fn stdout_lines_with_disconnect(child: &mut Child, disconnect: bool) -> mpsc::Receiver<String> {
    let stdout = child.stdout.take().expect("piped stdout");
    let (tx, rx) = mpsc::channel();
    std::thread::spawn(move || {
        for line in BufReader::new(stdout).lines() {
            let Ok(line) = line else { break };
            let close_pipe = disconnect && keyword(&line) == "PLUGIN_KEEPALIVE";
            if tx.send(line).is_err() || close_pipe {
                break;
            }
        }
    });
    rx
}

fn stderr_text(child: &mut Child) -> std::thread::JoinHandle<String> {
    let mut stderr = child.stderr.take().expect("piped stderr");
    std::thread::spawn(move || {
        let mut text = String::new();
        let _ = stderr.read_to_string(&mut text);
        text
    })
}

fn wait_with_deadline(child: &mut Child, timeout: Duration) -> Option<std::process::ExitStatus> {
    let deadline = Instant::now() + timeout;
    loop {
        if let Some(status) = child.try_wait().expect("poll netflow-plugin") {
            return Some(status);
        }
        if Instant::now() >= deadline {
            let _ = child.kill();
            let _ = child.wait();
            return None;
        }
        std::thread::sleep(Duration::from_millis(20));
    }
}

fn keyword(line: &str) -> &str {
    line.split_whitespace().next().unwrap_or("")
}

#[test]
fn listen_bind_failure_exits_without_publishing() {
    let journal_dir = tempfile::tempdir().expect("create journal dir");
    let occupied = UdpSocket::bind("127.0.0.1:0").expect("occupy a UDP port");
    let listen = occupied.local_addr().expect("occupied address").to_string();

    let mut child = spawn_plugin(&listen, journal_dir.path());
    // Hold stdin open, as the Agent does: closing it is a shutdown request and
    // would end the plugin through a different path.
    let _stdin = child.stdin.take().expect("piped stdin");
    let lines = stdout_lines(&mut child);
    let stderr = stderr_text(&mut child);

    let status = wait_with_deadline(&mut child, STARTUP_TIMEOUT);
    let stderr = stderr.join().expect("join stderr reader");
    let status = status.unwrap_or_else(|| {
        panic!(
            "plugin did not exit on a bind failure within {STARTUP_TIMEOUT:?}; stderr:\n{stderr}"
        )
    });
    // The child has exited, so its stdout reaches EOF and this terminates.
    let output: Vec<String> = lines.iter().collect();

    assert_eq!(
        status.code(),
        Some(1),
        "a startup failure must exit 1 so the Agent disables the plugin; stderr:\n{stderr}"
    );
    let published: Vec<&String> = output
        .iter()
        .filter(|line| !UNPUBLISHED_KEYWORDS.contains(&keyword(line)))
        .collect();
    assert!(
        published.is_empty(),
        "nothing may be published before the listener is bound, got {published:?}"
    );
    assert!(
        stderr.contains(&format!("failed to bind {listen}")),
        "the bind error must be logged; stderr:\n{stderr}"
    );
}

#[test]
fn successful_startup_publishes_function_and_charts() {
    let journal_dir = tempfile::tempdir().expect("create journal dir");

    let mut child = spawn_plugin("127.0.0.1:0", journal_dir.path());
    let stdin = child.stdin.take().expect("piped stdin");
    let lines = stdout_lines(&mut child);
    let stderr = stderr_text(&mut child);

    let deadline = Instant::now() + STARTUP_TIMEOUT;
    let mut function_declared = false;
    let mut chart_collected = false;
    while !(function_declared && chart_collected) {
        let remaining = deadline.saturating_duration_since(Instant::now());
        let Ok(line) = lines.recv_timeout(remaining) else {
            break;
        };
        match keyword(&line) {
            "FUNCTION" => function_declared |= line.contains(" flows:netflow "),
            "END" => chart_collected = true,
            _ => {}
        }
    }

    // Closing stdin is the Agent's shutdown request.
    drop(stdin);
    let status = wait_with_deadline(&mut child, STARTUP_TIMEOUT);
    let stderr = stderr.join().expect("join stderr reader");

    assert!(
        function_declared && chart_collected,
        "a started plugin must declare flows:netflow ({function_declared}) and emit chart data \
         ({chart_collected}); stderr:\n{stderr}"
    );
    assert_eq!(
        status.and_then(|status| status.code()),
        Some(0),
        "plugin must exit cleanly when stdin closes; stderr:\n{stderr}"
    );
}

#[cfg(unix)]
#[test]
fn blocked_facet_cache_load_sends_keepalive_without_publishing() {
    check_blocked_cache_startup(false);
}

#[cfg(unix)]
#[test]
fn disconnected_agent_is_reported_while_cache_read_is_blocked() {
    check_blocked_cache_startup(true);
}

#[cfg(unix)]
fn check_blocked_cache_startup(disconnect: bool) {
    use std::ffi::CString;
    use std::fs::OpenOptions;
    use std::io::Write;
    use std::os::unix::ffi::OsStrExt;
    use std::os::unix::fs::OpenOptionsExt;

    let journal_dir = tempfile::tempdir().expect("create journal dir");
    let cache_path = journal_dir.path().join("facet-state.bin");
    let fifo_path = CString::new(cache_path.as_os_str().as_bytes()).unwrap();
    // A FIFO deterministically stalls the real cache reader, without a test
    // switch in the shipped binary or a large journal fixture.
    assert_eq!(unsafe { libc::mkfifo(fifo_path.as_ptr(), 0o600) }, 0);
    let occupied = UdpSocket::bind("127.0.0.1:0").expect("occupy UDP port");
    let mut child = spawn_plugin(
        &occupied.local_addr().unwrap().to_string(),
        journal_dir.path(),
    );
    let _stdin = child.stdin.take().expect("piped stdin");
    let lines = stdout_lines_with_disconnect(&mut child, disconnect);
    let stderr_pipe = child.stderr.take().unwrap();
    let (stderr_tx, stderr_rx) = mpsc::channel();
    std::thread::spawn(move || {
        for line in BufReader::new(stderr_pipe).lines() {
            let Ok(line) = line else { break };
            if stderr_tx.send(line).is_err() {
                break;
            }
        }
    });
    let mut error_output = Vec::new();
    // Connecting the FIFO writer proves that startup reached the cache read.
    // Keep it open without data so the read remains blocked through a full
    // heartbeat interval. Nonblocking open bounds failures to reach the reader.
    let reader_deadline = Instant::now() + STARTUP_TIMEOUT;
    let mut cache_writer = loop {
        match OpenOptions::new()
            .write(true)
            .custom_flags(libc::O_NONBLOCK)
            .open(&cache_path)
        {
            Ok(file) => break Some(file),
            Err(err)
                if err.raw_os_error() == Some(libc::ENXIO) && Instant::now() < reader_deadline =>
            {
                std::thread::sleep(Duration::from_millis(20));
            }
            Err(_) => break None,
        }
    };
    if cache_writer.is_none() {
        let _ = child.kill();
        let _ = child.wait();
        let stderr = stderr_rx.iter().collect::<Vec<_>>().join("\n");
        panic!("facet cache reader did not reach FIFO; stderr={stderr}");
    }

    let mut output = Vec::new();
    let mut heartbeats = 0;
    // Two heartbeats cannot both be the immediate startup write. Allow one
    // production interval plus scheduling slack without a shipped test hook.
    let deadline = Instant::now() + STARTUP_TIMEOUT * 2;
    let expected_heartbeats = if disconnect { 1 } else { 2 };
    while heartbeats < expected_heartbeats {
        let remaining = deadline.saturating_duration_since(Instant::now());
        let Ok(line) = lines.recv_timeout(remaining) else {
            break;
        };
        if keyword(&line) == "PLUGIN_KEEPALIVE" {
            heartbeats += 1;
        }
        output.push(line);
    }
    if heartbeats < expected_heartbeats {
        let _ = child.kill();
        let _ = child.wait();
        let stderr = stderr_rx.iter().collect::<Vec<_>>().join("\n");
        panic!(
            "no recurring keepalive while facet cache loading was blocked; stdout={output:?}; stderr={stderr}"
        );
    }
    if disconnect {
        // The stdout reader dropped its pipe after the first heartbeat. The
        // next write must report failure without waiting for this cache read.
        let deadline = Instant::now() + STARTUP_TIMEOUT * 2;
        let mut reported = false;
        while let Ok(line) =
            stderr_rx.recv_timeout(deadline.saturating_duration_since(Instant::now()))
        {
            reported |= line
                .contains("failed to send PLUGIN_KEEPALIVE during NetFlow initialization")
                && line.contains("Broken pipe");
            error_output.push(line);
            if reported {
                break;
            }
        }
        if !reported {
            let _ = child.kill();
            let _ = child.wait();
            error_output.extend(stderr_rx.iter());
            panic!("keepalive failure hidden behind blocked cache read: {error_output:?}");
        }
    }
    let released = cache_writer
        .as_mut()
        .unwrap()
        .write_all(b"invalid cache")
        .is_ok();
    drop(cache_writer);
    if !released {
        let _ = child.kill();
        let _ = child.wait();
        let stderr = stderr_rx.iter().collect::<Vec<_>>().join("\n");
        panic!("could not release facet cache reader; stderr={stderr}");
    }
    let status = wait_with_deadline(&mut child, STARTUP_TIMEOUT);
    error_output.extend(stderr_rx.iter());
    let stderr = error_output.join("\n");
    output.extend(lines.iter());
    assert_eq!(status.and_then(|s| s.code()), Some(1), "{stderr}");
    assert!(
        output
            .iter()
            .all(|line| UNPUBLISHED_KEYWORDS.contains(&keyword(line))),
        "{output:?}"
    );
    if disconnect {
        assert!(
            !stderr.contains("starting netflow facet reconciliation"),
            "{stderr}"
        );
        assert!(
            !stderr.contains("scanning directory:"),
            "must stop before query and writer setup: {stderr}"
        );
    } else {
        assert!(stderr.contains("failed to bind"), "{stderr}");
    }
}

#[test]
fn unreadable_facet_journal_logs_underlying_error() {
    let journal_dir = tempfile::tempdir().expect("create journal dir");
    let raw = journal_dir.path().join("raw");
    std::fs::create_dir(&raw).unwrap();
    let path = raw
        .join("system@11111111111111111111111111111111-0000000000000001-00000000000f4240.journal");
    // Deliberately malformed: exercise the journal reader's nested error chain.
    std::fs::write(&path, b"invalid journal").unwrap();
    let occupied = UdpSocket::bind("127.0.0.1:0").unwrap();
    let mut child = spawn_plugin(
        &occupied.local_addr().unwrap().to_string(),
        journal_dir.path(),
    );
    let _stdin = child.stdin.take().unwrap();
    let lines = stdout_lines(&mut child);
    let stderr = stderr_text(&mut child);
    let status = wait_with_deadline(&mut child, STARTUP_TIMEOUT);
    let stderr = stderr.join().unwrap();
    let output: Vec<_> = lines.iter().collect();
    assert_eq!(status.and_then(|s| s.code()), Some(1), "{stderr}");
    assert!(
        output
            .iter()
            .all(|line| UNPUBLISHED_KEYWORDS.contains(&keyword(line)))
    );
    let warning = stderr
        .lines()
        .find(|line| {
            line.contains("skipping unreadable journal file")
                && line.contains("during netflow facet contribution scan")
        })
        .expect("facet scan warning");
    assert!(warning.contains(&path.display().to_string()), "{warning}");
    assert!(
        warning.contains("object exceeds file bounds"),
        "root cause missing: {warning}"
    );
}
