#!/usr/bin/env node

const WebSocket = require('ws');
const path = require('path');

// Get program name for logs
const PROGRAM_NAME = path.basename(process.argv[1] || 'nd-mcp-nodejs');

function usage() {
    console.error(`${PROGRAM_NAME}: Usage: ${PROGRAM_NAME} [--bearer TOKEN] ws://host/path`);
    process.exit(1);
}

const parsedArgs = process.argv.slice(2);
let targetURL = '';
let bearerToken = '';

for (let i = 0; i < parsedArgs.length;) {
    const arg = parsedArgs[i];

    if (arg === '--bearer') {
        if (i + 1 >= parsedArgs.length) usage();
        bearerToken = parsedArgs[i + 1].trim();
        i += 2;
    }
    else if (arg.startsWith('--bearer=')) {
        bearerToken = arg.substring('--bearer='.length).trim();
        i += 1;
    }
    else {
        if (targetURL) usage();
        targetURL = arg;
        i += 1;
    }
}

if (!targetURL) usage();

if (!bearerToken) {
    const envToken = process.env.ND_MCP_BEARER_TOKEN;
    if (envToken) bearerToken = envToken.trim();
}

if (bearerToken) {
    console.error(`${PROGRAM_NAME}: Authorization header enabled for MCP connection`);
}

// Reconnection settings
const MAX_RECONNECT_DELAY_MS = 60000; // 60 seconds
const BASE_DELAY_MS = 1000;           // 1 second
const CONNECTION_TIMEOUT_MS = 5000;   // 5 seconds timeout for initial connection
let reconnectAttempt = 0;
let ws = null;
let messageQueue = [];
let reconnectTimeout = null;
let stdinActive = true;
let connectingInProgress = false;
let pendingRequests = new Map(); // Store pending requests by their IDs

// Parse JSON-RPC message and extract ID if present
function parseJsonRpcMessage(message) {
    try {
        const data = JSON.parse(message);
        if (data && typeof data === 'object' && data.jsonrpc === '2.0') {
            return [data.id, data.method];
        }
        return [null, null];
    } catch (err) {
        return [null, null];
    }
}

// Create a JSON-RPC error response
function createJsonRpcError(id, code, message, data = null) {
    const response = {
        jsonrpc: '2.0',
        id: id,
        error: {
            code: code,
            message: message
        }
    };
    
    if (data !== null) {
        response.error.data = data;
    }
    
    return JSON.stringify(response);
}

// Handle request timeout for connection establishment only
function handleRequestTimeout(msgId) {
    // This timeout ONLY applies if we're still not connected after waiting
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        if (pendingRequests.has(msgId)) {
            console.error(`${PROGRAM_NAME}: Connection timeout for request ID ${msgId}, sending error response`);
            
            // Create and send error response
            const errorResponse = createJsonRpcError(
                msgId,
                -32000, // Server error code
                "MCP server connection failed",
                { details: "Could not establish connection to Netdata within timeout period" }
            );
            
            try {
                process.stdout.write(errorResponse + "\n");
            } catch (err) {
                console.error(`${PROGRAM_NAME}: ERROR: Failed to write error response to stdout: ${err.message}`);
            }
            
            // Get the original message from pendingRequests
            const originalMessage = pendingRequests.get(msgId);
            
            // Remove from pending requests
            pendingRequests.delete(msgId);
            
            // Also remove from messageQueue if it exists there
            const msgIndex = messageQueue.indexOf(originalMessage);
            if (msgIndex !== -1) {
                messageQueue.splice(msgIndex, 1);
                console.error(`${PROGRAM_NAME}: Removed timed-out request from message queue`);
            }
        }
    } else {
        // If we get here, it means the connection was established before the timeout
        // Just clear the pending request without any action
        if (pendingRequests.has(msgId)) {
            // Connection succeeded in time, just remove the tracking
            pendingRequests.delete(msgId);
        }
    }
}

