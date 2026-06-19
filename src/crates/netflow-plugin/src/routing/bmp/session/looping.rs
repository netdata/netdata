use super::super::*;
use super::decision::{BmpSessionDecision, bmp_session_decision};
use super::message::process_bmp_message;

pub(in crate::routing::bmp) async fn handle_bmp_connection(
    stream: tokio::net::TcpStream,
    remote_addr: SocketAddr,
    session_id: u64,
    config: RoutingDynamicBmpConfig,
    runtime: DynamicRoutingRuntime,
    sessions: Arc<Mutex<HashMap<SocketAddr, u64>>>,
    accepted_rds: Arc<HashSet<u64>>,
    shutdown: CancellationToken,
) {
    tracing::info!("BMP exporter connected: {}", remote_addr);
    let mut framed = Framed::new(stream, BmpCodec::default());
    let mut initialized = false;
    let mut consecutive_decode_errors = 0_usize;
    loop {
        tokio::select! {
            _ = shutdown.cancelled() => {
                break;
            }
            next = framed.next() => {
                match next {
                    Some(Ok(message)) => {
                        consecutive_decode_errors = 0;
                        let BmpMessage::V3(message) = message else {
                            continue;
                        };
                        match bmp_session_decision(&message, &mut initialized) {
                            BmpSessionDecision::Process => {}
                            BmpSessionDecision::CloseMissingInitiation => {
                                tracing::warn!(
                                    "closing BMP exporter {} session {}: first message was not initiation",
                                    remote_addr,
                                    session_id
                                );
                                break;
                            }
                            BmpSessionDecision::CloseTermination => {
                                tracing::info!(
                                    "closing BMP exporter {} session {}: termination received",
                                    remote_addr,
                                    session_id
                                );
                                break;
                            }
                        }
                        process_bmp_message(
                            remote_addr,
                            session_id,
                            message,
                            &config,
                            &accepted_rds,
                            &runtime,
                        );
                    }
                    Some(Err(err)) => {
                        consecutive_decode_errors += 1;
                        tracing::warn!(
                            "BMP decode error from {} ({} consecutive): {:?}",
                            remote_addr,
                            consecutive_decode_errors,
                            err
                        );
                        if consecutive_decode_errors >= config.max_consecutive_decode_errors {
                            tracing::warn!(
                                "closing BMP exporter {} session {} after {} consecutive decode errors",
                                remote_addr,
                                session_id,
                                consecutive_decode_errors
                            );
                            break;
                        }
                        continue;
                    }
                    None => {
                        break;
                    }
                }
            }
        }
    }

    let cleanup_runtime = runtime.clone();
    let cleanup_sessions = Arc::clone(&sessions);
    let cleanup_keep = config.keep;
    tokio::spawn(async move {
        sleep(cleanup_keep).await;
        let should_clear = {
            let mut guard = cleanup_sessions.lock().await;
            if guard.get(&remote_addr).copied() == Some(session_id) {
                guard.remove(&remote_addr);
                true
            } else {
                false
            }
        };
        if should_clear {
            cleanup_runtime.clear_session(remote_addr, session_id);
            tracing::info!(
                "cleared dynamic BMP routes for exporter {} session {} after keep interval",
                remote_addr,
                session_id
            );
        }
    });
}
