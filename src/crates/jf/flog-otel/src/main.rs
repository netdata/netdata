use anyhow::Result;
use chrono::{DateTime, Utc};
use clap::Parser;
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, logs_service_client::LogsServiceClient,
};
use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;
use serde::Deserialize;
use std::process::Stdio;
use std::sync::Arc;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::sync::Mutex;
use tokio::time::{Duration, Instant};
use tracing::{error, info, warn};
use tracing_subscriber;
use uuid::Uuid;

#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    #[arg(short, long, default_value = "1048576")]
    rate_limit_bytes: u64,

    #[arg(short, long, default_value = "http://127.0.0.1:21213")]
    otel_endpoint: String,

    #[arg(short, long, default_value = "1000")]
    log_count: u32,

    #[arg(long, default_value = "false")]
    loop_forever: bool,
}

#[derive(Deserialize, Debug)]
struct FlogEntry {
    host: String,
    #[serde(rename = "user-identifier")]
    user_identifier: String,
    datetime: String,
    method: String,
    request: String,
    protocol: String,
    status: u16,
    bytes: u64,
    referer: String,
}

struct RateLimiter {
    bytes_sent: u64,
    last_reset: Instant,
    limit: u64,
}

impl RateLimiter {
    fn new(limit: u64) -> Self {
        Self {
            bytes_sent: 0,
            last_reset: Instant::now(),
            limit,
        }
    }

    async fn can_send(&mut self, size: u64) -> bool {
        if self.last_reset.elapsed() >= Duration::from_secs(1) {
            self.bytes_sent = 0;
            self.last_reset = Instant::now();
        }

        if self.bytes_sent + size <= self.limit {
            self.bytes_sent += size;
            true
        } else {
            false
        }
    }
}

fn parse_flog_datetime(datetime_str: &str) -> Result<DateTime<Utc>> {
    let dt = chrono::DateTime::parse_from_str(datetime_str, "%d/%b/%Y:%H:%M:%S %z")?;
    Ok(dt.with_timezone(&Utc))
}

fn flog_to_otel(entry: FlogEntry) -> Result<LogRecord> {
    let dt = parse_flog_datetime(&entry.datetime)?;
    let time_unix_nano = dt.timestamp_nanos_opt().unwrap_or(0) as u64;

    let severity_number = match entry.status {
        200..=299 => 9,  // INFO
        300..=399 => 5,  // DEBUG
        400..=499 => 13, // WARN
        500..=599 => 17, // ERROR
        _ => 9,          // INFO
    };

    let log_message = format!(
        "{} {} {} {} {} {}",
        entry.host, entry.method, entry.request, entry.protocol, entry.status, entry.bytes
    );

    let attributes = vec![
        KeyValue {
            key: "host".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.host)),
            }),
        },
        KeyValue {
            key: "method".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.method)),
            }),
        },
        KeyValue {
            key: "request".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.request)),
            }),
        },
        KeyValue {
            key: "protocol".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.protocol)),
            }),
        },
        KeyValue {
            key: "status".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::IntValue(entry.status as i64)),
            }),
        },
        KeyValue {
            key: "response_bytes".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::IntValue(entry.bytes as i64)),
            }),
        },
        KeyValue {
            key: "referer".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.referer)),
            }),
        },
        KeyValue {
            key: "user_identifier".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(entry.user_identifier)),
            }),
        },
    ];

    let trace_id_bytes = Uuid::new_v4().as_u128().to_be_bytes().to_vec();
    let span_id_bytes = (Uuid::new_v4().as_u128() as u64).to_be_bytes().to_vec();

    Ok(LogRecord {
        time_unix_nano,
        severity_number: severity_number as i32,
        severity_text: String::new(),
        body: Some(AnyValue {
            value: Some(any_value::Value::StringValue(log_message)),
        }),
        attributes,
        dropped_attributes_count: 0,
        flags: 0,
        trace_id: trace_id_bytes,
        span_id: span_id_bytes,
        ..Default::default()
    })
}

