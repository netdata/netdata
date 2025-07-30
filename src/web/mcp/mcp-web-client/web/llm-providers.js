/**
 * LLM Provider integrations for OpenAI, Anthropic, and Google
 */

// Type definitions for API responses

/**
 * @typedef {Object} OpenAIMessage
 * @property {string} role
 * @property {string} content
 * @property {Array<OpenAIToolCall>} [tool_calls]
 */

/**
 * @typedef {Object} OpenAIToolCall
 * @property {string} id
 * @property {string} type
 * @property {Object} function
 * @property {string} function.name
 * @property {string} function.arguments
 */

/**
 * @typedef {Object} OpenAIChoice
 * @property {OpenAIMessage} message
 * @property {number} index
 * @property {string} finish_reason
 */

/**
 * @typedef {Object} OpenAIUsage
 * @property {number} prompt_tokens
 * @property {number} completion_tokens
 * @property {number} total_tokens
 */

/**
 * @typedef {Object} OpenAIResponse
 * @property {Array<OpenAIChoice>} choices
 * @property {OpenAIUsage} [usage]
 * @property {string} id
 * @property {string} model
 */

/**
 * @typedef {Object} AnthropicContent
 * @property {string} type - 'text' or 'tool_use'
 * @property {string} [text]
 * @property {string} [id]
 * @property {string} [name]
 * @property {Object} [input]
 */

/**
 * @typedef {Object} AnthropicUsage
 * @property {number} input_tokens
 * @property {number} output_tokens
 * @property {number} [cache_read_input_tokens]
 * @property {number} [cache_creation_input_tokens]
 */

/**
 * @typedef {Object} AnthropicResponse
 * @property {Array<AnthropicContent>} content
 * @property {AnthropicUsage} [usage]
 * @property {string} id
 * @property {string} model
 * @property {string} role
 */

/**
 * @typedef {Object} GoogleFunctionCall
 * @property {string} name
 * @property {Object} args
 */

/**
 * @typedef {Object} GooglePart
 * @property {string} [text]
 * @property {GoogleFunctionCall} [functionCall]
 */

/**
 * @typedef {Object} GoogleCandidate
 * @property {Object} content
 * @property {Array<GooglePart>} content.parts
 * @property {string} content.role
 * @property {number} index
 * @property {string} [finishReason] - 'STOP', 'MAX_TOKENS', 'SAFETY', etc.
 */

/**
 * @typedef {Object} GoogleUsageMetadata
 * @property {number} promptTokenCount
 * @property {number} candidatesTokenCount
 * @property {number} totalTokenCount
 */

/**
 * @typedef {Object} GoogleResponse
 * @property {Array<GoogleCandidate>} candidates
 * @property {GoogleUsageMetadata} [usageMetadata]
 */

// Internal message format types

/**
 * @typedef {Object} InternalMessage
 * @property {string} role - 'user', 'assistant', 'system', 'tool-results', etc.
 * @property {string|Array} content - Message content
 * @property {Array<ToolCall>} [toolCalls] - Tool calls for assistant messages
 * @property {Array<ToolResult>} [toolResults] - Tool results
 * @property {string} [type] - Message type
 * @property {string} [timestamp] - ISO timestamp
 */

/**
 * @typedef {Object} ToolCall
 * @property {string} id - Tool call ID
 * @property {string} name - Tool name
 * @property {Object} arguments - Tool arguments
 * @property {boolean} [includeInContext] - Whether to include in context
 */

/**
 * @typedef {Object} ToolResult
 * @property {string} toolCallId - ID of the tool call this result belongs to
 * @property {any} result - Tool execution result
 * @property {string} [toolName] - Name of the tool
 * @property {string} [id] - Result ID
 */

/**
 * @typedef {Object} LLMResponse
 * @property {string} content - Response content
 * @property {Array<ToolCall>} toolCalls - Tool calls to execute
 * @property {Object|null} usage - Token usage information
 * @property {number} [usage.promptTokens]
 * @property {number} [usage.completionTokens]
 * @property {number} [usage.totalTokens]
 * @property {number} [usage.cacheReadInputTokens]
 * @property {number} [usage.cacheCreationInputTokens]
 */

/**
 * Shared utility functions for message conversion
 */
const MessageConversionUtils = {
    /**
     * Integrates summary content into system prompt
     * @param {string} systemPrompt - Original system prompt
     * @param {string} summaryContent - Summary to integrate
     * @returns {string} Updated system prompt
     */
    integrateSummaryIntoSystemPrompt(systemPrompt, summaryContent) {
        if (!summaryContent) return systemPrompt;
        
        return `${systemPrompt}

Previous Conversation Summary:
${summaryContent}`;
    },
    
    /**
     * Extracts system prompt and optional summary from messages
     * @param {Array} messages - Validated messages array
     * @returns {Object} { systemPrompt: string, messages: Array, hasSummary: boolean }
     */
    extractSystemAndSummary(messages) {
        let systemPrompt = '';
        let summaryContent = '';
        let hasSummary = false;
        const remainingMessages = [];
        
        // First message is always system
        if (messages[0] && messages[0].role === 'system') {
            systemPrompt = messages[0].content;
        }
        
        // Check if second message is summary
        let startIdx = 1;
        if (messages.length > 1 && messages[1].role === 'summary') {
            summaryContent = messages[1].content;
            hasSummary = true;
            startIdx = 2;
        }
        
        // Collect remaining messages
        for (let i = startIdx; i < messages.length; i++) {
            remainingMessages.push(messages[i]);
        }
        
        // Integrate summary into system prompt if present
        if (hasSummary) {
            systemPrompt = MessageConversionUtils.integrateSummaryIntoSystemPrompt(systemPrompt, summaryContent);
        }
        
        return { systemPrompt, messages: remainingMessages, hasSummary };
    },
    
    /**
     * Formats MCP tool result content for providers
     * MCP returns { content: Array<{type: string, text?: string, data?: string, mimeType?: string}> }
     * @param {Object} result - MCP tool result
     * @returns {Object} Formatted result based on content type
     */
    formatMCPToolResult(result) {
        // Handle direct string results
        if (typeof result === 'string') {
            return { type: 'text', content: result };
        }
        
        // Handle MCP content array format
        if (result && result.content && Array.isArray(result.content)) {
            const formattedItems = [];
            
            for (const item of result.content) {
                if (item.type === 'text' && item.text) {
                    formattedItems.push({
                        type: 'text',
                        content: item.text
                    });
                } else if (item.type === 'image' && item.data) {
                    formattedItems.push({
                        type: 'image',
                        data: item.data,
                        mimeType: item.mimeType || 'image/png'
                    });
                } else if (item.type === 'resource' && item.resource) {
                    // Handle resource content type from MCP
                    formattedItems.push({
                        type: 'resource',
                        uri: item.resource.uri,
                        text: item.resource.text,
                        mimeType: item.resource.mimeType
                    });
                }
            }
            
            return { type: 'multi', items: formattedItems };
        }
        
        // Handle plain objects or other types
        return { type: 'json', content: result };
    }
};

/**
 * Validates messages array before sending to LLM API with strict sequence checking
 * @param {Array} messages - Messages array to validate
 * @throws {Error} If validation fails
 */
function validateMessagesForAPI(messages) {
    // Validation starts - removed verbose logging
    
    // Check for empty array
    if (!messages || messages.length === 0) {
        const error = 'No messages to send to API';
        console.error('[validateMessagesForAPI]', error);
        throw new Error(error);
    }
    
    // First message MUST be system
    if (messages[0].role !== 'system') {
        const error = `First message MUST be system, but found: ${messages[0].role}`;
        console.error('[validateMessagesForAPI]', error);
        console.error('[validateMessagesForAPI] Messages:', messages);
        throw new Error(error);
    }
    
    // Check for multiple system messages
    const systemCount = messages.filter(m => m.role === 'system').length;
    if (systemCount > 1) {
        const error = `Messages contain ${systemCount} system messages, but only 1 is allowed`;
        console.error('[validateMessagesForAPI]', error);
        throw new Error(error);
    }
    
    // Check for multiple summary messages
    const summaryCount = messages.filter(m => m.role === 'summary').length;
    if (summaryCount > 1) {
        const error = `Messages contain ${summaryCount} summary messages, but only 0 or 1 is allowed`;
        console.error('[validateMessagesForAPI]', error);
        throw new Error(error);
    }
    
    // Start index for sequence validation (skip system and optional summary)
    let startIdx = 1;
    
    // If second message is summary, skip it
    if (messages.length > 1 && messages[1].role === 'summary') {
        startIdx = 2;
    }
    
    // If we have no more messages after system (and optional summary), that's valid
    if (startIdx >= messages.length) {
        return true;
    }
    
    // Strict sequence validation: user -> assistant -> [tool-results -> assistant] -> user -> ...
    let expectedRole = 'user';
    let lastAssistantMessage = null;
    
    for (let i = startIdx; i < messages.length; i++) {
        const msg = messages[i];
        const msgRole = msg.role;
        
        // Check role at position i
        
        if (msgRole === 'user') {
            if (expectedRole !== 'user' && expectedRole !== 'user-or-tool-results') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'user'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.role || m.type}`));
                throw new Error(error);
            }
            expectedRole = 'assistant';
            lastAssistantMessage = null;
            
        } else if (msgRole === 'assistant') {
            if (expectedRole !== 'assistant' && expectedRole !== 'user-or-tool-results') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'assistant'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.role || m.type}`));
                throw new Error(error);
            }
            lastAssistantMessage = msg;
            // After assistant, we can have either tool-results, user, or another assistant
            expectedRole = 'user-or-tool-results';
            
        } else if (msgRole === 'tool-results') {
            if (expectedRole !== 'user-or-tool-results') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'tool-results'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.role || m.type}`));
                throw new Error(error);
            }
            
            // Validate tool results match the assistant's tool calls
            if (!lastAssistantMessage) {
                const error = `Tool results at position ${i} have no preceding assistant message`;
                console.error('[validateMessagesForAPI]', error);
                throw new Error(error);
            }
            
            // Validate tool calls match
            validateToolCalls(lastAssistantMessage, msg, i);
            
            // After tool-results, we must have assistant
            expectedRole = 'assistant';
            
        } else {
            const error = `Unexpected message role at position ${i}: '${msgRole}'. Allowed: user, assistant, tool-results`;
            console.error('[validateMessagesForAPI]', error);
            console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.role || m.type}`));
            throw new Error(error);
        }
    }
    
    // Final state validation
    // Valid ending states:
    // - expectedRole === 'user' → Ended with tool-results, waiting for user
    // - expectedRole === 'assistant' → Ended with user, waiting for assistant  
    // - expectedRole === 'user-or-tool-results' → Ended with assistant, can continue with either user or tools
    // All of these are valid states for sending to LLM
    
    // Special case: If the last message is a user message (expectedRole === 'assistant'),
    // it might be an orphaned message from a failed response (e.g., MAX_TOKENS).
    // This is valid for conversations but creates issues when sending to API.
    // For now, we allow it and let the provider handle it.
    
    // All validations passed
    return true;
}

/**
 * Extract tool calls from content array
 * @param {Array|string} content - Message content
 * @returns {Array} Array of tool calls
 */
function extractToolCallsFromContent(content) {
    if (!Array.isArray(content)) return [];
    return content
        .filter(block => block.type === 'tool_use')
        .map(block => ({
            id: block.id,
            name: block.name,
            arguments: block.input
        }));
}

/**
 * Validates that tool results match the assistant's tool calls exactly
 * @param {Object} assistantMsg - The assistant message with tool calls
 * @param {Object} toolResultsMsg - The tool-results message
 * @param {number} position - Position in messages array for error reporting
 * @throws {Error} If validation fails
 */
