/**
 * Message Optimizer Module
 * 
 * Centralizes all message filtering, transformation, and cost optimization logic
 * for preparing messages to send to LLM APIs.
 * 
 * STRICT MODE: All parameters are validated. Invalid inputs throw errors.
 * No backwards compatibility. Clean interfaces only.
 */

import { ToolSummarizer } from './tool-summarizer.js';
import * as SystemMsg from './system-msg.js';

class AssistantStateTracker {
    constructor() {
        this.currentTurn = 0;
        this.messageToTurn = new Map(); // Maps message index to turn number
        this.hasToolsInCurrentTurn = false;
    }

    updateState(msg, index) {
        // Consistently get the message type - messages use either 'role' or 'type'
        const messageType = msg.role;
        
        if (messageType === 'assistant') {
            if (this.hasToolCalls(msg)) {
                this.hasToolsInCurrentTurn = true;
                this.messageToTurn.set(index, this.currentTurn);
                return;
            }
            
            // Assistant message without tool calls = conclusion
            this.messageToTurn.set(index, this.currentTurn);
            // Only increment turn if we had tools in this turn
            if (this.hasToolsInCurrentTurn) {
                this.currentTurn++;
                this.hasToolsInCurrentTurn = false;
            }
        } else if (messageType === 'tool-results') {
            // Tool results belong to the current turn
            this.messageToTurn.set(index, this.currentTurn);
        } else if (messageType === 'user') {
            // User message doesn't reset turns, just marks that we're no longer in a tool-using state
            this.hasToolsInCurrentTurn = false;
            this.messageToTurn.set(index, this.currentTurn);
        } else {
            // Other messages belong to current turn
            this.messageToTurn.set(index, this.currentTurn);
        }
    }

    shouldFilterTools(messageIndex, threshold) {
        const messageTurn = this.messageToTurn.get(messageIndex) || 0;
        // Get the turn of the last message to know what turn we're actually in
        const lastMessageTurn = Math.max(...Array.from(this.messageToTurn.values()), 0);
        const turnDifference = lastMessageTurn - messageTurn;
        return turnDifference > threshold;
    }

    getMessageTurn(messageIndex) {
        return this.messageToTurn.get(messageIndex) || 0;
    }

    reset() {
        // Reset for new conversation
        this.currentTurn = 0;
        this.messageToTurn.clear();
        this.hasToolsInCurrentTurn = false;
    }

    hasToolCalls(msg) {
        if (Array.isArray(msg.content)) {
            return msg.content.some(block => block.type === 'tool_use');
        }
        return false;
    }

    isConclusion(msg) {
        // A conclusion is simply an assistant message without tool calls
        return !this.hasToolCalls(msg);
    }

}

export class MessageOptimizer {
    constructor(settings) {
        // STRICT: Validate settings structure
        if (!settings || typeof settings !== 'object') {
            throw new Error('[MessageOptimizer] settings must be a valid object');
        }
        
        this.settings = this.validateSettings(settings);
        
        // Initialize tool summarizer if enabled
        this.toolSummarizer = null;
        if (this.settings.optimisation.toolSummarisation.enabled) {
            if (!settings.llmProviderFactory) {
                throw new Error('[MessageOptimizer] llmProviderFactory required when tool summarisation is enabled');
            }
            
            // Convert model format for tool summarizer
            const primaryModel = `${this.settings.model.provider}:${this.settings.model.id}`;
            const summaryModel = this.settings.optimisation.toolSummarisation.model;
            const secondaryModel = summaryModel ? `${summaryModel.provider}:${summaryModel.id}` : null;
            
            this.toolSummarizer = new ToolSummarizer({
                llmProviderFactory: settings.llmProviderFactory,
                primaryModel,
                secondaryModel,
                threshold: this.settings.optimisation.toolSummarisation.thresholdKiB * 1024, // Convert KiB to bytes
                useSecondaryModel: !!secondaryModel
            });
        }
        
        // console.log('[MessageOptimizer] Initialized with settings:', this.settings);
    }

