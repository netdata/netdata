/**
 * Tool Response Summarizer Module
 * 
 * Handles summarization of large tool responses using secondary LLM models
 * to reduce costs while maintaining context quality.
 * 
 * STRICT MODE: All parameters are validated. Invalid inputs throw errors.
 */

export class ToolSummarizer {
    /**
     * Creates a new ToolSummarizer instance
     * @param {Object} config - Configuration object
     * @param {Function} config.llmProviderFactory - Function to create LLM provider instances
     * @param {string} config.primaryModel - Primary model identifier
     * @param {string|null} config.secondaryModel - Secondary model identifier for summarization
     * @param {number} config.threshold - Byte threshold for triggering summarization
     * @param {boolean} config.useSecondaryModel - Whether to use secondary model
     * @throws {Error} - If configuration is invalid
     */
    constructor(config) {
        // STRICT: Validate configuration
        if (!config || typeof config !== 'object') {
            throw new Error('[ToolSummarizer] config must be a valid object');
        }
        
        if (typeof config.llmProviderFactory !== 'function') {
            throw new Error('[ToolSummarizer] llmProviderFactory must be a function');
        }
        
        if (typeof config.primaryModel !== 'string') {
            throw new Error('[ToolSummarizer] primaryModel must be a string');
        }
        
        if (config.secondaryModel !== null && typeof config.secondaryModel !== 'string') {
            throw new Error('[ToolSummarizer] secondaryModel must be a string or null');
        }
        
        if (typeof config.threshold !== 'number' || config.threshold < 0) {
            throw new Error('[ToolSummarizer] threshold must be a positive number');
        }
        
        if (typeof config.useSecondaryModel !== 'boolean') {
            throw new Error('[ToolSummarizer] useSecondaryModel must be boolean');
        }
        
        this.llmProviderFactory = config.llmProviderFactory;
        this.primaryModel = config.primaryModel;
        this.secondaryModel = config.secondaryModel;
        this.threshold = config.threshold;
        this.useSecondaryModel = config.useSecondaryModel;
        
        // Cache for LLM providers
        this.providerCache = new Map();
    }
    
    /**
     * Determines if a tool result should be summarized based on size
     * @param {Object} toolResult - Tool result object
     * @returns {boolean} - True if should be summarized
     */
    shouldSummarize(toolResult) {
        if (!toolResult || !toolResult.result) {
            return false;
        }
        
        // Calculate size of the result
        const size = this.calculateSize(toolResult.result);
        return size > this.threshold;
    }
    
    /**
     * Calculates the byte size of a tool result
     * @param {any} result - Tool result (can be string, object, array)
     * @returns {number} - Size in bytes
     */
    calculateSize(result) {
        if (typeof result === 'string') {
            return new Blob([result]).size;
        }
        
        // For objects/arrays, stringify first
        try {
            const stringified = JSON.stringify(result);
            return new Blob([stringified]).size;
        } catch (_error) {
            // If can't stringify (circular refs etc), estimate
            return 0;
        }
    }
    