// Set up stdin processing once
process.stdin.setEncoding('utf8');
process.stdin.on('data', (data) => {
    try {
        // Flag to check if we received actual data
        let hasContent = false;
        
        data.split(/\r?\n/).forEach(line => {
            if (line.trim() !== '') {
                hasContent = true;
                
                // Parse the message for JSON-RPC ID
                const [msgId, method] = parseJsonRpcMessage(line);
                
                if (ws && ws.readyState === WebSocket.OPEN) {
                    // If connected, send immediately and track requests with IDs
                    ws.send(line);
                    
                    // If this is a request with an ID, track it (but don't set a timeout)
                    if (msgId !== null && msgId !== undefined) {
                        pendingRequests.set(msgId, line);
                    }
                } else {
                    // Queue the message if not connected
                    messageQueue.push(line);
                    
                    // If this is a request with an ID, set a timeout for CONNECTION establishment
                    if (msgId !== null && msgId !== undefined) {
                        console.error(`${PROGRAM_NAME}: Received request with ID ${msgId}, setting connection timeout`);
                        pendingRequests.set(msgId, line);
                        
                        // Set timeout to send error if connection not established quickly
                        // This ONLY applies to connection establishment, not request processing
                        setTimeout(() => handleRequestTimeout(msgId), CONNECTION_TIMEOUT_MS);
                    }
                }
            }
        });
        
        // If disconnected and we got content, try to reconnect immediately
        if (hasContent && (!ws || ws.readyState !== WebSocket.OPEN)) {
            console.error(`${PROGRAM_NAME}: Received stdin data, attempting immediate reconnection`);
            // Clear any pending reconnect timeout
            if (reconnectTimeout) {
                clearTimeout(reconnectTimeout);
                reconnectTimeout = null;
            }
            
            // Only attempt connection if not already connecting
            if (!connectingInProgress) {
                connect(true); // immediate=true
            }
        }
    } catch (err) {
        console.error(`${PROGRAM_NAME}: ERROR: Failed to process data: ${err.message}`);
    }
});

process.stdin.on('error', (err) => {
    console.error(`${PROGRAM_NAME}: ERROR: stdin: ${err.message}`);
    stdinActive = false;
    process.exit(1);
});

process.stdin.on('end', () => {
    console.error(`${PROGRAM_NAME}: End of stdin, will exit when WebSocket disconnects`);
    stdinActive = false;
    
    // If WebSocket is not active, exit immediately
    if (!ws || ws.readyState !== WebSocket.OPEN) {
        process.exit(0);
    }
});

function connect(immediate = false) {
    // Clear any existing reconnect timeout
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
    }
    
    // If stdin is no longer active (closed/error) and we're disconnected, exit
    if (!stdinActive && (!ws || ws.readyState !== WebSocket.OPEN)) {
        console.error(`${PROGRAM_NAME}: Stdin closed and WebSocket disconnected, exiting`);
        process.exit(0);
    }
    
    if (immediate || reconnectAttempt === 0) {
        attemptConnection();
    } else {
        // Exponential backoff with jitter
        const delay = Math.min(
            MAX_RECONNECT_DELAY_MS,
            BASE_DELAY_MS * Math.pow(2, reconnectAttempt - 1) * (0.5 + Math.random())
        );
        
        console.error(`${PROGRAM_NAME}: Reconnecting in ${(delay/1000).toFixed(1)} seconds (attempt ${reconnectAttempt+1})...`);
        reconnectTimeout = setTimeout(() => {
            reconnectTimeout = null;
            attemptConnection();
        }, delay);
    }
}

