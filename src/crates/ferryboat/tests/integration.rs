//! Integration tests for ferryboat's public API: `Connection`/`Listener`
//! round-trips, the error paths around them, and the multiplexed RPC layer.
//! Tests stay independent: in-process channel names and IPC socket paths
//! are unique per test (the IPC helper appends the process id).
//!
//! Pinned behaviors and the contracts they guard:
//!
//! - Round-trips over both transports, including several messages in
//!   sequence and several clients on one listener. In-process values move
//!   through mpsc channels with no serialization; IPC messages are
//!   bincode-serialized and sent as one length-delimited frame (the
//!   crate's framing contract).
//! - Disconnect: when the server side drops its connection, the client's
//!   next `recv` returns `Error::ConnectionClosed`; on RPC, a dropped
//!   client ends `serve` with `Ok(())`, and a dropped server session
//!   fails the client's calls.
//! - Registry: a channel name is tagged with the listener's message types,
//!   so a client built from a different pair fails with
//!   `Error::TypeMismatch`; connect retries until the listener binds, so
//!   either start order works; the name is bindable again after the
//!   listener is dropped.
//! - Size limit (IPC): a send exceeding `max_message_size` fails with
//!   `Error::MessageTooLarge` at serialize time, before any byte is
//!   written; both ends of the link must set the same limit.
//! - Compression (IPC): with `compress(true)` on both ends — the flag is
//!   part of the wire contract — small and large payloads round-trip
//!   intact.
//! - Connect retries: exhausting the budget without a peer returns the
//!   last `Error::Io` over IPC and `Error::ConnectionClosed` in-process.
//! - RPC (`mux.rs`): `RpcClient`/`RpcServer` round-trips over both
//!   transports; clones share one connection and its dispatch task, so
//!   concurrent calls multiplex and responses match by frame id regardless
//!   of completion order; the server serves several clients concurrently.
use std::time::Duration;

use ferryboat::{Connection, Endpoint, Error, Listener, RpcClient, RpcServer};
use serde::{Deserialize, Serialize};

// Nested serde payload: exercises bincode encoding beyond plain integers.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
struct Msg {
    pairs: Vec<(u64, u64)>,
}

// Socket path unique to this test run (PID suffix), so concurrent test
// runs never collide on one socket.
fn ipc_endpoint(name: &str) -> Endpoint {
    Endpoint::ipc(format!(
        "/tmp/ferryboat-test-{name}-{}.sock",
        std::process::id()
    ))
}

// --- In-process tests ---