    /**
     * Summarizes a tool result using the configured LLM
     * @param {Object} params - Summarization parameters
     * @param {Object} params.toolResult - The tool result to summarize
     * @param {string} params.toolName - Name of the tool
     * @param {Object} params.toolSchema - Tool's schema/description
     * @param {string} params.userQuestion - Original user question
     * @param {string} params.assistantReasoning - Assistant's reasoning before tool call
     * @param {Object} params.providerInfo - LLM provider info (url, apiKey)
     * @returns {Promise<Object>} - Summarized result with metadata
     * @throws {Error} - If summarization fails
     */
    async summarizeToolResult(params) {
        // STRICT: Validate all parameters
        if (!params || typeof params !== 'object') {
            throw new Error('[ToolSummarizer] params must be a valid object');
        }
        
        const required = ['toolResult', 'toolName', 'toolSchema', 'userQuestion', 'assistantReasoning', 'providerInfo'];
        for (const field of required) {
            if (!params[field]) {
                throw new Error(`[ToolSummarizer] Missing required parameter: ${field}`);
            }
        }
        
        // Determine which model to use
        const modelToUse = this.useSecondaryModel && this.secondaryModel 
            ? this.secondaryModel 
            : this.primaryModel;
            
        // Get or create LLM provider
        const provider = this.getProvider(modelToUse, params.providerInfo);
        
        // Build summarization prompt
        const prompt = this.buildSummarizationPrompt(params);
        
        try {
            // Send to LLM for summarization
            const response = await provider.sendMessage(
                [
                    { role: 'system', content: this.getSystemPrompt() },
                    { role: 'user', content: prompt }
                ],
                [], // No tools for summarization
                0.3, // Lower temperature for factual summarization
                'all-off', // No tools
                null, // No cache position
                null // No chat context needed for summarization
            );
            
            // Parse and validate response
            const summary = this.parseResponse(response.content);
            
            return {
                originalSize: this.calculateSize(params.toolResult.result),
                summarizedSize: this.calculateSize(summary),
                compressionRatio: this.calculateCompressionRatio(params.toolResult.result, summary),
                summary,
                model: modelToUse,
                usage: response.usage,
                timestamp: new Date().toISOString()
            };
            
        } catch (error) {
            console.error('[ToolSummarizer] Summarization failed:', error);
            throw new Error(`Tool summarization failed: ${error.message}`);
        }
    }
    
    /**
     * Gets or creates an LLM provider instance
     * @param {string} model - Model identifier
     * @param {Object} providerInfo - Provider configuration
     * @returns {Object} - LLM provider instance
     */
    getProvider(model, providerInfo) {
        const cacheKey = `${model}-${providerInfo.url}`;
        
        if (!this.providerCache.has(cacheKey)) {
            // Parse model identifier to get provider type
            const { provider, modelName } = this.parseModelIdentifier(model);
            
            // Create new provider instance
            const providerInstance = this.llmProviderFactory(
                provider,
                providerInfo.url,
                modelName
            );
            
            this.providerCache.set(cacheKey, providerInstance);
        }
        
        return this.providerCache.get(cacheKey);
    }
    
    /**
     * Parses model identifier to extract provider and model name
     * @param {string} modelId - Model identifier (e.g., "anthropic/claude-3-haiku")
     * @returns {Object} - { provider, modelName }
     */
    parseModelIdentifier(modelId) {
        const parts = modelId.split('/');
        if (parts.length !== 2) {
            throw new Error(`[ToolSummarizer] Invalid model identifier: ${modelId}`);
        }
        
        return {
            provider: parts[0],
            modelName: parts[1]
        };
    }
    
    /**
     * Builds the summarization prompt
     * @param {Object} params - Parameters containing context
     * @returns {string} - Formatted prompt
     */
    buildSummarizationPrompt(params) {
        const { toolResult, toolName, toolSchema, userQuestion, assistantReasoning } = params;
        
        // Format the tool result for inclusion
        const resultText = this.formatToolResult(toolResult.result);
        
        return `You are helping to summarize a large tool response to reduce token usage while preserving all important information.

## Context

**User's Original Question:**
${userQuestion}

**Assistant's Reasoning:**
${assistantReasoning}

**Tool Information:**
- Name: ${toolName}
- Description: ${toolSchema.description || 'No description available'}
- Purpose: ${this.inferToolPurpose(toolSchema)}

## Tool Response to Summarize

<tool_response>
${resultText}
</tool_response>

## Instructions

Create a concise summary of the tool response that:
1. Preserves ALL information relevant to answering the user's question
2. Maintains any specific data points, numbers, or identifiers the assistant might need
3. Removes redundant or verbose formatting while keeping the substance
4. Organizes information clearly for the assistant to process

Focus on what the assistant needs to answer the user's question effectively.

Provide ONLY the summary - no preamble or explanation.`;
    }
    
    /**
     * Gets the system prompt for summarization
     * @returns {string} - System prompt
     */
    getSystemPrompt() {
        return `You are a specialized assistant that summarizes tool responses to reduce token usage in LLM conversations. 
Your summaries must be accurate, complete, and preserve all information needed to answer user questions.
Never add information that wasn't in the original response.
Focus on clarity and conciseness while maintaining completeness.`;
    }
    