function validateToolCalls(assistantMsg, toolResultsMsg, position) {
    // Extract tool calls from assistant message content
    const toolCalls = extractToolCallsFromContent(assistantMsg.content);
    // STRICT: Only accept toolResults property
    const toolResults = toolResultsMsg.toolResults || [];
    
    if (!toolResultsMsg.toolResults) {
        const error = `Tool results message at position ${position} missing required 'toolResults' property`;
        console.error('[validateToolCalls]', error);
        console.error('[validateToolCalls] Message keys:', Object.keys(toolResultsMsg));
        console.error('[validateToolCalls] Full message:', toolResultsMsg);
        throw new Error(error);
    }
    
    // Validate tool calls count matches results count
    
    // Check counts match
    if (toolCalls.length !== toolResults.length) {
        const error = `Tool call mismatch at position ${position}: assistant requested ${toolCalls.length} tools, but got ${toolResults.length} results`;
        console.error('[validateToolCalls]', error);
        console.error('[validateToolCalls] Assistant message object:', assistantMsg);
        console.error('[validateToolCalls] Tool results message object:', toolResultsMsg);
        console.error('[validateToolCalls] Tool calls:', toolCalls.map(tc => ({ id: tc.id, name: tc.function?.name })));
        console.error('[validateToolCalls] Tool results:', toolResults.map(tr => ({ id: tr.toolCallId, name: tr.toolName })));
        throw new Error(error);
    }
    
    // Create a map of tool calls by ID for validation
    const toolCallMap = new Map();
    for (const call of toolCalls) {
        if (!call.id) {
            const error = `Tool call at position ${position} missing required 'id' field`;
            console.error('[validateToolCalls]', error);
            console.error('[validateToolCalls] Tool call:', call);
            throw new Error(error);
        }
        if (toolCallMap.has(call.id)) {
            const error = `Duplicate tool call ID '${call.id}' at position ${position}`;
            console.error('[validateToolCalls]', error);
            throw new Error(error);
        }
        toolCallMap.set(call.id, call);
    }
    
    // Validate each tool result
    for (const result of toolResults) {
        if (!result.toolCallId) {
            const error = `Tool result at position ${position} missing required 'toolCallId' field`;
            console.error('[validateToolCalls]', error);
            console.error('[validateToolCalls] Tool result:', result);
            throw new Error(error);
        }
        
        const matchingCall = toolCallMap.get(result.toolCallId);
        if (!matchingCall) {
            const error = `Tool result at position ${position} references unknown tool call ID '${result.toolCallId}'`;
            console.error('[validateToolCalls]', error);
            console.error('[validateToolCalls] Available IDs:', Array.from(toolCallMap.keys()));
            throw new Error(error);
        }
        
        // Mark as matched
        toolCallMap.delete(result.toolCallId);
    }
    
    // Check if any tool calls were not matched
    if (toolCallMap.size > 0) {
        const unmatchedIds = Array.from(toolCallMap.keys());
        const error = `Tool calls not matched by results at position ${position}: ${unmatchedIds.join(', ')}`;
        console.error('[validateToolCalls]', error);
        throw new Error(error);
    }
    
    // Tool validation passed
}

class LLMProvider {
    constructor(proxyUrl = 'http://localhost:8081') {
        this.onLog = null; // Logging callback
        this.proxyUrl = proxyUrl;
    }

    /**
     * Send messages to LLM provider - must be implemented by subclass
     * @abstract
     * @param {Array} _messages - Array of message objects
     * @param {Array} _tools - Array of available tools
     * @param {number} _temperature - Temperature for response generation
     * @param {string} _mode - Tool inclusion mode
     * @param {number|null} _cachePosition - Cache position for Anthropic
     * @returns {Promise<LLMResponse>}
     */
    async sendMessage(_messages, _tools, _temperature, _mode, _cachePosition = null, _chat = null) {
        const error = 'sendMessage must be implemented by subclass';
        console.error('[LLMProvider]', error);
        throw new Error(error);
    }

    log(direction, message, metadata = {}) {
        const logEntry = {
            timestamp: new Date().toISOString(),
            direction,
            message,
            metadata
        };
        
        // Log to UI callback only, not console
        
        // UI log
        if (this.onLog) {
            this.onLog(logEntry);
        }
    }

    /**
     * Check if tool metadata should be injected
     * @returns {boolean}
     */
    shouldInjectToolMetadata(chat) {
        // If no chat provided, can't inject metadata
        if (!chat) {
            return false;
        }
        
        // Don't inject metadata in sub-chats to prevent recursion
        if (chat.isSubChat) {
            return false;
        }
        
        // Check if tool summarization is enabled
        return chat.config?.optimisation?.toolSummarisation?.enabled === true;
    }

    /**
     * Inject metadata fields into tool schema
     * @param {Object} tool - The tool object
     * @param {Object} chat - The chat context
     * @returns {Object} - Tool with injected metadata fields
     */
    injectToolMetadata(tool, chat) {
        if (!this.shouldInjectToolMetadata(chat)) {
            return tool;
        }
        
        // Clone the tool to avoid modifying the original
        const modifiedTool = JSON.parse(JSON.stringify(tool));
        
        // Ensure inputSchema exists
        if (!modifiedTool.inputSchema) {
            modifiedTool.inputSchema = { type: 'object', properties: {} };
        }
        if (!modifiedTool.inputSchema.properties) {
            modifiedTool.inputSchema.properties = {};
        }
        
        // Inject metadata fields
        const metadataFields = {
            tool_purpose: {
                type: 'string',
                description: 'Why this tool is being used in the context of the user query'
            },
            expected_format: {
                type: 'string',
                description: 'Expected structure or format of the data you want from this tool'
            },
            key_information: {
                type: 'string',
                description: 'Specific values, patterns, or information to extract from the response'
            },
            success_indicators: {
                type: 'string',
                description: 'How to determine if the tool response is useful'
            },
            context_for_interpretation: {
                type: 'string',
                description: 'Additional context from the user discussion that may needed to interpret the tool results'
            }
        };
        
        // Add metadata fields to the tool schema
        Object.assign(modifiedTool.inputSchema.properties, metadataFields);
        
        return modifiedTool;
    }

    /**
     * Check request size before sending to API
     * @param {Object} requestBody - The request body to check
     * @param {number} maxSizeBytes - Maximum allowed size in bytes (default 400KB)
     * @throws {Error} If request exceeds size limit
     */
    checkRequestSize(requestBody, maxSizeBytes = 400 * 1024) {
        const jsonString = JSON.stringify(requestBody);
        const sizeInBytes = new TextEncoder().encode(jsonString).length;
        
        if (sizeInBytes > maxSizeBytes) {
            const sizeKB = (sizeInBytes / 1024).toFixed(1);
            const maxKB = (maxSizeBytes / 1024).toFixed(1);
            const errorMsg = `Request size is ${sizeKB} KiB. Maximum allowed is ${maxKB} KiB.`;
            console.error(`[LLMProvider] Request size exceeded:`, errorMsg);
            console.error(`[LLMProvider] Request details:`, {
                model: requestBody.model,
                messagesCount: requestBody.messages?.length || requestBody.contents?.length || 0,
                toolsCount: requestBody.tools?.length || 0,
                sizeBytes: sizeInBytes,
                maxBytes: maxSizeBytes
            });
            throw new Error(errorMsg);
        }
        
        this.log('info', `Request size: ${(sizeInBytes / 1024).toFixed(1)} KiB`, {
            sizeBytes: sizeInBytes,
            maxBytes: maxSizeBytes
        });
    }
    
    // Tool filtering removed - now handled entirely by message optimizer
    // The mode parameter now controls cache control behavior only
    
    /**
     * Get timezone info
     * @returns {{name: string, offset: string}}
     */
    getTimezoneInfo() {
        const date = new Date();
        const offset = date.getTimezoneOffset();
        const absOffset = Math.abs(offset);
        const hours = Math.floor(absOffset / 60);
        const minutes = absOffset % 60;
        const sign = offset <= 0 ? '+' : '-';
        const offsetString = `UTC${sign}${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}`;
        
        let timezoneName;
        try {
            // This returns something like "America/New_York"
            timezoneName = Intl.DateTimeFormat().resolvedOptions().timeZone;
        } catch {
            // Fallback to basic timezone string
            timezoneName = date.toString().match(/\(([^)]+)\)/)?.[1] || offsetString;
        }
        
        return {
            name: timezoneName,
            offset: offsetString
        };
    }
    
    /**
     * Add datetime prefix to user message content
     * @param {string} content - The original user message content
     * @param {string} timestamp - The message timestamp (ISO string)
     * @returns {string} The content with datetime prefix
     */
    addDateTimePrefix(content, timestamp) {
        // Use the message's timestamp, fallback to current time if not provided
        const messageDateTime = timestamp || new Date().toISOString();
        const timezoneInfo = this.getTimezoneInfo();
        return `Current datetime in rfc3339: ${messageDateTime}, timezone: ${timezoneInfo.name}\n\n${content}`;
    }
}

/**
 * Model endpoint configuration
 * Specifies which endpoint to use and whether tools are supported
 */
const MODEL_ENDPOINT_CONFIG = {
    // Models that MUST use /v1/responses
    'o3-pro': { endpoint: 'responses', supportsTools: true },
    'o3-pro-2025-06-10': { endpoint: 'responses', supportsTools: true },
    'o1-pro': { endpoint: 'responses', supportsTools: false },
    'o1-pro-2025-03-19': { endpoint: 'responses', supportsTools: false },
    
    // o3 models use /v1/responses with tool support
    'o3': { endpoint: 'responses', supportsTools: true },
    'o3-2025-04-16': { endpoint: 'responses', supportsTools: true },
    'o3-mini': { endpoint: 'responses', supportsTools: true },
    'o3-mini-2025-01-31': { endpoint: 'responses', supportsTools: true },
    
    // o1 models use /v1/responses without tool support
    'o1': { endpoint: 'responses', supportsTools: false },
    'o1-mini': { endpoint: 'responses', supportsTools: false },
    'o1-preview': { endpoint: 'responses', supportsTools: false },
    'o1-2024-12-17': { endpoint: 'responses', supportsTools: false },
    'o1-preview-2024-09-12': { endpoint: 'responses', supportsTools: false },
    'o1-mini-2024-09-12': { endpoint: 'responses', supportsTools: false }
    
    // All other models use /v1/chat/completions
};

/**
 * OpenAI GPT Provider
 */
class OpenAIProvider extends LLMProvider {
    constructor(proxyUrl, model = 'gpt-4-turbo-preview') {
        super(proxyUrl);
        this.model = model;
        this.type = 'openai';
    }

    get apiUrl() {
        // Check model-specific endpoint configuration
        const config = MODEL_ENDPOINT_CONFIG[this.model];
        if (config && config.endpoint === 'responses') {
            return `${this.proxyUrl}/proxy/openai/v1/responses`;
        }
        // Default to chat/completions for all other models
        return `${this.proxyUrl}/proxy/openai/v1/chat/completions`;
    }

