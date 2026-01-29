use crate::http_content::HttpContent;
use crate::line_parser::{Command, LineParser};
use crate::word_iterator::WordIterator;
use netdata_plugin_types::*;

/// Parser direction configuration
#[derive(Debug, Clone, Copy, PartialEq)]
enum ParserDirection {
    /// Input parser - (parsing commands from the agent to plugins)
    Input,
    /// Output parser (parsing commands from plugins to the agent)
    Output,
}

/// High-level message parser that groups related commands
#[derive(Debug)]
pub struct MessageParser {
    pub(crate) line_parser: LineParser,
    current_message: Option<Message>,
    direction: ParserDirection,
}

impl Default for MessageParser {
    fn default() -> Self {
        Self::new(ParserDirection::Output)
    }
}

#[derive(Debug)]
pub enum Message {
    FunctionDeclaration(Box<FunctionDeclaration>),
    FunctionCall(Box<FunctionCall>),
    FunctionResult(Box<FunctionResult>),
    FunctionCancel(Box<FunctionCancel>),
    FunctionProgress(Box<FunctionProgress>),
    ConfigDeclaration(Box<ConfigDeclaration>),
}

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
    /// Create a new message parser with the specified direction
    fn new(direction: ParserDirection) -> Self {
        Self {
            line_parser: LineParser::default(),
            current_message: None,
            direction,
        }
    }

    /// Create a new input parser (parsing commands sent TO plugins)
    pub fn input() -> Self {
        Self::new(ParserDirection::Input)
    }

    /// Create a new output parser (parsing commands FROM plugins)
    pub fn output() -> Self {
        Self::new(ParserDirection::Output)
    }

    /// Process a single command and optionally return a completed message
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
                if let Some(Message::FunctionCall(func_call)) = &mut self.current_message {
                    if let Some(ref mut payload) = func_call.payload {
                        payload.extend_from_slice(data);
                    } else {
                        func_call.payload = Some(data.to_vec());
                    }
                }
                self.current_message.take()
            }

            Command::FunctionPayload { args: _ } => None,

            Command::FunctionResultPayload { data } => {
                if let Some(Message::FunctionResult(func_result)) = &mut self.current_message {
                    func_result.payload.extend_from_slice(data);
                }
                None
            }

            Command::FunctionResultEnd { args: _ } => self.current_message.take(),

            Command::FunctionCancel { args } => {
                self.current_message = self.parse_function_cancel(args);
                self.current_message.take()
            }

            Command::FunctionProgress { args } => {
                self.current_message = self.parse_function_progress(args);
                self.current_message.take()
            }

            Command::Begin { args: _ } | Command::Set { args: _ } | Command::End { args: _ } => {
                None
            }

            cmd => {
                eprintln!("Got cmd: {:#?}", cmd);
                None
            }
        }
    }

    /// Parse FUNCTION command arguments - behavior depends on parser direction
    fn parse_function(&self, args: &[u8]) -> Option<Message> {
        match self.direction {
            ParserDirection::Input => self.parse_function_call(args),
            ParserDirection::Output => self.parse_function_declaration(args),
        }
    }

    /// Parse FUNCTION command arguments for output context (function definition)
    fn parse_function_declaration(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let (global, name) = {
            let first_word = words.next_string()?;

            if first_word == "GLOBAL" {
                // GLOBAL flag is present, next word is the function name
                (true, words.next_string()?)
            } else {
                // No GLOBAL flag, first word is the function name
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

    /// Parse FUNCTION command arguments for input context (function call)
    /// Expected format: FUNCTION transaction timeout function access source
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

    /// Parse FUNCTION_RESULT_BEGIN command and create FunctionResult with payload
    /// Expected format: FUNCTION_RESULT_BEGIN transaction status format expires
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

    /// Parse FUNCTION_PAYLOAD command and create FunctionCall with empty payload
    /// Expected format: FUNCTION_PAYLOAD transaction timeout function access source
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

    /// Parse FUNCTION_CANCEL command
    /// Expected format: FUNCTION_CANCEL transaction
    fn parse_function_cancel(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let function_cancel = Box::new(FunctionCancel { transaction });

        Some(Message::FunctionCancel(function_cancel))
    }

    /// Parse FUNCTION_PROGRESS command
    /// Expected format: FUNCTION_PROGRESS transaction
    fn parse_function_progress(&self, args: &[u8]) -> Option<Message> {
        let mut words = WordIterator::new(args);

        let transaction = words.next_string()?;
        let function_progress = Box::new(FunctionProgress { transaction });

        Some(Message::FunctionProgress(function_progress))
    }
}
