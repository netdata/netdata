use anyhow::{Context, Result};
use flatten_otel::flatten_metrics_request;
use opentelemetry_proto::tonic::collector::metrics::v1::{
    metrics_service_server::MetricsService, ExportMetricsServiceRequest,
    ExportMetricsServiceResponse,
};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tonic::{Request, Response, Status};

use crate::chart_config::ChartConfigManager;
use crate::flattened_point::FlattenedPoint;
use crate::netdata_chart::NetdataChart;
use crate::plugin_config::PluginConfig;
use crate::regex_cache::RegexCache;

pub struct NetdataMetricsService {
    regex_cache: RegexCache,
    charts: Arc<RwLock<HashMap<String, NetdataChart>>>,
    config: Arc<PluginConfig>,
    chart_config_manager: ChartConfigManager,
    call_count: std::sync::atomic::AtomicU64,
}

impl NetdataMetricsService {
    pub fn new(config: PluginConfig) -> Result<Self> {
        let mut chart_config_manager = ChartConfigManager::with_default_configs();

        // Load user chart configs if directory is specified
        if let Some(chart_configs_dir) = &config.metrics.chart_configs_dir {
            chart_config_manager
                .load_user_configs(chart_configs_dir)
                .with_context(|| {
                    format!(
                        "Failed to load chart configs from directory: {}",
                        chart_configs_dir
                    )
                })?;
        }

        Ok(Self {
            regex_cache: RegexCache::default(),
            charts: Arc::default(),
            config: Arc::new(config),
            chart_config_manager,
            call_count: std::sync::atomic::AtomicU64::new(0),
        })
    }

    async fn cleanup_stale_charts(&self, max_age: std::time::Duration) {
        let now = std::time::SystemTime::now();

        let mut guard = self.charts.write().await;
        guard.retain(|_, chart| {
            let Some(chart_time) = chart.last_collection_time() else {
                return true;
            };

            now.duration_since(chart_time)
                .unwrap_or(std::time::Duration::ZERO)
                < max_age
        });
    }
}

#[tonic::async_trait]
impl MetricsService for NetdataMetricsService {
    async fn export(
        &self,
        request: Request<ExportMetricsServiceRequest>,
    ) -> Result<Response<ExportMetricsServiceResponse>, Status> {
        let req = request.into_inner();

        let flattened_points = flatten_metrics_request(&req)
            .into_iter()
            .filter_map(|jm| {
                let cfg = self.chart_config_manager.find_matching_config(&jm);
                FlattenedPoint::new(jm, cfg, &self.regex_cache)
            })
            .collect::<Vec<_>>();

        if self.config.metrics.print_flattened {
            // Just print the flattened points
            for fp in &flattened_points {
                println!("{:#?}", fp);
            }

            return Ok(Response::new(ExportMetricsServiceResponse {
                partial_success: None,
            }));
        }

        // ingest
        {
            let mut newly_created_charts = 0;

            for fp in flattened_points.iter() {
                let mut guard = self.charts.write().await;

                if let Some(netdata_chart) = guard.get_mut(&fp.nd_instance_name) {
                    netdata_chart.ingest(fp);
                } else if newly_created_charts < self.config.metrics.throttle_charts {
                    let mut netdata_chart =
                        NetdataChart::from_flattened_point(fp, self.config.metrics.buffer_samples);
                    netdata_chart.ingest(fp);
                    guard.insert(fp.nd_instance_name.clone(), netdata_chart);

                    newly_created_charts += 1;
                }
            }
        }

        // process
        {
            let mut guard = self.charts.write().await;
            let mut output_buffer = String::new();

            for netdata_chart in guard.values_mut() {
                netdata_chart.process(&mut output_buffer);
            }

            // Write chart data to stdout
            print!("{}", output_buffer);
        }

        // cleanup stale charts
        {
            let prev_count = self
                .call_count
                .fetch_add(1, std::sync::atomic::Ordering::Relaxed);

            if prev_count % 60 == 0 {
                let one_hour = std::time::Duration::from_secs(3600);
                self.cleanup_stale_charts(one_hour).await;
            }
        }

        Ok(Response::new(ExportMetricsServiceResponse {
            partial_success: None,
        }))
    }
}
