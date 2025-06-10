/**
 * LLM Provider integrations for OpenAI, Anthropic, and Google
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
        if (messages[0].role === 'system') {
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
        // For tool-results, always use 'tool-results' even if role is set
        const msgRole = msg.type === 'tool-results' ? 'tool-results' : (msg.role || msg.type);
        
        // Check role at position i
        
        if (msgRole === 'user') {
            if (expectedRole !== 'user' && expectedRole !== 'user-or-tool-results') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'user'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.type === 'tool-results' ? 'tool-results' : (m.role || m.type)}`));
                throw new Error(error);
            }
            expectedRole = 'assistant';
            lastAssistantMessage = null;
            
        } else if (msgRole === 'assistant') {
            if (expectedRole !== 'assistant') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'assistant'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.type === 'tool-results' ? 'tool-results' : (m.role || m.type)}`));
                throw new Error(error);
            }
            lastAssistantMessage = msg;
            // After assistant, we can have either tool-results or user
            expectedRole = 'user-or-tool-results';
            
        } else if (msgRole === 'tool-results') {
            if (expectedRole !== 'user-or-tool-results') {
                const error = `Message sequence error at position ${i}: expected '${expectedRole}', but got 'tool-results'`;
                console.error('[validateMessagesForAPI]', error);
                console.error('[validateMessagesForAPI] Full sequence:', messages.map((m, idx) => `${idx}: ${m.type === 'tool-results' ? 'tool-results' : (m.role || m.type)}`));
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
 * Validates that tool results match the assistant's tool calls exactly
 * @param {Object} assistantMsg - The assistant message with tool calls
 * @param {Object} toolResultsMsg - The tool-results message
 * @param {number} position - Position in messages array for error reporting
 * @throws {Error} If validation fails
 */
