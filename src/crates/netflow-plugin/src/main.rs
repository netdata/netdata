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
mod local_journal_host;
mod memory_allocator;
mod memory_estimation;
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
mod test_cli;
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
    if let Err(err) = journal_sdk_core::install_sigbus_handler() {
        eprintln!("failed to install SIGBUS handler: {}", err);
        std::process::exit(1);
    }

    match test_cli::TestCommand::parse_from_env_args() {
        Ok(Some(command)) => {
            let worker_threads = runtime_worker_threads();
            let max_blocking_threads = runtime_blocking_threads(worker_threads);
            let runtime = match build_tokio_runtime(worker_threads, max_blocking_threads) {
                Ok(runtime) => runtime,
                Err(err) => {
                    eprintln!("failed to build tokio runtime: {}", err);
                    std::process::exit(1);
                }
            };
            if let Err(err) = runtime.block_on(test_cli::run(command)) {
                eprintln!("{err:#}");
                std::process::exit(1);
            }
            return;
        }
        Ok(None) => {}
        Err(err) => {
            eprintln!("{err}");
            std::process::exit(2);
        }
    }

    #[cfg(all(target_os = "linux", target_env = "gnu"))]
    let glibc_arena_max = memory_allocator::limit_glibc_arenas_for_process();

    println!("TRUST_DURATIONS 1");
    rt::init_tracing_with_identifier("netflow-plugin");

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

    let runtime = match build_tokio_runtime(worker_threads, max_blocking_threads) {
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
    if let Err(err) = query_service.initialize_facets_before_ingest().await {
        tracing::error!("failed to initialize facet runtime: {err:#}");
        return 1;
    }
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
    let _charts_task = charts::NetflowCharts::new(&mut runtime, &config.charts).spawn_sampler(
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
        resident_mapping_paths,
        config.charts.clone(),
        shutdown.clone(),
    );

    let listener_ready = CancellationToken::new();
    let direction_migration_pending = facet_runtime.direction_migration_pending();
    let query_service_for_events = Arc::clone(&query_service);
    let notify_listener_ready = listener_ready.clone();
    let notify_shutdown = shutdown.clone();
    let notify_task = tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        if direction_migration_pending
            && !wait_for_listener_ready(&notify_listener_ready, &notify_shutdown).await
        {
            return;
        }
        loop {
            let event = tokio::select! {
                biased;
                _ = notify_shutdown.cancelled() => break,
                event = notify_rx.recv() => event,
            };
            let Some(event) = event else {
                break;
            };
            let mut reconcile_required = query_service_for_events.process_notify_event(event);
            while let Ok(event) = notify_rx.try_recv() {
                reconcile_required |= query_service_for_events.process_notify_event(event);
            }

            if reconcile_required {
                match query_service_for_events
                    .initialize_facets_cancellable(notify_shutdown.clone())
                    .await
                {
                    Ok(true) => {}
                    Ok(false) => break,
                    Err(err) => {
                        tracing::warn!(
                            "netflow facet reconcile after file notification failed: {err:#}"
                        );
                    }
                }
            }
        }
        tracing::info!("netflow journal notify event task terminated");
    });

    let ingest_shutdown = shutdown.clone();
    let ingest_listener_ready = listener_ready.clone();
    let publish_listener_ready = listener_ready.clone();
    let ingest_task = tokio::spawn(async move {
        ingest_service
            .run_with_listener_ready_signal(ingest_shutdown, ingest_listener_ready)
            .await
    });
    let direction_migration_task = if direction_migration_pending {
        let migration_query_service = Arc::clone(&query_service);
        let migration_listener_ready = listener_ready;
        let migration_shutdown = shutdown.clone();
        Some(tokio::spawn(async move {
            if !wait_for_listener_ready(&migration_listener_ready, &migration_shutdown).await {
                return;
            }
            tracing::info!("starting background netflow DIRECTION facet migration");
            loop {
                match migration_query_service
                    .migrate_direction_facets(migration_shutdown.clone())
                    .await
                {
                    Ok(true) => {
                        tracing::info!("completed background netflow DIRECTION facet migration");
                        return;
                    }
                    Ok(false) if migration_shutdown.is_cancelled() => return,
                    Ok(false) => {
                        tracing::warn!(
                            "retained journals changed during DIRECTION facet migration; retrying"
                        );
                    }
                    Err(err) => {
                        tracing::warn!(
                            "background netflow DIRECTION facet migration failed; retrying: {err:#}"
                        );
                    }
                }
                tokio::select! {
                    _ = migration_shutdown.cancelled() => return,
                    _ = tokio::time::sleep(Duration::from_secs(30)) => {}
                }
            }
        }))
    } else {
        None
    };
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
    let mut ingest_task = ingest_task;
    let mut ingest_task_finished = false;

    // `runtime.run()` publishes: it declares the Function and starts the chart
    // registry. The Agent counts either as collected data. A plugin that exits
    // with an error before collecting anything is disabled; one that collected
    // something is restarted every 10 * update_every seconds, and a startup
    // failure that follows publication collects something on every run, so
    // the restarts never stop. So nothing is published until the ingest
    // service has finished its fallible startup (tier rebuild, every listener
    // bound). Keepalives cover the wait, which the Agent otherwise times out
    // after two minutes of silence.
    let keepalive_writer = runtime.writer();
    let ingest_started = tokio::select! {
        biased;
        _ = publish_listener_ready.cancelled() => true,
        result = &mut ingest_task => {
            ingest_task_finished = true;
            exit_code = ingest_task_exit_code(result, false);
            false
        }
        err = async move {
            let mut interval = tokio::time::interval(Duration::from_secs(60));
            loop {
                interval.tick().await;
                if let Err(err) = keepalive_writer.lock().await.write_raw(b"PLUGIN_KEEPALIVE\n").await {
                    break err;
                }
            }
        }, if !std::io::stdout().is_terminal() => {
            tracing::error!(
                "failed to send PLUGIN_KEEPALIVE to the Agent while waiting for the UDP listeners \
                 (expected the write to stdout to succeed): {err}"
            );
            exit_code = 1;
            false
        }
    };

    // Only a failed keepalive ends the wait with the ingest task still running.
    // Its tier rebuild does not observe `shutdown`, so abort it rather than let
    // an Agent that is already gone wait for the rebuild to finish.
    if !ingest_started && !ingest_task_finished {
        ingest_task.abort();
    }

    if ingest_started {
        tokio::select! {
            result = runtime.run() => {
                if let Err(err) = result {
                    tracing::error!("plugin runtime error: {err:#}");
                    exit_code = 1;
                }
            }
            result = &mut ingest_task => {
                ingest_task_finished = true;
                exit_code = ingest_task_exit_code(result, false);
            }
        }
    }

    shutdown.cancel();

    if !ingest_task_finished {
        exit_code = exit_code.max(ingest_task_exit_code(ingest_task.await, true));
    }
    if let Some(task) = direction_migration_task {
        exit_code = exit_code.max(task_join_exit_code(task.await, "DIRECTION migration task"));
    }
    exit_code = exit_code.max(task_join_exit_code(
        notify_task.await,
        "journal notify event task",
    ));
    if let Some(task) = bmp_task {
        exit_code = exit_code.max(task_join_exit_code(task.await, "BMP listener task"));
    }
    if let Some(task) = bioris_task {
        exit_code = exit_code.max(task_join_exit_code(task.await, "BioRIS listener task"));
    }
    if let Some(task) = network_sources_task {
        exit_code = exit_code.max(task_join_exit_code(task.await, "network-sources task"));
    }

    exit_code
}

