use std::io;
use std::path::{Path, PathBuf};

use tokio::net::windows::named_pipe::{
    ClientOptions, NamedPipeClient, NamedPipeServer, ServerOptions,
};

pub type ServerStream = NamedPipeServer;
pub type ClientStream = NamedPipeClient;

pub struct Listener {
    server: NamedPipeServer,
    path: PathBuf,
}

impl Listener {
    pub fn bind(path: impl AsRef<Path>) -> io::Result<Self> {
        let path = path.as_ref().to_path_buf();
        let server = ServerOptions::new()
            .first_pipe_instance(true)
            .create(&path)?;
        Ok(Self { server, path })
    }

    pub async fn accept(&mut self) -> io::Result<ServerStream> {
        // Wait for a client to connect to the current server instance.
        self.server.connect().await?;

        // Swap out the connected instance and create a fresh one
        // so the next client has somewhere to connect.
        let connected =
            std::mem::replace(&mut self.server, ServerOptions::new().create(&self.path)?);
        Ok(connected)
    }
}

// Named pipes are cleaned up automatically by the OS.

pub async fn connect(path: impl AsRef<Path>) -> io::Result<ClientStream> {
    // Named pipe open is synchronous.
    ClientOptions::new().open(path.as_ref())
}
