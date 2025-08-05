#![allow(dead_code)]

use journal_function::JournalMetrics;
use journal_registry::Monitor;

mod catalog;
use catalog::CatalogFunction;

mod charts;
use charts::Metrics;
mod tracing_config;

use rt::PluginRuntime;
use tracing::{error, info};

/// Entry point for log-viewer-plugin - can be called from multi-call binary
///
/// # Arguments
/// * `args` - Command-line arguments (should include argv[0] as "log-viewer-plugin")
///
/// # Returns
/// Exit code (0 for success, non-zero for errors)
pub fn run(args: Vec<String>) -> i32 {
    // log-viewer-plugin is async, so we need a tokio runtime
    let runtime = tokio::runtime::Runtime::new().unwrap();
    runtime.block_on(async_run(args))
}

async fn async_run(_args: Vec<String>) -> i32 {
    println!("TRUST_DURATIONS 1");

    tracing_config::initialize_tracing(tracing_config::TracingConfig::default());

    let result = run_internal().await;

    match result {
        Ok(()) => {
            info!("Plugin runtime stopped");
            0
        }
        Err(e) => {
            error!("Plugin error: {:#}", e);
            1
        }
    }
}

async fn run_internal() -> std::result::Result<(), Box<dyn std::error::Error>> {
    let mut runtime = PluginRuntime::new("log-viewer");
    info!("Plugin runtime created");

    // Initialize plugin-level metrics
    let _plugin_metrics = Metrics::new(&mut runtime);
    info!("Plugin metrics initialized");

    // Initialize journal-function metrics
    let journal_metrics = JournalMetrics::new(&mut runtime);
    info!("Journal metrics initialized");

    let (monitor, notify_rx) = match Monitor::new() {
        Ok(t) => t,
        Err(e) => {
            error!("Failed to setup notify monitoring: {}", e);
            return Ok(());
        }
    };

    // Create catalog function with disk-backed cache
    info!("Creating catalog function with Foyer hybrid cache");
    let catalog_function = CatalogFunction::new(
        monitor,
        "/mnt/ramfs/foyer-cache",
        1000,             // memory capacity
        32 * 1024 * 1024, // disk capacity: 32 MiB
        journal_metrics.file_indexing.clone(),
        journal_metrics.bucket_cache.clone(),
        journal_metrics.bucket_operations.clone(),
    )
    .await?;
    info!("Catalog function initialized");

    // let path = "/var/log/journal";
    let path = "/home/vk/repos/tmp/aws-4320";
    match catalog_function.watch_directory(path) {
        Ok(()) => {}
        Err(e) => {
            error!("Failed to watch directory: {:#?}", e);
        }
    };

    runtime.register_handler(catalog_function.clone());
    info!("Catalog function handler registered");

    // Spawn task to process notify events
    let catalog_function_clone = catalog_function.clone();
    tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        while let Some(event) = notify_rx.recv().await {
            catalog_function_clone.process_notify_event(event);
        }
        info!("Notify event processing task terminated");
    });

    info!("Starting plugin runtime - ready to process function calls");
    runtime.run().await?;

    Ok(())
}