    /**
     * Validates and normalizes settings structure
     * @param {Object} settings - Raw settings object
     * @returns {Object} - Validated settings with defaults
     * @throws {Error} - If settings are invalid
     */
    validateSettings(settings) {
        const defaults = {
            model: {
                provider: null,
                id: null,
                params: {
                    temperature: 0.7,
                    topP: 0.9,
                    maxTokens: 4096,
                    seed: {
                        enabled: false,
                        value: Math.floor(Math.random() * 1000000)
                    }
                }
            },
            optimisation: {
                toolSummarisation: {
                    enabled: false,
                    thresholdKiB: 20,
                    model: null
                },
                autoSummarisation: {
                    enabled: false,
                    triggerPercent: 50,
                    model: null
                },
                toolMemory: {
                    enabled: false,
                    forgetAfterConclusions: 1
                },
                cacheControl: 'all-off',
                titleGeneration: {
                    enabled: true,
                    model: null
                }
            },
            mcpServer: null
        };

        // Deep merge with validation
        const validated = JSON.parse(JSON.stringify(defaults));
        
        // Validate model structure
        if (settings.model) {
            if (!settings.model.provider || !settings.model.id) {
                throw new Error('[MessageOptimizer] model must have provider and id');
            }
            validated.model = settings.model;
        } else if (!validated.model || !validated.model.provider || !validated.model.id) {
            throw new Error('[MessageOptimizer] model configuration is required with provider and id');
        }
        
        // Validate optimisation structure
        if (settings.optimisation) {
            validated.optimisation = { ...validated.optimisation, ...settings.optimisation };
        }

        // Validate optimisation settings if provided
        if (validated.optimisation) {
            const opt = validated.optimisation;
            
            // Validate tool summarisation
            if (opt.toolSummarisation) {
                const ts = opt.toolSummarisation;
                if (ts.enabled !== undefined && typeof ts.enabled !== 'boolean') {
                    throw new Error('[MessageOptimizer] toolSummarisation.enabled must be boolean');
                }
                if (ts.thresholdKiB !== undefined && (typeof ts.thresholdKiB !== 'number' || ts.thresholdKiB < 0)) {
                    throw new Error('[MessageOptimizer] toolSummarisation.thresholdKiB must be positive number');
                }
                if (ts.model !== undefined && ts.model !== null && (!ts.model.provider || !ts.model.id)) {
                    throw new Error('[MessageOptimizer] toolSummarisation.model must have provider and id');
                }
            }

            // Validate auto summarisation
            if (opt.autoSummarisation) {
                const as = opt.autoSummarisation;
                if (as.enabled !== undefined && typeof as.enabled !== 'boolean') {
                    throw new Error('[MessageOptimizer] autoSummarisation.enabled must be boolean');
                }
                if (as.triggerPercent !== undefined && (typeof as.triggerPercent !== 'number' || as.triggerPercent < 0 || as.triggerPercent > 100)) {
                    throw new Error('[MessageOptimizer] autoSummarisation.triggerPercent must be 0-100');
                }
                if (as.model !== undefined && as.model !== null && (!as.model.provider || !as.model.id)) {
                    throw new Error('[MessageOptimizer] autoSummarisation.model must have provider and id');
                }
            }

            // Validate tool memory settings
            if (opt.toolMemory) {
                const tm = opt.toolMemory;
                if (tm.enabled !== undefined && typeof tm.enabled !== 'boolean') {
                    throw new Error('[MessageOptimizer] toolMemory.enabled must be boolean');
                }
                if (tm.forgetAfterConclusions !== undefined) {
                    if (typeof tm.forgetAfterConclusions !== 'number' || 
                        tm.forgetAfterConclusions < 0 || 
                        tm.forgetAfterConclusions > 5) {
                        throw new Error('[MessageOptimizer] toolMemory.forgetAfterConclusions must be 0-5');
                    }
                }
            }

            // Validate cache control settings
            if (opt.cacheControl !== undefined) {
                if (typeof opt.cacheControl !== 'string') {
                    throw new Error('[MessageOptimizer] cacheControl must be a string');
                }
                const validModes = ['all-off', 'system', 'cached'];
                if (!validModes.includes(opt.cacheControl)) {
                    throw new Error(`[MessageOptimizer] cacheControl must be one of: ${validModes.join(', ')}`);
                }
            }

            // Validate title generation settings
            if (opt.titleGeneration) {
                const tg = opt.titleGeneration;
                if (tg.enabled !== undefined && typeof tg.enabled !== 'boolean') {
                    throw new Error('[MessageOptimizer] titleGeneration.enabled must be boolean');
                }
                if (tg.model !== undefined && tg.model !== null && (!tg.model.provider || !tg.model.id)) {
                    throw new Error('[MessageOptimizer] titleGeneration.model must have provider and id');
                }
            }
        }

        return validated;
    }