function attemptConnection() {
    // Prevent multiple concurrent connection attempts
    if (connectingInProgress) {
        return;
    }
    
    connectingInProgress = true;
    console.error(`${PROGRAM_NAME}: Connecting to ${targetURL}...`);
    
    // Close any existing websocket
    if (ws) {
        try {
            ws.terminate();
        } catch (e) {
            // Ignore errors
        }
        ws = null;
    }
    
    const wsOptions = {
        perMessageDeflate: true,
        maxPayload: 16 * 1024 * 1024, // 16MB to match Netdata's limits
        handshakeTimeout: 10000, // 10 seconds for initial handshake
        followRedirects: true, // Follow HTTP redirects
        // Keep the connection alive with pings
        pingInterval: 30000, // 30 seconds
        pingTimeout: 10000   // 10 seconds to wait for pong
    };
    
    if (bearerToken) {
        wsOptions.headers = {
            Authorization: `Bearer ${bearerToken}`
        };
    }

    ws = new WebSocket(targetURL, wsOptions);
    
    // Set a timeout for initial connection
    const connectionTimeout = setTimeout(() => {
        if (ws && ws.readyState !== WebSocket.OPEN) {
            console.error(`${PROGRAM_NAME}: Connection timeout, closing...`);
            ws.terminate();
        }
    }, 15000); // 15 seconds connection timeout
    
    ws.on('open', () => {
        clearTimeout(connectionTimeout);
        console.error(`${PROGRAM_NAME}: Connected`);
        connectingInProgress = false;
        reconnectAttempt = 0; // Reset counter on successful connection
        
        // Process any queued messages
        if (messageQueue.length > 0) {
            console.error(`${PROGRAM_NAME}: Sending ${messageQueue.length} queued message(s)`);
            const queueCopy = [...messageQueue];
            messageQueue = [];
            queueCopy.forEach(msg => {
                try {
                    ws.send(msg);
                    
                    // If this was a pending request with an ID, remove it from pending
                    const [msgId] = parseJsonRpcMessage(msg);
                    if (msgId !== null && pendingRequests.has(msgId)) {
                        pendingRequests.delete(msgId);
                    }
                    
                    // Ensure we don't let the message queue grow indefinitely
                    if (messageQueue.length > 1000) {
                        console.error(`${PROGRAM_NAME}: Message queue too large (${messageQueue.length}), trimming older messages`);
                        messageQueue = messageQueue.slice(-500); // Keep only the 500 most recent messages
                    }
                } catch (err) {
                    console.error(`${PROGRAM_NAME}: ERROR: Failed to send queued message: ${err.message}`);
                    // Re-queue the message if connection is still open
                    if (ws.readyState === WebSocket.OPEN) {
                        messageQueue.push(msg);
                    }
                }
            });
        }
    });
    
    ws.on('message', (message) => {
        try {
            process.stdout.write(message + "\n");
            
            // If this is a response to a request, check if we can remove it from pending
            try {
                const data = JSON.parse(message);
                if (data && typeof data === 'object' && data.jsonrpc === '2.0' && data.id !== undefined) {
                    if (pendingRequests.has(data.id)) {
                        pendingRequests.delete(data.id);
                    }
                }
            } catch (err) {
                // Ignore parsing errors
            }
            
            // Memory management: Don't let pendingRequests grow indefinitely
            if (pendingRequests.size > 1000) {
                console.error(`${PROGRAM_NAME}: Too many pending requests (${pendingRequests.size}), clearing older ones`);
                // Since Map iteration is in insertion order, we keep the newest entries
                const entries = Array.from(pendingRequests.entries());
                pendingRequests.clear();
                entries.slice(-500).forEach(([id, msg]) => pendingRequests.set(id, msg));
            }
        } catch (err) {
            console.error(`${PROGRAM_NAME}: ERROR: Failed to write to stdout: ${err.message}`);
            // If stdout is broken, exit
            process.exit(1);
        }
    });
    
    ws.on('close', (code, reason) => {
        clearTimeout(connectionTimeout);
        console.error(`${PROGRAM_NAME}: WebSocket closed with code ${code}${reason ? ': ' + reason : ''}`);
        connectingInProgress = false;
        
        // Only increment reconnect attempt if we're still supposed to be running
        if (stdinActive) {
            reconnectAttempt++;
            connect();
        } else {
            console.error(`${PROGRAM_NAME}: Stdin closed, exiting`);
            process.exit(0);
        }
    });
    
    ws.on('error', (err) => {
        console.error(`${PROGRAM_NAME}: ERROR: WebSocket error: ${err.message}`);
        connectingInProgress = false;
        // The close event should handle reconnection
    });
    
    // Verify the connection is healthy periodically
    ws.on('pong', () => {
        // Connection is alive
    });
}

// Handle process termination
process.on('SIGINT', () => {
    console.error(`${PROGRAM_NAME}: Received SIGINT, shutting down`);
    // Clear any pending reconnect
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
        reconnectTimeout = null;
    }
    
    if (ws) {
        try {
            ws.close(1000, 'Normal closure');
        } catch (e) {
            // Ignore errors during cleanup
            try {
                ws.terminate();
            } catch (e) {
                // Last resort
            }
        }
    }
    process.exit(0);
});

process.on('SIGTERM', () => {
    console.error(`${PROGRAM_NAME}: Received SIGTERM, shutting down`);
    // Same cleanup as SIGINT
    if (reconnectTimeout) {
        clearTimeout(reconnectTimeout);
    }
    
    if (ws) {
        try {
            ws.close(1000, 'Normal closure');
        } catch (e) {
            try {
                ws.terminate();
            } catch (e) {
                // Ignore
            }
        }
    }
    process.exit(0);
});

// Start the connection process
connect();
