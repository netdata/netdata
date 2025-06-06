/**
 * MCP Client for WebSocket communication with Netdata MCP server
 */
class MCPClient {
    constructor() {
        this.ws = null;
        this.url = null;
        this.apiKey = null;
        this.requestId = 1;
        this.pendingRequests = new Map();
        this.connectionPromise = null;
        this.capabilities = null;
        this.serverInfo = null;
        this.tools = new Map();
        this.resources = new Map();
        this.prompts = new Map();
        this.isInitialized = false;
        
        // Event handlers
        this.onConnectionChange = null;
        this.onMessage = null;
        this.onError = null;
        this.onNotification = null;
        this.onLog = null; // New handler for logging
    }

    /**
     * Log a communication event
     */
    log(direction, message, metadata = {}) {
        if (this.onLog) {
            this.onLog({
                timestamp: new Date().toISOString(),
                direction: direction, // 'sent', 'received', 'error', 'info'
                message: message,
                metadata: metadata
            });
        }
    }

    /**
     * Connect to the MCP WebSocket server
     */
    async connect(url) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            throw new Error('Already connected');
        }

        this.url = url;
        
        return new Promise((resolve, reject) => {
            try {
                // URL already contains API key if needed
                this.ws = new WebSocket(url, ['mcp']);
                
                this.ws.onopen = async () => {
                    console.log('WebSocket connected');
                    this.log('info', 'WebSocket connection established', { url: url });
                    if (this.onConnectionChange) {
                        this.onConnectionChange('connected');
                    }
                    
                    try {
                        // Initialize MCP session
                        await this.initialize();
                        resolve();
                    } catch (error) {
                        reject(error);
                    }
                };
                
                this.ws.onmessage = (event) => {
                    this.handleMessage(event.data);
                };
                
                this.ws.onerror = (error) => {
                    console.error('WebSocket error:', error);
                    this.log('error', `WebSocket error: ${error.message || 'Unknown error'}`, { error: error });
                    if (this.onError) {
                        this.onError(error);
                    }
                    reject(error);
                };
                
                this.ws.onclose = (event) => {
                    console.log('WebSocket disconnected');
                    this.log('info', 'WebSocket connection closed', { 
                        code: event.code, 
                        reason: event.reason || 'No reason provided',
                        wasClean: event.wasClean 
                    });
                    this.isInitialized = false;
                    if (this.onConnectionChange) {
                        this.onConnectionChange('disconnected');
                    }
                    this.cleanup();
                };
                
            } catch (error) {
                this.log('error', `Failed to create WebSocket connection: ${error.message}`, { url: url, error: error });
                reject(error);
            }
        });
    }

    /**
     * Initialize MCP session with the server
     */
    async initialize() {
        // Send initialize request
        const initResponse = await this.sendRequest('initialize', {
            protocolVersion: '2024-11-05',
            capabilities: {
                tools: {},
                logging: {}
            },
            clientInfo: {
                name: 'Netdata MCP Web Client',
                version: '1.0.0'
            }
        });

        this.serverInfo = initResponse.serverInfo;
        this.capabilities = initResponse.capabilities;
        
        // Notify server that we're initialized
        await this.sendNotification('notifications/initialized', {});
        
        // List available tools
        if (this.capabilities?.tools) {
            await this.listTools();
        }
        
        // List available resources
        if (this.capabilities?.resources) {
            await this.listResources();
        }
        
        // List available prompts
        if (this.capabilities?.prompts) {
            await this.listPrompts();
        }
        
        this.isInitialized = true;
    }

    /**
     * List available tools from the server
     */
    async listTools() {
        const response = await this.sendRequest('tools/list', {});
        if (response.tools) {
            this.tools.clear();
            response.tools.forEach(tool => {
                this.tools.set(tool.name, tool);
            });
        }
        return response.tools;
    }

    /**
     * List available resources from the server
     */
    async listResources() {
        const response = await this.sendRequest('resources/list', {});
        if (response.resources) {
            this.resources.clear();
            response.resources.forEach(resource => {
                this.resources.set(resource.uri, resource);
            });
        }
        return response.resources;
    }

    /**
     * List available prompts from the server
     */
    async listPrompts() {
        const response = await this.sendRequest('prompts/list', {});
        if (response.prompts) {
            this.prompts.clear();
            response.prompts.forEach(prompt => {
                this.prompts.set(prompt.name, prompt);
            });
        }
        return response.prompts;
    }

    /**
     * Call a tool on the MCP server
     */
    async callTool(toolName, args = {}) {
        if (!this.tools.has(toolName)) {
            throw new Error(`Tool '${toolName}' not found`);
        }
        
        return await this.sendRequest('tools/call', {
            name: toolName,
            arguments: args
        });
    }

    /**
     * Read a resource from the MCP server
     */
    async readResource(uri) {
        if (!this.resources.has(uri)) {
            throw new Error(`Resource '${uri}' not found`);
        }
        
        return await this.sendRequest('resources/read', {
            uri: uri
        });
    }

    /**
     * Get a prompt from the MCP server
     */
    async getPrompt(promptName, args = {}) {
        if (!this.prompts.has(promptName)) {
            throw new Error(`Prompt '${promptName}' not found`);
        }
        
        return await this.sendRequest('prompts/get', {
            name: promptName,
            arguments: args
        });
    }

    /**
     * Send a JSON-RPC request to the server
     */
    async sendRequest(method, params = {}) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            throw new Error('WebSocket is not connected');
        }

        const id = this.requestId++;
        const request = {
            jsonrpc: '2.0',
            method: method,
            params: params,
            id: id
        };

        return new Promise((resolve, reject) => {
            this.pendingRequests.set(id, { resolve, reject });
            const requestStr = JSON.stringify(request);
            this.log('sent', requestStr, { method, params });
            this.ws.send(requestStr);
            
            // Set timeout for request
            setTimeout(() => {
                if (this.pendingRequests.has(id)) {
                    this.pendingRequests.delete(id);
                    this.log('error', `Request ${id} timed out`, { method, id });
                    reject(new Error(`Request ${id} timed out`));
                }
            }, 30000); // 30 second timeout
        });
    }

    /**
     * Send a JSON-RPC notification to the server
     */
    async sendNotification(method, params = {}) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            throw new Error('WebSocket is not connected');
        }

        const notification = {
            jsonrpc: '2.0',
            method: method,
            params: params
        };

        const notificationStr = JSON.stringify(notification);
        this.log('sent', notificationStr, { method, params, type: 'notification' });
        this.ws.send(notificationStr);
    }

    /**
     * Handle incoming messages from the server
     */
    handleMessage(data) {
        try {
            this.log('received', data);
            const message = JSON.parse(data);
            
            // Handle response to a request
            if (message.id !== undefined) {
                const pending = this.pendingRequests.get(message.id);
                if (pending) {
                    this.pendingRequests.delete(message.id);
                    if (message.error) {
                        this.log('error', `Request ${message.id} failed: ${message.error.message}`, { error: message.error });
                        pending.reject(new Error(message.error.message || 'Unknown error'));
                    } else {
                        pending.resolve(message.result);
                    }
                }
            }
            // Handle notifications from server
            else if (message.method) {
                if (this.onNotification) {
                    this.onNotification(message.method, message.params);
                }
            }
            
            // Pass message to general handler
            if (this.onMessage) {
                this.onMessage(message);
            }
            
        } catch (error) {
            console.error('Error parsing message:', error);
            this.log('error', `Failed to parse message: ${error.message}`, { rawData: data });
            if (this.onError) {
                this.onError(error);
            }
        }
    }

    /**
     * Disconnect from the MCP server
     */
    disconnect() {
        if (this.ws) {
            this.log('info', 'Closing WebSocket connection', { url: this.url, state: 'disconnecting' });
            this.ws.close();
        }
    }

    /**
     * Clean up resources
     */
    cleanup() {
        this.pendingRequests.clear();
        this.tools.clear();
        this.resources.clear();
        this.prompts.clear();
        this.ws = null;
    }

    /**
     * Check if connected and initialized
     */
    isReady() {
        return this.ws && 
               this.ws.readyState === WebSocket.OPEN && 
               this.isInitialized;
    }
}

// Export for use in other modules
window.MCPClient = MCPClient;
