//! journal-viewer-plugin standalone binary

#![allow(dead_code)]

use journal_function::JournalMetrics;
use journal_registry::Monitor;

mod catalog;
use catalog::CatalogFunction;

mod charts;
use charts::Metrics;

mod plugin_config;
use plugin_config::PluginConfig;

use rt::PluginRuntime;
use tracing::{error, info};

#[tokio::main]
async fn main() {
    println!("TRUST_DURATIONS 1");

    rt::init_tracing("info");

    let result = run_plugin().await;

    match result {
        Ok(()) => {
            info!("plugin runtime stopped");
        }
        Err(e) => {
            error!("plugin error: {:#}", e);
            std::process::exit(1);
        }
    }
}

async fn run_plugin() -> std::result::Result<(), Box<dyn std::error::Error>> {
    // Load configuration
    let plugin_config = PluginConfig::new()?;
    let config = &plugin_config.config;

    info!(
        "configuration loaded: journal_paths={:?}, cache_dir={}, memory_capacity={}, disk_capacity={}, workers={}",
        config.journal.paths,
        config.cache.directory,
        config.cache.memory_capacity,
        config.cache.disk_capacity,
        config.cache.workers
    );

    let mut runtime = PluginRuntime::new("journal-viewer");
    info!("plugin runtime created");

    // Initialize plugin-level metrics
    let _plugin_metrics = Metrics::new(&mut runtime);
    info!("plugin metrics initialized");

    // Initialize journal-function metrics
    let journal_metrics = JournalMetrics::new(&mut runtime);
    info!("journal metrics initialized");

    let (monitor, notify_rx) = match Monitor::new() {
        Ok(t) => t,
        Err(e) => {
            error!("failed to setup notify monitoring: {}", e);
            return Ok(());
        }
    };

    // Create catalog function with disk-backed cache
    info!("creating catalog function with Foyer hybrid cache");
    let catalog_function = CatalogFunction::new(
        monitor,
        &config.cache.directory,
        config.cache.memory_capacity,
        config.cache.disk_capacity.as_u64() as usize,
        journal_metrics.file_indexing.clone(),
        journal_metrics.bucket_cache.clone(),
        journal_metrics.bucket_operations.clone(),
    )
    .await?;
    info!("catalog function initialized");

    // Watch configured journal directories
    for path in &config.journal.paths {
        match catalog_function.watch_directory(path) {
            Ok(()) => {
                info!("watching journal directory: {}", path);
            }
            Err(e) => {
                error!("failed to watch directory {}: {:#?}", path, e);
            }
        }
    }

    runtime.register_handler(catalog_function.clone());
    info!("catalog function handler registered");

    // Spawn task to process notify events
    let catalog_function_clone = catalog_function.clone();
    tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        while let Some(event) = notify_rx.recv().await {
            catalog_function_clone.process_notify_event(event);
        }
        info!("notify event processing task terminated");
    });

    // Keepalive future to prevent Netdata from killing the plugin
    let writer = runtime.writer();
    let keepalive = async move {
        let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(60));
        loop {
            interval.tick().await;
            if let Ok(mut w) = writer.try_lock() {
                let _ = w.write_raw(b"PLUGIN_KEEPALIVE\n").await;
            }
        }
    };

    info!("starting plugin runtime");

    // Run plugin runtime and keepalive concurrently
    tokio::select! {
        result = runtime.run() => {
            result?;
        }
        _ = keepalive => {
            // Keepalive loop never completes normally
        }
    }

    Ok(())
}