#[tokio::test]
async fn in_process_send_recv() {
    let name = "test-basic";

    let server = tokio::spawn(async move {
        let mut listener = Listener::<Msg, Msg>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        let msg = conn.recv().await.unwrap();
        conn.send(msg).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let mut client = Connection::<Msg, Msg>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let msg = Msg {
        pairs: vec![(1, 2), (3, 4)],
    };
    client.send(msg.clone()).await.unwrap();
    let received = client.recv().await.unwrap();
    assert_eq!(received, msg);

    server.await.unwrap();
}

#[tokio::test]
async fn in_process_multiple_messages() {
    let name = "test-multi-msg";

    let server = tokio::spawn(async move {
        let mut listener = Listener::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        for _ in 0..5 {
            let n = conn.recv().await.unwrap();
            conn.send(n * n).await.unwrap();
        }
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let mut client = Connection::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    for i in 0..5u64 {
        client.send(i).await.unwrap();
        let resp = client.recv().await.unwrap();
        assert_eq!(resp, i * i);
    }

    server.await.unwrap();
}

#[tokio::test]
async fn in_process_connection_closed_on_drop() {
    let name = "test-conn-closed";

    let mut listener = Listener::<u64, u64>::bind(Endpoint::in_process(name))
        .open()
        .unwrap();

    let mut client = Connection::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let conn = listener.accept().await.unwrap();

    // The server side drops its accepted connection.
    drop(conn);

    // The client's next recv returns ConnectionClosed.
    let result = client.recv().await;
    assert!(matches!(result, Err(Error::ConnectionClosed)));
}

// The registry entry is tagged with the listener's message types: a
// client built from a different pair fails with Error::TypeMismatch on
// its first attempt, without spending the retry budget.
#[tokio::test]
async fn in_process_type_mismatch() {
    let name = "test-type-mismatch";
    let _listener = Listener::<u32, u32>::bind(Endpoint::in_process(name))
        .open()
        .unwrap();

    let result = Connection::<String, String>::connect(Endpoint::in_process(name))
        .max_retries(Some(1))
        .open()
        .await;

    assert!(matches!(result, Err(Error::TypeMismatch(_))));
}

#[tokio::test]
async fn in_process_connect_before_bind() {
    let name = "test-connect-first";

    let connect = tokio::spawn(async move {
        Connection::<u32, u32>::connect(Endpoint::in_process(name))
            .retry_interval(Duration::from_millis(10))
            .max_retries(Some(50))
            .open()
            .await
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut listener = Listener::<u32, u32>::bind(Endpoint::in_process(name))
        .open()
        .unwrap();

    let mut client = connect.await.unwrap().unwrap();
    let mut conn = listener.accept().await.unwrap();

    client.send(42).await.unwrap();
    assert_eq!(conn.recv().await.unwrap(), 42);
}

#[tokio::test]
async fn in_process_multi_client() {
    let name = "test-multi-client-inproc";

    let server = tokio::spawn(async move {
        let mut listener = Listener::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();

        for _ in 0..2 {
            let mut conn = listener.accept().await.unwrap();
            tokio::spawn(async move {
                let n = conn.recv().await.unwrap();
                conn.send(n + 100).await.unwrap();
            });
        }
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let mut c1 = Connection::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();
    let mut c2 = Connection::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    c1.send(1).await.unwrap();
    c2.send(2).await.unwrap();

    assert_eq!(c1.recv().await.unwrap(), 101);
    assert_eq!(c2.recv().await.unwrap(), 102);

    server.await.unwrap();
}

// --- IPC tests ---

#[tokio::test]
async fn ipc_send_recv() {
    let ep = ipc_endpoint("basic");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<Msg, Msg>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        let msg = conn.recv().await.unwrap();
        conn.send(msg).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<Msg, Msg>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    let msg = Msg {
        pairs: vec![(10, 20)],
    };
    client.send(msg.clone()).await.unwrap();
    let received = client.recv().await.unwrap();
    assert_eq!(received, msg);

    server.await.unwrap();
}

#[tokio::test]
async fn ipc_multiple_messages() {
    let ep = ipc_endpoint("multi-msg");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<u64, u64>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        for _ in 0..100 {
            let n = conn.recv().await.unwrap();
            conn.send(n).await.unwrap();
        }
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<u64, u64>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    for i in 0..100u64 {
        client.send(i).await.unwrap();
        assert_eq!(client.recv().await.unwrap(), i);
    }

    server.await.unwrap();
}

#[tokio::test]
async fn ipc_multi_client() {
    let ep = ipc_endpoint("multi-client");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<u64, u64>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();

        for _ in 0..3 {
            let mut conn = listener.accept().await.unwrap();
            tokio::spawn(async move {
                let n = conn.recv().await.unwrap();
                conn.send(n + 100).await.unwrap();
            });
        }
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut clients = Vec::new();
    for _ in 0..3 {
        let client = Connection::<u64, u64>::connect(Endpoint::ipc(&path))
            .retry_interval(Duration::from_millis(10))
            .max_retries(Some(10))
            .open()
            .await
            .unwrap();
        clients.push(client);
    }

    for (i, client) in clients.iter_mut().enumerate() {
        client.send(i as u64).await.unwrap();
    }

    for (i, client) in clients.iter_mut().enumerate() {
        assert_eq!(client.recv().await.unwrap(), (i as u64) + 100);
    }

    server.await.unwrap();
}

// Both ends set the same 1024-byte limit. The oversized send fails at
// serialize time, before any byte reaches the socket.
#[tokio::test]
async fn ipc_message_too_large() {
    let ep = ipc_endpoint("too-large");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let _server = tokio::spawn(async move {
        let mut listener = Listener::<Vec<u8>, Vec<u8>>::bind(Endpoint::ipc(&p))
            .max_message_size(1024)
            .open()
            .unwrap();
        let _conn = listener.accept().await;
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<Vec<u8>, Vec<u8>>::connect(Endpoint::ipc(&path))
        .max_message_size(1024)
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    let result = client.send(vec![0u8; 2048]).await;
    assert!(matches!(result, Err(Error::MessageTooLarge { .. })));
}

#[tokio::test]
async fn ipc_connect_before_bind() {
    let ep = ipc_endpoint("connect-first");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let connect = tokio::spawn(async move {
        Connection::<u32, u32>::connect(Endpoint::ipc(&p))
            .retry_interval(Duration::from_millis(10))
            .max_retries(Some(50))
            .open()
            .await
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut listener = Listener::<u32, u32>::bind(Endpoint::ipc(&path))
        .open()
        .unwrap();

    let mut client = connect.await.unwrap().unwrap();
    let mut conn = listener.accept().await.unwrap();

    client.send(99).await.unwrap();
    assert_eq!(conn.recv().await.unwrap(), 99);
}

// --- Compression tests ---

#[tokio::test]
async fn ipc_compressed_send_recv() {
    let ep = ipc_endpoint("compress");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<Msg, Msg>::bind(Endpoint::ipc(&p))
            .compress(true)
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        let msg = conn.recv().await.unwrap();
        conn.send(msg).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<Msg, Msg>::connect(Endpoint::ipc(&path))
        .compress(true)
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    let msg = Msg {
        pairs: vec![(1, 2), (3, 4), (5, 6), (7, 8)],
    };
    client.send(msg.clone()).await.unwrap();
    let received = client.recv().await.unwrap();
    assert_eq!(received, msg);

    server.await.unwrap();
}

#[tokio::test]
async fn ipc_compressed_large_payload() {
    let ep = ipc_endpoint("compress-large");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<Vec<u8>, Vec<u8>>::bind(Endpoint::ipc(&p))
            .compress(true)
            .open()
            .unwrap();
        let mut conn = listener.accept().await.unwrap();
        let data = conn.recv().await.unwrap();
        conn.send(data).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<Vec<u8>, Vec<u8>>::connect(Endpoint::ipc(&path))
        .compress(true)
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    let data = vec![42u8; 100_000];
    client.send(data.clone()).await.unwrap();
    let received = client.recv().await.unwrap();
    assert_eq!(received, data);

    server.await.unwrap();
}

// --- IPC connection closed ---

#[tokio::test]
async fn ipc_connection_closed_on_drop() {
    let ep = ipc_endpoint("conn-closed");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    let server = tokio::spawn(async move {
        let mut listener = Listener::<u64, u64>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();
        let conn = listener.accept().await.unwrap();
        drop(conn);
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let mut client = Connection::<u64, u64>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    server.await.unwrap();

    let result = client.recv().await;
    assert!(matches!(result, Err(Error::ConnectionClosed)));
}

// --- In-process rebind after listener drop ---

#[tokio::test]
async fn in_process_rebind_after_drop() {
    let name = "test-rebind";

    // Dropping the first listener must leave the channel name bindable:
    // the second bind succeeds and serves a client.
    let listener = Listener::<u64, u64>::bind(Endpoint::in_process(name))
        .open()
        .unwrap();
    drop(listener);

    let mut listener = Listener::<u64, u64>::bind(Endpoint::in_process(name))
        .open()
        .unwrap();

    let mut client = Connection::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let mut conn = listener.accept().await.unwrap();
    client.send(7).await.unwrap();
    assert_eq!(conn.recv().await.unwrap(), 7);
}

// --- Retry behavior ---

// Exhausting the retry budget over IPC returns the last connect error
// (Error::Io); the in-process twin below pins Error::ConnectionClosed.
#[tokio::test]
async fn connect_fails_after_max_retries() {
    let result =
        Connection::<u32, u32>::connect(Endpoint::ipc("/tmp/ferryboat-test-nonexistent.sock"))
            .retry_interval(Duration::from_millis(1))
            .max_retries(Some(3))
            .open()
            .await;

    assert!(matches!(result, Err(Error::Io(_))));
}

#[tokio::test]
async fn in_process_connect_fails_after_max_retries() {
    let result = Connection::<u32, u32>::connect(Endpoint::in_process("nonexistent-channel"))
        .retry_interval(Duration::from_millis(1))
        .max_retries(Some(3))
        .open()
        .await;

    assert!(matches!(result, Err(Error::ConnectionClosed)));
}

// --- Multiplexed RPC tests ---

#[tokio::test]
async fn rpc_in_process_basic() {
    let name = "rpc-basic";

    tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        session.serve(|n| async move { n * n }).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let client = RpcClient::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    assert_eq!(client.call(5).await.unwrap(), 25);
    assert_eq!(client.call(7).await.unwrap(), 49);
}

#[tokio::test]
async fn rpc_ipc_basic() {
    let ep = ipc_endpoint("rpc-basic");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    tokio::spawn(async move {
        let mut server = RpcServer::<String, String>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        session
            .serve(|msg| async move { format!("echo: {msg}") })
            .await
            .unwrap();
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let client = RpcClient::<String, String>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    assert_eq!(client.call("hello".into()).await.unwrap(), "echo: hello");
}

// Clones share one connection and its dispatch task; responses match by
// frame id even though the handler delays the even inputs.
#[tokio::test]
async fn rpc_concurrent_calls() {
    let name = "rpc-concurrent";

    tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        session
            .serve(|n| async move {
                if n % 2 == 0 {
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
                n * 10
            })
            .await
            .unwrap();
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let client = RpcClient::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let mut handles = Vec::new();
    for i in 0..10u64 {
        let c = client.clone();
        handles.push(tokio::spawn(async move { (i, c.call(i).await.unwrap()) }));
    }

    for handle in handles {
        let (i, result) = handle.await.unwrap();
        assert_eq!(result, i * 10);
    }
}

#[tokio::test]
async fn rpc_out_of_order_responses() {
    let name = "rpc-ooo";

    tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        session
            .serve(|n| async move {
                if n == 0 {
                    tokio::time::sleep(Duration::from_millis(50)).await;
                }
                n + 100
            })
            .await
            .unwrap();
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let client = RpcClient::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let c1 = client.clone();
    let slow = tokio::spawn(async move { c1.call(0).await.unwrap() });

    let c2 = client.clone();
    let fast = tokio::spawn(async move { c2.call(1).await.unwrap() });

    assert_eq!(fast.await.unwrap(), 101);
    assert_eq!(slow.await.unwrap(), 100);
}

#[tokio::test]
async fn rpc_multi_client() {
    let ep = ipc_endpoint("rpc-multi-client");
    let path = match &ep {
        Endpoint::Ipc(p) => p.clone(),
        _ => unreachable!(),
    };

    let p = path.clone();
    tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::ipc(&p))
            .open()
            .unwrap();
        loop {
            let session = server.accept().await.unwrap();
            tokio::spawn(session.serve(|n| async move { n + 1 }));
        }
    });

    tokio::time::sleep(Duration::from_millis(30)).await;

    let c1 = RpcClient::<u64, u64>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    let c2 = RpcClient::<u64, u64>::connect(Endpoint::ipc(&path))
        .retry_interval(Duration::from_millis(10))
        .max_retries(Some(10))
        .open()
        .await
        .unwrap();

    assert_eq!(c1.call(10).await.unwrap(), 11);
    assert_eq!(c2.call(20).await.unwrap(), 21);
}

// Dropping the client ends the connection; the unwrap on the server task
// pins that serve returns Ok(()) on a clean client disconnect.
#[tokio::test]
async fn rpc_client_disconnect() {
    let name = "rpc-client-dc";

    let server = tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        session.serve(|n| async move { n }).await.unwrap();
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let client = RpcClient::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    assert_eq!(client.call(1).await.unwrap(), 1);
    drop(client);

    server.await.unwrap();
}

#[tokio::test]
async fn rpc_server_disconnect() {
    let name = "rpc-server-dc";

    tokio::spawn(async move {
        let mut server = RpcServer::<u64, u64>::bind(Endpoint::in_process(name))
            .open()
            .unwrap();
        let session = server.accept().await.unwrap();
        drop(session);
    });

    tokio::time::sleep(Duration::from_millis(10)).await;

    let client = RpcClient::<u64, u64>::connect(Endpoint::in_process(name))
        .open()
        .await
        .unwrap();

    let result = client.call(1).await;
    assert!(result.is_err());
}
