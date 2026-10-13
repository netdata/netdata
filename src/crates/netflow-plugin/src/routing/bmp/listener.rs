use super::{
    rd::parse_configured_rds,
    session::{configure_receive_buffer, handle_bmp_connection},
    *,
};

pub(crate) async fn run_bmp_listener(
    config: RoutingDynamicBmpConfig,
    runtime: DynamicRoutingRuntime,
    shutdown: CancellationToken,
) -> Result<()> {
    if !config.enabled {
        return Ok(());
    }

    let listen_addr = config
        .listen
        .parse::<SocketAddr>()
        .with_context(|| format!("invalid BMP listen address {}", config.listen))?;
    let listener = TcpListener::bind(listen_addr)
        .await
        .with_context(|| format!("failed to bind BMP listener on {}", listen_addr))?;
    run_bmp_listener_with_bound_listener(listener, config, runtime, shutdown).await
}

pub(crate) async fn run_bmp_listener_with_bound_listener(
    listener: TcpListener,
    config: RoutingDynamicBmpConfig,
    runtime: DynamicRoutingRuntime,
    shutdown: CancellationToken,
) -> Result<()> {
    let listen_addr = listener
        .local_addr()
        .with_context(|| "failed to read BMP listener address")?;
    let accepted_rds = Arc::new(
        parse_configured_rds(&config.rds)
            .with_context(|| "invalid enrichment.routing_dynamic.bmp.rds entries")?,
    );
    tracing::info!("dynamic BMP routing listener started on {}", listen_addr);

    let sessions = Arc::new(Mutex::new(HashMap::<SocketAddr, u64>::new()));
    let next_session = AtomicU64::new(1);

    loop {
        tokio::select! {
            _ = shutdown.cancelled() => {
                tracing::info!("dynamic BMP routing listener shutdown requested");
                return Ok(());
            }
            accepted = listener.accept() => {
                let (stream, remote_addr) = match accepted {
                    Ok(v) => v,
                    Err(err) => {
                        tracing::warn!("failed to accept BMP connection: {}", err);
                        continue;
                    }
                };
                configure_receive_buffer(&stream, remote_addr, config.receive_buffer);

                let session_id = next_session.fetch_add(1, Ordering::Relaxed);
                {
                    let mut guard = sessions.lock().await;
                    guard.insert(remote_addr, session_id);
                }

                let connection_runtime = runtime.clone();
                let connection_shutdown = shutdown.clone();
                let connection_config = config.clone();
                let connection_sessions = Arc::clone(&sessions);
                let connection_accepted_rds = Arc::clone(&accepted_rds);
                tokio::spawn(async move {
                    handle_bmp_connection(
                        stream,
                        remote_addr,
                        session_id,
                        connection_config,
                        connection_runtime,
                        connection_sessions,
                        connection_accepted_rds,
                        connection_shutdown,
                    )
                    .await;
                });
            }
        }
    }
}
