//! otel-signal-viewer standalone binary

use journal_function::IndexingLimits;
use journal_registry::Monitor;

mod catalog;
use catalog::CatalogFunction;

mod plugin_config;
use plugin_config::PluginConfig;

use rt::PluginRuntime;
use tracing::{error, info};

#[tokio::main]
async fn main() {
    // Install SIGBUS handler first, before any memory-mapped file operations,
    // because systemd can rotate journal files at any time.
    if let Err(e) = journal_core::install_sigbus_handler() {
        eprintln!("failed to install SIGBUS handler: {}", e);
        std::process::exit(1);
    }

    println!("TRUST_DURATIONS 1");

    rt::init_tracing();

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
        "configuration loaded: journal_paths={:?}, cache_dir={}, memory_capacity={}, disk_capacity={}, workers={}, max_unique_values_per_field={}, max_field_payload_size={}",
        config.journal.paths,
        config.cache.directory,
        config.cache.memory_capacity,
        config.cache.disk_capacity,
        config.cache.workers,
        config.indexing.max_unique_values_per_field,
        config.indexing.max_field_payload_size
    );

    let mut runtime = PluginRuntime::new("otel-signal-viewer");
    info!("plugin runtime created");

    let (monitor, notify_rx) = match Monitor::new() {
        Ok(t) => t,
        Err(e) => {
            error!("failed to setup notify monitoring: {}", e);
            return Ok(());
        }
    };

    // Create catalog function with disk-backed cache
    let indexing_limits = IndexingLimits {
        max_unique_values_per_field: config.indexing.max_unique_values_per_field,
        max_field_payload_size: config.indexing.max_field_payload_size,
    };
    let catalog_function = CatalogFunction::new(
        monitor,
        &config.cache.directory,
        config.cache.memory_capacity,
        config.cache.disk_capacity.as_u64() as usize,
        indexing_limits,
    )
    .await?;

    // Watch configured journal directories
    for path in &config.journal.paths {
        if let Err(e) = catalog_function.watch_directory(path) {
            error!("failed to watch directory {}: {:#?}", path, e);
        }
    }

    runtime.register_handler(catalog_function.clone());

    // Spawn task to process notify events
    let catalog_function_clone = catalog_function.clone();
    tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        while let Some(event) = notify_rx.recv().await {
            catalog_function_clone.process_notify_event(event);
        }
        info!("notify event processing task terminated");
    });

    runtime.run().await?;

    Ok(())
}
