//! Framed I/O for the pluginsd protocol: [`MessageReader`] and
//! [`MessageWriter`] wrap any `AsyncRead`/`AsyncWrite` in `tokio_util::codec`'s
//! `FramedRead`/`FramedWrite` over the `Decoder`/`Encoder` impls in
//! `tokio_codec`, and [`Transport`] bundles both around one stdin/stdout pair.
//!
//! Direction is fixed at construction and never re-checked: `MessageReader`
//! decodes with `MessageParser::input()` (the agent-to-plugin direction),
//! `MessageWriter` is built with `MessageParser::output()` although its
//! Encoder is direction-agnostic. Both ends move newline-terminated lines;
//! plugin output outside the [`Message`] model (chart protocol lines,
//! `PLUGIN_KEEPALIVE`) is written verbatim with [`MessageWriter::write_raw`].
//!
//! Errors surface as [`NetdataPluginError`]: I/O failures while encoding as
//! the `Transport` variant, decode failures as the `Protocol` variant; end of
//! stream yields `None`.
use crate::message_parser::{Message, MessageParser};
use futures::{SinkExt, Stream, StreamExt};
use netdata_plugin_error::{NetdataPluginError, Result};
use std::pin::Pin;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio_util::codec::{FramedRead, FramedWrite};

/// Compatibility alias; the transport error type is [`NetdataPluginError`].
pub type TransportError = NetdataPluginError;

/// Decodes the agent-to-plugin side of the plugin channel: wraps an
/// `AsyncRead` in a `FramedRead` whose codec is `MessageParser::input()`,
/// yielding the input-direction [`Message`] subset (function calls,
/// FUNCTION_CANCEL, progress requests, QUIT).
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
    /// Wrap the stream the agent writes plugin commands to (a plugin
    /// process's stdin in the usual wiring).
    pub fn new(reader: R) -> Self {
        Self {
            reader: FramedRead::new(reader, MessageParser::input()),
        }
    }

    /// Wait for the next decoded message.
    ///
    /// Returns `None` at end of stream. Returns `Some(Err)` when the framed
    /// decoder fails, mapped to `NetdataPluginError::Protocol` whose message is
    /// only the Debug form of the `line_parser` error (`"Io"`) — in practice
    /// just underlying I/O failures, which include EOF with a trailing partial
    /// line (the codec's default `decode_eof`). After an error the stream
    /// yields `None`.
    pub async fn recv(&mut self) -> Option<Result<Message>> {
        self.reader.next().await.map(|result| {
            result.map_err(|e| NetdataPluginError::Protocol {
                message: format!("{:?}", e),
            })
        })
    }
}

// Same items and error mapping as recv(), for Stream-based callers.
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

/// Encodes the plugin-to-agent side of the plugin channel: wraps an
/// `AsyncWrite` in a `FramedWrite` whose Encoder (in `tokio_codec`) writes
/// each [`Message`]'s wire line(s). The output-direction `MessageParser` it is
/// built with only supplies that Encoder; its decoder is never used.
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
    /// Wrap the stream the plugin writes agent output to (its stdout in the
    /// usual wiring).
    pub fn new(writer: W) -> Self {
        Self {
            writer: FramedWrite::new(writer, MessageParser::output()),
        }
    }

    /// Encode `message` and deliver it to the agent: the encoded line(s) are
    /// flushed through the codec buffer and the underlying stream before
    /// returning, so the agent sees the output immediately. Inbound-only
    /// variants (`Quit`, `FunctionProgressRequest`) encode to nothing.
    /// I/O failures map to `NetdataPluginError::Transport`; dropping the future
    /// mid-send can leave a torn line for the agent's line parser.
    pub async fn send(&mut self, message: Message) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.send(message).await?;
        self.writer.flush().await?;
        self.writer.get_mut().flush().await?;
        Ok(())
    }

    /// Flush the underlying stream. This does not flush the codec's write
    /// buffer; [`Self::send`] flushes both steps itself.
    pub async fn flush(&mut self) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.get_mut().flush().await?;
        Ok(())
    }

    /// Write raw bytes verbatim to the stream, bypassing the codec, and flush
    /// it. For plugin output outside the [`Message`] model: chart protocol
    /// lines (CHART, DIMENSION, BEGIN, SET, END, ...) and bare keywords such as
    /// `PLUGIN_KEEPALIVE` and `TRUST_DURATIONS`. The data must be complete
    /// newline-terminated lines; no newline is added.
    pub async fn write_raw(&mut self, data: &[u8]) -> Result<()> {
        use tokio::io::AsyncWriteExt;
        self.writer.get_mut().write_all(data).await?;
        self.writer.get_mut().flush().await?;
        Ok(())
    }
}

/// Reader-plus-writer over one pair of streams. A legacy convenience wrapper —
/// new code composes [`MessageReader`] and [`MessageWriter`] directly, as the
/// `rt` event loop and the otel-plugin supervisor do.
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
    /// Build a transport from separate read and write streams (a plugin
    /// process's stdin and stdout in the typical wiring; the two need not be
    /// one object).
    pub fn new_with_streams(reader: R, writer: W) -> Self {
        Self {
            reader: MessageReader::new(reader),
            writer: MessageWriter::new(writer),
        }
    }

    /// Send a message; see [`MessageWriter::send`].
    pub async fn send(&mut self, message: Message) -> Result<()> {
        self.writer.send(message).await
    }

    /// Flush the underlying writer; see [`MessageWriter::flush`].
    pub async fn flush(&mut self) -> Result<()> {
        self.writer.flush().await
    }

    /// Receive the next message; see [`MessageReader::recv`].
    pub async fn recv(&mut self) -> Option<Result<Message>> {
        self.reader.recv().await
    }

    /// Send a message, then return the next inbound message: `Ok(None)` when
    /// the peer closed first, `Err` on a send or decode failure. Whatever
    /// arrives next is returned as-is; matching a response to its transaction
    /// is the caller's job.
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
    /// Build a transport over this process's stdin/stdout, the usual plugin
    /// wiring.
    pub fn new() -> Self {
        Self::new_with_streams(tokio::io::stdin(), tokio::io::stdout())
    }
}

impl Default for Transport<tokio::io::Stdin, tokio::io::Stdout> {
    fn default() -> Self {
        Self::new()
    }
}
