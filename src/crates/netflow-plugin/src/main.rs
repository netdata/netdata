//! netflow-plugin standalone binary

mod api;
mod charts;
mod decoder;
mod enrichment;
mod facet_catalog;
mod facet_runtime;
mod flow;
#[allow(dead_code)]
mod flow_index;
mod ingest;
mod memory_allocator;
#[cfg(test)]
mod memory_tests;
mod network_sources;
mod plugin_config;
mod presentation;
mod query;
#[cfg(test)]
mod rollup;
mod routing;
#[cfg(test)]
mod startup_memory_tests;
mod tiering;

pub(crate) use api::NetflowFlowsHandler;
#[cfg(test)]
pub(crate) use api::{
    FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse,
    flows_required_params,
};

use rt::PluginRuntime;
use std::io::{IsTerminal, Write};
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio_util::sync::CancellationToken;

const MAX_RUNTIME_WORKER_THREADS: usize = 4;
const MIN_RUNTIME_BLOCKING_THREADS: usize = 8;

fn main() {
    if let Err(err) = journal_core::install_sigbus_handler() {
        eprintln!("failed to install SIGBUS handler: {}", err);
        std::process::exit(1);
    }

    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    let glibc_arena_max = memory_allocator::limit_glibc_arenas_for_process();

    println!("TRUST_DURATIONS 1");
    rt::init_tracing();

    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    match glibc_arena_max {
        Some(arena_max) => {
            tracing::info!(arena_max, "capped glibc malloc arenas for netflow process");
        }
        None => {
            tracing::warn!("failed to cap glibc malloc arenas for netflow process");
        }
    }

    #[cfg(target_os = "linux")]
    {
        if memory_allocator::disable_transparent_huge_pages_for_process() {
            tracing::info!("disabled transparent huge pages for netflow process");
        } else {
            tracing::warn!("failed to disable transparent huge pages for netflow process");
        }
    }

    let worker_threads = runtime_worker_threads();
    let max_blocking_threads = runtime_blocking_threads(worker_threads);
    tracing::info!(
        worker_threads,
        max_blocking_threads,
        "configured netflow tokio runtime"
    );

    let runtime = match tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .worker_threads(worker_threads)
        .max_blocking_threads(max_blocking_threads)
        .build()
    {
        Ok(runtime) => runtime,
        Err(err) => {
            eprintln!("failed to build tokio runtime: {}", err);
            std::process::exit(1);
        }
    };

    let exit_code = runtime.block_on(async_main());
    if exit_code != 0 {
        std::process::exit(exit_code);
    }
}

