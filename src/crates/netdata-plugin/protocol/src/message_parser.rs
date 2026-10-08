//! Message-level assembly for the pluginsd function protocol.
//!
//! Middle layer of this crate's decode path: [`LineParser`] frames lines and
//! payload blocks into [`Command`]s, [`MessageParser`] folds those commands
//! into whole [`Message`] values, and the `Decoder`/`Encoder` impls in
//! `tokio_codec` hook that state machine into `FramedRead`/`FramedWrite` —
//! exposed to plugins as `MessageReader`/`MessageWriter` in `transport`. Only
//! `Message` is re-exported from `lib.rs`; the parser itself stays
//! crate-internal.
//!
//! A parser decodes one direction at a time. `MessageParser::input()` reads
//! the direction the agent writes to plugins (the direction `MessageReader`
//! uses): FUNCTION calls, FUNCTION_CANCEL, FUNCTION_PROGRESS requests and
//! QUIT. `MessageParser::output()` reads the direction plugins write to the
//! agent (function declarations, FUNCTION_RESULT blocks, progress reports);
//! `MessageWriter` is built with it, though the Encoder impl ignores
//! direction. `Default` is the output direction.
//!
//! Field orders follow the agent-side definitions: the send/parse handlers in
//! `src/plugins.d/pluginsd_functions.c` and the `PLUGINSD_KEYWORD_*` /
//! `PLUGINSD_CALL_*` wire vocabulary in
//! `src/libnetdata/functions_evloop/functions_evloop.h`.
//!
//! Robustness contract: only the function protocol is understood here.
//! BEGIN/SET/END are dropped silently; any other unhandled command is logged
//! to stderr and dropped. A message whose required words are missing or
//! non-numeric yields None — malformed input is discarded, never surfaced as
//! an error. The line layer has already consumed the line at that point, so
//! the stream never loses sync.
//!
//! Payload blocks are framed by the line layer
//! (`LineParser::parse_payload_block`): one data command carries a block's
//! entire payload and the END marker is consumed by the line layer itself.
//! Payload-mode calls and results therefore complete at different points; see
//! `process_command`.

use crate::http_content::HttpContent;
use crate::line_parser::{Command, LineParser};
use crate::word_iterator::WordIterator;
use netdata_plugin_types::*;

/// Decode direction of a [`MessageParser`]: which side of the plugin channel
/// its decoder reads.
#[derive(Debug, Clone, Copy, PartialEq)]
enum ParserDirection {
    /// Lines sent by the agent to plugins: FUNCTION calls, FUNCTION_CANCEL,
    /// FUNCTION_PROGRESS requests and QUIT. The direction `MessageReader`
    /// decodes with.
    Input,
    /// Lines sent by plugins to the agent: function declarations,
    /// FUNCTION_RESULT blocks and progress reports. `MessageWriter` is
    /// constructed with this direction, though its Encoder impl ignores it.
    Output,
}

/// State machine that folds a [`Command`](crate::line_parser::Command) stream
/// into [`Message`] values. It holds at most one in-flight message in
/// `current_message`; see `process_command` for when that message is released.
#[derive(Debug)]
pub struct MessageParser {
    pub(crate) line_parser: LineParser,
    current_message: Option<Message>,
    direction: ParserDirection,
}

impl Default for MessageParser {
    /// Defaults to the output direction.
    fn default() -> Self {
        Self::new(ParserDirection::Output)
    }
}

