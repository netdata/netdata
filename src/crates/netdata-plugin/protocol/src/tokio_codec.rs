//! `tokio_util::codec` adapter between this crate's parser state machine and
//! framed stdin/stdout I/O: implements [`Decoder`]/[`Encoder`] for
//! [`MessageParser`] so `tokio_util::codec::FramedRead`/`FramedWrite` can drive
//! them. The only user is `transport`: `MessageReader` wraps a `FramedRead`
//! over `MessageParser::input()`, `MessageWriter` a `FramedWrite` over
//! `MessageParser::output()`. The `Encoder` impl ignores the parser's
//! direction and state; it switches only on the [`Message`] variant.
//!
//! Decoder semantics: `decode` consumes complete lines as it goes and yields
//! at most one `Message` per call; the details (partial reads, dropped lines,
//! error handling) are on the `impl Decoder` below. All protocol state lives
//! in the `MessageParser` (payload mode in its `LineParser` plus the in-flight
//! message), so one instance must see the stream in order.
//!
//! Encoder semantics: every `Message` with a wire form is appended to the
//! output buffer as its newline-terminated keyword line(s). Payload bodies go
//! in raw between the begin header and the `..._END` marker, with a `\n`
//! appended when the body does not end in one, so the marker stays alone on
//! its line — the agent ends a deferred block on the first line whose first
//! word is the end keyword (`parser_action` in
//! `src/plugins.d/pluginsd_parser.h`) and stops the plugin when the block
//! exceeds `PLUGINSD_MAX_DEFERRED_SIZE` (100 MiB, `src/libnetdata/libnetdata.h`).

use crate::message_parser::{Message, MessageParser};
use bytes::{Buf, BytesMut};
use netdata_plugin_types::FunctionCall;
use tokio_util::codec::{Decoder, Encoder};

/// Quote a value for a pluginsd field when it cannot travel bare: the empty
/// string becomes `''`, and anything containing a space, tab, `'` or `"` is
/// wrapped in single quotes with embedded `'` escaped as `\'`.
///
/// The agent splits fields with `quoted_strings_splitter`
/// (`src/libnetdata/line_splitter/line_splitter.h`): only the quote character
/// the field opened with closes it, and a backslash before any byte is skipped
/// as a pair without being removed — so `\'` protects the quote but the agent
/// still sees the literal backslash in the value. Note the pluginsd map also
/// splits unquoted words on `\r`, `\f`, `\v` and `=`, which this function does
/// not guard against.
fn quote_if_needed(s: &str) -> String {
    if s.is_empty() {
        "''".to_string()
    } else if s.contains([' ', '\t', '\'', '"']) {
        // the agent never unescapes, so the backslash stays in the value
        format!("'{}'", s.replace('\'', "\\'"))
    } else {
        s.to_string()
    }
}

/// Trailing fields of the FUNCTION / FUNCTION_PAYLOAD call header, after the
/// keyword: quoted transaction, timeout in seconds, quoted function name
/// (`FunctionCall::args`, the words after the name, are not re-encoded), then
/// optional access (hex, as `HttpAccess` prints) and source. Access and source
/// are appended independently, so a source without access shifts fields for a
/// positional reader. Mirrors the call format the agent sends to plugins in
/// `pluginsd_calls_insert_cb` (`src/plugins.d/pluginsd_functions.c`).
fn build_function_parts(func_call: &FunctionCall, command: &str) -> Vec<String> {
    let mut parts = vec![
        command.to_string(),
        quote_if_needed(&func_call.transaction),
        func_call.timeout.to_string(),
        quote_if_needed(&func_call.name),
    ];

    if let Some(access) = func_call.access {
        parts.push(access.to_string());
    }

    if let Some(source) = func_call.source.as_ref() {
        parts.push(quote_if_needed(source));
    }

    parts
}

/// `tokio_util::codec::Decoder` for [`MessageParser`], driven by the
/// `FramedRead<R, MessageParser>` inside `transport::MessageReader`.
///
/// Eats complete lines until [`MessageParser::process_command`] yields a
/// message, then consumes the buffer through the bytes that produced it and
/// yields it. An incomplete line is not an error: `Error::IncompleteLine`
/// makes the call return `Ok(None)` with the partial line left in the buffer
/// until a later read completes it, so messages may straddle reads. Only
/// `Error::IncompleteLine` is ever produced today (`line_parser::parse` has no
/// other failure path), but any other error would end the `MessageReader`
/// stream — `transport` maps every error to `NetdataPluginError::Protocol`.
impl Decoder for MessageParser {
    type Item = Message;
    type Error = crate::line_parser::Error;

