use std::future::Future;
use std::ops::ControlFlow;
use std::time::Duration;

use futures::{SinkExt, StreamExt};
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::Message as WsMessage;
use tracing::{error, info, warn};

/// Run a WebSocket event loop.
///
/// Connects to `url`, splits the stream, and for each text message parses the
/// JSON into a `serde_json::Value` and passes it to `handler`. The handler
/// returns `ControlFlow::Continue(())` to keep going or
/// `ControlFlow::Break(())` to stop the loop (typically because the downstream
/// channel is closed).
///
/// If `ping` is `Some`, sends WebSocket pings at that interval to keep the
/// connection alive. When `ping` is `None`, the write half of the connection
/// is dropped immediately.
pub async fn run<H, F>(
    name: &str,
    url: &str,
    ping: Option<Duration>,
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

    let mut ping_timer: Option<tokio::time::Interval> = match ping {
        Some(d) => {
            let mut t = tokio::time::interval(d);
            t.tick().await; // consume the immediate first tick
            Some(t)
        }
        None => None,
    };

    loop {
        let msg = match &mut ping_timer {
            Some(timer) => {
                tokio::select! {
                    msg = read.next() => msg,
                    _ = timer.tick() => {
                        if let Err(e) = write.send(WsMessage::Ping(vec![].into())).await {
                            error!("Failed to send WebSocket ping: {e}");
                            break Ok(());
                        }
                        continue;
                    }
                }
            }
            None => read.next().await,
        };

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