/// One message of the pluginsd function protocol: what the codec decodes from
/// or serializes onto the plugin channel (the `Decoder`/`Encoder` impls in
/// `tokio_codec`), and what producer tasks pass in-process to writer tasks in
/// `rt`, `otel-plugin` and the bridge crates.
///
/// The decoder produces the agent-role subset (`FunctionCall`,
/// `FunctionCancel`, `FunctionProgressRequest`, `Quit`) in the input
/// direction and the plugin-role subset (`FunctionDeclaration`,
/// `FunctionResult`, `FunctionProgressResponse`) in the output direction;
/// `ConfigDeclaration` is never decoded. The Encoder serializes every variant
/// except `FunctionProgressRequest` and `Quit`, which have no wire form in
/// this crate.
#[derive(Debug)]
pub enum Message {
    FunctionDeclaration(Box<FunctionDeclaration>),
    /// Inline FUNCTION call. `payload` is `Some` only for FUNCTION_PAYLOAD
    /// block calls, and the Encoder picks the wire form from that field.
    FunctionCall(Box<FunctionCall>),
    FunctionResult(Box<FunctionResult>),
    FunctionCancel(Box<FunctionCancel>),
    FunctionProgressRequest(Box<FunctionProgressRequest>),
    FunctionProgressResponse(Box<FunctionProgressResponse>),
    ConfigDeclaration(Box<ConfigDeclaration>),
    Quit,
}

/// Split the wire word that carries a function name and its arguments
/// (`"name arg1 arg2 ..."`, already unquoted by
/// [`WordIterator`](crate::word_iterator::WordIterator)) into the name and
/// the argument strings (lossy UTF-8). Returns None when there are no words.
/// Used by the FUNCTION and FUNCTION_PAYLOAD parsers below.
pub fn name_and_args(buffer: &[u8]) -> Option<(String, Vec<String>)> {
    let mut words = WordIterator::new(buffer);
    let name = words.next_string()?;

    let mut args = Vec::new();
    for word in words {
        args.push(String::from_utf8_lossy(word).into_owned());
    }

    Some((name, args))
}

impl MessageParser {
    /// Create a message parser for the given direction.
    fn new(direction: ParserDirection) -> Self {
        Self {
            line_parser: LineParser::default(),
            current_message: None,
            direction,
        }
    }

    /// Create a decoder for the agent-to-plugin direction (what the agent
    /// sends plugins; the direction `MessageReader` uses).
    pub fn input() -> Self {
        Self::new(ParserDirection::Input)
    }

    /// Create a decoder for the plugin-to-agent direction; the direction
    /// `MessageWriter` is constructed with.
    pub fn output() -> Self {
        Self::new(ParserDirection::Output)
    }