    fn decode(&mut self, src: &mut BytesMut) -> crate::line_parser::Result<Option<Self::Item>> {
        let mut total_consumed = 0;
        let buffer = src.as_ref();

        loop {
            let remaining = &buffer[total_consumed..];
            if remaining.is_empty() {
                break;
            }

            let parsed_line = match self.line_parser.parse(remaining) {
                Ok(parsed_line) => parsed_line,
                Err(crate::line_parser::Error::IncompleteLine) => {
                    // Line incomplete: keep the tail for the next call, drop
                    // the lines consumed so far
                    src.advance(total_consumed);
                    return Ok(None);
                }
                Err(e) => {
                    // Propagate the error after dropping the consumed lines
                    src.advance(total_consumed);
                    return Err(e);
                }
            };

            total_consumed += parsed_line.consumed;

            let Some(command) = parsed_line.command else {
                continue;
            };

            if let Some(message) = self.process_command(command) {
                // Message complete: consume everything up to and including
                // this line and yield it; what follows stays for later calls
                src.advance(total_consumed);
                return Ok(Some(message));
            }
        }

        // No complete message in the available data: drop what was consumed
        // and ask for more bytes
        src.advance(total_consumed);
        Ok(None)
    }
}

/// `tokio_util::codec::Encoder` for [`MessageParser`], driven by the
/// `FramedWrite<W, MessageParser>` inside `transport::MessageWriter` (which
/// `MessageWriter::send` flushes after each message). Appends the
/// newline-terminated wire form of each [`Message`] to `dst`, taking the
/// message by value; the parser's direction is irrelevant. Inbound-only
/// variants encode to nothing.
///
/// Current senders exercise only the declaration, result and progress arms:
/// `rt`'s event loop and `otel-plugin`'s supervisor send them through
/// `MessageWriter`, while the bridge emits progress over in-process channels
/// whose drivers translate it into their own progress responses; see the
/// per-variant comments for which arms have an agent-side consumer at all.
impl Encoder<Message> for MessageParser {
    type Error = std::io::Error;