    /**
     * Send messages to OpenAI API
     * @param {Array} messages - Array of message objects
     * @param {Array} tools - Array of available tools
     * @param {number} temperature - Temperature for response generation
     * @param {string} mode - Tool inclusion mode
     * @param {number|null} _cachePosition - Cache position (unused for OpenAI)
     * @returns {Promise<LLMResponse>}
     */
    async sendMessage(messages, tools, temperature, mode, _cachePosition = null, chat = null) {
        // Check model configuration for endpoint and tool support
        const modelConfig = MODEL_ENDPOINT_CONFIG[this.model];
        const useResponsesEndpoint = modelConfig && modelConfig.endpoint === 'responses';
        const supportsTools = !modelConfig || modelConfig.supportsTools !== false;
        
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages from internal format to OpenAI format
        const openaiMessages = this.convertMessages(messages, mode);
        
        // Convert tools to OpenAI completions format (with nested function)
        const openaiCompletionsTools = tools.map(tool => {
            const injectedTool = this.injectToolMetadata(tool, chat);
            return {
                type: 'function',
                function: {
                    name: injectedTool.name,
                    description: injectedTool.description,
                    parameters: injectedTool.inputSchema || {}
                }
            };
        });
        
        // Convert tools to OpenAI responses format (requires type and name fields)
        const openaiResponsesTools = tools.map(tool => {
            const injectedTool = this.injectToolMetadata(tool, chat);
            return {
                type: 'function',
                name: injectedTool.name,
                description: injectedTool.description,
                parameters: injectedTool.inputSchema || {}
            };
        });
        
        let requestBody;
        
        if (useResponsesEndpoint) {
            // Extract system prompt for instructions field
            const { systemPrompt } = MessageConversionUtils.extractSystemAndSummary(messages);
            
            // Convert messages to input format (string or array)
            const inputMessages = [];
            for (const msg of openaiMessages) {
                if (msg.role === 'system') continue; // Skip system, use instructions instead
                
                if (msg.role === 'user') {
                    inputMessages.push({
                        role: 'user',
                        content: msg.content || ''
                    });
                } else if (msg.role === 'assistant') {
                    // Extract text content from array if needed
                    let textContent = msg.content;
                    if (Array.isArray(msg.content)) {
                        const textBlocks = msg.content.filter(block => block.type === 'text');
                        textContent = textBlocks.map(block => block.text || '').join('\n\n').trim() || '';
                    }
                    // Ensure content is never null for o3/o1 models
                    if (textContent === null || textContent === undefined) {
                        textContent = '';
                    }
                    inputMessages.push({
                        role: 'assistant',
                        content: textContent
                    });
                } else if (msg.role === 'tool') {
                    // Tool results in responses format
                    inputMessages.push({
                        role: 'tool',
                        tool_call_id: msg.tool_call_id,
                        content: msg.content || ''
                    });
                }
            }
            
            // Build request for v1/responses endpoint
            // Order fields consistently: tools → instructions (system) → input (messages) → model
            requestBody = {};
            
            // Add tools first if supported
            if (supportsTools && openaiResponsesTools.length > 0) {
                requestBody.tools = openaiResponsesTools;
                requestBody.tool_choice = 'auto';
                requestBody.parallel_tool_calls = true;
            }
            
            // Add system prompt as instructions
            if (systemPrompt) {
                requestBody.instructions = systemPrompt;
            }
            
            // Add messages
            requestBody.input = inputMessages;
            
            // Add model and other parameters
            requestBody.model = this.model;
            requestBody.max_output_tokens = 4096;
            requestBody.stream = false;
            requestBody.store = true;
            
            // O3/O1 models don't support temperature parameter
            if (!this.model.startsWith('o3') && !this.model.startsWith('o1')) {
                requestBody.temperature = temperature;
            }
            
            // Optional: Add reasoning configuration for o3/o1 models
            if (this.model.startsWith('o3') || this.model.startsWith('o1')) {
                requestBody.reasoning = { 
                    effort: 'medium',
                    summary: 'detailed'
                };
            }
        } else {
            // Regular models use standard v1/chat/completions structure
            // Order fields consistently: tools → messages → model
            requestBody = {
                tools: openaiCompletionsTools.length > 0 ? openaiCompletionsTools : undefined,
                tool_choice: openaiCompletionsTools.length > 0 ? 'auto' : undefined,
                messages: openaiMessages,
                model: this.model,
                temperature,
                max_tokens: 4096
            };
        }

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'openai', 
            model: this.model,
            url: this.apiUrl
        });

        // Check request size before sending
        this.checkRequestSize(requestBody);

        // Log the full request
        console.log(`[OPENAI SEND] (${mode}, subchat: ${chat?.isSubChat || false}):`, requestBody);

        let response;
        try {
            response = await fetch(this.apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(requestBody)
            });
        } catch (error) {
            this.log('error', `Failed to send request: ${error.message}`, { 
                provider: 'openai',
                error: error.toString(),
                url: this.apiUrl
            });
            if (error.name === 'TypeError' && error.message.includes('Failed to fetch')) {
                const connectionError = `Connection Error: Cannot reach OpenAI API at ${this.apiUrl}. Please ensure the proxy server is running.`;
                console.error('[OpenAIProvider] Connection error:', connectionError, '\nOriginal error:', error);
                throw new Error(connectionError);
            }
            throw error;
        }

        if (!response.ok) {
            const error = await response.json();
            this.log('error', `API error response: ${JSON.stringify(error)}`, { 
                provider: 'openai',
                status: response.status,
                statusText: response.statusText
            });
            
            // Special handling for rate limit errors (429)
            if (response.status === 429) {
                const retryAfter = response.headers.get('retry-after') || response.headers.get('x-ratelimit-reset-after');
                const baseMessage = error.error?.message || 'Rate limit exceeded';
                let rateLimitMessage = `Rate limit exceeded: ${baseMessage}`;
                
                if (retryAfter) {
                    rateLimitMessage += ` (retry after ${retryAfter}s)`;
                }
                
                const apiError = `OpenAI API error: ${rateLimitMessage} (429)`;
                console.error('[OpenAIProvider] Rate limit error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
                throw new Error(apiError);
            }
            
            const apiError = `OpenAI API error: ${error.error?.message || response.statusText}`;
            console.error('[OpenAIProvider] API error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
            throw new Error(apiError);
        }

        /** @type {OpenAIResponse} */
        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'openai' });
        
        // Log the full response
        console.log(`[OPENAI RECEIVED] (${mode}, subchat: ${chat?.isSubChat || false}):`, data);
        
        let choice;
        
        if (useResponsesEndpoint) {
            // All o1/o3 models use v1/responses with different response structure
            if (!data.output) {
                const error = 'OpenAI API returned no output';
                console.error('[OpenAIProvider]', error);
                this.log('error', error, { provider: 'openai', model: this.model });
                throw new Error(error);
            }
            
            // Parse the output array to find the message
            let messageContent = '';
            const toolCalls = [];
            
            if (Array.isArray(data.output)) {
                // o3 format: array of objects with type and content
                for (const outputItem of data.output) {
                    if (outputItem.type === 'message' && outputItem.content) {
                        // Extract text from content array
                        if (Array.isArray(outputItem.content)) {
                            for (const contentItem of outputItem.content) {
                                if (contentItem.type === 'output_text' && contentItem.text) {
                                    messageContent += contentItem.text;
                                }
                            }
                        }
                    } else if (outputItem.type === 'function_call') {
                        // Handle individual function call in v1/responses format
                        let args = outputItem.arguments;
                        // Parse arguments if they're a string
                        if (typeof args === 'string') {
                            try {
                                args = JSON.parse(args);
                            } catch (e) {
                                console.warn('Failed to parse tool arguments:', e);
                                args = {};
                            }
                        }
                        toolCalls.push({
                            id: outputItem.call_id || this.generateId(),
                            name: outputItem.name,
                            arguments: args || {}
                        });
                    } else if (outputItem.type === 'tool_calls') {
                        // Handle tool calls in v1/responses format (array format)
                        // The structure can be either outputItem.calls or outputItem.content
                        const calls = outputItem.calls || outputItem.content || [];
                        for (const toolCall of calls) {
                            // Skip if not a tool call type
                            if (toolCall.type && toolCall.type !== 'tool_call') continue;
                            
                            let args = toolCall.arguments || toolCall.function?.arguments;
                            // Parse arguments if they're a string
                            if (typeof args === 'string') {
                                try {
                                    args = JSON.parse(args);
                                } catch (e) {
                                    console.warn('Failed to parse tool arguments:', e);
                                    args = {};
                                }
                            }
                            toolCalls.push({
                                id: toolCall.id || this.generateId(),
                                name: toolCall.name || toolCall.function?.name,
                                arguments: args || {}
                            });
                        }
                    }
                }
            } else if (typeof data.output === 'string') {
                // o1 format: simple string
                messageContent = data.output;
            } else {
                // Unknown format, stringify as fallback
                console.warn('o3/o1 model returned unknown output format:', data.output);
                messageContent = JSON.stringify(data.output);
            }
            
            choice = {
                message: {
                    content: messageContent,
                    tool_calls: toolCalls.length > 0 ? toolCalls : null
                }
            };
        } else {
            // Standard models use choices array
            if (!data.choices || data.choices.length === 0) {
                const error = 'OpenAI API returned no choices';
                console.error('[OpenAIProvider]', error);
                this.log('error', error, { provider: 'openai', model: this.model });
                throw new Error(error);
            }
            choice = data.choices[0];
        }
        
        // Convert OpenAI response to unified content array format
        const contentArray = [];
        
        // Add text content if present
        if (choice.message.content) {
            contentArray.push({ type: 'text', text: choice.message.content });
        }
        
        // Add tool calls to content array
        if (choice.message.tool_calls) {
            for (const tc of choice.message.tool_calls) {
                // Handle both standard OpenAI format and o3 simplified format
                let toolCallId, toolCallName, toolCallArgs;
                
                if (tc.function) {
                    // Standard OpenAI format: {id, type, function: {name, arguments}}
                    toolCallId = tc.id;
                    toolCallName = tc.function.name;
                    toolCallArgs = tc.function.arguments;
                } else {
                    // o3 simplified format: {id, name, arguments}
                    toolCallId = tc.id;
                    toolCallName = tc.name;
                    toolCallArgs = tc.arguments;
                }
                
                // Parse arguments
                let parsedArgs;
                if (typeof toolCallArgs === 'string') {
                    try {
                        parsedArgs = JSON.parse(toolCallArgs);
                    } catch (_e) {
                        console.error('[OpenAIProvider] Failed to parse tool arguments:', toolCallArgs);
                        parsedArgs = {};
                    }
                } else {
                    // Already parsed (o3 format)
                    parsedArgs = toolCallArgs || {};
                }
                
                contentArray.push({
                    type: 'tool_use',
                    id: toolCallId,
                    name: toolCallName,
                    input: parsedArgs
                });
            }
        }
        
        // Fallback: Parse legacy tool calling formats from content
        if (!choice.message.tool_calls && choice.message.content) {
            const legacyToolCalls = this.parseLegacyToolCalls(choice.message.content);
            if (legacyToolCalls.length > 0) {
                // Remove tool text from content
                const cleanedText = this.cleanContentFromToolCalls(choice.message.content);
                // Replace text content
                contentArray.length = 0;
                if (cleanedText) {
                    contentArray.push({ type: 'text', text: cleanedText });
                }
                // Add legacy tools to content
                for (const tc of legacyToolCalls) {
                    contentArray.push({
                        type: 'tool_use',
                        id: tc.id,
                        name: tc.name,
                        input: tc.arguments
                    });
                }
            }
        }
        
        return {
            content: contentArray,
            toolCalls: [],
            usage: data.usage ? {
                // Handle both standard and responses endpoint formats
                promptTokens: data.usage.prompt_tokens || data.usage.input_tokens || 0,
                completionTokens: data.usage.completion_tokens || data.usage.output_tokens || 0,
                totalTokens: data.usage.total_tokens || 0,
                cacheReadInputTokens: data.usage.prompt_tokens_details?.cached_tokens || data.usage.input_tokens_details?.cached_tokens || 0,
                cacheCreationInputTokens: data.usage.prompt_tokens_details?.cache_creation_tokens || 0,
                // Add reasoning tokens for o3/o1 models
                reasoningTokens: data.usage.completion_tokens_details?.reasoning_tokens || data.usage.output_tokens_details?.reasoning_tokens || 0
            } : null
        };
    }

    /**
     * Parse legacy tool calling formats from content text
     * @param {string} content - Message content that might contain tool calls
     * @returns {Array<ToolCall>} Parsed tool calls
     */
    parseLegacyToolCalls(content) {
        const toolCalls = [];
        
        try {
            // Pattern 0: <|parallel|> format
            // Example: <|parallel|>{ tool_uses: [...] }</|parallel|>
            const parallelPattern = /<\|parallel\|>([\s\S]*?)<\/\|parallel\|>/;
            const parallelMatch = content.match(parallelPattern);
            let searchContent = content;
            
            if (parallelMatch) {
                // Extract content between the tags
                searchContent = parallelMatch[1];
            }
            
            // Pattern 1: JSON-like format with tool_uses array
            // Example: { tool_uses: [{ recipient_name: "function.name", parameters: {...} }] }
            const jsonPattern = /\{\s*(?:"?tool_uses"?|tool_uses)\s*:\s*\[(.*?)\]\s*\}/s;
            const jsonMatch = searchContent.match(jsonPattern);
            
            if (jsonMatch) {
                try {
                    // Clean up JavaScript-style syntax to make it valid JSON
                    let cleanedContent = jsonMatch[1]
                        .replace(/\/\/[^\n\r]*/g, '') // Remove // comments
                        .replace(/\/\*[\s\S]*?\*\//g, '') // Remove /* */ comments
                        .replace(/,(\s*[}\]])/g, '$1') // Remove trailing commas before } or ]
                        .replace(/(\w+)(\s*:)/g, '"$1"$2'); // Quote unquoted property names
                    
                    // Handle values that need quoting but aren't quoted yet
                    // Be careful not to quote numbers or already quoted strings
                    cleanedContent = cleanedContent.replace(/:\s*([a-zA-Z_][a-zA-Z0-9_]*)/g, (match, value) => {
                        // Don't quote if it looks like a number or boolean
                        if (/^(true|false|\d+(\.\d+)?)$/.test(value)) {
                            return match;
                        }
                        return `: "${value}"`;
                    });
                    
                    // Try to parse as valid JSON
                    const toolData = JSON.parse(`{"tool_uses":[${cleanedContent}]}`);
                    if (toolData.tool_uses && Array.isArray(toolData.tool_uses)) {
                        for (const tool of toolData.tool_uses) {
                            if (tool.recipient_name && tool.parameters) {
                                // Extract function name from recipient_name (e.g., "functions.list_alerts" -> "list_alerts")
                                const functionName = tool.recipient_name.split('.').pop();
                                toolCalls.push({
                                    id: this.generateId(),
                                    name: functionName,
                                    arguments: tool.parameters
                                });
                            }
                        }
                    }
                } catch (parseError) {
                    // If JSON parsing fails, try manual parsing
                    this.log('debug', 'Failed to parse tool calls as JSON, trying manual parsing', { 
                        error: parseError.message, 
                        content: jsonMatch[1].substring(0, 200) 
                    });
                    
                    // Manual parsing for each tool object
                    const toolPattern = /\{\s*recipient_name\s*:\s*["']([^"']+)["']\s*,\s*parameters\s*:\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*\}/g;
                    let match;
                    
                    while ((match = toolPattern.exec(jsonMatch[1])) !== null) {
                        try {
                            const functionName = match[1].split('.').pop();
                            let parametersStr = match[2];
                            
                            // Clean up the parameters string more carefully
                            parametersStr = parametersStr
                                .replace(/\/\/[^\n\r]*/g, '') // Remove // comments
                                .replace(/\/\*[\s\S]*?\*\//g, '') // Remove /* */ comments
                                .replace(/,(\s*[}\]])/g, '$1') // Remove trailing commas
                                .replace(/(\w+)(\s*:)/g, '"$1"$2'); // Quote property names
                            
                            // Handle unquoted string values carefully to avoid breaking ISO timestamps
                            parametersStr = parametersStr.replace(/:\s*([a-zA-Z_][a-zA-Z0-9_\-:.]*)/g, (fullMatch, value) => {
                                // Don't quote numbers, booleans, or things that look like ISO timestamps
                                if (/^(true|false|\d+(\.\d+)?|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{3})?Z?)$/.test(value)) {
                                    return fullMatch;
                                }
                                return `: "${value}"`;
                            });
                            
                            const parameters = JSON.parse(`{${parametersStr}}`);
                            
                            toolCalls.push({
                                id: this.generateId(),
                                name: functionName,
                                arguments: parameters
                            });
                        } catch (manualParseError) {
                            this.log('debug', 'Failed to parse individual tool call parameters', { 
                                functionName: match[1], 
                                parametersStr: match[2], 
                                error: manualParseError.message 
                            });
                        }
                    }
                }
            }
            
            // Pattern 2: multi_tool_use.parallel self-closing tag format
            // Example: <multi_tool_use.parallel tool_uses={[...]}/>
            if (toolCalls.length === 0) {
                const multiToolPattern = /<multi_tool_use\.parallel\s+tool_uses=\{(\[[\s\S]*?\])\}\/>/;
                const multiToolMatch = content.match(multiToolPattern);
                
                if (multiToolMatch) {
                    try {
                        // Extract the array content
                        let arrayContent = multiToolMatch[1];
                        
                        // Clean up JavaScript-style syntax
                        arrayContent = arrayContent
                            .replace(/\/\/[^\n\r]*/g, '') // Remove // comments
                            .replace(/recipient_name:\s*/g, '"recipient_name":') // Quote property names
                            .replace(/parameters:\s*/g, '"parameters":')
                            .replace(/(\w+):\s*"([^"]+)"/g, '"$1": "$2"') // Ensure all properties are quoted
                            .replace(/(\w+):\s*(-?\d+)/g, '"$1": $2') // Handle numeric values
                            .replace(/(\w+):\s*\[/g, '"$1": [') // Handle array values
                            .replace(/,(\s*[}\]])/g, '$1'); // Remove trailing commas
                        
                        // Parse the array
                        const toolArray = JSON.parse(arrayContent);
                        
                        for (const tool of toolArray) {
                            if (tool.recipient_name && tool.parameters) {
                                const functionName = tool.recipient_name.split('.').pop();
                                toolCalls.push({
                                    id: this.generateId(),
                                    name: functionName,
                                    arguments: tool.parameters
                                });
                            }
                        }
                    } catch (parseError) {
                        this.log('debug', 'Failed to parse multi_tool_use.parallel format', {
                            error: parseError.message,
                            content: multiToolMatch[1].substring(0, 200)
                        });
                    }
                }
            }
            
            // Pattern 3: Individual tool objects without array wrapper
            // Example: { recipient_name: "function.name", parameters: {...} }
            if (toolCalls.length === 0) {
                const toolPattern = /\{\s*(?:"?recipient_name"?|recipient_name)\s*:\s*["']([^"']+)["']\s*,\s*(?:"?parameters"?|parameters)\s*:\s*\{([^{}]*(?:\{[^{}]*\}[^{}]*)*)\}\s*\}/g;
                let match;
                while ((match = toolPattern.exec(content)) !== null) {
                    try {
                        const functionName = match[1].split('.').pop();
                        let parametersStr = match[2];
                        
                        // Same careful cleaning as above
                        parametersStr = parametersStr
                            .replace(/\/\/[^\n\r]*/g, '')
                            .replace(/\/\*[\s\S]*?\*\//g, '')
                            .replace(/,(\s*[}\]])/g, '$1')
                            .replace(/(\w+)(\s*:)/g, '"$1"$2');
                        
                        parametersStr = parametersStr.replace(/:\s*([a-zA-Z_][a-zA-Z0-9_\-:.]*)/g, (fullMatch, value) => {
                            if (/^(true|false|\d+(\.\d+)?|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{3})?Z?)$/.test(value)) {
                                return fullMatch;
                            }
                            return `: "${value}"`;
                        });
                        
                        const parameters = JSON.parse(`{${parametersStr}}`);
                        
                        toolCalls.push({
                            id: this.generateId(),
                            name: functionName,
                            arguments: parameters
                        });
                    } catch (parseError) {
                        this.log('debug', 'Failed to parse individual tool call', { 
                            match: match[0], 
                            error: parseError.message 
                        });
                    }
                }
            }
        } catch (error) {
            this.log('error', 'Error parsing legacy tool calls', { 
                error: error.message, 
                content: content.substring(0, 200) 
            });
        }
        
        return toolCalls;
    }
    
    /**
     * Remove tool call text from content
     * @param {string} content - Original content
     * @returns {string} Cleaned content
     */
    cleanContentFromToolCalls(content) {
        // Remove <|parallel|> blocks
        let cleaned = content.replace(/<\|parallel\|>[\s\S]*?<\/\|parallel\|>/g, '');
        
        // Remove JSON-like tool call blocks
        cleaned = cleaned.replace(/\{\s*(?:"?tool_uses"?|tool_uses)\s*:\s*\[.*?\]\s*\}/gs, '');
        
        // Remove individual tool call blocks
        cleaned = cleaned.replace(/\{\s*(?:"?recipient_name"?|recipient_name)\s*:.*?\}\s*\}/gs, '');
        
        // Remove multi_tool_use blocks (both opening/closing and self-closing formats)
        cleaned = cleaned.replace(/<multi_tool_use\.parallel>.*?<\/multi_tool_use\.parallel>/gs, '');
        cleaned = cleaned.replace(/<multi_tool_use\.parallel\s+tool_uses=\{[\s\S]*?\}\/>/g, '');
        
        // Clean up extra whitespace and newlines
        cleaned = cleaned.replace(/\n\s*\n\s*\n/g, '\n\n').trim();
        
        return cleaned;
    }
    
    generateId() {
        return 'call_' + Math.random().toString(36).substring(2, 11);
    }

    convertMessages(messages, _mode) {
        // Check if we're using the responses endpoint
        const modelConfig = MODEL_ENDPOINT_CONFIG[this.model];
        const useResponsesEndpoint = modelConfig && modelConfig.endpoint === 'responses';
        
        if (useResponsesEndpoint) {
            // For /v1/responses, we return messages as-is (they'll be used in 'input' field)
            // System prompt will be handled via 'instructions' parameter
            return messages;
        }
        
        // Convert messages for standard OpenAI format
        const converted = [];
        
        // Extract system prompt and handle summary
        const { systemPrompt, messages: remainingMessages } = MessageConversionUtils.extractSystemAndSummary(messages);
        
        // Add system prompt as first message
        if (systemPrompt) {
            converted.push({
                role: 'system',
                content: systemPrompt
            });
        }
        
        // Process remaining messages
        for (const msg of remainingMessages) {
            const msgRole = msg.role;
            
            if (msgRole === 'user') {
                converted.push({
                    role: 'user',
                    content: this.addDateTimePrefix(msg.content, msg.timestamp)
                });
            } else if (msgRole === 'assistant') {
                // Extract text content and tool calls from message
                let textContent = null;
                let toolCalls = [];
                
                if (Array.isArray(msg.content)) {
                    // Extract text blocks
                    const textBlocks = msg.content.filter(block => block.type === 'text');
                    if (textBlocks.length > 0) {
                        textContent = textBlocks.map(block => block.text || '').join('\n\n').trim();
                    }
                    // Extract tool calls
                    toolCalls = extractToolCallsFromContent(msg.content);
                } else if (typeof msg.content === 'string') {
                    textContent = msg.content;
                }
                
                // Convert assistant message to OpenAI format
                const openaiMsg = {
                    role: 'assistant',
                    content: textContent
                };
                
                // Add tool calls if present
                if (toolCalls.length > 0) {
                    openaiMsg.tool_calls = toolCalls.map(tc => ({
                        id: tc.id,
                        type: 'function',
                        function: {
                            name: tc.name,
                            arguments: typeof tc.arguments === 'string' 
                                ? tc.arguments 
                                : JSON.stringify(tc.arguments || {})
                        }
                    }));
                }
                
                converted.push(openaiMsg);
            } else if (msgRole === 'tool-results') {
                // Convert each tool result to OpenAI format
                // STRICT: Only accept toolResults property
                if (msg.toolResults && Array.isArray(msg.toolResults)) {
                    for (const toolResult of msg.toolResults) {
                        converted.push(this.formatToolResponse(
                            toolResult.toolCallId,
                            toolResult.result,
                            toolResult.toolName
                        ));
                    }
                }
            } else {
                console.warn('[OpenAI] Unexpected message role:', msgRole);
            }
        }
        
        return converted;
    }

    formatToolResponse(toolCallId, result, _toolName) {
        // Format MCP tool results for OpenAI
        const formatted = MessageConversionUtils.formatMCPToolResult(result);
        let content;
        
        if (formatted.type === 'text') {
            content = formatted.content;
        } else if (formatted.type === 'multi') {
            // Handle multiple content items from MCP
            const parts = [];
            for (const item of formatted.items) {
                if (item.type === 'text') {
                    parts.push(item.content);
                } else if (item.type === 'image') {
                    // OpenAI expects base64 images in a specific format for vision models
                    // For now, we'll just indicate an image was returned
                    parts.push(`[Image: ${item.mimeType}]`);
                } else if (item.type === 'resource') {
                    parts.push(`[Resource: ${item.uri}]\n${item.text || ''}`);
                }
            }
            content = parts.join('\n\n');
        } else if (formatted.type === 'json') {
            content = JSON.stringify(formatted.content, null, 2);
        } else {
            content = JSON.stringify(result);
        }
        
        return {
            role: 'tool',
            tool_call_id: toolCallId,
            content
        };
    }
}