async fn async_main() -> i32 {
    let config = match plugin_config::PluginConfig::new() {
        Ok(cfg) => cfg,
        Err(err) => {
            tracing::error!("failed to load configuration: {err:#}");
            return 1;
        }
    };

    if !config.enabled {
        tracing::info!("netflow plugin disabled by config (enabled=false)");
        if !std::io::stdout().is_terminal() {
            let mut stdout = std::io::stdout();
            let _ = stdout.write_all(b"DISABLE\n");
            let _ = stdout.flush();
        }
        return 0;
    }

    let shutdown = CancellationToken::new();
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let facet_runtime = Arc::new(facet_runtime::FacetRuntime::new(&config.journal.base_dir()));
    let (query_service, notify_rx) =
        match query::FlowQueryService::new_with_facet_runtime(&config, Arc::clone(&facet_runtime))
            .await
        {
            Ok(service) => service,
            Err(err) => {
                tracing::error!("failed to initialize query service: {err:#}");
                return 1;
            }
        };
    let query_service = Arc::new(query_service);
    if let Err(err) = query_service.initialize_facets().await {
        tracing::error!("failed to initialize facet runtime: {err:#}");
        return 1;
    }

    let ingest_service = match ingest::IngestService::new_with_facet_runtime(
        config.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
    ) {
        Ok(service) => service,
        Err(err) => {
            tracing::error!("failed to initialize ingestion service: {err:#}");
            return 1;
        }
    };
    let routing_runtime = ingest_service.routing_runtime();
    let network_sources_runtime = ingest_service.network_sources_runtime();

    let mut runtime = PluginRuntime::new("netflow-plugin");
    runtime.register_handler(NetflowFlowsHandler::new(
        Arc::clone(&metrics),
        Arc::clone(&query_service),
    ));
    let resident_mapping_paths = charts::ProcessResidentMappingPaths::new(
        &config.journal.raw_tier_dir(),
        &config.journal.minute_1_tier_dir(),
        &config.journal.minute_5_tier_dir(),
        &config.journal.hour_1_tier_dir(),
        &config.enrichment.geoip.asn_database,
        &config.enrichment.geoip.geo_database,
    );
    let _charts_task = charts::NetflowCharts::new(&mut runtime).spawn_sampler(
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
        resident_mapping_paths,
        shutdown.clone(),
    );

    let query_service_for_events = Arc::clone(&query_service);
    tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        while let Some(event) = notify_rx.recv().await {
            let mut reconcile_required = query_service_for_events.process_notify_event(event);
            while let Ok(event) = notify_rx.try_recv() {
                reconcile_required |= query_service_for_events.process_notify_event(event);
            }

            if reconcile_required
                && let Err(err) = query_service_for_events.initialize_facets().await
            {
                tracing::warn!("netflow facet reconcile after file notification failed: {err:#}");
            }
        }
        tracing::info!("netflow journal notify event task terminated");
    });

    let ingest_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { ingest_service.run(ingest_shutdown).await });
    let mut bmp_task = None;
    if config.enrichment.routing_dynamic.bmp.enabled {
        if let Some(runtime_state) = routing_runtime.clone() {
            let bmp_cfg = config.enrichment.routing_dynamic.bmp.clone();
            let bmp_shutdown = shutdown.clone();
            bmp_task = Some(tokio::spawn(async move {
                if let Err(err) =
                    routing::run_bmp_listener(bmp_cfg, runtime_state, bmp_shutdown).await
                {
                    tracing::error!("dynamic BMP routing listener failed: {err:#}");
                }
            }));
        } else {
            tracing::warn!(
                "dynamic BMP routing is enabled but enrichment runtime is unavailable; listener not started"
            );
        }
    }
    let mut bioris_task = None;
    if config.enrichment.routing_dynamic.bioris.enabled {
        if let Some(runtime_state) = routing_runtime.clone() {
            let bioris_cfg = config.enrichment.routing_dynamic.bioris.clone();
            let bioris_metrics = Arc::clone(&metrics);
            let bioris_shutdown = shutdown.clone();
            bioris_task = Some(tokio::spawn(async move {
                if let Err(err) = routing::run_bioris_listener(
                    bioris_cfg,
                    runtime_state,
                    bioris_metrics,
                    bioris_shutdown,
                )
                .await
                {
                    tracing::error!("dynamic BioRIS routing listener failed: {err:#}");
                }
            }));
        } else {
            tracing::warn!(
                "dynamic BioRIS routing is enabled but enrichment runtime is unavailable; listener not started"
            );
        }
    }
    let mut network_sources_task = None;
    if let Some(runtime_state) = network_sources_runtime {
        let network_sources_cfg = config.enrichment.network_sources.clone();
        if !network_sources_cfg.is_empty() {
            let sources_shutdown = shutdown.clone();
            network_sources_task = Some(tokio::spawn(async move {
                if let Err(err) = network_sources::run_network_sources_refresher(
                    network_sources_cfg,
                    runtime_state,
                    sources_shutdown,
                )
                .await
                {
                    tracing::error!("network-sources refresher failed: {err:#}");
                }
            }));
        }
    }

    let mut exit_code = 0;
    let keepalive_required = !std::io::stdout().is_terminal();
    let mut ingest_task = ingest_task;
    let mut ingest_task_finished = false;

    tokio::select! {
        result = async {
            if keepalive_required {
                let writer = runtime.writer();
                let keepalive = async move {
                    let mut interval = tokio::time::interval(Duration::from_secs(60));
                    loop {
                        interval.tick().await;
                        if let Ok(mut w) = writer.try_lock() {
                            let _ = w.write_raw(b"PLUGIN_KEEPALIVE\n").await;
                        }
                    }
                };

                tokio::select! {
                    result = runtime.run() => result,
                    _ = keepalive => Ok(()),
                }
            } else {
                runtime.run().await
            }
        } => {
            if let Err(err) = result {
                tracing::error!("plugin runtime error: {err:#}");
                exit_code = 1;
            }
        }
        result = &mut ingest_task => {
            ingest_task_finished = true;
            match result {
                Ok(Ok(())) => {
                    tracing::error!("ingestion task exited unexpectedly");
                    exit_code = 1;
                }
                Ok(Err(err)) => {
                    tracing::error!("ingestion task error: {err:#}");
                    exit_code = 1;
                }
                Err(err) if !err.is_cancelled() => {
                    tracing::error!("ingestion task join error: {err}");
                    exit_code = 1;
                }
                Err(_) => {}
            }
        }
    }

    shutdown.cancel();

    if !ingest_task_finished {
        match ingest_task.await {
            Ok(Ok(())) => {}
            Ok(Err(err)) => {
                tracing::error!("ingestion task error: {err:#}");
                exit_code = 1;
            }
            Err(err) if !err.is_cancelled() => {
                tracing::error!("ingestion task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }
    if let Some(task) = bmp_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("BMP listener task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }
    if let Some(task) = bioris_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("BioRIS listener task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }
    if let Some(task) = network_sources_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("network-sources task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }

    exit_code
}

fn runtime_worker_threads() -> usize {
    std::thread::available_parallelism()
        .map(|value| value.get())
        .unwrap_or(1)
        .clamp(1, MAX_RUNTIME_WORKER_THREADS)
}

fn runtime_blocking_threads(worker_threads: usize) -> usize {
    MIN_RUNTIME_BLOCKING_THREADS.max(worker_threads)
}

#[cfg(test)]
#[path = "main_tests.rs"]
mod tests;
