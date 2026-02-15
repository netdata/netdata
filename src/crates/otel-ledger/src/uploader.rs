//! Uploader component that copies index files to remote object storage.
//!
//! Each upload runs in a spawned async task. Multiple uploads can be
//! in flight concurrently.

use opendal::Operator;
use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{UploaderRequest, UploaderResponse};

pub struct Uploader;

impl Component for Uploader {
    type Request = UploaderRequest;
    type Response = UploaderResponse;
    type Args = Operator;

    async fn run(
        operator: Operator,
        mut rx: mpsc::UnboundedReceiver<UploaderRequest>,
        tx: mpsc::UnboundedSender<UploaderResponse>,
        cancel: CancellationToken,
    ) {
        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(UploaderRequest::Upload { seq, local_path, remote_key }) => {
                        let op = operator.clone();
                        let tx = tx.clone();
                        tokio::spawn(async move {
                            let start = Instant::now();
                            tracing::info!("upload started seq={seq} remote_key={remote_key}");

                            let resp = match tokio::fs::read(&local_path).await {
                                Ok(data) => match op.write(&remote_key, data).await {
                                    Ok(_) => {
                                        tracing::info!(
                                            "upload complete seq={seq} remote_key={remote_key} elapsed_ms={}",
                                            start.elapsed().as_millis(),
                                        );
                                        UploaderResponse::Uploaded { seq, remote_key }
                                    }
                                    Err(e) => {
                                        tracing::error!("upload failed seq={seq}: {e}");
                                        UploaderResponse::UploadFailed {
                                            seq,
                                            error: e.to_string(),
                                        }
                                    }
                                },
                                Err(e) => {
                                    tracing::error!(
                                        "failed to read local file {}: {e}",
                                        local_path.display()
                                    );
                                    UploaderResponse::UploadFailed {
                                        seq,
                                        error: e.to_string(),
                                    }
                                }
                            };

                            let _ = tx.send(resp);
                        });
                    }
                    None => break,
                },
            }
        }
    }
}
