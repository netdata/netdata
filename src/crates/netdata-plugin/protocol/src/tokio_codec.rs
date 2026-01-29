//! tokio_util::codec implementations for the Netdata protocol

use crate::message_parser::{Message, MessageParser};
use bytes::{Buf, BytesMut};
use netdata_plugin_types::FunctionCall;
use tokio_util::codec::{Decoder, Encoder};

/// Helper function to quote strings if needed
fn quote_if_needed(s: &str) -> String {
    if s.is_empty() {
        "''".to_string()
    } else if s.contains([' ', '\t', '\'', '"']) {
        // Use single quotes and escape any single quotes within
        format!("'{}'", s.replace('\'', "\\'"))
    } else {
        s.to_string()
    }
}

/// Helper function to build function command parts
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

/// Decoder implementation for MessageParser
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
                    // Need more data - advance buffer by what we consumed so far
                    src.advance(total_consumed);
                    return Ok(None);
                }
                Err(e) => {
                    // Advance buffer and return error
                    src.advance(total_consumed);
                    return Err(e);
                }
            };

            total_consumed += parsed_line.consumed;

            let Some(command) = parsed_line.command else {
                continue;
            };

            if let Some(message) = self.process_command(command) {
                // Found a complete message - advance buffer and return it
                src.advance(total_consumed);
                return Ok(Some(message));
            }
        }

        // Processed all available data but no complete message yet
        src.advance(total_consumed);
        Ok(None)
    }
}

/// Encoder implementation for MessageParser to serialize Messages back to protocol format
impl Encoder<Message> for MessageParser {
    type Error = std::io::Error;

    fn encode(
        &mut self,
        item: Message,
        dst: &mut BytesMut,
    ) -> std::result::Result<(), Self::Error> {
        match item {
            Message::ConfigDeclaration(cfg_decl) => {
                // CONFIG <id> CREATE <status> <type> <path> <source_type> <source> <cmds> <view_access> <edit_access>
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
                // FUNCTION [GLOBAL] name timeout help [tags [access [priority [version]]]
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
                Some(payload) => {
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
                    let parts = build_function_parts(&func_call, "FUNCTION");
                    dst.extend_from_slice(format!("{}\n", parts.join(" ")).as_bytes());
                }
            },

            Message::FunctionResult(func_result) => {
                // FUNCTION_RESULT_BEGIN [transaction_id] [status_code] [content_type] [expires]
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

                // Function payload data
                if !func_result.payload.is_empty() {
                    dst.extend_from_slice(func_result.payload.as_slice());
                    if !func_result.payload.ends_with(b"\n") {
                        dst.extend_from_slice(b"\n");
                    }
                }

                // FUNCTION_RESULT_END
                dst.extend_from_slice(b"FUNCTION_RESULT_END\n");
            }

            Message::FunctionCancel(func_cancel) => {
                // FUNCTION_CANCEL transaction
                dst.extend_from_slice(
                    format!(
                        "FUNCTION_CANCEL {}\n",
                        quote_if_needed(&func_cancel.transaction)
                    )
                    .as_bytes(),
                );
            }
            Message::FunctionProgress(_) => {
                unimplemented!()
            }
        }

        Ok(())
    }
}
