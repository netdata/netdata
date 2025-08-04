use hyper_util::rt::TokioIo;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::io::{duplex, AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::sync::mpsc;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tonic::transport::{Endpoint, Server, Uri};
use tonic::Request;
use tower::service_fn;

pub mod netdata {
    tonic::include_proto!("netdata");
}

pub use netdata::netdata_plugin_server::NetdataPlugin;
use netdata::{
    netdata_plugin_client::NetdataPluginClient, netdata_plugin_server::NetdataPluginServer,
    PingRequest,
};

#[derive(Debug, Clone)]
pub struct WorkRequest {
    pub job_id: String,
    pub duration_ms: u64,
}

#[derive(Debug)]
pub struct WorkResponse {
    pub job_id: String,
    pub result: String,
}

#[derive(Debug)]
enum Command {
    Ping,
    Work {
        transaction_id: String,
        request: WorkRequest,
    },
    Cancel {
        transaction_id: String,
    },
}

#[derive(Debug)]
enum Response {
    Pong {
        message: String,
    },
    WorkResult {
        transaction_id: String,
        result: String,
    },
    Error {
        transaction_id: Option<String>,
        message: String,
    },
}

type TransactionManager = Arc<Mutex<HashMap<String, CancellationToken>>>;

pub async fn run_plugin<T>(plugin: T) -> Result<(), Box<dyn std::error::Error>>
where
    T: NetdataPlugin + Send + Sync + 'static,
{
    // Create in-memory duplex channel
    let (client_io, server_io) = duplex(1024);

    // Spawn the gRPC server
    tokio::spawn(async move {
        Server::builder()
            .add_service(NetdataPluginServer::new(plugin))
            .serve_with_incoming(tokio_stream::once(Ok::<_, std::io::Error>(server_io)))
            .await
            .unwrap();
    });

    // Create client
    let mut client_io = Some(client_io);
    let channel = Endpoint::try_from("http://dummy")?
        .connect_with_connector(service_fn(move |_: Uri| {
            let client = client_io.take();
            async move {
                if let Some(client) = client {
                    Ok(TokioIo::new(client))
                } else {
                    Err(std::io::Error::other("Client already taken"))
                }
            }
        }))
        .await?;

    let client = NetdataPluginClient::new(channel);

    // Transaction management
    let transactions: TransactionManager = Arc::new(Mutex::new(HashMap::new()));

    // Channels for communication
    let (command_tx, command_rx) = mpsc::unbounded_channel::<Command>();
    let (response_tx, mut response_rx) = mpsc::unbounded_channel::<Response>();

    // Spawn stdin reader task
    let stdin_task = tokio::spawn(async move {
        let stdin = tokio::io::stdin();
        let mut reader = BufReader::new(stdin);
        let mut line = String::new();

        loop {
            line.clear();
            match reader.read_line(&mut line).await {
                Ok(0) => break, // EOF
                Ok(_) => {
                    let trimmed = line.trim();
                    if trimmed.is_empty() {
                        continue;
                    }

                    let command = parse_command(trimmed);
                    if command_tx.send(command).is_err() {
                        break; // Channel closed
                    }
                }
                Err(_) => break,
            }
        }
    });

    // Spawn stdout writer task
    let stdout_task = tokio::spawn(async move {
        let mut stdout = tokio::io::stdout();

        while let Some(response) = response_rx.recv().await {
            let output = format_response(response);
            if let Err(_) = stdout.write_all(output.as_bytes()).await {
                break;
            }
            if let Err(_) = stdout.flush().await {
                break;
            }
        }
    });

    // Main command processing loop
    let command_processor = tokio::spawn(async move {
        let mut command_rx = command_rx;

        while let Some(command) = command_rx.recv().await {
            match command {
                Command::Ping => {
                    let mut client = client.clone();
                    let response_tx = response_tx.clone();

                    let request = Request::new(PingRequest {});
                    match client.ping(request).await {
                        Ok(response) => {
                            let pong = response.into_inner();
                            let _ = response_tx.send(Response::Pong {
                                message: pong.message,
                            });
                        }
                        Err(e) => {
                            let _ = response_tx.send(Response::Error {
                                transaction_id: None,
                                message: format!("Ping failed: {}", e),
                            });
                        }
                    }
                }

                Command::Work {
                    transaction_id,
                    request,
                } => {
                    // Handle work request asynchronously
                    let cancel_token = CancellationToken::new();

                    // Register transaction
                    {
                        let mut transactions = transactions.lock().await;
                        transactions.insert(transaction_id.clone(), cancel_token.clone());
                    }

                    let transactions_clone = transactions.clone();
                    let response_tx = response_tx.clone();
                    let mut client = client.clone(); // Clone the gRPC client

                    tokio::spawn(async move {
                        let result =
                            execute_work_with_cancellation(&mut client, request, cancel_token)
                                .await;

                        // Cleanup transaction
                        {
                            let mut transactions = transactions_clone.lock().await;
                            transactions.remove(&transaction_id);
                        }

                        let response = match result {
                            Ok(result) => Response::WorkResult {
                                transaction_id,
                                result,
                            },
                            Err(e) => Response::Error {
                                transaction_id: Some(transaction_id),
                                message: e,
                            },
                        };

                        let _ = response_tx.send(response);
                    });
                }

                Command::Cancel { transaction_id } => {
                    // Cancel a running transaction
                    let mut transactions = transactions.lock().await;
                    if let Some(cancel_token) = transactions.remove(&transaction_id) {
                        cancel_token.cancel();
                    }
                    // Note: No response required for cancel commands per protocol
                }
            }
        }
    });

    // Wait for all tasks
    tokio::select! {
        _ = stdin_task => {},
        _ = stdout_task => {},
        _ = command_processor => {},
    }

    Ok(())
}

fn parse_command(line: &str) -> Command {
    if line == "PING" {
        return Command::Ping;
    }

    // Example: "WORK txn_123 job_abc 5000"
    if line.starts_with("WORK ") {
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() >= 4 {
            let transaction_id = parts[1].to_string();
            let job_id = parts[2].to_string();
            if let Ok(duration_ms) = parts[3].parse::<u64>() {
                return Command::Work {
                    transaction_id,
                    request: WorkRequest {
                        job_id,
                        duration_ms,
                    },
                };
            }
        }
    }

    // Example: "CANCEL txn_123"
    if line.starts_with("CANCEL ") {
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() >= 2 {
            return Command::Cancel {
                transaction_id: parts[1].to_string(),
            };
        }
    }

    // Default to ping for unknown commands
    Command::Ping
}

fn format_response(response: Response) -> String {
    match response {
        Response::Pong { message } => format!("PONG {}\n", message),
        Response::WorkResult {
            transaction_id,
            result,
        } => {
            format!("WORK_RESULT {} {}\n", transaction_id, result)
        }
        Response::Error {
            transaction_id,
            message,
        } => match transaction_id {
            Some(txn_id) => format!("ERROR {} {}\n", txn_id, message),
            None => format!("ERROR {}\n", message),
        },
    }
}

async fn execute_work_with_cancellation(
    client: &mut NetdataPluginClient<tonic::transport::Channel>,
    request: WorkRequest,
    cancel_token: CancellationToken,
) -> Result<String, String> {
    let grpc_request = Request::new(netdata::WorkRequest {
        job_id: request.job_id.clone(),
        duration_ms: request.duration_ms,
    });

    tokio::select! {
        result = client.do_work(grpc_request) => {
            match result {
                Ok(response) => Ok(response.into_inner().result),
                Err(e) => Err(format!("Work failed: {}", e)),
            }
        }
        _ = cancel_token.cancelled() => {
            Err(format!("Job {} was cancelled", request.job_id))
        }
    }
}