    fn encode(
        &mut self,
        item: Message,
        dst: &mut BytesMut,
    ) -> std::result::Result<(), Self::Error> {
        match item {
            Message::ConfigDeclaration(cfg_decl) => {
                // dyncfg registration, parsed by pluginsd_config
                // (src/plugins.d/pluginsd_dyncfg.c):
                // CONFIG <id> CREATE <status> <type> <path> <source_type>
                // <source> <cmds> <view_access> <edit_access>
                // status/type/source_type print their lowercase vocabulary
                // names and access flags as hex; cmds prints pipe-separated
                // with spaces ("get | schema"), which the agent's
                // space-splitting dyncfg_cmds2id parses by ignoring the "|"
                // words. Caveat: the agent matches the action word
                // case-sensitively against PLUGINSD_KEYWORD_CONFIG_ACTION_CREATE
                // ("create", src/libnetdata/functions_evloop/functions_evloop.h),
                // so this uppercase CREATE is logged as an unknown action and
                // the declaration is not registered. No Rust sender exercises
                // the message outside
                // protocol/examples/config_declaration_encode.rs.
                let parts = vec![
                    "CONFIG".to_string(),
                    quote_if_needed(&cfg_decl.id),
                    "CREATE".to_string(),
                    cfg_decl.status.to_string(),
                    cfg_decl.type_.to_string(),
                    quote_if_needed(&cfg_decl.path),
                    cfg_decl.source_type.to_string(),
                    quote_if_needed(&cfg_decl.source),
                    quote_if_needed(&cfg_decl.cmds.to_string()),
                    cfg_decl.view_access.to_string(),
                    cfg_decl.edit_access.to_string(),
                ];

                dst.extend_from_slice(format!("{}\n", parts.join(" ")).as_bytes());
            }
            Message::FunctionDeclaration(func_decl) => {
                // Function registration, parsed by pluginsd_function
                // (src/plugins.d/pluginsd_functions.c), which reads the words
                // positionally — hence the strictly nested optional fields:
                // FUNCTION [GLOBAL] <name> <timeout_s> <help>
                //   [<tags> [<access> [<priority> [<version>]]]]
                // timeout is in seconds; access prints as hex
                // (HTTP_ACCESS_FORMAT); help/name/tags are quote_if_needed.
                let mut parts = Vec::with_capacity(8);
                parts.push("FUNCTION".to_string());

                if func_decl.global {
                    parts.push("GLOBAL".to_string());
                }

                parts.push(quote_if_needed(&func_decl.name));
                parts.push(func_decl.timeout.to_string());
                parts.push(quote_if_needed(&func_decl.help));

                if let Some(tags) = func_decl.tags.as_ref() {
                    parts.push(quote_if_needed(tags));

                    if let Some(access) = func_decl.access {
                        parts.push(access.to_string());

                        if let Some(priority) = func_decl.priority {
                            parts.push(priority.to_string());

                            if let Some(version) = func_decl.version {
                                parts.push(version.to_string());
                            }
                        }
                    }
                }

                dst.extend_from_slice(format!("{}\n", parts.join(" ")).as_bytes());
            }

            Message::FunctionCall(func_call) => match &func_call.payload {
                // A function call the plugin sends to the agent. No Rust code
                // currently sends one — the Decoder produces these inbound —
                // and the agent has no consumer for it either: its plugin-facing
                // parser reads a plugin-sent FUNCTION as a declaration
                // (pluginsd_function) and has no FUNCTION_PAYLOAD keyword
                // (src/plugins.d/gperf-hashtable.h), so such a line is an
                // unknown keyword that stops the plugin's parser
                // (src/plugins.d/pluginsd_parser.c). The wire form mirrors the
                // call the agent sends to plugins.
                Some(payload) => {
                    // FUNCTION_PAYLOAD <transaction> <timeout_s> <name>
                    // [access [source]], then the raw payload, then the END
                    // marker. The agent's own payload-call header carries a
                    // content_type word (pluginsd_functions.c) that this
                    // header omits.
                    let parts = build_function_parts(&func_call, "FUNCTION_PAYLOAD");
                    dst.extend_from_slice(format!("{}\n", parts.join(" ")).as_bytes());

                    if !payload.is_empty() {
                        dst.extend_from_slice(payload.as_slice());
                        if !payload.ends_with(b"\n") {
                            dst.extend_from_slice(b"\n");
                        }
                    }
                    dst.extend_from_slice(b"FUNCTION_PAYLOAD_END\n");
                }
                None => {
                    // FUNCTION <transaction> <timeout_s> <name> [access [source]]
                    let parts = build_function_parts(&func_call, "FUNCTION");
                    dst.extend_from_slice(format!("{}\n", parts.join(" ")).as_bytes());
                }
            },

            Message::FunctionResult(func_result) => {
                // Function result, parsed by pluginsd_function_result_begin
                // (src/plugins.d/pluginsd_functions.c), which puts the agent
                // into deferred mode collecting raw lines until
                // FUNCTION_RESULT_END (parser_action in
                // src/plugins.d/pluginsd_parser.h):
                // FUNCTION_RESULT_BEGIN <transaction> <status> <format> <expires>
                // Header fields are not quoted, so the transaction must not
                // contain spaces (the agent treats it as a UUID); status is an
                // HTTP status code, format the canonical MIME name
                // (http_content) and expires unix epoch seconds.
                dst.extend_from_slice(
                    format!(
                        "FUNCTION_RESULT_BEGIN {} {} {} {}\n",
                        func_result.transaction,
                        func_result.status,
                        func_result.format,
                        func_result.expires
                    )
                    .as_bytes(),
                );

                // Payload body, `\n`-terminated so the END marker below stays
                // alone on its line (see the Encoder contract in the module
                // docs)
                if !func_result.payload.is_empty() {
                    dst.extend_from_slice(func_result.payload.as_slice());
                    if !func_result.payload.ends_with(b"\n") {
                        dst.extend_from_slice(b"\n");
                    }
                }

                // Closes the deferred block; must be alone on its line
                dst.extend_from_slice(b"FUNCTION_RESULT_END\n");
            }

            Message::FunctionCancel(func_cancel) => {
                // FUNCTION_CANCEL <transaction> — the agent sends this keyword
                // to plugins (PLUGINSD_CALL_FUNCTION_CANCEL,
                // src/plugins.d/pluginsd_functions.c); its plugin-facing
                // parser has no FUNCTION_CANCEL keyword of its own
                // (src/plugins.d/gperf-hashtable.h), so a plugin-sent line
                // like this one has no agent-side handler
                dst.extend_from_slice(
                    format!(
                        "FUNCTION_CANCEL {}\n",
                        quote_if_needed(&func_cancel.transaction)
                    )
                    .as_bytes(),
                );
            }
            Message::FunctionProgressResponse(progress) => {
                // Progress on a call the agent is waiting on, parsed by
                // pluginsd_function_progress
                // (src/plugins.d/pluginsd_functions.c): done and all are
                // unit counts of the same call, not percentages.
                dst.extend_from_slice(
                    format!(
                        "FUNCTION_PROGRESS {} {} {}\n",
                        quote_if_needed(&progress.transaction),
                        progress.done,
                        progress.all,
                    )
                    .as_bytes(),
                );
            }
            Message::FunctionProgressRequest(_) | Message::Quit => {
                // Inbound-only, agent→plugin: FunctionProgressRequest is the
                // agent's nudge for a progress report
                // (PLUGINSD_CALL_FUNCTION_PROGRESS) and Quit its shutdown
                // request (PLUGINSD_CALL_QUIT); nothing is written for them
            }
        }

        Ok(())
    }
}