    /// Fold one command from the line layer into the parser state, returning
    /// a message when one completes.
    ///
    /// Single-line commands (FUNCTION, FUNCTION_CANCEL, FUNCTION_PROGRESS,
    /// QUIT) complete immediately. Payload blocks differ because of the line
    /// layer's framing: the whole payload arrives as one data command with the
    /// END marker already consumed by the line layer, so a payload-mode
    /// `FunctionCall` is returned together with its data. A `FunctionResult` built from a
    /// well-formed FUNCTION_RESULT_BEGIN..FUNCTION_RESULT_END block, in
    /// contrast, is never completed by the block itself — no
    /// `FunctionResultEnd` command exists on that path — so the assembled
    /// result stays in `current_message` until a later begin-type command
    /// returns it as its previous message, or a stray FUNCTION_RESULT_END
    /// line is parsed as an ordinary line. A FUNCTION command replaces a held
    /// message without returning it. `MessageReader` decodes with the input
    /// direction only, so this output-direction result path is currently
    /// unexercised in-tree.
    pub(crate) fn process_command(&mut self, command: Command) -> Option<Message> {
        match command {
            Command::Function { args } => {
                self.current_message = self.parse_function(args);
                self.current_message.take()
            }

            Command::FunctionResultBegin { args } => {
                let prev_message = self.current_message.take();
                self.current_message = self.parse_function_result_begin(args);
                prev_message
            }

            Command::FunctionPayloadBegin { args } => {
                let prev_message = self.current_message.take();
                self.current_message = self.parse_function_payload_begin(args);
                prev_message
            }

            Command::FunctionPayloadData { data } => {
                // The line layer delivers a whole block's payload as this one
                // command (its END marker is consumed there), so the held call
                // is complete and is returned now.
                if let Some(Message::FunctionCall(func_call)) = &mut self.current_message {
                    if let Some(ref mut payload) = func_call.payload {
                        payload.extend_from_slice(data);
                    } else {
                        func_call.payload = Some(data.to_vec());
                    }
                }
                self.current_message.take()
            }

            // Stray FUNCTION_PAYLOAD_END marker; nothing to do.
            Command::FunctionPayload { args: _ } => None,

            Command::FunctionResultPayload { data } => {
                // Append the block data to the held result; it is not released
                // here — see the FunctionResultEnd arm.
                if let Some(Message::FunctionResult(func_result)) = &mut self.current_message {
                    func_result.payload.extend_from_slice(data);
                }
                None
            }

            // Releases the held message; see process_command for why this only
            // fires outside a well-formed block.
            Command::FunctionResultEnd { args: _ } => self.current_message.take(),

            Command::FunctionCancel { args } => {
                self.current_message = self.parse_function_cancel(args);
                self.current_message.take()
            }

            Command::FunctionProgress { args } => {
                self.current_message = self.parse_function_progress(args);
                self.current_message.take()
            }

            Command::Quit => Some(Message::Quit),

            // Chart-update commands are not function-protocol traffic: plugins
            // write chart lines as raw bytes (see MessageWriter::write_raw), so
            // these are simply dropped.
            Command::Begin { args: _ } | Command::Set { args: _ } | Command::End { args: _ } => {
                None
            }

            // Unhandled command (e.g. a chart-definition line): report and
            // drop. The line layer already consumed the line, so the stream
            // stays in sync.
            cmd => {
                eprintln!("Got cmd: {:#?}", cmd);
                None
            }
        }
    }

    /// Parse a FUNCTION line. The keyword is direction-ambiguous — the agent
    /// sends it to call a function, a plugin sends it to declare one — so the
    /// argument layout is chosen by the parser direction.
    fn parse_function(&self, args: &[u8]) -> Option<Message> {
        match self.direction {
            ParserDirection::Input => self.parse_function_call(args),
            ParserDirection::Output => self.parse_function_declaration(args),
        }
    }

    /// Parse a function declaration sent by a plugin:
    /// `FUNCTION [GLOBAL] name timeout help [tags [access [priority [version]]]]`.
    /// Words are strictly positional, so the optional trailing fields can only
    /// be omitted from the end (the Encoder in `tokio_codec` writes them the
    /// same way). name, timeout and help are required; a missing or
    /// non-numeric timeout yields None. Agent-side counterpart:
    /// `pluginsd_function()` in src/plugins.d/pluginsd_functions.c.
    fn parse_function_declaration(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let (global, name) = {
            let first_word = words.next_string()?;

            if first_word == "GLOBAL" {
                (true, words.next_string()?)
            } else {
                (false, first_word)
            }
        };

        let timeout = words.next_u32()?;
        let help = words.next_string()?;
        let tags = words.next_string();
        let access = words.next().map(HttpAccess::from_slice);
        let priority = words.next_u32();
        let version = words.next_u32();

        let function_declaration = Box::new(FunctionDeclaration {
            global,
            name,
            timeout,
            help,
            tags,
            access,
            priority,
            version,
        });

        Some(Message::FunctionDeclaration(function_declaration))
    }

