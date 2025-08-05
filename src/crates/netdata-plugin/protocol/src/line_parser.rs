#![allow(dead_code)]

//! Low-level line parser for Netdata's plugin protocol.

use crate::word_iterator::WordIterator;

/// Tokens for pluginsd protocol commands
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Token {
    Chart,
    ChartDefinitionEnd,
    Dimension,
    Begin,
    End,
    Set,
    Flush,
    Disable,
    Variable,
    Label,
    Overwrite,
    Clabel,
    ClabelCommit,
    Exit,
    Begin2,
    Set2,
    End2,
    HostDefine,
    HostDefineEnd,
    HostLabel,
    Host,
    ReplayChart,
    Rbegin,
    Rset,
    RdState,
    RsState,
    Rend,
    Function,
    FunctionResultBegin,
    FunctionResultEnd,
    FunctionPayloadBegin,
    FunctionPayloadEnd,
    FunctionCancel,
    FunctionProgress,
    Quit,
    Config,
    NodeId,
    ClaimedId,
    Json,
    JsonPayloadEnd,
    StreamPath,
    MlModel,
    TrustDurations,
    DynCfg,
    DynCfgRegisterModule,
    DynCfgRegisterJob,
    DynCfgReset,
    ReportJobStatus,
    DeleteJob,
}

// Include the generated phf map
include!(concat!(env!("OUT_DIR"), "/tokens.rs"));

/// Result type for parser operations
pub type Result<T> = core::result::Result<T, Error>;

/// Parser errors
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Error {
    /// Need more data to complete parsing
    IncompleteLine,
    /// Line that is not recognized
    MalformedLine,
    /// I/O error
    Io,
}

// Implement conversion from std::io::Error for tokio_util compatibility
impl From<std::io::Error> for Error {
    fn from(_: std::io::Error) -> Self {
        Error::Io
    }
}

