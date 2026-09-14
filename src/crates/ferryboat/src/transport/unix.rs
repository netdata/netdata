use std::io;
use std::path::{Path, PathBuf};

use tokio::net::{UnixListener, UnixStream};

pub type ConnectionStream = UnixStream;

pub struct Listener {
    inner: UnixListener,
    path: PathBuf,
}

impl Listener {
    pub fn bind(path: impl AsRef<Path>) -> io::Result<Self> {
        let path = path.as_ref().to_path_buf();
        let _ = std::fs::remove_file(&path);
        let inner = UnixListener::bind(&path)?;
        Ok(Self { inner, path })
    }

    pub async fn accept(&self) -> io::Result<ConnectionStream> {
        let (stream, _) = self.inner.accept().await?;
        Ok(stream)
    }
}

impl Drop for Listener {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.path);
    }
}

pub async fn connect(path: impl AsRef<Path>) -> io::Result<ConnectionStream> {
    UnixStream::connect(path.as_ref()).await
}