/// Log the ingestion task's outcome and map it to the process exit code.
/// `shutdown_requested` says whether the task was asked to stop; if not, even a
/// clean return is a failure, because ingestion only ends on shutdown.
fn ingest_task_exit_code(
    result: Result<anyhow::Result<()>, tokio::task::JoinError>,
    shutdown_requested: bool,
) -> i32 {
    match result {
        Ok(Ok(())) if shutdown_requested => 0,
        Ok(Ok(())) => {
            tracing::error!("ingestion task exited unexpectedly");
            1
        }
        Ok(Err(err)) => {
            tracing::error!("ingestion task error: {err:#}");
            1
        }
        Err(err) if !err.is_cancelled() => {
            tracing::error!("ingestion task join error: {err}");
            1
        }
        Err(_) => 0,
    }
}

/// Log a background task that ended in a panic and map it to the exit code.
fn task_join_exit_code(result: Result<(), tokio::task::JoinError>, task: &str) -> i32 {
    match result {
        Err(err) if !err.is_cancelled() => {
            tracing::error!("{task} join error: {err}");
            1
        }
        _ => 0,
    }
}

async fn wait_for_listener_ready(
    listener_ready: &CancellationToken,
    shutdown: &CancellationToken,
) -> bool {
    tokio::select! {
        biased;
        _ = shutdown.cancelled() => false,
        _ = listener_ready.cancelled() => true,
    }
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

fn build_tokio_runtime(
    worker_threads: usize,
    max_blocking_threads: usize,
) -> std::io::Result<tokio::runtime::Runtime> {
    tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .worker_threads(worker_threads)
        .max_blocking_threads(max_blocking_threads)
        .build()
}

#[cfg(test)]
#[path = "main_tests.rs"]
mod tests;
