#![cfg(target_os = "linux")]

use netipc::transport::shm::{ShmContext, ShmError};
use std::time::{Duration, Instant};

#[test]
fn shm_timeout_abi() {
    let dir = std::env::temp_dir().join(format!("nipc-rust-timeout-{}", std::process::id()));
    std::fs::create_dir(&dir).unwrap();
    let path = dir.to_str().unwrap();
    let mut server = ShmContext::server_create(path, "timeout", 1, 1024, 1024).unwrap();
    let mut client = ShmContext::client_attach(path, "timeout", 1).unwrap();
    let mut buf = [0u8; 64];
    println!(
        "ABI: pointer={} time_t={} timespec={}",
        std::mem::size_of::<usize>(),
        std::mem::size_of_val(&libc::timespec::default().tv_sec),
        std::mem::size_of::<libc::timespec>()
    );
    for timeout in [100, 1100] {
        let start = Instant::now();
        assert_eq!(server.receive(&mut buf, timeout), Err(ShmError::Timeout));
        let elapsed = start.elapsed();
        println!("idle {timeout} ms: {elapsed:?}");
        assert!(elapsed >= Duration::from_millis(timeout as u64));
        assert!(elapsed < Duration::from_secs(6));
    }
    for timeout in [1000, 0, u32::MAX] {
        let dir = dir.clone();
        let sender = std::thread::spawn(move || {
            let mut peer = ShmContext::client_attach(dir.to_str().unwrap(), "timeout", 1).unwrap();
            std::thread::sleep(Duration::from_millis(50));
            peer.send(b"delayed peer message").unwrap();
        });
        let len = server.receive(&mut buf, timeout).unwrap();
        assert_eq!(&buf[..len], b"delayed peer message");
        sender.join().unwrap();
    }
    client.close();
    server.destroy();
    std::fs::remove_dir(dir).unwrap();
}
