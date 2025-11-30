//! Netdata Plugins - Multi-call binary for log-viewer-plugin and otel-plugin
//!
//! This binary can be invoked as either `log-viewer-plugin` or `otel-plugin` depending on
//! how it's called (via symlinks or hardlinks).

use multicall::{MultiCall, ToolContext};

fn main() {
    let mut mc = MultiCall::new();

    // Register plugins
    mc.register("log-viewer-plugin", run_log_viewer_plugin);
    mc.register("otel-plugin", run_otel_plugin);

    // Dispatch
    let args: Vec<String> = std::env::args().collect();
    let exit_code = mc.dispatch(&args);
    std::process::exit(exit_code);
}

fn run_log_viewer_plugin(_ctx: ToolContext, args: Vec<String>) -> i32 {
    // Build full args with tool name as argv[0]
    let mut full_args = vec!["log-viewer-plugin".to_string()];
    full_args.extend(args);

    log_viewer_plugin::run(full_args)
}

fn run_otel_plugin(_ctx: ToolContext, args: Vec<String>) -> i32 {
    // Build full args with tool name as argv[0]
    let mut full_args = vec!["otel-plugin".to_string()];
    full_args.extend(args);

    otel_plugin::run(full_args)
}
