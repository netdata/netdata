use std::future::Future;
use std::ops::ControlFlow;
use std::time::Duration;

use futures::{SinkExt, StreamExt};
use tokio::time::Instant;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::Message as WsMessage;
use tracing::{error, info, warn};

/// Longest silence tolerated on a connection. The sources are firehoses and
/// certstream's pings draw pongs, so a minute without any frame means the
/// connection is dead.
const IDLE_TIMEOUT: Duration = Duration::from_secs(60);

/// Resolves on the next ping tick, or never when pings are disabled.
async fn next_tick(timer: &mut Option<tokio::time::Interval>) {
    match timer {
        Some(timer) => {
            timer.tick().await;
        }
        None => std::future::pending().await,
    }
}

/// Run a WebSocket event loop.
///
/// Connects to `url`, splits the stream, and for each text message parses the
/// JSON into a `serde_json::Value` and passes it to `handler`. The handler
/// returns `ControlFlow::Continue(())` to keep going or
/// `ControlFlow::Break(())` to stop the loop (typically because the downstream
/// channel is closed).
///
/// If `subscribe` is `Some`, the text frame is sent once immediately after the
/// connection opens — required by sources that only start streaming after an
/// application-level subscribe message (e.g. RIS Live). Sources that stream on
/// connect (jetstream, certstream) pass `None`.
///
/// If `ping` is `Some`, sends WebSocket pings at that interval to keep the
/// connection alive.
///
/// Returns `Ok` when no frame arrives for [`IDLE_TIMEOUT`], so the caller's
/// reconnect loop replaces a connection whose peer went silent without
/// closing it.
pub async fn run<H, F>(
    name: &str,
    url: &str,
    subscribe: Option<String>,
    ping: Option<Duration>,
    handler: H,
) -> anyhow::Result<()>
where
    H: FnMut(serde_json::Value) -> F + Send + 'static,
    F: Future<Output = ControlFlow<(), ()>> + Send,
{
    run_with_idle_timeout(name, url, subscribe, ping, IDLE_TIMEOUT, handler).await
}

async fn run_with_idle_timeout<H, F>(
    name: &str,
    url: &str,
    subscribe: Option<String>,
    ping: Option<Duration>,
    idle_timeout: Duration,
    mut handler: H,
) -> anyhow::Result<()>
where
    H: FnMut(serde_json::Value) -> F + Send + 'static,
    F: Future<Output = ControlFlow<(), ()>> + Send,
{
    info!("Connecting to {name}: {url}");

    let (ws_stream, _response) = connect_async(url).await?;
    info!("Connected to {name}");

    let (mut write, mut read) = ws_stream.split();

    if let Some(subscribe) = subscribe {
        info!("{name} sending subscribe frame");
        write.send(WsMessage::Text(subscribe.into())).await?;
    }

    let mut ping_timer: Option<tokio::time::Interval> = match ping {
        Some(d) => {
            let mut t = tokio::time::interval(d);
            t.tick().await; // consume the immediate first tick
            Some(t)
        }
        None => None,
    };

    let idle = tokio::time::sleep(idle_timeout);
    tokio::pin!(idle);

    loop {
        // Biased toward the read: frames buffered while the handler applied
        // backpressure must win over an idle deadline that expired meanwhile.
        let msg = tokio::select! {
            biased;
            msg = read.next() => msg,
            _ = &mut idle => {
                warn!(
                    "{name} received nothing for {}s, reconnecting",
                    idle_timeout.as_secs()
                );
                break Ok(());
            }
            _ = next_tick(&mut ping_timer) => {
                if let Err(e) = write.send(WsMessage::Ping(vec![].into())).await {
                    error!("Failed to send WebSocket ping: {e}");
                    break Ok(());
                }
                continue;
            }
        };
        idle.as_mut().reset(Instant::now() + idle_timeout);

        let Some(msg) = msg else {
            info!("{name} WebSocket stream ended");
            break Ok(());
        };

        match msg {
            Ok(WsMessage::Text(text)) => {
                let raw_json: serde_json::Value = match serde_json::from_str(&text) {
                    Ok(v) => v,
                    Err(e) => {
                        warn!("Failed to parse message as JSON: {e}");
                        continue;
                    }
                };

                match handler(raw_json).await {
                    ControlFlow::Continue(()) => {}
                    ControlFlow::Break(()) => break Ok(()),
                }
            }
            Ok(WsMessage::Close(_)) => {
                info!("{name} WebSocket closed by server");
                break Ok(());
            }
            Err(e) => {
                error!("{name} WebSocket error: {e}");
                break Ok(());
            }
            _ => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tokio::net::TcpListener;

    // Real time: a paused clock auto-advances whenever the runtime waits on
    // loopback I/O, which fires the deadlines early.
    #[tokio::test]
    async fn idle_peer_ends_the_loop_and_frames_reset_the_deadline() {
        const IDLE: Duration = Duration::from_millis(500);
        const GAP: Duration = Duration::from_millis(150);
        const FRAMES: usize = 4;

        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();

        // Frames spaced well inside IDLE but spanning more than IDLE in total;
        // then the peer goes silent but keeps the socket open, like the
        // stalled RIS feed.
        tokio::spawn(async move {
            let (tcp, _) = listener.accept().await.unwrap();
            let mut ws = tokio_tungstenite::accept_async(tcp).await.unwrap();
            for i in 0..FRAMES {
                tokio::time::sleep(GAP).await;
                ws.send(WsMessage::Text(format!("{{\"n\":{i}}}").into()))
                    .await
                    .unwrap();
            }
            std::future::pending::<()>().await;
        });

        let received = Arc::new(AtomicUsize::new(0));
        let counter = received.clone();
        let url = format!("ws://{addr}");
        let run = run_with_idle_timeout("test", &url, None, None, IDLE, move |_| {
            let counter = counter.clone();
            async move {
                counter.fetch_add(1, Ordering::SeqCst);
                ControlFlow::Continue(())
            }
        });
        // Bounded so a missing idle deadline fails instead of hanging.
        tokio::time::timeout(Duration::from_secs(10), run)
            .await
            .expect("run kept waiting on a silent peer")
            .unwrap();

        // The last frame came after IDLE, so every frame pushed the deadline.
        assert_eq!(received.load(Ordering::SeqCst), FRAMES);
    }
}