/**
 * Anthropic Claude Provider
 */
class AnthropicProvider extends LLMProvider {
    constructor(proxyUrl, model = 'claude-3-opus-20240229') {
        super(proxyUrl);
        this.model = model;
        this.type = 'anthropic';
    }

    get apiUrl() {
        return `${this.proxyUrl}/proxy/anthropic/v1/messages`;
    }

    /**
     * Send messages to Anthropic API
     * @param {Array} messages - Array of message objects
     * @param {Array} tools - Array of available tools
     * @param {number} temperature - Temperature for response generation
     * @param {string} mode - Tool inclusion mode
     * @param {number|null} cachePosition - Cache position for Anthropic
     * @returns {Promise<LLMResponse>}
     */
    async sendMessage(messages, tools, temperature, mode, cachePosition = null, chat = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages to Anthropic format
        let anthropicMessages, system;
        
        if (mode === 'cached') {
            // Use the caching version which returns different format
            const result = this.convertMessagesWithCaching(messages, mode, cachePosition);
            anthropicMessages = result.converted;
            // Extract system from original messages for cached mode
            const systemMsg = messages.find(m => m.role === 'system');
            if (systemMsg) {
                system = [{
                    type: 'text',
                    text: systemMsg.content,
                    cache_control: { type: 'ephemeral' }  // Cache system prompt for 'cached' mode
                }];
            }
        } else {
            // Use regular conversion which handles system properly
            const result = this.convertMessages(messages, mode);
            anthropicMessages = result.messages;
            if (result.system) {
                system = [{
                    type: 'text',
                    text: result.system
                }];
                // Add cache control to system prompt for 'system' mode
                if (mode === 'system') {
                    system[0].cache_control = { type: 'ephemeral' };
                }
            }
        }
        
        // Convert tools to Anthropic format (no cache control on tools)
        const anthropicTools = tools.map(tool => {
            const injectedTool = this.injectToolMetadata(tool, chat);
            return {
                name: injectedTool.name,
                description: injectedTool.description,
                input_schema: injectedTool.inputSchema || {}
            };
        });

        // Order fields according to Anthropic's cache hierarchy: tools → system → messages → model
        // This ensures efficient caching as tools change least frequently, then system, then messages
        // Model comes after messages so cache can be reused across different models
        const requestBody = {
            tools: anthropicTools.length > 0 ? anthropicTools : undefined,
            system,
            messages: anthropicMessages,
            model: this.model,
            max_tokens: 4096,
            temperature
        };

        // Removed debug logging for message structure validation
        /*
        anthropicMessages.map((msg, idx) => ({
            index: idx,
            role: msg.role,
            contentType: typeof msg.content,
            contentLength: Array.isArray(msg.content) ? msg.content.length : 'not array',
            firstBlock: Array.isArray(msg.content) && msg.content[0] ? {
                type: msg.content[0].type,
                hasText: 'text' in msg.content[0],
                textType: typeof msg.content[0].text
            } : null
        }));
        */

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'anthropic', 
            model: this.model,
            url: this.apiUrl
        });

        // Check request size before sending
        this.checkRequestSize(requestBody);

        // Log the full request
        console.log(`[ANTHROPIC SEND] (${mode}, subchat: ${chat?.isSubChat || false}):`, requestBody);

        let response;
        try {
            response = await fetch(this.apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'anthropic-version': '2023-06-01',
                    'anthropic-beta': 'prompt-caching-2024-07-31' // Enable caching
                },
                body: JSON.stringify(requestBody)
            });
        } catch (error) {
            this.log('error', `Failed to send request: ${error.message}`, { 
                provider: 'anthropic',
                error: error.toString(),
                url: this.apiUrl
            });
            if (error.name === 'TypeError' && error.message.includes('Failed to fetch')) {
                const connectionError = `Connection Error: Cannot reach Anthropic API at ${this.apiUrl}. Please ensure the proxy server is running.`;
                console.error('[AnthropicProvider] Connection error:', connectionError, '\nOriginal error:', error);
                throw new Error(connectionError);
            }
            throw error;
        }

        if (!response.ok) {
            const error = await response.json();
            this.log('error', `API error response: ${JSON.stringify(error)}`, { 
                provider: 'anthropic',
                status: response.status,
                statusText: response.statusText
            });
            
            // Special handling for rate limit errors (429)
            if (response.status === 429) {
                const retryAfter = response.headers.get('retry-after') || response.headers.get('x-ratelimit-reset-after');
                const baseMessage = error.error?.message || 'Rate limit exceeded';
                let rateLimitMessage = `Rate limit exceeded: ${baseMessage}`;
                
                if (retryAfter) {
                    rateLimitMessage += ` (retry after ${retryAfter}s)`;
                }
                
                const apiError = `Anthropic API error: ${rateLimitMessage} (429)`;
                console.error('[AnthropicProvider] Rate limit error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
                throw new Error(apiError);
            }
            
            const apiError = `Anthropic API error: ${error.error?.message || response.statusText}`;
            console.error('[AnthropicProvider] API error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
            throw new Error(apiError);
        }

        /** @type {AnthropicResponse} */
        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'anthropic' });
        
        // Log the processed response
        const processedResponse = { 
            content: data.content,
            toolCalls: [],
            usage: data.usage ? {
                promptTokens: data.usage.input_tokens,
                completionTokens: data.usage.output_tokens,
                totalTokens: (data.usage.input_tokens || 0) + (data.usage.output_tokens || 0),
                cacheCreationInputTokens: data.usage.cache_creation_input_tokens,
                cacheReadInputTokens: data.usage.cache_read_input_tokens
            } : null
        };
        
        console.log(`[ANTHROPIC RECEIVED] (${mode}, subchat: ${chat?.isSubChat || false}):`, data);
        
        return processedResponse;
    }

    convertMessagesWithCaching(messages, mode, cachePosition = null) {
        // Convert messages WITHOUT adding cache control yet
        const converted = [];
        // let lastRole = null; // Removed - variable was never read
        
        let summaryContent = null;
        
        for (let i = 0; i < messages.length; i++) {
            const msg = messages[i];
            
            if (msg.role === 'system') {
                // System messages are handled separately in sendMessage
                continue;
            }
            
            // Capture summary content to add to system prompt later
            if (msg.role === 'summary') {
                summaryContent = msg.content;
                continue;  // Don't add summary as a regular message
            }
            
            // Handle internal message format
            // Use same role detection logic as validation
            const msgRole = msg.role;
            
            if (msgRole === 'user') {
                // Check if this is a user message with tool results
                if (Array.isArray(msg.content) && msg.content.length > 0) {
                    // Check if the first element looks like a tool result
                    const firstItem = msg.content[0];
                    if (firstItem && firstItem.type === 'tool_result') {
                        // This is already properly formatted tool results
                        converted.push({
                            role: 'user',
                            content: msg.content
                        });
                        // lastRole = 'user';
                        continue;
                    } else if (typeof firstItem === 'string' && firstItem.includes('tool_result')) {
                        // This might be a stringified tool results array
                        try {
                            const parsedContent = JSON.parse(firstItem);
                            if (Array.isArray(parsedContent) && parsedContent[0] && parsedContent[0].type === 'tool_result') {
                                converted.push({
                                    role: 'user',
                                    content: parsedContent
                                });
                                // lastRole = 'user';
                                continue;
                            }
                        } catch {
                            // Not JSON, continue with normal processing
                        }
                    }
                }
                
                // User messages - handle both string content and already-formatted content
                let textContent;
                if (typeof msg.content === 'string') {
                    textContent = msg.content;
                } else if (Array.isArray(msg.content)) {
                    // Content is an array - extract text from first text block
                    const textBlock = msg.content.find(block => block.type === 'text');
                    if (textBlock && textBlock.text) {
                        textContent = textBlock.text;
                    } else if (msg.content[0] && typeof msg.content[0] === 'string') {
                        // Array of strings
                        textContent = msg.content[0];
                    } else {
                        console.warn('Unknown user message array format, using fallback:', msg.content);
                        textContent = JSON.stringify(msg.content);
                    }
                } else if (msg.content && typeof msg.content === 'object') {
                    // Single object - extract text
                    if (msg.content.text) {
                        textContent = msg.content.text;
                    } else {
                        console.warn('Unknown user message object format, using fallback:', msg.content);
                        textContent = JSON.stringify(msg.content);
                    }
                } else {
                    textContent = '';
                }
                
                // Ensure textContent is a valid string
                if (textContent === null || textContent === undefined) {
                    textContent = '';
                }
                
                converted.push({
                    role: 'user',
                    content: [{ type: 'text', text: this.addDateTimePrefix(String(textContent), msg.timestamp) }]
                });
                // lastRole = 'user';
            } else if (msgRole === 'assistant') {
                // Convert assistant message to Anthropic format
                let content;
                
                if (msg.content) {
                    if (typeof msg.content === 'string') {
                        // Legacy string format - convert to array
                        content = [{ type: 'text', text: msg.content }];
                    } else if (Array.isArray(msg.content)) {
                        // Content is already an array - validate and fix text blocks
                        // This preserves tool_use blocks that were filtered by MessageOptimizer
                        content = msg.content.map(block => {
                            if (block.type === 'text') {
                                let textValue = block.text;
                                
                                // Handle nested array structure (shouldn't happen, but defensive coding)
                                if (Array.isArray(textValue)) {
                                    console.warn('[AnthropicProvider] Found nested array in text block, flattening:', textValue);
                                    // Extract text from nested structure
                                    const textBlocks = textValue.filter(item => item && item.type === 'text');
                                    textValue = textBlocks.map(item => item.text || '').join('\n\n').trim();
                                }
                                
                                // Ensure text property is a valid string
                                return {
                                    type: 'text',
                                    text: String(textValue || '')
                                };
                            }
                            // Return other block types (like tool_use) as-is
                            return block;
                        });
                    } else {
                        // Unknown format - convert to text
                        content = [{ type: 'text', text: String(msg.content) }];
                    }
                } else {
                    content = [];
                }
                
                
                if (content.length > 0) {
                    converted.push({
                        role: 'assistant',
                        content
                    });
                    // lastRole = 'assistant'; // Not needed - not used after this
                }
            } else if (msgRole === 'tool-results') {
                // Convert tool results to Anthropic format
                // Tool filtering now handled by optimizer - include all tools sent to provider
                const content = [];
                // STRICT: Only accept toolResults property
                const toolResults = msg.toolResults || [];
                
                for (const result of toolResults) {
                    // Tool results for Anthropic need to be tool_result blocks
                    const formattedResult = this.formatToolResultForAnthropic(
                        result.toolCallId || result.id,
                        result.result,
                        result.toolName || result.name
                    );
                    content.push(formattedResult);
                }
                
                if (content.length > 0) {
                    // Tool results must be in user messages
                    converted.push({
                        role: 'user',
                        content
                    });
                    // lastRole = 'user'; // Not needed - last assignment
                }
            }
        }
        
        // Apply cache control based on mode and cachePosition parameter
        // Note: System prompt cache control is handled separately above
        if (mode === 'cached') {
            // For 'cached' mode, apply cache control to the strategy-determined position
            if (cachePosition !== null && cachePosition >= 0 && cachePosition < converted.length) {
                // Apply cache control to specific position
                const targetMsg = converted[cachePosition];
                if (targetMsg && Array.isArray(targetMsg.content) && targetMsg.content.length > 0) {
                    // Add cache control to last content block of the specified message
                    targetMsg.content[targetMsg.content.length - 1].cache_control = { type: 'ephemeral' };
                }
            } else {
                // Default behavior - find the absolute last content block across all messages
                let lastContentBlock = null;
                
                // Iterate backwards through messages to find the last content block
                for (let i = converted.length - 1; i >= 0; i--) {
                    const msg = converted[i];
                    if (Array.isArray(msg.content) && msg.content.length > 0) {
                        // Found a message with content, get its last block
                        lastContentBlock = msg.content[msg.content.length - 1];
                        break;
                    }
                }
                
                // Add cache_control to only the very last content block
                if (lastContentBlock) {
                    lastContentBlock.cache_control = { type: 'ephemeral' };
                }
            }
        }
        // For 'system' mode: only system prompt is cached (handled above)
        // For 'all-off' mode: no cache control applied
        
        return { converted, summaryContent };
    }

    convertMessages(messages, _mode) {
        // Convert messages for Anthropic format
        const converted = [];
        
        // Extract system prompt and handle summary (Anthropic uses separate system parameter)
        const { systemPrompt, messages: remainingMessages } = MessageConversionUtils.extractSystemAndSummary(messages);
        
        // Process remaining messages
        for (const msg of remainingMessages) {
            // Use same role detection logic as validation
            const msgRole = msg.role;
            
            if (msgRole === 'user') {
                // Convert user message to Anthropic format with content blocks
                converted.push({
                    role: 'user',
                    content: [{ type: 'text', text: this.addDateTimePrefix(msg.content, msg.timestamp) }]
                });
            } else if (msgRole === 'assistant') {
                // Convert assistant message to Anthropic format
                let content;
                
                if (msg.content) {
                    if (typeof msg.content === 'string') {
                        // Legacy string format - convert to array
                        content = [{ type: 'text', text: msg.content }];
                    } else if (Array.isArray(msg.content)) {
                        // Content is already an array - validate and fix text blocks
                        // This preserves tool_use blocks that were included
                        content = msg.content.map(block => {
                            if (block.type === 'text') {
                                let textValue = block.text;
                                
                                // Handle nested array structure (shouldn't happen, but defensive coding)
                                if (Array.isArray(textValue)) {
                                    console.warn('[AnthropicProvider] Found nested array in text block, flattening:', textValue);
                                    // Extract text from nested structure
                                    const textBlocks = textValue.filter(item => item && item.type === 'text');
                                    textValue = textBlocks.map(item => item.text || '').join('\n\n').trim();
                                }
                                
                                // Ensure text property is a valid string
                                return {
                                    type: 'text',
                                    text: String(textValue || '')
                                };
                            }
                            // Return other block types (like tool_use) as-is
                            return block;
                        });
                    } else {
                        // Unknown format - convert to text
                        content = [{ type: 'text', text: String(msg.content) }];
                    }
                } else {
                    content = [];
                }
                
                if (content.length > 0) {
                    converted.push({
                        role: 'assistant',
                        content
                    });
                }
            } else if (msgRole === 'tool-results') {
                // Convert tool results to Anthropic format (user message with tool_result blocks)
                const content = [];
                
                // STRICT: Only accept toolResults property
                if (msg.toolResults && Array.isArray(msg.toolResults)) {
                    for (const toolResult of msg.toolResults) {
                        content.push(this.formatToolResultForAnthropic(
                            toolResult.toolCallId,
                            toolResult.result,
                            toolResult.toolName
                        ));
                    }
                }
                
                if (content.length > 0) {
                    // Tool results must be in user messages
                    converted.push({
                        role: 'user',
                        content
                    });
                }
            } else {
                console.warn('[Anthropic] Unexpected message role:', msgRole);
            }
        }
        
        // Return both messages and system prompt (needed by sendMessage)
        return { messages: converted, system: systemPrompt };
    }


    formatToolResultForAnthropic(toolCallId, result, _toolName) {
        // Format MCP tool results for Anthropic's tool_result blocks
        const formatted = MessageConversionUtils.formatMCPToolResult(result);
        let content = [];
        
        if (formatted.type === 'text') {
            content = [{ type: 'text', text: formatted.content }];
        } else if (formatted.type === 'multi') {
            // Handle multiple content items from MCP
            for (const item of formatted.items) {
                if (item.type === 'text') {
                    content.push({ type: 'text', text: item.content });
                } else if (item.type === 'image') {
                    // Anthropic supports images in tool results
                    content.push({
                        type: 'image',
                        source: {
                            type: 'base64',
                            media_type: item.mimeType,
                            data: item.data
                        }
                    });
                } else if (item.type === 'resource') {
                    // Convert resource to text
                    content.push({
                        type: 'text',
                        text: `[Resource: ${item.uri}]\n${item.text || ''}`
                    });
                }
            }
        } else if (formatted.type === 'json') {
            content = [{ type: 'text', text: JSON.stringify(formatted.content, null, 2) }];
        } else {
            // Fallback
            content = [{ type: 'text', text: JSON.stringify(result) }];
        }
        
        return {
            type: 'tool_result',
            tool_use_id: toolCallId,
            content
        };
    }
}

