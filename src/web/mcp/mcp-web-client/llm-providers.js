/**
 * LLM Provider integrations for OpenAI, Anthropic, and Google
 */

class LLMProvider {
    constructor(proxyUrl = 'http://localhost:8081') {
        this.onLog = null; // Logging callback
        this.proxyUrl = proxyUrl;
    }

    async sendMessage(messages, tools = [], temperature = 0.7, mode = 'cached', cachePosition = null) {
        throw new Error('sendMessage must be implemented by subclass');
    }

    log(direction, message, metadata = {}) {
        const logEntry = {
            timestamp: new Date().toISOString(),
            direction: direction,
            message: message,
            metadata: metadata
        };
        
        // Console log for debugging
        console.log(`[LLM ${direction.toUpperCase()}]`, logEntry);
        
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
            messages: messages,
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

    formatToolResponse(toolCallId, result) {
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
        // Convert messages to Anthropic format with caching (only if mode is 'cached')
        const anthropicMessages = mode === 'cached' 
            ? this.convertMessagesWithCaching(messages, cachePosition) 
            : this.convertMessages(messages);
        
        // Convert tools to Anthropic format (no cache control on tools)
        const anthropicTools = tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            input_schema: tool.inputSchema || {}
        }));

        // Find system message (no cache control on system message)
        let system = undefined;
        const systemMsg = messages.find(m => m.role === 'system');
        if (systemMsg) {
            system = [
                {
                    type: "text",
                    text: systemMsg.content
                }
            ];
        }

        const requestBody = {
            model: this.model,
            messages: anthropicMessages,
            system: system,
            tools: anthropicTools.length > 0 ? anthropicTools : undefined,
            max_tokens: 4096,
            temperature: temperature
        };


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

    convertMessagesWithCaching(messages, cachePosition = null) {
        // Convert messages WITHOUT adding cache control yet
        const converted = [];
        let lastRole = null;
        
        for (let i = 0; i < messages.length; i++) {
            const msg = messages[i];
            
            if (msg.role === 'system') {
                // System messages are handled separately in sendMessage
                continue;
            }
            
            const role = msg.role;
            
            // Check if we need to merge consecutive messages with same role
            if (role === lastRole && converted.length > 0) {
                const last = converted[converted.length - 1];
                
                // Convert string content to array if needed
                if (typeof last.content === 'string') {
                    last.content = [{ type: 'text', text: last.content }];
                }
                
                // Merge content
                if (typeof msg.content === 'string') {
                    last.content.push({ type: 'text', text: msg.content });
                } else if (Array.isArray(msg.content)) {
                    // Important: clone the content blocks to avoid modifying the original
                    const clonedBlocks = msg.content.map(block => ({...block}));
                    last.content.push(...clonedBlocks);
                }
            } else {
                // Process the message content
                let processedContent = msg.content;
                
                if (typeof msg.content === 'string') {
                    processedContent = [{
                        type: 'text',
                        text: msg.content
                    }];
                } else if (Array.isArray(msg.content)) {
                    // Important: clone the content blocks to avoid modifying the original
                    processedContent = msg.content.map(block => ({...block}));
                }
                
                converted.push({
                    role: role,
                    content: processedContent
                });
                lastRole = role;
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
        
        return converted;
    }

    convertMessages(messages) {
        // Fallback method without caching for compatibility
        const converted = [];
        let lastRole = null;
        
        for (const msg of messages) {
            if (msg.role === 'system') {
                // System messages will be handled separately
                continue;
            }
            
            const role = msg.role;
            
            // Check if we need to merge consecutive messages with same role
            if (role === lastRole && converted.length > 0) {
                const last = converted[converted.length - 1];
                
                // Convert string content to array if needed
                if (typeof last.content === 'string') {
                    last.content = [{ type: 'text', text: last.content }];
                }
                
                // Merge content
                if (typeof msg.content === 'string') {
                    last.content.push({ type: 'text', text: msg.content });
                } else if (Array.isArray(msg.content)) {
                    last.content.push(...msg.content);
                }
            } else {
                // Add message as-is
                converted.push({
                    role: role,
                    content: msg.content
                });
                lastRole = role;
            }
        }
        
        return converted;
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
        // Convert messages to Gemini format
        const contents = this.convertMessages(messages);
        
        // Convert tools to Gemini format
        const functionDeclarations = tools.map(tool => ({
            name: tool.name,
            description: tool.description,
            parameters: tool.inputSchema || {}
        }));

        const requestBody = {
            contents: contents,
            generationConfig: {
                temperature: temperature,
                maxOutputTokens: 4096
            }
        };

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
        
        // Extract content and function calls
        let content = '';
        const toolCalls = [];
        
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
        
        // Google returns token counts in usageMetadata
        const usage = data.usageMetadata ? {
            promptTokens: data.usageMetadata.promptTokenCount,
            completionTokens: data.usageMetadata.candidatesTokenCount,
            totalTokens: data.usageMetadata.totalTokenCount
        } : null;
        
        return { content, toolCalls, usage };
    }

    convertMessages(messages) {
        const contents = [];
        
        for (const msg of messages) {
            if (msg.role === 'system') {
                // Prepend system message to first user message
                continue;
            }
            
            const parts = [];
            
            if (msg.role === 'tool') {
                // Function response from formatToolResponse
                parts.push({
                    functionResponse: {
                        name: msg.tool_name,
                        response: {
                            content: msg.content
                        }
                    }
                });
            } else if (msg.role === 'assistant' && msg.toolCalls && msg.toolCalls.length > 0) {
                // Assistant with text and tool calls
                if (msg.content) {
                    parts.push({ text: msg.content });
                }
                for (const tc of msg.toolCalls) {
                    parts.push({
                        functionCall: {
                            name: tc.name,
                            args: tc.arguments
                        }
                    });
                }
            } else if (msg.content) {
                // Regular text message
                parts.push({ text: msg.content });
            }
            
            if (parts.length > 0) {
                contents.push({
                    role: msg.role === 'assistant' ? 'model' : 'user',
                    parts: parts
                });
            }
        }
        
        // Add system message to first content if exists
        const systemMsg = messages.find(m => m.role === 'system');
        if (systemMsg && contents.length > 0 && contents[0].parts[0].text) {
            contents[0].parts[0].text = systemMsg.content + '\n\n' + contents[0].parts[0].text;
        }
        
        return contents;
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