function validateToolCalls(assistantMsg, toolResultsMsg, position) {
    // Extract tool calls from assistant message
    const toolCalls = assistantMsg.toolCalls || [];
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

    async sendMessage() {
        throw new Error('sendMessage must be implemented by subclass');
    }

    log(direction, message, metadata = {}) {
        const logEntry = {
            timestamp: new Date().toISOString(),
            direction: direction,
            message: message,
            metadata: metadata
        };
        
        // Log to UI callback only, not console
        
        // UI log
        if (this.onLog) {
            this.onLog(logEntry);
        }
    }

    setProxyUrl(proxyUrl) {
        this.proxyUrl = proxyUrl;
    }
}

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
        return `${this.proxyUrl}/proxy/openai/v1/chat/completions`;
    }

    async sendMessage(messages, tools = [], temperature = 0.7, mode = 'cached', cachePosition = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages from internal format to OpenAI format
        const openaiMessages = this.convertMessages(messages, mode);
        
        const openaiTools = tools.map(tool => ({
            type: 'function',
            function: {
                name: tool.name,
                description: tool.description,
                parameters: tool.inputSchema || {}
            }
        }));

        const requestBody = {
            model: this.model,
            messages: openaiMessages,
            tools: openaiTools.length > 0 ? openaiTools : undefined,
            tool_choice: openaiTools.length > 0 ? 'auto' : undefined,
            temperature: temperature,
            max_tokens: 4096
        };

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'openai', 
            model: this.model,
            url: this.apiUrl
        });

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
                throw new Error(`Connection Error: Cannot reach OpenAI API at ${this.apiUrl}. Please ensure the proxy server is running.`);
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
            throw new Error(`OpenAI API error: ${error.error?.message || response.statusText}`);
        }

        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'openai' });
        
        const choice = data.choices[0];
        
        return {
            content: choice.message.content,
            toolCalls: choice.message.tool_calls?.map(tc => ({
                id: tc.id,
                name: tc.function.name,
                arguments: JSON.parse(tc.function.arguments)
            })) || [],
            usage: data.usage ? {
                promptTokens: data.usage.prompt_tokens,
                completionTokens: data.usage.completion_tokens,
                totalTokens: data.usage.total_tokens
            } : null
        };
    }

    convertMessages(messages, mode = 'cached') {
        // Convert messages for OpenAI format
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
            const msgRole = msg.role || msg.type;
            
            if (msgRole === 'user') {
                converted.push({
                    role: 'user',
                    content: msg.content
                });
            } else if (msgRole === 'assistant') {
                // Convert assistant message to OpenAI format
                const openaiMsg = {
                    role: 'assistant',
                    content: msg.content || null
                };
                
                // Add tool calls if present
                if (msg.toolCalls && msg.toolCalls.length > 0) {
                    openaiMsg.tool_calls = msg.toolCalls.map(tc => ({
                        id: tc.id,
                        type: 'function',
                        function: {
                            name: tc.function?.name || tc.name,
                            arguments: typeof tc.function?.arguments === 'string' 
                                ? tc.function.arguments 
                                : JSON.stringify(tc.function?.arguments || tc.arguments || {})
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

    shouldIncludeToolCalls(msg, mode) {
        // Determine if tool calls should be included based on mode
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    shouldIncludeToolResults(msg, mode) {
        // Tool results should only be included if their corresponding calls were included
        // This logic matches the tool call inclusion logic
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    formatToolResponse(toolCallId, result, toolName) {
        // Format MCP tool results for OpenAI
        const formatted = MessageConversionUtils.formatMCPToolResult(result);
        let content = '';
        
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
            content: content
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

    async sendMessage(messages, tools = [], temperature = 0.7, mode = 'cached', cachePosition = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages to Anthropic format
        let anthropicMessages, system;
        
        if (mode === 'cached') {
            // Use the caching version which returns different format
            const result = this.convertMessagesWithCaching(messages, cachePosition, mode);
            anthropicMessages = result.converted;
            // Extract system from original messages for cached mode
            const systemMsg = messages.find(m => m.role === 'system');
            if (systemMsg) {
                system = [{
                    type: 'text',
                    text: systemMsg.content
                }];
            }
        } else {
            // Use regular conversion which handles system properly
            const result = this.convertMessages(messages, mode);
            anthropicMessages = result.messages;
            system = result.system ? [{
                type: 'text',
                text: result.system
            }] : undefined;
        }
        
        // Convert tools to Anthropic format (no cache control on tools)
        const anthropicTools = tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            input_schema: tool.inputSchema || {}
        }));

        const requestBody = {
            model: this.model,
            messages: anthropicMessages,
            system: system,
            tools: anthropicTools.length > 0 ? anthropicTools : undefined,
            max_tokens: 4096,
            temperature: temperature
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
                throw new Error(`Connection Error: Cannot reach Anthropic API at ${this.apiUrl}. Please ensure the proxy server is running.`);
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
            throw new Error(`Anthropic API error: ${error.error?.message || response.statusText}`);
        }

        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'anthropic' });
        
        // Extract content and tool calls
        let content = '';
        const toolCalls = [];
        
        for (const block of data.content) {
            if (block.type === 'text') {
                content += block.text;
            } else if (block.type === 'tool_use') {
                toolCalls.push({
                    id: block.id,
                    name: block.name,
                    arguments: block.input
                });
            } else {
                // Log any unknown block types
                console.warn('Unknown block type in Anthropic response:', block.type, block);
            }
        }
        
        return { 
            content, 
            toolCalls,
            usage: data.usage ? {
                promptTokens: data.usage.input_tokens,
                completionTokens: data.usage.output_tokens,
                totalTokens: (data.usage.input_tokens || 0) + (data.usage.output_tokens || 0),
                cacheCreationInputTokens: data.usage.cache_creation_input_tokens,
                cacheReadInputTokens: data.usage.cache_read_input_tokens
            } : null
        };
    }

    convertMessagesWithCaching(messages, cachePosition = null, mode = 'cached') {
        // Convert messages WITHOUT adding cache control yet
        const converted = [];
        let lastRole = null;
        
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
            const msgRole = msg.type === 'tool-results' ? 'tool-results' : (msg.role || msg.type);
            
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
                        lastRole = 'user';
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
                                lastRole = 'user';
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
                    } else if (msg.content.type === 'text' && msg.content.text) {
                        textContent = msg.content.text;
                    } else {
                        console.warn('Unknown user message object format, using fallback:', msg.content);
                        textContent = JSON.stringify(msg.content);
                    }
                } else {
                    textContent = '';
                }
                
                converted.push({
                    role: 'user',
                    content: [{ type: 'text', text: textContent }]
                });
                lastRole = 'user';
            } else if (msgRole === 'assistant') {
                // Convert assistant message to Anthropic format
                const content = [];
                
                // Add text content if present
                if (msg.content) {
                    // msg.content might be a string or already an array
                    if (typeof msg.content === 'string') {
                        content.push({ type: 'text', text: msg.content });
                    } else if (Array.isArray(msg.content)) {
                        // Content is already an array - extract text content only
                        const textContent = msg.content.find(item => 
                            typeof item === 'string' || (item && item.type === 'text')
                        );
                        if (typeof textContent === 'string') {
                            content.push({ type: 'text', text: textContent });
                        } else if (textContent && textContent.text) {
                            content.push({ type: 'text', text: textContent.text });
                        }
                    }
                }
                
                // Add tool use blocks if present and should be included
                if (msg.toolCalls && msg.toolCalls.length > 0 && this.shouldIncludeToolCalls(msg, mode)) {
                    for (const tc of msg.toolCalls) {
                        content.push({
                            type: 'tool_use',
                            id: tc.id,
                            name: tc.name,
                            input: tc.arguments
                        });
                    }
                }
                
                if (content.length > 0) {
                    converted.push({
                        role: 'assistant',
                        content: content
                    });
                    lastRole = 'assistant';
                }
            } else if (msgRole === 'tool-results') {
                // Convert tool results to Anthropic format
                // Only include if corresponding tool calls were included
                if (this.shouldIncludeToolResults(msg, mode)) {
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
                            content: content
                        });
                        lastRole = 'user';
                    }
                }
            }
        }
        
        // Apply cache control based on cachePosition parameter
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
        
        return { converted, summaryContent };
    }

    convertMessages(messages, mode = 'cached') {
        // Convert messages for Anthropic format
        const converted = [];
        
        // Extract system prompt and handle summary (Anthropic uses separate system parameter)
        const { systemPrompt, messages: remainingMessages } = MessageConversionUtils.extractSystemAndSummary(messages);
        
        // Process remaining messages
        for (const msg of remainingMessages) {
            // Use same role detection logic as validation
            const msgRole = msg.type === 'tool-results' ? 'tool-results' : (msg.role || msg.type);
            
            if (msgRole === 'user') {
                // Convert user message to Anthropic format with content blocks
                converted.push({
                    role: 'user',
                    content: [{ type: 'text', text: msg.content }]
                });
            } else if (msgRole === 'assistant') {
                // Convert assistant message to Anthropic format
                const content = [];
                
                // Add text content if present
                if (msg.content) {
                    content.push({ type: 'text', text: msg.content });
                }
                
                // Add tool use blocks if present
                if (msg.toolCalls && msg.toolCalls.length > 0) {
                    for (const tc of msg.toolCalls) {
                        content.push({
                            type: 'tool_use',
                            id: tc.id,
                            name: tc.function?.name || tc.name,
                            input: tc.function?.arguments || tc.arguments || {}
                        });
                    }
                }
                
                if (content.length > 0) {
                    converted.push({
                        role: 'assistant',
                        content: content
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
                        content: content
                    });
                }
            } else {
                console.warn('[Anthropic] Unexpected message role:', msgRole);
            }
        }
        
        // Return both messages and system prompt (needed by sendMessage)
        return { messages: converted, system: systemPrompt };
    }

    shouldIncludeToolCalls(msg, mode) {
        // Determine if tool calls should be included based on mode
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    shouldIncludeToolResults(msg, mode) {
        // Tool results should only be included if their corresponding calls were included
        // This logic matches the tool call inclusion logic
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    formatToolResultForAnthropic(toolCallId, result, toolName) {
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
            content: content
        };
    }

    formatToolResponse(toolCallId, result) {
        // For Anthropic, tool results must be in user messages with tool_result blocks
        // Handle different types of results
        let content;
        if (typeof result === 'string') {
            content = result;
        } else if (Array.isArray(result)) {
            // For arrays, stringify each element if needed and join
            content = result.map(item => 
                typeof item === 'string' ? item : JSON.stringify(item)
            ).join('\n\n');
        } else {
            content = JSON.stringify(result);
        }
        
        // Return in Anthropic's expected format
        return {
            role: 'user',
            content: [{
                type: 'tool_result',
                tool_use_id: toolCallId,
                content: content
            }]
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

    async sendMessage(messages, tools = [], temperature = 0.7, mode = 'cached', cachePosition = null) {
        // Validate messages before processing
        validateMessagesForAPI(messages);
        
        // Convert messages to Gemini format
        const { contents, systemInstruction } = this.convertMessages(messages, mode);
        
        // Convert tools to Gemini format
        const functionDeclarations = tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            parameters: this.cleanSchemaForGoogle(tool.inputSchema || {})
        }));

        const requestBody = {
            contents: contents,
            generationConfig: {
                temperature: temperature,
                maxOutputTokens: 4096
            }
        };
        
        // Add system instruction if present
        if (systemInstruction) {
            requestBody.systemInstruction = {
                parts: [{ text: systemInstruction }]
            };
        }

        if (functionDeclarations.length > 0) {
            requestBody.tools = [{
                function_declarations: functionDeclarations
            }];
        }

        this.log('sent', JSON.stringify(requestBody, null, 2), { 
            provider: 'google', 
            model: this.model,
            url: this.apiUrl
        });

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
                throw new Error(`Connection Error: Cannot reach Google AI API at ${this.apiUrl}. Please ensure the proxy server is running.`);
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
            throw new Error(`Google API error: ${error.error?.message || response.statusText}`);
        }

        const data = await response.json();
        this.log('received', JSON.stringify(data, null, 2), { provider: 'google' });
        const candidate = data.candidates[0];
        
        // Check finish reason for potential issues
        if (candidate.finishReason === 'MAX_TOKENS') {
            // Handle token limit error
            const error = new Error('Google Gemini response was truncated due to token limit. The model\'s context window is full. Consider starting a new conversation or summarizing the current one.');
            error.code = 'MAX_TOKENS';
            throw error;
        } else if (candidate.finishReason === 'SAFETY') {
            // Handle safety filter
            const error = new Error('Google Gemini blocked the response due to safety filters.');
            error.code = 'SAFETY';
            throw error;
        } else if (candidate.finishReason && candidate.finishReason !== 'STOP') {
            // Handle other unexpected finish reasons
            const error = new Error(`Google Gemini response ended unexpectedly: ${candidate.finishReason}`);
            error.code = candidate.finishReason;
            throw error;
        }
        
        // Extract content and function calls
        let content = '';
        const toolCalls = [];
        
        // Ensure parts array exists
        if (candidate.content && candidate.content.parts) {
            for (const part of candidate.content.parts) {
                if (part.text) {
                    content += part.text;
                } else if (part.functionCall) {
                    toolCalls.push({
                        id: this.generateId(),
                        name: part.functionCall.name,
                        arguments: part.functionCall.args
                    });
                }
            }
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
        
        return { content, toolCalls, usage };
    }

    convertMessages(messages, mode = 'cached') {
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
            const msgRole = msg.type === 'tool-results' ? 'tool-results' : (msg.role || msg.type);
            
            // Check for tool calls in assistant messages
            if (msgRole === 'assistant' && msg.toolCalls && this.shouldIncludeToolCalls(msg, mode)) {
                for (const tc of msg.toolCalls) {
                    if (!allToolCalls.has(tc.name)) {
                        allToolCalls.set(tc.name, []);
                    }
                    allToolCalls.get(tc.name).push(i);
                }
            } else if (msgRole === 'tool-results' && this.shouldIncludeToolResults(msg, mode)) {
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
                        messages: messages.map((m, i) => ({
                            index: i,
                            role: m.role,
                            hasToolCalls: !!m.toolCalls,
                            toolCallNames: m.toolCalls?.map(tc => tc.name)
                        }))
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
            const msgRole = msg.type === 'tool-results' ? 'tool-results' : (msg.role || msg.type);
            
            if (msgRole === 'system') {
                // Prepend system message to first user message
                continue;
            }
            
            if (msgRole === 'tool-results') {
                // STRICT: Only accept toolResults property
                const toolResults = msg.toolResults || [];
                // Process tool results
                
                // Only include if should be included
                if (!this.shouldIncludeToolResults(msg, mode)) {
                    continue;
                }
                
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
                lastAssistantHadFunctionCalls = msg.toolCalls && msg.toolCalls.length > 0 && 
                                               this.shouldIncludeToolCalls(msg, mode);
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
                parts.push({ text: msg.content });
            } else if (msgRole === 'assistant') {
                // Assistant messages - include text and optionally tool calls
                if (msg.content) {
                    parts.push({ text: msg.content });
                }
                if (msg.toolCalls && msg.toolCalls.length > 0 && this.shouldIncludeToolCalls(msg, mode)) {
                    for (const tc of msg.toolCalls) {
                        parts.push({
                            functionCall: {
                                name: tc.name,
                                args: tc.arguments
                            }
                        });
                    }
                }
            }
            
            if (parts.length > 0) {
                contents.push({
                    role: msgRole === 'assistant' ? 'model' : 'user',
                    parts: parts
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

    shouldIncludeToolCalls(msg, mode) {
        // Determine if tool calls should be included based on mode
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    shouldIncludeToolResults(msg, mode) {
        // Tool results should only be included if their corresponding calls were included
        // This logic matches the tool call inclusion logic
        if (mode === 'all-off') return false;
        if (mode === 'all-on') return true;
        if (mode === 'manual') {
            // Check individual tool inclusion state (would need to be passed in)
            return true; // Default to include for now
        }
        // For 'auto' and 'cached' modes, include by default
        return true;
    }

    formatToolResultForGoogle(toolCallId, result, toolName) {
        // Format MCP tool results for Google's functionResponse
        const formatted = MessageConversionUtils.formatMCPToolResult(result);
        let responseContent = {};
        
        if (formatted.type === 'text') {
            responseContent = { text: formatted.content };
        } else if (formatted.type === 'multi') {
            // Handle multiple content items from MCP
            const parts = [];
            for (const item of formatted.items) {
                if (item.type === 'text') {
                    parts.push(item.content);
                } else if (item.type === 'image') {
                    // Google doesn't support images in function responses directly
                    parts.push(`[Image: ${item.mimeType}]`);
                } else if (item.type === 'resource') {
                    parts.push(`[Resource: ${item.uri}]\n${item.text || ''}`);
                }
            }
            responseContent = { text: parts.join('\n\n') };
        } else if (formatted.type === 'json') {
            responseContent = formatted.content; // Google accepts objects directly
        } else {
            responseContent = result; // Fallback
        }
        
        return {
            functionResponse: {
                name: toolName,
                response: responseContent
            }
        };
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

    formatToolResponse(toolCallId, result, toolName) {
        // Handle different types of results
        let content;
        if (typeof result === 'string') {
            content = result;
        } else if (Array.isArray(result)) {
            // For arrays, stringify each element if needed and join
            content = result.map(item => 
                typeof item === 'string' ? item : JSON.stringify(item)
            ).join('\n\n');
        } else {
            content = JSON.stringify(result);
        }
        
        return {
            role: 'tool',
            tool_call_id: toolCallId,
            tool_name: toolName,
            content: content
        };
    }

    generateId() {
        return 'call_' + Math.random().toString(36).substr(2, 9);
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
 * Factory function to create appropriate LLM provider
 */
function createLLMProvider(provider, proxyUrl, model) {
    switch (provider) {
        case 'openai':
            return new OpenAIProvider(proxyUrl, model);
        case 'anthropic':
            return new AnthropicProvider(proxyUrl, model);
        case 'google':
            return new GoogleProvider(proxyUrl, model);
        default:
            throw new Error(`Unknown provider: ${provider}`);
    }
}

// Export for use in other modules
window.createLLMProvider = createLLMProvider;