/**
 * Google Gemini Provider
 */
class GoogleProvider extends LLMProvider {
    constructor(proxyUrl, model = 'gemini-pro') {
        super(proxyUrl);
        this.model = model;
        this.type = 'google';
    }

    get apiUrl() {
        return `${this.proxyUrl}/proxy/google/v1beta/models/${this.model}/generateContent`;
    }

    /**
     * Send messages to Google Gemini API
     * @param {Array} messages - Array of message objects
     * @param {Array} tools - Array of available tools
     * @param {number} temperature - Temperature for response generation
     * @param {string} mode - Tool inclusion mode
     * @param {number|null} _cachePosition - Cache position (unused for Google)
     * @returns {Promise<LLMResponse>}
     */
    async sendMessage(messages, tools, temperature, mode, _cachePosition = null, chat = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages to Gemini format
        const { contents, systemInstruction } = this.convertMessages(messages, mode);
        
        // Convert tools to Gemini format
        const functionDeclarations = tools.map(tool => {
            const injectedTool = this.injectToolMetadata(tool, chat);
            return {
                name: injectedTool.name,
                description: injectedTool.description,
                parameters: this.cleanSchemaForGoogle(injectedTool.inputSchema || {})
            };
        });

        // Order fields consistently: tools → systemInstruction (system) → contents (messages) → generationConfig
        const requestBody = {};
        
        // Add tools first if present
        if (functionDeclarations.length > 0) {
            requestBody.tools = [{
                function_declarations: functionDeclarations
            }];
        }
        
        // Add system instruction if present
        if (systemInstruction) {
            requestBody.systemInstruction = {
                parts: [{ text: systemInstruction }]
            };
        }
        
        // Add messages
        requestBody.contents = contents;
        
        // Add generation config (includes model info implicitly via this.model in the URL)
        requestBody.generationConfig = {
            temperature,
            maxOutputTokens: 4096
        };

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'google', 
            model: this.model,
            url: this.apiUrl
        });

        // Check request size before sending
        this.checkRequestSize(requestBody);

        // Log the full request
        console.log(`[GOOGLE SEND] (${mode}, subchat: ${chat?.isSubChat || false}):`, requestBody);

        let response;
        try {
            response = await fetch(this.apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(requestBody)
            });
        } catch (error) {
            this.log('error', `Failed to send request: ${error.message}`, { 
                provider: 'google',
                error: error.toString(),
                url: this.apiUrl
            });
            if (error.name === 'TypeError' && error.message.includes('Failed to fetch')) {
                const connectionError = `Connection Error: Cannot reach Google AI API at ${this.apiUrl}. Please ensure the proxy server is running.`;
                console.error('[GoogleProvider] Connection error:', connectionError, '\nOriginal error:', error);
                throw new Error(connectionError);
            }
            throw error;
        }

        if (!response.ok) {
            const error = await response.json();
            this.log('error', `API error response: ${JSON.stringify(error)}`, { 
                provider: 'google',
                status: response.status,
                statusText: response.statusText
            });
            
            // Special handling for rate limit errors (429)
            if (response.status === 429) {
                const retryAfter = response.headers.get('retry-after') || response.headers.get('x-ratelimit-reset-after');
                const baseMessage = error.error?.message || 'Rate limit exceeded';
                let rateLimitMessage = `Rate limit exceeded: ${baseMessage}`;
                
                if (retryAfter) {
                    rateLimitMessage += ` (retry after ${retryAfter}s)`;
                }
                
                const apiError = `Google API error: ${rateLimitMessage} (429)`;
                console.error('[GoogleProvider] Rate limit error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
                throw new Error(apiError);
            }
            
            const apiError = `Google API error: ${error.error?.message || response.statusText}`;
            console.error('[GoogleProvider] API error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
            throw new Error(apiError);
        }

        /** @type {GoogleResponse} */
        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'google' });
        
        // Log the full response
        console.log(`[GOOGLE RECEIVED] (${mode}, subchat: ${chat?.isSubChat || false}):`, data);
        const candidate = data.candidates[0];
        
        // Check finish reason for potential issues
        if (candidate.finishReason === 'MAX_TOKENS') {
            // Handle token limit error
            const errorMsg = 'Google Gemini response was truncated due to token limit. The model\'s context window is full. Consider starting a new conversation or summarizing the current one.';
            console.error('[GoogleProvider] MAX_TOKENS error:', errorMsg);
            this.log('error', errorMsg, { provider: 'google', code: 'MAX_TOKENS', finishReason: candidate.finishReason });
            const error = new Error(errorMsg);
            error.code = 'MAX_TOKENS';
            throw error;
        } else if (candidate.finishReason === 'SAFETY') {
            // Handle safety filter
            const errorMsg = 'Google Gemini blocked the response due to safety filters.';
            console.error('[GoogleProvider] SAFETY error:', errorMsg);
            this.log('error', errorMsg, { provider: 'google', code: 'SAFETY', finishReason: candidate.finishReason });
            const error = new Error(errorMsg);
            error.code = 'SAFETY';
            throw error;
        } else if (candidate.finishReason === 'MALFORMED_FUNCTION_CALL') {
            // Handle malformed function call - return error message instead of throwing
            console.warn('[GoogleProvider] MALFORMED_FUNCTION_CALL:', 'Google Gemini returned a malformed function call');
            this.log('warn', 'Google Gemini returned a malformed function call', { provider: 'google', finishReason: candidate.finishReason });
            return {
                content: 'I apologize, but I encountered an error while trying to execute tools to answer your question. This appears to be a temporary issue with the function calling system. Please try rephrasing your question or asking it again.',
                toolCalls: [],
                usage: data.usageMetadata ? {
                    promptTokens: data.usageMetadata.promptTokenCount || 0,
                    completionTokens: data.usageMetadata.candidatesTokenCount || 0,
                    totalTokens: data.usageMetadata.totalTokenCount || 0
                } : { promptTokens: 0, completionTokens: 0, totalTokens: 0 }
            };
        } else if (candidate.finishReason && candidate.finishReason !== 'STOP') {
            // Handle other unexpected finish reasons
            const errorMsg = `Google Gemini response ended unexpectedly: ${candidate.finishReason}`;
            console.error('[GoogleProvider] Unexpected finish reason:', errorMsg);
            this.log('error', errorMsg, { provider: 'google', code: candidate.finishReason, finishReason: candidate.finishReason });
            const error = new Error(errorMsg);
            error.code = candidate.finishReason;
            throw error;
        }
        
        // Convert Google response to unified content array format
        const contentArray = [];
        let textContent = '';
        
        // Ensure parts array exists
        if (candidate.content && candidate.content.parts) {
            for (const part of candidate.content.parts) {
                if (part.text) {
                    textContent += part.text;
                } else if (part.functionCall) {
                    // If we have accumulated text, add it first
                    if (textContent) {
                        contentArray.push({ type: 'text', text: textContent });
                        textContent = '';
                    }
                    // Add tool use block
                    contentArray.push({
                        type: 'tool_use',
                        id: this.generateId(),
                        name: part.functionCall.name,
                        input: part.functionCall.args
                    });
                }
            }
        }
        
        // Add any remaining text content
        if (textContent) {
            contentArray.push({ type: 'text', text: textContent });
        }
        
        // Google returns token counts in usageMetadata
        const usage = data.usageMetadata ? {
            promptTokens: data.usageMetadata.promptTokenCount || 0,
            completionTokens: data.usageMetadata.candidatesTokenCount || 0,
            totalTokens: data.usageMetadata.totalTokenCount || 0,
            // Additional Gemini-specific token counts
            cachedContentTokenCount: data.usageMetadata.cachedContentTokenCount || 0,
            thoughtsTokenCount: data.usageMetadata.thoughtsTokenCount || 0
        } : null;
        
        return { 
            content: contentArray,
            toolCalls: [],
            usage 
        };
    }

    convertMessages(messages, _mode) {
        // Convert messages for Google format
        /*
        messages.map((m, i) => ({
            index: i,
            role: m.role,
            hasToolCalls: m.toolCalls && m.toolCalls.length > 0,
            toolCalls: m.toolCalls,
            content: m.content ? m.content.substring(0, 50) + '...' : ''
        }));
        */
        
        // Pre-scan to find ALL tool calls and responses
        const allToolCalls = new Map(); // Map of tool name to array of indices
        const allToolResponses = new Map(); // Map of tool name to array of indices
        
        for (let i = 0; i < messages.length; i++) {
            const msg = messages[i];
            // Use same role detection logic as validation
            const msgRole = msg.role;
            
            // Check for tool calls in assistant messages
            const toolCalls = extractToolCallsFromContent(msg.content);
            if (msgRole === 'assistant' && toolCalls.length > 0) {
                for (const tc of toolCalls) {
                    if (!allToolCalls.has(tc.name)) {
                        allToolCalls.set(tc.name, []);
                    }
                    allToolCalls.get(tc.name).push(i);
                }
            } else if (msgRole === 'tool-results') {
                // Handle internal tool-results format
                // STRICT: Only accept toolResults property
                const toolResults = msg.toolResults || [];
                for (const result of toolResults) {
                    const toolName = result.name || result.toolName;
                    if (!allToolResponses.has(toolName)) {
                        allToolResponses.set(toolName, []);
                    }
                    allToolResponses.get(toolName).push(i);
                }
            }
        }
        
        // Check for orphaned responses
        for (const [toolName, responseIndices] of allToolResponses) {
            const callIndices = allToolCalls.get(toolName) || [];
            
            for (const responseIndex of responseIndices) {
                // Find if there's a call before this response
                const hasCallBefore = callIndices.some(callIndex => callIndex < responseIndex);
                
                if (!hasCallBefore) {
                    console.error('[Google Provider] Found orphaned tool response:', {
                        toolName,
                        responseIndex,
                        callIndices,
                        responseIndices,
                        messages: messages.map((m, i) => {
                            const tools = extractToolCallsFromContent(m.content);
                            return {
                                index: i,
                                role: m.role,
                                hasToolCalls: tools.length > 0,
                                toolCallNames: tools.map(tc => tc.name)
                            };
                        })
                    });
                    throw new Error(
                        `Google API Error: Tool "${toolName}" response found without a preceding function call. ` +
                        `This may be due to tool inclusion settings filtering out the assistant's tool calls. ` +
                        `To fix this, either: 1) Edit the message that should have called this tool, or ` +
                        `2) Change tool inclusion mode to ensure tool calls are included when their responses exist.`
                    );
                }
            }
        }
        
        const contents = [];
        let pendingToolResponses = [];
        let lastAssistantHadFunctionCalls = false;
        
        for (let i = 0; i < messages.length; i++) {
            const msg = messages[i];
            // Use same role detection logic as validation
            const msgRole = msg.role;
            
            if (msgRole === 'system') {
                // Prepend system message to first user message
                continue;
            }
            
            if (msgRole === 'tool-results') {
                // STRICT: Only accept toolResults property
                const toolResults = msg.toolResults || [];
                // Process tool results
                
                // Tool filtering now handled by optimizer - include all tools sent to provider
                
                // Check if these tool responses have corresponding function calls
                if (!lastAssistantHadFunctionCalls) {
                    // This is an orphaned tool response - stop with an error
                    console.error('[Google Provider] Orphaned tool responses detected');
                    
                    throw new Error(
                        `Google API Error: Orphaned tool responses detected. ` +
                        `Tool responses have no corresponding function calls from the assistant. ` +
                        `This would cause an infinite loop. Please check the conversation flow.`
                    );
                }
                
                // Valid tool responses - add to pending responses
                for (const result of toolResults) {
                    pendingToolResponses.push({
                        functionResponse: {
                            name: result.name || result.toolName,
                            response: {
                                content: this.formatToolResultContent(result.result)
                            }
                        }
                    });
                }
                continue;
            }
            
            // Reset the function call tracking when we encounter a new assistant message
            if (msgRole === 'assistant') {
                const toolCalls = extractToolCallsFromContent(msg.content);
                lastAssistantHadFunctionCalls = toolCalls.length > 0;
                // Track if assistant message has function calls
            }
            
            // If we have pending tool responses and this is not a tool message, 
            // add them as a user message first
            if (pendingToolResponses.length > 0) {
                contents.push({
                    role: 'user',
                    parts: [...pendingToolResponses]
                });
                pendingToolResponses = [];
            }
            
            const parts = [];
            
            if (msgRole === 'user') {
                // User messages
                let textContent = '';
                
                // Handle both string and array content
                if (typeof msg.content === 'string') {
                    textContent = msg.content;
                } else if (Array.isArray(msg.content)) {
                    // Extract text from content array (in case of tool results in user message)
                    const textBlocks = msg.content.filter(block => block.type === 'text');
                    textContent = textBlocks.map(block => block.text || '').join('\n\n').trim();
                }
                
                if (textContent || textContent === '') {
                    parts.push({ text: this.addDateTimePrefix(textContent, msg.timestamp) });
                }
            } else if (msgRole === 'assistant') {
                // Assistant messages - include text and optionally tool calls
                let textContent = '';
                
                // Extract text content from array if needed
                if (typeof msg.content === 'string') {
                    textContent = msg.content;
                } else if (Array.isArray(msg.content)) {
                    // Extract text from content array
                    const textBlocks = msg.content.filter(block => block.type === 'text');
                    textContent = textBlocks.map(block => block.text || '').join('\n\n').trim();
                }
                
                // Add text content if present
                if (textContent) {
                    parts.push({ text: textContent });
                }
                
                // Extract and add tool calls
                const toolCalls = extractToolCallsFromContent(msg.content);
                if (toolCalls.length > 0) {
                    for (const tc of toolCalls) {
                        // Use destructuring to avoid direct 'arguments' reference
                        const { arguments: tcArgs } = tc || {};
                        parts.push({
                            functionCall: {
                                name: tc.name,
                                args: tcArgs
                            }
                        });
                    }
                }
            }
            
            if (parts.length > 0) {
                contents.push({
                    role: msgRole === 'assistant' ? 'model' : 'user',
                    parts
                });
            }
        }
        
        // Handle any remaining tool responses at the end
        if (pendingToolResponses.length > 0) {
            if (!lastAssistantHadFunctionCalls) {
                console.error('[Google Provider] Orphaned tool responses at end of conversation');
                throw new Error(
                    `Google API Error: Tool responses found at end of conversation without corresponding function calls. ` +
                    `This would cause an infinite loop. Please check the conversation flow.`
                );
            }
            contents.push({
                role: 'user',
                parts: [...pendingToolResponses]
            });
        }
        
        // Extract system message for separate handling
        const systemMsg = messages.find(m => m.role === 'system');
        let systemInstruction = null;
        
        if (systemMsg) {
            systemInstruction = systemMsg.content;
        }
        
        // Also handle summary messages that should be integrated into system prompt
        const summaryMsg = messages.find(m => m.role === 'summary');
        if (summaryMsg) {
            systemInstruction = MessageConversionUtils.integrateSummaryIntoSystemPrompt(
                systemInstruction || '', 
                summaryMsg.content
            );
        }
        
        return { contents, systemInstruction };
    }


    formatToolResultContent(result) {
        // Handle different types of results for Google format
        if (typeof result === 'string') {
            return result;
        } else if (Array.isArray(result)) {
            // For arrays, stringify each element if needed and join
            return result.map(item => 
                typeof item === 'string' ? item : JSON.stringify(item)
            ).join('\n\n');
        } else {
            return JSON.stringify(result);
        }
    }

    generateId() {
        return 'call_' + Math.random().toString(36).substring(2, 11);
    }

    /**
     * Clean MCP schema for Google's function calling format
     * Removes additionalProperties and other unsupported fields recursively
     */
    cleanSchemaForGoogle(schema) {
        if (!schema || typeof schema !== 'object') {
            return schema;
        }

        // Create a deep copy to avoid modifying the original
        const cleaned = JSON.parse(JSON.stringify(schema));
        
        // Recursively clean the schema
        this.removeUnsupportedFields(cleaned);
        
        return cleaned;
    }

    /**
     * Recursively remove fields that Google's function calling doesn't support
     */
    removeUnsupportedFields(obj) {
        if (!obj || typeof obj !== 'object') {
            return;
        }

        // Remove additionalProperties at any level
        if ('additionalProperties' in obj) {
            delete obj.additionalProperties;
        }

        // Remove other unsupported fields that might cause issues
        const unsupportedFields = [
            '$schema',
            'title',
            'examples',
            'default',
            'const'
        ];
        
        unsupportedFields.forEach(field => {
            if (field in obj) {
                delete obj[field];
            }
        });

        // Recursively process nested objects and arrays
        Object.values(obj).forEach(value => {
            if (typeof value === 'object' && value !== null) {
                if (Array.isArray(value)) {
                    value.forEach(item => this.removeUnsupportedFields(item));
                } else {
                    this.removeUnsupportedFields(value);
                }
            }
        });
    }
}

