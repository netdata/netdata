use crate::message_parser::{Message, MessageParser};
use futures::{SinkExt, Stream, StreamExt};
use netdata_plugin_error::{NetdataPluginError, Result};
use std::pin::Pin;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_util::codec::{FramedRead, FramedWrite};

// TransportError is now replaced by NetdataPluginError
pub type TransportError = NetdataPluginError;

/// Reader for receiving Netdata protocol messages
#[derive(Debug)]
pub struct MessageReader<R>
where
    R: AsyncRead + Unpin,
{
    reader: FramedRead<R, MessageParser>,
}

impl<R> MessageReader<R>
where
    R: AsyncRead + Unpin,
{
    /// Create a new message reader
    pub fn new(reader: R) -> Self {
        Self {
            reader: FramedRead::new(reader, MessageParser::input()),
        }
    }

    /// Receive the next message
    pub async fn recv(&mut self) -> Option<Result<Message>> {
        self.reader.next().await.map(|result| {
            result.map_err(|e| NetdataPluginError::Protocol {
                message: format!("{:?}", e),
            })
        })
    }
}

impl<R> Stream for MessageReader<R>
where
    R: AsyncRead + Unpin,
{
    type Item = Result<Message>;

    fn poll_next(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<Option<Self::Item>> {
        self.reader.poll_next_unpin(cx).map(|opt| {
            opt.map(|result| {
                result.map_err(|e| NetdataPluginError::Protocol {
                    message: format!("{:?}", e),
                })
            })
        })
    }
}

/// Writer for sending Netdata protocol messages
#[derive(Debug)]
pub struct MessageWriter<W>
where
    W: AsyncWrite + Unpin,
{
    writer: FramedWrite<W, MessageParser>,
}

impl<W> MessageWriter<W>
where
    W: AsyncWrite + Unpin,
{
    /// Create a new message writer
    pub fn new(writer: W) -> Self {
        Self {
            writer: FramedWrite::new(writer, MessageParser::output()),
        }
    }

    /// Send a message
    pub async fn send(&mut self, message: Message) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.send(message).await?;
        self.writer.flush().await?;
        self.writer.get_mut().flush().await?;
        Ok(())
    }

    /// Force flush the underlying writer
    pub async fn flush(&mut self) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.get_mut().flush().await?;
        Ok(())
    }

    /// Write raw bytes directly to the underlying writer (for chart protocol)
    ///
    /// This bypasses the message framing and writes raw bytes directly.
    /// Used for emitting chart protocol data (CHART, BEGIN, SET, END commands).
    pub async fn write_raw(&mut self, data: &[u8]) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.get_mut().write_all(data).await?;
        self.writer.get_mut().flush().await?;
        Ok(())
    }
}

/// Legacy Transport type for backward compatibility
/// Consider using MessageReader and MessageWriter directly for new code
pub struct Transport<R, W>
where
    R: AsyncRead + Unpin,
    W: AsyncWrite + Unpin,
{
    reader: MessageReader<R>,
    writer: MessageWriter<W>,
}

impl<R, W> Transport<R, W>
where
    R: AsyncRead + Unpin,
    W: AsyncWrite + Unpin,
{
    /// Create a new transport with separate reader and writer streams
    pub fn new_with_streams(reader: R, writer: W) -> Self {
        Self {
            reader: MessageReader::new(reader),
            writer: MessageWriter::new(writer),
        }
    }

    /// Send a message through the transport
    pub async fn send(&mut self, message: Message) -> Result<()> {
        self.writer.send(message).await
    }

    /// Force flush the underlying writer
    pub async fn flush(&mut self) -> Result<()> {
        self.writer.flush().await
    }

    /// Receive the next message from the transport
    pub async fn recv(&mut self) -> Option<Result<Message>> {
        self.reader.recv().await
    }

    /// Send a message and receive the next response
    pub async fn request(&mut self, message: Message) -> Result<Option<Message>> {
        self.send(message).await?;
        match self.recv().await {
            Some(Ok(response)) => Ok(Some(response)),
            Some(Err(e)) => Err(e),
            None => Ok(None),
        }
    }
}

impl Transport<tokio::io::Stdin, tokio::io::Stdout> {
    /// Create a new transport using stdin/stdout
    pub fn new() -> Self {
        Self::new_with_streams(tokio::io::stdin(), tokio::io::stdout())
    }
}

impl Default for Transport<tokio::io::Stdin, tokio::io::Stdout> {
    fn default() -> Self {
        Self::new()
    }
}
