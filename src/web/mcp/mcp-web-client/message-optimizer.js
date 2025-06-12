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

export class MessageOptimizer {
    constructor(settings) {
        // STRICT: Validate settings structure
        if (!settings || typeof settings !== 'object') {
            throw new Error('[MessageOptimizer] settings must be a valid object');
        }
        
        this.settings = this.validateSettings(settings);
        this.conclusionDetector = new ConclusionDetector();
        
        // Initialize tool summarizer if enabled
        this.toolSummarizer = null;
        if (this.settings.toolSummarization.enabled) {
            if (!settings.llmProviderFactory) {
                throw new Error('[MessageOptimizer] llmProviderFactory required when tool summarization is enabled');
            }
            
            this.toolSummarizer = new ToolSummarizer({
                llmProviderFactory: settings.llmProviderFactory,
                primaryModel: this.settings.primaryModel,
                secondaryModel: this.settings.secondaryModel,
                threshold: this.settings.toolSummarization.threshold,
                useSecondaryModel: this.settings.toolSummarization.useSecondaryModel
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
            primaryModel: null,
            secondaryModel: null,
            toolSummarization: {
                enabled: false,
                threshold: 50000,
                useSecondaryModel: true
            },
            toolMemory: {
                enabled: false,
                forgetAfterConclusions: 1
            },
            cacheControl: {
                enabled: false,
                strategy: 'smart'
            },
            autoSummarization: {
                enabled: false,
                triggerPercent: 50,
                useSecondaryModel: true
            }
        };

        // Deep merge with validation
        const validated = { ...defaults };
        
        if (settings.primaryModel !== undefined) {
            if (typeof settings.primaryModel !== 'string') {
                throw new Error('[MessageOptimizer] primaryModel must be a string');
            }
            validated.primaryModel = settings.primaryModel;
        }
        
        if (settings.secondaryModel !== undefined) {
            if (settings.secondaryModel !== null && typeof settings.secondaryModel !== 'string') {
                throw new Error('[MessageOptimizer] secondaryModel must be a string or null');
            }
            validated.secondaryModel = settings.secondaryModel;
        }

        // Validate tool summarization settings
        if (settings.toolSummarization) {
            const ts = settings.toolSummarization;
            if (ts.enabled !== undefined && typeof ts.enabled !== 'boolean') {
                throw new Error('[MessageOptimizer] toolSummarization.enabled must be boolean');
            }
            if (ts.threshold !== undefined) {
                if (typeof ts.threshold !== 'number' || ts.threshold < 0) {
                    throw new Error('[MessageOptimizer] toolSummarization.threshold must be positive number');
                }
            }
            Object.assign(validated.toolSummarization, ts);
        }

        // Validate tool memory settings
        if (settings.toolMemory) {
            const tm = settings.toolMemory;
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
            Object.assign(validated.toolMemory, tm);
        }

        // Validate cache control settings
        if (settings.cacheControl) {
            const cc = settings.cacheControl;
            if (cc.enabled !== undefined && typeof cc.enabled !== 'boolean') {
                throw new Error('[MessageOptimizer] cacheControl.enabled must be boolean');
            }
            if (cc.strategy !== undefined) {
                const validStrategies = ['aggressive', 'smart', 'minimal'];
                if (!validStrategies.includes(cc.strategy)) {
                    throw new Error(`[MessageOptimizer] cacheControl.strategy must be one of: ${validStrategies.join(', ')}`);
                }
            }
            Object.assign(validated.cacheControl, cc);
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
    buildMessagesForAPI(chat, freezeCache = false) {
        // STRICT: Validate input parameters
        if (!chat || typeof chat !== 'object') {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] chat must be a valid object');
        }
        
        if (!Array.isArray(chat.messages)) {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] chat.messages must be an array');
        }
        
        if (chat.messages.length === 0) {
            throw new Error('[MessageOptimizer.buildMessagesForAPI] chat.messages cannot be empty');
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

        const stats = {
            originalMessages: chat.messages.length,
            optimizedMessages: 0,
            toolsFiltered: 0,
            toolsSummarized: 0,
            messagesSummarized: 0,
            cacheStrategy: this.settings.cacheControl.strategy
        };

        try {
            // Step 1: Find starting point (after any summary checkpoint)
            const { startIndex, summaryMessage } = this.findSummaryCheckpoint(chat.messages);
            // console.log(`[MessageOptimizer] Found summary checkpoint, starting from index ${startIndex}`);

            // Step 2: Initialize messages array with system prompt
            const messages = this.initializeMessagesArray(chat.messages, summaryMessage);

            // Step 3: Track assistant state for smart filtering
            const assistantTracker = new AssistantStateTracker();

            // Step 4: Process each message from checkpoint onwards
            for (let i = startIndex; i < chat.messages.length; i++) {
                const msg = chat.messages[i];
                
                // Validate each message structure
                this.validateMessage(msg, i);
                
                // Skip UI-only messages
                if (this.shouldSkipMessage(msg)) {
                    // console.log(`[MessageOptimizer] Skipping message ${i}: ${msg.role || msg.type}`);
                    continue;
                }

                // Update assistant state tracking
                assistantTracker.updateState(msg);

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
            
            // console.log(`[MessageOptimizer] Optimization complete:`, stats);

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
     * @param {Array} allMessages - All chat messages
     * @param {Object|null} summaryMessage - Summary message if found
     * @returns {Array} - Initial messages array
     */
    initializeMessagesArray(allMessages, summaryMessage) {
        const messages = [];
        
        // Always include system prompt if it exists
        if (allMessages.length > 0 && allMessages[0].role === 'system') {
            messages.push(allMessages[0]);
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
        
        if (!msg.role && !msg.type) {
            throw new Error(`[MessageOptimizer] Message at index ${index} missing both role and type`);
        }
        
        // Additional validation based on role
        const messageType = msg.type || msg.role;
        
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
        
        return skipRoles.includes(msg.role) || skipRoles.includes(msg.type);
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
        const messageType = msg.type || msg.role;
        
        switch (messageType) {
            case 'system':
                // Skip system message if it's the first message (already added)
                return index === 0 ? null : msg;
                
            case 'user':
                // Reset assistant state on new user input
                tracker.reset();
                return msg;
                
            case 'assistant':
                return msg;
                
            case 'tool-results':
                return this.processToolResults(msg, tracker, stats);
                
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
     * Processes tool results based on optimization settings
     * @param {Object} msg - Tool results message
     * @param {AssistantStateTracker} tracker - State tracker
     * @param {Object} stats - Statistics object
     * @returns {Object|null} - Processed message or null if filtered
     */
    processToolResults(msg, tracker, stats) {
        // Check if tools should be filtered based on assistant state
        if (this.settings.toolMemory.enabled) {
            const shouldFilter = tracker.shouldFilterTools(
                this.settings.toolMemory.forgetAfterConclusions
            );
            
            if (shouldFilter) {
                stats.toolsFiltered += msg.toolResults.length;
                // console.log(`[MessageOptimizer] Filtered ${msg.toolResults.length} tool results (assistant concluded)`);
                return null;
            }
        }

        // Check if tools should be summarized
        if (this.settings.toolSummarization.enabled) {
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
        if (!this.settings.cacheControl.enabled) {
            return -1;
        }

        if (freezeCache && lastCacheIndex !== null) {
            // console.log(`[MessageOptimizer] Using frozen cache index: ${lastCacheIndex}`);
            return lastCacheIndex;
        }

        const strategy = this.settings.cacheControl.strategy;
        // console.log(`[MessageOptimizer] Applying cache strategy: ${strategy}`);

        switch (strategy) {
            case 'aggressive':
                return Math.max(0, messages.length - 2);
                
            case 'minimal':
                return 0;
                
            case 'smart':
            default:
                // Cache up to 70% of messages, avoiding recent tool results
                const seventyPercent = Math.floor(messages.length * 0.7);
                
                for (let i = seventyPercent; i >= 0; i--) {
                    if (messages[i] && messages[i].role !== 'tool-results') {
                        return i;
                    }
                }
                return 0;
        }
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
        
        if (!this.toolSummarizer || !this.settings.toolSummarization.enabled) {
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

/**
 * Tracks assistant state for smart tool filtering
 */
class AssistantStateTracker {
    constructor() {
        this.state = 'idle';
        this.conclusionCount = 0;
        this.lastToolMessageIndex = -1;
    }

    /**
     * Updates state based on message
     * @param {Object} msg - Message to analyze
     */
    updateState(msg) {
        if (msg.role === 'assistant') {
            // Check for tool usage
            if (this.hasToolCalls(msg)) {
                this.state = 'using-tools';
                return;
            }
            
            // Check for conclusion
            if (this.isConclusion(msg)) {
                this.state = 'concluded';
                this.conclusionCount++;
                return;
            }
            
            this.state = 'thinking';
        } else if (msg.role === 'tool-results') {
            this.lastToolMessageIndex = this.conclusionCount;
        } else if (msg.role === 'user') {
            this.reset();
        }
    }

    /**
     * Checks if tools should be filtered based on conclusion count
     * @param {number} threshold - Number of conclusions after which to filter
     * @returns {boolean} - True if tools should be filtered
     */
    shouldFilterTools(threshold) {
        return this.state === 'concluded' && 
               (this.conclusionCount - this.lastToolMessageIndex) > threshold;
    }

    /**
     * Resets state (on new user message)
     */
    reset() {
        this.state = 'idle';
        // Don't reset conclusion count - it's cumulative
    }

    /**
     * Checks if message contains tool calls
     * @param {Object} msg - Assistant message
     * @returns {boolean} - True if message has tool calls
     */
    hasToolCalls(msg) {
        if (Array.isArray(msg.content)) {
            return msg.content.some(block => block.type === 'tool_use');
        }
        return false;
    }

    /**
     * Checks if assistant message indicates conclusion
     * @param {Object} msg - Assistant message
     * @returns {boolean} - True if message indicates conclusion
     */
    isConclusion(msg) {
        const text = this.extractText(msg).toLowerCase();
        
        const conclusionPhrases = [
            'done', 'completed', 'finished',
            'here is', 'here\'s', 'here are',
            'successfully', 'accomplished'
        ];
        
        const workingPhrases = [
            'let me', 'I\'ll', 'I will',
            'checking', 'looking', 'analyzing'
        ];
        
        let score = 0;
        conclusionPhrases.forEach(phrase => {
            if (text.includes(phrase)) score += 2;
        });
        
        workingPhrases.forEach(phrase => {
            if (text.includes(phrase)) score -= 1;
        });
        
        return score > 0;
    }

    /**
     * Extracts text content from message
     * @param {Object} msg - Message
     * @returns {string} - Extracted text
     */
    extractText(msg) {
        if (typeof msg.content === 'string') {
            return msg.content;
        }
        
        if (Array.isArray(msg.content)) {
            return msg.content
                .filter(block => block.type === 'text')
                .map(block => block.text)
                .join(' ');
        }
        
        return '';
    }
}

/**
 * Detects when assistant has concluded vs still working
 * @deprecated - Use AssistantStateTracker instead
 */
class ConclusionDetector {
    constructor() {
        // console.warn('[ConclusionDetector] Deprecated - use AssistantStateTracker');
    }
    
    isConclusion() {
        return false;
    }
}