/**
 * Ollama Provider
 */
class OllamaProvider extends LLMProvider {
    constructor(proxyUrl, model = 'llama3.3:latest') {
        super(proxyUrl);
        this.model = model;
        this.type = 'ollama';
    }

    get apiUrl() {
        return `${this.proxyUrl}/proxy/ollama/api/chat`;
    }

    /**
     * Send messages to Ollama API
     * @param {Array} messages - Array of message objects
     * @param {Array} tools - Array of available tools
     * @param {number} temperature - Temperature for response generation
     * @param {string} mode - Tool inclusion mode (unused for Ollama)
     * @param {number|null} _cachePosition - Cache position (unused for Ollama)
     * @returns {Promise<LLMResponse>}
     */
    async sendMessage(messages, tools, temperature, mode, _cachePosition = null, chat = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages to Ollama format
        const ollamaMessages = this.convertMessages(messages);
        
        // Convert tools to Ollama format
        const ollamaTools = tools.map(tool => {
            const injectedTool = this.injectToolMetadata(tool, chat);
            return {
                type: 'function',
                function: {
                    name: injectedTool.name,
                    description: injectedTool.description,
                    parameters: injectedTool.inputSchema || {}
                }
            };
        });

        // Build request body
        const requestBody = {
            model: this.model,
            messages: ollamaMessages,
            temperature,
            stream: false,
            options: {
                num_predict: 4096  // Similar to max_tokens
            }
        };
        
        // Try first with tools if available
        let useToolFallback = false;
        if (ollamaTools.length > 0) {
            requestBody.tools = ollamaTools;
        }

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'ollama', 
            model: this.model,
            url: this.apiUrl
        });

        // Check request size before sending
        this.checkRequestSize(requestBody);

        // Log the full request
        console.log(`[OLLAMA SEND] (${mode}, subchat: ${chat?.isSubChat || false}):`, requestBody);

        let response;
        try {
            response = await fetch(this.apiUrl, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(requestBody)
            });
        } catch (error) {
            this.log('error', `Failed to send request: ${error.message}`, { 
                provider: 'ollama',
                error: error.toString(),
                url: this.apiUrl
            });
            if (error.name === 'TypeError' && error.message.includes('Failed to fetch')) {
                const connectionError = `Connection Error: Cannot reach Ollama API at ${this.apiUrl}. Please ensure Ollama is running.`;
                console.error('[OllamaProvider] Connection error:', connectionError, '\nOriginal error:', error);
                throw new Error(connectionError);
            }
            throw error;
        }

        if (!response.ok) {
            let error;
            try {
                error = await response.json();
            } catch {
                error = { error: response.statusText };
            }
            
            // Check if the error is about tool support
            const errorMessage = error.error || response.statusText;
            if (errorMessage.includes('does not support tools') && ollamaTools.length > 0) {
                console.log('[OllamaProvider] Model does not support tools, falling back to system prompt injection');
                
                // Retry without tools, injecting them into system prompt instead
                useToolFallback = true;
                delete requestBody.tools;
                
                // Inject tools into system prompt
                const toolsPrompt = this.formatToolsForSystemPrompt(ollamaTools);
                const systemMessageIndex = ollamaMessages.findIndex(msg => msg.role === 'system');
                if (systemMessageIndex >= 0) {
                    ollamaMessages[systemMessageIndex].content += '\n\n' + toolsPrompt;
                } else {
                    // Add system message if it doesn't exist
                    ollamaMessages.unshift({
                        role: 'system',
                        content: toolsPrompt
                    });
                }
                requestBody.messages = ollamaMessages;
                
                // Retry the request
                try {
                    response = await fetch(this.apiUrl, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/json'
                        },
                        body: JSON.stringify(requestBody)
                    });
                    
                    if (!response.ok) {
                        let retryError;
                        try {
                            retryError = await response.json();
                        } catch {
                            retryError = { error: response.statusText };
                        }
                        const retryApiError = `Ollama API error (fallback): ${retryError.error || response.statusText}`;
                        console.error('[OllamaProvider] API error on retry:', retryApiError);
                        throw new Error(retryApiError);
                    }
                } catch (retryError) {
                    console.error('[OllamaProvider] Failed to retry with tool fallback:', retryError);
                    throw retryError;
                }
            } else {
                // Not a tool support error, throw as is
                this.log('error', `API error response: ${JSON.stringify(error)}`, { 
                    provider: 'ollama',
                    status: response.status,
                    statusText: response.statusText
                });
                
                const apiError = `Ollama API error: ${errorMessage}`;
                console.error('[OllamaProvider] API error:', apiError, '\nStatus:', response.status, '\nResponse:', error);
                throw new Error(apiError);
            }
        }

        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'ollama' });
        
        // Log the full response
        console.log(`[OLLAMA RECEIVED] (${mode}, subchat: ${chat?.isSubChat || false}):`, data);
        
        // Debug log for tool parsing
        if (data.message && data.message.content) {
            console.log('[OLLAMA DEBUG] Message content:', data.message.content);
            console.log('[OLLAMA DEBUG] Content includes python_tag?', data.message.content.includes('python_tag'));
            console.log('[OLLAMA DEBUG] Content includes <|python_tag|>?', data.message.content.includes('<|python_tag|>'));
        }
        
        // Convert Ollama response to unified format
        const contentArray = [];
        
        // If we used tool fallback, parse tools from text response
        if (useToolFallback && data.message && data.message.content) {
            const parsedContent = this.parseToolCallsFromText(data.message.content, tools);
            contentArray.push(...parsedContent);
        } else {
            // Check if content contains tool calls in special formats
            let contentHasToolCalls = false;
            if (data.message && data.message.content) {
                const content = data.message.content;
                // Check for various tool call indicators
                if (content.includes('python_tag') || 
                    content.includes('"type": "function"') ||
                    (content.includes('```json') && content.includes('"tool_use"'))) {
                    console.log('[OllamaProvider] Detected tool calls in content, parsing...', content);
                    const parsedContent = this.parseToolCallsFromText(content, tools);
                    contentArray.push(...parsedContent);
                    contentHasToolCalls = true;
                }
            }
            
            // Add text content if present and we didn't parse tool calls from it
            if (data.message && data.message.content && !contentHasToolCalls) {
                contentArray.push({ type: 'text', text: data.message.content });
            }
            
            // Handle tool calls if present in the proper field
            if (data.message && data.message.tool_calls) {
                for (const toolCall of data.message.tool_calls) {
                    contentArray.push({
                        type: 'tool_use',
                        id: toolCall.id || this.generateId(),
                        name: toolCall.function.name,
                        input: toolCall.function.arguments || {}
                    });
                }
            }
        }
        
        // Calculate token usage from response metadata
        const usage = {
            promptTokens: data.prompt_eval_count || 0,
            completionTokens: data.eval_count || 0,
            totalTokens: (data.prompt_eval_count || 0) + (data.eval_count || 0),
            cacheReadInputTokens: 0,  // Ollama doesn't support caching
            cacheCreationInputTokens: 0
        };
        
        return {
            content: contentArray,
            toolCalls: [],
            usage
        };
    }

    /**
     * Convert messages from internal format to Ollama format
     * @param {Array} messages - Internal format messages
     * @returns {Array} Ollama format messages
     */
    convertMessages(messages) {
        const converted = [];
        
        for (const msg of messages) {
            const msgRole = msg.role;
            
            if (msgRole === 'system') {
                converted.push({
                    role: 'system',
                    content: msg.content
                });
            } else if (msgRole === 'user') {
                // Add datetime prefix to user messages
                const content = this.addDateTimePrefix(msg.content, msg.timestamp);
                converted.push({
                    role: 'user',
                    content
                });
            } else if (msgRole === 'assistant') {
                // Convert assistant message - check for tool calls
                const assistantMsg = { role: 'assistant' };
                
                if (Array.isArray(msg.content)) {
                    // Extract text and tool calls from content array
                    let textContent = '';
                    const toolCalls = [];
                    
                    for (const block of msg.content) {
                        if (block.type === 'text') {
                            textContent += block.text;
                        } else if (block.type === 'tool_use') {
                            toolCalls.push({
                                id: block.id,
                                type: 'function',
                                function: {
                                    name: block.name,
                                    arguments: block.input
                                }
                            });
                        }
                    }
                    
                    if (textContent) {
                        assistantMsg.content = textContent;
                    }
                    if (toolCalls.length > 0) {
                        assistantMsg.tool_calls = toolCalls;
                    }
                } else {
                    assistantMsg.content = msg.content || '';
                }
                
                converted.push(assistantMsg);
            } else if (msgRole === 'tool-results') {
                // Convert tool results to Ollama format
                if (msg.toolResults && Array.isArray(msg.toolResults)) {
                    for (const toolResult of msg.toolResults) {
                        converted.push({
                            role: 'tool',
                            content: this.formatToolResult(toolResult.result),
                            tool_call_id: toolResult.toolCallId
                        });
                    }
                }
            }
        }
        
        return converted;
    }
    
    /**
     * Format tool result for Ollama
     * @param {any} result - Tool result
     * @returns {string} Formatted result
     */
    formatToolResult(result) {
        const formatted = MessageConversionUtils.formatMCPToolResult(result);
        
        if (formatted.type === 'text') {
            return formatted.content;
        } else if (formatted.type === 'multi') {
            // Handle multiple content items
            const parts = [];
            for (const item of formatted.items) {
                if (item.type === 'text') {
                    parts.push(item.content);
                } else if (item.type === 'image') {
                    parts.push(`[Image: ${item.mimeType}]`);
                } else if (item.type === 'resource') {
                    parts.push(`[Resource: ${item.uri}]\n${item.text || ''}`);
                }
            }
            return parts.join('\n\n');
        } else if (formatted.type === 'json') {
            return JSON.stringify(formatted.content, null, 2);
        } else {
            return JSON.stringify(result);
        }
    }
    
    /**
     * Generate a unique ID for tool calls
     * @returns {string} Unique ID
     */
    generateId() {
        return 'tool_' + Math.random().toString(36).substr(2, 9);
    }
    
    /**
     * Format tools for inclusion in system prompt
     * @param {Array} tools - Array of tool objects
     * @returns {string} Formatted tools prompt
     */
    formatToolsForSystemPrompt(tools) {
        let prompt = 'You have access to the following tools:\n\n';
        
        tools.forEach(tool => {
            prompt += `Tool: ${tool.function.name}\n`;
            prompt += `Description: ${tool.function.description}\n`;
            prompt += `Parameters: ${JSON.stringify(tool.function.parameters, null, 2)}\n\n`;
        });
        
        prompt += 'To use a tool, respond with a JSON block in the following format:\n';
        prompt += '```json\n';
        prompt += '{\n';
        prompt += '  "tool_use": {\n';
        prompt += '    "name": "tool_name",\n';
        prompt += '    "arguments": {\n';
        prompt += '      "param1": "value1",\n';
        prompt += '      "param2": "value2"\n';
        prompt += '    }\n';
        prompt += '  }\n';
        prompt += '}\n';
        prompt += '```\n\n';
        prompt += 'You can include explanatory text before or after the tool use JSON block.';
        
        return prompt;
    }
    
    /**
     * Parse tool calls from text response
     * @param {string} text - Text response that may contain tool calls
     * @param {Array} availableTools - Available tools for validation
     * @returns {Array} Array of content blocks
     */
    parseToolCallsFromText(text, availableTools) {
        const contentArray = [];
        
        // Regular expressions for different tool call formats
        const patterns = [
            // JSON blocks with tool_use format
            {
                regex: /```json\s*\n?\s*(\{[\s\S]*?"tool_use"[\s\S]*?\})\s*\n?\s*```/g,
                parser: (match) => {
                    const toolCallJson = JSON.parse(match[1]);
                    if (toolCallJson.tool_use && toolCallJson.tool_use.name) {
                        return {
                            name: toolCallJson.tool_use.name,
                            input: toolCallJson.tool_use.arguments || {}
                        };
                    }
                    return null;
                }
            },
            // Llama's python_tag format - handle multi-line JSON
            {
                regex: /<\|python_tag\|>/g,
                parser: (match, fullText, matchIndex) => {
                    // Find the JSON starting after the python_tag
                    const startIndex = matchIndex + match[0].length;
                    const textAfterTag = fullText.substring(startIndex);
                    
                    // Find the complete JSON object by counting braces
                    let braceCount = 0;
                    let inString = false;
                    let escapeNext = false;
                    let endIndex = -1;
                    let jsonStartIndex = -1;
                    
                    for (let i = 0; i < textAfterTag.length; i++) {
                        const char = textAfterTag[i];
                        
                        // Skip whitespace before JSON starts
                        if (jsonStartIndex === -1 && char === '{') {
                            jsonStartIndex = i;
                        }
                        
                        if (jsonStartIndex === -1) continue;
                        
                        if (escapeNext) {
                            escapeNext = false;
                            continue;
                        }
                        
                        if (char === '\\') {
                            escapeNext = true;
                            continue;
                        }
                        
                        if (char === '"' && !escapeNext) {
                            inString = !inString;
                        }
                        
                        if (!inString) {
                            if (char === '{') braceCount++;
                            else if (char === '}') {
                                braceCount--;
                                if (braceCount === 0) {
                                    endIndex = i + 1;
                                    break;
                                }
                            }
                        }
                    }
                    
                    if (jsonStartIndex === -1 || endIndex === -1) {
                        console.log('[OllamaProvider] Could not find complete JSON after python_tag');
                        return null;
                    }
                    
                    const jsonStr = textAfterTag.substring(jsonStartIndex, endIndex);
                    
                    try {
                        const toolCallJson = JSON.parse(jsonStr);
                        if (toolCallJson.type === 'function' && toolCallJson.name) {
                            // Convert parameters to input format
                            const params = toolCallJson.parameters || {};
                            // Remove any meta parameters that aren't actual tool parameters
                            const { 
                                context_for_interpretation, 
                                expected_format, 
                                key_information, 
                                success_indicators, 
                                tool_purpose,
                                ...actualParams 
                            } = params;
                            
                            return {
                                name: toolCallJson.name,
                                input: actualParams
                            };
                        }
                    } catch (e) {
                        console.log('[OllamaProvider] Failed to parse Llama tool JSON:', jsonStr, e);
                    }
                    return null;
                }
            }
        ];
        
        let lastIndex = 0;
        const allMatches = [];
        
        // Find all matches from all patterns
        for (const pattern of patterns) {
            pattern.regex.lastIndex = 0;
            let match;
            while ((match = pattern.regex.exec(text)) !== null) {
                allMatches.push({
                    index: match.index,
                    length: match[0].length,
                    parser: pattern.parser,
                    match: match
                });
            }
        }
        
        // Sort matches by index
        allMatches.sort((a, b) => a.index - b.index);
        
        // Process matches in order
        for (const matchInfo of allMatches) {
            // Add any text before the tool call
            if (matchInfo.index > lastIndex) {
                const textBefore = text.substring(lastIndex, matchInfo.index).trim();
                if (textBefore) {
                    contentArray.push({ type: 'text', text: textBefore });
                }
            }
            
            try {
                const parsed = matchInfo.parser(matchInfo.match, text, matchInfo.index);
                if (parsed) {
                    // Validate that the tool exists
                    const toolName = parsed.name;
                    const toolExists = availableTools.some(t => 
                        (t.name === toolName) || 
                        (t.function && t.function.name === toolName)
                    );
                    
                    if (toolExists) {
                        contentArray.push({
                            type: 'tool_use',
                            id: this.generateId(),
                            name: toolName,
                            input: parsed.input
                        });
                    } else {
                        // Tool doesn't exist, treat as text
                        contentArray.push({ type: 'text', text: matchInfo.match[0] });
                    }
                } else {
                    // Invalid format, treat as text
                    contentArray.push({ type: 'text', text: matchInfo.match[0] });
                }
            } catch (e) {
                // Parsing failed, treat as text
                console.log('[OllamaProvider] Failed to parse tool call:', e, matchInfo.match[0]);
                contentArray.push({ type: 'text', text: matchInfo.match[0] });
            }
            
            lastIndex = matchInfo.index + matchInfo.length;
        }
        
        // Add any remaining text
        if (lastIndex < text.length) {
            const remainingText = text.substring(lastIndex).trim();
            if (remainingText) {
                contentArray.push({ type: 'text', text: remainingText });
            }
        }
        
        // If no content was parsed, return the original text
        if (contentArray.length === 0) {
            contentArray.push({ type: 'text', text: text });
        }
        
        return contentArray;
    }
}

/**
 * Factory function to create appropriate LLM provider
 */
function createLLMProvider(provider, proxyUrl, model) {
    switch (provider) {
        case 'openai':
        case 'openai-responses':
            return new OpenAIProvider(proxyUrl, model);
        case 'anthropic':
            return new AnthropicProvider(proxyUrl, model);
        case 'google':
            return new GoogleProvider(proxyUrl, model);
        case 'ollama':
            return new OllamaProvider(proxyUrl, model);
        default:
            const error = `Unknown provider type: ${provider}`;
            console.error('[createLLMProvider]', error);
            throw new Error(error);
    }
}

// Export for use in other modules
window.createLLMProvider = createLLMProvider;