async fn send_otel_logs(logs: Vec<LogRecord>, endpoint: &str) -> Result<()> {
    let mut client = LogsServiceClient::connect(endpoint.to_string()).await?;

    let resource_logs = vec![ResourceLogs {
        resource: Some(Resource {
            attributes: vec![
                KeyValue {
                    key: "service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(
                            "flog-otel-wrapper".to_string(),
                        )),
                    }),
                },
                KeyValue {
                    key: "service.version".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("0.1.0".to_string())),
                    }),
                },
            ],
            dropped_attributes_count: 0,
        }),
        scope_logs: vec![ScopeLogs {
            scope: Some(
                opentelemetry_proto::tonic::common::v1::InstrumentationScope {
                    name: "flog-otel-wrapper".to_string(),
                    version: "0.1.0".to_string(),
                    attributes: vec![],
                    dropped_attributes_count: 0,
                },
            ),
            log_records: logs,
            schema_url: String::new(),
        }],
        schema_url: String::new(),
    }];

    let request = tonic::Request::new(ExportLogsServiceRequest { resource_logs });

    let response = client.export(request).await?;

    if response.get_ref().partial_success.is_some() {
        let partial_success = response.get_ref().partial_success.as_ref().unwrap();
        if partial_success.rejected_log_records > 0 {
            warn!("Some logs were rejected: {}", partial_success.error_message);
        }
    }

    info!("Successfully sent logs to OTEL collector via gRPC");
    Ok(())
}

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt::init();

    let args = Args::parse();
    let rate_limiter = Arc::new(Mutex::new(RateLimiter::new(args.rate_limit_bytes)));

    info!(
        "Starting flog-otel-wrapper with rate limit: {} bytes/sec",
        args.rate_limit_bytes
    );
    info!("OTEL endpoint: {}", args.otel_endpoint);

    loop {
        let log_count_str = args.log_count.to_string();
        let mut flog_args = vec!["-f", "json", "-t", "stdout"];

        if args.loop_forever {
            flog_args.extend(&["-l"]);
        } else {
            flog_args.extend(&["-n", &log_count_str]);
        }

        let mut child = tokio::process::Command::new("flog")
            .args(&flog_args)
            .stdout(Stdio::piped())
            .spawn()?;

        let stdout = child.stdout.take().unwrap();
        let reader = BufReader::new(stdout);
        let mut lines = reader.lines();

        let mut log_batch = Vec::new();
        const BATCH_SIZE: usize = 100;

        while let Some(line) = lines.next_line().await? {
            match serde_json::from_str::<FlogEntry>(&line) {
                Ok(entry) => match flog_to_otel(entry) {
                    Ok(otel_log) => {
                        log_batch.push(otel_log);

                        if log_batch.len() >= BATCH_SIZE {
                            let batch_size = (log_batch.len() * 1024) as u64;

                            let mut limiter = rate_limiter.lock().await;
                            if limiter.can_send(batch_size).await {
                                drop(limiter);

                                if let Err(e) =
                                    send_otel_logs(log_batch.clone(), &args.otel_endpoint).await
                                {
                                    error!("Failed to send OTEL logs: {}", e);
                                }
                                log_batch.clear();
                            } else {
                                drop(limiter);
                                info!("Rate limit reached, waiting 1 second...");
                                tokio::time::sleep(Duration::from_secs(1)).await;
                            }
                        }
                    }
                    Err(e) => error!("Failed to convert to OTEL format: {}", e),
                },
                Err(e) => error!("Failed to parse JSON: {}", e),
            }
        }

        if !log_batch.is_empty() {
            let batch_size = (log_batch.len() * 1024) as u64;
            let mut limiter = rate_limiter.lock().await;
            if limiter.can_send(batch_size).await {
                drop(limiter);
                if let Err(e) = send_otel_logs(log_batch, &args.otel_endpoint).await {
                    error!("Failed to send final OTEL logs: {}", e);
                }
            }
        }

        child.wait().await?;

        if !args.loop_forever {
            break;
        }
    }

    Ok(())
}