    /// Parse a function call sent by the agent:
    /// `FUNCTION transaction timeout "name [args ...]" [access] [source]`.
    /// The name word carries the function name optionally followed by
    /// space-separated arguments, split by [`name_and_args`]; access is a hex
    /// or legacy-role word (`HttpAccess::from_slice`). transaction, timeout
    /// and the name word are required. The result has `payload = None`;
    /// payload-mode calls arrive as FUNCTION_PAYLOAD blocks instead.
    /// Agent-side sender: `pluginsd_calls_insert_cb()` in
    /// src/plugins.d/pluginsd_functions.c.
    fn parse_function_call(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let timeout = words.next_u32()?;
        let (name, args) = {
            let buffer = words.next_str()?.as_bytes();
            name_and_args(buffer)?
        };
        let access = words.next().map(HttpAccess::from_slice);
        let source = words.next_string();
        let payload = None;

        let function_call = Box::new(FunctionCall {
            transaction,
            timeout,
            name,
            args,
            access,
            source,
            payload,
        });

        Some(Message::FunctionCall(function_call))
    }

    /// Parse the header of a function result sent by a plugin:
    /// `FUNCTION_RESULT_BEGIN transaction status format expires`, where
    /// expires is a unix timestamp in seconds and format is a content type
    /// (`http_content::HttpContent`; unrecognized values fall back to
    /// text/plain). All four words are required. The result is held with an
    /// empty payload and the block's data is appended to it; see
    /// `process_command` for when the message is released. Agent-side
    /// counterpart: `pluginsd_function_result_begin()`.
    fn parse_function_result_begin(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let status = words.next_u32()?;
        let format = HttpContent::from_str_or_default(words.next_str()?).to_string();
        let expires = words.next_u64()?;

        let function_result = Box::new(FunctionResult {
            transaction,
            status,
            format,
            expires,
            payload: Vec::new(),
        });

        Some(Message::FunctionResult(function_result))
    }

    /// Parse the header of a payload-mode function call sent by the agent
    /// (wire keyword FUNCTION_PAYLOAD):
    /// `FUNCTION_PAYLOAD transaction timeout "name [args ...]" [access]
    /// [source] [content_type]`. Same words as [`Self::parse_function_call`],
    /// but `payload` starts as an empty `Some` so the block's data command
    /// appends into it, and the agent appends the payload's content type as a
    /// trailing quoted word that this parser does not read. transaction,
    /// timeout and the name word are required. Agent-side sender:
    /// `pluginsd_calls_insert_cb()`.
    fn parse_function_payload_begin(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let timeout = words.next_u32()?;
        let (name, args) = {
            let buffer = words.next_str()?.as_bytes();
            name_and_args(buffer)?
        };
        let access = words.next().map(HttpAccess::from_slice);
        let source = words.next_string();

        let function_call = Box::new(FunctionCall {
            transaction,
            timeout,
            name,
            args,
            access,
            source,
            payload: Some(Vec::new()),
        });

        Some(Message::FunctionCall(function_call))
    }

    /// Parse FUNCTION_CANCEL from the agent: a single transaction id.
    fn parse_function_cancel(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let function_cancel = Box::new(FunctionCancel { transaction });

        Some(Message::FunctionCancel(function_cancel))
    }

    /// Parse FUNCTION_PROGRESS, direction-ambiguous like FUNCTION: a progress
    /// request from the agent (input) or a progress report from a plugin
    /// (output).
    fn parse_function_progress(&self, args: &[u8]) -> Option<Message> {
        match self.direction {
            ParserDirection::Input => self.parse_function_progress_request(args),
            ParserDirection::Output => self.parse_function_progress_response(args),
        }
    }

    /// Parse a progress request sent by the agent:
    /// `FUNCTION_PROGRESS transaction`. A nudge asking the plugin to report
    /// progress for that transaction.
    fn parse_function_progress_request(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;

        Some(Message::FunctionProgressRequest(Box::new(
            FunctionProgressRequest { transaction },
        )))
    }

    /// Parse a progress report sent by a plugin:
    /// `FUNCTION_PROGRESS transaction done all` — work units completed out of
    /// the total. All three words are required.
    fn parse_function_progress_response(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let done = words.next_usize()?;
        let all = words.next_usize()?;

        Some(Message::FunctionProgressResponse(Box::new(
            FunctionProgressResponse {
                transaction,
                done,
                all,
            },
        )))
    }
}