    /**
     * Main entry point - builds optimized message array for API
     * @param {Object} chat - The chat object containing all messages
     * @param {boolean} freezeCache - Whether to freeze cache control position
     * @returns {Object} - { messages, cacheControlIndex, toolInclusionMode, stats }
     * @throws {Error} - If chat is invalid or processing fails
     */
    buildMessagesForAPI(chat, freezeCache = false, mcpInstructions = null) {
        // STRICT: Validate input parameters
        if (!chat || typeof chat !== 'object') {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] chat must be a valid object');
        }
        
        if (!Array.isArray(chat.messages)) {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] chat.messages must be an array');
        }
        
        // Validate all messages have basic structure before processing
        for (let i = 0; i < chat.messages.length; i++) {
            if (!chat.messages[i] || typeof chat.messages[i] !== 'object') {
                throw new Error(`[MessageOptimizer.buildMessagesForAPI] Message at index ${i} is not a valid object`);
            }
        }
        
        if (typeof freezeCache !== 'boolean') {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] freezeCache must be boolean');
        }

        // console.log(`[MessageOptimizer.buildMessagesForAPI] Processing ${chat.messages.length} messages for chat ${chat.id || 'unknown'}`);
        // console.log(`[MessageOptimizer.buildMessagesForAPI] Tool Memory Settings:`, this.settings.optimisation.toolMemory);

        const stats = {
            originalMessages: chat.messages.length,
            optimizedMessages: 0,
            toolsFiltered: 0,
            toolsSummarized: 0,
            messagesSummarized: 0,
            cacheMode: this.settings.optimisation.cacheControl
        };

        try {
            // Step 1: Find starting point (after any summary checkpoint)
            const { startIndex, summaryMessage } = this.findSummaryCheckpoint(chat.messages);
            // console.log(`[MessageOptimizer] Found summary checkpoint, starting from index ${startIndex}`);

            // Step 2: Initialize messages array with system prompt
            const messages = this.initializeMessagesArray(chat, summaryMessage, mcpInstructions);

            // Step 3: Track assistant state for smart filtering
            const assistantTracker = new AssistantStateTracker();

            // Step 4: First pass - build turn map
            if (this.settings.optimisation.toolMemory.enabled) {
                // console.log('[MessageOptimizer] Tool Memory is ENABLED with forgetAfterConclusions:', this.settings.optimisation.toolMemory.forgetAfterConclusions);
                for (let i = startIndex; i < chat.messages.length; i++) {
                    const msg = chat.messages[i];
                    if (!this.shouldSkipMessage(msg)) {
                        assistantTracker.updateState(msg, i);
                    }
                }
            }

            // Step 5: Second pass - process messages with filtering
            for (let i = startIndex; i < chat.messages.length; i++) {
                const msg = chat.messages[i];
                
                // Validate each message structure
                this.validateMessage(msg, i);
                
                // Skip UI-only messages
                if (this.shouldSkipMessage(msg)) {
                    continue;
                }

                // Process based on message type with strict validation
                const processedMsg = this.processMessage(
                    msg, 
                    i, 
                    assistantTracker, 
                    stats
                );
                
                if (processedMsg) {
                    messages.push(processedMsg);
                }
            }

            // Step 5: Apply cache control strategy
            const cacheControlIndex = this.determineCacheControl(
                messages, 
                freezeCache, 
                chat.lastCacheControlIndex
            );

            // Step 6: Final validation
            this.validateFinalMessages(messages);

            stats.optimizedMessages = messages.length;
            
            

            return {
                messages,
                cacheControlIndex,
                toolInclusionMode: chat.toolInclusionMode || 'auto',
                currentTurn: chat.currentTurn || 0,
                stats
            };

        } catch (error) {
            console.error('[MessageOptimizer.buildMessagesForAPI] Processing failed:', error);
            throw error;
        }
    }

    /**
     * Finds the last summary checkpoint in messages
     * @param {Array} messages - Array of chat messages
     * @returns {Object} - { startIndex, summaryMessage }
     */
    findSummaryCheckpoint(messages) {
        for (let i = messages.length - 1; i >= 0; i--) {
            if (messages[i] && messages[i].role === 'summary') {
                return {
                    startIndex: i + 1,
                    summaryMessage: messages[i]
                };
            }
        }
        
        return { startIndex: 0, summaryMessage: null };
    }

    /**
     * Initializes the messages array with system prompt and summary
     * @param {Object} chat - The chat object containing messages and systemPrompt
     * @param {Object|null} summaryMessage - Summary message if found
     * @param {string|null} mcpInstructions - MCP server instructions to append
     * @returns {Array} - Initial messages array
     */
    initializeMessagesArray(chat, summaryMessage, mcpInstructions = null) {
        const messages = [];
        
        // Always include system prompt, either from messages array or from chat.systemPrompt
        let systemMessage = null;
        
        // First check if there's a system message in the messages array
        if (chat.messages.length > 0 && chat.messages[0].role === 'system') {
            systemMessage = chat.messages[0];
        } 
        // Otherwise use the chat's systemPrompt property
        else if (chat.systemPrompt) {
            systemMessage = {
                role: 'system',
                content: chat.systemPrompt
            };
        }
        
        // If we found a system message, enhance it with MCP instructions
        if (systemMessage) {
            const enhancedSystemMessage = SystemMsg.enhanceSystemMessageWithMcp(
                systemMessage, 
                mcpInstructions
            );
            messages.push(enhancedSystemMessage);
        }
        
        // Include summary if found
        if (summaryMessage) {
            messages.push(summaryMessage);
        }
        
        return messages;
    }

    /**
     * Validates individual message structure
     * @param {Object} msg - Message to validate
     * @param {number} index - Message index for error reporting
     * @throws {Error} - If message is invalid
     */
    validateMessage(msg, index) {
        if (!msg || typeof msg !== 'object') {
            throw new Error(`[MessageOptimizer] Message at index ${index} is not a valid object`);
        }
        
        if (!msg.role) {
            throw new Error(`[MessageOptimizer] Message at index ${index} missing role`);
        }
        
        // Additional validation based on role
        const messageType = msg.role;
        
        switch (messageType) {
            case 'tool-results':
                if (!Array.isArray(msg.toolResults)) {
                    throw new Error(`[MessageOptimizer] tool-results message at index ${index} missing toolResults array`);
                }
                break;
                
            case 'assistant':
            case 'user':
                if (msg.content === undefined) {
                    throw new Error(`[MessageOptimizer] ${messageType} message at index ${index} missing content`);
                }
                break;
                
            default:
                // Other message types don't require specific validation
                break;
        }
    }

    /**
     * Determines if a message should be skipped entirely
     * @param {Object} msg - Message to check
     * @returns {boolean} - True if message should be skipped
     */
    shouldSkipMessage(msg) {
        const skipRoles = [
            'system-title', 
            'system-summary', 
            'title', 
            'summary', 
            'accounting', 
            'error',
            'tool-summary-request'
        ];
        
        const messageType = msg.role;
        return skipRoles.includes(messageType);
    }

    /**
     * Processes a single message based on optimization settings
     * @param {Object} msg - Message to process
     * @param {number} index - Message index
     * @param {AssistantStateTracker} tracker - State tracker
     * @param {Object} stats - Statistics object to update
     * @returns {Object|null} - Processed message or null if filtered
     */
    processMessage(msg, index, tracker, stats) {
        const messageType = msg.role;
        
        switch (messageType) {
            case 'system':
                // Skip system message if it's the first message (already added)
                return index === 0 ? null : msg;
                
            case 'user':
                return msg;
                
            case 'assistant':
                return this.processAssistantMessage(msg, index, tracker, stats);
                
            case 'tool-results':
                return this.processToolResults(msg, index, tracker, stats);
                
            case 'tool-summary':
                // This will be handled by transforming existing tool-results
                // For now, skip - transformation happens elsewhere
                return null;
                
            default:
                console.warn(`[MessageOptimizer] Unknown message type: ${messageType} at index ${index}`);
                return msg;
        }
    }

    /**
     * Processes assistant messages, potentially filtering tool calls
     * @param {Object} msg - Assistant message
     * @param {number} index - Message index
     * @param {AssistantStateTracker} tracker - State tracker
     * @param {Object} stats - Statistics object
     * @returns {Object} - Processed message
     */
    processAssistantMessage(msg, index, tracker, _stats) {
        // If tool memory is not enabled, return as-is
        if (!this.settings.optimisation.toolMemory.enabled) {
            return msg;
        }
        
        // If message has no tool calls, return as-is
        if (!tracker.hasToolCalls(msg)) {
            return msg;
        }
        
        // Check if we should filter the tool calls based on turn age
        const shouldFilter = tracker.shouldFilterTools(
            index,
            this.settings.optimisation.toolMemory.forgetAfterConclusions
        );
        
        if (!shouldFilter) {
            return msg;
        }
        
        // Filter out tool_use blocks from content
        if (Array.isArray(msg.content)) {
            const filteredContent = msg.content.filter(block => block.type !== 'tool_use');
            
            // If all content was tool calls, return null to skip the message entirely
            if (filteredContent.length === 0) {
                return null;
            }
            
            return {
                ...msg,
                content: filteredContent
            };
        }
        
        return msg;
    }

    /**
     * Processes tool results based on optimization settings
     * @param {Object} msg - Tool results message
     * @param {number} index - Message index
     * @param {AssistantStateTracker} tracker - State tracker
     * @param {Object} stats - Statistics object
     * @returns {Object|null} - Processed message or null if filtered
     */
    processToolResults(msg, index, tracker, stats) {
        // Check if tools should be filtered based on assistant state
        if (this.settings.optimisation.toolMemory.enabled) {
            const shouldFilter = tracker.shouldFilterTools(
                index,
                this.settings.optimisation.toolMemory.forgetAfterConclusions
            );
            
            if (shouldFilter) {
                stats.toolsFiltered += 1; // Count filtered tool result messages, not their internals
                return null;
            }
        }

        // Check if tools should be summarized
        if (this.settings.optimisation.toolSummarisation.enabled) {
            return this.maybeSummarizeToolResults(msg, stats);
        }

        return msg;
    }

    /**
     * Checks if tool results should be summarized and applies summarization
     * @param {Object} msg - Tool results message
     * @param {Object} stats - Statistics object
     * @returns {Object} - Original or modified message
     */
    maybeSummarizeToolResults(msg, stats) {
        if (!this.toolSummarizer) {
            // Tool summarization not enabled
            return msg;
        }
        
        // Check each tool result
        const toolResults = msg.toolResults || [];
        let anySummarized = false;
        
        const processedResults = toolResults.map(result => {
            if (this.toolSummarizer.shouldSummarize(result)) {
                stats.toolsSummarized++;
                anySummarized = true;
                
                // Mark this result for summarization
                return {
                    ...result,
                    _needsSummarization: true,
                    _originalSize: this.toolSummarizer.calculateSize(result.result)
                };
            }
            return result;
        });
        
        if (!anySummarized) {
            return msg;
        }
        
        // Return message with marked results
        // Actual summarization will happen asynchronously
        return {
            ...msg,
            toolResults: processedResults,
            _hasPendingSummarization: true
        };
    }

    /**
     * Determines optimal cache control placement
     * @param {Array} messages - Messages array
     * @param {boolean} freezeCache - Whether to freeze cache position
     * @param {number|null} lastCacheIndex - Previous cache index
     * @returns {number} - Cache control index (-1 for no cache)
     */
    determineCacheControl(messages, freezeCache, lastCacheIndex) {
        const cacheMode = this.settings.optimisation.cacheControl;
        
        // Handle different cache control modes
        switch (cacheMode) {
            case 'all-off':
                return -1; // No cache control
                
            case 'system':
                return -1; // System prompt caching handled by provider, no message-level cache
                
            case 'cached':
                // Use smart strategy logic for message-level caching
                // System prompt caching is handled by provider
                break;
                
            default:
                console.warn('[MessageOptimizer] Unknown cache control mode:', cacheMode);
                return -1;
        }

        // Only 'cached' mode continues here - apply smart strategy
        if (freezeCache && lastCacheIndex !== null) {
            // console.log(`[MessageOptimizer] Using frozen cache index: ${lastCacheIndex}`);
            return lastCacheIndex;
        }

        // Apply smart strategy for 'cached' mode
        // Cache up to 70% of messages, avoiding recent tool results
        const seventyPercent = Math.floor(messages.length * 0.7);
        
        for (let i = seventyPercent; i >= 0; i--) {
            if (messages[i] && messages[i].role !== 'tool-results') {
                return i;
            }
        }
        return 0;
    }

    /**
     * Validates the final messages array before returning
     * @param {Array} messages - Final messages array
     * @throws {Error} - If messages are invalid
     */
    validateFinalMessages(messages) {
        if (!Array.isArray(messages)) {
            throw new Error('[MessageOptimizer] Final messages must be an array');
        }
        
        if (messages.length === 0) {
            throw new Error('[MessageOptimizer] Final messages array cannot be empty');
        }
        
        // Ensure first message is system if present
        if (messages[0] && messages[0].role !== 'system') {
            // console.warn('[MessageOptimizer] First message is not system prompt');
        }
        
        // console.log(`[MessageOptimizer] Final validation passed: ${messages.length} messages ready for API`);
    }
    
    /**
     * Performs async tool summarization for messages that need it
     * @param {Array} messages - Messages array with marked tool results
     * @param {Object} context - Context for summarization
     * @returns {Promise<Array>} - Messages with summarized tool results
     */
    async performToolSummarization(messages, context) {
        // STRICT: Validate inputs
        if (!Array.isArray(messages)) {
            throw new Error('[MessageOptimizer.performToolSummarization] messages must be an array');
        }
        
        if (!context || typeof context !== 'object') {
            throw new Error('[MessageOptimizer.performToolSummarization] context must be a valid object');
        }
        
        if (!this.toolSummarizer || !this.settings.optimisation.toolSummarisation.enabled) {
            // Tool summarization not enabled, return messages as-is
            return messages;
        }
        
        // Find messages with pending summarization
        const messagesToProcess = [];
        messages.forEach((msg, index) => {
            if (msg._hasPendingSummarization && msg.toolResults) {
                messagesToProcess.push({ message: msg, index });
            }
        });
        
        if (messagesToProcess.length === 0) {
            return messages;
        }
        
        // console.log(`[MessageOptimizer] Performing tool summarization for ${messagesToProcess.length} messages`);
        
        // Process each message with tool results
        const updatedMessages = [...messages];
        
        for (const { message, index } of messagesToProcess) {
            try {
                // Get context for this message
                const userQuestion = this.findPrecedingUserQuestion(messages, index);
                const assistantReasoning = this.findAssistantReasoning(messages, index);
                
                // Prepare summarization context
                const summaryContext = {
                    ...context,
                    userQuestion,
                    assistantReasoning
                };
                
                // Get summaries for all large tool results
                const toolsToSummarize = message.toolResults.filter(r => r._needsSummarization);
                // eslint-disable-next-line no-await-in-loop
                const summaries = await this.toolSummarizer.summarizeMultipleTools(
                    toolsToSummarize,
                    summaryContext
                );
                
                // Apply summaries to tool results
                const updatedToolResults = message.toolResults.map(result => {
                    if (result._needsSummarization && summaries.has(result.toolCallId)) {
                        const summary = summaries.get(result.toolCallId);
                        
                        // Create summarized version
                        return {
                            toolCallId: result.toolCallId,
                            toolName: result.toolName,
                            result: {
                                _type: 'summarized',
                                summary: summary.summary,
                                originalSize: summary.originalSize,
                                summarizedSize: summary.summarizedSize,
                                compressionRatio: summary.compressionRatio,
                                model: summary.model
                            }
                        };
                    }
                    
                    // Remove internal flags from non-summarized results
                    const { _needsSummarization, _originalSize, ...cleanResult } = result;
                    return cleanResult;
                });
                
                // Update message
                const { _hasPendingSummarization, ...cleanMessage } = message;
                updatedMessages[index] = {
                    ...cleanMessage,
                    toolResults: updatedToolResults
                };
                
            } catch (error) {
                console.error(`[MessageOptimizer] Failed to summarize tools at index ${index}:`, error);
                // Keep original message on error
            }
        }
        
        return updatedMessages;
    }
    
    /**
     * Finds the preceding user question for context
     * @param {Array} messages - All messages
     * @param {number} toolResultIndex - Index of tool result message
     * @returns {string} - User question or default
     */
    findPrecedingUserQuestion(messages, toolResultIndex) {
        // Search backwards for the most recent user message
        for (let i = toolResultIndex - 1; i >= 0; i--) {
            if (messages[i].role === 'user' && messages[i].content) {
                return messages[i].content;
            }
        }
        return 'No specific question provided';
    }
    
    /**
     * Finds the assistant's reasoning before tool calls
     * @param {Array} messages - All messages  
     * @param {number} toolResultIndex - Index of tool result message
     * @returns {string} - Assistant reasoning or default
     */
    findAssistantReasoning(messages, toolResultIndex) {
        // Tool results should immediately follow assistant message with tool calls
        if (toolResultIndex > 0) {
            const prevMsg = messages[toolResultIndex - 1];
            if (prevMsg.role === 'assistant' && prevMsg.content) {
                return prevMsg.content;
            }
        }
        return 'Assistant decided to use tools';
    }
}