    /**
     * Formats tool result for inclusion in prompt
     * @param {any} result - Tool result
     * @returns {string} - Formatted result
     */
    formatToolResult(result) {
        if (typeof result === 'string') {
            return result;
        }
        
        // For objects/arrays, pretty print
        try {
            return JSON.stringify(result, null, 2);
        } catch (_error) {
            return String(result);
        }
    }
    
    /**
     * Infers tool purpose from schema
     * @param {Object} schema - Tool schema
     * @returns {string} - Inferred purpose
     */
    inferToolPurpose(schema) {
        // Look for common patterns in tool schemas
        const inputSchema = schema.inputSchema || {};
        const properties = inputSchema.properties || {};
        
        // Some heuristics based on common MCP tools
        if (properties.path || properties.file_path) {
            return 'File system operation';
        }
        if (properties.command || properties.script) {
            return 'Command execution';
        }
        if (properties.query || properties.sql) {
            return 'Data query';
        }
        if (properties.url || properties.endpoint) {
            return 'Network request';
        }
        
        return 'General tool operation';
    }
    
    /**
     * Parses and validates the summary response
     * @param {string} response - Raw response from LLM
     * @returns {string} - Validated summary
     */
    parseResponse(response) {
        if (!response || typeof response !== 'string') {
            throw new Error('[ToolSummarizer] Invalid response format');
        }
        
        // Trim whitespace
        const trimmed = response.trim();
        
        if (trimmed.length === 0) {
            throw new Error('[ToolSummarizer] Empty summary response');
        }
        
        // Check if summary is actually shorter than a reasonable limit
        if (trimmed.length > 10000) {
            console.warn('[ToolSummarizer] Summary seems too long, might not be effective');
        }
        
        return trimmed;
    }
    
    /**
     * Calculates compression ratio
     * @param {any} original - Original content
     * @param {string} summary - Summarized content
     * @returns {number} - Compression ratio (0-1, lower is better compression)
     */
    calculateCompressionRatio(original, summary) {
        const originalSize = this.calculateSize(original);
        const summarySize = this.calculateSize(summary);
        
        if (originalSize === 0) {
            return 1;
        }
        
        return summarySize / originalSize;
    }
    
    /**
     * Processes multiple tool results in parallel
     * @param {Array} toolResults - Array of tool results to potentially summarize
     * @param {Object} context - Context for summarization
     * @returns {Promise<Map>} - Map of toolCallId to summary result
     */
    async summarizeMultipleTools(toolResults, context) {
        if (!Array.isArray(toolResults)) {
            throw new Error('[ToolSummarizer] toolResults must be an array');
        }
        
        // Filter tools that need summarization
        const toSummarize = toolResults.filter(tr => this.shouldSummarize(tr));
        
        if (toSummarize.length === 0) {
            return new Map();
        }
        
        // console.log(`[ToolSummarizer] Summarizing ${toSummarize.length} large tool responses`);
        
        // Process in parallel with concurrency limit
        const concurrencyLimit = 3;
        const results = new Map();
        
        for (let i = 0; i < toSummarize.length; i += concurrencyLimit) {
            const batch = toSummarize.slice(i, i + concurrencyLimit);
            
            // eslint-disable-next-line no-await-in-loop
            const batchResults = await Promise.all(
                batch.map(async (toolResult) => {
                    try {
                        const summary = await this.summarizeToolResult({
                            toolResult,
                            toolName: toolResult.toolName,
                            toolSchema: context.toolSchemas.get(toolResult.toolName) || {},
                            userQuestion: context.userQuestion,
                            assistantReasoning: context.assistantReasoning,
                            providerInfo: context.providerInfo
                        });
                        
                        return {
                            toolCallId: toolResult.toolCallId,
                            summary
                        };
                    } catch (error) {
                        console.error(`[ToolSummarizer] Failed to summarize tool ${toolResult.toolName}:`, error);
                        return null;
                    }
                })
            );
            
            // Store successful results
            batchResults.filter(r => r !== null).forEach(result => {
                results.set(result.toolCallId, result.summary);
            });
        }
        
        return results;
    }
}

/**
 * Factory function to create ToolSummarizer instance
 * @param {Object} config - Configuration object
 * @returns {ToolSummarizer} - New instance
 */
export function createToolSummarizer(config) {
    return new ToolSummarizer(config);
}