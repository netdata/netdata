#![allow(dead_code)]

mod collectors;
mod samples_chart;

use collectors::{ScalarChartCollector, SummaryChartCollector};
use otel_utils::{DataPointCollector, OtelSort, ScopeConfig};
use samples_chart::{SamplesChart, SamplesChartCollector};

use opentelemetry_proto::tonic::{
    collector::metrics::v1::{
        metrics_service_server::{MetricsService, MetricsServiceServer},
        ExportMetricsServiceRequest, ExportMetricsServiceResponse,
    },
    common::v1::InstrumentationScope,
};
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use tonic::{codec::CompressionEncoding, transport::Server, Response};

struct MetricsCollector {
    cfg: RwLock<otel_utils::Config>,
    charts: RwLock<HashMap<String, SamplesChart>>,
}

impl MetricsCollector {
    fn new(cfg: otel_utils::Config) -> Self {
        Self {
            cfg: RwLock::new(cfg),
            charts: Default::default(),
        }
    }

    fn lookup_scope_config(
        &self,
        scope: &Option<InstrumentationScope>,
    ) -> Result<Option<Arc<ScopeConfig>>, tonic::Status> {
        if let Some(scope) = scope {
            {
                let cfg = self
                    .cfg
                    .read()
                    .map_err(|_| tonic::Status::internal("Failed to acquire config lock"))?;

                if let Some(scope_cfg) = cfg.scope_config(&scope.name) {
                    return Ok(Some(scope_cfg));
                }
            }

            let mut cfg = self
                .cfg
                .write()
                .map_err(|_| tonic::Status::internal("Failed to acquire config lock"))?;

            if let Some(scope_cfg) = cfg.match_scope(&scope.name) {
                cfg.insert_scope(&scope.name, scope_cfg.clone());
                Ok(Some(scope_cfg))
            } else {
                Ok(None)
            }
        } else {
            Ok(None)
        }
    }
}

#[tonic::async_trait]
impl MetricsService for MetricsCollector {
    async fn export(
        &self,
        request: tonic::Request<ExportMetricsServiceRequest>,
    ) -> Result<tonic::Response<ExportMetricsServiceResponse>, tonic::Status> {
        let mut metrics = request.into_inner();

        for rm in metrics.resource_metrics.iter_mut() {
            rm.otel_sort();
        }

        let mut dpc = DataPointCollector::new(&self.cfg);

        let mut gauge_points = Vec::with_capacity(1024);
        let mut sum_points = Vec::with_capacity(1024);
        let mut histogram_points = Vec::with_capacity(1024);
        let mut exponential_histogram_points = Vec::with_capacity(1024);
        let mut summary_points = Vec::with_capacity(1024);

        dpc.collect(
            &metrics.resource_metrics,
            &mut gauge_points,
            &mut sum_points,
            &mut histogram_points,
            &mut exponential_histogram_points,
            &mut summary_points,
        )?;

        let mut guard = self
            .charts
            .write()
            .map_err(|_| tonic::Status::internal("Failed to acquire config lock"))?;

        let mut scalar_chart_collector = ScalarChartCollector::default();

        for points in sum_points.chunk_by(|a, b| a.hash == b.hash) {
            scalar_chart_collector.ingest(points);

            let instance_name = scalar_chart_collector.chart_name();

            if !guard.contains_key(instance_name) {
                guard.insert(String::from(instance_name), SamplesChart::default());
            }

            let chart = guard.get_mut(instance_name).expect("A valid chart entry");
            scalar_chart_collector.emit_sample_points(chart);
        }

        for points in gauge_points.chunk_by(|a, b| a.hash == b.hash) {
            scalar_chart_collector.ingest(points);

            let instance_name = scalar_chart_collector.chart_name();

            if !guard.contains_key(instance_name) {
                guard.insert(String::from(instance_name), SamplesChart::default());
            }

            let chart = guard.get_mut(instance_name).expect("A valid chart entry");
            scalar_chart_collector.emit_sample_points(chart);
        }

        let mut summary_chart_collector = SummaryChartCollector::default();

        for points in summary_points.chunk_by(|a, b| a.hash == b.hash) {
            summary_chart_collector.ingest(points);

            let instance_name = summary_chart_collector.chart_name();

            if !guard.contains_key(instance_name) {
                guard.insert(String::from(instance_name), SamplesChart::default());
            }

            let chart = guard.get_mut(instance_name).expect("A valid chart entry");
            summary_chart_collector.emit_sample_points(chart);
        }

        Ok(Response::new(ExportMetricsServiceResponse::default()))
    }
}

fn print_usage() {
    println!("Usage: <otel.plugin> [OPTIONS]");
    println!("Options:");
    println!("  -c, --config <FILE>   Path to OpenTelemetry receivers configuration file");
    println!("  -p, --port <PORT>     Port to listen on (default: 21212)");
    println!("  -b, --bind <ADDRESS>  IP address to bind to (default: 0.0.0.0)");
    println!("  -h, --help           Print this help message");
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    let mut config_path = std::env::var("NETDATA_CONFIG_DIR")
        .ok()
        .map(|p| std::path::PathBuf::from(p + "/otel-receivers-config.yaml"));
    let mut port = 21212;
    let mut bind_addr = String::from("0.0.0.0");

    if config_path.is_none() {
        let mut i = 1;
        while i < args.len() {
            match args[i].as_str() {
                "-h" | "--help" => {
                    print_usage();
                    return Ok(());
                }
                "-c" | "--config" => {
                    if i + 1 < args.len() {
                        config_path = Some(std::path::PathBuf::from(&args[i + 1]));
                        i += 2;
                    } else {
                        eprintln!("Error: Missing value for --config option");
                        print_usage();
                        std::process::exit(1);
                    }
                }
                "-p" | "--port" => {
                    if i + 1 < args.len() {
                        match args[i + 1].parse() {
                            Ok(p) => {
                                port = p;
                                i += 2;
                            }
                            Err(_) => {
                                eprintln!("Error: Invalid port number");
                                print_usage();
                                std::process::exit(1);
                            }
                        }
                    } else {
                        eprintln!("Error: Missing value for --port option");
                        print_usage();
                        std::process::exit(1);
                    }
                }
                "-b" | "--bind" => {
                    if i + 1 < args.len() {
                        bind_addr = args[i + 1].clone();
                        i += 2;
                    } else {
                        eprintln!("Error: Missing value for --bind option");
                        print_usage();
                        std::process::exit(1);
                    }
                }
                _ => {
                    eprintln!("Error: Unknown option: {}", args[i]);
                    print_usage();
                    std::process::exit(1);
                }
            }
        }
    }

    let config_path = config_path.expect("missing configuration file (--config <path>)");
    let collector = MetricsCollector::new(otel_utils::Config::load(config_path)?);
    let addr = format!("{}:{}", bind_addr, port).parse()?;

    Server::builder()
        .add_service(
            MetricsServiceServer::new(collector)
                .accept_compressed(CompressionEncoding::Gzip)
                .send_compressed(CompressionEncoding::Gzip),
        )
        .serve(addr)
        .await?;

    Ok(())
}
