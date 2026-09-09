use super::super::*;

pub(in crate::routing::bmp) fn configure_receive_buffer(
    stream: &tokio::net::TcpStream,
    remote_addr: SocketAddr,
    requested: usize,
) {
    if requested == 0 {
        return;
    }

    let socket = SockRef::from(stream);
    if let Err(err) = socket.set_recv_buffer_size(requested) {
        tracing::warn!(
            "failed to set BMP receive buffer for exporter {} (requested {} bytes): {}",
            remote_addr,
            requested,
            err
        );
        return;
    }

    match socket.recv_buffer_size() {
        Ok(actual) if actual < requested => {
            tracing::warn!(
                "BMP receive buffer for exporter {} is below requested size: requested={} actual={}",
                remote_addr,
                requested,
                actual
            );
        }
        Ok(actual) => {
            tracing::info!(
                "BMP receive buffer configured for exporter {}: requested={} actual={}",
                remote_addr,
                requested,
                actual
            );
        }
        Err(err) => {
            tracing::warn!(
                "failed to read BMP receive buffer size for exporter {}: {}",
                remote_addr,
                err
            );
        }
    }
}
