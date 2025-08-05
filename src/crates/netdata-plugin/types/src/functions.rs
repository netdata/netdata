use crate::HttpAccess;

/// A function declaration message for Netdata's external plugin protocol
#[derive(Debug, Clone)]
pub struct FunctionDeclaration {
    /// True if the function is global
    pub global: bool,
    /// The name of the function
    pub name: String,
    /// Timeout in seconds for function execution
    pub timeout: u32,
    /// Help text describing what the function does
    pub help: String,
    /// Tags of the function
    pub tags: Option<String>,
    /// Access control flags for the function
    pub access: Option<HttpAccess>,
    /// Priority level for function execution
    pub priority: Option<u32>,
    /// Version of the function
    pub version: Option<u32>,
}

impl FunctionDeclaration {
    pub fn new(name: &str, help: &str) -> Self {
        Self {
            name: String::from(name),
            help: String::from(help),
            timeout: 10,
            tags: Some("logs".to_string()),
            access: Some(HttpAccess::from_u32(0)),
            priority: Some(200),
            version: Some(1),
            global: false,
        }
    }
}

/// A function call message for invoking functions
#[derive(Debug, Clone)]
pub struct FunctionCall {
    /// Transaction ID for this function call
    pub transaction: String,
    /// Timeout in seconds for function execution
    pub timeout: u32,
    /// Function name to call
    pub name: String,
    /// Function arguments
    pub args: Vec<String>,
    /// Access control flags for the function
    pub access: Option<HttpAccess>,
    /// Source information containing caller details
    pub source: Option<String>,
    /// Payload data for the function call (optional)
    pub payload: Option<Vec<u8>>,
}

/// A function result message containing the response payload
#[derive(Debug, Clone)]
pub struct FunctionResult {
    /// Transaction ID or unique identifier for this function call
    pub transaction: String,
    /// Status of the function call
    pub status: u32,
    /// Content type of the result (e.g., "application/json", "text/plain")
    pub format: String,
    /// Expires timestamp
    pub expires: u64,
    /// Result payload data
    pub payload: Vec<u8>,
}

/// A function cancel message for terminating function execution
#[derive(Debug, Clone)]
pub struct FunctionCancel {
    /// Transaction ID of the function call to cancel
    pub transaction: String,
}

/// A message for reporting function call progress
#[derive(Debug, Clone)]
pub struct FunctionProgress {
    /// Transaction ID of the function call that should report the progress
    pub transaction: String,
}
