use thiserror::Error;

/// Result type for netdata plugin operations
pub type Result<T> = std::result::Result<T, NetdataPluginError>;

/// Error types that can occur in netdata plugin operations
#[derive(Error, Debug)]
pub enum NetdataPluginError {
    /// Transport layer error (I/O, network)
    #[error("transport error: {0}")]
    Transport(#[from] std::io::Error),
    
    /// Protocol parsing or communication error
    #[error("protocol error: {message}")]
    Protocol { message: String },
    
    /// Runtime error during plugin execution
    #[error("runtime error: {message}")]  
    Runtime { message: String },
    
    /// Configuration error
    #[error("configuration error: {message}")]
    Config { message: String },
    
    /// Function handler error
    #[error("function handler error: {message}")]
    FunctionHandler { message: String },
    
    /// Schema validation error
    #[error("schema validation error: {message}")]
    Schema { message: String },
    
    /// Transport is closed
    #[error("transport is closed")]
    Closed,
    
    /// Generic error with custom message
    #[error("{message}")]
    Other { message: String },
}