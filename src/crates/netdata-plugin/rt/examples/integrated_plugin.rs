//! Example demonstrating integrated function handlers and charts in rt
//!
//! This example shows how to use both function handlers and chart metrics
//! in a single plugin, with all output coordinated through the shared writer.

use async_trait::async_trait;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::FunctionDeclaration;
use rt::{FunctionHandler, NetdataChart, PluginRuntime};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

// Define request/response types for function handler
#[derive(Deserialize)]
struct EchoRequest {
    message: String,
}

#[derive(Serialize)]
struct EchoResponse {
    echo: String,
    timestamp: u64,
}

// Define metrics chart
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "plugin.calls"),
    extend("x-chart-title" = "Function Call Metrics"),
    extend("x-chart-units" = "calls"),
    extend("x-chart-type" = "line"),
)]
struct CallMetrics {
    total_calls: u64,
    successful_calls: u64,
    failed_calls: u64,
}

// Function handler
struct EchoHandler {
    metrics: rt::ChartHandle<CallMetrics>,
}

#[async_trait]
impl FunctionHandler for EchoHandler {
    type Request = EchoRequest;
    type Response = EchoResponse;

    async fn on_call(&self, request: Self::Request) -> Result<Self::Response> {
        // Update metrics
        self.metrics.update(|m| {
            m.total_calls += 1;
            m.successful_calls += 1;
        });

        // Simulate some work
        tokio::time::sleep(Duration::from_millis(100)).await;

        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();

        Ok(EchoResponse {
            echo: request.message,
            timestamp,
        })
    }

    async fn on_cancellation(&self) -> Result<Self::Response> {
        // Update metrics for cancelled call
        self.metrics.update(|m| {
            m.total_calls += 1;
            m.failed_calls += 1;
        });

        Err(netdata_plugin_error::NetdataPluginError::Other {
            message: "Operation cancelled".to_string(),
        })
    }

    async fn on_progress(&self) {
        eprintln!("Progress requested for echo operation");
    }

    fn declaration(&self) -> FunctionDeclaration {
        FunctionDeclaration::new("echo", "Echo back the provided message")
    }
}

#[tokio::main]
async fn main() -> std::result::Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt::init();

    eprintln!("=== Integrated Plugin Example ===");
    eprintln!("This plugin demonstrates:");
    eprintln!("- Function handlers (echo function)");
    eprintln!("- Chart metrics (call statistics)");
    eprintln!("- Coordinated output through shared writer");
    eprintln!();

    // Create runtime
    let mut runtime = PluginRuntime::new("integrated_example");

    // Register chart
    let metrics = runtime.register_chart(
        CallMetrics::default(),
        Duration::from_secs(1),
    );

    // Register function handler with metrics reference
    runtime.register_handler(EchoHandler {
        metrics: metrics.clone(),
    });

    // Simulate some background activity that updates metrics
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(Duration::from_secs(5)).await;
            // Could update metrics here if needed
        }
    });

    eprintln!("Plugin started. Charts and functions are now active.");
    eprintln!("To test: echo '{{\"message\": \"Hello!\"}}' | ./integrated_plugin");
    eprintln!();

    // Run the plugin
    runtime.run().await?;

    eprintln!("Plugin shut down cleanly");
    Ok(())
}