/// Commands supported by Netdata's protocol
pub enum Command<'a> {
    // Chart definition commands
    Chart { args: &'a [u8] },
    Dimension { args: &'a [u8] },
    Variable { args: &'a [u8] },
    ChartDefinitionEnd,

    // Data update commands
    Begin { args: &'a [u8] },
    Set { args: &'a [u8] },
    End { args: &'a [u8] },

    // Function commands
    Function { args: &'a [u8] },
    FunctionPayloadBegin { args: &'a [u8] },
    FunctionPayload { args: &'a [u8] },
    FunctionResultBegin { args: &'a [u8] },
    FunctionResultEnd { args: &'a [u8] },
    FunctionCancel { args: &'a [u8] },
    FunctionProgress { args: &'a [u8] },

    // Label commands
    Clabel { args: &'a [u8] },
    ClabelCommit,

    // Multi-line commands (these would trigger state changes)
    Json { args: &'a [u8] },

    // Payload data (when in multi-line mode)
    FunctionResultPayload { data: &'a [u8] },
    FunctionPayloadData { data: &'a [u8] },

    // Unknown command (fallback)
    Unknown,

    EmptyLine,
}

impl core::fmt::Debug for Command<'_> {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            Command::Chart { args } => f
                .debug_struct("Chart")
                .field("args", &ByteStr(args))
                .finish(),
            Command::Dimension { args } => f
                .debug_struct("Dimension")
                .field("args", &ByteStr(args))
                .finish(),
            Command::Variable { args } => f
                .debug_struct("Variable")
                .field("args", &ByteStr(args))
                .finish(),
            Command::ChartDefinitionEnd => f.write_str("ChartDefinitionEnd"),
            Command::Begin { args } => f
                .debug_struct("Begin")
                .field("args", &ByteStr(args))
                .finish(),
            Command::Set { args } => f.debug_struct("Set").field("args", &ByteStr(args)).finish(),
            Command::End { args } => f.debug_struct("End").field("args", &ByteStr(args)).finish(),
            Command::Function { args } => f
                .debug_struct("Function")
                .field("args", &ByteStr(args))
                .finish(),
            Command::FunctionPayloadBegin { args } => f
                .debug_struct("FunctionPayloadBegin")
                .field("args", &ByteStr(args))
                .finish(),
            Command::FunctionPayload { args } => f
                .debug_struct("FunctionPayload")
                .field("args", &ByteStr(args))
                .finish(),
            Command::Clabel { args } => f
                .debug_struct("Clabel")
                .field("args", &ByteStr(args))
                .finish(),
            Command::ClabelCommit => f.write_str("ClabelCommit"),
            Command::Json { args } => f
                .debug_struct("Json")
                .field("args", &ByteStr(args))
                .finish(),
            Command::FunctionResultPayload { data } => f
                .debug_struct("FunctionResultPayload")
                .field("data", &ByteStr(data))
                .finish(),
            Command::FunctionPayloadData { data } => f
                .debug_struct("FunctionPayloadData")
                .field("data", &ByteStr(data))
                .finish(),
            Command::Unknown => f.debug_struct("Unknown").finish(),
            Command::EmptyLine => f.write_str("EmptyLine"),
            Command::FunctionResultBegin { args } => f
                .debug_struct("FunctionResultBegin")
                .field("data", &ByteStr(args))
                .finish(),
            Command::FunctionResultEnd { args } => f
                .debug_struct("FunctionResultEnd")
                .field("data", &ByteStr(args))
                .finish(),
            Command::FunctionCancel { args } => f
                .debug_struct("FunctionCancel")
                .field("args", &ByteStr(args))
                .finish(),
            Command::FunctionProgress { args } => f
                .debug_struct("FunctionProgress")
                .field("args", &ByteStr(args))
                .finish(),
        }
    }
}

// Helper wrapper for displaying byte slices as strings
struct ByteStr<'a>(&'a [u8]);

impl core::fmt::Debug for ByteStr<'_> {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        write!(f, "\"")?;
        for &byte in self.0 {
            match byte {
                b'\n' => write!(f, "\\n")?,
                b'\r' => write!(f, "\\r")?,
                b'\t' => write!(f, "\\t")?,
                b'\\' => write!(f, "\\\\")?,
                b'"' => write!(f, "\\\"")?,
                0x20..=0x7E => write!(f, "{}", byte as char)?,
                _ => write!(f, "\\x{:02x}", byte)?,
            }
        }
        write!(f, "\"")?;
        Ok(())
    }
}

/// A parsed line with references to the original buffer
#[derive(Debug)]
pub struct ParsedLine<'a> {
    /// The parsed command
    pub command: Option<Command<'a>>,
    /// Byte offset where this line ends in the original buffer (after line terminator)
    pub consumed: usize,
}

/// Parser state for handling multi-line commands
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParserState {
    /// Normal line-by-line parsing mode
    Normal,
    /// Inside a FUNCTION_RESULT block, looking for FUNCTION_RESULT_END
    FunctionResult,
    /// Inside a FUNCTION_PAYLOAD block, looking for FUNCTION_PAYLOAD_END
    FunctionPayload,
    /// Inside a JSON block, looking for JSON_PAYLOAD_END
    JsonPayload,
}

impl Default for ParserState {
    fn default() -> Self {
        Self::Normal
    }
}

/// Low-level line parser
#[derive(Default, Debug)]
pub struct LineParser {
    state: ParserState,
    /// Position where the current payload block started
    payload_start: Option<usize>,
}

impl LineParser {
    /// Parse the next line or payload block from the buffer
    pub fn parse<'a>(&mut self, buffer: &'a [u8]) -> Result<ParsedLine<'a>> {
        match self.state {
            ParserState::Normal => self.parse_normal_line(buffer),
            ParserState::FunctionResult => self.parse_payload_block(buffer, b"FUNCTION_RESULT_END"),
            ParserState::FunctionPayload => {
                self.parse_payload_block(buffer, b"FUNCTION_PAYLOAD_END")
            }
            ParserState::JsonPayload => self.parse_payload_block(buffer, b"JSON_PAYLOAD_END"),
        }
    }

    /// Parse a normal line (terminated by \n)
    fn parse_normal_line<'a>(&mut self, buffer: &'a [u8]) -> Result<ParsedLine<'a>> {
        let Some(newline_pos) = find_byte(buffer, b'\n') else {
            return Err(Error::IncompleteLine);
        };

        let mut words = WordIterator::new(&buffer[..newline_pos]);
        let consumed = newline_pos + 1;

        let Some(token) = words.next() else {
            return Ok(ParsedLine {
                command: None,
                consumed,
            });
        };

        let args = words.remainder();

        // Parse the command word using perfect hash lookup
        let command = match COMMAND_MAP.get(token) {
            Some(Token::Chart) => Command::Chart { args },
            Some(Token::ChartDefinitionEnd) => Command::ChartDefinitionEnd,
            Some(Token::Dimension) => Command::Dimension { args },
            Some(Token::Clabel) => Command::Clabel { args },
            Some(Token::ClabelCommit) => Command::ClabelCommit,

            Some(Token::Begin) => Command::Begin { args },
            Some(Token::Set) => Command::Set { args },
            Some(Token::End) => Command::End { args },
            Some(Token::Function) => Command::Function { args },
            Some(Token::FunctionResultBegin) => {
                self.state = ParserState::FunctionResult;
                self.payload_start = Some(0);
                Command::FunctionResultBegin { args }
            }
            Some(Token::FunctionResultEnd) => Command::FunctionResultEnd { args },
            Some(Token::FunctionPayloadBegin) => {
                self.state = ParserState::FunctionPayload;
                self.payload_start = Some(0);
                Command::FunctionPayloadBegin { args }
            }
            Some(Token::FunctionPayloadEnd) => Command::FunctionPayload { args },
            Some(Token::FunctionProgress) => Command::FunctionProgress { args },
            Some(Token::FunctionCancel) => Command::FunctionCancel { args },
            Some(Token::Json) => {
                self.state = ParserState::JsonPayload;
                self.payload_start = Some(0);
                Command::Json { args }
            }

            Some(token) => {
                panic!("Unhandled line parser token: {:#?}", token)
            }
            None => Command::Unknown,
        };

        Ok(ParsedLine {
            command: Some(command),
            consumed,
        })
    }

    /// Parse a payload block (data between start and end markers)
    fn parse_payload_block<'a>(
        &mut self,
        buffer: &'a [u8],
        end_marker: &[u8],
    ) -> Result<ParsedLine<'a>> {
        let start_offset = self.payload_start.unwrap_or(0);

        // If start_offset is beyond buffer, we need to adjust
        if start_offset >= buffer.len() {
            return Err(Error::IncompleteLine);
        }

        // Look for the end marker on its own line, starting from the payload start
        let search_buffer = &buffer[start_offset..];
        let mut pos = 0;

        while pos < search_buffer.len() {
            if let Some(newline_pos) = find_byte(&search_buffer[pos..], b'\n') {
                let line_start = pos;
                let line_end = pos + newline_pos;
                let line = &search_buffer[line_start..line_end];

                // Check if this line is the end marker
                if line == end_marker {
                    // Found the end marker
                    let payload_end = line_start; // End of payload (before the end marker line)
                    let consumed = start_offset + line_end + 1; // Total consumed from original buffer

                    // The payload is everything from start to just before the end marker
                    let payload = if payload_end > 0 {
                        &search_buffer[..payload_end - 1] // Exclude the newline before end marker
                    } else {
                        b"" // Empty payload
                    };

                    let command = match self.state {
                        ParserState::FunctionResult => {
                            Command::FunctionResultPayload { data: payload }
                        }
                        ParserState::FunctionPayload => {
                            Command::FunctionPayloadData { data: payload }
                        }
                        ParserState::JsonPayload => {
                            Command::FunctionResultPayload { data: payload }
                        } // JSON uses same as function result
                        ParserState::Normal => {
                            unreachable!("Should not be parsing payload in Normal state")
                        }
                    };

                    self.state = ParserState::Normal;
                    self.payload_start = None;

                    return Ok(ParsedLine {
                        command: Some(command),
                        consumed,
                    });
                }

                // Move to the next line
                pos = line_end + 1;
            } else {
                // No complete line found, need more data
                return Err(Error::IncompleteLine);
            }
        }

        Err(Error::IncompleteLine)
    }
}

/// Helper function to find a byte in a slice
fn find_byte(haystack: &[u8], needle: u8) -> Option<usize> {
    haystack.iter().position(|&b| b == needle)
}
