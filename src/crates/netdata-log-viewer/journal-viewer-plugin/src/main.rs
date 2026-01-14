//! journal-viewer-plugin standalone binary

use journal_registry::Monitor;

mod catalog;
use catalog::CatalogFunction;

mod plugin_config;
use plugin_config::PluginConfig;

use rt::PluginRuntime;
use tracing::{error, info};

/// Initialize Sentry error tracking.
///
/// Returns a guard that must be kept alive for the duration of the program.
fn init_sentry() -> Option<sentry::ClientInitGuard> {
    if std::env::var("DISABLE_TELEMETRY").is_ok() {
        return None;
    }

    let dsn =
        "https://e6e6b1488ab279fc702439e15d6112cc@o382276.ingest.us.sentry.io/4510709226143744";

    let guard = sentry::init((
        dsn,
        sentry::ClientOptions {
            release: sentry::release_name!(),
            // Enable session tracking for counting plugin starts
            auto_session_tracking: true,
            // Set sample rate for transactions (0.0 = no transactions, saves quota)
            traces_sample_rate: 0.0,
            ..Default::default()
        },
    ));

    // Start a session to track this plugin run
    sentry::start_session();

    Some(guard)
}

#[tokio::main]
async fn main() {
    println!("TRUST_DURATIONS 1");

    // Initialize Sentry before tracing so panics are captured
    let _sentry_guard = init_sentry();

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
        "configuration loaded: journal_paths={:?}, cache_dir={}, memory_capacity={}, disk_capacity={}, workers={}",
        config.journal.paths,
        config.cache.directory,
        config.cache.memory_capacity,
        config.cache.disk_capacity,
        config.cache.workers
    );

    let mut runtime = PluginRuntime::new("journal-viewer");
    info!("plugin runtime created");

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
