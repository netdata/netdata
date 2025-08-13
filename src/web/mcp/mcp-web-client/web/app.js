/**
 * Main application logic for the Netdata MCP LLM Client
 */

import {MessageOptimizer} from './message-optimizer.js';
import * as ChatConfig from './chat-config.js';
import * as TitleGenerator from './title.js';
import * as SystemMsg from './system-msg.js';
import {SafetyChecker, SafetyLimitError, SAFETY_LIMITS} from './safety-limits.js';

class NetdataMCPChat {
    constructor() {
        // Log version on startup
        console.log('🚀 Netdata MCP Web Client v1.0.67 - Multi-Chat Input Management Fixes');
        
        this.mcpServers = new Map(); // Multiple MCP servers
        this.mcpConnections = new Map(); // Active MCP connections
        this.llmProviders = new Map(); // Multiple LLM providers
        this.chats = new Map(); // Chat sessions
        this.communicationLog = []; // Universal log (not saved)
        this.tokenUsageHistory = new Map(); // Track token usage per chat
        this.toolInclusionStates = new Map(); // Track which tools are included/excluded per chat
        // Removed global currentContextWindow - now stored per chat
        // Removed global shouldStopProcessing - now stored per chat
        // Removed global isProcessing - now stored per chat
        this.modelPricing = {}; // Initialize model pricing storage
        this.modelLimits = {}; // Initialize model context limits storage
        this.copiedModel = null; // Track copied model for paste functionality
        
        // Safety protections
        this.safetyChecker = new SafetyChecker();
        
        // Per-chat DOM management
        this.chatContainers = new Map(); // Map of chatId -> DOM container (includes both main and sub-chats)

        // Models will be loaded dynamically from the proxy server
        // No hardcoded model list needed
        
        
        // Default system prompt
        this.defaultSystemPrompt = SystemMsg.DEFAULT_SYSTEM_PROMPT;
        
        // Load last used system prompt from localStorage or use default
        this.lastSystemPrompt = localStorage.getItem('lastSystemPrompt') || this.defaultSystemPrompt;
        
        this.initializeUI();
        
        // Delay resizable initialization to ensure DOM is ready
        setTimeout(() => {
            this.initializeResizable();
        }, 0);
        
        // Clear current chat ID to always start fresh
        localStorage.removeItem('currentChatId');
        
        // Get reference to main container
        this.chatContainersEl = document.getElementById('chatContainers');
        this.welcomeScreen = document.getElementById('welcomeScreen');
        
        // Show welcome screen initially
        if (this.welcomeScreen) {
            this.welcomeScreen.style.display = 'flex';
        }
        
        this.loadSettings();
        
        // Track if user has interacted with chat selection
        this.userHasSelectedChat = false;
        
        // Track if we have a pending new chat load
        this.pendingNewChatLoad = false;
        
        // Track if providers are loaded
        this.providersLoaded = false;
        
        // Initialize providers and then create default chat
        // First initialize LLM provider, then MCP servers (which need the provider URL)
        this.initializeDefaultLLMProvider().then(() => {
            return this.initializeDefaultMCPServers();
        }).then(async () => {
            // Mark providers as loaded
            this.providersLoaded = true;
            
            // Update chat sessions after providers are loaded
            this.updateChatSessions();
            
            // Only create a new chat if we have both MCP servers and LLM providers
            if (this.mcpServers.size > 0 && this.llmProviders.size > 0) {
                // Always create a new chat on startup
                const newChatId = await this.createDefaultChatIfNeeded();
                
                // If a new chat was created AND user hasn't selected a chat, load it
                if (newChatId && !this.userHasSelectedChat) {
                // Mark that we have a pending new chat load
                this.pendingNewChatLoad = true;
                this.pendingNewChatId = newChatId;
                
                // Give DOM time to update after chat creation
                this.pendingNewChatTimeout = setTimeout(() => {
                    // Double-check user hasn't selected a chat in the meantime
                    if (!this.userHasSelectedChat && this.pendingNewChatLoad) {
                        this.loadChat(this.pendingNewChatId);
                    }
                    // Clear the pending flag
                    this.pendingNewChatLoad = false;
                    this.pendingNewChatTimeout = null;
                    
                    // Clear the pending chat ID after a delay to ensure blocking works
                    setTimeout(() => {
                        this.pendingNewChatId = '';
                    }, 500);
                }, 100);
            }
            } // Close the if (this.mcpServers.size > 0 && this.llmProviders.size > 0)
        }).catch(error => {
            console.error('Failed to initialize providers:', error);
            // Still update chat sessions even if providers fail
            this.updateChatSessions();
        });
        
        // Add global error handlers to catch unhandled errors
        this.setupGlobalErrorHandlers();
    }
    
    setupGlobalErrorHandlers() {
        // Catch unhandled JavaScript errors
        window.addEventListener('error', (event) => {
            console.error('Unhandled JavaScript error:', event.error);
            this.showGlobalError(`JavaScript Error: ${event.error?.message || 'Unknown error'}`);
        });
        
        // Catch unhandled promise rejections
        window.addEventListener('unhandledrejection', (event) => {
            console.error('Unhandled promise rejection:', event.reason);
            this.showGlobalError(`Promise Error: ${event.reason?.message || event.reason || 'Unknown promise rejection'}`);
            // Prevent the default browser error console log
            event.preventDefault();
        });
    }
    
    /**
     * Extract tool calls from message content array
     * @param {Array|string} content - Message content (can be array of blocks or string)
     * @returns {Array} - Array of tool call objects with id, name, and arguments
     */
    extractToolsFromContent(content) {
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
     * Safe message operations that automatically persist changes
     * These methods ensure messages are never lost by auto-saving after each operation
     * 
     * IMPORTANT: Always use these methods instead of direct array manipulation
     * - addMessage() instead of messages.push()
     * - insertMessage() instead of messages.splice(index, 0, item)
     * - removeMessage() instead of messages.splice(index, count)
     * - removeLastMessage() instead of messages.pop()
     * 
     * CRITICAL ORDERING RULE: Always save messages BEFORE displaying them!
     * 1. Call addMessage() to save the message
     * 2. Call processRenderEvent() to display it
     * This ensures users never see messages that aren't persisted
     * 
     * ATOMIC OPERATIONS: Use batchMode for multi-step operations
     * this.batchMode = true;
     * try {
     *     // Multiple operations
     * } finally {
     *     this.batchMode = false;
     *     this.autoSave(chatId);
     * }
     */
    addMessage(chatId, message) {
        if (!chatId) {
            console.error('addMessage called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('addMessage: chat not found for chatId:', chatId);
            return;
        }
        
        // Calculate and add price to message if it has token usage
        if (message.usage && message.model) {
            const price = this.calculateMessagePrice(message.model, message.usage);
            if (price !== null) {
                message.price = price;
            }
        }
        
        chat.messages.push(message);
        chat.updatedAt = new Date().toISOString();
        
        // Update cumulative token pricing
        this.updateChatTokenPricing(chat);
        
        // NOTE: Sub-chat cost accumulation is now handled in processSingleLLMResponse
        // after tool-results are added to the parent chat, ensuring proper timing
        
        // Update the cumulative token display
        this.updateCumulativeTokenDisplay(chatId);
        
        this.autoSave(chatId);
    }
    
    insertMessage(chatId, index, message) {
        if (!chatId) {
            console.error('insertMessage called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('insertMessage: chat not found for chatId:', chatId);
            return;
        }
        
        chat.messages.splice(index, 0, message);
        chat.updatedAt = new Date().toISOString();
        
        // Update cumulative token pricing
        this.updateChatTokenPricing(chat);
        
        // Update the cumulative token display
        this.updateCumulativeTokenDisplay(chatId);
        
        this.autoSave(chatId);
    }
    
    removeMessage(chatId, index, count = 1) {
        if (!chatId) {
            console.error('removeMessage called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('removeMessage: chat not found for chatId:', chatId);
            return;
        }
        
        chat.messages.splice(index, count);
        chat.updatedAt = new Date().toISOString();
        
        // Update cumulative token pricing
        this.updateChatTokenPricing(chat);
        
        // Update the cumulative token display
        this.updateCumulativeTokenDisplay(chatId);
        
        this.autoSave(chatId);
    }
    
    removeLastMessage(chatId) {
        if (!chatId) {
            console.error('removeLastMessage called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat || !this.hasUserContent(chat)) {
            console.error('removeLastMessage: chat not found or no user content for chatId:', chatId);
            return;
        }
        
        // Use removeMessage API instead of direct pop()
        if (chat.messages.length > 0) {
            this.removeMessage(chatId, chat.messages.length - 1, 1);
        }
    }
    
    /**
     * Truncate messages from a specific index onwards, creating accounting records if needed
     * @param {string} chatId - The chat ID
     * @param {number} startIndex - Index from which to truncate (exclusive - messages from this index onwards are removed)
     * @param {string} reason - Reason for truncation (e.g., 'Redo from user message')
     */
    truncateMessages(chatId, startIndex, reason = 'Messages truncated') {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('truncateMessages: chat not found for chatId:', chatId);
            return;
        }
        
        // Calculate messages to discard
        const messagesToDiscard = chat.messages.length - startIndex;
        if (messagesToDiscard <= 0) {
            // Nothing to truncate
            return;
        }
        
        // Find messages that will be discarded (from startIndex onwards)
        const discardedMessages = chat.messages.slice(startIndex);
        
        // Check if any discarded messages have non-zero tokens/costs
        let hasTokens = false;
        for (const message of discardedMessages) {
            if (message.usage && message.model) {
                const usage = message.usage;
                if ((usage.promptTokens || 0) > 0 || 
                    (usage.completionTokens || 0) > 0 || 
                    (usage.cacheReadInputTokens || 0) > 0 || 
                    (usage.cacheCreationInputTokens || 0) > 0) {
                    hasTokens = true;
                    break;
                }
            } else if (message.role === 'accounting' && message.cumulativeTokens) {
                // Also check accounting nodes for tokens
                const tokens = message.cumulativeTokens;
                if ((tokens.inputTokens || 0) > 0 || 
                    (tokens.outputTokens || 0) > 0 || 
                    (tokens.cacheReadTokens || 0) > 0 || 
                    (tokens.cacheCreationTokens || 0) > 0) {
                    hasTokens = true;
                    break;
                }
            }
        }
        
        // Only create accounting nodes if there are tokens to preserve
        if (hasTokens) {
            // Group discarded tokens by model
            const tokensByModel = new Map();
            
            for (const message of discardedMessages) {
                if (message.usage && message.model) {
                    const model = message.model;
                    if (!tokensByModel.has(model)) {
                        tokensByModel.set(model, {
                            inputTokens: 0,
                            outputTokens: 0,
                            cacheReadTokens: 0,
                            cacheCreationTokens: 0,
                            messageCount: 0
                        });
                    }
                    
                    const tokens = tokensByModel.get(model);
                    tokens.inputTokens += message.usage.promptTokens || 0;
                    tokens.outputTokens += message.usage.completionTokens || 0;
                    tokens.cacheCreationTokens += message.usage.cacheCreationInputTokens || 0;
                    tokens.cacheReadTokens += message.usage.cacheReadInputTokens || 0;
                    tokens.messageCount++;
                } else if (message.role === 'accounting' && message.model && message.cumulativeTokens) {
                    // CRITICAL: Also collect tokens from accounting nodes being replaced
                    const model = message.model;
                    if (!tokensByModel.has(model)) {
                        tokensByModel.set(model, {
                            inputTokens: 0,
                            outputTokens: 0,
                            cacheReadTokens: 0,
                            cacheCreationTokens: 0,
                            messageCount: 0
                        });
                    }
                    
                    const tokens = tokensByModel.get(model);
                    const cumTokens = message.cumulativeTokens;
                    tokens.inputTokens += cumTokens.inputTokens || 0;
                    tokens.outputTokens += cumTokens.outputTokens || 0;
                    tokens.cacheCreationTokens += cumTokens.cacheCreationTokens || 0;
                    tokens.cacheReadTokens += cumTokens.cacheReadTokens || 0;
                    tokens.messageCount += message.discardedMessages || 0;
                }
            }
            
            // Create accounting nodes for each model
            let insertIndex = startIndex;
            for (const [model, tokens] of tokensByModel) {
                // Only create accounting node if this model has non-zero tokens
                if (tokens.inputTokens > 0 || tokens.outputTokens > 0 || 
                    tokens.cacheReadTokens > 0 || tokens.cacheCreationTokens > 0) {
                    const accountingNode = {
                        role: 'accounting',
                        timestamp: new Date().toISOString(),
                        model,
                        cumulativeTokens: tokens,
                        reason,
                        discardedMessages: tokens.messageCount
                    };
                    this.insertMessage(chatId, insertIndex, accountingNode);
                    insertIndex++;
                }
            }
            
            // Remove all messages after accounting nodes
            const toRemove = chat.messages.length - insertIndex;
            if (toRemove > 0) {
                this.removeMessage(chatId, insertIndex, toRemove);
            }
        } else {
            // No tokens to preserve, just remove messages
            this.removeMessage(chatId, startIndex, messagesToDiscard);
        }
        
        this.autoSave(chatId);
    }
    
    /**
     * Check if a chat has any real user content (excluding system messages)
     */
    hasUserContent(chat) {
        if (!chat || !chat.messages) {return false;}
        return chat.messages.some(m => 
            m.role !== 'system' && 
            m.role !== 'system-title' && 
            m.role !== 'system-summary' &&
            m.role !== 'title' &&
            m.role !== 'summary' &&
            m.role !== 'accounting'
        );
    }
    
    /**
     * Check if this is the first real user message in the chat
     */
    isFirstUserMessage(chat) {
        if (!chat || !chat.messages) {return false;}
        const userMessages = chat.messages.filter(m => 
            m.role === 'user'
        );
        return userMessages.length === 1;
    }
    
    /**
     * Count real assistant messages (excluding title responses)
     */
    countAssistantMessages(chat) {
        if (!chat || !chat.messages) {return 0;}
        return chat.messages.filter(m => 
            m.role === 'assistant'
        ).length;
    }
    
    /**
     * Check if a string contains markdown formatting
     */
    isMarkdownContent(content) {
        if (typeof content !== 'string' || !content.trim()) {
            return false;
        }
        
        // Common markdown patterns
        const markdownPatterns = [
            /^#+\s/m,                    // Headers: # ## ###
            /\*\*.*\*\*/,               // Bold: **text**
            /\*.*\*/,                   // Italic: *text*
            /`.*`/,                     // Inline code: `code`
            /```[\s\S]*?```/,           // Code blocks: ```code```
            /^\s*[-*+]\s/m,             // Unordered lists: - * +
            /^\s*\d+\.\s/m,             // Ordered lists: 1. 2.
            /^\s*>\s/m,                 // Blockquotes: >
            /\[.*\]\(.*\)/,             // Links: [text](url)
            /!\[.*\]\(.*\)/,            // Images: ![alt](url)
            /^\s*\|.*\|/m,              // Tables: | col1 | col2 |
            /^---+$/m,                  // Horizontal rules: ---
            /~~.*~~/,                   // Strikethrough: ~~text~~
        ];
        
        return markdownPatterns.some(pattern => pattern.test(content));
    }
    
    /**
     * Auto-save with debouncing for performance
     * Saves only the specific chat that was modified
     */
    autoSave(chatId) {
        if (!chatId) {return;}
        
        // For per-chat saves, we can be more aggressive since we're only saving one chat
        // Clear any pending save for this specific chat
        if (this.pendingSaveTimeouts) {
            if (this.pendingSaveTimeouts[chatId]) {
                clearTimeout(this.pendingSaveTimeouts[chatId]);
            }
        } else {
            this.pendingSaveTimeouts = {};
        }
        
        // Save this specific chat after a short delay
        this.pendingSaveTimeouts[chatId] = setTimeout(() => {
            this.saveChatToStorage(chatId);
            delete this.pendingSaveTimeouts[chatId];
        }, 100); // 100ms debounce
    }
    
    /**
     * Save chat configuration - only saves to chatConfig_chat_XXX for saved chats
     * For unsaved chats, only updates lastChatConfig
     */
    saveChatConfigSmart(chatId, config) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[saveChatConfigSmart] Chat not found for chatId: ${chatId}`);
            return;
        }
        
        // Always save as last config for new chats to inherit
        ChatConfig.saveLastConfig(config);
        
        // Only save chat-specific config if the chat is saved
        if (chat.isSaved !== false && chat.messages.length > 0) {
            ChatConfig.saveChatConfig(chatId, config);
        }
    }
    
    // Calculate price for a single message based on its model and usage
    calculateMessagePrice(model, usage) {
        if (!usage || !model) {return null;}
        
        // Extract model name from format "provider:model-name"
        let modelName = model;
        if (typeof model === 'string') {
            modelName = ChatConfig.getModelDisplayName(model);
        } else if (model?.id) {
            modelName = model.id;
        }
        
        const pricing = this.modelPricing[modelName];
        if (!pricing) {return null;}
        
        let totalCost = 0;
        
        const promptTokens = usage.promptTokens || 0;
        const completionTokens = usage.completionTokens || 0;
        const cacheReadTokens = usage.cacheReadInputTokens || 0;
        const cacheCreationTokens = usage.cacheCreationInputTokens || 0;
        
        // For Anthropic models with cache pricing
        if (pricing.cacheWrite !== undefined && pricing.cacheRead !== undefined) {
            totalCost += promptTokens / 1_000_000 * pricing.input;
            totalCost += cacheReadTokens / 1_000_000 * pricing.cacheRead;
            totalCost += cacheCreationTokens / 1_000_000 * pricing.cacheWrite;
            totalCost += completionTokens / 1_000_000 * pricing.output;
        }
        // For OpenAI models with cache pricing
        else if (pricing.cacheRead !== undefined) {
            const cachedInputTokens = cacheReadTokens + cacheCreationTokens;
            totalCost += promptTokens / 1_000_000 * pricing.input;
            totalCost += cachedInputTokens / 1_000_000 * pricing.cacheRead;
            totalCost += completionTokens / 1_000_000 * pricing.output;
        }
        // For models without cache pricing
        else {
            const allInputTokens = promptTokens + cacheReadTokens + cacheCreationTokens;
            totalCost += allInputTokens / 1_000_000 * pricing.input;
            totalCost += completionTokens / 1_000_000 * pricing.output;
        }
        
        return totalCost;
    }

    // Update the chat's cumulative token pricing
    updateChatTokenPricing(chat) {
        if (!chat) {
            console.error('updateChatTokenPricing called without chat object');
            return;
        }
        
        // Initialize if not present
        if (!chat.totalTokensPrice) {
            chat.totalTokensPrice = {
                input: 0,
                output: 0,
                cacheRead: 0,
                cacheCreation: 0,
                totalCost: 0
            };
        }
        
        if (!chat.perModelTokensPrice) {
            chat.perModelTokensPrice = {};
        }

        // Reset totals
        chat.totalTokensPrice = {
            input: 0,
            output: 0,
            cacheRead: 0,
            cacheCreation: 0,
            totalCost: 0
        };
        chat.perModelTokensPrice = {};
        
        // Calculate from all messages
        for (const message of chat.messages) {
            if (message.usage) {
                const model = message.model || ChatConfig.getChatModelString(chat); // Fallback to chat model for old messages
                if (!model) {continue;}
                
                // Update total tokens
                chat.totalTokensPrice.input += message.usage.promptTokens || 0;
                chat.totalTokensPrice.output += message.usage.completionTokens || 0;
                chat.totalTokensPrice.cacheRead += message.usage.cacheReadInputTokens || 0;
                chat.totalTokensPrice.cacheCreation += message.usage.cacheCreationInputTokens || 0;
                
                // Update per-model tokens
                if (!chat.perModelTokensPrice[model]) {
                    chat.perModelTokensPrice[model] = {
                        input: 0,
                        output: 0,
                        cacheRead: 0,
                        cacheCreation: 0,
                        totalCost: 0
                    };
                }
                
                chat.perModelTokensPrice[model].input += message.usage.promptTokens || 0;
                chat.perModelTokensPrice[model].output += message.usage.completionTokens || 0;
                chat.perModelTokensPrice[model].cacheRead += message.usage.cacheReadInputTokens || 0;
                chat.perModelTokensPrice[model].cacheCreation += message.usage.cacheCreationInputTokens || 0;
                
                // Add price if available
                if (message.price !== undefined) {
                    chat.totalTokensPrice.totalCost += message.price;
                    chat.perModelTokensPrice[model].totalCost += message.price;
                }
            }
            
            // Handle accounting nodes
            if (message.role === 'accounting' && message.cumulativeTokens) {
                // Add the preserved tokens from accounting node
                chat.totalTokensPrice.input += message.cumulativeTokens.inputTokens || 0;
                chat.totalTokensPrice.output += message.cumulativeTokens.outputTokens || 0;
                chat.totalTokensPrice.cacheRead += message.cumulativeTokens.cacheReadTokens || 0;
                chat.totalTokensPrice.cacheCreation += message.cumulativeTokens.cacheCreationTokens || 0;
                
                // Note: We can't attribute accounting node tokens to specific models
                // They represent aggregated tokens from deleted messages
            }
        }
        
        // Add sub-chat costs from tool-results
        this.aggregateSubChatCostsFromToolResults(chat);
    }

    /**
     * Update parent's tool-result with sub-chat costs
     */
    updateParentToolResultCosts(parentChatId, toolCallId, subChat) {
        const parentChat = this.chats.get(parentChatId);
        if (!parentChat) {
            console.error(`[updateParentToolResultCosts] Parent chat ${parentChatId} not found`);
            return;
        }
        
        
        // Find the tool-result in parent messages
        for (const message of parentChat.messages) {
            if (message.role === 'tool-results' && message.toolResults) {
                const toolResult = message.toolResults.find(tr => tr.toolCallId === toolCallId);
                if (toolResult) {
                    // Store the sub-chat's current costs
                    toolResult.subChatCosts = {
                        totalTokens: { ...subChat.totalTokensPrice },
                        perModel: {}
                    };
                    
                    // Deep copy per-model costs
                    for (const [model, costs] of Object.entries(subChat.perModelTokensPrice || {})) {
                        toolResult.subChatCosts.perModel[model] = { ...costs };
                    }
                    
                    // Save parent chat to persist the updated costs
                    this.autoSave(parentChatId);
                    return;
                }
            }
        }
        
        console.error(`[updateParentToolResultCosts] Tool result ${toolCallId} not found in parent messages`);
    }

    /**
     * Aggregate sub-chat costs from tool-results that have subChatCosts
     */
    aggregateSubChatCostsFromToolResults(chat) {
        for (const message of chat.messages) {
            if (message.role === 'tool-results' && message.toolResults) {
                for (const toolResult of message.toolResults) {
                    if (toolResult.subChatCosts) {
                        const costs = toolResult.subChatCosts;

                        // Add to total tokens
                        chat.totalTokensPrice.input += costs.totalTokens.input || 0;
                        chat.totalTokensPrice.output += costs.totalTokens.output || 0;
                        chat.totalTokensPrice.cacheRead += costs.totalTokens.cacheRead || 0;
                        chat.totalTokensPrice.cacheCreation += costs.totalTokens.cacheCreation || 0;
                        chat.totalTokensPrice.totalCost += costs.totalTokens.totalCost || 0;

                        // Add to per-model tokens
                        for (const [model, modelCosts] of Object.entries(costs.perModel)) {
                            if (!chat.perModelTokensPrice[model]) {
                                chat.perModelTokensPrice[model] = {
                                    input: 0,
                                    output: 0,
                                    cacheRead: 0,
                                    cacheCreation: 0,
                                    totalCost: 0
                                };
                            }

                            chat.perModelTokensPrice[model].input += modelCosts.input || 0;
                            chat.perModelTokensPrice[model].output += modelCosts.output || 0;
                            chat.perModelTokensPrice[model].cacheRead += modelCosts.cacheRead || 0;
                            chat.perModelTokensPrice[model].cacheCreation += modelCosts.cacheCreation || 0;
                            chat.perModelTokensPrice[model].totalCost += modelCosts.totalCost || 0;
                        }
                    }
                }
            }
        }
    }

    
    // Migrate old chat data to include token pricing
    migrateTokenPricing(chat) {
        // Initialize structures if not present
        if (!chat.totalTokensPrice) {
            chat.totalTokensPrice = {
                input: 0,
                output: 0,
                cacheRead: 0,
                cacheCreation: 0,
                totalCost: 0
            };
        }
        
        if (!chat.perModelTokensPrice) {
            chat.perModelTokensPrice = {};
        }
        
        // Process all messages to calculate prices
        for (const message of chat.messages) {
            if (message.usage && !message.price) {
                // Add model if missing (use chat's model as fallback)
                if (!message.model) {
                    message.model = ChatConfig.getChatModelString(chat);
                }
                
                // Calculate price
                if (message.model) {
                    const price = this.calculateMessagePrice(message.model, message.usage);
                    if (price !== null) {
                        message.price = price;
                    }
                }
            }
        }
        
        // Recalculate cumulative pricing
        this.updateChatTokenPricing(chat);
        
        // Save the migrated data
        this.saveChatToStorage(chat.id);
    }

    initializeUI() {
        // Chat sidebar
        this.newChatBtn = document.getElementById('newChatBtn');
        this.newChatBtn.addEventListener('click', () => this.createNewChatDirectly());
        this.chatSessions = document.getElementById('chatSessions');
        
        // Event delegation for delete buttons
        this.chatSessions.addEventListener('click', (e) => {
            // Cast to Element to help IDE recognize DOM methods
            /** @type {Element} */
            const target = e.target;
            const deleteBtn = target.closest('.btn-delete-chat');
            if (deleteBtn) {
                e.stopPropagation();
                const chatId = deleteBtn.dataset.chatId;
                if (chatId) {
                    this.deleteChat(chatId);
                }
            }
        });
        
        // Sidebar footer controls
        this.themeToggle = document.getElementById('themeToggle');
        this.themeToggle.addEventListener('click', () => this.toggleTheme());
        this.settingsBtn = document.getElementById('settingsBtn');
        this.settingsBtn.addEventListener('click', () => this.showModal('settingsModal'));
        
        // Chat area - Main containers only, not individual chat elements
        this.chatContainersEl = document.getElementById('chatContainers');
        this.welcomeScreen = document.getElementById('welcomeScreen');
        
        // These will be set when switching chats for backward compatibility
        this.chatTitle = null;
        this.sendMessageBtn = null;
        this.reconnectMcpBtn = null;
        this.copyMetricsBtn = null;
        this.summarizeBtn = null;
        this.generateTitleBtn = null;
        this.llmModelDropdown = null;
        this.currentModelText = null;
        this.mcpServerDropdown = null;
        this.currentMcpText = null;
        
        // Close dropdowns when clicking outside (global handler)
        document.addEventListener('click', () => {
            // Close all open dropdowns in all chat containers
            this.chatContainers.forEach((container) => {
                const elements = container._elements;
                if (elements) {
                    // Check each dropdown exists before accessing style
                    if (elements.llmModelDropdown && elements.llmModelDropdown.style) {
                        elements.llmModelDropdown.style.display = 'none';
                    }
                    if (elements.mcpServerDropdown && elements.mcpServerDropdown.style) {
                        elements.mcpServerDropdown.style.display = 'none';
                    }
                }
            });
        });
        
        // Log panel
        this.logPanel = document.getElementById('logPanel');
        this.toggleLogBtn = document.getElementById('toggleLogBtn');
        this.expandLogBtn = document.getElementById('expandLogBtn');
        this.clearLogBtn = document.getElementById('clearLogBtn');
        this.downloadLogBtn = document.getElementById('downloadLogBtn');
        this.logContent = document.getElementById('logContent');
        
        this.toggleLogBtn.addEventListener('click', () => this.toggleLog());
        this.expandLogBtn.addEventListener('click', () => this.toggleLog());
        this.clearLogBtn.addEventListener('click', () => this.clearLog());
        this.downloadLogBtn.addEventListener('click', () => this.downloadLog());
        
        // Sidebar management
        this.chatSidebar = document.getElementById('chatSidebar');
        this.toggleSidebarBtn = document.getElementById('toggleSidebarBtn');
        
        // Set up sidebar toggle button
        this.toggleSidebarBtn.addEventListener('click', () => this.toggleChatSidebar());
        
        // Load sidebar states from localStorage
        this.loadSidebarStates();
        
        // Temperature control - will be set when switching chats
        
        // Settings modal
        this.settingsModal = document.getElementById('settingsModal');
        this.setupModal('settingsModal', 'settingsBackdrop', 'closeSettingsBtn');
        
        // Settings lists
        this.mcpServersList = document.getElementById('mcpServersList');
        this.addMcpServerBtn = document.getElementById('addMcpServerBtn');
        
        this.addMcpServerBtn.addEventListener('click', () => this.showModal('addMcpModal'));
        
        // New chat modal - no longer used, kept for potential future use
        // this.setupModal('newChatModal', 'newChatBackdrop', 'closeNewChatBtn');
        // this.newChatMcpServer = document.getElementById('newChatMcpServer');
        // this.newChatLlmProvider = document.getElementById('newChatLlmProvider');
        // this.newChatModelGroup = document.getElementById('newChatModelGroup');
        // this.newChatModel = document.getElementById('newChatModel');
        // this.newChatTitle = document.getElementById('newChatTitle');
        // this.createChatBtn = document.getElementById('createChatBtn');
        // this.cancelNewChatBtn = document.getElementById('cancelNewChatBtn');
        // 
        // this.newChatLlmProvider.addEventListener('change', () => this.updateNewChatModels());
        // this.createChatBtn.addEventListener('click', () => this.createNewChat());
        // this.cancelNewChatBtn.addEventListener('click', () => this.hideModal('newChatModal'));
        
        // Add MCP server modal
        this.setupModal('addMcpModal', 'addMcpBackdrop', 'closeAddMcpBtn');
        this.mcpServerUrl = document.getElementById('mcpServerUrl');
        this.mcpServerName = document.getElementById('mcpServerName');
        this.saveMcpServerBtn = document.getElementById('saveMcpServerBtn');
        this.cancelAddMcpBtn = document.getElementById('cancelAddMcpBtn');
        
        this.saveMcpServerBtn.addEventListener('click', () => this.addMcpServer());
        this.cancelAddMcpBtn.addEventListener('click', () => this.hideModal('addMcpModal'));
        
        // System prompt modal controls
        this.systemPromptModal = document.getElementById('systemPromptModal');
        this.systemPromptTextarea = document.getElementById('systemPromptTextarea');
        this.closeSystemPromptBtn = document.getElementById('closeSystemPromptBtn');
        this.systemPromptBackdrop = document.getElementById('systemPromptBackdrop');
        this.cancelSystemPromptBtn = document.getElementById('cancelSystemPromptBtn');
        this.saveSystemPromptBtn = document.getElementById('saveSystemPromptBtn');
        this.resetToDefaultPromptBtn = document.getElementById('resetToDefaultPromptBtn');
        
        this.closeSystemPromptBtn.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.systemPromptBackdrop.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.cancelSystemPromptBtn.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.saveSystemPromptBtn.addEventListener('click', () => {
            // Get the chatId from the modal's data attribute
            const chatId = this.systemPromptModal.dataset.chatId;
            if (chatId) {
                this.saveSystemPrompt(chatId);
            }
        });
        this.resetToDefaultPromptBtn.addEventListener('click', () => {
            this.systemPromptTextarea.value = this.defaultSystemPrompt;
        });
        
        // Auto-generate server name from URL
        this.mcpServerUrl.addEventListener('input', () => {
            if (!this.mcpServerName.value) {
                try {
                    const url = new URL(this.mcpServerUrl.value);
                    this.mcpServerName.value = url.hostname || 'MCP Server';
                } catch {
                    // Invalid URL, ignore
                }
            }
        });
        
        // Tooltips are now CSS-only, no initialization needed
        
        // Setup no models modal
        this.noModelsModal = document.getElementById('noModelsModal');
        this.noModelsBackdrop = document.getElementById('noModelsBackdrop');
        this.noModelsProxyUrl = document.getElementById('noModelsProxyUrl');
        this.retryModelsBtn = document.getElementById('retryModelsBtn');
        
        // Retry button handler
        this.retryModelsBtn.addEventListener('click', async () => {
            this.hideModal('noModelsModal');
            await this.initializeDefaultLLMProvider();
        });
    }

    setupModal(modalId, backdropId, closeId) {
        const backdrop = document.getElementById(backdropId);
        const closeBtn = document.getElementById(closeId);
        
        backdrop.addEventListener('click', () => this.hideModal(modalId));
        closeBtn.addEventListener('click', () => this.hideModal(modalId));
    }


    showModal(modalId) {
        document.getElementById(modalId).classList.add('show');
    }

    hideModal(modalId) {
        document.getElementById(modalId).classList.remove('show');
    }
    
    showNoModelsModal(proxyUrl) {
        // Update the proxy URL in the modal
        this.noModelsProxyUrl.textContent = proxyUrl;
        
        // Show the modal
        this.showModal('noModelsModal');
        
        // Disable the backdrop click since we don't want users to close it
        this.noModelsBackdrop.onclick = null;
    }
    
    validateChatModels() {
        // Validate each chat's model
        for (const [chatId, chat] of this.chats) {
            // Skip if chat doesn't have proper config
            if (!chat.config || !chat.config.model) {
                continue;
            }
            
            if (chat.llmProviderId) {
                const provider = this.llmProviders.get(chat.llmProviderId);
                if (provider && provider.availableProviders) {
                    // Check if the model exists
                    let modelExists = false;
                    const providerType = chat.config.model.provider;
                    const modelName = chat.config.model.id;
                    
                    if (providerType && modelName && provider.availableProviders[providerType]) {
                        const models = provider.availableProviders[providerType].models || [];
                        modelExists = models.some(m => {
                            const mId = typeof m === 'string' ? m : m.id;
                            return mId === modelName;
                        });
                    }
                    
                    if (!modelExists) {
                        const oldModelString = ChatConfig.modelConfigToString(chat.config.model);
                        console.error(`Chat ${chatId} has invalid model ${oldModelString}. Model not found in available providers.`);
                        
                        // Mark the chat as having an invalid model
                        chat.hasInvalidModel = true;
                        
                        // DO NOT automatically reset or save!
                        // The user must manually select a valid model
                    }
                }
            }
        }
    }
    
    /**
     * Update the model display in the UI for a chat
     */
    updateModelDisplay(chat) {
        const chatId = chat.id;
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements) {return;}
        
        const elements = container._elements;
        const provider = this.llmProviders.get(chat.llmProviderId);
        
        // Update LLM model display
        if (provider && chat.config?.model?.id) {
            const modelDisplay = chat.config.model.id;
            if (elements.llmMeta) {
                elements.llmMeta.textContent = modelDisplay;
            }
            if (elements.currentModelText) {
                elements.currentModelText.textContent = modelDisplay;
            }
        } else {
            if (elements.llmMeta) {
                elements.llmMeta.textContent = 'Model: Not found';
            }
            if (elements.currentModelText) {
                elements.currentModelText.textContent = 'Select Model';
            }
        }
    }
    
    isModelValid(model, provider) {
        if (!model || !provider || !provider.availableProviders) {return false;}
        
        // Handle both string format and config object
        let providerType, modelName;
        if (typeof model === 'string') {
            const modelConfig = ChatConfig.modelConfigFromString(model);
            providerType = modelConfig?.provider;
            modelName = modelConfig?.id;
        } else if (model.provider && model.id) {
            providerType = model.provider;
            modelName = model.id;
        } else {
            return false;
        }
        
        if (!providerType || !modelName || !provider.availableProviders[providerType]) {return false;}
        
        const models = provider.availableProviders[providerType].models || [];
        return models.some(m => {
            const mId = typeof m === 'string' ? m : m.id;
            return mId === modelName;
        });
    }
    
    populateModelDropdown(chatId, dropdown = null, buttonElement = null) {
        if (!chatId) {
            console.error('[populateModelDropdown] Called without chatId');
            return;
        }
        const targetChatId = chatId;
        let targetDropdown = dropdown || this.llmModelDropdown;
        
        const chat = this.chats.get(targetChatId);
        if (!chat) {
            console.error(`[showModelSelector] Chat not found for chatId: ${targetChatId}`);
            return;
        }
        
        const provider = this.llmProviders.get(chat.llmProviderId);
        if (!provider || !provider.availableProviders) {
            console.error(`[showModelSelector] Provider not found or has no available providers for providerId: ${chat.llmProviderId}`, { provider, hasAvailableProviders: provider?.availableProviders });
            return;
        }
        
        // Create a modal overlay instead of using the dropdown
        const overlay = document.createElement('div');
        overlay.className = 'model-selector-overlay';
        overlay.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(0, 0, 0, 0.5);
            z-index: 9999;
        `;
        
        // Get button position for dropdown-like positioning
        const buttonRect = buttonElement ? buttonElement.getBoundingClientRect() : null;
        
        const modalContent = document.createElement('div');
        modalContent.style.cssText = `
            width: 900px !important;
            min-width: 900px !important;
            max-width: 900px !important;
            max-height: 64vh;
            position: fixed;
            background: var(--background-color);
            border-radius: 8px;
            box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
            border: 1px solid var(--border-color);
            padding: 0;
            display: flex;
            flex-direction: column;
        `;
        
        // Position the modal like a dropdown
        if (buttonRect) {
            // Position below the button
            const spaceBelow = window.innerHeight - buttonRect.bottom;
            const spaceAbove = buttonRect.top;
            
            if (spaceBelow >= 400 || spaceBelow > spaceAbove) {
                // Show below button
                modalContent.style.top = `${buttonRect.bottom + 5}px`;
                modalContent.style.bottom = 'auto';
            } else {
                // Show above button
                modalContent.style.bottom = `${window.innerHeight - buttonRect.top + 5}px`;
                modalContent.style.top = 'auto';
            }
            
            // Center horizontally relative to button
            const modalWidth = 900;
            const buttonCenter = buttonRect.left + (buttonRect.width / 2);
            let left = buttonCenter - (modalWidth / 2);
            
            // Keep within viewport bounds
            if (left < 10) left = 10;
            if (left + modalWidth > window.innerWidth - 10) {
                left = window.innerWidth - modalWidth - 10;
            }
            
            modalContent.style.left = `${left}px`;
        } else {
            // Fallback to center if no button provided
            modalContent.style.top = '50%';
            modalContent.style.left = '50%';
            modalContent.style.transform = 'translate(-50%, -50%)';
        }
        
        // Close when clicking overlay
        overlay.addEventListener('click', (e) => {
            if (e.target === overlay) {
                overlay.remove();
                // Update displays
                this.updateChatHeader(chatId);
                this.updateChatSessions();
            }
        });
        
        // Prevent clicks inside modal from closing
        modalContent.addEventListener('click', (e) => {
            e.stopPropagation();
        });
        
        overlay.appendChild(modalContent);
        document.body.appendChild(overlay);
        
        // Use modalContent as our target for populating
        targetDropdown = modalContent;
        
        // Get current config
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        
        // Ensure the config is assigned to the chat object
        if (!chat.config) {
            chat.config = config;
        }
        
        // Create header section (fixed)
        const headerSection = document.createElement('div');
        headerSection.style.cssText = `
            flex-shrink: 0;
            position: relative;
            padding: 12px 16px;
            border-bottom: 1px solid var(--border-color);
            background: var(--surface-color);
        `;
        
        // Add title
        const headerTitle = document.createElement('h3');
        headerTitle.style.cssText = `
            margin: 0;
            font-size: 16px;
            font-weight: 600;
            color: var(--text-primary);
        `;
        headerTitle.textContent = 'Model & Optimization Settings';
        headerSection.appendChild(headerTitle);
        
        // Add close button at the top
        const closeButton = document.createElement('button');
        closeButton.style.cssText = `
            position: absolute;
            top: 10px;
            right: 10px;
            background: none;
            border: none;
            font-size: 24px;
            cursor: pointer;
            color: var(--text-secondary);
            z-index: 1;
            padding: 0;
            width: 32px;
            height: 32px;
            display: flex;
            align-items: center;
            justify-content: center;
            border-radius: 4px;
            transition: background 0.2s;
        `;
        closeButton.innerHTML = '×';
        closeButton.addEventListener('mouseenter', () => {
            closeButton.style.background = 'var(--hover-color)';
        });
        closeButton.addEventListener('mouseleave', () => {
            closeButton.style.background = 'none';
        });
        closeButton.addEventListener('click', () => {
            overlay.remove();
            // Update displays
            this.updateChatHeader(chatId);
            this.updateChatSessions();
        });
        headerSection.appendChild(closeButton);
        targetDropdown.appendChild(headerSection);
        
        // Create scrollable content container
        const contentContainer = document.createElement('div');
        contentContainer.style.cssText = `
            flex: 1;
            overflow-y: auto;
            min-height: 200px;
            max-height: calc(64vh - 120px); /* Account for header and footer */
            scrollbar-width: thin;
            scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);
        `;
        
        // Add cost optimization settings section to content container
        this.addCostOptimizationSection(contentContainer, chatId, config);
        targetDropdown.appendChild(contentContainer);
        
        // Add footer section with cost estimation
        // Footer section removed - no cost estimation needed
    }

    addCostOptimizationSection(dropdown, chatId, config) {
        const section = document.createElement('div');
        section.style.cssText = `
            padding: 8px 12px;
            background: var(--surface-color);
            border-bottom: 1px solid var(--border-color);
        `;
        
        section.innerHTML = `
            <div style="font-weight: 600; font-size: 13px; margin-bottom: 8px; color: var(--text-primary);">
                Cost Optimizations
            </div>
        `;
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('addCostOptimizationSection: Chat not found for ID:', chatId);
            return;
        }
        
        // Get all available models - removed as unused
        // const allModels = this.getAllAvailableModels();
        
        // Chat Model Selection with Max Tokens
        const chatModelDiv = document.createElement('div');
        chatModelDiv.style.cssText = 'display: flex; align-items: center; gap: 8px; margin-bottom: 8px; flex-wrap: wrap;';
        
        const currentMaxTokens = chat.config.model.params.maxTokens;
        chatModelDiv.innerHTML = `
            <span>Chat with</span>
            <div class="model-select-wrapper" style="position: relative; display: inline-block;">
                <button class="model-select-btn" id="chatModel_${chatId}" 
                        style="padding: 2px 8px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); cursor: pointer;
                               display: flex; align-items: center; gap: 4px;">
                    <span class="model-name">${ChatConfig.getChatModelString(chat) || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
            
            <div style="display: flex; align-items: center; gap: 8px; margin-left: auto; flex-wrap: wrap;">
                <div style="display: flex; align-items: center; gap: 4px;">
                    <label style="font-size: 12px; color: var(--text-secondary);">max output tokens:</label>
                    <input type="text" id="maxTokens_${chatId}" 
                           list="maxTokensList_${chatId}"
                           value="${currentMaxTokens}"
                           style="width: 70px; padding: 2px 6px; border: 1px solid var(--border-color); 
                                  border-radius: 4px; background: var(--background-color); 
                                  color: var(--text-primary); font-size: 12px;">
                    <datalist id="maxTokensList_${chatId}">
                        <option value="1024"></option>
                        <option value="2048"></option>
                        <option value="4096"></option>
                        <option value="8192"></option>
                        <option value="16384"></option>
                        <option value="32768"></option>
                        <option value="65536"></option>
                        <option value="131072"></option>
                    </datalist>
                </div>
                <div id="contextWindowControl_${chatId}" style="display: ${this.shouldShowContextWindowControl(chat) ? 'flex' : 'none'}; align-items: center; gap: 4px;">
                    <label style="font-size: 12px; color: var(--text-secondary);">context window:</label>
                    <input type="text" id="contextWindow_${chatId}" 
                           list="contextWindowList_${chatId}"
                           value="${chat.config?.model?.params?.contextWindow || this.getDefaultContextWindow(chat)}"
                           style="width: 70px; padding: 2px 6px; border: 1px solid var(--border-color); 
                                  border-radius: 4px; background: var(--background-color); 
                                  color: var(--text-primary); font-size: 12px;">
                    <datalist id="contextWindowList_${chatId}">
                        ${this.getContextWindowDatalistOptions(chat)}
                    </datalist>
                </div>
            </div>
        `;
        section.appendChild(chatModelDiv);
        
        // Tool Summarization Option
        const toolSumDiv = document.createElement('div');
        const _isEnabled = true; // Feature is now implemented
        toolSumDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px;`;
        
        const currentThreshold = chat.config.optimisation.toolSummarisation.thresholdKiB ?? 20; // Default 20KB, allow 0
        const toolSumModel = ChatConfig.modelConfigToString(chat.config.optimisation.toolSummarisation.model) || ChatConfig.getChatModelString(chat);
        
        toolSumDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="toolSummarization_${chatId}" ${_isEnabled ? '' : 'disabled'}
                       ${chat.config.optimisation.toolSummarisation.enabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Summarize tool responses of at least</span>
            </label>
            <select id="toolThreshold_${chatId}" ${_isEnabled ? '' : 'disabled'}
                    style="width: 70px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;">
                <option value="0" ${currentThreshold === 0 ? 'selected' : ''}>0 (all)</option>
                <option value="5" ${currentThreshold === 5 ? 'selected' : ''}>5</option>
                <option value="10" ${currentThreshold === 10 ? 'selected' : ''}>10</option>
                <option value="20" ${currentThreshold === 20 ? 'selected' : ''}>20</option>
                <option value="30" ${currentThreshold === 30 ? 'selected' : ''}>30</option>
                <option value="40" ${currentThreshold === 40 ? 'selected' : ''}>40</option>
                <option value="50" ${currentThreshold === 50 ? 'selected' : ''}>50</option>
                <option value="60" ${currentThreshold === 60 ? 'selected' : ''}>60</option>
                <option value="70" ${currentThreshold === 70 ? 'selected' : ''}>70</option>
                <option value="80" ${currentThreshold === 80 ? 'selected' : ''}>80</option>
                <option value="90" ${currentThreshold === 90 ? 'selected' : ''}>90</option>
                <option value="100" ${currentThreshold === 100 ? 'selected' : ''}>100</option>
            </select>
            <span>KiB size, with</span>
            <div class="model-select-wrapper" style="position: relative; display: inline-block;">
                <button class="model-select-btn" id="toolSumModel_${chatId}" ${_isEnabled ? '' : 'disabled'}
                        style="padding: 2px 8px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); cursor: pointer;
                               display: flex; align-items: center; gap: 4px;">
                    <span class="model-name">${toolSumModel || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
        `;
        
        section.appendChild(toolSumDiv);
        
        // Auto-summarization Option
        const autoSumDiv = document.createElement('div');
        const _autoSumEnabled = true; // Auto-summarization is now implemented
        autoSumDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px;`;
        
        const currentPercent = chat.config.optimisation.autoSummarisation.triggerPercent || 50;
        const autoSumModel = ChatConfig.modelConfigToString(chat.config.optimisation.autoSummarisation.model) || ChatConfig.getChatModelString(chat);
        
        autoSumDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="autoSummarization_${chatId}" ${chat.config.optimisation.autoSummarisation.enabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Summarize conversation when context window above</span>
            </label>
            <select id="autoSumThreshold_${chatId}"
                    style="width: 70px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;">
                <option value="30">30%</option>
                <option value="40">40%</option>
                <option value="50" ${currentPercent === 50 ? 'selected' : ''}>50%</option>
                <option value="60">60%</option>
                <option value="70">70%</option>
                <option value="80">80%</option>
                <option value="90">90%</option>
            </select>
            <span>with</span>
            <div class="model-select-wrapper" style="position: relative; display: inline-block;">
                <button class="model-select-btn" id="autoSumModel_${chatId}"
                        style="padding: 2px 8px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); cursor: pointer;
                               display: flex; align-items: center; gap: 4px;">
                    <span class="model-name">${autoSumModel || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
        `;
        
        section.appendChild(autoSumDiv);
        
        // Title Generation Option
        const titleGenDiv = document.createElement('div');
        const titleGenEnabled = chat.config.optimisation.titleGeneration?.enabled !== false; // Default to true
        titleGenDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!titleGenEnabled ? 'opacity: 0.5;' : ''}`;
        
        const titleGenModel = ChatConfig.modelConfigToString(chat.config.optimisation.titleGeneration?.model);
        
        titleGenDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="titleGeneration_${chatId}" ${titleGenEnabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Generate chat titles with</span>
            </label>
            <div class="model-select-wrapper" style="position: relative; display: inline-block;">
                <button class="model-select-btn" id="titleGenModel_${chatId}" 
                        style="padding: 2px 8px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); cursor: pointer;
                               display: flex; align-items: center; gap: 4px;"
                        ${!titleGenEnabled ? 'disabled' : ''}>
                    <span class="model-name">${titleGenModel || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
        `;
        
        section.appendChild(titleGenDiv);
        
        // Tool Memory Option
        const toolMemoryDiv = document.createElement('div');
        const toolMemoryEnabled = chat.config.optimisation.toolMemory.enabled;
        toolMemoryDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!toolMemoryEnabled ? 'opacity: 0.5;' : ''}`;
        
        const forgetAfterConclusions = chat.config.optimisation.toolMemory.forgetAfterConclusions;
        
        toolMemoryDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="toolMemory_${chatId}" ${toolMemoryEnabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Stop sending tool responses after the assistant concludes</span>
            </label>
            <select id="toolMemoryThreshold_${chatId}" 
                    style="width: 50px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;"
                    ${!toolMemoryEnabled ? 'disabled' : ''}>
                <option value="0" ${forgetAfterConclusions === 0 ? 'selected' : ''}>0</option>
                <option value="1" ${forgetAfterConclusions === 1 ? 'selected' : ''}>1</option>
                <option value="2" ${forgetAfterConclusions === 2 ? 'selected' : ''}>2</option>
                <option value="3" ${forgetAfterConclusions === 3 ? 'selected' : ''}>3</option>
            </select>
            <span>times</span>
        `;
        
        section.appendChild(toolMemoryDiv);
        
        // Cache Control Option (for providers that support it)
        // Get the provider API type to determine cache support
        const providerType = chat.config.model?.provider;
        const provider = this.llmProviders.get(chat.llmProviderId);
        const providerApiType = provider?.availableProviders?.[providerType]?.type || providerType;
        const supportsCacheControl = providerApiType === 'anthropic';
        
        const cacheControlDiv = document.createElement('div');
        const cacheControlMode = chat.config.optimisation.cacheControl;
        const cacheControlDisabled = !supportsCacheControl;
        cacheControlDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${cacheControlDisabled ? 'opacity: 0.5;' : ''}`;
        
        cacheControlDiv.innerHTML = `
            <label style="display: flex; align-items: center;">
                <span>Cache control:</span>
            </label>
            <select id="cacheControl_${chatId}" 
                    style="width: 100px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;"
                    ${cacheControlDisabled ? 'disabled' : ''}>
                <option value="all-off" ${cacheControlMode === 'all-off' ? 'selected' : ''}>Off</option>
                <option value="system" ${cacheControlMode === 'system' ? 'selected' : ''}>System</option>
                <option value="cached" ${cacheControlMode === 'cached' ? 'selected' : ''}>Cached</option>
            </select>
            ${!supportsCacheControl ? '<span style="color: var(--text-secondary); font-size: 12px;">Anthropic only</span>' : ''}
        `;
        
        section.appendChild(cacheControlDiv);
        
        // Temperature and TopP Controls
        const paramsDiv = document.createElement('div');
        paramsDiv.style.cssText = 'margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border-color);';
        
        const currentTemp = chat.config.model.params.temperature;
        const currentTopP = chat.config.model.params.topP;
        
        paramsDiv.innerHTML = `
            <div style="display: flex; flex-direction: column; gap: 12px;">
                <!-- Temperature Control -->
                <div style="display: flex; align-items: center; gap: 12px;">
                    <label style="font-size: 13px; font-weight: 600; color: var(--text-primary); min-width: 100px;">
                        Temperature
                    </label>
                    <div style="flex: 1; display: flex; align-items: center; gap: 8px;">
                        <span style="font-size: 11px; color: var(--text-tertiary); min-width: 50px;">Focused</span>
                        <input type="range" id="temperature_${chatId}" min="0" max="2" step="0.1" value="${currentTemp}" 
                               style="flex: 1; height: 4px; accent-color: var(--primary-color);">
                        <span style="font-size: 11px; color: var(--text-tertiary); min-width: 50px; text-align: right;">Creative</span>
                        <span id="tempValue_${chatId}" style="font-size: 12px; font-weight: 600; color: var(--primary-color); min-width: 30px; text-align: right;">${currentTemp.toFixed(1)}</span>
                    </div>
                </div>
                
                <!-- TopP Control -->
                <div style="display: flex; align-items: center; gap: 12px;">
                    <label style="font-size: 13px; font-weight: 600; color: var(--text-primary); min-width: 100px;">
                        Top P
                    </label>
                    <div style="flex: 1; display: flex; align-items: center; gap: 8px;">
                        <span style="font-size: 11px; color: var(--text-tertiary); min-width: 50px;">Precise</span>
                        <input type="range" id="topP_${chatId}" min="0" max="1" step="0.05" value="${currentTopP}" 
                               style="flex: 1; height: 4px; accent-color: var(--primary-color);">
                        <span style="font-size: 11px; color: var(--text-tertiary); min-width: 50px; text-align: right;">Diverse</span>
                        <span id="topPValue_${chatId}" style="font-size: 12px; font-weight: 600; color: var(--primary-color); min-width: 30px; text-align: right;">${currentTopP.toFixed(2)}</span>
                    </div>
                </div>
            </div>
        `;
        
        section.appendChild(paramsDiv);
        
        // Temperature and TopP event listeners
        const tempSlider = paramsDiv.querySelector(`#temperature_${chatId}`);
        const tempValueLabel = paramsDiv.querySelector(`#tempValue_${chatId}`);
        const topPSlider = paramsDiv.querySelector(`#topP_${chatId}`);
        const topPValueLabel = paramsDiv.querySelector(`#topPValue_${chatId}`);
        
        tempSlider.addEventListener('input', (e) => {
            const value = parseFloat(e.target.value);
            tempValueLabel.textContent = value.toFixed(1);
        });
        
        tempSlider.addEventListener('change', (e) => {
            chat.config.model.params.temperature = parseFloat(e.target.value);
            this.saveChatConfigSmart(chatId, chat.config);
            this.autoSave(chatId);
        });
        
        topPSlider.addEventListener('input', (e) => {
            const value = parseFloat(e.target.value);
            topPValueLabel.textContent = value.toFixed(2);
        });
        
        topPSlider.addEventListener('change', (e) => {
            chat.config.model.params.topP = parseFloat(e.target.value);
            this.saveChatConfigSmart(chatId, chat.config);
            this.autoSave(chatId);
        });
        
        section.appendChild(document.createElement('div')); // spacer
        
        // Initialize model selection buttons
        this.initializeModelSelectionButtons(section, chatId, chat, config);
        
        // Add event listeners
        const toolSumCheckbox = section.querySelector(`#toolSummarization_${chatId}`);
        const thresholdSelect = section.querySelector(`#toolThreshold_${chatId}`);
        const toolModelBtn = section.querySelector(`#toolSumModel_${chatId}`);
        
        toolSumCheckbox.addEventListener('change', (e) => {
            e.stopPropagation();
            const enabled = toolSumCheckbox.checked;
            thresholdSelect.disabled = !enabled;
            toolModelBtn.disabled = !enabled;
            toolSumDiv.style.opacity = enabled ? '1' : '0.5';
            this.updateOptimizationSetting(chatId, 'toolSummarization', enabled);
        });
        
        thresholdSelect.addEventListener('change', (e) => {
            e.stopPropagation();
            const kbValue = parseInt(e.target.value, 10);
            const validKbValue = isNaN(kbValue) ? 20 : kbValue; // Default 20KB only if invalid, allow 0
            const byteValue = validKbValue * 1024;
            this.updateToolThreshold(chatId, byteValue);
        });
        
        // Auto-summarization controls
        const autoSumCheckbox = section.querySelector(`#autoSummarization_${chatId}`);
        const autoSumSelect = section.querySelector(`#autoSumThreshold_${chatId}`);
        const autoModelBtn = section.querySelector(`#autoSumModel_${chatId}`);
        
        autoSumCheckbox.addEventListener('change', (e) => {
            e.stopPropagation();
            const enabled = autoSumCheckbox.checked;
            autoSumSelect.disabled = !enabled;
            autoModelBtn.disabled = !enabled;
            autoSumDiv.style.opacity = enabled ? '1' : '0.5';
            this.updateOptimizationSetting(chatId, 'autoSummarization', enabled);
        });
        
        autoSumSelect.addEventListener('change', (e) => {
            e.stopPropagation();
            const percent = parseInt(e.target.value, 10) || 50;
            this.updateAutoSumThreshold(chatId, percent);
        });
        
        // Title Generation controls
        const titleGenCheckbox = section.querySelector(`#titleGeneration_${chatId}`);
        const titleModelBtn = section.querySelector(`#titleGenModel_${chatId}`);
        
        titleGenCheckbox.addEventListener('change', (e) => {
            e.stopPropagation();
            const enabled = titleGenCheckbox.checked;
            titleModelBtn.disabled = !enabled;
            titleGenDiv.style.opacity = enabled ? '1' : '0.5';
            this.updateOptimizationSetting(chatId, 'titleGeneration', enabled);
        });
        
        // Tool Memory controls
        const toolMemoryCheckbox = section.querySelector(`#toolMemory_${chatId}`);
        const toolMemorySelect = section.querySelector(`#toolMemoryThreshold_${chatId}`);
        
        toolMemoryCheckbox.addEventListener('change', (e) => {
            e.stopPropagation();
            const enabled = toolMemoryCheckbox.checked;
            toolMemorySelect.disabled = !enabled;
            toolMemoryDiv.style.opacity = enabled ? '1' : '0.5';
            
            // Cache control is no longer mutually exclusive with tool memory
            
            this.updateOptimizationSetting(chatId, 'toolMemory', enabled);
        });
        
        toolMemorySelect.addEventListener('change', (e) => {
            e.stopPropagation();
            const newForgetAfterConclusions = parseInt(e.target.value, 10);
            this.updateToolMemoryThreshold(chatId, newForgetAfterConclusions);
        });
        
        // Max tokens input event listener
        const maxTokensInput = section.querySelector(`#maxTokens_${chatId}`);
        if (maxTokensInput) {
            // Handle both manual input and datalist selection
            maxTokensInput.addEventListener('change', (e) => {
                e.stopPropagation();
                const newMaxTokens = parseInt(e.target.value, 10);
                if (!isNaN(newMaxTokens) && newMaxTokens > 0) {
                    chat.config.model.params.maxTokens = newMaxTokens;
                    this.saveChatConfigSmart(chatId, chat.config);
                    this.autoSave(chatId);
                }
            });
            // Also handle when user presses Enter
            maxTokensInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    e.stopPropagation();
                    e.preventDefault();
                    const newMaxTokens = parseInt(e.target.value, 10);
                    if (!isNaN(newMaxTokens) && newMaxTokens > 0) {
                        chat.config.model.params.maxTokens = newMaxTokens;
                        this.saveChatConfigSmart(chatId, chat.config);
                        this.autoSave(chatId);
                    }
                }
            });
        }
        
        // Context window input event listener (Ollama only)
        const contextWindowInput = section.querySelector(`#contextWindow_${chatId}`);
        if (contextWindowInput) {
            // Handle both manual input and datalist selection
            contextWindowInput.addEventListener('change', (e) => {
                e.stopPropagation();
                const newContextWindow = parseInt(e.target.value, 10);
                if (!isNaN(newContextWindow) && newContextWindow > 0) {
                    chat.config.model.params.contextWindow = newContextWindow;
                    this.saveChatConfigSmart(chatId, chat.config);
                    this.autoSave(chatId);
                    // Update the header immediately
                    this.updateContextWindowIndicator(chatId);
                }
            });
            // Also handle when user presses Enter
            contextWindowInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    e.stopPropagation();
                    e.preventDefault();
                    const newContextWindow = parseInt(e.target.value, 10);
                    if (!isNaN(newContextWindow) && newContextWindow > 0) {
                        chat.config.model.params.contextWindow = newContextWindow;
                        this.saveChatConfigSmart(chatId, chat.config);
                        this.autoSave(chatId);
                        // Update the header immediately
                        this.updateContextWindowIndicator(chatId);
                    }
                }
            });
        }
        
        // Cache control dropdown event listener
        const cacheControlSelect = section.querySelector(`#cacheControl_${chatId}`);
        if (cacheControlSelect) {
            cacheControlSelect.addEventListener('change', (e) => {
                e.stopPropagation();
                const newCacheMode = e.target.value;
                this.updateCacheControlMode(chatId, newCacheMode);
            });
        }
        
        // Other checkboxes (smart filtering, cache control)
        section.querySelectorAll('input[type="checkbox"]:not(#toolSummarization_' + chatId + '):not(#autoSummarization_' + chatId + ')').forEach(checkbox => {
            checkbox.addEventListener('change', (e) => {
                e.stopPropagation();
                this.updateOptimizationSetting(chatId, checkbox.id.split('_')[0], checkbox.checked);
            });
        });
        
        section.querySelectorAll('label').forEach(label => {
            label.addEventListener('click', (e) => {
                e.stopPropagation();
            });
        });
        
        dropdown.appendChild(section);
    }

    formatContextWindow(limit) {
        if (!limit) return '--';
        if (limit >= 1000000) return `${(limit / 1000000).toFixed(1)}M`;
        if (limit >= 1000) return `${(limit / 1000).toFixed(0)}k`;
        return limit.toString();
    }

    /**
     * Check if context window control should be shown for this chat
     * @param {Object} chat - The chat object
     * @returns {boolean} True if context window control should be shown
     */
    shouldShowContextWindowControl(chat) {
        if (!chat || !chat.config || !chat.config.model) {
            return false;
        }
        // Show context window control only for Ollama provider
        const provider = chat.config.model.provider;
        return provider === 'ollama';
    }

    /**
     * Update context window visibility dynamically
     * @param {string} chatId - Chat ID
     * @param {Object} chat - The chat object
     */
    updateContextWindowVisibility(chatId, chat) {
        const contextWindowControl = document.querySelector(`#contextWindowControl_${chatId}`);
        if (contextWindowControl) {
            const shouldShow = this.shouldShowContextWindowControl(chat);
            contextWindowControl.style.display = shouldShow ? 'flex' : 'none';
            
            // If showing, update the datalist options for the new model
            if (shouldShow) {
                const datalist = document.querySelector(`#contextWindowList_${chatId}`);
                if (datalist) {
                    datalist.innerHTML = this.getContextWindowDatalistOptions(chat);
                }
                // Set default value if not already set
                const input = document.querySelector(`#contextWindow_${chatId}`);
                if (input && !chat.config?.model?.params?.contextWindow) {
                    input.value = this.getDefaultContextWindow(chat);
                }
            }
        }
    }

    /**
     * Get default context window for a chat
     * @param {Object} chat - The chat object
     * @returns {number} Default context window size
     */
    getDefaultContextWindow(chat) {
        if (!chat || !chat.config || !chat.config.model) {
            return 128000;
        }
        const modelId = chat.config.model.id;
        return this.modelLimits[modelId] || 128000;
    }

    /**
     * Get the effective context window for a chat
     * @param {Object} chat - The chat object
     * @returns {number} The effective context window size (user-configured or model limit)
     */
    getEffectiveContextWindow(chat) {
        // If chat has an explicitly configured context window, use it
        if (chat?.config?.model?.params?.contextWindow) {
            return chat.config.model.params.contextWindow;
        }
        // Otherwise use the model's limit
        return this.getDefaultContextWindow(chat);
    }

    /**
     * Generate context window datalist options
     * @param {Object} chat - The chat object
     * @returns {string} HTML string with option elements for datalist
     */
    getContextWindowDatalistOptions(chat) {
        if (!chat || !chat.config || !chat.config.model) {
            return '<option value="128000"></option>';
        }

        const modelId = chat.config.model.id;
        const maxContext = this.modelLimits[modelId] || 128000;
        
        // Generate options based on max context window
        const options = [];
        const values = [2048, 4096, 8192, 16384, 32768, 65536, 128000, 256000, 512000, 1000000, 2000000];
        
        for (const value of values) {
            if (value <= maxContext) {
                options.push(`<option value="${value}"></option>`);
            }
        }
        
        // If max context is not in our list, add it
        if (!values.includes(maxContext)) {
            options.push(`<option value="${maxContext}"></option>`);
        }
        
        return options.join('');
    }

    /**
     * Generate context window dropdown options
     * @param {Object} chat - The chat object
     * @returns {string} HTML string with option elements
     */
    getContextWindowOptions(chat) {
        if (!chat || !chat.config || !chat.config.model) {
            return '<option value="128000">128k</option>';
        }

        const modelId = chat.config.model.id;
        const maxContext = this.modelLimits[modelId] || 128000;
        const currentContextWindow = chat.config.model.params.contextWindow || maxContext;
        
        // Generate options based on max context window
        const options = [];
        const values = [2048, 4096, 8192, 16384, 32768, 65536, 128000, 256000, 512000, 1000000, 2000000];
        
        for (const value of values) {
            if (value <= maxContext) {
                const label = this.formatContextWindow(value);
                const selected = value === currentContextWindow ? 'selected' : '';
                options.push(`<option value="${value}" ${selected}>${label}</option>`);
            }
        }
        
        // If max context is not in our list, add it
        if (!values.includes(maxContext)) {
            const label = this.formatContextWindow(maxContext);
            const selected = maxContext === currentContextWindow ? 'selected' : '';
            options.push(`<option value="${maxContext}" ${selected}>${label} (max)</option>`);
        }
        
        return options.join('');
    }
    
    /**
     * Create a formatted HTML tooltip for model information
     * @param {Object} chat - The chat object
     * @returns {string} HTML string for the tooltip
     */
    createModelTooltip(chat) {
        if (!chat || !chat.config || !chat.config.model) {
            return 'No model configured';
        }
        
        const config = chat.config;
        const modelString = ChatConfig.modelConfigToString(config.model);
        const modelInfo = this.modelPricing[config.model.id] || {};
        const contextLimit = this.modelLimits[config.model.id] || 128000;
        
        // Get MCP server name
        const mcpServer = this.mcpServers.get(config.mcpServer);
        const mcpServerName = mcpServer ? mcpServer.name : config.mcpServer;
        
        // Get optimization models
        const toolSumModel = config.optimisation.toolSummarisation.model ? 
            ChatConfig.getModelDisplayName(ChatConfig.modelConfigToString(config.optimisation.toolSummarisation.model)) : 
            'Primary';
        const autoSumModel = config.optimisation.autoSummarisation.model ? 
            ChatConfig.getModelDisplayName(ChatConfig.modelConfigToString(config.optimisation.autoSummarisation.model)) : 
            'Primary';
        const titleGenModel = config.optimisation.titleGeneration.model ? 
            ChatConfig.getModelDisplayName(ChatConfig.modelConfigToString(config.optimisation.titleGeneration.model)) : 
            'Primary';
        
        // Format prices more compactly with bold
        const formatPrice = (price) => {
            if (price === undefined || price === null) return 'N/A';
            return `<b>$${price.toFixed(2)}</b>`;
        };
        
        // Helper to show enabled/disabled status compactly
        const status = (enabled) => enabled ? 
            '<span style="color: var(--success-color);">✓</span>' : 
            '<span style="color: var(--error-color);">✗</span>';
        
        let tooltipHtml = `
            <div style="min-width: 300px;">
                <table style="width: 100%; font-size: 11px; border-collapse: collapse;">
                    <tr style="border-bottom: 1px solid var(--border-color);">
                        <td colspan="2" style="padding: 6px; font-weight: 600; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                            ${modelString}
                        </td>
                    </tr>`;
        
        // Model parameters section (no provider, more condensed)
        tooltipHtml += `
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Context:</td>
                        <td style="padding: 4px 6px; text-align: right;">${this.formatContextWindow(contextLimit)}</td>
                    </tr>
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Params:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            T=${config.model.params.temperature} P=${config.model.params.topP} Max=${config.model.params.maxTokens}${config.model.params.seed.enabled ? ` Seed=${config.model.params.seed.value}` : ''}
                        </td>
                    </tr>`;
        
        // Pricing section (condensed with bold prices)
        if (modelInfo.input || modelInfo.output) {
            tooltipHtml += `
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Pricing/1M:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            In: ${formatPrice(modelInfo.input)} Out: ${formatPrice(modelInfo.output)}`;
            
            if (modelInfo.cacheRead !== undefined) {
                tooltipHtml += ` CR: ${formatPrice(modelInfo.cacheRead)}`;
            }
            if (modelInfo.cacheWrite !== undefined) {
                tooltipHtml += ` CW: ${formatPrice(modelInfo.cacheWrite)}`;
            }
            
            tooltipHtml += `</td></tr>`;
        }
        
        // All optimization settings in a compact section
        tooltipHtml += `
                    <tr style="border-top: 1px solid var(--border-color);">
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Tool Summary:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            ${status(config.optimisation.toolSummarisation.enabled)} 
                            ${config.optimisation.toolSummarisation.enabled ? `${config.optimisation.toolSummarisation.thresholdKiB}KiB ${toolSumModel}` : 'Disabled'}
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Auto Summary:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            ${status(config.optimisation.autoSummarisation.enabled)} 
                            ${config.optimisation.autoSummarisation.enabled ? `${config.optimisation.autoSummarisation.triggerPercent}% ${autoSumModel}` : 'Disabled'}
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Tool Memory:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            ${status(config.optimisation.toolMemory.enabled)} 
                            ${config.optimisation.toolMemory.enabled ? 
                                (config.optimisation.toolMemory.forgetAfterConclusions === 0 ? 'forget immediately' :
                                 config.optimisation.toolMemory.forgetAfterConclusions === 1 ? 'forget after 1 turn' :
                                 `forget after ${config.optimisation.toolMemory.forgetAfterConclusions} turns`) : 
                                'Always remember'}
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Cache Control:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            ${config.optimisation.cacheControl === 'all-off' ? 'Off' : 
                              config.optimisation.cacheControl === 'system' ? 'System only' : 
                              config.optimisation.cacheControl === 'cached' ? 'Cached' : config.optimisation.cacheControl}
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 4px 6px; color: var(--text-secondary);">Auto Title:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px;">
                            ${status(config.optimisation.titleGeneration.enabled)} 
                            ${config.optimisation.titleGeneration.enabled ? titleGenModel : 'Disabled'}
                        </td>
                    </tr>`;
        
        // Server info with name
        tooltipHtml += `
                    <tr style="border-top: 1px solid var(--border-color);">
                        <td style="padding: 4px 6px; color: var(--text-secondary);">MCP Server:</td>
                        <td style="padding: 4px 6px; text-align: right; font-size: 10px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 150px;">
                            ${mcpServerName}
                        </td>
                    </tr>
                </table>
            </div>`;
        
        return tooltipHtml;
    }
    
    getAllAvailableModels() {
        const models = [];
        
        // Iterate through all LLM providers
        this.llmProviders.forEach((provider) => {
            // Check if provider has availableProviders (the actual structure from the proxy)
            if (provider.availableProviders) {
                Object.entries(provider.availableProviders).forEach(([providerType, providerConfig]) => {
                    if (providerConfig.models && Array.isArray(providerConfig.models)) {
                        providerConfig.models.forEach(model => {
                            const modelId = typeof model === 'string' ? model : model.id;
                            const contextWindow = typeof model === 'object' ? model.contextWindow : null;
                            const pricing = typeof model === 'object' ? model.pricing : null;
                            
                            if (modelId) {
                                models.push({
                                    id: modelId,
                                    providerId: providerType,
                                    providerApiType: providerConfig.type || providerType, // API type for this provider
                                    contextWindow: contextWindow || 128000, // Default context
                                    pricing: pricing || null,
                                    endpoint: typeof model === 'object' ? model.endpoint : undefined,
                                    supportsTools: typeof model === 'object' ? model.supportsTools : undefined
                                });
                            }
                        });
                    }
                });
            }
        });
        
        // Sort by provider and then by model name
        models.sort((a, b) => {
            if (a.providerId !== b.providerId) {
                return a.providerId.localeCompare(b.providerId);
            }
            return a.id.localeCompare(b.id);
        });
        
        return models;
    }
    
    initializeModelSelectionButtons(section, chatId, chat, _settings) {
        // Helper to create model dropdown with pricing table
        const createModelDropdown = (buttonId, currentModel, onSelect) => {
            const button = section.querySelector(`#${buttonId}`);
            if (!button) return;
            
            // Store the full model string with the button for copy/paste operations
            button.dataset.fullModelString = currentModel || '';
            
            // Add context menu for copy/paste
            button.addEventListener('contextmenu', (e) => {
                e.preventDefault();
                e.stopPropagation();
                
                // Create context menu
                const menu = document.createElement('div');
                menu.className = 'model-context-menu';
                menu.style.cssText = `
                    position: fixed;
                    left: ${e.clientX}px;
                    top: ${e.clientY}px;
                    background: var(--background-color);
                    border: 1px solid var(--border-color);
                    border-radius: 4px;
                    box-shadow: 0 2px 8px rgba(0,0,0,0.15);
                    padding: 4px 0;
                    z-index: 10000;
                `;
                
                const modelName = button.querySelector('.model-name').textContent;
                const fullModelString = button.dataset.fullModelString;
                const hasModel = fullModelString && modelName && modelName !== 'Select model';
                
                if (hasModel) {
                    const copyItem = document.createElement('div');
                    copyItem.style.cssText = `
                        padding: 6px 12px;
                        cursor: pointer;
                        font-size: 13px;
                    `;
                    copyItem.textContent = `Copy "${modelName}"`;
                    copyItem.addEventListener('mouseenter', () => {
                        copyItem.style.background = 'var(--hover-color)';
                    });
                    copyItem.addEventListener('mouseleave', () => {
                        copyItem.style.background = '';
                    });
                    copyItem.addEventListener('click', () => {
                        // Store the full model string (provider:model format)
                        this.copiedModel = fullModelString;
                        this.copiedModelDisplayName = modelName;
                        if (menu.parentNode) {
                            document.body.removeChild(menu);
                        }
                        this.showToast(`Copied model: ${modelName}`, 'success-toast');
                    });
                    menu.appendChild(copyItem);
                }
                
                if (this.copiedModel) {
                    const pasteItem = document.createElement('div');
                    pasteItem.style.cssText = `
                        padding: 6px 12px;
                        cursor: pointer;
                        font-size: 13px;
                    `;
                    const displayName = this.copiedModelDisplayName || this.copiedModel;
                    pasteItem.textContent = `Paste "${displayName}"`;
                    pasteItem.addEventListener('mouseenter', () => {
                        pasteItem.style.background = 'var(--hover-color)';
                    });
                    pasteItem.addEventListener('mouseleave', () => {
                        pasteItem.style.background = '';
                    });
                    pasteItem.addEventListener('click', () => {
                        // Update button display and data
                        button.querySelector('.model-name').textContent = this.copiedModelDisplayName || this.copiedModel;
                        button.dataset.fullModelString = this.copiedModel;
                        // Pass the full model string to the callback
                        onSelect(this.copiedModel);
                        if (menu.parentNode) {
                            document.body.removeChild(menu);
                        }
                        this.showToast(`Pasted model: ${this.copiedModelDisplayName || this.copiedModel}`, 'success-toast');
                    });
                    menu.appendChild(pasteItem);
                }
                
                if (menu.children.length === 0) {
                    const emptyItem = document.createElement('div');
                    emptyItem.style.cssText = `
                        padding: 6px 12px;
                        color: var(--text-secondary);
                        font-size: 13px;
                    `;
                    emptyItem.textContent = 'No model to copy/paste';
                    menu.appendChild(emptyItem);
                }
                
                document.body.appendChild(menu);
                
                // Remove menu on click outside
                const removeMenu = (evt) => {
                    // Don't close if clicking inside the menu
                    if (!menu.contains(evt.target)) {
                        // Check if menu is still in DOM before removing
                        if (menu.parentNode) {
                            document.body.removeChild(menu);
                        }
                        // Remove all event listeners
                        document.removeEventListener('click', removeMenu, true);
                        document.removeEventListener('mousedown', removeMenu, true);
                        document.removeEventListener('contextmenu', removeMenu, true);
                    }
                };
                
                // Use setTimeout to avoid immediate closure
                setTimeout(() => {
                    // Add listeners in capture phase to ensure we catch all clicks
                    document.addEventListener('click', removeMenu, true);
                    document.addEventListener('mousedown', removeMenu, true);
                    document.addEventListener('contextmenu', removeMenu, true);
                }, 0);
            });
            
            // Regular click to open model selection
            button.addEventListener('click', (e) => {
                e.stopPropagation();
                
                // Check if this button already has a dropdown open (toggle behavior)
                if (button.getAttribute('data-dropdown-open') === 'true') {
                    const existingDropdown = document.body.querySelector('.model-selection-dropdown');
                    if (existingDropdown) {
                        existingDropdown.remove();
                        button.removeAttribute('data-dropdown-open');
                    }
                    return;
                }
                
                // Close any other open dropdowns
                document.querySelectorAll('.model-selection-dropdown').forEach(d => d.remove());
                document.querySelectorAll('[data-dropdown-open]').forEach(b => b.removeAttribute('data-dropdown-open'));
                
                // Create model selection dropdown with pricing table
                const dropdown = document.createElement('div');
                dropdown.className = 'model-selection-dropdown';
                
                // Mark button as having an open dropdown
                button.setAttribute('data-dropdown-open', 'true');
                
                // Calculate button position relative to viewport
                const buttonRect = button.getBoundingClientRect();
                const viewportHeight = window.innerHeight;
                const viewportWidth = window.innerWidth;
                const dropdownHeight = 400; // Max height of dropdown
                const dropdownMinWidth = 600;
                
                // Determine if dropdown should appear above or below the button
                const spaceBelow = viewportHeight - buttonRect.bottom;
                const shouldShowAbove = spaceBelow < dropdownHeight && buttonRect.top > dropdownHeight;
                
                // Calculate left position - ensure dropdown doesn't go off-screen
                let leftPosition = buttonRect.left;
                if (leftPosition + dropdownMinWidth > viewportWidth) {
                    leftPosition = Math.max(10, viewportWidth - dropdownMinWidth - 10);
                }
                
                dropdown.style.cssText = `
                    position: fixed;
                    ${shouldShowAbove ? 'bottom' : 'top'}: ${shouldShowAbove ? (viewportHeight - buttonRect.top + 4) : (buttonRect.bottom + 4)}px;
                    left: ${leftPosition}px;
                    background: var(--background-color);
                    border: 1px solid var(--border-color);
                    border-radius: 4px;
                    box-shadow: 0 4px 12px rgba(0,0,0,0.15);
                    z-index: 10000;
                    max-height: 400px;
                    min-width: 600px;
                    display: flex;
                    flex-direction: column;
                `;
                
                // Add search box
                const searchContainer = document.createElement('div');
                searchContainer.style.cssText = `
                    padding: 8px;
                    border-bottom: 1px solid var(--border-color);
                    background: var(--surface-color);
                    position: sticky;
                    top: 0;
                    z-index: 2;
                `;
                
                const searchInput = document.createElement('input');
                searchInput.type = 'text';
                searchInput.placeholder = 'Search models...';
                searchInput.style.cssText = `
                    width: 100%;
                    padding: 6px 10px;
                    border: 1px solid var(--border-color);
                    border-radius: 4px;
                    background: var(--background-color);
                    color: var(--text-primary);
                    font-size: 13px;
                `;
                searchContainer.appendChild(searchInput);
                dropdown.appendChild(searchContainer);
                
                // Create scrollable content container
                const contentContainer = document.createElement('div');
                contentContainer.style.cssText = `
                    flex: 1;
                    overflow-y: auto;
                `;
                dropdown.appendChild(contentContainer);
                
                // Focus search input when dropdown opens
                setTimeout(() => searchInput.focus(), 0);
                
                // Function to update dropdown position on scroll/resize
                const updateDropdownPosition = () => {
                    const newButtonRect = button.getBoundingClientRect();
                    const newViewportHeight = window.innerHeight;
                    const newViewportWidth = window.innerWidth;
                    const newSpaceBelow = newViewportHeight - newButtonRect.bottom;
                    const newShouldShowAbove = newSpaceBelow < dropdownHeight && newButtonRect.top > dropdownHeight;
                    
                    if (newShouldShowAbove) {
                        dropdown.style.top = 'auto';
                        dropdown.style.bottom = `${newViewportHeight - newButtonRect.top + 4}px`;
                    } else {
                        dropdown.style.bottom = 'auto';
                        dropdown.style.top = `${newButtonRect.bottom + 4}px`;
                    }
                    
                    // Update horizontal position
                    let newLeftPosition = newButtonRect.left;
                    if (newLeftPosition + dropdownMinWidth > newViewportWidth) {
                        newLeftPosition = Math.max(10, newViewportWidth - dropdownMinWidth - 10);
                    }
                    dropdown.style.left = `${newLeftPosition}px`;
                };
                
                // Create pricing table
                const models = this.getAllAvailableModels();
                
                // Sort models by provider, then by input price desc, then by name desc
                models.sort((a, b) => {
                    // First sort by provider
                    if (a.providerId !== b.providerId) {
                        return a.providerId.localeCompare(b.providerId);
                    }
                    
                    // Within same provider, sort by input price descending
                    const aInputPrice = a.pricing?.input || 0;
                    const bInputPrice = b.pricing?.input || 0;
                    
                    if (aInputPrice !== bInputPrice) {
                        return bInputPrice - aInputPrice; // Descending order (expensive first)
                    }
                    
                    // If prices are equal, sort by name descending (newer models typically have later names)
                    return b.id.localeCompare(a.id);
                });
                
                // Check if there are any models
                if (!models || models.length === 0) {
                    contentContainer.innerHTML = `
                        <div style="padding: 20px; text-align: center; color: var(--text-secondary);">
                            No models available. Please check your LLM provider configuration.
                        </div>
                    `;
                    document.body.appendChild(dropdown);
                    
                    // Close dropdown on outside click
                    const closeDropdown = (evt) => {
                        if (!dropdown.contains(evt.target) && !button.contains(evt.target)) {
                            if (dropdown.parentElement) {
                                dropdown.parentElement.removeChild(dropdown);
                            }
                            document.removeEventListener('click', closeDropdown, true);
                            document.removeEventListener('mousedown', closeDropdown, true);
                            window.removeEventListener('scroll', updateDropdownPosition, true);
                            window.removeEventListener('resize', updateDropdownPosition);
                        }
                    };
                    
                    // Use capture phase to ensure we catch clicks before they're stopped by modal
                    setTimeout(() => {
                        document.addEventListener('click', closeDropdown, true);
                        document.addEventListener('mousedown', closeDropdown, true);
                        window.addEventListener('scroll', updateDropdownPosition, true);
                        window.addEventListener('resize', updateDropdownPosition);
                    }, 0);
                    return;
                }
                const table = document.createElement('table');
                table.style.cssText = `
                    width: 100%;
                    border-collapse: collapse;
                    font-size: 12px;
                `;
                
                // Table header
                const thead = document.createElement('thead');
                thead.innerHTML = `
                    <tr style="background: var(--surface-color); position: sticky; top: 0; z-index: 1;">
                        <th style="padding: 8px; text-align: left; border-bottom: 1px solid var(--border-color);">Model</th>
                        <th style="padding: 8px; text-align: right; border-bottom: 1px solid var(--border-color);">Context</th>
                        <th style="padding: 8px; text-align: right; border-bottom: 1px solid var(--border-color);">Input $/MTok</th>
                        <th style="padding: 8px; text-align: right; border-bottom: 1px solid var(--border-color);">Output $/MTok</th>
                        <th style="padding: 8px; text-align: right; border-bottom: 1px solid var(--border-color);">CacheR $/MTok</th>
                        <th style="padding: 8px; text-align: right; border-bottom: 1px solid var(--border-color);">CacheW $/MTok</th>
                    </tr>
                `;
                table.appendChild(thead);
                
                const tbody = document.createElement('tbody');
                
                // Function to rebuild table body with filtered models
                const rebuildTableBody = (filteredModels) => {
                    tbody.innerHTML = '';
                    let currentProvider = null;
                    
                    filteredModels.forEach(model => {
                    // Add provider header row when provider changes
                    if (model.providerId !== currentProvider) {
                        currentProvider = model.providerId;
                        const providerRow = document.createElement('tr');
                        providerRow.style.cssText = `
                            background: var(--surface-color);
                            font-weight: 600;
                            color: var(--text-secondary);
                            cursor: default;
                        `;
                        providerRow.innerHTML = `
                            <td colspan="6" style="padding: 8px; text-transform: uppercase; font-size: 11px;">
                                ${currentProvider}
                            </td>
                        `;
                        tbody.appendChild(providerRow);
                    }
                    
                    const tr = document.createElement('tr');
                    
                    // Capture the full model string in the closure
                    const fullModelId = `${model.providerId}:${model.id}`;
                    
                    // Check if this is the currently selected model
                    const isSelected = fullModelId === currentModel;
                    
                    tr.style.cssText = `
                        cursor: pointer;
                        transition: background 0.1s;
                        border-bottom: 1px solid var(--border-subtle, var(--border-color));
                        ${isSelected ? 'background: var(--hover-color);' : ''}
                    `;
                    
                    // Mark selected row for scrolling
                    if (isSelected) {
                        tr.setAttribute('data-selected', 'true');
                    }
                    
                    tr.addEventListener('mouseenter', () => {
                        tr.style.background = 'var(--hover-color)';
                    });
                    tr.addEventListener('mouseleave', () => {
                        if (!isSelected) {
                            tr.style.background = '';
                        }
                    });
                    
                    // Add click handler directly here
                    tr.addEventListener('click', () => {
                        const modelName = ChatConfig.getModelDisplayName(fullModelId);
                        button.querySelector('.model-name').textContent = modelName;
                        button.dataset.fullModelString = fullModelId;  // Update button's stored model string
                        onSelect(fullModelId);
                        document.body.removeChild(dropdown);
                        button.removeAttribute('data-dropdown-open');
                    });
                    
                    const pricing = model.pricing || {};
                    const inputPrice = pricing.input || 0;
                    const outputPrice = pricing.output || 0;
                    const cacheReadPrice = pricing.cacheRead !== undefined ? pricing.cacheRead : '-';
                    const cacheWritePrice = pricing.cacheWrite !== undefined ? pricing.cacheWrite : '-';
                    
                    tr.innerHTML = `
                        <td style="padding: 8px; font-weight: 500;">${model.id}</td>
                        <td style="padding: 8px; text-align: right; color: var(--text-secondary);">${this.formatContextWindow(model.contextWindow)}</td>
                        <td style="padding: 8px; text-align: right;">$${inputPrice.toFixed(2)}</td>
                        <td style="padding: 8px; text-align: right;">$${outputPrice.toFixed(2)}</td>
                        <td style="padding: 8px; text-align: right;">${cacheReadPrice === '-' ? '-' : '$' + cacheReadPrice.toFixed(2)}</td>
                        <td style="padding: 8px; text-align: right;">${cacheWritePrice === '-' ? '-' : '$' + cacheWritePrice.toFixed(2)}</td>
                    `;
                    
                    tbody.appendChild(tr);
                    });
                };
                
                // Initial build with all models
                rebuildTableBody(models);
                
                // Auto-scroll to currently selected model after initial table build
                requestAnimationFrame(() => {
                    const selectedRow = tbody.querySelector('tr[data-selected="true"]');
                    if (selectedRow) {
                        const scrollContainer = contentContainer;
                        const containerRect = scrollContainer.getBoundingClientRect();
                        const rowRect = selectedRow.getBoundingClientRect();
                        
                        const rowTop = rowRect.top - containerRect.top + scrollContainer.scrollTop;
                        const rowBottom = rowTop + rowRect.height;
                        const containerHeight = scrollContainer.clientHeight;
                        
                        // Check if row is outside visible area
                        if (rowTop < scrollContainer.scrollTop || rowBottom > scrollContainer.scrollTop + containerHeight) {
                            // Center the selected row in the viewport
                            const scrollTarget = rowTop - (containerHeight / 2) + (rowRect.height / 2);
                            scrollContainer.scrollTop = Math.max(0, scrollTarget);
                        }
                    }
                });
                
                // Add search functionality
                searchInput.addEventListener('input', (event) => {
                    const searchTerm = event.target.value.toLowerCase().trim();
                    
                    if (!searchTerm) {
                        rebuildTableBody(models);
                        return;
                    }
                    
                    const filteredModels = models.filter(model => {
                        const modelId = model.id.toLowerCase();
                        const providerId = model.providerId.toLowerCase();
                        const fullId = `${providerId}:${modelId}`.toLowerCase();
                        
                        return modelId.includes(searchTerm) || 
                               providerId.includes(searchTerm) || 
                               fullId.includes(searchTerm);
                    });
                    
                    if (filteredModels.length === 0) {
                        tbody.innerHTML = `
                            <tr>
                                <td colspan="6" style="padding: 20px; text-align: center; color: var(--text-secondary);">
                                    No models found matching "${searchTerm}"
                                </td>
                            </tr>
                        `;
                    } else {
                        rebuildTableBody(filteredModels);
                        
                        // Auto-scroll to selected model after search rebuild
                        requestAnimationFrame(() => {
                            const selectedRow = tbody.querySelector('tr[data-selected="true"]');
                            if (selectedRow) {
                                const scrollContainer = contentContainer;
                                const containerRect = scrollContainer.getBoundingClientRect();
                                const rowRect = selectedRow.getBoundingClientRect();
                                
                                const rowTop = rowRect.top - containerRect.top + scrollContainer.scrollTop;
                                const rowBottom = rowTop + rowRect.height;
                                const containerHeight = scrollContainer.clientHeight;
                                
                                if (rowTop < scrollContainer.scrollTop || rowBottom > scrollContainer.scrollTop + containerHeight) {
                                    const scrollTarget = rowTop - (containerHeight / 2) + (rowRect.height / 2);
                                    scrollContainer.scrollTop = Math.max(0, scrollTarget);
                                }
                            }
                        });
                    }
                });
                
                // Handle keyboard navigation
                searchInput.addEventListener('keydown', (keyEvent) => {
                    if (keyEvent.key === 'Escape') {
                        dropdown.remove();
                        button.removeAttribute('data-dropdown-open');
                    } else if (keyEvent.key === 'ArrowDown') {
                        keyEvent.preventDefault();
                        const firstRow = tbody.querySelector('tr[style*="cursor: pointer"]');
                        if (firstRow) {
                            firstRow.focus();
                            firstRow.style.background = 'var(--hover-color)';
                        }
                    }
                });
                
                // Close dropdown on outside click
                const closeDropdown = (evt) => {
                    // Check if click is outside dropdown and button
                    if (!dropdown.contains(evt.target) && !button.contains(evt.target)) {
                        if (dropdown.parentElement) {
                            dropdown.parentElement.removeChild(dropdown);
                        }
                        button.removeAttribute('data-dropdown-open');
                        document.removeEventListener('click', closeDropdown, true);
                        document.removeEventListener('mousedown', closeDropdown, true);
                        window.removeEventListener('scroll', updateDropdownPosition, true);
                        window.removeEventListener('resize', updateDropdownPosition);
                    }
                };
                
                // Click listeners are now added directly when creating rows
                
                // Add event listeners
                // Use capture phase to ensure we catch clicks before they're stopped by modal
                setTimeout(() => {
                    document.addEventListener('click', closeDropdown, true);
                    document.addEventListener('mousedown', closeDropdown, true);
                    window.addEventListener('scroll', updateDropdownPosition, true);
                    window.addEventListener('resize', updateDropdownPosition);
                }, 0);
                
                // Now append elements after functions are defined
                table.appendChild(tbody);
                contentContainer.appendChild(table);
                
                // Append dropdown to body for proper z-index layering
                document.body.appendChild(dropdown);
            });
        };
        
        // Initialize all model selection buttons
        createModelDropdown(`chatModel_${chatId}`, ChatConfig.getChatModelString(chat), (model) => {
            this.updateChatModel(chatId, model);
        });
        
        createModelDropdown(`toolSumModel_${chatId}`, ChatConfig.modelConfigToString(chat.config.optimisation.toolSummarisation.model) || ChatConfig.getChatModelString(chat), (model) => {
            this.updateToolSummarizationModel(chatId, model);
        });
        
        createModelDropdown(`autoSumModel_${chatId}`, ChatConfig.modelConfigToString(chat.config.optimisation.autoSummarisation.model) || ChatConfig.getChatModelString(chat), (model) => {
            this.updateAutoSummarizationModel(chatId, model);
        });
        
        createModelDropdown(`titleGenModel_${chatId}`, ChatConfig.modelConfigToString(chat.config.optimisation.titleGeneration?.model), (model) => {
            this.updateTitleGenerationModel(chatId, model);
        });
    }

    updateOptimizationSetting(chatId, settingType, enabled) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateOptimizationSetting] Chat not found for chatId: ${chatId}, settingType: ${settingType}`);
            return;
        }

        // Get current config or create defaults
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        
        // Update the specific setting
        switch (settingType) {
            case 'toolSummarization':
                config.optimisation.toolSummarisation.enabled = enabled;
                break;
            case 'toolMemory':
                config.optimisation.toolMemory.enabled = enabled;
                break;
            case 'autoSummarization':
                config.optimisation.autoSummarisation.enabled = enabled;
                break;
            case 'titleGeneration':
                config.optimisation.titleGeneration.enabled = enabled;
                break;
            default:
                console.warn(`Unknown setting type: ${settingType}`);
                break;
        }

        // Update chat config
        chat.config = config;

        // Recreate MessageOptimizer with new settings
        const optimizerSettings = {
            ...config,
            llmProviderFactory: config.optimisation.toolSummarisation.enabled ? window.createLLMProvider : undefined
        };
        
        try {
            chat.messageOptimizer = new MessageOptimizer(optimizerSettings);
        } catch (error) {
            console.error('[updateOptimizationSetting] Failed to create MessageOptimizer:', error);
        }

        // Save config
        this.saveChatConfigSmart(chatId, config);
        
        // Auto-save chat
        this.autoSave(chatId);
    }

    updateCacheControlMode(chatId, cacheMode) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateCacheControlMode] Chat not found for chatId: ${chatId}, cacheMode: ${cacheMode}`);
            return;
        }

        // Get current config or create defaults
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        
        // Update cache control mode
        config.optimisation.cacheControl = cacheMode;

        // Update chat config
        chat.config = config;

        // Recreate MessageOptimizer with new settings
        const optimizerSettings = {
            ...config,
            llmProviderFactory: config.optimisation.toolSummarisation.enabled ? window.createLLMProvider : undefined
        };
        
        try {
            chat.messageOptimizer = new MessageOptimizer(optimizerSettings);
        } catch (error) {
            console.error('[updateCacheControlMode] Failed to create MessageOptimizer:', error);
        }

        // Save config
        this.saveChatConfigSmart(chatId, config);
        
        // Auto-save chat
        this.autoSave(chatId);
    }
    
    updateCacheControlUI(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateCacheControlUI] Chat not found for chatId: ${chatId}`);
            return;
        }
        
        // Get the cache control select element
        const cacheControlSelect = document.querySelector(`#cacheControl_${chatId}`);
        if (!cacheControlSelect) {
            return; // UI not available
        }
        
        // Get the parent div for opacity control
        const cacheControlDiv = cacheControlSelect.closest('div');
        if (!cacheControlDiv) {
            return;
        }
        
        // Determine if cache control is supported for the current model
        const providerType = chat.config.model?.provider;
        const provider = this.llmProviders.get(chat.llmProviderId);
        const providerApiType = provider?.availableProviders?.[providerType]?.type || providerType;
        const supportsCacheControl = providerApiType === 'anthropic';
        
        // Update UI state
        cacheControlSelect.disabled = !supportsCacheControl;
        cacheControlDiv.style.opacity = supportsCacheControl ? '1' : '0.5';
    }
    
    updateChatModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateChatModel] Chat not found for chatId: ${chatId}, model: ${model}`);
            return;
        }
        
        // Update config
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        const modelConfig = ChatConfig.modelConfigFromString(model);
        if (modelConfig) {
            // Preserve existing params
            modelConfig.params = config.model?.params || modelConfig.params;
            
            // ALWAYS reset context window to the model's maximum
            // This ensures correct calculations and predictable behavior
            modelConfig.params.contextWindow = this.getDefaultContextWindow({config: {model: modelConfig}});
            
            config.model = modelConfig;
        }
        
        // Note: We intentionally do NOT auto-update optimization feature models
        // If a user explicitly selected a model for a feature, it should stay as that model
        // Only null values (which mean "use chat model") will automatically follow the chat model
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        this.saveChatConfigSmart(chatId, config);
        this.autoSave(chatId);
        
        // Update context window visibility dynamically
        this.updateContextWindowVisibility(chatId, chat);
        
        // Update the context window input field if it exists (for Ollama models)
        const contextWindowInput = document.querySelector(`#contextWindow_${chatId}`);
        if (contextWindowInput && modelConfig) {
            contextWindowInput.value = modelConfig.params.contextWindow;
        }
        
        // Update displays
        this.updateChatHeader(chatId);
        this.updateContextWindowIndicator(chatId); // Update context window immediately
        this.updateChatSessions();
        
        // Update cache control UI based on new model's provider
        this.updateCacheControlUI(chatId);
    }
    
    updateToolSummarizationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateToolSummarizationModel] Chat not found for chatId: ${chatId}, model: ${model}`);
            return;
        }
        
        try {
            const config = chat.config || ChatConfig.loadChatConfig(chatId);
            const modelConfig = ChatConfig.modelConfigFromString(model);
            
            // Set context window for secondary model (Option 2: inherit from main, capped at model max)
            if (modelConfig && modelConfig.provider === 'ollama') {
                const mainContextWindow = config.model?.params?.contextWindow || this.getDefaultContextWindow(chat);
                const modelMaxContextWindow = this.getDefaultContextWindow({config: {model: modelConfig}});
                modelConfig.params.contextWindow = Math.min(mainContextWindow, modelMaxContextWindow);
            }
            
            config.optimisation.toolSummarisation.model = modelConfig;
            
            chat.config = config;
            this.recreateMessageOptimizer(chat, config);
            this.saveChatConfigSmart(chatId, config);
            this.autoSave(chatId);
        } catch (error) {
            console.error(`[updateToolSummarizationModel] Invalid model format for chatId: ${chatId}, model: ${model}`, error);
            this.showToast(`Invalid model format: ${model}. Expected format: provider:model`, 'error-toast');
        }
    }
    
    updateAutoSummarizationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateAutoSummarizationModel] Chat not found for chatId: ${chatId}, model: ${model}`);
            return;
        }
        
        try {
            const config = chat.config || ChatConfig.loadChatConfig(chatId);
            const modelConfig = ChatConfig.modelConfigFromString(model);
            
            // Set context window for secondary model (Option 2: inherit from main, capped at model max)
            if (modelConfig && modelConfig.provider === 'ollama') {
                const mainContextWindow = config.model?.params?.contextWindow || this.getDefaultContextWindow(chat);
                const modelMaxContextWindow = this.getDefaultContextWindow({config: {model: modelConfig}});
                modelConfig.params.contextWindow = Math.min(mainContextWindow, modelMaxContextWindow);
            }
            
            config.optimisation.autoSummarisation.model = modelConfig;
            
            chat.config = config;
            this.recreateMessageOptimizer(chat, config);
            this.saveChatConfigSmart(chatId, config);
            this.autoSave(chatId);
        } catch (error) {
            console.error(`[updateAutoSummarizationModel] Invalid model format for chatId: ${chatId}, model: ${model}`, error);
            this.showToast(`Invalid model format: ${model}. Expected format: provider:model`, 'error-toast');
        }
    }
    
    updateTitleGenerationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateTitleGenerationModel] Chat not found for chatId: ${chatId}, model: ${model}`);
            return;
        }
        
        try {
            const config = chat.config || ChatConfig.loadChatConfig(chatId);
            // Use feature-specific defaults for title generation
            const modelConfig = ChatConfig.modelConfigFromString(model, {
                temperature: 0.7,
                topP: 0.9,
                maxTokens: 100  // Title generation should use limited tokens
            });
            
            // Set context window for secondary model (Option 2: inherit from main, capped at model max)
            if (modelConfig && modelConfig.provider === 'ollama') {
                const mainContextWindow = config.model?.params?.contextWindow || this.getDefaultContextWindow(chat);
                const modelMaxContextWindow = this.getDefaultContextWindow({config: {model: modelConfig}});
                modelConfig.params.contextWindow = Math.min(mainContextWindow, modelMaxContextWindow);
            }
            
            config.optimisation.titleGeneration.model = modelConfig;
            
            chat.config = config;
            this.recreateMessageOptimizer(chat, config);
            this.saveChatConfigSmart(chatId, config);
            this.autoSave(chatId);
        } catch (error) {
            console.error(`[updateTitleGenerationModel] Invalid model format for chatId: ${chatId}, model: ${model}`, error);
            this.showToast(`Invalid model format: ${model}. Expected format: provider:model`, 'error-toast');
        }
    }
    
    updateToolThreshold(chatId, threshold) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.toolSummarisation.thresholdKiB = Math.floor(threshold / 1024);
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        this.saveChatConfigSmart(chatId, config);
        this.autoSave(chatId);
    }
    
    updateAutoSumThreshold(chatId, percent) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.autoSummarisation.triggerPercent = percent;
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        this.saveChatConfigSmart(chatId, config);
        this.autoSave(chatId);
    }
    
    updateToolMemoryThreshold(chatId, forgetAfterConclusions) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.toolMemory.forgetAfterConclusions = forgetAfterConclusions;
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        this.saveChatConfigSmart(chatId, config);
        this.autoSave(chatId);
    }
    
    recreateMessageOptimizer(chat, config) {
        // Add factory for tool summarization if enabled
        const optimizerSettings = {
            ...config,
            llmProviderFactory: config.optimisation.toolSummarisation.enabled ? window.createLLMProvider : undefined
        };
        
        try {
            chat.messageOptimizer = new MessageOptimizer(optimizerSettings);
        } catch (error) {
            console.error('[recreateMessageOptimizer] Failed to create MessageOptimizer:', error);
        }
    }

    
    populateMCPDropdown(chatId, dropdown = null) {
        if (!chatId) {
            console.error('[populateMCPDropdown] Called without chatId');
            return;
        }
        const targetChatId = chatId;
        const targetDropdown = dropdown || this.mcpServerDropdown;
        
        targetDropdown.innerHTML = '';
        
        // Sort servers by name
        const sortedServers = Array.from(this.mcpServers.entries())
            .sort(([, a], [, b]) => a.name.localeCompare(b.name));
        
        for (const [id, server] of sortedServers) {
            const item = document.createElement('button');
            item.className = 'dropdown-item';
            
            // Check actual connection status from mcpConnections
            const mcpConnection = this.mcpConnections.get(id);
            const isConnected = mcpConnection && mcpConnection.isReady();
            
            item.innerHTML = `
                <div style="display: flex; align-items: flex-start; gap: 8px;">
                    <span style="flex-shrink: 0;">${isConnected ? '🟢' : '🔴'}</span>
                    <div style="flex: 1; min-width: 0;">
                        <div>${server.name}</div>
                        <small style="display: block; color: var(--text-tertiary); font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">${server.url}</small>
                    </div>
                </div>
            `;
            
            const chat = this.chats.get(targetChatId);
            if (chat && chat.mcpServerId === id) {
                item.classList.add('active');
            }
            
            item.onclick = () => {
                this.switchMcpServer(id, targetChatId).catch(error => {
                    console.error('Failed to switch MCP server:', error);
                    this.showError('Failed to switch MCP server', targetChatId, false);
                });
                targetDropdown.style.display = 'none';
            };
            
            targetDropdown.appendChild(item);
        }
    }
    
    async switchMcpServer(newServerId, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || chat.mcpServerId === newServerId) {return;}
        
        try {
            // Ensure connection to new MCP server
            await this.ensureMcpConnection(newServerId);
            
            chat.mcpServerId = newServerId;
            this.autoSave(chat.id);
            
            // Update UI
            const server = this.mcpServers.get(newServerId);
            this.currentMcpText.textContent = server.name;
            
            // Clear tool inclusion states for the new server (will be populated on next use)
            const chatToolStates = this.toolInclusionStates.get(chatId);
            if (chatToolStates) {
                chatToolStates.clear();
            }
            
            // Save the updated config as default for new chats
            if (chat.config) {
                const updatedConfig = { ...chat.config };
                updatedConfig.mcpServer = newServerId;
                ChatConfig.saveLastConfig(updatedConfig);
            }
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `Switched to MCP server: ${server.name}`
            });
            
        } catch (error) {
            this.showError(`Failed to switch MCP server: ${error.message}`, chatId, false);
        }
    }

    saveSystemPrompt(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        const newPrompt = this.systemPromptTextarea.value.trim();
        if (!newPrompt) {
            this.showError('System prompt cannot be empty', chatId);
            return;
        }
        
        // Check if prompt actually changed
        if (newPrompt === chat.systemPrompt) {
            this.hideModal('systemPromptModal');
            return;
        }
        
        // Update the chat's system prompt
        chat.systemPrompt = newPrompt;
        
        // Clear messages and reset the conversation
        chat.messages = [];
        chat.updatedAt = new Date().toISOString();
        
        // Save the new prompt as the last used one
        this.lastSystemPrompt = newPrompt;
        localStorage.setItem('lastSystemPrompt', newPrompt);
        
        // Clear token usage history for this chat
        this.tokenUsageHistory.set(chatId, {
            requests: [],
            model: ChatConfig.getChatModelString(chat)
        });
        
        // Save settings
        this.saveSettings();
        
        // Reload the chat (force refresh since we cleared messages)
        this.loadChat(chatId, true);
        
        // Hide modal
        this.hideModal('systemPromptModal');
        
        // Show notification
        this.addSystemMessage('System prompt updated. Conversation has been reset.', chatId);
    }

    toggleTheme() {
        const html = document.documentElement;
        const currentTheme = html.getAttribute('data-theme');
        const newTheme = currentTheme === 'light' ? 'dark' : 'light';
        html.setAttribute('data-theme', newTheme);
        localStorage.setItem('theme', newTheme);
        
        // Theme switching for tooltips is now handled by CSS variables
    }
    initializeResizable() {
        // Chat sidebar resize
        const chatSidebar = document.getElementById('chatSidebar');
        const chatSidebarResize = document.getElementById('chatSidebarResize');
        
        // Make sure the resize handle exists
        if (!chatSidebarResize) {
            console.error('Chat sidebar resize handle not found');
        } else {
            this.setupResize(chatSidebarResize, 'horizontal', (delta) => {
                const isCollapsed = chatSidebar.classList.contains('collapsed');
                const currentWidth = chatSidebar.offsetWidth;
                const newWidth = currentWidth + delta;
                
                // If collapsed and dragging to expand (delta > 0)
            if (isCollapsed && newWidth > 100) {
                // Expand the sidebar
                chatSidebar.classList.remove('collapsed');
                const icon = this.toggleSidebarBtn.querySelector('i');
                icon.className = 'fas fa-chevron-left';
                localStorage.setItem('chatSidebarCollapsed', 'false');
                
                // Set the new width
                const finalExpandWidth = Math.max(200, Math.min(400, newWidth));
                chatSidebar.style.setProperty('width', finalExpandWidth + 'px', 'important');
                chatSidebar.style.setProperty('min-width', finalExpandWidth + 'px', 'important');
                chatSidebar.style.setProperty('max-width', finalExpandWidth + 'px', 'important');
            }
            // If expanded and dragging to collapse (width getting too small)
            else if (!isCollapsed && newWidth < 100) {
                // Collapse the sidebar
                chatSidebar.classList.add('collapsed');
                const icon = this.toggleSidebarBtn.querySelector('i');
                icon.className = 'fas fa-chevron-left';
                localStorage.setItem('chatSidebarCollapsed', 'true');
                chatSidebar.style.width = '';
            }
            // Normal resize when expanded
            else if (!isCollapsed) {
                const finalWidth = Math.max(200, Math.min(400, newWidth));
                
                // Override all width-related CSS properties
                chatSidebar.style.setProperty('width', finalWidth + 'px', 'important');
                chatSidebar.style.setProperty('min-width', finalWidth + 'px', 'important');
                chatSidebar.style.setProperty('max-width', finalWidth + 'px', 'important');
            }
            
            this.savePaneSizes();
        });
        }

        // Log panel resize
        const logPanel = document.getElementById('logPanel');
        const logPanelResize = document.getElementById('logPanelResize');
        
        this.setupResize(logPanelResize, 'horizontal', (delta) => {
            // First, ensure the panel is not collapsed
            if (logPanel.classList.contains('collapsed')) {
                // Expand it first
                logPanel.classList.remove('collapsed');
                this.toggleLogBtn.innerHTML = '<i class="fas fa-chevron-right"></i>';
                this.expandLogBtn.style.display = 'none';
                localStorage.setItem('logCollapsed', 'false');
                // Set initial width when expanding
                logPanel.style.width = '300px';
            }
            
            const currentWidth = logPanel.offsetWidth;
            // For right panel, dragging left (negative delta) should increase width
            const newWidth = Math.max(200, Math.min(650, currentWidth + -delta));
            logPanel.style.width = newWidth + 'px';
            this.savePaneSizes();
        }, logPanel);

        // Chat input resize is now handled per-chat in the createChatDOM method
        // No global chat input container exists anymore
    }

    setupResize(handle, direction, onResize, element) {
        if (!handle) {
            console.warn('setupResize called with null handle');
            return;
        }
        
        let isResizing = false;
        let startPos = 0;
        
        const startResize = (e) => {
            isResizing = true;
            startPos = direction === 'horizontal' ? e.clientX : e.clientY;
            document.body.style.cursor = direction === 'horizontal' ? 'col-resize' : 'row-resize';
            document.body.style.userSelect = 'none';
            e.preventDefault();
            
            // Add active class for visual feedback
            handle.classList.add('resize-active');
            
            // Add resizing class to element if provided
            if (element) {
                element.classList.add('resizing');
            }
        };
        
        const doResize = (e) => {
            if (!isResizing) {return;}
            
            const currentPos = direction === 'horizontal' ? e.clientX : e.clientY;
            const delta = currentPos - startPos;
            startPos = currentPos;
            
            onResize(delta);
        };
        
        const stopResize = () => {
            if (!isResizing) {return;}
            isResizing = false;
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
            
            // Remove active class
            handle.classList.remove('resize-active');
            
            // Remove resizing class from element if provided
            if (element) {
                element.classList.remove('resizing');
            }
        };
        
        handle.addEventListener('mousedown', startResize);
        document.addEventListener('mousemove', doResize);
        document.addEventListener('mouseup', stopResize);
        
        // Also handle mouse leave to stop resize
        document.addEventListener('mouseleave', stopResize);
    }
    
    makeResizable(handle, container, direction, minSize, maxSize) {
        if (!handle || !container) {
            console.warn('makeResizable called with null handle or container');
            return;
        }
        
        this.setupResize(handle, direction, (delta) => {
            const isVertical = direction === 'vertical';
            const currentSize = isVertical ? container.offsetHeight : container.offsetWidth;
            const newSize = Math.max(minSize || 100, Math.min(maxSize || 1000, currentSize + (isVertical ? -delta : delta)));
            
            if (isVertical) {
                container.style.height = newSize + 'px';
            } else {
                container.style.width = newSize + 'px';
            }
        }, container);
    }

    savePaneSizes() {
        const sizes = {
            chatSidebar: this.chatSidebar ? this.chatSidebar.offsetWidth : 280,
            logPanel: this.logPanel ? this.logPanel.classList.contains('collapsed') ? 40 : this.logPanel.offsetWidth : 300,
            logPanelCollapsed: this.logPanel ? this.logPanel.classList.contains('collapsed') : false
        };
        localStorage.setItem('paneSizes', JSON.stringify(sizes));
    }

    loadPaneSizes() {
        const savedSizes = localStorage.getItem('paneSizes');
        if (savedSizes) {
            try {
                const sizes = JSON.parse(savedSizes);
                
                if (sizes.chatSidebar && this.chatSidebar && !this.chatSidebar.classList.contains('collapsed')) {
                    this.chatSidebar.style.width = sizes.chatSidebar + 'px';
                }
                
                if (sizes.logPanel && this.logPanel) {
                    // Only set width if the panel is not currently collapsed
                    if (!this.logPanel.classList.contains('collapsed')) {
                        this.logPanel.style.width = sizes.logPanel + 'px';
                    }
                }
                
                // Chat input container sizing is now handled per-chat, skip global sizing
            } catch (e) {
                console.error('Failed to load pane sizes:', e);
            }
        }
    }

    toggleLog() {
        const isCollapsed = this.logPanel.classList.toggle('collapsed');
        this.toggleLogBtn.innerHTML = isCollapsed ? '<i class="fas fa-chevron-left"></i>' : '<i class="fas fa-chevron-right"></i>';
        this.expandLogBtn.style.display = isCollapsed ? 'block' : 'none';
        localStorage.setItem('logCollapsed', String(isCollapsed));
        this.savePaneSizes();
    }
    
    toggleChatSidebar() {
        const isCollapsed = this.chatSidebar.classList.toggle('collapsed');
        
        // Update button icon - always keep as chevron-left, CSS handles rotation when collapsed
        const icon = this.toggleSidebarBtn.querySelector('i');
        icon.className = 'fas fa-chevron-left';
        
        if (isCollapsed) {
            // Store current width before collapsing
            const currentWidth = this.chatSidebar.offsetWidth;
            if (currentWidth > 40) {
                localStorage.setItem('chatSidebarWidth', String(currentWidth));
            }
            // Override any inline width when collapsed
            this.chatSidebar.style.width = '';
        } else {
            // Restore previous width
            const savedWidth = localStorage.getItem('chatSidebarWidth') || '280';
            this.chatSidebar.style.width = savedWidth + 'px';
        }
        
        localStorage.setItem('chatSidebarCollapsed', String(isCollapsed));
        this.savePaneSizes();
    }
    
    loadSidebarStates() {
        // Load chat sidebar state
        const chatSidebarCollapsed = localStorage.getItem('chatSidebarCollapsed') === 'true';
        if (chatSidebarCollapsed) {
            this.chatSidebar.classList.add('collapsed');
            // Note: CSS rotates the icon 180deg when collapsed, so keep it as chevron-left
            const icon = this.toggleSidebarBtn.querySelector('i');
            icon.className = 'fas fa-chevron-left';
        }
        
        // Load log panel state
        const logCollapsed = localStorage.getItem('logCollapsed') === 'true';
        if (logCollapsed) {
            this.logPanel.classList.add('collapsed');
            this.toggleLogBtn.innerHTML = '<i class="fas fa-chevron-left"></i>';
            this.expandLogBtn.style.display = 'block';
        }
    }

    handleRateLimitError(chatId, retryAfterSeconds, retryCount = 0) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        // Store retry count in chat for tracking
        chat.rateLimitRetryCount = retryCount;
        
        // If we couldn't parse retry time, use exponential backoff
        let waitTime;
        if (retryAfterSeconds && retryAfterSeconds > 0) {
            // Add a small buffer to ensure we wait long enough
            waitTime = retryAfterSeconds + 1;
        } else {
            // Exponential backoff: 5s, 10s, 20s, 40s, 80s...
            waitTime = Math.min(5 * Math.pow(2, retryCount), 120); // Cap at 2 minutes
            console.log(`[Rate Limit] No retry time found, using exponential backoff: ${waitTime}s (attempt ${retryCount + 1})`);
        }
        
        let remainingSeconds = Math.ceil(waitTime);
        
        // Mark that we're in rate limit countdown - this prevents other operations from clearing the spinner
        chat.isInRateLimitCountdown = true;
        
        // Show waiting spinner with countdown immediately
        this.showWaitingCountdown(chatId, remainingSeconds);
        
        const updateCountdown = () => {
            remainingSeconds--;
            if (remainingSeconds > 0) {
                // Only update if we're still in countdown mode
                if (chat.isInRateLimitCountdown) {
                    this.updateWaitingCountdown(chatId, remainingSeconds);
                    setTimeout(updateCountdown, 1000);
                }
            } else {
                // Clear the flag and retry
                chat.isInRateLimitCountdown = false;
                // Don't hide the countdown - let retryLLMRequest transition to thinking spinner
                // This ensures there's no gap in the spinner display
                this.retryLLMRequest(chatId);
            }
        };
        
        // Start the countdown
        setTimeout(updateCountdown, 1000);
    }
    
    async retryLLMRequest(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        try {
            const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
            const proxyProvider = this.llmProviders.get(chat.llmProviderId);
            
            if (!mcpConnection || !proxyProvider || !chat.config?.model) {
                this.showError('Cannot retry: MCP server or LLM provider not available', chatId);
                return;
            }
            
            // Create provider instance
            const providerType = chat.config.model.provider;
            const modelName = chat.config.model.id;
            
            // Get the API type from the provider configuration
            const providerApiType = proxyProvider.availableProviders?.[providerType]?.type || providerType;
            
            const provider = createLLMProvider(providerApiType, proxyProvider.proxyUrl, modelName, chat.config.model);
            provider.onLog = (logEntry) => {
                const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
                const providerName = providerType.charAt(0).toUpperCase() + providerType.slice(1);
                this.addLogEntry(`${prefix}: ${providerName}`, logEntry);
            };
            
            // Build messages from current state
            const { messages, cacheControlIndex } = this.buildMessagesForAPI(chat, provider.prefersCachedTools, mcpConnection);
            
            // Get available tools
            const tools = Array.from(mcpConnection.tools.values());
            
            // Increment retry count for next attempt
            const currentRetryCount = chat.rateLimitRetryCount || 0;
            chat.rateLimitRetryCount = currentRetryCount + 1;
            
            // Call assistant with proper error handling
            const temperature = this.getCurrentTemperature(chatId);
            const response = await this.callAssistant({
                chatId,
                provider,
                messages,
                tools,
                temperature,
                cacheControlIndex,
                context: `Retry (attempt ${chat.rateLimitRetryCount})`
            });
            
            // Check if rate limit was handled
            if (response._rateLimitHandled) {
                return; // Rate limit retry will happen automatically with exponential backoff
            }
            
            // Success - reset retry count
            chat.rateLimitRetryCount = 0;
            
            // Process the response - extract the core loop logic from processMessageWithTools
            // This continues the conversation from where it left off
            await this.processLLMResponseLoop(chat, mcpConnection, provider, messages, tools, cacheControlIndex, response);
            
            // Success - assistant has concluded
            this.assistantConcluded(chatId);
            
        } catch (error) {
            // Clean up on error - but preserve rate limit waiting spinner
            this.assistantFailed(chatId, error);
            
            // Only show error if not a handled rate limit
            if (!error._rateLimitHandled) {
                // Show error with manual retry button
                const errorMessage = `${error.context || 'Error'}: ${error.message}`;
                const lastUserMessageIndex = chat.messages.findLastIndex(m => m.role === 'user');
                
                // Determine error type
                let errorType = 'llm_error';
                if (error.message.includes('MCP') || error.message.includes('connection')) {
                    errorType = 'mcp_error';
                } else if (error.message.includes('Tool')) {
                    errorType = 'tool_error';
                }
                
                // Update error state
                this.showError(errorMessage, chatId, false, errorType);
                
                // Add error message
                this.addMessage(chatId, { 
                    role: 'error', 
                    content: errorMessage, 
                    errorMessageIndex: lastUserMessageIndex,
                    errorType
                });
                
                this.processRenderEvent({ 
                    type: 'error-message', 
                    content: errorMessage, 
                    errorMessageIndex: lastUserMessageIndex,
                    errorType
                }, chatId);
            }
        }
    }
    
    async processLLMResponseLoop(chat, mcpConnection, provider, messages, tools, cacheControlIndex, initialResponse = null) {
        // If we have an initial response (from retry), process it first
        if (initialResponse) {
            await this.processSingleLLMResponse(chat, mcpConnection, provider, messages, tools, cacheControlIndex, initialResponse);
        }
        
        // Continue the loop
        while (true) {
            // Check if we should stop processing for this chat
            if (chat.shouldStopProcessing) {
                break;
            }
            
            // Check if the last response had tool calls
            const lastMessage = messages[messages.length - 1];
            if (!lastMessage || lastMessage.role !== 'tool-results') {
                // No more tool results to process, we're done
                break;
            }
            
            // Safety check: Check iteration limit before continuing
            try {
                // Only check iteration limit here, not request size
                // Request size will be checked in the provider when actual request is built
                const currentIterations = this.safetyChecker.getIterationCount(chat.id);
                if (currentIterations >= SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS) {
                    throw new SafetyLimitError('ITERATIONS', SAFETY_LIMITS.ERRORS.TOO_MANY_ITERATIONS(currentIterations, SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS));
                }
            } catch (error) {
                if (error instanceof SafetyLimitError) {
                    this.addMessage(chat.id, { 
                        role: 'error', 
                        content: error.message,
                        errorType: 'safety_limit',
                        isRetryable: false
                    });
                    this.processRenderEvent({ 
                        type: 'error-message', 
                        content: error.message, 
                        errorType: 'safety_limit'
                    }, chat.id);
                    return;
                }
                throw error;
            }
            
            // Send next request to LLM
            const temperature = this.getCurrentTemperature(chat.id);
            // eslint-disable-next-line no-await-in-loop
            const response = await this.callAssistant({
                chatId: chat.id,
                provider,
                messages,
                tools,
                temperature,
                cacheControlIndex,
                context: 'Processing tools'
            });
            
            // Check if rate limit was handled automatically
            if (response._rateLimitHandled) {
                return { rateLimitHandled: true };
            }
            
            // Process the response
            // eslint-disable-next-line no-await-in-loop
            await this.processSingleLLMResponse(chat, mcpConnection, provider, messages, tools, cacheControlIndex, response);
        }
    }
    
    async processSingleLLMResponse(chat, mcpConnection, provider, messages, tools, cacheControlIndex, response) {
        const llmResponseTime = response._responseTime || 0;
        
        // Track token usage
        if (response.usage) {
            this.updateTokenUsage(chat.id, response.usage, ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`);
        }
        
        // If no tool calls, display response and finish
        console.log('[processSingleLLMResponse] Response content:', JSON.stringify(response.content, null, 2));
        console.log('[processSingleLLMResponse] Response content is array:', Array.isArray(response.content));
        const toolsInContent = this.extractToolsFromContent(response.content);
        console.log('[processSingleLLMResponse] Extracted tools:', toolsInContent);
        if (toolsInContent.length === 0) {
            // Emit metrics event first
            this.processRenderEvent({ 
                type: 'assistant-metrics', 
                usage: response.usage, 
                responseTime: llmResponseTime, 
                model: ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`
            }, chat.id);
            
            if (response.content) {
                // Create and save the assistant message
                const assistantMsg = { 
                    role: 'assistant', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`,
                    turn: chat.currentTurn
                };
                this.addMessage(chat.id, assistantMsg);
                
                // Display it
                const messageIndex = chat.messages.length - 1;
                this.processRenderEvent({ 
                    type: 'assistant-message', 
                    content: response.content,
                    messageIndex
                }, chat.id);
                
                // Track cumulative tokens
                if (response.usage) {
                    const modelUsed = ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`;
                    this.addCumulativeTokens(
                        chat.id,
                        modelUsed,
                        response.usage.promptTokens || 0,
                        response.usage.completionTokens || 0,
                        response.usage.cacheReadInputTokens || 0,
                        response.usage.cacheCreationInputTokens || 0
                    );
                }
                
                // Clean and add to messages for API
                const cleanedContent = this.cleanContentForAPI(response.content);
                // Always add assistant message when no tool calls, even if content is empty
                // This prevents infinite loops when LLM responds with only thinking tags
                messages.push({ role: 'assistant', content: cleanedContent || '' });
            }
            return;
        }
        
        // Process response with tool calls
        let assistantMessageIndex = null;
        
        // Save assistant message first
        if (response.content || toolsInContent.length > 0) {
            const assistantMessage = {
                role: 'assistant',
                content: response.content || '',
                usage: response.usage || null,
                responseTime: llmResponseTime || null,
                model: ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`,
                turn: chat.currentTurn,
                cacheControlIndex
            };
            this.addMessage(chat.id, assistantMessage);
            assistantMessageIndex = chat.messages.length - 1;
            
            // Track cumulative tokens
            if (response.usage) {
                const modelUsed = ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`;
                this.addCumulativeTokens(
                    chat.id,
                    modelUsed,
                    response.usage.promptTokens || 0,
                    response.usage.completionTokens || 0,
                    response.usage.cacheReadInputTokens || 0,
                    response.usage.cacheCreationInputTokens || 0
                );
            }
            
            // Display metrics and content
            this.processRenderEvent({ 
                type: 'assistant-metrics', 
                usage: response.usage, 
                responseTime: llmResponseTime, 
                model: ChatConfig.getChatModelString(chat) || `${provider.type}:${provider.model}`
            }, chat.id);
            
            if (response.content) {
                this.processRenderEvent({ 
                    type: 'assistant-message', 
                    content: response.content,
                    messageIndex: assistantMessageIndex
                }, chat.id);
            }
            
            // Add to API messages - keep original content with tool calls
            const assistantMsg = {
                role: 'assistant',
                content: response.content || ''
            };
            
            // Only add if there's content or tool calls
            const cleanedContent = response.content ? this.cleanContentForAPI(response.content) : '';
            if (cleanedContent || this.extractToolsFromContent(response.content).length > 0) {
                messages.push(assistantMsg);
            }
        }
        
        // Execute tool calls - extract from content array
        const extractedTools = this.extractToolsFromContent(response.content);
        if (extractedTools.length > 0) {
            // Increment iteration counter when we have tool calls
            this.safetyChecker.incrementIterations(chat.id);
            
            // Safety check for concurrent tools
            try {
                this.safetyChecker.checkConcurrentToolsLimit(extractedTools);
            } catch (error) {
                if (error instanceof SafetyLimitError) {
                    this.addMessage(chat.id, { 
                        role: 'error', 
                        content: error.message,
                        errorType: 'safety_limit',
                        isRetryable: false
                    });
                    this.processRenderEvent({ 
                        type: 'error-message', 
                        content: error.message, 
                        errorType: 'safety_limit'
                    }, chat.id);
                    return;
                }
                throw error;
            }
            
            // Ensure assistant group exists
            if (!this.getCurrentAssistantGroup(chat.id)) {
                this.processRenderEvent({ 
                    type: 'assistant-message', 
                    content: '',
                    messageIndex: assistantMessageIndex
                }, chat.id);
            }
            
            // Execute tools and collect results
            const toolResults = await this.executeToolCalls(chat, mcpConnection, extractedTools, assistantMessageIndex);
            
            // Store tool results
            if (toolResults.length > 0) {
                this.addMessage(chat.id, {
                    role: 'tool-results',
                    toolResults,
                    turn: chat.currentTurn
                });
                
                // CRITICAL: Now that tool-results are added, update sub-chat costs
                for (const toolResult of toolResults) {
                    if (toolResult.subChatId && toolResult.wasProcessedBySubChat) {
                        const subChat = this.chats.get(toolResult.subChatId);
                        if (subChat) {
                            // Ensure sub-chat has up-to-date token pricing
                            this.updateChatTokenPricing(subChat);
                            // Store costs in parent tool-result
                            this.updateParentToolResultCosts(chat.id, toolResult.toolCallId, subChat);
                        } else {
                            console.error(`[processSingleLLMResponse] ERROR: Sub-chat ${toolResult.subChatId} not found when trying to update parent costs for tool ${toolResult.toolCallId}`);
                        }
                    }
                }
                
                // CRITICAL FIX: Update parent chat token pricing to include all sub-chat costs
                this.updateChatTokenPricing(chat);
                this.updateAllTokenDisplays(chat.id);
                
                // Add to messages for API
                const includedResults = toolResults.filter(tr => tr.includeInContext !== false);
                if (includedResults.length > 0) {
                    const toolResultsMessage = {
                        role: 'tool-results',
                        toolResults: includedResults.map(tr => ({
                            toolCallId: tr.toolCallId,
                            toolName: tr.name,
                            result: tr.result
                        }))
                    };
                    
                    messages.push(toolResultsMessage);
                    
                    // Sub-chats are now processed immediately during tool execution (interleaved)
                }
                
                // Reset assistant group and show thinking spinner for next iteration
                this.processRenderEvent({ type: 'reset-assistant-group' }, chat.id);
                this.showAssistantThinking(chat.id);
            }
        }
    }
    
    async executeToolCalls(chat, mcpConnection, toolCalls, assistantMessageIndex) {
        const toolResults = [];
        
        for (const toolCall of toolCalls) {
            if (!toolCall.id) {
                console.error('[executeToolCalls] Tool call missing required id:', toolCall);
                continue;
            }
            
            try {
                const { arguments: toolArgs } = toolCall || {};
                
                // Show tool call in UI
                this.processRenderEvent({ 
                    type: 'tool-call', 
                    name: toolCall.name, 
                    arguments: toolArgs,
                    id: toolCall.id,
                    includeInContext: toolCall.includeInContext !== false 
                }, chat.id);
                
                // Show tool execution spinner
                this.showToolExecuting(chat.id, toolCall.name);
                
                // Execute tool
                const toolStartTime = Date.now();
                // eslint-disable-next-line no-await-in-loop
                const rawResult = await mcpConnection.callTool(toolCall.name, toolArgs);
                const toolResponseTime = Date.now() - toolStartTime;
                
                // Hide tool execution spinner
                this.hideToolExecuting(chat.id);
                
                // Parse result
                const result = this.parseToolResult(rawResult);
                const responseSize = typeof result === 'string' 
                    ? result.length 
                    : JSON.stringify(result).length;
                
                // Check if we should create a sub-chat for this tool response
                // eslint-disable-next-line no-await-in-loop
                const shouldCreateSubChat = await this.shouldCreateSubChat(chat, responseSize, toolCall);

                // Show result only if we're not creating a sub-chat
                if (!shouldCreateSubChat) {
                    this.processRenderEvent({ 
                        type: 'tool-result', 
                        name: toolCall.name, 
                        result, 
                        toolCallId: toolCall.id,
                        responseTime: toolResponseTime, 
                        responseSize, 
                        messageIndex: assistantMessageIndex 
                    }, chat.id);
                }
                
                if (shouldCreateSubChat) {
                    console.log(`[executeToolCalls] Creating sub-chat for tool ${toolCall.id} (${toolCall.name})`);
                    
                    // Create sub-chat for processing this tool response
                    // eslint-disable-next-line no-await-in-loop
                    const subChatId = await this.createSubChatForTool(chat, toolCall, result);
                    
                    console.log(`[executeToolCalls] Created sub-chat ${subChatId} for tool ${toolCall.id}`);
                    
                    // Show secondary assistant waiting spinner for main chat
                    this.showSecondaryAssistantWaiting(chat.id);
                    
                    // Render sub-chat DOM BEFORE processing starts so users can see it populate
                    this.renderSubChatAsItem(chat.id, subChatId, toolCall.id, 'processing');
                    
                    // CRITICAL: Ensure the sub-chat container is available before processing
                    // The container should have been created by renderSubChatAsItem
                    const subChatContainer = this.chatContainers.get(subChatId);
                    if (!subChatContainer) {
                        console.error(`[executeToolCalls] Sub-chat container not found after renderSubChatAsItem for ${subChatId}`);
                        // Update status to failed since we can't process without a container
                        this.updateSubChatStatus(chat.id, toolCall.id, 'failed');
                        continue;
                    }
                    
                    // Process the sub-chat immediately (interleaved execution)
                    let summarizedResult = null;
                    try {
                        // eslint-disable-next-line no-await-in-loop
                        summarizedResult = await this.processSubChat(subChatId, chat.id, toolCall.id);
                        
                        if (!summarizedResult) {
                            console.warn('[Sub-chat Processing] No summarized result returned, keeping original');
                        } else {
                            console.log(`[Sub-chat Processing] Replacing tool result for ${toolCall.id} with summarized content (${summarizedResult.length} chars)`);
                        }
                        
                        // Update sub-chat status to final state
                        this.updateSubChatStatus(chat.id, toolCall.id, summarizedResult ? 'success' : 'failed');
                    } catch (error) {
                        console.error('[Sub-chat Processing] Failed:', error);
                        // Update sub-chat status to failed
                        this.updateSubChatStatus(chat.id, toolCall.id, 'failed');
                    }
                    
                    // Hide secondary assistant waiting spinner
                    this.hideSecondaryAssistantWaiting(chat.id);
                    
                    this.processRenderEvent({ 
                        type: 'tool-result', 
                        name: toolCall.name, 
                        result: summarizedResult || result, // Use summarized result if available
                        toolCallId: toolCall.id,
                        responseTime: toolResponseTime, 
                        responseSize, 
                        messageIndex: assistantMessageIndex,
                        subChatId,
                        wasProcessedBySubChat: !!summarizedResult
                    }, chat.id);
                    
                    // Add the processed tool result
                    const processedToolResult = {
                        toolCallId: toolCall.id,
                        name: toolCall.name,
                        result: summarizedResult || result, // Use summarized result if available
                        includeInContext: true,
                        subChatId, // Keep for tracking
                        wasProcessedBySubChat: !!summarizedResult,
                        subChatFailed: !summarizedResult
                    };
                    toolResults.push(processedToolResult);
                    
                    // Update the DOM display to show final state
                    this.updateToolResultDisplay(chat.id, toolCall.id, processedToolResult);
                    
                    // CRITICAL: Save the updated parent chat with summarized results
                    this.autoSave(chat.id);
                } else {
                    toolResults.push({
                        toolCallId: toolCall.id,
                        name: toolCall.name,
                        result,
                        includeInContext: true
                    });
                }
                
            } catch (error) {
                const errorMsg = `Tool error (${toolCall.name}): ${error.message}`;
                this.processRenderEvent({ 
                    type: 'tool-result', 
                    name: toolCall.name, 
                    result: { error: errorMsg }, 
                    responseTime: 0, 
                    responseSize: errorMsg.length, 
                    messageIndex: assistantMessageIndex, 
                    toolCallId: toolCall.id 
                }, chat.id);
                
                toolResults.push({
                    toolCallId: toolCall.id,
                    name: toolCall.name,
                    result: { error: errorMsg },
                    includeInContext: true
                });
            }
        }
        
        return toolResults;
    }
    
    /**
     * Centralized function to call the assistant API with consistent error handling
     * @param {Object} params - Parameters for the assistant call
     * @param {string} params.chatId - Chat ID
     * @param {Object} params.provider - LLM provider instance
     * @param {Array} params.messages - Messages array
     * @param {Array} params.tools - Available tools
     * @param {number} params.temperature - Temperature setting
     * @param {number} params.cacheControlIndex - Cache control index
     * @param {string} params.context - Context for error messages (e.g., 'Redo', 'Retry', 'Send')
     * @returns {Promise<Object>} Response from the assistant
     */
    async callAssistant({ chatId, provider, messages, tools, temperature, cacheControlIndex, context = 'Request' }) {
        if (!chatId) {
            throw new Error('[callAssistant] Missing required chatId parameter');
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            throw new Error(`[callAssistant] Chat not found: ${chatId}`);
        }
        
        // Show thinking spinner
        this.showAssistantThinking(chatId);
        
        try {
            // Track timing
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, tools, temperature, chat.config.optimisation.cacheControl || 'all-off', cacheControlIndex, chat);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Store response time
            response._responseTime = llmResponseTime;
            
            // Hide spinner on success
            this.hideAssistantThinking(chatId);
            
            return response;
            
        } catch (error) {
            // Check for rate limit error FIRST before hiding spinner
            const isRateLimitError = error.message && (
                error.message.includes('Rate limit') || 
                error.message.includes('429') ||
                error.message.includes('rate_limit_exceeded') ||
                error.message.includes('529') ||
                error.message.includes('overloaded_error') ||
                error.message.includes('Overloaded')
            );
            
            // Extract retry-after seconds if available (handles multiple formats)
            let retryAfterSeconds = null;
            
            // Try different patterns
            const patterns = [
                /Please try again in (\d+(?:\.\d+)?)s/,  // "Please try again in 4.742s"
                /Please retry after (\d+) second/,        // "Please retry after 5 seconds"
                /try again in (\d+(?:\.\d+)?) second/i   // Various formats
            ];
            
            for (const pattern of patterns) {
                const match = error.message && error.message.match(pattern);
                if (match) {
                    retryAfterSeconds = parseFloat(match[1]);
                    break;
                }
            }
            
            if (isRateLimitError) {
                // Always handle rate limit errors, even without retry time
                // Don't hide spinner - let handleRateLimitError manage the transition
                const retryCount = chat.rateLimitRetryCount || 0;
                this.handleRateLimitError(chatId, retryAfterSeconds, retryCount);
                // Return a special marker to indicate rate limit handling
                return { _rateLimitHandled: true };
            }
            
            // Only hide spinner for non-rate-limit errors
            this.hideAssistantThinking(chatId);
            
            // Add context to error
            error.context = context;
            throw error;
        }
    }
    
    /**
     * Called when the assistant has finished processing and no more actions will occur
     * Ensures proper cleanup of UI state and chat processing flags
     */
    assistantConcluded(chatId) {
        if (!chatId) {
            console.error('[assistantConcluded] Called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[assistantConcluded] Chat not found: ${chatId}`);
            return;
        }
        
        // Clear any spinners
        this.clearSpinnerState(chatId);
        
        // Clear processing states
        chat.isProcessing = false;
        
        // Clear stop-related flags to ensure they're ready for next time
        chat.shouldStopProcessing = false;
        chat.processingWasStoppedByUser = false;
        
        // Clear error state on successful conclusion
        this.clearError(chatId);
        
        // Clear current assistant group
        this.clearCurrentAssistantGroup(chatId);
        
        // Reset safety checker iterations for next user message
        this.safetyChecker.resetIterations(chatId);
        
        // Update only this chat's tile
        this.updateChatTileStatus(chatId);
        
        // Save the chat state
        chat.updatedAt = new Date().toISOString();
        this.autoSave(chatId);
        
        // Always re-enable input for the chat that concluded
        const container = this.getChatContainer(chatId);
        if (container && container._elements) {
            const input = container._elements.input;
            if (input) {
                // Always re-enable contentEditable
                input.contentEditable = true;
                
                // Only focus if this is the active chat
                if (chatId === this.getActiveChatId()) {
                    input.focus();
                }
            }
            
            // Update send button state
            const sendBtn = container._elements.sendBtn;
            if (sendBtn && input) {
                try {
                    const content = this.getEditableContent(input).trim();
                    sendBtn.disabled = !content;
                } catch (error) {
                    console.error('[assistantConcluded] ERROR getting editable content:', error);
                    sendBtn.disabled = true;
                }
            }
        }
    }
    
    /**
     * Called when the assistant fails with an error
     * Handles cleanup differently based on error type
     */
    assistantFailed(chatId, error = null) {
        if (!chatId) {
            console.error('[assistantFailed] Called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[assistantFailed] Chat not found: ${chatId}`);
            return;
        }
        
        // Check if this is a rate limit error that's being handled
        const isRateLimitHandled = error && error._rateLimitHandled;
        
        // Only clear spinners if NOT a handled rate limit error
        if (!isRateLimitHandled) {
            this.clearSpinnerState(chatId);
        }
        // If rate limit is handled, the waiting spinner should continue
        
        // Clear processing states
        chat.isProcessing = false;
        
        // Clear stop-related flags when failure is handled
        chat.shouldStopProcessing = false;
        chat.processingWasStoppedByUser = false;
        
        // Clear current assistant group
        this.clearCurrentAssistantGroup(chatId);
        
        // Reset safety checker iterations for next user message
        this.safetyChecker.resetIterations(chatId);
        
        // Update only this chat's tile
        this.updateChatTileStatus(chatId);
        
        // Save the chat state
        chat.updatedAt = new Date().toISOString();
        this.autoSave(chatId);
        
        // Re-enable input if not rate limited (always for the chat that failed)
        if (!isRateLimitHandled) {
            const container = this.getChatContainer(chatId);
            if (container && container._elements) {
                const input = container._elements.input;
                if (input) {
                    // Always re-enable contentEditable
                    input.contentEditable = true;
                    
                    // Only focus if this is the active chat
                    if (chatId === this.getActiveChatId()) {
                        input.focus();
                    }
                }
            }
        }
    }
    
    /**
     * Shows a professional confirmation dialog modal
     * @param {string} title - Dialog title
     * @param {string} message - Confirmation message
     * @param {string} confirmText - Text for confirm button (default: 'OK')
     * @param {string} cancelText - Text for cancel button (default: 'Cancel')
     * @param {boolean} isDanger - Whether this is a dangerous action (shows red confirm button)
     * @returns {Promise<boolean>} - Resolves to true if confirmed, false if cancelled
     */
    showConfirmDialog(title, message, confirmText = 'OK', cancelText = 'Cancel', isDanger = false) {
        return new Promise((resolve) => {
            // Create modal container
            const modal = document.createElement('div');
            modal.className = 'confirm-modal-container';
            modal.innerHTML = `
                <div class="confirm-modal-backdrop"></div>
                <div class="confirm-modal">
                    <div class="confirm-modal-header">
                        <h3>${this.escapeHtml(title)}</h3>
                    </div>
                    <div class="confirm-modal-body">
                        <p>${this.escapeHtml(message)}</p>
                    </div>
                    <div class="confirm-modal-footer">
                        <button class="btn btn-secondary confirm-cancel">${this.escapeHtml(cancelText)}</button>
                        <button class="btn ${isDanger ? 'btn-danger' : 'btn-primary'} confirm-ok">${this.escapeHtml(confirmText)}</button>
                    </div>
                </div>
            `;
            
            document.body.appendChild(modal);
            
            // Focus the confirm button
            const confirmBtn = modal.querySelector('.confirm-ok');
            const cancelBtn = modal.querySelector('.confirm-cancel');
            confirmBtn.focus();
            
            // Define all functions using function declarations to avoid hoisting issues
            function cleanup() {
                modal.remove();
                document.removeEventListener('keydown', handleKeydown);
            }
            
            function handleConfirm() {
                cleanup();
                resolve(true);
            }
            
            function handleCancel() {
                cleanup();
                resolve(false);
            }
            
            function handleKeydown(e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    handleConfirm();
                } else if (e.key === 'Escape') {
                    e.preventDefault();
                    handleCancel();
                }
            }
            
            // Add event listeners
            confirmBtn.addEventListener('click', handleConfirm);
            cancelBtn.addEventListener('click', handleCancel);
            modal.querySelector('.confirm-modal-backdrop').addEventListener('click', handleCancel);
            document.addEventListener('keydown', handleKeydown);
        });
    }
    
    /**
     * Escapes HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    showError(message, chatId, saveToMessages = true, errorType = 'general') {
        // Log to console
        console.error('MCP Client Error:', message, chatId ? `(Chat ID: ${chatId})` : '(Global)');
        
        // Show error toast
        const toast = document.createElement('div');
        toast.className = 'error-toast';
        toast.textContent = message;
        document.getElementById('errorToastContainer').appendChild(toast);
        
        // Remove after animation
        setTimeout(() => toast.remove(), 3000);
        
        // Log error
        this.addLogEntry('ERROR', {
            timestamp: new Date().toISOString(),
            direction: 'error',
            message
        });
        
        // Also show in chat if chatId is provided
        if (chatId) {
            const chat = this.chats.get(chatId);
            
            if (chat) {
                // Clear any spinners (from second method)
                this.clearSpinnerState(chatId);
                
                // Save to messages if requested
                if (saveToMessages) {
                    const lastUserMessageIndex = chat.messages.findLastIndex(m => m.role === 'user');
                    this.addMessage(chatId, { 
                        role: 'error', 
                        content: message, 
                        errorMessageIndex: lastUserMessageIndex,
                        timestamp: new Date().toISOString()
                    });
                }
                
                // Set error state with structured error object (enhanced from second method)
                chat.hasError = true;
                chat.lastError = {
                    message,
                    type: errorType,
                    timestamp: Date.now()
                };
                
                // Update UI
                this.updateChatSessions();
                this.updateChatTileStatus(chatId);
            }
            
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                messageDiv.innerHTML = `<i class="fas fa-times-circle"></i> ${message}`;
                container._elements.messages.appendChild(messageDiv);
                
                // Scroll to bottom using the chat-specific container
                container._elements.messages.scrollTop = container._elements.messages.scrollHeight;
            }
        }
    }
    
    // Global errors (not specific to any chat)
    showGlobalError(message) {
        this.showError(message, null);
    }
    
    showToast(message, className = 'error-toast') {
        // Show toast notification only (no chat message)
        const toast = document.createElement('div');
        toast.className = className;
        toast.textContent = message;
        document.getElementById('errorToastContainer').appendChild(toast);
        
        // Remove after animation
        setTimeout(() => toast.remove(), 3000);
    }
    
    showErrorWithRetry(message, retryCallback, buttonLabel, chatId) {
        // Show error toast
        const toast = document.createElement('div');
        toast.className = 'error-toast';
        toast.textContent = message;
        document.getElementById('errorToastContainer').appendChild(toast);
        
        // Remove after animation
        setTimeout(() => toast.remove(), 3000);
        
        // Log error
        this.addLogEntry('ERROR', {
            timestamp: new Date().toISOString(),
            direction: 'error',
            message
        });
        
        // Also show in chat with retry button if chatId is provided
        if (chatId) {
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                const buttonIcon = buttonLabel === 'Continue' ? 'fa-play' : 'fa-redo';
                messageDiv.innerHTML = `
                    <div><i class="fas fa-times-circle"></i> ${message}</div>
                    <button class="btn btn-warning btn-small" style="margin-top: 8px;">
                        <i class="fas ${buttonIcon}"></i> ${buttonLabel}
                    </button>
                `;
                
                const retryBtn = messageDiv.querySelector('button');
                retryBtn.onclick = async () => {
                    retryBtn.disabled = true;
                    retryBtn.textContent = 'Retrying...';
                    await retryCallback();
                };
                
                container._elements.messages.appendChild(messageDiv);
                
                // Scroll to bottom using the chat-specific container
                container._elements.messages.scrollTop = container._elements.messages.scrollHeight;
            }
        }
    }

    addLogEntry(source, entry) {
        const logEntry = {
            ...entry,
            source
        };
        this.communicationLog.push(logEntry);
        this.updateLogDisplay(logEntry);
    }

    updateLogDisplay(entry) {
        const entryDiv = document.createElement('div');
        entryDiv.className = 'log-entry';
        
        const directionClass = entry.direction;
        let directionSymbol;
        switch(entry.direction) {
            case 'sent': directionSymbol = '→'; break;
            case 'received': directionSymbol = '←'; break;
            case 'error': directionSymbol = '⚠'; break;
            case 'info': directionSymbol = 'ℹ'; break;
            default: directionSymbol = '•'; break;
        }
        
        let metadataHtml = '';
        if (entry.metadata && Object.keys(entry.metadata).length > 0) {
            metadataHtml = `<div class="log-metadata">`;
            for (const [key, value] of Object.entries(entry.metadata)) {
                metadataHtml += `<span class="metadata-item">${key}: ${value}</span>`;
            }
            metadataHtml += `</div>`;
        }
        
        // Create a unique ID for this entry
        const entryId = `log-entry-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`;
        
        // Create header with copy button
        const headerDiv = document.createElement('div');
        headerDiv.className = 'log-entry-header';
        
        const infoDiv = document.createElement('div');
        infoDiv.className = 'log-entry-info';
        infoDiv.innerHTML = `
            <span class="log-timestamp">${new Date(entry.timestamp).toLocaleTimeString()}</span>
            <span class="log-source">[${entry.source}]</span>
            <span class="log-direction ${directionClass}">${directionSymbol}</span>
        `;
        
        // Create copy button using standardized method
        const copyBtn = this.createCopyButton({
            buttonClass: 'btn-copy-log',
            onCopy: () => {
                const messageElement = document.getElementById(entryId);
                return messageElement.textContent || messageElement.innerText;
            }
        });
        copyBtn.setAttribute('data-entry-id', entryId);
        
        headerDiv.appendChild(infoDiv);
        headerDiv.appendChild(copyBtn);
        
        entryDiv.appendChild(headerDiv);
        
        // Add metadata if present
        if (metadataHtml) {
            const metadataDiv = document.createElement('div');
            metadataDiv.innerHTML = metadataHtml;
            entryDiv.appendChild(metadataDiv);
        }
        
        // Add message content
        const messageDiv = document.createElement('div');
        messageDiv.className = 'log-message';
        messageDiv.id = entryId;
        messageDiv.textContent = this.formatLogMessage(entry.message);
        entryDiv.appendChild(messageDiv);
        
        this.logContent.appendChild(entryDiv);
        
        // Only scroll if user is already near the bottom
        const threshold = 100; // pixels from bottom to consider "at bottom"
        const isAtBottom = this.logContent.scrollHeight - this.logContent.scrollTop - this.logContent.clientHeight < threshold;
        
        if (isAtBottom) {
            this.logContent.scrollTop = this.logContent.scrollHeight;
        }
    }

    formatLogMessage(message) {
        try {
            const parsed = JSON.parse(message);
            return JSON.stringify(parsed, null, 2);
        } catch {
            return message;
        }
    }

    // Shared clipboard utility that works in all contexts
    async writeToClipboard(text) {
        // Check if clipboard API is available
        if (navigator.clipboard && navigator.clipboard.writeText) {
            return navigator.clipboard.writeText(text);
        } else {
            // Fallback for older browsers or insecure contexts
            const textArea = document.createElement('textarea');
            textArea.value = text;
            textArea.style.position = 'fixed';
            textArea.style.left = '-999999px';
            textArea.style.top = '-999999px';
            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();
            
            try {
                const successful = document.execCommand('copy');
                if (!successful) {
                    throw new Error('Copy command failed');
                }
            } finally {
                textArea.remove();
            }
        }
    }

    // Create a standardized copy button
    createCopyButton(options = {}) {
        const {
            tooltip = 'Copy to clipboard',
            iconClass = 'fas fa-clipboard',
            buttonClass = '',
            onCopy = null
        } = options;

        const button = document.createElement('button');
        button.className = `copy-button ${buttonClass}`.trim();
        button.setAttribute('data-tooltip', tooltip);
        button.innerHTML = `<i class="${iconClass}"></i>`;
        
        button.addEventListener('click', async (e) => {
            e.stopPropagation();
            if (onCopy) {
                try {
                    const textToCopy = await onCopy();
                    if (textToCopy !== undefined && textToCopy !== null) {
                        await this.handleCopyButtonClick(button, textToCopy);
                    }
                } catch (error) {
                    console.error('Copy button error:', error);
                    this.showCopyButtonError(button);
                }
            }
        });
        
        return button;
    }

    // Handle copy button click with standardized feedback
    async handleCopyButtonClick(button, text) {
        const originalHTML = button.innerHTML;
        
        try {
            await this.writeToClipboard(text);
            
            // Show success feedback
            button.innerHTML = '<i class="fas fa-check"></i>';
            button.style.color = 'var(--success-color)';
            
            setTimeout(() => {
                button.innerHTML = originalHTML;
                button.style.color = '';
            }, 1500);
        } catch (err) {
            console.error('Failed to copy to clipboard:', err);
            this.showCopyButtonError(button, originalHTML);
        }
    }

    // Show error feedback on copy button
    showCopyButtonError(button, originalHTML = null) {
        const htmlToRestore = originalHTML || button.innerHTML;
        
        button.innerHTML = '<i class="fas fa-times"></i>';
        button.style.color = 'var(--danger-color)';
        
        setTimeout(() => {
            button.innerHTML = htmlToRestore;
            button.style.color = '';
        }, 1500);
    }

    // Legacy method for backward compatibility
    async copyToClipboard(text, button) {
        await this.handleCopyButtonClick(button, text);
    }
    
    // Redo from a specific point in the conversation
    async redoFromMessage(messageIndex, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Clear broken state when starting redo operation
        chat.wasWaitingOnLoad = false;
        
        // Find the message to redo from
        const message = chat.messages[messageIndex];
        if (!message) {return;}
        
        // Get the MCP connection and provider
        const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
        const proxyProvider = this.llmProviders.get(chat.llmProviderId);
        
        if (!mcpConnection || !proxyProvider || !chat.config || !chat.config.model) {
            this.showError('Cannot redo: MCP server or LLM provider not available', chatId);
            return;
        }
        
        // Get model config
        const providerType = chat.config.model.provider;
        const modelName = chat.config.model.id;
        if (!providerType || !modelName) {
            this.showError('Invalid model configuration in chat', chatId);
            return;
        }
        
        // Get the API type from the provider configuration
        const providerApiType = proxyProvider.availableProviders?.[providerType]?.type || providerType;
        
        // Create the LLM provider
        const provider = createLLMProvider(providerApiType, proxyProvider.proxyUrl, modelName, chat.config.model);
        provider.onLog = (logEntry) => {
            const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
            const providerName = providerType.charAt(0).toUpperCase() + providerType.slice(1);
            this.addLogEntry(`${prefix}: ${providerName}`, logEntry);
        };
        
        try {
            if (message.role === 'user') {
                // Redo from user message - truncate everything AFTER this message
                // Keep all history up to and including this message
                this.truncateMessages(chatId, messageIndex + 1, 'Redo from user message');
                this.loadChat(chatId, true);
                
                // Show thinking spinner AFTER loadChat to prevent it from being cleared
                this.showAssistantThinking(chatId);
                
                // Get fresh chat object after loadChat
                const freshChat = this.chats.get(chatId);
                if (!freshChat) {
                    throw new Error('Chat not found after reload');
                }
                
                // Resend the user message with full prior context
                const result = await this.processMessageWithTools(freshChat, mcpConnection, provider, message.content);
                
                // Check if rate limit was handled - don't conclude if so
                if (result && result.rateLimitHandled) {
                    return;
                }
                
                // Success - assistant has concluded
                this.assistantConcluded(chatId);
            } else if (message.role === 'assistant') {
                // Redo from assistant message - find the user message that triggered it
                let triggeringUserMessage = null;
                
                // Find the most recent user message before this assistant message
                for (let i = messageIndex - 1; i >= 0; i--) {
                    if (chat.messages[i].role === 'user') {
                        triggeringUserMessage = chat.messages[i];
                        break;
                    }
                }
                
                if (triggeringUserMessage) {
                    // Truncate from THIS assistant message onwards (not from the user message)
                    // This handles cases where assistant sent multiple messages (e.g., with tool calls)
                    this.truncateMessages(chatId, messageIndex, 'Redo from assistant message');
                    this.loadChat(chatId, true);
                    
                    // Show thinking spinner AFTER loadChat to prevent it from being cleared
                    this.showAssistantThinking(chatId);
                    
                    // Get fresh chat object after loadChat
                    const freshChat = this.chats.get(chatId);
                    if (!freshChat) {
                        throw new Error('Chat not found after reload');
                    }
                    
                    // Resend the triggering user message with full prior context
                    const result = await this.processMessageWithTools(freshChat, mcpConnection, provider, triggeringUserMessage.content);
                    
                    // Check if rate limit was handled - don't conclude if so
                    if (result && result.rateLimitHandled) {
                        return;
                    }
                    
                    // Success - assistant has concluded
                    this.assistantConcluded(chatId);
                } else {
                    this.showError('Cannot find the user message that triggered this response', chatId);
                }
            } else {
                // For any other message type, show error (shouldn't happen with our button logic)
                this.showError('Redo is only available for user and assistant messages', chatId);
            }
        } catch (error) {
            // Check for rate limit error
            const isRateLimitError = error.message && (
                error.message.includes('Rate limit') || 
                error.message.includes('429') ||
                error.message.includes('rate_limit_exceeded') ||
                error.message.includes('529') ||
                error.message.includes('overloaded_error') ||
                error.message.includes('Overloaded')
            );
            const retryMatch = error.message.match(/Please try again in (\d+(?:\.\d+)?)s/);
            const retryAfterSeconds = retryMatch ? parseFloat(retryMatch[1]) : null;
            
            if (isRateLimitError) {
                // Handle rate limit with automatic retry (even without retry time)
                const retryCount = chat.rateLimitRetryCount || 0;
                this.handleRateLimitError(chatId, retryAfterSeconds, retryCount);
                // Mark error as handled to prevent spinner clearing
                error._rateLimitHandled = true;
                // Don't show error UI for rate limits
                this.assistantFailed(chatId, error);
                return;
            } else {
                // Show error with retry button
                const errorMessage = `Redo failed: ${error.message}`;
                const lastUserMessageIndex = chat.messages.findLastIndex(m => m.role === 'user');
                
                // Determine error type
                let errorType = 'llm_error';
                if (error.message.includes('MCP') || error.message.includes('connection')) {
                    errorType = 'mcp_error';
                } else if (error.message.includes('Tool')) {
                    errorType = 'tool_error';
                }
                
                this.addMessage(chatId, { 
                    role: 'error', 
                    content: errorMessage, 
                    errorMessageIndex: lastUserMessageIndex,
                    errorType 
                });
                
                this.processRenderEvent({ 
                    type: 'error-message', 
                    content: errorMessage, 
                    errorMessageIndex: lastUserMessageIndex,
                    errorType
                }, chatId);
            }
            
            // Clean up on error - but preserve rate limit waiting spinner
            this.assistantFailed(chatId, error);
        }
    }

    clearLog() {
        this.showConfirmDialog(
            'Clear Communication Log',
            'Are you sure you want to clear all communication logs?',
            'Clear',
            'Cancel'
        ).then(confirmed => {
            if (confirmed) {
                this.communicationLog = [];
                this.logContent.innerHTML = '';
            }
        });
    }

    downloadLog() {
        const logText = this.communicationLog.map(entry => {
            return `[${entry.timestamp}] [${entry.source}] ${entry.direction}: ${entry.message}`;
        }).join('\n\n');
        
        const blob = new Blob([logText], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `mcp-communication-log-${new Date().toISOString()}.txt`;
        a.click();
        URL.revokeObjectURL(url);
    }

    loadSettings() {
        // Note: file:// protocol is now supported thanks to the proxy server
        // The proxy handles CORS issues that would normally prevent direct API access
        
        // Load theme
        const savedTheme = localStorage.getItem('theme') || 'dark';
        document.documentElement.setAttribute('data-theme', savedTheme);
        
        // Load log collapsed state (default to collapsed)
        const logCollapsed = localStorage.getItem('logCollapsed') !== 'false';
        if (logCollapsed) {
            this.logPanel.classList.add('collapsed');
            this.toggleLogBtn.innerHTML = '<i class="fas fa-chevron-left"></i>';
            this.expandLogBtn.style.display = 'block';
        }
        
        // Load pane sizes
        this.loadPaneSizes();
        
        // Load MCP servers from localStorage (but these will be merged with proxy servers later)
        const savedMcpServers = localStorage.getItem('mcpServers');
        if (savedMcpServers) {
            try {
                const servers = JSON.parse(savedMcpServers);
                servers.forEach(server => {
                    this.mcpServers.set(server.id, server);
                });
                // Don't update the UI yet - wait for proxy servers to be loaded too
            } catch (e) {
                console.error('Failed to load MCP servers:', e);
            }
        }
        
        // LLM providers will be fetched from proxy on demand
        
        // Load chats - first try split storage, then fall back to legacy
        this.loadChatsFromStorage();
    }

    loadChatsFromStorage() {
        // Load chats with pattern chat_TIMESTAMP
        const chatKeyPrefix = 'chat_';
        
        // Scan localStorage for individual chat keys
        for (let i = 0; i < localStorage.length; i++) {
            const key = localStorage.key(i);
            if (key && key.startsWith(chatKeyPrefix)) {
                try {
                    const chatData = JSON.parse(localStorage.getItem(key));
                    if (chatData && chatData.id) {
                        // Ensure no chat is marked as active on load
                        chatData.isActive = false;
                        // Clean up empty draft messages
                        if (chatData.draftMessage === '' || (chatData.draftMessage && chatData.draftMessage.trim().length === 0)) {
                            chatData.draftMessage = null;
                        }
                        this.validateAndAddChat(chatData);
                    }
                } catch (e) {
                    console.error(`Failed to load chat from key ${key}:`, e);
                }
            }
        }
        
        // Don't update chat sessions yet - wait until after default chat is created
        // this.updateChatSessions();
    }
    
    
    validateAndAddChat(chat) {
        // IMMEDIATE MIGRATION - Delete ALL old properties
        delete chat.model;
        delete chat.temperature;
        delete chat.optimizerSettings;
        delete chat.primaryModel;
        delete chat.secondaryModel;
        delete chat.toolSummarization;
        delete chat.autoSummarization;
        delete chat.toolMemory;
        delete chat.cacheControl;
        
        // Migrate old chat format to new config format if needed
        if (!chat.config) {
            // No config - create default
            chat.config = ChatConfig.createDefaultConfig();
            // If we had an mcpServerId, preserve it
            if (chat.mcpServerId) {
                chat.config.mcpServer = chat.mcpServerId;
            }
        }
        
        // Ensure config is valid
        chat.config = ChatConfig.validateConfig(chat.config);
        
        // Migrate old tool-results format to new format
        if (chat.messages && Array.isArray(chat.messages)) {
            chat.messages = chat.messages.map(msg => {
                // Convert old tool-results format with results field
                if (msg.type === 'tool-results' && msg.results) {
                    // Convert to new format
                    return {
                        role: 'tool-results',
                        toolResults: msg.results.map(result => ({
                            toolCallId: result.toolCallId,
                            toolName: result.toolName || 'unknown',
                            result: result.content || result.result || ''
                        })),
                        timestamp: msg.timestamp
                    };
                }
                
                // Convert tool-results that have type but no role
                if (msg.type === 'tool-results' && !msg.role && msg.toolResults) {
                    const cleanMsg = { ...msg };
                    delete cleanMsg.type;
                    cleanMsg.role = 'tool-results';
                    return cleanMsg;
                }
                
                // Clean up messages that have both type and role - remove type
                if (msg.type && msg.role) {
                    const cleanMsg = { ...msg };
                    delete cleanMsg.type;
                    return cleanMsg;
                }
                
                // Convert any remaining messages with type but no role
                if (msg.type && !msg.role) {
                    const cleanMsg = { ...msg };
                    cleanMsg.role = cleanMsg.type;
                    delete cleanMsg.type;
                    return cleanMsg;
                }
                
                return msg;
            });
        }
        
        // Validate that the chat's model still exists
        if (chat.config && chat.config.model && chat.llmProviderId) {
            const provider = this.llmProviders.get(chat.llmProviderId);
            if (provider && provider.availableProviders) {
                // Check if the model exists in the provider's available models
                let modelExists = false;
                const providerType = chat.config.model.provider;
                const modelName = chat.config.model.id;
                
                if (providerType && modelName && provider.availableProviders[providerType]) {
                    const models = provider.availableProviders[providerType].models || [];
                    modelExists = models.some(m => {
                        const mId = typeof m === 'string' ? m : m.id;
                        return mId === modelName;
                    });
                }
                
                if (!modelExists) {
                    const oldModelString = ChatConfig.modelConfigToString(chat.config.model);
                    console.error(`Chat ${chat.id} has invalid model ${oldModelString}. Model not found in available providers.`);
                    
                    // Mark the chat as having an invalid model
                    chat.hasInvalidModel = true;
                    
                    // DO NOT automatically reset or save!
                    // The user must manually select a valid model
                }
            }
        }
        
        // Validate MCP server ID - set to null if it doesn't exist
        if (chat.config && chat.config.mcpServer) {
            if (!this.mcpServers.has(chat.config.mcpServer)) {
                console.error(`Chat ${chat.id} has invalid MCP server: ${chat.config.mcpServer} - setting to null`);
                chat.config.mcpServer = null;
            }
            // Sync mcpServerId with config
            chat.mcpServerId = chat.config.mcpServer;
        } else if (chat.mcpServerId && !this.mcpServers.has(chat.mcpServerId)) {
            console.error(`Chat ${chat.id} has invalid mcpServerId: ${chat.mcpServerId} - setting to null`);
            chat.mcpServerId = null;
            if (chat.config) {
                chat.config.mcpServer = null;
            }
        }
        
        // Ensure currentAssistantGroup exists for loaded chats
        if (!Object.prototype.hasOwnProperty.call(chat, 'currentAssistantGroup')) {
            chat.currentAssistantGroup = null;
        }
        
        // Ensure pendingToolCalls is a Map (it gets serialized as {} in localStorage)
        if (!chat.pendingToolCalls || !(chat.pendingToolCalls instanceof Map)) {
            chat.pendingToolCalls = new Map();
        }
        
        // Check if the chat was saved while waiting for a response (broken state)
        if (chat.spinnerState || chat.isProcessing) {
            chat.wasWaitingOnLoad = true;
            // Clear the spinner state since we're not actually waiting anymore
            chat.spinnerState = null;
            chat.isProcessing = false;
        }
        
        // Reconstruct MessageOptimizer instance for loaded chats
        if (!chat.messageOptimizer || !(chat.messageOptimizer instanceof MessageOptimizer)) {
            // Config should already be migrated by this point
            if (!chat.config) {
                throw new Error(`Chat ${chat.id} has no config after migration`);
            }
            
            // Get optimizer settings from config
            const optimizerSettings = ChatConfig.getOptimizerSettings(chat.config, window.createLLMProvider);
            
            // Create MessageOptimizer instance
            chat.messageOptimizer = new MessageOptimizer(optimizerSettings);
        }
        
        // Ensure pendingToolCalls map exists
        if (!chat.pendingToolCalls) {
            chat.pendingToolCalls = new Map();
        }
        
        // Add to memory - DO NOT SAVE!
        // The chat is now in the correct format in memory
        // It will only be saved when the user actually modifies it
        this.chats.set(chat.id, chat);
        
        // Migrate token pricing and fix incomplete model names
        this.migrateTokenPricing(chat);
    }
    
    async initializeDefaultLLMProvider() {
        // Always fetch models even if we have providers (to get fresh model list)

        // Auto-detect the proxy URL from the current origin
        const proxyUrl = window.location.origin;
        
        try {
            // Fetch available models from the same origin
            const response = await fetch(`${proxyUrl}/models`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const data = await response.json();
            const providers = data.providers || {};
            
            if (Object.keys(providers).length === 0) {
                console.warn('No LLM providers configured in proxy');
                this.showNoModelsModal(proxyUrl);
                return;
            }
            
            // Update availableProviders for all existing providers with the same proxyUrl
            let updated = false;
            for (const [, provider] of this.llmProviders) {
                if (provider.proxyUrl === proxyUrl) {
                    provider.availableProviders = providers;
                    updated = true;
                }
            }
            
            // If no existing provider, create a new one
            if (!updated) {
                const providerId = 'default_llm_provider';
                const provider = {
                    id: providerId,
                    name: 'LLM Provider',
                    proxyUrl,
                    availableProviders: providers,
                    onLog: (logEntry) => {
                        const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
                        this.addLogEntry(`${prefix}: LLM Provider`, logEntry);
                    }
                };
                this.llmProviders.set(providerId, provider);
            }
            
            // Always populate modelLimits and pricing from fresh data
            this.modelPricing = {};
            Object.entries(providers).forEach(([_providerType, config]) => {
                if (config.models) {
                    config.models.forEach(model => {
                        if (typeof model === 'object' && model.id) {
                            if (model.contextWindow) {
                                this.modelLimits[model.id] = model.contextWindow;
                            }
                            if (model.pricing) {
                                this.modelPricing[model.id] = model.pricing;
                            }
                        }
                    });
                }
            });
            
            this.saveSettings();
            this.updateLlmProvidersList();

            // Validate all existing chats have valid models
            this.validateChatModels();
            
            // Update token displays for the current chat now that pricing is loaded
            const activeChatId = this.getActiveChatId();
            if (activeChatId) {
                this.updateAllTokenDisplays(activeChatId);
            }
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `Auto-configured LLM proxy from ${proxyUrl}`
            });
            
        } catch (error) {
            console.error('Failed to auto-configure LLM provider:', error);
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'error',
                message: `Failed to auto-configure LLM provider: ${error.message}`
            });
            this.showNoModelsModal(proxyUrl);
        }
    }

    async createDefaultChatIfNeeded() {
        // Make sure we have at least one MCP server and LLM provider
        if (this.mcpServers.size === 0 || this.llmProviders.size === 0) {return;}
        
        // Get the last used configuration
        const config = ChatConfig.getLastConfig();
        let mcpServerId = config.mcpServer;
        const llmProviderId = this.llmProviders.keys().next().value;
        
        // Use defaults if needed
        if (!mcpServerId || !this.mcpServers.has(mcpServerId)) {
            // Get first server from sorted list
            const sortedServers = Array.from(this.mcpServers.entries())
                .sort(([, a], [, b]) => a.name.localeCompare(b.name));
            mcpServerId = sortedServers[0]?.[0];
        }
        
        
        // Check if the config has a valid model, otherwise get first available
        let model = null;
        if (!config || !config.model || !config.model.provider || !config.model.id) {
            const provider = this.llmProviders.get(llmProviderId);
            if (provider && provider.availableProviders) {
                const firstProvider = Object.keys(provider.availableProviders)[0];
                const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                if (firstModel) {
                    const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                    model = `${firstProvider}:${modelId}`;
                }
            }
            
            if (!model) {return;}
        }
        
        // Create an unsaved chat
        const createOptions = {
            mcpServerId,
            llmProviderId,
            title: 'New Chat',
            isSaved: false,
            config  // Pass the config
        };
        
        // Only pass model if we had to find one
        if (model) {
            createOptions.model = model;
        }
        
        return this.createNewChat(createOptions);
    }

    async initializeDefaultMCPServers() {
        try {
            // Get the proxy URL - try to get from LLM provider or use current origin
            let proxyUrl;
            
            const defaultProvider = [...this.llmProviders.values()].find(p => p.url);
            if (defaultProvider && defaultProvider.url) {
                proxyUrl = defaultProvider.url;
            } else {
                // Use the same origin as the current page (since we're being served by the proxy)
                proxyUrl = window.location.origin;
            }
            
            const response = await fetch(`${proxyUrl}/mcp-servers`);
            if (!response.ok) {
                console.warn('Failed to fetch default MCP servers from proxy:', response.status);
                return;
            }
            
            const data = await response.json();
            const defaultServers = data.servers || [];
            
            if (defaultServers.length === 0) {
                return;
            }

            // Handle migration of old default_mcp_server if it exists
            const oldDefaultServer = this.mcpServers.get('default_mcp_server');
        if (oldDefaultServer && oldDefaultServer.url === 'ws://localhost:19999/mcp') {
            // Remove the old default server as it's being replaced by Costa-Desktop
            this.mcpServers.delete('default_mcp_server');
        }

        // Create a map of existing servers by URL for easy lookup
        const existingServersByUrl = new Map();
        for (const [id, server] of this.mcpServers) {
            existingServersByUrl.set(server.url, { id, server });
        }

        // Add or update default servers
        for (const defaultServer of defaultServers) {
            const existing = existingServersByUrl.get(defaultServer.url);
            
            if (existing) {
                // Server with this URL exists, update it with default info if it's one of our defaults
                // But keep user-defined servers untouched
                if (existing.id.startsWith('costa_') || existing.id.startsWith('prod_') || 
                    existing.id.startsWith('demos_') || existing.id === 'agent_events' ||
                    existing.id === 'default_mcp_server') {
                    // It's one of our default servers, update it
                    existing.server.id = defaultServer.id;
                    existing.server.name = defaultServer.name;
                    this.mcpServers.delete(existing.id);
                    this.mcpServers.set(defaultServer.id, existing.server);
                }
                // Otherwise it's a user-defined server, leave it alone
            } else {
                // Server doesn't exist, add it
                const server = {
                    id: defaultServer.id,
                    name: defaultServer.name,
                    url: defaultServer.url,
                    connected: false
                };
                this.mcpServers.set(defaultServer.id, server);
            }
        }

        // Save the updated server list
            this.saveSettings();
            this.updateMcpServersList();

            // Log the initialization
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: 'Initialized default MCP servers'
            });
        } catch (error) {
            console.error('Failed to initialize default MCP servers:', error);
            // Continue without default servers - user can add them manually
        }
    }

    saveSettings() {
        // Save MCP servers
        const serversToSave = Array.from(this.mcpServers.values());
        localStorage.setItem('mcpServers', JSON.stringify(serversToSave));
        
        // Don't save LLM providers - always fetch fresh from proxy
        
        // Note: Chats are now saved individually via saveChatToStorage()
        // when they are modified, not all at once
        
        // Save current chat ID
        const activeChatId = this.getActiveChatId();
        if (activeChatId) {
            localStorage.setItem('currentChatId', activeChatId);
        }
    }
    
    saveChatToStorage(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || chat.isSaved === false) {return;}
        
        try {
            localStorage.setItem(chatId, JSON.stringify(chat));
        } catch (e) {
            console.error(`Failed to save chat ${chatId}:`, e);
            if (e.name === 'QuotaExceededError') {
                this.showGlobalError('Storage quota exceeded. Consider deleting old chats.');
            }
        }
    }

    // MCP Server Management
    async addMcpServer() {
        const url = this.mcpServerUrl.value.trim();
        const name = this.mcpServerName.value.trim();
        
        if (!url || !name) {
            this.showGlobalError('Please fill in all fields');
            return;
        }
        
        // Test connection
        try {
            const testClient = new MCPClient();
            testClient.onLog = (logEntry) => {
                const prefix = logEntry.direction === 'sent' ? 'mcp-request' : 'mcp-response';
                this.addLogEntry(`${prefix}: ${name}`, logEntry);
            };
            await testClient.connect(url);
            
            // Connection successful, save server
            const serverId = `mcp_${Date.now()}`;
            const server = {
                id: serverId,
                name,
                url,
                connected: true
            };
            
            this.mcpServers.set(serverId, server);
            this.mcpConnections.set(serverId, testClient);
            
            this.saveSettings();
            this.updateMcpServersList();

            // Clear form
            this.mcpServerUrl.value = '';
            this.mcpServerName.value = '';
            this.hideModal('addMcpModal');
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `MCP server "${name}" added successfully`
            });
            
        } catch (error) {
            this.showError(`Failed to connect to MCP server: ${error.message}`, this.getActiveChatId());
        }
    }

    updateMcpServersList() {
        this.mcpServersList.innerHTML = '';
        
        if (this.mcpServers.size === 0) {
            this.mcpServersList.innerHTML = '<div class="text-center text-muted">No MCP servers configured</div>';
            return;
        }
        
        // Sort servers by name
        const sortedServers = Array.from(this.mcpServers.entries())
            .sort(([, a], [, b]) => a.name.localeCompare(b.name));
        
        for (const [id, server] of sortedServers) {
            const connection = this.mcpConnections.get(id);
            const isConnected = connection && connection.isReady();
            
            const serverDiv = document.createElement('div');
            serverDiv.className = 'config-item';
            serverDiv.innerHTML = `
                <div class="config-item-info">
                    <div class="config-item-name">${server.name}</div>
                    <div class="config-item-details">${server.url}</div>
                </div>
                <div class="config-item-actions">
                    <div class="config-item-status">
                        <span class="status-dot ${isConnected ? 'connected' : 'disconnected'}"></span>
                        <span>${isConnected ? 'Connected' : 'Disconnected'}</span>
                    </div>
                    <button class="btn btn-small btn-danger" onclick="app.removeMcpServer('${id}')">Remove</button>
                </div>
            `;
            this.mcpServersList.appendChild(serverDiv);
        }
    }

    async removeMcpServer(serverId) {
        const server = this.mcpServers.get(serverId);
        const serverName = server ? server.name : 'this MCP server';
        
        const confirmed = await this.showConfirmDialog(
            'Remove MCP Server',
            `Are you sure you want to remove "${serverName}"?`,
            'Remove',
            'Cancel',
            true // danger style
        );
        
        if (confirmed) {
            // Disconnect if connected
            const connection = this.mcpConnections.get(serverId);
            if (connection) {
                connection.disconnect();
                this.mcpConnections.delete(serverId);
            }
            
            this.mcpServers.delete(serverId);
            this.saveSettings();
            this.updateMcpServersList();

            // Check if any chats use this server
            for (const chat of this.chats.values()) {
                if (chat.mcpServerId === serverId) {
                    chat.mcpServerId = null;
                    // Note: Chat becomes unusable without MCP server
                }
            }
            this.saveSettings();
        }
    }

    // LLM Provider Management
    updateLlmProvidersList() {
        // This function is no longer needed since LLM providers are auto-configured
        // but we'll keep it for backward compatibility
    }

    // Chat Management
    async createNewChatDirectly() {
        // Check if providers are still loading
        if (!this.providersLoaded) {
            this.showGlobalError('Please wait, loading providers...');
            return;
        }
        
        // Check if there's an unsaved chat
        const unsavedChat = Array.from(this.chats.values()).find(chat => chat.isSaved === false);
        if (unsavedChat) {
            console.log(`[createNewChatDirectly] Found unsaved chat: ${unsavedChat.id}, will switch to it instead of creating new`);
            const activeChatId = this.getActiveChatId();

            // Check if we're already in the unsaved chat
            if (activeChatId === unsavedChat.id) {
                // Already in the unsaved chat, just show toast
                this.showToast('Please use the current chat or save it by sending a message before creating a new one.');
            } else {
                // Switch to the unsaved chat instead of creating a new one
                this.loadChat(unsavedChat.id);
            }
            return;
        }
        
        // Make sure we have at least one MCP server and LLM provider
        if (this.mcpServers.size === 0 || this.llmProviders.size === 0) {
            this.showGlobalError('Please configure at least one MCP server and LLM provider');
            return;
        }
        
        // Get the last used configuration
        const config = ChatConfig.getLastConfig();
        let mcpServerId = config.mcpServer;
        const llmProviderId = this.llmProviders.keys().next().value; // Always use first available
        
        // Use defaults if needed
        if (!mcpServerId || !this.mcpServers.has(mcpServerId)) {
            // Get first server from sorted list
            const sortedServers = Array.from(this.mcpServers.entries())
                .sort(([, a], [, b]) => a.name.localeCompare(b.name));
            mcpServerId = sortedServers[0]?.[0];
        }
        
        // Check if the config has a valid model, otherwise get first available
        let model = null;
        if (!config || !config.model || !config.model.provider || !config.model.id) {
            const provider = this.llmProviders.get(llmProviderId);
            if (provider && provider.availableProviders) {
                const firstProvider = Object.keys(provider.availableProviders)[0];
                const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                if (firstModel) {
                    const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                    model = `${firstProvider}:${modelId}`;
                }
            }
            
            if (!model) {
                this.showGlobalError('No models available');
                return;
            }
        }
        
        // Create an unsaved chat
        const createOptions = {
            mcpServerId,
            llmProviderId,
            title: 'New Chat',
            isSaved: false,
            config  // Pass the full config
        };
        
        // Only pass model if we had to find one
        if (model) {
            createOptions.model = model;
        }
        
        const chatId = await this.createNewChat(createOptions);
        
        // Load the chat immediately when created via button click
        if (chatId) {
            this.loadChat(chatId);
        }
    }
    
    // Per-chat DOM management
    getChatContainer(chatId) {
        // All containers (including sub-chats) are now in chatContainers
        // No more temporary containers!
        
        // First check if container already exists
        if (this.chatContainers.has(chatId)) {
            return this.chatContainers.get(chatId);
        }
        
        // Container doesn't exist - need to create one
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[getChatContainer] Chat ${chatId} not found`);
            return null;
        }
        
        // Create container based on chat type
        if (chat.isSubChat) {
            // Sub-chats should ONLY have containers created by renderSubChatAsItem
            // If we're here, it means there's a sequencing error
            console.error(`[getChatContainer] CRITICAL: Attempted to get container for sub-chat ${chatId} before renderSubChatAsItem was called. This is a bug in the code flow.`);
            console.trace(); // Show stack trace to debug the issue
            return null; // Return null to fail fast
        }
        
        // Regular chat - create full DOM
        const container = this.createChatDOM(chatId);
        if (container) {
            this.chatContainersEl.appendChild(container);
            this.chatContainers.set(chatId, container);
            
            // Apply any pending connection state
            if (chat.pendingConnectionState) {
                this.updateChatConnectionUI(chatId, chat.pendingConnectionState.state, chat.pendingConnectionState.details);
                delete chat.pendingConnectionState;
            }
        }
        
        return container;
    }
    
    // Helper method to get the input element for a specific chat
    getChatInput(chatId) {
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.input) {
            return container._elements.input;
        }
        return null;
    }
    
    createChatDOM(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return null;}
        
        const container = document.createElement('div');
        container.className = 'chat-container';
        container.dataset.chatId = chatId;
        
        // Create the complete chat UI structure
        container.innerHTML = `
            <div class="chat-header">
                <div class="chat-info">
                    <div>
                        <h3 class="chat-title">${chat.title}</h3>
                        <div class="chat-meta">
                            <span class="chat-mcp"></span>
                            <span class="chat-llm">
                                <span class="model-name"></span>
                            </span>
                        </div>
                    </div>
                    <div class="chat-controls">
                        <div class="metrics-dashboard">
                            <!-- Context Window Indicator -->
                            <div class="context-window-section" data-tooltip="Shows how much of the model's context is being used">
                                <div class="context-window-header">
                                    <span class="context-window-label">CONTEXT WINDOW</span>
                                </div>
                                <div class="context-window-bar-container">
                                    <div class="context-window-bar">
                                        <div class="context-window-fill" style="width: 0"></div>
                                        <span class="context-window-stats">0 / 4k</span>
                                    </div>
                                </div>
                            </div>
                            
                            <!-- Cumulative Token Counters -->
                            <div class="token-counters-section">
                                <div class="token-counters-headers">
                                    <span class="token-header-primary">TOKENS</span>
                                    <span class="token-header-item"><i class="fas fa-file-alt"></i> INPUT</span>
                                    <span class="token-header-item"><i class="fas fa-memory"></i> CACHE R</span>
                                    <span class="token-header-item"><i class="fas fa-save"></i> CACHE W</span>
                                    <span class="token-header-item"><i class="fas fa-upload"></i> OUTPUT</span>
                                    <span class="token-header-item"><i class="fas fa-dollar-sign"></i> COST</span>
                                </div>
                                <div class="token-counters-values">
                                    <span class="token-value-primary">PRIMARY</span>
                                    <span class="cumulative-input-tokens token-value-item">0</span>
                                    <span class="cumulative-cache-read-tokens token-value-item">0</span>
                                    <span class="cumulative-cache-creation-tokens token-value-item">0</span>
                                    <span class="cumulative-output-tokens token-value-item">0</span>
                                    <span class="cumulative-cost token-value-item" style="color: #4CAF50;">$0.00</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="chat-content">
                <div class="chat-messages"></div>
                <div class="chat-controls-bar" style="display: flex; align-items: center; justify-content: center; gap: 4px; margin: 5px auto; flex-wrap: wrap; padding: 0 10px; max-width: 900px;">
                    
                    <!-- Model and MCP Server Selection -->
                    <div class="dropdown" style="position: relative;">
                        <button class="llm-model-btn btn btn-secondary dropdown-toggle">
                            <span><i class="fas fa-robot"></i></span>
                            <span class="current-model-text">Model</span>
                            <span style="margin-left: 5px;"><i class="fas fa-chevron-down"></i></span>
                        </button>
                        <div class="llm-model-dropdown dropdown-menu" style="display: none; position: absolute; bottom: 100%; left: 0; margin-bottom: 5px; max-height: 300px; overflow-y: auto;"></div>
                    </div>
                    
                    <div class="dropdown" style="position: relative;">
                        <button class="mcp-server-btn btn btn-secondary dropdown-toggle" data-tooltip="Switch MCP server">
                            <span><i class="fas fa-plug"></i></span>
                            <span class="current-mcp-text">MCP Server</span>
                            <span style="margin-left: 5px;"><i class="fas fa-chevron-down"></i></span>
                        </button>
                        <div class="mcp-server-dropdown dropdown-menu" style="display: none; position: absolute; bottom: 100%; left: 0; margin-bottom: 5px; max-height: 300px; overflow-y: auto;"></div>
                    </div>
                    
                    <!-- Other buttons -->
                    <button class="copy-metrics-btn btn btn-secondary" data-tooltip="Copy all message metadata including tokens and timing">
                        <span><i class="fas fa-copy"></i></span>
                        <span>Log</span>
                    </button>
                    <button class="summarize-btn btn btn-secondary" data-tooltip="Summarize conversation to reduce context size">
                        <span><i class="fas fa-compress-alt"></i></span>
                        <span>Summarize</span>
                    </button>
                    <button class="generate-title-btn btn btn-secondary" data-tooltip="Generate or update chat title using AI">
                        <span><i class="fas fa-edit"></i></span>
                        <span>Title</span>
                    </button>
                </div>
                <div class="resize-handle resize-handle-horizontal"></div>
                <div class="chat-input-container">
                    <button class="reconnect-mcp-btn btn btn-primary" style="display: none;">Reconnect MCP Server</button>
                    <div class="chat-input-wrapper">
                        <div 
                            class="chat-input" 
                            contenteditable="true"
                            data-placeholder="Ask about your Netdata metrics..."
                            style="min-height: 4.5em; max-height: 200px; overflow-y: auto; white-space: pre-wrap;"
                        ></div>
                        <button class="send-message-btn btn btn-send">Send</button>
                    </div>
                </div>
            </div>
        `;
        
        // Store element references for easy access
        container._elements = {
            header: container.querySelector('.chat-header'),
            title: container.querySelector('.chat-title'),
            mcpMeta: container.querySelector('.chat-mcp'),
            llmMeta: container.querySelector('.chat-llm'),
            messages: container.querySelector('.chat-messages'),
            input: container.querySelector('.chat-input'),
            sendBtn: container.querySelector('.send-message-btn'),
            reconnectBtn: container.querySelector('.reconnect-mcp-btn'),
            
            // Context window elements
            contextFill: container.querySelector('.context-window-fill'),
            contextStats: container.querySelector('.context-window-stats'),
            
            // Token counter elements
            cumulativeInputTokens: container.querySelector('.cumulative-input-tokens'),
            cumulativeCacheReadTokens: container.querySelector('.cumulative-cache-read-tokens'),
            cumulativeCacheCreationTokens: container.querySelector('.cumulative-cache-creation-tokens'),
            cumulativeOutputTokens: container.querySelector('.cumulative-output-tokens'),
            cumulativeCost: container.querySelector('.cumulative-cost'),
            
            // Control buttons
            
            llmModelBtn: container.querySelector('.llm-model-btn'),
            llmModelDropdown: container.querySelector('.llm-model-dropdown'),
            currentModelText: container.querySelector('.current-model-text'),
            
            mcpServerBtn: container.querySelector('.mcp-server-btn'),
            mcpServerDropdown: container.querySelector('.mcp-server-dropdown'),
            currentMcpText: container.querySelector('.current-mcp-text'),
            
            copyMetricsBtn: container.querySelector('.copy-metrics-btn'),
            summarizeBtn: container.querySelector('.summarize-btn'),
            generateTitleBtn: container.querySelector('.generate-title-btn'),
            
            // Resize handle
            inputResizeHandle: container.querySelector('.resize-handle-horizontal')
        };
        
        // Attach event listeners
        this.attachChatEventListeners(container, chatId);
        
        return container;
    }
    
    attachChatEventListeners(container, chatId) {
        const elements = container._elements;
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Send button
        elements.sendBtn.addEventListener('click', () => {
            if (chat && chat.isProcessing) {
                // Stop processing for this specific chat
                chat.shouldStopProcessing = true;
                chat.isProcessing = false;
                this.updateSendButton(chatId);
                // Re-enable chat-specific input
                if (elements.input) {
                    elements.input.contentEditable = true;
                } else if (!chat.isSubChat) {
                    // Only log error for main chats - sub-chats don't have input elements
                    console.error('[sendBtn.click] ERROR: Could not find input element when stopping processing for chat', chatId);
                }
                // Don't add a system message as it breaks message sequencing
                // The assistantFailed handler will take care of the UI feedback
            } else {
                // Send message
                this.sendMessage(chatId).catch(error => {
                    console.error('Failed to send message:', error);
                    this.showError('Failed to send message', chatId);
                });
            }
        });
        
        // Input field
        elements.input.addEventListener('input', (e) => {
            // For contentEditable, get content while preserving formatting
            const content = this.getEditableContent(e.target);
            chat.draftMessage = content;
            
            // Update send button state
            elements.sendBtn.disabled = !content.trim();
            
            // Debounce saving to storage - save after 2 seconds of no typing
            if (this.draftSaveTimeout) {
                clearTimeout(this.draftSaveTimeout);
            }
            this.draftSaveTimeout = setTimeout(() => {
                this.autoSave(chatId);
                // Still don't update UI here - just save to storage
            }, 2000);
        });
        
        // Handle paste events to convert HTML to markdown immediately
        elements.input.addEventListener('paste', (e) => {
            e.preventDefault(); // Prevent default paste
            
            // Get clipboard data
            const clipboardData = e.clipboardData || window.clipboardData;
            if (!clipboardData) return;
            
            // Try to get HTML content first, fall back to plain text
            let content = clipboardData.getData('text/html');
            const hasHtml = content && content.trim() !== '';
            
            if (!hasHtml) {
                // No HTML, just use plain text
                content = clipboardData.getData('text/plain');
                if (content) {
                    // Insert plain text at cursor position
                    document.execCommand('insertText', false, content);
                }
                return;
            }
            
            // Convert HTML to markdown
            const markdown = this.convertHtmlToMarkdown(content);
            
            // Insert markdown as plain text
            if (markdown) {
                document.execCommand('insertText', false, markdown);
            }
        });
        
        // Enter to send
        elements.input.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage(chatId).catch(error => {
                    console.error('Failed to send message:', error);
                    this.showError('Failed to send message', chatId);
                });
            }
        });
        
        // Model selector
        elements.llmModelBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            // Close any existing model selector overlays
            document.querySelectorAll('.model-selector-overlay').forEach(el => el.remove());
            this.populateModelDropdown(chatId, elements.llmModelDropdown, elements.llmModelBtn);
        });
        
        // MCP server selector
        elements.mcpServerBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.populateMCPDropdown(chatId, elements.mcpServerDropdown);
            this.toggleChatDropdown(elements.mcpServerDropdown);
        });
        
        // Other buttons
        elements.copyMetricsBtn.addEventListener('click', () => {
            this.copyConversationMetrics(chatId).catch(error => {
                console.error('Failed to copy metrics:', error);
                this.showError('Failed to copy metrics', chatId);
            });
        });
        
        elements.summarizeBtn.addEventListener('click', () => {
            this.summarizeConversation(chatId).catch(error => {
                console.error('Failed to summarize conversation:', error);
                this.showError('Failed to summarize conversation', chatId);
            });
        });
        
        elements.generateTitleBtn.addEventListener('click', () => {
            this.handleGenerateTitleClick(chatId).catch(error => {
                console.error('Failed to generate title:', error);
                this.showError('Failed to generate title', chatId);
            });
        });
        
        elements.reconnectBtn.addEventListener('click', () => {
            const mcpServerId = this.reconnectMcpBtn ? this.reconnectMcpBtn.dataset.mcpServerId : null;
            if (mcpServerId) {
                this.reconnectMcpServer(mcpServerId).catch(error => {
                    console.error('Failed to reconnect MCP server:', error);
                    this.showError('Failed to reconnect MCP server', chatId);
                });
            }
        });
        
        // Resize handle for input - delay to ensure DOM is ready
        if (elements.inputResizeHandle) {
            requestAnimationFrame(() => {
                this.makeResizable(elements.inputResizeHandle, elements.input.parentElement.parentElement, 'vertical', 150, 400);
                elements.inputResizeHandle._resizeInitialized = true;
            });
        }
        
        // Note: Global dropdown close handler is already set up in initializeUI()
    }
    
    toggleChatDropdown(dropdownEl) {
        // Close all other dropdowns in all chats
        this.chatContainers.forEach(container => {
            const elements = container._elements;
            if (elements) {
                if (elements.llmModelDropdown && elements.llmModelDropdown !== dropdownEl) {
                    elements.llmModelDropdown.style.display = 'none';
                }
                if (elements.mcpServerDropdown && elements.mcpServerDropdown !== dropdownEl) {
                    elements.mcpServerDropdown.style.display = 'none';
                }
            }
        });
        
        // Toggle the requested dropdown
        dropdownEl.style.display = dropdownEl.style.display === 'none' ? 'block' : 'none';
    }
    
    switchChatDOM(chatId) {
        // If this is the pending new chat trying to switch while user has selected another chat, block it
        if (this.pendingNewChatId === chatId && this.userHasSelectedChat) {
            const activeChatId = this.getActiveChatId();
            if (activeChatId && activeChatId !== chatId) {
                return;
            }
        }
        
        // Hide welcome screen
        if (this.welcomeScreen) {
            this.welcomeScreen.style.display = 'none';
        }
        
        // Hide all chat containers
        this.chatContainers.forEach((container, id) => {
            if (container && container.classList) {
                container.classList.remove('active');
            }
            const chat = this.chats.get(id);
            if (chat) {
                chat.isActive = false;
            }
        });
        
        // Get or create the container for this chat
        const container = this.getChatContainer(chatId);
        
        // Show the selected chat
        if (container) {
            container.classList.add('active');
            const chat = this.chats.get(chatId);
            if (chat) {
                chat.isActive = true;
                
                // No longer need to update global chatInput reference
                this.sendMessageBtn = container._elements.sendBtn;
                this.reconnectMcpBtn = container._elements.reconnectBtn;
                this.chatTitle = container._elements.title;

                // Control buttons
                this.copyMetricsBtn = container._elements.copyMetricsBtn;
                this.summarizeBtn = container._elements.summarizeBtn;
                this.generateTitleBtn = container._elements.generateTitleBtn;
                
                // Dropdowns
                this.llmModelDropdown = container._elements.llmModelDropdown;
                this.currentModelText = container._elements.currentModelText;
                this.mcpServerDropdown = container._elements.mcpServerDropdown;
                this.currentMcpText = container._elements.currentMcpText;
                
                // Temperature controls
                
                // Focus input if ready
                if (this.isChatReady(chat)) {
                    container._elements.input.focus();
                }
            }
        }
    }
    
    updateChatHeader(chatId) {
        const container = this.getChatContainer(chatId);
        if (!container) {return;}
        
        const elements = container._elements;
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        const server = this.mcpServers.get(chat.mcpServerId);
        const provider = this.llmProviders.get(chat.llmProviderId);
        
        // Update title
        elements.title.textContent = chat.title;
        
        // Update MCP server
        elements.mcpMeta.textContent = server ? server.name : 'MCP: Not found';
        
        // Update LLM model
        const chatModelString = ChatConfig.getChatModelString(chat);
        if (provider && chatModelString) {
            const config = chat.config || ChatConfig.loadChatConfig(chatId);
            
            // Collect all unique models being used
            const modelsInUse = new Map();
            modelsInUse.set('Chat', chatModelString);
            
            if (config.optimisation.toolSummarisation.enabled && config.optimisation.toolSummarisation.model) {
                modelsInUse.set('Tools', ChatConfig.modelConfigToString(config.optimisation.toolSummarisation.model));
            }
            
            if (config.optimisation.autoSummarisation.enabled && config.optimisation.autoSummarisation.model) {
                modelsInUse.set('Summaries', ChatConfig.modelConfigToString(config.optimisation.autoSummarisation.model));
            }
            
            if (config.optimisation.titleGeneration?.enabled !== false && config.optimisation.titleGeneration?.model) {
                modelsInUse.set('Titles', ChatConfig.modelConfigToString(config.optimisation.titleGeneration.model));
            }
            
            // Create display string with unique models
            const uniqueModels = [...new Set(modelsInUse.values())];
            const modelDisplay = uniqueModels.join(' / ');
            
            // Update model display in the header
            const modelNameSpan = elements.llmMeta.querySelector('.model-name');
            if (modelNameSpan) {
                modelNameSpan.textContent = modelDisplay;
                
                // Remove old data-tooltip and add mouse handlers
                elements.llmMeta.removeAttribute('data-tooltip');
                if (!elements.llmMeta.hasModelTooltipHandlers) {
                    elements.llmMeta.hasModelTooltipHandlers = true;
                    elements.llmMeta.addEventListener('mouseenter', (e) => {
                        const currentChat = this.chats.get(chatId);
                        if (currentChat) this.showModelTooltip(e, currentChat);
                    });
                    elements.llmMeta.addEventListener('mouseleave', () => {
                        this.hideModelTooltip();
                    });
                }
            }
            
            // Also update dropdown button
            elements.currentModelText.textContent = modelDisplay;
            elements.llmModelBtn.removeAttribute('data-tooltip');
            if (!elements.llmModelBtn.hasModelTooltipHandlers) {
                elements.llmModelBtn.hasModelTooltipHandlers = true;
                elements.llmModelBtn.addEventListener('mouseenter', (e) => {
                    const currentChat = this.chats.get(chatId);
                    if (currentChat) this.showModelTooltip(e, currentChat);
                });
                elements.llmModelBtn.addEventListener('mouseleave', () => {
                    this.hideModelTooltip();
                });
            }
        } else {
            elements.llmMeta.textContent = 'Model: Not found';
            elements.currentModelText.textContent = 'Select Model';
        }
        
        // Update MCP dropdown text
        elements.currentMcpText.textContent = server ? server.name : 'Select MCP';
        
    }
    
    isChatReady(chat) {
        return chat && 
               this.mcpServers.has(chat.mcpServerId) && 
               this.llmProviders.has(chat.llmProviderId) &&
               this.mcpConnections.has(chat.mcpServerId) &&
               !chat.isProcessing &&
               !chat.spinnerState;
    }
    
    updateChatInputState(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        const container = this.getChatContainer(chatId);
        if (!container) {return;}
        
        const elements = container._elements;
        const server = this.mcpServers.get(chat.mcpServerId);
        const provider = this.llmProviders.get(chat.llmProviderId);
        
        if (server && provider && this.mcpConnections.has(chat.mcpServerId)) {
            const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
            if (mcpConnection && mcpConnection.isReady()) {
                // Only enable if connection ready AND chat not busy
                if (!chat.isProcessing && !chat.spinnerState) {
                    elements.input.contentEditable = true;
                    elements.sendBtn.disabled = false;
                    elements.input.setAttribute('data-placeholder', 'Ask about your Netdata metrics...');
                    elements.reconnectBtn.style.display = 'none';
                    
                    // Focus input if this is the active chat
                    if (this.getActiveChatId() === chatId) {
                        elements.input.focus();
                    }
                } else {
                    // Chat is busy - keep input disabled but hide reconnect button
                    elements.input.contentEditable = false;
                    elements.sendBtn.disabled = true;
                    elements.input.setAttribute('data-placeholder', 'Processing...');
                    elements.reconnectBtn.style.display = 'none';
                }
                
                return;
            }
        }
        
        // Still not ready, check again
        setTimeout(() => {
            if (this.getActiveChatId() === chatId) {
                this.updateChatInputState(chatId);
            }
        }, 500);
    }

    async createNewChat(options = {}) {
        // Now only supports programmatic creation since we removed the modal
        const mcpServerId = options.mcpServerId;
        const llmProviderId = options.llmProviderId;
        const selectedModel = options.model;
        let title = options.title || '';
        const isSaved = options.isSaved !== undefined ? options.isSaved : true;
        
        // Sub-chat support
        const parentChatId = options.parentChatId || null;
        const parentToolCallId = options.parentToolCallId || null;
        const toolMetadata = options.toolMetadata || null;
        
        if (!mcpServerId || !llmProviderId) {
            this.showError('Cannot create chat: Missing MCP server or LLM provider', null);
            return;
        }
        
        // For backward compatibility, if selectedModel is provided as a string, use it
        // Otherwise, the model should be in the config
        if (!selectedModel && (!options.config || !options.config.model || !options.config.model.provider || !options.config.model.id)) {
            console.warn('[createNewChat] No model provided and no valid model in config');
            return null;
        }
        
        // Generate chat ID early so we can show the tab immediately
        const chatId = `chat_${Date.now()}`;
        
        // Generate title if not provided
        if (!title) {
            const server = this.mcpServers.get(mcpServerId);
            const provider = this.llmProviders.get(llmProviderId);
            title = `${server.name} - ${provider.name}`;
        }
        
        // Create system message using centralized logic
        const systemMessage = SystemMsg.createSystemMessage({
            basePrompt: this.lastSystemPrompt,
            includeDateTimeContext: true,
            mcpInstructions: null // MCP instructions will be added later during message building
        });
        
        // Create configuration from options
        let chatConfig;
        try {
            chatConfig = ChatConfig.createConfigFromOptions({
                config: options.config,
                model: selectedModel,
                mcpServerId
            });
        } catch (error) {
            console.error('[createNewChat] Failed to create valid configuration:', error);
            return null;
        }
        
        // Get optimizer settings
        const optimizerSettings = ChatConfig.getOptimizerSettings(chatConfig, window.createLLMProvider);
        
        // Create the chat object
        const chat = {
            id: chatId,
            title,
            mcpServerId,
            llmProviderId,
            model: selectedModel || ChatConfig.modelConfigToString(chatConfig.model), // Selected model for this chat (legacy format)
            messages: [systemMessage],
            systemPrompt: systemMessage.content, // System prompt with timestamp
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            currentTurn: 0, // Track conversation turns
            isSaved, // Track whether this chat has been saved to localStorage
            titleGenerated: false, // Track whether title has been generated
            // Per-chat rendering state
            renderingState: {
                lastDisplayedTurn: 0, // Track last displayed turn for separators
                currentStepInTurn: 1 // Track current step in turn
            },
            // Per-chat assistant group tracking (DOM element for grouping messages)
            currentAssistantGroup: null,
            // Per-chat pending tool calls map
            pendingToolCalls: new Map(),
            // Sub-chat support
            parentChatId,
            isSubChat: !!parentChatId,
            parentToolCallId,
            toolMetadata
        };
        
        // Create isolated MessageOptimizer instance for this chat
        chat.messageOptimizer = new MessageOptimizer(optimizerSettings);
        chat.config = chatConfig;
        
        // Add connection state to the chat
        chat.connectionState = 'CONNECTING';
        
        this.chats.set(chatId, chat);
        
        // Initialize token usage history for new chat
        this.tokenUsageHistory.set(chatId, {
            requests: [],
            model: selectedModel
        });
        
        // Save the config for next time (but not for sub-chats)
        if (!chat.isSubChat) {
            ChatConfig.saveLastConfig(chatConfig);
        }
        
        // Only save settings if this is a saved chat
        if (isSaved) {
            this.saveSettings();
        }
        this.updateChatSessions();
        
        // Connect to MCP server asynchronously
        this.ensureMcpConnection(mcpServerId, chatId)
            .then(() => {
                chat.connectionState = 'CONNECTED';
                // Update server list to show connected status
                this.updateMcpServersList();
            })
            .catch((error) => {
                chat.connectionState = 'FAILED';
                this.showError(`Failed to connect to MCP server: ${error.message}`, chatId);
            });
        
        return chatId;
    }

    // Get the currently active chat ID from the DOM
    getActiveChatId() {
        // First check the sidebar for active chat
        const activeSession = document.querySelector('.chat-session-item.active');
        if (activeSession && activeSession.dataset.chatId) {
            return activeSession.dataset.chatId;
        }
        // Check if any chat container has the active class
        const activeContainer = document.querySelector('.chat-container.active');
        if (activeContainer && activeContainer.dataset.chatId) {
            return activeContainer.dataset.chatId;
        }
        // Fallback: check chat.isActive property
        for (const [chatId, chat] of this.chats) {
            if (chat.isActive) {
                return chatId;
            }
        }
        return null;
    }

    updateChatSessions() {
        this.chatSessions.innerHTML = '';
        
        if (this.chats.size === 0) {
            this.chatSessions.innerHTML = '<div class="text-center text-muted mt-2">No chats yet</div>';
            return;
        }
        
        const sortedChats = Array.from(this.chats.values())
            .filter(chat => !chat.isSubChat) // Hide sub-chats from sidebar
            .sort((a, b) => {
                // Show unsaved chats first
                if (a.isSaved === false && b.isSaved !== false) {return -1;}
                if (b.isSaved === false && a.isSaved !== false) {return 1;}
                // Then sort by creation date (static order)
                // Use updatedAt as fallback for older chats without createdAt
                const aDate = a.createdAt || a.updatedAt;
                const bDate = b.createdAt || b.updatedAt;
                return new Date(bDate) - new Date(aDate);
            });
        
        // Separate new/unsaved chats from saved chats
        const newChats = sortedChats.filter(chat => chat.isSaved === false);
        const savedChats = sortedChats.filter(chat => chat.isSaved !== false);
        
        // Create sticky container for new chats if any exist
        if (newChats.length > 0) {
            const stickyContainer = document.createElement('div');
            stickyContainer.className = 'new-chats-sticky';
            
            for (const chat of newChats) {
                const sessionDiv = this.createChatSessionElement(chat);
                stickyContainer.appendChild(sessionDiv);
            }
            
            this.chatSessions.appendChild(stickyContainer);
        }
        
        // Add saved chats
        for (const chat of savedChats) {
            const sessionDiv = this.createChatSessionElement(chat);
            this.chatSessions.appendChild(sessionDiv);
        }
    }
    
    createChatSessionElement(chat) {
        const sessionDiv = document.createElement('div');
        sessionDiv.className = `chat-session-item ${chat.id === this.getActiveChatId() ? 'active' : ''} ${chat.isSaved === false ? 'unsaved' : ''}`;
        sessionDiv.dataset.chatId = chat.id;
        
            // Determine status
            let statusIcon;
            let statusClass;
            if (chat.wasWaitingOnLoad) {
                // Chat was saved while waiting for a response - broken state
                statusIcon = '<i class="fas fa-chain-broken"></i>';
                statusClass = 'status-broken';
            } else if (chat.hasError) {
                statusIcon = '<i class="fas fa-exclamation-triangle"></i>';
                statusClass = 'status-error';
            } else if (chat.spinnerState) {
                // Active spinner state
                switch (chat.spinnerState.type) {
                    case 'thinking':
                        statusIcon = '<i class="fas fa-robot"></i>';
                        statusClass = 'status-llm-active';
                        break;
                    case 'tool':
                        statusIcon = '<i class="fas fa-plug"></i>';
                        statusClass = 'status-mcp-active';
                        break;
                    case 'waiting':
                        statusIcon = '<i class="fas fa-clock"></i>';
                        statusClass = 'status-waiting';
                        break;
                    default:
                        statusIcon = '<i class="fas fa-spinner fa-spin"></i>';
                        statusClass = 'status-processing';
                }
            } else if (chat.isProcessing) {
                // Legacy processing state (fallback)
                statusIcon = '<i class="fas fa-spinner fa-spin"></i>';
                statusClass = 'status-processing';
            } else if (chat.messages && chat.messages.length > 1) {
                statusIcon = '<i class="fas fa-check-circle"></i>';
                statusClass = 'status-ready';
            } else {
                statusIcon = '<i class="fas fa-pause-circle"></i>';
                statusClass = 'status-idle';
            }
            
            // Check for draft message - must have actual content
            const hasDraft = chat.draftMessage && chat.draftMessage.trim().length > 0;
            
            
            // Get model display name
            let modelDisplay = 'No model';
            if (chat.hasInvalidModel) {
                modelDisplay = '⚠️ Invalid model';
            } else {
                const config = chat.config || ChatConfig.loadChatConfig(chat.id);
                if (config && config.model) {
                    const primaryModel = ChatConfig.modelConfigToString(config.model);
                    
                    modelDisplay = ChatConfig.getModelDisplayName(primaryModel);
                    // Shorten long model names
                    if (modelDisplay.length > 25) {
                        modelDisplay = modelDisplay.substring(0, 22) + '...';
                    }
                }
            }
            
            // Get MCP server display name
            let mcpDisplay = 'No MCP server';
            if (chat.mcpServerId && this.mcpServers.has(chat.mcpServerId)) {
                const server = this.mcpServers.get(chat.mcpServerId);
                mcpDisplay = server.name;
                // Shorten long server names
                if (mcpDisplay.length > 25) {
                    mcpDisplay = mcpDisplay.substring(0, 22) + '...';
                }
            }
            
            // Calculate context usage and tokens
            const tokenHistory = this.getTokenUsageForChat(chat.id);
            let contextPercent = '-';
            let contextPercentValue = 0;
            
            // Only show percentage if chat has been rendered or has token history
            if (chat.hasBeenRendered || tokenHistory.totalTokens > 0) {
                if (chat.config?.model?.id) {
                    const limit = this.getEffectiveContextWindow(chat);
                    contextPercentValue = Math.min(100, Math.round((tokenHistory.totalTokens / limit) * 100));
                    contextPercent = contextPercentValue + '';
                }
            }
            
            // Get cumulative tokens and price
            const cumulative = this.getCumulativeTokenUsage(chat.id);
            const totalTokens = cumulative.inputTokens + cumulative.outputTokens + 
                               cumulative.cacheReadTokens + cumulative.cacheCreationTokens;
            const totalPrice = this.calculateTokenCost(chat.id) || 0;
            
            // Format tokens
            let tokenDisplay;
            if (totalTokens >= 1000000) {
                tokenDisplay = `${(totalTokens / 1000000).toFixed(1)}M`;
            } else if (totalTokens >= 1000) {
                tokenDisplay = `${Math.round(totalTokens / 1000)}k`;
            } else {
                tokenDisplay = totalTokens.toString();
            }
            
            // Format date and time
            const updatedDate = new Date(chat.updatedAt);
            const dateStr = updatedDate.toLocaleDateString();
            const timeStr = updatedDate.toLocaleTimeString();
            
            // Add indicator for unsaved chats
            const titlePrefix = chat.isSaved === false ? '• ' : '';
            
            sessionDiv.innerHTML = `
                <div class="session-content">
                    <div class="session-row session-title-row">
                        <span class="session-title" data-tooltip="${chat.title}">${titlePrefix}${chat.title}</span>
                    </div>
                    <div class="session-row session-model-row">
                        <span class="session-model">${modelDisplay}</span>
                        <span class="session-date">${dateStr}</span>
                    </div>
                    <div class="session-row session-mcp-row">
                        <span class="session-mcp">${mcpDisplay}</span>
                        <span class="session-time">${timeStr}</span>
                    </div>
                    <div class="session-row session-metrics-row">
                        <div class="session-metrics">
                            <span class="metric-context" data-tooltip="Context window usage">${contextPercent}%</span>
                            <span class="metric-separator">•</span>
                            <span class="metric-tokens" data-tooltip="Total tokens">${tokenDisplay}</span>
                            <span class="metric-separator">•</span>
                            <span class="metric-price" data-tooltip="Total cost">$${totalPrice.toFixed(4)}</span>
                        </div>
                        <div class="session-actions">
                            ${hasDraft ? '<span class="status-icon status-draft" data-tooltip="Draft message"><i class="fas fa-edit"></i></span>' : ''}
                            <span class="status-icon ${statusClass}" data-tooltip="Chat status">${statusIcon}</span>
                            <button class="btn-delete-chat" data-chat-id="${chat.id}" data-tooltip="Delete chat">
                                <i class="fas fa-trash-alt"></i>
                            </button>
                        </div>
                    </div>
                </div>
            `;
            
            // Add event listeners
            const sessionContent = sessionDiv.querySelector('.session-content');
            sessionContent.addEventListener('click', () => {
                this.loadChat(chat.id);
            });
            
            const deleteBtn = sessionDiv.querySelector('.btn-delete-chat');
            deleteBtn.addEventListener('click', (event) => {
                event.stopPropagation();
                this.deleteChat(chat.id);
            });
            
            // Add model tooltip handlers
            const modelSpan = sessionDiv.querySelector('.session-model');
            if (modelSpan) {
                modelSpan.addEventListener('mouseenter', (e) => {
                    this.showModelTooltip(e, chat);
                });
                modelSpan.addEventListener('mouseleave', () => {
                    this.hideModelTooltip();
                });
            }
            
            return sessionDiv;
    }

    loadChat(chatId, forceRender = false) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Save draft and update UI for the previous active chat before switching
        const previousChatId = this.getActiveChatId();
        if (previousChatId && previousChatId !== chatId) {
            const previousChat = this.chats.get(previousChatId);
            if (previousChat) {
                // Save the previous chat (including its draft)
                this.autoSave(previousChatId);
                // Update chat list to show/hide draft indicator for the previous chat
                this.updateChatSessions();
            }
        }
        
        // Clean up empty draft messages from old chats
        if (chat.draftMessage === '' || (chat.draftMessage && chat.draftMessage.trim().length === 0)) {
            chat.draftMessage = null;
        }
        
        // Reset global state when switching chats to prevent state leakage
        this.resetGlobalChatState();
        
        // Also clear chat-specific stop flag when loading a chat
        chat.processingWasStoppedByUser = false;
        
        // If user has already selected a chat and this is the auto-created new chat trying to load, ignore it
        if (this.userHasSelectedChat && this.pendingNewChatId === chatId && this.getActiveChatId() !== chatId) {
            return;
        }
        
        // Mark that user has selected a chat
        this.userHasSelectedChat = true;
        
        // Cancel any pending new chat load if this is a different chat
        if (this.pendingNewChatId && this.pendingNewChatId !== chatId) {
            this.pendingNewChatLoad = false;
            // Don't clear pendingNewChatId here - we need it to block the switch
            
            // Also clear the timeout if it exists
            if (this.pendingNewChatTimeout) {
                clearTimeout(this.pendingNewChatTimeout);
                this.pendingNewChatTimeout = null;
            }
        }
        
        // Cancel any pending default chat creation
        if (this.defaultChatTimeout) {
            clearTimeout(this.defaultChatTimeout);
            this.defaultChatTimeout = null;
        }

        // Mark chat as active in the DOM (will be handled by switchChatDOM)
        
        // Switch to the chat's DOM
        this.switchChatDOM(chatId);
        
        // Ensure the chat container is properly laid out
        const chatContainer = this.getChatContainer(chatId);
        if (chatContainer) {
            // Force a reflow to ensure proper layout calculation
            // Force reflow by reading offsetHeight
            chatContainer.offsetHeight;
            
            // Ensure resize handles are properly initialized
            const elements = chatContainer._elements;
            if (elements && elements.inputResizeHandle) {
                // Re-initialize resize handle in case it wasn't properly set up
                requestAnimationFrame(() => {
                    if (!elements.inputResizeHandle._resizeInitialized) {
                        this.makeResizable(elements.inputResizeHandle, elements.input.parentElement.parentElement, 'vertical', 150, 400);
                        elements.inputResizeHandle._resizeInitialized = true;
                    }
                });
            }
        }
        
        // Update sidebar to show active chat
        this.updateChatSessions();
        
        // Migrate old chat data if needed
        if (!chat.totalTokensPrice || !chat.perModelTokensPrice) {
            this.migrateTokenPricing(chat);
        }
        
        // Ensure chat has context window set (for older chats that may not have it)
        if (chat.config && chat.config.model && !chat.config.model.params.contextWindow) {
            // For older chats without context window, set it to the model's default
            chat.config.model.params.contextWindow = this.getDefaultContextWindow(chat);
            // Save the updated config
            ChatConfig.saveChatConfig(chatId, chat.config);
            // Note: updateContextWindowIndicator will be called by updateChatHeader below
        }
        
        // Initialize token usage history for this chat if it doesn't exist
        if (!this.tokenUsageHistory.has(chatId)) {
            this.tokenUsageHistory.set(chatId, {
                requests: [],
                model: ChatConfig.getChatModelString(chat)
            });
            
            // Rebuild token history from saved messages
            const reconstructedRequests = [];
            for (const msg of chat.messages) {
                // Skip title and system messages - they don't affect context window
                if (['title', 'system-title', 'system-summary'].includes(msg.role)) {
                    continue;
                }
                
                // Include ALL messages with usage data for cumulative totals
                if (msg.usage && msg.usage.totalTokens) {
                    reconstructedRequests.push({
                        timestamp: msg.timestamp || new Date().toISOString(),
                        model: msg.model || chat.model, // Use message model if available, fallback to chat model
                        promptTokens: msg.usage.promptTokens || 0,
                        completionTokens: msg.usage.completionTokens || 0,
                        totalTokens: msg.usage.totalTokens,
                        cacheCreationInputTokens: msg.usage.cacheCreationInputTokens || 0,
                        cacheReadInputTokens: msg.usage.cacheReadInputTokens || 0
                    });
                }
            }
            
            if (reconstructedRequests.length > 0) {
                this.tokenUsageHistory.get(chatId).requests = reconstructedRequests;
            }
        }
        
        // Update header information
        this.updateChatHeader(chatId);
        
        // Get the container elements for this chat - use getChatContainer to ensure it exists
        const container = this.getChatContainer(chatId);
        if (!container) {return;}
        
        const elements = container._elements;
        const server = this.mcpServers.get(chat.mcpServerId);
        const provider = this.llmProviders.get(chat.llmProviderId);
        
        // Enable/disable input based on server and provider availability
        if (server && provider) {
            // Check if we have a connection
            if (this.mcpConnections.has(chat.mcpServerId)) {
                // Check if the connection is actually ready
                const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
                if (mcpConnection && mcpConnection.isReady()) {
                    // Only enable if connection ready AND chat not busy
                    if (!chat.isProcessing && !chat.spinnerState) {
                        elements.input.contentEditable = true;
                        elements.sendBtn.disabled = false;
                        elements.input.setAttribute('data-placeholder', 'Ask about your Netdata metrics...');
                        elements.reconnectBtn.style.display = 'none';
                    } else {
                        // Chat is busy - keep input disabled
                        elements.input.contentEditable = false;
                        elements.sendBtn.disabled = true;
                        elements.input.setAttribute('data-placeholder', 'Processing...');
                        elements.reconnectBtn.style.display = 'none';
                    }
                    
                } else {
                    // Connection exists but not ready yet
                    elements.input.contentEditable = false;
                    elements.sendBtn.disabled = true;
                    elements.input.setAttribute('data-placeholder', 'Connecting to MCP server...');
                    elements.reconnectBtn.style.display = 'none';
                    
                    // Check again in a moment
                    setTimeout(() => {
                        if (this.getActiveChatId() === chatId) {
                            this.updateChatInputState(chatId);
                        }
                    }, 500);
                }
            } else {
                // No connection yet - try to establish it
                elements.input.contentEditable = false;
                elements.sendBtn.disabled = true;
                elements.input.setAttribute('data-placeholder', 'Connecting to MCP server...');
                elements.reconnectBtn.style.display = 'none';
                
                // Try to establish connection
                this.ensureMcpConnection(chat.mcpServerId)
                    .then(() => {
                        // Connection established, update state
                        if (this.getActiveChatId() === chatId) {
                            this.updateChatInputState(chatId);
                        }
                    })
                    .catch(error => {
                        // Connection failed
                        console.error('Failed to connect to MCP server:', error);
                        if (this.getActiveChatId() === chatId) {
                            elements.input.setAttribute('data-placeholder', 'MCP server connection failed - click Reconnect');
                            elements.reconnectBtn.style.display = 'block';
                        }
                    });
            }
        } else {
            elements.input.contentEditable = false;
            elements.sendBtn.disabled = true;
            
            if (!server) {
                elements.input.setAttribute('data-placeholder', 'MCP server not found');
            } else if (!provider) {
                elements.input.setAttribute('data-placeholder', 'LLM provider not found');
            } else {
                elements.input.setAttribute('data-placeholder', 'MCP server or LLM provider not available');
            }
        }
        
        // Reset rendering state when loading a chat
        this.clearCurrentAssistantGroup(chatId); // Reset any current group (DOM element)
        if (chat.renderingState) {
            chat.renderingState.currentStepInTurn = 1; // Initialize step counter
            chat.renderingState.lastDisplayedTurn = 0; // Track last displayed turn for separators
        } else {
            // For older chats that don't have renderingState
            chat.renderingState = {
                lastDisplayedTurn: 0,
                currentStepInTurn: 1
            };
        }
        // Reset context window counter for delta calculation (stored per chat)
        chat.currentContextWindow = 0;
        
        // Check if we need to re-render messages
        // Re-render if: 1) Never rendered before, 2) DOM is empty (switched from another chat), 3) Force render requested
        const needsRender = !chat.hasBeenRendered || elements.messages.children.length === 0 || forceRender;
        
        if (needsRender) {
            // Clear and re-render messages
            elements.messages.innerHTML = '';
            
            // Clear pending tool calls map when re-rendering
            if (chat.pendingToolCalls) {
                chat.pendingToolCalls.clear();
            }
            
            // Display system prompt as first message
            const systemPromptToDisplay = chat.systemPrompt || this.defaultSystemPrompt;
            this.displaySystemPrompt(systemPromptToDisplay, chatId);
            
            // Track previous prompt tokens for delta calculation
            // const previousPromptTokens = 0;
            
            let inTitleGeneration = false;
            let inSummaryGeneration = false;
            
            for (let i = 0; i < chat.messages.length; i++) {
                const msg = chat.messages[i];
                if (msg.role === 'system') {continue;}
                
                // Check if this is the start of title generation
                if (msg.role === 'system-title' && !inTitleGeneration) {
                    inTitleGeneration = true;
                    // The "Generating chat title..." message is now shown in renderMessage before the collapsible
                }
                
                // Check if this is the start of summary generation
                if (msg.role === 'system-summary' && !inSummaryGeneration) {
                    inSummaryGeneration = true;
                }
                
                // Pass the message index to displayStoredMessage
                this.displayStoredMessage(msg, i, chatId);
                
                // Update previous tokens if this message had usage
                // if (msg.usage && msg.usage.promptTokens) {
                //     previousPromptTokens = msg.usage.promptTokens;
                // }
                
                // Check if title generation completed
                if (inTitleGeneration && msg.role === 'title') {
                    inTitleGeneration = false;
                    // Add ACTION section to the existing Chat Title block
                    const currentContainer = this.getChatContainer(chatId);
                    const messagesContainer = currentContainer && currentContainer._elements && currentContainer._elements.messages;
                    const titleBlock = messagesContainer ? messagesContainer.querySelector('.system-block[data-type="title"]') : null;
                    if (titleBlock) {
                        const systemContent = titleBlock.querySelector('.tool-content');
                        if (systemContent) {
                            // Check if ACTION section already exists
                            const existingAction = Array.from(systemContent.querySelectorAll('.tool-section-label'))
                                .find(label => label.textContent === 'ACTION');
                            if (!existingAction) {
                            // Add separator
                            const separator = document.createElement('div');
                            separator.className = 'tool-separator';
                            systemContent.appendChild(separator);
                            
                            // Add action section
                            const actionSection = document.createElement('div');
                            actionSection.className = 'tool-response-section';
                            
                            const actionControls = document.createElement('div');
                            actionControls.className = 'tool-section-controls';
                            
                            const actionLabel = document.createElement('span');
                            actionLabel.className = 'tool-section-label';
                            actionLabel.textContent = 'ACTION';
                            
                            actionControls.appendChild(actionLabel);
                            actionSection.appendChild(actionControls);
                            
                            const actionContent = document.createElement('pre');
                            actionContent.textContent = `Chat title updated: "${chat.title}"`;
                            actionSection.appendChild(actionContent);
                            
                            systemContent.appendChild(actionSection);
                            }
                        }
                    }
                }
                
                // Check if summary generation completed
                if (inSummaryGeneration && msg.role === 'summary') {
                    inSummaryGeneration = false;
                    // Find the summary block and add the RESPONSE section
                    const currentContainer = this.getChatContainer(chatId);
                    const messagesContainer = currentContainer && currentContainer._elements && currentContainer._elements.messages;
                    const summaryBlocks = messagesContainer ? messagesContainer.querySelectorAll('.system-block[data-type="summary"]') : [];
                    const summaryBlock = summaryBlocks[summaryBlocks.length - 1]; // Get the last one
                    
                    if (summaryBlock) {
                        const systemContent = summaryBlock.querySelector('.tool-content');
                        if (systemContent) {
                            // Check if RESPONSE section already exists
                            const existingResponse = Array.from(systemContent.querySelectorAll('.tool-section-label'))
                                .find(label => label.textContent === 'RESPONSE');
                            if (!existingResponse) {
                                // Add separator
                                const separator = document.createElement('div');
                                separator.className = 'tool-separator';
                                systemContent.appendChild(separator);
                                
                                // Add response section with the actual summary
                                const responseSection = document.createElement('div');
                                responseSection.className = 'tool-response-section';
                                
                                const responseControls = document.createElement('div');
                                responseControls.className = 'tool-section-controls';
                                
                                const responseLabel = document.createElement('span');
                                responseLabel.className = 'tool-section-label';
                                responseLabel.textContent = 'RESPONSE';
                                
                                // Add copy button for the response
                                const copyBtn = document.createElement('button');
                                copyBtn.className = 'tool-section-copy';
                                copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
                                copyBtn.onclick = () => {
                                    this.copyToClipboard(msg.content, copyBtn).catch(error => {
                                        console.error('Failed to copy to clipboard:', error);
                                    });
                                };
                                
                                responseControls.appendChild(responseLabel);
                                responseControls.appendChild(copyBtn);
                                responseSection.appendChild(responseControls);
                                
                                // Create a div for the summary content with markdown rendering
                                const responseContent = document.createElement('div');
                                responseContent.className = 'message-content';
                                responseContent.innerHTML = marked.parse(msg.content, {
                                    breaks: true, gfm: true, sanitize: false
                                });
                                responseSection.appendChild(responseContent);
                                
                                systemContent.appendChild(responseSection);
                            }
                        }
                    }
                }
                
                // Reset assistant group after each message to ensure proper separation
                if (msg.role === 'tool-results') {
                    this.clearCurrentAssistantGroup(chatId);
                }
            }
            // Clear current group after loading
            this.clearCurrentAssistantGroup(chatId);
            
            // Mark chat as rendered  
            chat.hasBeenRendered = true;
            
            // For loaded chats, sub-chats are already rendered as part of tool results
            // via the sub-chat-indicator in addToolResult, so we don't need to render them separately
        }
        
        // Update global toggle UI based on chat's tool inclusion mode
        
        // Always update the context window and cumulative tokens
        this.updateContextWindowIndicator(chatId);
        this.updateCumulativeTokenDisplay(chatId);
        
        // Update all token displays
        this.updateAllTokenDisplays(chatId);
        
        // Update chat sessions to reflect the calculated context window
        this.updateChatSessions();
        
        // Force scroll to bottom when loading chat (ignore the isAtBottom check)
        // Use setTimeout with requestAnimationFrame to ensure layout is complete
        // This is important when switching chats during page load
        setTimeout(() => {
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    const targetContainer = this.getChatContainer(chatId);
                    const messagesContainer = targetContainer && targetContainer._elements && targetContainer._elements.messages;
                    if (messagesContainer) {
                        messagesContainer.scrollTop = messagesContainer.scrollHeight;
                    }
                    
                    // Focus the chat input after scrolling
                    const chatInput = container && container._elements && container._elements.input;
                    if (chatInput && chatInput.contentEditable === 'true') {
                        chatInput.focus();
                    }
                    
                    // Restore draft message if exists
                    if (chatInput && chat.draftMessage) {
                        // Convert markdown to HTML for contentEditable display
                        const htmlContent = chat.draftMessage
                            .replace(/&/g, '&amp;')
                            .replace(/</g, '&lt;')
                            .replace(/>/g, '&gt;')
                            .replace(/\n/g, '<br>');
                        chatInput.innerHTML = htmlContent;
                        
                        // Update send button state
                        const sendBtn = container._elements.sendBtn;
                        if (sendBtn) {
                            sendBtn.disabled = !chat.draftMessage.trim();
                        }
                    }
                });
            });
        }, 50); // Small delay to ensure DOM is fully ready
    }

    displayStoredMessage(msg, messageIndex, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Handle turn tracking for stored messages
        if (msg.role === 'user') {
            const msgTurn = msg.turn || 0;
            
            // Add turn separator if we're entering a new turn
            if (msgTurn > 0 && msgTurn !== chat.renderingState.lastDisplayedTurn) {
                this.addTurnSeparator(msgTurn, chatId);
                chat.renderingState.currentStepInTurn = 1;
                chat.renderingState.lastDisplayedTurn = msgTurn;
            }
        }
        
        // Convert stored message to event stream
        const events = this.convertMessageToEvents(msg);
        
        // Process each event
        for (const event of events) {
            // Pass turn info to events if available
            if (msg.turn !== undefined) {
                event.turn = msg.turn;
            }
            // Pass message index for redo functionality
            if (messageIndex !== undefined) {
                event.messageIndex = messageIndex;
            }
            this.processRenderEvent(event, chatId);
        }
    }
    
    // Convert a stored message into a sequence of rendering events
    convertMessageToEvents(msg) {
        const events = [];
        
        // Handle messages by role (new system) or type (legacy)
        const messageRole = msg.role;
        
        switch(messageRole) {
            case 'user':
                events.push({ type: 'user-message', content: msg.content });
                break;
                
            case 'system-title':
                events.push({ type: 'system-title-message', content: msg.content, usage: msg.usage, responseTime: msg.responseTime, model: msg.model, price: msg.price });
                break;
                
            case 'system-summary':
                events.push({ type: 'system-summary-message', content: msg.content, usage: msg.usage, responseTime: msg.responseTime, model: msg.model, price: msg.price });
                break;
                
            case 'title':
                events.push({ type: 'title-message', content: msg.content, usage: msg.usage, responseTime: msg.responseTime, model: msg.model, price: msg.price });
                break;
                
            case 'summary':
                events.push({ type: 'summary-message', content: msg.content, usage: msg.usage, responseTime: msg.responseTime, model: msg.model, price: msg.price });
                break;
                
            case 'accounting':
                events.push({ type: 'accounting-message', data: msg });
                break;
                
            case 'assistant':
                // Add assistant metrics event FIRST
                if (msg.usage || msg.responseTime) {
                    events.push({ 
                        type: 'assistant-metrics', 
                        usage: msg.usage, 
                        responseTime: msg.responseTime,
                        model: msg.model
                    });
                }
                
                // Then add assistant message event
                if (msg.content) {
                    events.push({ type: 'assistant-message', content: msg.content });
                }
                
                // Extract and add tool calls from content array
                const tools = this.extractToolsFromContent(msg.content);
                if (tools.length > 0) {
                    for (const tool of tools) {
                        if (!tool.id) {
                            console.error('[convertMessageToEvents] Tool call missing required id:', tool);
                            continue; // Skip invalid tool calls
                        }
                        events.push({ 
                            type: 'tool-call', 
                            name: tool.name, 
                            arguments: tool.arguments,
                            id: tool.id,  // Required tool call ID for matching
                            includeInContext: true, // Tools in content are always included
                            turn: msg.turn
                        });
                    }
                }
                break;
                
            case 'tool-results':
                // Add tool results with their inclusion state
                const toolResults = msg.toolResults || [];
                for (const result of toolResults) {
                    if (!result.toolCallId) {
                        console.error('[convertMessageToEvents] Tool result missing required toolCallId:', result);
                        continue; // Skip invalid tool results
                    }
                    events.push({ 
                        type: 'tool-result', 
                        name: result.name || result.toolName, 
                        result: result.result,
                        toolCallId: result.toolCallId,  // Required tool call ID for matching
                        includeInContext: result.includeInContext,
                        subChatId: result.subChatId,  // Include sub-chat ID if present
                        wasProcessedBySubChat: result.wasProcessedBySubChat  // Include processing status
                    });
                }
                // Reset assistant group after tool results
                events.push({ type: 'reset-assistant-group' });
                break;
                
            // Handle old format - MUST have id
            case 'tool-call':
                if (!msg.id) {
                    console.error('[convertMessageToEvents] tool-call message missing required id:', msg);
                    break;
                }
                events.push({ type: 'tool-call', name: msg.toolName, arguments: msg.args, id: msg.id });
                break;
            case 'tool-result':
                if (!msg.toolCallId) {
                    console.error('[convertMessageToEvents] tool-result message missing required toolCallId:', msg);
                    break;
                }
                events.push({ type: 'tool-result', name: msg.toolName, result: msg.result, toolCallId: msg.toolCallId });
                break;
                
            case 'system':
                events.push({ type: 'system-message', content: msg.content });
                break;
                
            case 'error':
                events.push({ 
                    type: 'error-message', 
                    content: msg.content, 
                    errorMessageIndex: msg.errorMessageIndex,
                    isRetryable: msg.isRetryable,
                    retryButtonLabel: msg.retryButtonLabel
                });
                break;
                
            default:
                console.warn('Unknown message role:', messageRole);
                break;
        }
        
        return events;
    }
    
    // Process a single rendering event
    processRenderEvent(event, chatId) {
        if (!chatId) {
            console.error('processRenderEvent called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`processRenderEvent: chat ${chatId} not found`);
            return;
        }
        
        // Clear error state when processing any event (it will be set again if there's an error)
        chat.hasError = false;
        chat.lastError = null;
        
        // Ensure chat has renderingState
        if (!chat.renderingState) {
            chat.renderingState = {
                lastDisplayedTurn: 0,
                currentStepInTurn: 1
            };
        }
        
        switch(event.type) {
            case 'user-message':
                this.clearCurrentAssistantGroup(chatId);
                const turn = event.turn !== undefined ? event.turn : chat ? chat.currentTurn : 0;
                
                // For live messages, add turn separator if this is a new turn
                if (event.turn === undefined && chat && chat.currentTurn > 0 && chat.currentTurn !== chat.renderingState.lastDisplayedTurn) {
                    this.addTurnSeparator(chat.currentTurn, chatId);
                    chat.renderingState.currentStepInTurn = 1; // Reset step counter
                    chat.renderingState.lastDisplayedTurn = chat.currentTurn;
                }
                
                this.renderMessage('user', event.content, event.messageIndex, chatId);
                if (turn > 0) {
                    this.addStepNumber(turn, chat.renderingState.currentStepInTurn++, chatId);
                }
                break;
                
            case 'assistant-metrics':
                // Simply append metrics to the chat
                this.appendMetricsToChat(event.usage, event.responseTime, event.model, chatId, 'assistant');
                break;
                
            case 'assistant-message':
                this.renderMessage('assistant', event.content, event.messageIndex, chatId);
                const turn2 = event.turn !== undefined ? event.turn : chat ? chat.currentTurn : 0;
                if (turn2 > 0 && this.getCurrentAssistantGroup(chatId)) {
                    this.addStepNumber(turn2, chat.renderingState.currentStepInTurn++, chatId, this.getCurrentAssistantGroup(chatId));
                }
                break;
                
            case 'tool-call':
                // Ensure we have a group
                if (!this.getCurrentAssistantGroup(chatId)) {
                    this.renderMessage('assistant', '', undefined, chatId);
                }
                // Use destructuring to avoid direct 'arguments' reference
                const { arguments: eventArgs } = event || {};
                this.addToolCall(event.name, eventArgs, chatId, event.turn, event.messageIndex, event.includeInContext !== false, event.id);
                break;
                
            case 'tool-result':
                // Check if we need to render sub-chat DOM for loaded chats
                if (event.subChatId) {
                    const subChat = this.chats.get(event.subChatId);
                    if (!subChat) {
                        console.error(`[processRenderEvent] Sub-chat not found for subChatId: ${event.subChatId} in tool-result event for chatId: ${chatId}, toolCallId: ${event.toolCallId}`);
                        break;
                    }
                    if (subChat) {
                        // Check if sub-chat DOM already exists
                        const container = this.getChatContainer(chatId);
                        const existingSubChatDom = container && container._elements.messages.querySelector(`[data-sub-chat-id="${event.subChatId}"]`);
                        
                        if (!existingSubChatDom) {
                            console.log(`[processRenderEvent] Sub-chat DOM not found for ${event.subChatId}, creating it now`);
                            // Determine status based on whether the sub-chat successfully processed the tool
                            const status = event.wasProcessedBySubChat ? 'success' : 'failed';
                            // Render sub-chat DOM element
                            this.renderSubChatAsItem(chatId, event.subChatId, event.toolCallId, status);
                        } else {
                            console.log(`[processRenderEvent] Sub-chat DOM already exists for ${event.subChatId}, skipping duplicate creation`);
                        }
                    }
                }
                
                this.addToolResult(event.name, event.result, chatId, event.responseTime || 0, event.responseSize || null, event.includeInContext, event.messageIndex, event.toolCallId, event.subChatId);
                break;
                
                
            case 'system-message':
                this.addSystemMessage(event.content, chatId);
                break;
                
            case 'system-title-message':
                this.renderMessage('system-title', event.content, event.messageIndex, chatId);
                break;
                
            case 'system-summary-message':
                this.renderMessage('system-summary', event.content, event.messageIndex, chatId);
                break;
                
            case 'title-message':
                this.renderMessage('title', event.content, event.messageIndex, chatId);
                // For title/summary, append metrics AFTER rendering the message
                if (event.usage) {
                    this.appendMetricsToChat(event.usage, event.responseTime, event.model, chatId, 'title');
                }
                break;
                
            case 'summary-message':
                this.renderMessage('summary', event.content, event.messageIndex, chatId);
                // For title/summary, append metrics AFTER rendering the message
                if (event.usage) {
                    this.appendMetricsToChat(event.usage, event.responseTime, event.model, chatId, 'summary');
                }
                break;
                
            case 'accounting-message':
                this.addAccountingNode(event.data, chatId);
                break;
                
            case 'error-message':
                // Set error state for error messages
                chat.hasError = true;
                chat.lastError = event.content;
                
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                messageDiv.style.position = 'relative';
                
                // Show the error message
                messageDiv.innerHTML = `<div><i class="fas fa-times-circle"></i> ${event.content}</div>`;
                
                // Handle different button types based on the error
                if (event.isRetryable && event.retryButtonLabel) {
                    // Custom retry button (e.g., Continue for user stop)
                    const retryBtn = document.createElement('button');
                    retryBtn.className = 'btn btn-warning btn-small';
                    retryBtn.style.marginTop = '8px';
                    const buttonIcon = event.retryButtonLabel === 'Continue' ? 'fa-play' : 'fa-redo';
                    retryBtn.innerHTML = `<i class="fas ${buttonIcon}"></i> ${event.retryButtonLabel}`;
                    
                    retryBtn.onclick = async () => {
                        retryBtn.disabled = true;
                        retryBtn.textContent = 'Processing...';
                        
                        // Find and remove the pause message
                        const errorIndex = chat.messages.findIndex(m => m.role === 'error' && m.content === event.content);
                        if (errorIndex !== -1) {
                            this.removeMessage(chatId, errorIndex, 1);
                            
                            // Reload chat to remove the error from display
                            this.loadChat(chatId, true);
                        }
                        
                        // Resume the interrupted processing without adding a new user message
                        await this.resumeProcessing(chatId);
                    };
                    
                    messageDiv.appendChild(retryBtn);
                } else if (!chat.isSubChat && event.errorType !== 'safety_limit' && (event.errorMessageIndex !== undefined && event.errorMessageIndex >= 0)) {
                    // This error has context - use redo button (but not in sub-chats)
                    const redoBtn = document.createElement('button');
                    redoBtn.className = 'redo-button';
                    redoBtn.textContent = 'Redo';
                    redoBtn.onclick = () => this.redoFromMessage(event.errorMessageIndex, chatId);
                    messageDiv.appendChild(redoBtn);
                } else if (event.errorType !== 'safety_limit') {
                    // Create a retry button with error type info
                    const retryBtn = document.createElement('button');
                    retryBtn.className = 'btn btn-warning btn-small';
                    retryBtn.style.marginTop = '8px';
                    retryBtn.innerHTML = '<i class="fas fa-redo"></i> Retry';
                    
                    // Store error type and chat ID in the button
                    retryBtn.dataset.chatId = chatId;
                    retryBtn.dataset.errorType = event.errorType || 'llm_error';
                    
                    retryBtn.onclick = async () => {
                        retryBtn.disabled = true;
                        retryBtn.textContent = 'Retrying...';
                        
                        const errorType = retryBtn.dataset.errorType;
                        const retryChatId = retryBtn.dataset.chatId;
                        
                        // Remove the error message from chat
                        const currentChat = this.chats.get(retryChatId);
                        if (!currentChat) {
                            console.error(`[processRenderEvent] Chat not found for retry operation, retryChatId: ${retryChatId}`);
                            return;
                        }
                        
                        const errorIndex = currentChat.messages.findIndex(m => m.role === 'error' && m.content === event.content);
                        if (errorIndex !== -1) {
                            this.removeMessage(retryChatId, errorIndex, 1);
                            this.loadChat(retryChatId, true);
                        }
                        
                        try {
                            switch(errorType) {
                                case 'llm_error':
                                    await this.retryLLMRequest(retryChatId);
                                    break;
                                    
                                case 'mcp_error':
                                    // Try to reconnect MCP first
                                    const mcpConnection = this.mcpConnections.get(currentChat.mcpServerId);
                                    if (!mcpConnection || !mcpConnection.connected) {
                                        await this.connectMCPServer(currentChat.mcpServerId);
                                    }
                                    // Then retry the request
                                    await this.retryLLMRequest(retryChatId);
                                    break;
                                    
                                case 'tool_error':
                                    // For tool errors, retry from the last LLM state
                                    await this.retryLLMRequest(retryChatId);
                                    break;
                                    
                                default:
                                    // Default to LLM retry
                                    await this.retryLLMRequest(retryChatId);
                            }
                        } catch (error) {
                            console.error('Retry failed:', error);
                            // Error will be displayed by the retry methods
                        }
                    };
                    
                    messageDiv.appendChild(retryBtn);
                }
                
                // Use chat-specific messages container
                const container = this.getChatContainer(chatId);
                if (container && container._elements && container._elements.messages) {
                    container._elements.messages.appendChild(messageDiv);
                }
                break;
                
            case 'show-spinner':
                // Determine spinner type based on text
                if (event.text && event.text.includes('Executing')) {
                    // Extract tool name from "Executing toolName..."
                    const match = event.text.match(/Executing (.+)\.\.\./);
                    const toolName = match ? match[1] : 'tool';
                    this.showToolExecuting(chatId, toolName);
                } else if (event.text && event.text.includes('Waiting')) {
                    // Extract seconds from "Waiting... Xs"
                    const match = event.text.match(/Waiting\.\.\. (\d+)s/);
                    const seconds = match ? parseInt(match[1], 10) : 0;
                    this.showWaitingCountdown(chatId, seconds);
                } else {
                    // Default to thinking
                    this.showAssistantThinking(chatId);
                }
                break;
                
            case 'hide-spinner':
                // Clear any spinner state
                this.clearSpinnerState(chatId);
                break;
                
            case 'reset-assistant-group':
                // Reset the current assistant group so next content starts fresh
                this.clearCurrentAssistantGroup(chatId);
                break;
                
            default:
                console.warn('Unknown render event type:', event.type);
                break;
        }
        
        // Use chat-specific methods
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
    }

    async deleteChat(chatId) {
        if (!chatId) {return;}
        
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        const confirmed = await this.showConfirmDialog(
            'Delete Chat',
            `Are you sure you want to delete "${chat.title}"?`,
            'Delete',
            'Cancel',
            true // danger style
        );
        
        if (confirmed) {
            // Delete sub-chats first (cascade delete)
            for (const [subChatId, subChat] of this.chats) {
                if (subChat.parentChatId === chatId) {
                    // Clean up sub-chat container
                    if (this.chatContainers.has(subChatId)) {
                        this.chatContainers.delete(subChatId);
                    }
                    this.chats.delete(subChatId);
                    localStorage.removeItem(subChatId);
                    localStorage.removeItem(`chatConfig_${subChatId}`);
                }
            }
            
            this.chats.delete(chatId);
            
            // Remove from storage
            localStorage.removeItem(chatId);
            
            // Also remove chat config if it exists
            localStorage.removeItem(`chatConfig_${chatId}`);
            
            this.updateChatSessions();
            
            // If this was the current chat, clear the display
            if (this.getActiveChatId() === chatId) {
                // Remove the chat container if it exists
                if (this.chatContainers.has(chatId)) {
                    const container = this.chatContainers.get(chatId);
                    if (container && container.parentNode) {
                        container.parentNode.removeChild(container);
                    }
                    this.chatContainers.delete(chatId);
                }
                // Switch to another chat or create a new one
                if (this.chats.size === 0) {
                    // No chats remain, create a new one
                    this.createNewChatDirectly().catch(error => {
                        console.error('Failed to create new chat:', error);
                    });
                } else {
                    // Switch to the most recently updated chat
                    let mostRecentChat = null;
                    let mostRecentTime = null;
                    
                    for (const [id, chatEntry] of this.chats) {
                        const updatedTime = new Date(chatEntry.updatedAt);
                        if (!mostRecentTime || updatedTime > mostRecentTime) {
                            mostRecentTime = updatedTime;
                            mostRecentChat = id;
                        }
                    }
                    
                    if (mostRecentChat) {
                        this.loadChat(mostRecentChat);
                    }
                }
                // These elements might not exist since we're between chats
                if (this.sendMessageBtn) {
                    this.sendMessageBtn.disabled = true;
                }
                if (this.reconnectMcpBtn) {
                    this.reconnectMcpBtn.style.display = 'none';
                }
                // Temperature control removed - now in modal
                
                // Reset dropdown buttons if they exist
                if (this.currentModelText) {
                    this.currentModelText.textContent = 'Model';
                }
                if (this.currentMcpText) {
                    this.currentMcpText.textContent = 'MCP Server';
                }
                
                // Show empty context window indicator
                const indicator = document.getElementById('contextWindowIndicator');
                if (indicator) {
                    indicator.style.display = 'flex';
                    // Clear the global indicator when no chat is selected
                    const fillElement = indicator.querySelector('.context-window-fill');
                    const statsElement = indicator.querySelector('.context-window-stats');
                    if (fillElement) {fillElement.style.width = '0%';}
                    if (statsElement) {statsElement.textContent = '0 / 0';}
                }
                // Hide token counters when no chat is selected
                const tokenCounters = document.getElementById('tokenCounters');
                if (tokenCounters) {
                    tokenCounters.style.display = 'none';
                }
            }
        }
    }

    // Update send button appearance based on processing state
    updateSendButton(chatId) {
        const chat = this.chats.get(chatId);
        if (chat && chat.isProcessing) {
            this.sendMessageBtn.textContent = 'Stop';
            this.sendMessageBtn.classList.remove('btn-send');
            this.sendMessageBtn.classList.add('btn-danger');
            this.sendMessageBtn.disabled = false;
        } else {
            this.sendMessageBtn.textContent = 'Send';
            this.sendMessageBtn.classList.remove('btn-danger');
            this.sendMessageBtn.classList.add('btn-send');
            // Get chat-specific input
            const chatInput = this.getChatInput(chatId);
            if (!chatInput) {
                const chatData = this.chats.get(chatId);
                if (!chatData || !chatData.isSubChat) {
                    // Only log error for main chats - sub-chats don't have input elements
                    console.error('[updateSendButton] ERROR: Could not find input for chat', chatId);
                }
                this.sendMessageBtn.disabled = true;
            } else {
                try {
                    const content = this.getEditableContent(chatInput);
                    this.sendMessageBtn.disabled = !content.trim();
                } catch (error) {
                    console.error('[updateSendButton] ERROR getting editable content:', error);
                    this.sendMessageBtn.disabled = true;
                }
            }
        }
    }
    
    // Messaging
    async sendMessage(chatId, messageParam = null, isResume = false) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}

        // If no message provided, get it from the input
        let message = messageParam;
        if (message === null) {
            // Get chat-specific input
            const chatInput = this.getChatInput(chatId);
            if (!chatInput) {
                if (!chat.isSubChat) {
                    // Only log error for main chats - sub-chats don't have input elements
                    console.error('[sendMessage] ERROR: Could not find input for chat', chatId);
                    this.showError('Chat input not found', chatId);
                    return;
                }
                // For sub-chats, continue without input element
            }
            
            // For contentEditable input, extract formatted content
            try {
                message = this.getEditableContent(chatInput).trim();
                if (!message && !isResume) {return;}
            } catch (error) {
                console.error('[sendMessage] ERROR extracting message content:', error);
                this.showError('Failed to get message content', chatId);
                return;
            }
        }

        // Clear error state when sending new message
        chat.hasError = false;
        chat.lastError = null;
        
        // Clear broken state when starting new interaction
        chat.wasWaitingOnLoad = false;
        
        // Validate configuration early
        const proxyProvider = this.llmProviders.get(chat.llmProviderId);
        if (!proxyProvider) {
            this.showError('LLM proxy not available', chat.id);
            return;
        }
        
        // Get model config
        const providerType = chat.config?.model?.provider;
        const modelName = chat.config?.model?.id;
        if (!providerType || !modelName) {
            this.showError('sendMessage(): Invalid model configuration', chat.id);
            return;
        }
        
        // Clear any current assistant group since we're starting a new conversation turn
        this.clearCurrentAssistantGroup(chat.id);
        
        // Reset safety iteration counter for new user message
        this.safetyChecker.resetIterations(chat.id);
        
        // Clear the draft since we're sending the message
        chat.draftMessage = null;
        // Clear any pending draft save timeout
        if (this.draftSaveTimeout) {
            clearTimeout(this.draftSaveTimeout);
            this.draftSaveTimeout = null;
        }
        this.updateChatSessions(); // Update UI to remove draft indicator
        
        // Disable input and update button to Stop
        // Get chat-specific input element (only for main chats, not sub-chats)
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.input) {
            const input = container._elements.input;
            if (!isResume) {
                input.innerHTML = '';
            }
            input.contentEditable = false;
        } else if (!chat.isSubChat) {
            // Only log error for main chats - sub-chats don't have input elements
            console.error('[sendMessage] ERROR: Could not find chat input element for chat', chatId);
        }
        chat.isProcessing = true;
        chat.shouldStopProcessing = false;
        this.updateSendButton(chatId);
        
        // Only add user message if this is not a resume (resume continues from existing messages)
        if (!isResume) {
            // Increment turn counter for new user message
            chat.currentTurn = (chat.currentTurn || 0) + 1;
            
            // CRITICAL: Add and display the user's message immediately for better UX
            // Add stable timestamp to ensure cache control works properly
            this.addMessage(chat.id, { 
                role: 'user', 
                content: message, 
                turn: chat.currentTurn,
                timestamp: new Date().toISOString()
            });
            const userMessageIndex = chat.messages.length - 1;
            this.processRenderEvent({ type: 'user-message', content: message, messageIndex: userMessageIndex }, chat.id);
        }
        
        // Force scroll to bottom after DOM update
        requestAnimationFrame(() => {
            this.scrollToBottom(chat.id, true);
        });
        
        // If this is the first message in an unsaved chat, mark it as saved
        if (chat.isSaved === false && this.isFirstUserMessage(chat)) {
            chat.isSaved = true;
            this.updateChatSessions();
        }
        
        // Save this specific chat after adding message
        this.autoSave(chat.id);
        
        // Now check MCP connection asynchronously
        let mcpConnection;
        try {
            // Try to ensure MCP connection (will reconnect if needed)
            mcpConnection = await this.ensureMcpConnection(chat.mcpServerId);
        } catch (error) {
            this.showError(`Failed to connect to MCP server: ${error.message}`, chat.id);
            // Re-enable input so user can try again
            const errorContainer = this.getChatContainer(chatId);
            if (errorContainer && errorContainer._elements && errorContainer._elements.input) {
                errorContainer._elements.input.contentEditable = true;
            } else if (!chat.isSubChat) {
                // Only log error for main chats - sub-chats don't have input elements
                console.error('[sendMessage] ERROR: Could not find chat input element when re-enabling after MCP error for chat', chatId);
            }
            chat.isProcessing = false;
            this.updateSendButton(chatId);
            return;
        }
        
        // Get the API type from the provider configuration
        const providerApiType = proxyProvider.availableProviders?.[providerType]?.type || providerType;
        
        // Create the actual LLM provider instance
        const provider = window.createLLMProvider(providerApiType, proxyProvider.proxyUrl, modelName, chat.config.model);
        provider.onLog = (logEntry) => {
            const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
            const providerName = providerType.charAt(0).toUpperCase() + providerType.slice(1);
            this.addLogEntry(`${prefix}: ${providerName}`, logEntry);
        };
        
        // Show thinking spinner
        this.showAssistantThinking(chat.id);
        
        try {
            const result = await this.processMessageWithTools(chat, mcpConnection, provider, isResume ? null : message);
            
            // Check if rate limit was handled - don't conclude if so
            if (result && result.rateLimitHandled) {
                return;
            }
            
            // Check if we should generate a title automatically
            // Skip title generation if:
            // 1. Processing was stopped by user
            // 2. The last message sequence is incomplete (no assistant response)
            const lastMessage = chat.messages[chat.messages.length - 1];
            const hasCompleteSequence = lastMessage && lastMessage.role === 'assistant';
            
            if (!chat.shouldStopProcessing && hasCompleteSequence && this.isFirstUserMessage(chat) && TitleGenerator.shouldGenerateTitleAutomatically(chat)) {
                const llmProxy = this.llmProviders.get(chat.llmProviderId);
                if (llmProxy) {
                    const titleProvider = TitleGenerator.getTitleGenerationProvider(
                        chat, 
                        llmProxy, 
                        provider, 
                        createLLMProvider
                    );
                    
                    // Generate title automatically (force=false)
                    await TitleGenerator.generateChatTitle(
                        chat, 
                        mcpConnection, 
                        titleProvider, 
                        true, 
                        false,
                        this.getTitleGenerationCallbacks()
                    );
                }
            }
            
            // Check if we should generate a summary (conditions to be defined later)
            if (this.shouldGenerateSummary(chat)) {
                try {
                    await this.generateChatSummary(chat, mcpConnection, provider, true);
                } catch (error) {
                    // Log but don't interrupt the flow for automatic summaries
                    console.error('Automatic summary generation failed:', error);
                }
            }
            
            // Success - assistant has concluded
            this.assistantConcluded(chat.id);
            
            // Reset processing state for this chat
            chat.isProcessing = false;
            chat.shouldStopProcessing = false;
            
            // Re-enable send button if the input has text
            const chatInput = this.getChatInput(chatId);
            if (chatInput) {
                try {
                    const content = this.getEditableContent(chatInput).trim();
                    if (content && this.sendBtn) {
                        this.sendBtn.disabled = false;
                    }
                } catch (error) {
                    console.error('[sendMessage] ERROR checking input content:', error);
                }
            } else if (!chat.isSubChat) {
                // Only log warning for main chats - sub-chats don't have input elements
                console.error('[sendMessage] WARNING: Could not find input for chat when trying to re-enable send button', chatId);
            }
        } catch (error) {
            // Check if the user stopped processing
            if (chat.shouldStopProcessing || chat.processingWasStoppedByUser || error.isUserStop) {
                // Clear the flag for next time
                chat.processingWasStoppedByUser = false;
                
                // Clear the spinner immediately
                this.clearSpinnerState(chat.id);
                
                // User clicked Stop - show a friendly message with Continue option
                const continueMessage = '⏸️ Processing paused. You can continue the conversation by sending a new message or clicking Continue.';
                
                // Find the last user message index for error tracking
                const lastUserMsgIdx = chat.messages.findLastIndex(m => m.role === 'user');
                
                // Add the error message with retry info to chat history
                const errorMsg = {
                    role: 'error',
                    content: continueMessage,
                    errorMessageIndex: lastUserMsgIdx,
                    isRetryable: true,
                    retryButtonLabel: 'Continue'
                };
                
                this.addMessage(chat.id, errorMsg);
                
                // Process the render event to display it (only once)
                this.processRenderEvent({ 
                    type: 'error-message', 
                    content: continueMessage, 
                    errorMessageIndex: lastUserMsgIdx,
                    isRetryable: true,
                    retryButtonLabel: 'Continue'
                }, chat.id);
                
                // Clean up states
                this.assistantFailed(chat.id, error);
            } else {
                // Check for rate limit error (429 or 529)
                const isRateLimitError = error.message && (
                    error.message.includes('Rate limit') || 
                    error.message.includes('429') ||
                    error.message.includes('rate_limit_exceeded') ||
                    error.message.includes('529') ||
                    error.message.includes('overloaded_error') ||
                    error.message.includes('Overloaded')
                );
                const retryMatch = error.message.match(/Please try again in (\d+(?:\.\d+)?)s/);
                const retryAfterSeconds = retryMatch ? parseFloat(retryMatch[1]) : null;
                
                if (isRateLimitError) {
                    // Handle rate limit with automatic retry (even without retry time)
                    const retryCount = chat.rateLimitRetryCount || 0;
                    this.handleRateLimitError(chat.id, retryAfterSeconds, retryCount);
                    // Mark error as handled to prevent spinner clearing
                    error._rateLimitHandled = true;
                    // Clean up but preserve waiting spinner
                    this.assistantFailed(chat.id, error);
                } else {
                    // Regular error handling
                    const errorMessage = `Error: ${error.message}`;
                    const lastUserMessageIndex = chat.messages.findLastIndex(m => m.role === 'user');
                    
                    // Determine error type for retry
                    let errorType = 'llm_error'; // Default to LLM error
                    if (error.message.includes('MCP') || error.message.includes('connection')) {
                        errorType = 'mcp_error';
                    } else if (error.message.includes('Tool')) {
                        errorType = 'tool_error';
                    }
                    
                    this.addMessage(chat.id, { 
                        role: 'error', 
                        content: errorMessage, 
                        errorMessageIndex: lastUserMessageIndex,
                        errorType 
                    });
                    
                    // Display the error message with error type
                    this.processRenderEvent({ 
                        type: 'error-message', 
                        content: errorMessage, 
                        errorMessageIndex: lastUserMessageIndex,
                        errorType
                    }, chat.id);
                    
                    // Clean up on error
                    this.assistantFailed(chat.id, error);
                }
            }
            
            // Reset processing state for this chat (for all cases)
            chat.isProcessing = false;
            chat.shouldStopProcessing = false;
            this.updateSendButton(chatId);
        }
    }


    /**
     * Build messages for LLM context, stripping tool-related content
     * This creates a clean conversation flow without tool calls/results
     * Used for title generation and summarization to reduce costs
     * @param {Array} messages - Raw chat messages
     * @param {boolean} includeSystemPrompt - Whether to include the system prompt
     * @returns {Array} Clean messages array with only conversational content
     */
    buildConversationalMessages(messages, includeSystemPrompt = true) {
        const cleanMessages = [];
        
        // Add system prompt if requested and it exists
        if (includeSystemPrompt && messages.length > 0 && messages[0].role === 'system') {
            cleanMessages.push({ 
                role: 'system', 
                content: messages[0].content 
            });
        }
        
        let lastRole = null;
        
        // Process messages, skipping tool-related content
        for (let i = includeSystemPrompt && messages[0]?.role === 'system' ? 1 : 0; i < messages.length; i++) {
            const msg = messages[i];
            
            // Skip system roles and tool results
            if (['system-title', 'system-summary', 'title', 'summary', 'accounting', 'tool-results', 'error'].includes(msg.role)) {
                continue;
            }
            
            const msgRole = msg.role;
            
            if (msgRole === 'user') {
                // Check if we have consecutive user messages (which can happen after filtering)
                if (lastRole === 'user' && cleanMessages.length > 1) {
                    // Insert a placeholder assistant message to maintain alternation
                    cleanMessages.push({
                        role: 'assistant',
                        content: '[Tool interaction removed for summary]'
                    });
                }
                cleanMessages.push({ 
                    role: 'user', 
                    content: msg.content 
                });
                lastRole = 'user';
            } else if (msgRole === 'assistant') {
                // Only include assistant messages that DON'T have tool calls
                // Skip ALL messages with tool calls, even if they have content
                const hasToolCalls = this.extractToolsFromContent(msg.content).length > 0;
                if (!hasToolCalls) {
                    // This is a pure conversational response
                    if (msg.content) {
                        const cleanedContent = this.cleanContentForAPI(msg.content);
                        if (cleanedContent && cleanedContent.trim().length > 0) {
                            cleanMessages.push({ 
                                role: 'assistant', 
                                content: cleanedContent 
                            });
                            lastRole = 'assistant';
                        }
                    }
                } else {
                    // Assistant message with tool calls - mark that we had an assistant response
                    lastRole = 'assistant';
                }
            }
        }
        
        return cleanMessages;
    }

    buildMessagesForAPI(chat, freezeCache = false, mcpConnection = null) {
        // STRICT: Use the chat's MessageOptimizer instance
        if (!chat.messageOptimizer) {
            throw new Error('[buildMessagesForAPI] Chat missing messageOptimizer instance');
        }
        
        try {
            // Get MCP instructions if connection is provided
            const mcpInstructions = mcpConnection && mcpConnection.instructions ? mcpConnection.instructions : null;
            
            // Delegate to the chat's MessageOptimizer
            return chat.messageOptimizer.buildMessagesForAPI(chat, freezeCache, mcpInstructions);
        } catch (error) {
            console.error('[buildMessagesForAPI] MessageOptimizer failed:', error);
            this.showError(`Message optimization failed: ${error.message}`, chat.id);
            throw error;
        }
    }

    async resumeProcessing(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('Chat not found:', chatId);
            return;
        }

        // Clear any stop flags
        chat.shouldStopProcessing = false;
        chat.processingWasStoppedByUser = false;
        
        // Simply call sendMessage without adding a new user message
        // The last message in the chat should still be the user message that was interrupted
        await this.sendMessage(chatId, null, true); // Pass true to indicate this is a resume
    }

    async processMessageWithTools(chat, mcpConnection, provider, userMessage) {
        // Set processing state for this chat
        chat.isProcessing = true;
        this.updateChatSessions(); // Update sidebar to show activity indicator
        
        try {
            // Build conversation history using the new function
            const { messages, cacheControlIndex } = this.buildMessagesForAPI(chat, false, mcpConnection);
            
            // Safety check: Ensure we have at least one message
            if (messages.length === 0) {
                console.error('No messages to send to API - this should not happen');
                console.error('Chat messages:', chat.messages);
                console.error('Chat ID:', chat.id);
                console.error('User message parameter:', userMessage);
                
                // This can happen if the chat only contains system messages that are filtered out
                // This is an edge case that shouldn't happen in normal usage
                throw new Error('No valid conversation messages found. Please start a new conversation.');
            }
            
            // Get available tools
            const tools = Array.from(mcpConnection.tools.values());
            
            // let attempts = 0;
            // No limit on attempts - let the LLM decide when it's done
            
            // Create assistant group at the start of processing
            // We'll add all content from this conversation turn to this single group
            this.clearCurrentAssistantGroup(chat.id);
            
            while (true) {
            // attempts++;
            
            // Check if we should stop processing for this chat
            if (chat.shouldStopProcessing) {
                // Mark that processing was stopped in the chat object
                chat.processingWasStoppedByUser = true;
                // Throw an error to trigger the catch block in sendMessage
                const stopError = new Error('Processing stopped by user');
                stopError.isUserStop = true;
                throw stopError;
            }
            
            
            // SAFETY CHECK: Check iteration limit before continuing
            try {
                const currentIterations = this.safetyChecker.getIterationCount(chat.id);
                if (currentIterations >= SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS) {
                    throw new SafetyLimitError('ITERATIONS', SAFETY_LIMITS.ERRORS.TOO_MANY_ITERATIONS(currentIterations, SAFETY_LIMITS.MAX_CONSECUTIVE_TOOL_ITERATIONS));
                }
            } catch (error) {
                if (error instanceof SafetyLimitError) {
                    // Show safety error with no retry option
                    this.addMessage(chat.id, { 
                        role: 'error', 
                        content: error.message,
                        errorType: 'safety_limit',
                        isRetryable: false
                    });
                    this.processRenderEvent({ 
                        type: 'error-message', 
                        content: error.message, 
                        errorType: 'safety_limit'
                    }, chat.id);
                    return; // Stop processing completely
                }
                throw error; // Re-throw non-safety errors
            }
            
            // Send to LLM with current temperature
            const temperature = this.getCurrentTemperature(chat.id);
            // eslint-disable-next-line no-await-in-loop
            const response = await this.callAssistant({
                chatId: chat.id,
                provider,
                messages,
                tools,
                temperature,
                cacheControlIndex,
                context: 'Processing tools sequentially'
            });
            
            // Check if rate limit was handled automatically
            if (response._rateLimitHandled) {
                return { rateLimitHandled: true };
            }
            
            // Process the response
            // eslint-disable-next-line no-await-in-loop
            await this.processSingleLLMResponse(chat, mcpConnection, provider, messages, tools, cacheControlIndex, response);
            
            // Check if we should continue (if the response had tool calls)
            const lastMessage = messages[messages.length - 1];
            if (!lastMessage || lastMessage.role !== 'tool-results') {
                // No tool results, we're done
                break;
            }
            }
        } catch (error) {
            // Don't set error state for user stop
            if (!error.isUserStop) {
                // Set error state
                chat.hasError = true;
                chat.lastError = error.message;
            }
            throw error;
        } finally {
            // Note: Full cleanup is handled by the caller (sendMessage) via assistantConcluded()
            // We only update the sidebar here to remove activity indicator
            this.updateChatSessions();
        }
    }

    renderMessage(role, content, messageIndex, chatId) {
        if (!chatId) {
            console.error('renderMessage called without chatId');
            return;
        }
        
        // Extract text content from array if needed
        let messageContent = content;
        if (Array.isArray(messageContent)) {
            // Content is an array with tool calls and text blocks
            // Extract only the text blocks
            const textBlocks = messageContent.filter(block => block.type === 'text');
            messageContent = textBlocks.map(block => block.text || '').join('\n\n').trim();
        } else if (typeof messageContent !== 'string') {
            console.error('renderMessage: unexpected content type', { role, content: messageContent, messageIndex });
            messageContent = String(messageContent || '');
        }
        
        let messageDiv;
        
        if (role === 'assistant') {
            // For assistant messages, we create or use the current group
            const chat = this.chats.get(chatId);
            if (!chat) {
                console.error(`[renderMessage] Chat ${chatId} not found`);
                return;
            }
            
            if (!this.getCurrentAssistantGroup(chatId)) {
                // Create new assistant group
                const groupDiv = document.createElement('div');
                groupDiv.className = 'assistant-group';
                
                this.setCurrentAssistantGroup(chatId, groupDiv);
                // Use chat-specific messages container
                const container = this.getChatContainer(chatId);
                if (container && container._elements && container._elements.messages) {
                    container._elements.messages.appendChild(groupDiv);
                }
            }
            
            // Use the current group as our target
            messageDiv = this.getCurrentAssistantGroup(chatId);
        } else {
            // For non-assistant messages, create a regular message div
            messageDiv = document.createElement('div');
            messageDiv.className = `message ${role}`;
        }
        
        // Add redo button only for user and assistant messages (but not in sub-chats)
        const chat = this.chats.get(chatId);
        const isSubChat = chat && chat.isSubChat;
        
        if (!isSubChat && messageIndex !== undefined && (role === 'user' || role === 'assistant')) {
            const redoBtn = document.createElement('button');
            redoBtn.className = 'redo-button';
            redoBtn.textContent = 'Redo';
            redoBtn.onclick = () => this.redoFromMessage(messageIndex, chatId);
            messageDiv.style.position = 'relative';
            messageDiv.appendChild(redoBtn);
        }
        
        // Check if content has thinking tags
        const thinkingRegex = /<thinking>([\s\S]*?)<\/thinking>/g;
        const hasThinking = thinkingRegex.test(messageContent);
        
        // Make user messages editable on click
        if (role === 'user' && chatId) {
            messageDiv.classList.add('editable-message');
        }
        
        // Process content
        if (hasThinking && role === 'assistant') {
            // Reset regex for actual processing
            messageContent.match(/<thinking>([\s\S]*?)<\/thinking>/g);
            
            // Split content into parts
            const parts = [];
            let lastIndex = 0;
            let match;
            const regex = /<thinking>([\s\S]*?)<\/thinking>/g;
            
            while ((match = regex.exec(messageContent)) !== null) {
                // Add text before thinking
                if (match.index > lastIndex) {
                    parts.push({
                        type: 'text',
                        content: messageContent.substring(lastIndex, match.index).trim()
                    });
                }
                
                // Add thinking content
                parts.push({
                    type: 'thinking',
                    content: match[1].trim()
                });
                
                lastIndex = regex.lastIndex;
            }
            
            // Add remaining text
            if (lastIndex < messageContent.length) {
                const remaining = messageContent.substring(lastIndex).trim();
                if (remaining) {
                    parts.push({
                        type: 'text',
                        content: remaining
                    });
                }
            }
            
            // Render parts
            parts.forEach((part) => {
                if (part.type === 'text' && part.content) {
                    const textDiv = document.createElement('div');
                    textDiv.className = 'message-content';
                    // Use marked to render markdown
                    textDiv.innerHTML = marked.parse(part.content, {
                        breaks: true, gfm: true, sanitize: false
                    });
                    messageDiv.appendChild(textDiv);
                } else if (part.type === 'thinking') {
                    const thinkingDiv = document.createElement('div');
                    thinkingDiv.className = 'thinking-block';
                    
                    const thinkingHeader = document.createElement('div');
                    thinkingHeader.className = 'thinking-header';
                    thinkingHeader.innerHTML = `
                        <span class="thinking-toggle">▶</span>
                        <span class="thinking-label">💭 Assistant's reasoning</span>
                    `;
                    
                    const thinkingContent = document.createElement('div');
                    thinkingContent.className = 'thinking-content collapsed';
                    
                    const contentWrapper = document.createElement('div');
                    contentWrapper.className = 'message-content';
                    contentWrapper.innerHTML = marked.parse(part.content, {
                        breaks: true, gfm: true, sanitize: false
                    });
                    
                    thinkingContent.appendChild(contentWrapper);
                    
                    thinkingHeader.addEventListener('click', () => {
                        const isCollapsed = thinkingContent.classList.contains('collapsed');
                        thinkingContent.classList.toggle('collapsed');
                        const toggle = thinkingHeader.querySelector('.thinking-toggle');
                        if (toggle) {
                            toggle.textContent = isCollapsed ? '▼' : '▶';
                        }
                    });
                    
                    thinkingDiv.appendChild(thinkingHeader);
                    thinkingDiv.appendChild(thinkingContent);
                    messageDiv.appendChild(thinkingDiv);
                }
            });
        } else if (role === 'system-title' || role === 'system-summary') {
            // Handle system messages like tool blocks
            
            // Check if we need to create a new system block or add to existing one
            const container = this.getChatContainer(chatId);
            const messagesContainer = container && container._elements && container._elements.messages;
            let systemBlock = messagesContainer ? messagesContainer.querySelector('.system-block:last-child') : null;
            const isTitle = role === 'system-title';
            const blockType = isTitle ? 'title' : 'summary';
            
            if (!systemBlock || systemBlock.dataset.type !== blockType) {
                // For title generation, add "Generating chat title..." message before the collapsible
                if (isTitle) {
                    this.addSystemMessage('Generating chat title...', chatId, 'fas fa-edit');
                }
                
                // Create new system block
                systemBlock = document.createElement('div');
                systemBlock.className = 'tool-block system-block';
                systemBlock.dataset.type = blockType;
                
                const systemHeader = document.createElement('div');
                systemHeader.className = 'tool-header';
                systemHeader.style.cursor = 'pointer';
                
                const systemToggle = document.createElement('span');
                systemToggle.className = 'tool-toggle';
                systemToggle.textContent = '▶';
                
                const systemLabel = document.createElement('span');
                systemLabel.className = 'tool-label';
                systemLabel.textContent = isTitle ? 'Chat Title' : 'Chat Summary';
                
                systemHeader.appendChild(systemToggle);
                systemHeader.appendChild(systemLabel);
                
                // Add delete button for summary blocks
                if (!isTitle) {
                    const toolInfo = document.createElement('span');
                    toolInfo.className = 'tool-info';
                    
                    const deleteBtn = document.createElement('button');
                    deleteBtn.className = 'btn btn-danger btn-small';
                    deleteBtn.innerHTML = '<i class="fas fa-trash"></i> Delete';
                    deleteBtn.title = 'Delete summary and replace with accounting';
                    deleteBtn.style.marginLeft = '10px';
                    deleteBtn.onclick = (e) => {
                        e.stopPropagation(); // Prevent header toggle
                        const summaryChat = this.chats.get(chatId);
                        if (summaryChat) {
                            // Find the most recent summary message
                            for (let i = summaryChat.messages.length - 1; i >= 0; i--) {
                                if (summaryChat.messages[i]?.role === 'summary') {
                                    this.showConfirmDialog(
                                        'Delete Summary',
                                        'Delete this summary and replace with accounting record?',
                                        'Delete',
                                        'Cancel',
                                        true // danger style
                                    ).then(confirmed => {
                                        if (confirmed) {
                                            this.deleteSummaryMessages(i, chatId);
                                        }
                                    });
                                    break;
                                }
                            }
                        }
                    };
                    
                    toolInfo.appendChild(deleteBtn);
                    systemHeader.appendChild(toolInfo);
                }
                
                const systemContent = document.createElement('div');
                systemContent.className = 'tool-content collapsed';
                
                // Add click handler to toggle
                systemHeader.addEventListener('click', () => {
                    const isCollapsed = systemContent.classList.contains('collapsed');
                    systemContent.classList.toggle('collapsed');
                    systemToggle.textContent = isCollapsed ? '▶' : '▼';
                });
                
                systemBlock.appendChild(systemHeader);
                systemBlock.appendChild(systemContent);
                // Use chat-specific messages container
                const targetContainer = this.getChatContainer(chatId);
                if (targetContainer && targetContainer._elements && targetContainer._elements.messages) {
                    targetContainer._elements.messages.appendChild(systemBlock);
                }
            }
            
            // Add request section
            const systemContent = systemBlock.querySelector('.tool-content');
            
            const requestSection = document.createElement('div');
            requestSection.className = 'tool-request-section';
            
            const requestControls = document.createElement('div');
            requestControls.className = 'tool-section-controls';
            
            const requestLabel = document.createElement('span');
            requestLabel.className = 'tool-section-label';
            requestLabel.textContent = 'REQUEST';
            
            requestControls.appendChild(requestLabel);
            requestSection.appendChild(requestControls);
            
            const requestContent = document.createElement('pre');
            requestContent.textContent = content;
            requestSection.appendChild(requestContent);
            
            systemContent.appendChild(requestSection);
            
            return; // Don't append messageDiv to chat
        } else if (role === 'title' || role === 'summary') {
            // Handle system responses - add to existing system block
            
            const isTitle = role === 'title';
            const blockType = isTitle ? 'title' : 'summary';
            
            // Find the LAST system block of this type (most recent) in chat-specific container
            const container = this.getChatContainer(chatId);
            const chatMessages = container && container._elements && container._elements.messages;
            if (!chatMessages) {return;}
            
            const systemBlocks = chatMessages.querySelectorAll(`.system-block[data-type="${blockType}"]`);
            const systemBlock = systemBlocks[systemBlocks.length - 1];
            
            
            if (systemBlock && !systemBlock.querySelector('.tool-response-section')) {
                // Only add response if there isn't one already
                const systemContent = systemBlock.querySelector('.tool-content');
                
                // Add separator
                const separator = document.createElement('div');
                separator.className = 'tool-separator';
                systemContent.appendChild(separator);
                
                // Add response section
                const responseSection = document.createElement('div');
                responseSection.className = 'tool-response-section';
                
                const responseControls = document.createElement('div');
                responseControls.className = 'tool-section-controls';
                
                const responseLabel = document.createElement('span');
                responseLabel.className = 'tool-section-label';
                responseLabel.textContent = 'RESPONSE';
                
                // Add copy button for the response
                const copyBtn = document.createElement('button');
                copyBtn.className = 'tool-section-copy';
                copyBtn.innerHTML = '<i class="fas fa-copy"></i>';
                copyBtn.onclick = () => {
                    this.copyToClipboard(content, copyBtn).catch(error => {
                        console.error('Failed to copy to clipboard:', error);
                    });
                };
                
                responseControls.appendChild(responseLabel);
                responseControls.appendChild(copyBtn);
                responseSection.appendChild(responseControls);
                
                // For summary, use markdown rendering
                if (role === 'summary') {
                    const responseContent = document.createElement('div');
                    responseContent.className = 'message-content';
                    responseContent.innerHTML = marked.parse(messageContent, {
                        breaks: true, gfm: true, sanitize: false
                    });
                    responseSection.appendChild(responseContent);
                } else {
                    // For title, use plain text
                    const responseContent = document.createElement('pre');
                    responseContent.textContent = messageContent;
                    responseSection.appendChild(responseContent);
                }
                
                systemContent.appendChild(responseSection);
                
                return; // Don't append messageDiv to chat
            } 
                console.warn(`No ${blockType} block found for response:`, messageContent);
            
        } else {
            // Regular message without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            
            if (role === 'assistant' || role === 'user') {
                // Use marked to render markdown for assistant and user messages
                // Configure marked to preserve line breaks and handle whitespace properly
                contentDiv.innerHTML = marked.parse(messageContent, {
                    breaks: true, // Convert line breaks to <br>
                    gfm: true, // GitHub Flavored Markdown
                    sanitize: false // Allow HTML (needed for thinking tags)
                });
            } else {
                // Other messages (system, error) as plain text
                contentDiv.textContent = content;
            }
            
            messageDiv.appendChild(contentDiv);
        }
        
        // Only append non-assistant messages to chat (assistant groups are already appended)
        if (role !== 'assistant') {
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                container._elements.messages.appendChild(messageDiv);
            }
        }
        
        // Add edit trigger after element is in DOM
        if (role === 'user' && chatId && content) {
            const contentDiv = messageDiv.querySelector('.message-content');
            if (contentDiv) {
                this.addEditTrigger(contentDiv, content, 'user', chatId);
            }
        }
        
        // Scroll to bottom after DOM update
        // For user messages, force scroll since user just sent it
        requestAnimationFrame(() => {
            if (role === 'user') {
                this.scrollToBottom(chatId, true); // Force scroll for user messages
            } else {
                this.scrollToBottom(chatId); // Normal scroll behavior for other messages
            }
            this.moveSpinnerToBottom(chatId);
        });
    }

    addSystemMessage(content, chatId, icon = null) {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message system';
        
        if (icon) {
            messageDiv.innerHTML = `<i class="${icon}"></i> ${content}`;
        } else {
            messageDiv.textContent = content;
        }
        
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.messages) {
            container._elements.messages.appendChild(messageDiv);
        }
        
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
    }
    
    addAccountingNode(data, chatId) {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message accounting';
        
        const formatNumber = (num) => num ? num.toLocaleString() : '0';
        const tokens = data.cumulativeTokens || {};
        
        // Calculate price - always show it, even if zero
        let price = 0;
        if (data.model) {
            const usage = {
                promptTokens: tokens.inputTokens || 0,
                completionTokens: tokens.outputTokens || 0,
                cacheReadInputTokens: tokens.cacheReadTokens || 0,
                cacheCreationInputTokens: tokens.cacheCreationTokens || 0
            };
            price = this.calculateMessagePrice(data.model, usage) || 0;
        }
        
        const priceHtml = `
            <span class="accounting-token-item" data-tooltip="Cost">
                <i class="fas fa-dollar-sign"></i>
                <span>${price.toFixed(4)}</span>
            </span>`;
        
        // Add model display
        const modelHtml = data.model ? `
            <span class="accounting-token-item" data-tooltip="Model used">
                <i class="fas fa-robot"></i>
                <span>${ChatConfig.getModelDisplayName(data.model)}</span>
            </span>` : '';
        
        messageDiv.innerHTML = `
            <div class="accounting-line">
                <div class="accounting-content">
                    <span class="accounting-icon"><i class="fas fa-coins"></i></span>
                    <div class="accounting-tokens">
                        ${modelHtml}
                        <span class="accounting-token-item" data-tooltip="Input tokens">
                            <i class="fas fa-download"></i>
                            <span>${formatNumber(tokens.inputTokens || 0)}</span>
                        </span>
                        <span class="accounting-token-item" data-tooltip="Cached tokens (90% discount)">
                            <i class="fas fa-memory"></i>
                            <span>${formatNumber(tokens.cacheReadTokens || 0)}</span>
                        </span>
                        <span class="accounting-token-item" data-tooltip="Cache creation tokens (25% surcharge)">
                            <i class="fas fa-save"></i>
                            <span>${formatNumber(tokens.cacheCreationTokens || 0)}</span>
                        </span>
                        <span class="accounting-token-item" data-tooltip="Output tokens">
                            <i class="fas fa-upload"></i>
                            <span>${formatNumber(tokens.outputTokens || 0)}</span>
                        </span>
                        ${priceHtml}
                    </div>
                    <span class="accounting-reason">${data.reason || 'Token checkpoint'}</span>
                </div>
            </div>
        `;
        
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.messages) {
            container._elements.messages.appendChild(messageDiv);
        }
        
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
    }
    
    createAccountingNodes(reason, messagesToDiscard, chatId) {
        // Calculate tokens ONLY from messages that will be discarded
        const chat = this.chats.get(chatId);
        if (!chat) {return [];}
        
        // Find messages that will be discarded (from the end)
        const startIndex = Math.max(0, chat.messages.length - messagesToDiscard);
        const discardedMessages = chat.messages.slice(startIndex);
        
        // Group discarded tokens by model
        const tokensByModel = new Map();
        
        for (const message of discardedMessages) {
            if (message.usage && message.model) {
                const model = message.model;
                if (!tokensByModel.has(model)) {
                    tokensByModel.set(model, {
                        inputTokens: 0,
                        outputTokens: 0,
                        cacheReadTokens: 0,
                        cacheCreationTokens: 0,
                        messageCount: 0
                    });
                }
                
                const tokens = tokensByModel.get(model);
                tokens.inputTokens += message.usage.promptTokens || 0;
                tokens.outputTokens += message.usage.completionTokens || 0;
                tokens.cacheCreationTokens += message.usage.cacheCreationInputTokens || 0;
                tokens.cacheReadTokens += message.usage.cacheReadInputTokens || 0;
                tokens.messageCount++;
            }
        }
        
        // Create separate accounting nodes for each model
        const accountingNodes = [];
        for (const [model, tokens] of tokensByModel) {
            accountingNodes.push({
                role: 'accounting',
                timestamp: new Date().toISOString(),
                model,
                cumulativeTokens: tokens,
                reason,
                discardedMessages: tokens.messageCount
            });
        }
        
        return accountingNodes;
    }
    
    displaySystemPrompt(prompt, chatId) {
        if (!chatId) {
            console.error('[displaySystemPrompt] Called without chatId');
            return;
        }
        const targetChatId = chatId;
        if (!targetChatId) {
            console.error('displaySystemPrompt: No chat ID provided');
            return;
        }
        
        const container = this.getChatContainer(targetChatId);
        if (!container) {
            console.error('displaySystemPrompt: No container found for chat', targetChatId);
            return;
        }
        
        if (!container._elements) {
            console.error('displaySystemPrompt: Container has no _elements', container);
            return;
        }
        
        const chatMessages = container._elements.messages;
        if (!chatMessages) {
            console.error('displaySystemPrompt: No messages element found', container._elements);
            return;
        }
        
        const promptDiv = document.createElement('div');
        promptDiv.className = 'system-prompt-display';
        
        // Create collapsible header (similar to thinking blocks)
        const headerDiv = document.createElement('div');
        headerDiv.className = 'system-prompt-header';
        headerDiv.style.cursor = 'pointer';
        headerDiv.style.userSelect = 'none';
        
        const toggle = document.createElement('span');
        toggle.className = 'system-prompt-toggle';
        toggle.textContent = '▶';
        toggle.style.fontSize = '12px';
        toggle.style.fontFamily = 'monospace';
        toggle.style.marginRight = '8px';
        
        const label = document.createElement('span');
        label.className = 'system-prompt-label';
        label.textContent = 'System Prompt';
        
        headerDiv.appendChild(toggle);
        headerDiv.appendChild(label);
        
        // Create collapsible content
        const contentDiv = document.createElement('div');
        contentDiv.className = 'system-prompt-content collapsed';
        contentDiv.textContent = prompt;
        
        // Toggle functionality
        let isCollapsed = true;
        headerDiv.onclick = () => {
            isCollapsed = !isCollapsed;
            if (isCollapsed) {
                contentDiv.classList.add('collapsed');
                toggle.textContent = '▶';
            } else {
                contentDiv.classList.remove('collapsed');
                toggle.textContent = '▼';
            }
        };
        
        promptDiv.appendChild(headerDiv);
        promptDiv.appendChild(contentDiv);
        
        chatMessages.appendChild(promptDiv);
        
        // Add edit trigger after element is in DOM (only when expanded)
        const originalAddEditTrigger = () => this.addEditTrigger(contentDiv, prompt, 'system', targetChatId);
        
        // Override the header click to also handle edit trigger
        const originalClick = headerDiv.onclick;
        headerDiv.onclick = (event) => {
            if (originalClick) originalClick.call(headerDiv, event);
            // Add edit trigger when expanding for the first time
            if (!isCollapsed && !contentDiv.dataset.editTriggerAdded) {
                setTimeout(originalAddEditTrigger, 300); // Wait for animation
                contentDiv.dataset.editTriggerAdded = 'true';
            }
        };
    }
    
    addEditTrigger(contentDiv, originalContent, type, chatId) {
        const wrapper = contentDiv.parentElement;
        if (!wrapper) {
            console.warn('Cannot add edit trigger - element not yet in DOM');
            return;
        }
        wrapper.style.position = 'relative';
        
        // Create edit balloon
        const editBalloon = document.createElement('div');
        editBalloon.className = 'edit-balloon';
        editBalloon.innerHTML = 'Edit';
        // CSS handles initial state and transitions
        wrapper.appendChild(editBalloon);
        
        // CSS handles hover visibility via transitions, no JavaScript needed
        
        // Handle click on balloon
        editBalloon.onclick = () => {
            if (type === 'user') {
                this.editUserMessage(contentDiv, originalContent, chatId);
            } else if (type === 'system') {
                this.editSystemPromptInline(contentDiv, originalContent, chatId);
            }
            editBalloon.style.display = 'none';
        };
    }
    
    editSystemPromptInline(contentDiv, originalPrompt, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Prevent multiple edit sessions
        if (contentDiv.classList.contains('editing')) {return;}
        
        // Make content editable
        contentDiv.contentEditable = true;
        contentDiv.classList.add('editing');
        
        // Just focus, don't select all - let user position cursor
        contentDiv.focus();
        
        // Create floating save/cancel buttons
        const buttonsDiv = document.createElement('div');
        buttonsDiv.className = 'edit-actions-floating';
        buttonsDiv.innerHTML = `
            <button class="btn btn-small btn-primary" data-tooltip="Save & Restart Chat (Enter)"><i class="fas fa-check"></i></button>
            <button class="btn btn-small btn-secondary" data-tooltip="Cancel (Escape)"><i class="fas fa-times"></i></button>
        `;
        contentDiv.parentElement.appendChild(buttonsDiv);
        
        // Position buttons
        const rect = contentDiv.getBoundingClientRect();
        buttonsDiv.style.top = rect.bottom - contentDiv.parentElement.getBoundingClientRect().top + 4 + 'px';
        
        // Handle cancel
        const cancel = () => {
            contentDiv.contentEditable = false;
            contentDiv.classList.remove('editing');
            contentDiv.textContent = originalPrompt;
            buttonsDiv.remove();
            // Restore the edit trigger
            this.addEditTrigger(contentDiv, originalPrompt, 'system', chatId);
        };
        
        // Handle save
        const save = () => {
            const newPrompt = contentDiv.textContent.trim();
            if (!newPrompt) {
                this.showError('System prompt cannot be empty', chatId);
                return;
            }
            
            if (newPrompt === originalPrompt) {
                cancel();
                return;
            }
            
            // Update the chat's system prompt
            chat.systemPrompt = newPrompt;
            
            // Clear messages and reset the conversation
            chat.messages = [];
            chat.updatedAt = new Date().toISOString();
            
            // Save the new prompt as the last used one
            this.lastSystemPrompt = newPrompt;
            localStorage.setItem('lastSystemPrompt', newPrompt);
            
            // Clear token usage history for this chat
            this.tokenUsageHistory.set(chatId, {
                requests: [],
                model: ChatConfig.getChatModelString(chat)
            });
            
            // Save settings
            this.saveSettings();
            
            // Reload the chat (force render to refresh UI)
            this.loadChat(chatId, true);
            
            // Show notification
            this.addSystemMessage('System prompt updated. Conversation has been reset.', chatId);
        };
        
        buttonsDiv.querySelector('.btn-primary').onclick = save;
        buttonsDiv.querySelector('.btn-secondary').onclick = cancel;
        
        // Handle keyboard shortcuts
        contentDiv.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                save();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancel();
            }
        });
        
        // Handle click outside
        const clickOutside = (e) => {
            if (!contentDiv.contains(e.target) && !buttonsDiv.contains(e.target)) {
                cancel();
                document.removeEventListener('click', clickOutside);
            }
        };
        setTimeout(() => document.addEventListener('click', clickOutside), 0);
    }
    
    
    
    // Simple function to append metrics to the chat DOM
    // This is the ONLY function that should be used for adding metrics headers
    appendMetricsToChat(usage, responseTime, model, chatId, messageType = 'assistant') {
        if (!chatId) {
            console.error('appendMetricsToChat called without chatId');
            return;
        }
        
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements) {return;}
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error('appendMetricsToChat: chat not found for chatId', chatId);
            return;
        }
        
        const chatMessages = container._elements.messages;
        
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        
        const formatNumber = (num) => num.toLocaleString();
        let metricsHtml = '';
        
        // 1. MODEL
        if (model) {
            const modelDisplay = ChatConfig.getModelDisplayName(model);
            metricsHtml += `<span class="metric-item" data-tooltip="Model used"><i class="fas fa-robot"></i> ${modelDisplay}</span>`;
        }
        
        // 2. TIME
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item" data-tooltip="Response time"><i class="fas fa-clock"></i> ${timeSeconds}s</span>`;
        }
        
        // Add token usage if available
        if (usage) {
            // 3. INPUT
            metricsHtml += `<span class="metric-item" data-tooltip="Input tokens"><i class="fas fa-download"></i> ${formatNumber(usage.promptTokens || 0)}</span>`;
            
            // 4. CACHE R (always show, even if 0)
            metricsHtml += `<span class="metric-item" data-tooltip="Cached tokens read (90% discount)"><i class="fas fa-memory"></i> ${formatNumber(usage.cacheReadInputTokens || 0)}</span>`;
            
            // 5. CACHE W (always show, even if 0)
            metricsHtml += `<span class="metric-item" data-tooltip="Cache tokens written (25% surcharge)"><i class="fas fa-save"></i> ${formatNumber(usage.cacheCreationInputTokens || 0)}</span>`;
            
            // 6. OUTPUT
            metricsHtml += `<span class="metric-item" data-tooltip="Output tokens"><i class="fas fa-upload"></i> ${formatNumber(usage.completionTokens || 0)}</span>`;
            
            // 7. REASONING (only show if present)
            if (usage.reasoningTokens) {
                metricsHtml += `<span class="metric-item" data-tooltip="Reasoning tokens (o3/o1 models)"><i class="fas fa-brain"></i> ${formatNumber(usage.reasoningTokens)}</span>`;
            }
            
            // 8. TOTAL
            const totalTokens = (usage.promptTokens || 0) + 
                               (usage.cacheReadInputTokens || 0) + 
                               (usage.cacheCreationInputTokens || 0) + 
                               (usage.completionTokens || 0);
            metricsHtml += `<span class="metric-item" data-tooltip="Total tokens"><i class="fas fa-chart-bar"></i> ${formatNumber(totalTokens)}</span>`;
            
            // 9. DELTA - calculate based on message type
            let deltaTokens;
            
            if (messageType === 'title') {
                // Titles don't affect context window
                deltaTokens = 0;
                // Don't update currentContextWindow
            } else if (messageType === 'summary') {
                // Summaries reset context to just their output tokens
                deltaTokens = (usage.completionTokens || 0) - (chat.currentContextWindow || 0);
                // Reset context window to just the summary's output
                chat.currentContextWindow = usage.completionTokens || 0;
            } else {
                // Regular assistant messages
                deltaTokens = totalTokens - (chat.currentContextWindow || 0);
                // Update the running total
                chat.currentContextWindow = totalTokens;
            }
            
            // Always show delta, even if zero
            const deltaSign = deltaTokens > 0 ? '+' : '';
            metricsHtml += `<span class="metric-item" data-tooltip="Change from previous">Δ ${deltaSign}${formatNumber(deltaTokens)}</span>`;
            
            // 10. PRICE
            if (model) {
                const price = this.calculateMessagePrice(model, usage) || 0;
                metricsHtml += `<span class="metric-item" data-tooltip="Cost"><i class="fas fa-dollar-sign"></i> ${price.toFixed(4)}</span>`;
            }
        }
        
        metricsFooter.innerHTML = metricsHtml;
        
        // Simply append to the chat messages container
        chatMessages.appendChild(metricsFooter);
    }
    
    
    
    /**
     * Extracts content from a contentEditable div while preserving formatting
     * Converts HTML to markdown format to preserve visual formatting
     */
    getEditableContent(contentDiv) {
        // CRITICAL: Add error checking for undefined contentDiv
        if (!contentDiv) {
            console.error('[getEditableContent] ERROR: contentDiv is undefined or null');
            return '';
        }
        
        // Get the HTML content
        let htmlContent = contentDiv.innerHTML || '';
        
        // Convert HTML tables to markdown tables before other processing
        htmlContent = this.convertHtmlTablesToMarkdown(htmlContent);
        
        // Convert HTML to markdown-like format to preserve formatting
        return htmlContent
            // Convert headers (h1-h6)
            .replace(/<h1[^>]*>(.*?)<\/h1>/gi, '\n# $1\n')
            .replace(/<h2[^>]*>(.*?)<\/h2>/gi, '\n## $1\n')
            .replace(/<h3[^>]*>(.*?)<\/h3>/gi, '\n### $1\n')
            .replace(/<h4[^>]*>(.*?)<\/h4>/gi, '\n#### $1\n')
            .replace(/<h5[^>]*>(.*?)<\/h5>/gi, '\n##### $1\n')
            .replace(/<h6[^>]*>(.*?)<\/h6>/gi, '\n###### $1\n')
            
            // Convert bold and strong
            .replace(/<(b|strong)[^>]*>(.*?)<\/(b|strong)>/gi, '**$2**')
            
            // Convert italic and emphasis
            .replace(/<(i|em)[^>]*>(.*?)<\/(i|em)>/gi, '*$2*')
            
            // Convert underline to emphasis (markdown doesn't have underline)
            .replace(/<u[^>]*>(.*?)<\/u>/gi, '*$1*')
            
            // Convert strikethrough
            .replace(/<(s|strike|del)[^>]*>(.*?)<\/(s|strike|del)>/gi, '~~$2~~')
            
            // Convert code blocks
            .replace(/<pre[^>]*><code[^>]*>(.*?)<\/code><\/pre>/gi, '\n```\n$1\n```\n')
            
            // Convert inline code
            .replace(/<code[^>]*>(.*?)<\/code>/gi, '`$1`')
            
            // Convert ordered lists
            .replace(/<ol[^>]*>/gi, '\n')
            .replace(/<\/ol>/gi, '\n')
            
            // Convert unordered lists
            .replace(/<ul[^>]*>/gi, '\n')
            .replace(/<\/ul>/gi, '\n')
            
            // Convert list items
            .replace(/<li[^>]*>(.*?)<\/li>/gi, (match, content) => {
                // Check if it's within an ordered list by looking at context
                // For now, use bullet points for all lists
                return '- ' + content.trim() + '\n';
            })
            
            // Convert blockquotes
            .replace(/<blockquote[^>]*>(.*?)<\/blockquote>/gi, '\n> $1\n')
            
            // Convert horizontal rules
            .replace(/<hr[^>]*>/gi, '\n---\n')
            
            // Convert line breaks
            .replace(/<br\s*\/?>/gi, '\n')
            
            // Convert paragraphs
            .replace(/<p[^>]*>/gi, '\n')
            .replace(/<\/p>/gi, '\n')
            
            // Convert divs (contentEditable creates these)
            .replace(/<div[^>]*>/gi, '\n')
            .replace(/<\/div>/gi, '')
            
            // Convert links
            .replace(/<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)<\/a>/gi, '[$2]($1)')
            
            // Replace non-breaking spaces
            .replace(/&nbsp;/gi, ' ')
            
            // Remove any remaining HTML tags
            .replace(/<[^>]*>/g, '')
            
            // Decode HTML entities
            .replace(/&lt;/g, '<')
            .replace(/&gt;/g, '>')
            .replace(/&amp;/g, '&')
            .replace(/&quot;/g, '"')
            .replace(/&#39;/g, "'")
            .replace(/&apos;/g, "'")
            .replace(/&#(\d+);/g, (match, dec) => String.fromCharCode(dec))
            .replace(/&#x([a-f0-9]+);/gi, (match, hex) => String.fromCharCode(parseInt(hex, 16)))
            
            // Clean up excessive whitespace
            .replace(/\n\s*\n\s*\n/g, '\n\n')
            .replace(/[ \t]+$/gm, '')
            .trim();
    }
    
    /**
     * Converts HTML tables to markdown tables
     */
    convertHtmlTablesToMarkdown(html) {
        // Find all tables and convert them
        return html.replace(/<table[^>]*>([\s\S]*?)<\/table>/gi, (match, tableContent) => {
            try {
                // Parse the table content
                const rows = [];
                const tableRows = tableContent.match(/<tr[^>]*>([\s\S]*?)<\/tr>/gi) || [];
                
                tableRows.forEach((tr, _rowIndex) => {
                    const cells = [];
                    // Match both th and td tags
                    const cellMatches = tr.match(/<(th|td)[^>]*>([\s\S]*?)<\/(th|td)>/gi) || [];
                    
                    cellMatches.forEach(cell => {
                        // Extract cell content
                        const cellContent = cell
                            .replace(/<(th|td)[^>]*>/gi, '')
                            .replace(/<\/(th|td)>/gi, '')
                            .replace(/<br\s*\/?>/gi, ' ')
                            .replace(/<[^>]*>/g, '')
                            .replace(/&nbsp;/gi, ' ')
                            .trim();
                        cells.push(cellContent);
                    });
                    
                    if (cells.length > 0) {
                        rows.push(cells);
                    }
                });
                
                if (rows.length === 0) {
                    return '';
                }
                
                // Build markdown table
                let markdownTable = '\n\n';
                
                // Add header row
                markdownTable += '| ' + rows[0].join(' | ') + ' |\n';
                
                // Add separator row
                markdownTable += '|' + rows[0].map(() => ' --- ').join('|') + '|\n';
                
                // Add data rows
                for (let i = 1; i < rows.length; i++) {
                    // Ensure the row has the same number of columns as the header
                    while (rows[i].length < rows[0].length) {
                        rows[i].push('');
                    }
                    markdownTable += '| ' + rows[i].join(' | ') + ' |\n';
                }
                
                markdownTable += '\n';
                
                return markdownTable;
            } catch (error) {
                console.error('[convertHtmlTablesToMarkdown] Error converting table:', error);
                // Return the original table HTML if conversion fails
                return match;
            }
        });
    }
    
    /**
     * Comprehensive HTML to Markdown converter
     * Handles full HTML documents from clipboard
     */
    convertHtmlToMarkdown(html) {
        let result = html;
        
        // Remove any style tags and their content
        result = result.replace(/<style[^>]*>[\s\S]*?<\/style>/gi, '');
        
        // Remove any script tags and their content
        result = result.replace(/<script[^>]*>[\s\S]*?<\/script>/gi, '');
        
        // Remove HTML comments
        result = result.replace(/<!--[\s\S]*?-->/g, '');
        
        // Extract body content if it's a full HTML document
        const bodyMatch = result.match(/<body[^>]*>([\s\S]*?)<\/body>/i);
        if (bodyMatch) {
            result = bodyMatch[1];
        }
        
        // Convert tables first (before other processing)
        result = this.convertHtmlTablesToMarkdown(result);
        
        // Process nested elements properly by converting from innermost to outermost
        // Convert links with proper text extraction
        result = result.replace(/<a[^>]+href=["']([^"']+)["'][^>]*>([\s\S]*?)<\/a>/gi, (match, url, text) => {
            // Clean up the link text
            const cleanText = text.replace(/<[^>]*>/g, '').trim();
            return `[${cleanText}](${url})`;
        });
        
        // Convert images
        result = result.replace(/<img[^>]+src=["']([^"']+)["'](?:[^>]+alt=["']([^"']+)["'])?[^>]*>/gi, (match, src, alt) => {
            return alt ? `![${alt}](${src})` : `![](${src})`;
        });
        
        // Convert headers
        result = result.replace(/<h1[^>]*>([\s\S]*?)<\/h1>/gi, '\n# $1\n');
        result = result.replace(/<h2[^>]*>([\s\S]*?)<\/h2>/gi, '\n## $1\n');
        result = result.replace(/<h3[^>]*>([\s\S]*?)<\/h3>/gi, '\n### $1\n');
        result = result.replace(/<h4[^>]*>([\s\S]*?)<\/h4>/gi, '\n#### $1\n');
        result = result.replace(/<h5[^>]*>([\s\S]*?)<\/h5>/gi, '\n##### $1\n');
        result = result.replace(/<h6[^>]*>([\s\S]*?)<\/h6>/gi, '\n###### $1\n');
        
        // Convert lists - handle nested lists
        // Process lists from innermost to outermost
        let previousHtml;
        do {
            previousHtml = result;
            
            // Convert unordered lists
            result = result.replace(/<ul[^>]*>([\s\S]*?)<\/ul>/gi, (match, content) => {
                const items = content.match(/<li[^>]*>([\s\S]*?)<\/li>/gi) || [];
                const converted = items.map(item => {
                    const itemContent = item.replace(/<li[^>]*>/i, '').replace(/<\/li>/i, '').trim();
                    return '- ' + itemContent;
                }).join('\n');
                return '\n' + converted + '\n';
            });
            
            // Convert ordered lists  
            result = result.replace(/<ol[^>]*>([\s\S]*?)<\/ol>/gi, (match, content) => {
                const items = content.match(/<li[^>]*>([\s\S]*?)<\/li>/gi) || [];
                const converted = items.map((item, index) => {
                    const itemContent = item.replace(/<li[^>]*>/i, '').replace(/<\/li>/i, '').trim();
                    return `${index + 1}. ${itemContent}`;
                }).join('\n');
                return '\n' + converted + '\n';
            });
        } while (result !== previousHtml);
        
        // Convert code blocks
        result = result.replace(/<pre[^>]*><code[^>]*>([\s\S]*?)<\/code><\/pre>/gi, '\n```\n$1\n```\n');
        result = result.replace(/<pre[^>]*>([\s\S]*?)<\/pre>/gi, '\n```\n$1\n```\n');
        
        // Convert inline code
        result = result.replace(/<code[^>]*>([\s\S]*?)<\/code>/gi, '`$1`');
        
        // Convert formatting
        result = result.replace(/<(b|strong)[^>]*>([\s\S]*?)<\/(b|strong)>/gi, '**$2**');
        result = result.replace(/<(i|em)[^>]*>([\s\S]*?)<\/(i|em)>/gi, '*$2*');
        result = result.replace(/<u[^>]*>([\s\S]*?)<\/u>/gi, '*$1*');
        result = result.replace(/<(s|strike|del)[^>]*>([\s\S]*?)<\/(s|strike|del)>/gi, '~~$2~~');
        
        // Convert blockquotes
        result = result.replace(/<blockquote[^>]*>([\s\S]*?)<\/blockquote>/gi, '\n> $1\n');
        
        // Convert horizontal rules
        result = result.replace(/<hr[^>]*>/gi, '\n---\n');
        
        // Convert line breaks
        result = result.replace(/<br\s*\/?>/gi, '\n');
        
        // Convert paragraphs
        result = result.replace(/<p[^>]*>([\s\S]*?)<\/p>/gi, '\n$1\n');
        
        // Convert divs
        result = result.replace(/<div[^>]*>([\s\S]*?)<\/div>/gi, '\n$1\n');
        
        // Remove any remaining HTML tags
        result = result.replace(/<[^>]*>/g, '');
        
        // Decode HTML entities
        result = result.replace(/&nbsp;/gi, ' ');
        result = result.replace(/&lt;/g, '<');
        result = result.replace(/&gt;/g, '>');
        result = result.replace(/&amp;/g, '&');
        result = result.replace(/&quot;/g, '"');
        result = result.replace(/&#39;/g, "'");
        result = result.replace(/&apos;/g, "'");
        result = result.replace(/&#(\d+);/g, (match, dec) => String.fromCharCode(dec));
        result = result.replace(/&#x([a-f0-9]+);/gi, (match, hex) => String.fromCharCode(parseInt(hex, 16)));
        
        // Clean up excessive whitespace
        result = result.replace(/\n\s*\n\s*\n/g, '\n\n');
        result = result.replace(/[ \t]+$/gm, '');
        result = result.trim();
        
        return result;
    }

    editUserMessage(contentDiv, originalContent, chatId) {
        if (!chatId) {return;}
        
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Prevent multiple edit sessions
        if (contentDiv.classList.contains('editing')) {return;}
        
        // Find the message index by searching through all user message divs
        let messageIndex = -1;
        const container = this.getChatContainer(chatId);
        const messagesContainer = container && container._elements && container._elements.messages;
        const allUserMessages = messagesContainer ? messagesContainer.querySelectorAll('.message.user .message-content') : [];
        
        for (let i = 0; i < allUserMessages.length; i++) {
            if (allUserMessages[i] === contentDiv) {
                // Count user messages up to this point
                let userMessageCount = 0;
                for (let j = 0; j < chat.messages.length; j++) {
                    if (chat.messages[j].role === 'user') {
                        if (userMessageCount === i) {
                            messageIndex = j;
                            break;
                        }
                        userMessageCount++;
                    }
                }
                break;
            }
        }
        
        if (messageIndex === -1) {
            this.showError('Message not found in chat history', chatId);
            return;
        }
        
        // Get the original raw content from chat history
        const originalMessage = chat.messages[messageIndex];
        const originalRawContent = originalMessage ? originalMessage.content : '';
        
        // Make content editable and populate with original text for editing
        contentDiv.contentEditable = true;
        contentDiv.classList.add('editing');
        
        // CRITICAL FIX: Convert newlines to <br> tags so they display properly in contentEditable
        const editableContent = originalRawContent
            .replace(/&/g, '&amp;')    // Escape ampersands first
            .replace(/</g, '&lt;')     // Escape less-than
            .replace(/>/g, '&gt;')     // Escape greater-than
            .replace(/\n/g, '<br>');   // Convert newlines to <br> tags
        
        contentDiv.innerHTML = editableContent;
        
        // Just focus, don't select all - let user position cursor
        contentDiv.focus();
        
        // Create floating save/cancel buttons
        const buttonsDiv = document.createElement('div');
        buttonsDiv.className = 'edit-actions-floating';
        buttonsDiv.innerHTML = `
            <button class="btn btn-small btn-primary" data-tooltip="Save & Resend (Enter)"><i class="fas fa-check"></i></button>
            <button class="btn btn-small btn-secondary" data-tooltip="Cancel (Escape)"><i class="fas fa-times"></i></button>
        `;
        contentDiv.parentElement.appendChild(buttonsDiv);
        
        // Position buttons
        const rect = contentDiv.getBoundingClientRect();
        buttonsDiv.style.top = rect.bottom - contentDiv.parentElement.getBoundingClientRect().top + 4 + 'px';
        
        // Declare variables first to avoid circular dependency
        // eslint-disable-next-line prefer-const
        let cancel, save;
        const keyHandler = (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                save().catch(error => {
                    console.error('Failed to save edit:', error);
                    this.showError('Failed to save edit', chatId);
                });
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancel();
            }
        };
        
        const clickOutside = (e) => {
            if (!contentDiv.contains(e.target) && !buttonsDiv.contains(e.target)) {
                cancel();
            }
        };
        
        // Use arrow functions to preserve 'this' context
        cancel = () => {
            contentDiv.contentEditable = false;
            contentDiv.classList.remove('editing');
            // Restore the original rendered HTML content (markdown processed)
            contentDiv.innerHTML = marked.parse(originalRawContent, {
                breaks: true, gfm: true, sanitize: false
            });
            buttonsDiv.remove();
            // Clean up event listeners
            contentDiv.removeEventListener('keydown', keyHandler);
            document.removeEventListener('click', clickOutside);
            // Restore the edit trigger
            this.addEditTrigger(contentDiv, originalRawContent, 'user', chatId);
        };
        
        save = async () => {
            // CRITICAL FIX: Get edited content while preserving formatting
            const newContent = this.getEditableContent(contentDiv).trim();
            if (!newContent) {
                this.showError('Message cannot be empty', chatId);
                return;
            }
            
            // Always proceed even if content is the same - user may want to regenerate response
            
            // Truncate messages after the edited message (keeping the edited message)
            this.truncateMessages(chatId, messageIndex, 'Message edited');
            
            // Reload the chat to show clipped history (force render to refresh UI)
            this.loadChat(chatId, true);
            
            // Send the new message
            const chatInput = this.getChatInput(chatId);
            if (chatInput) {
                // Convert markdown to HTML for contentEditable
                const htmlContent = newContent
                    .replace(/&/g, '&amp;')
                    .replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;')
                    .replace(/\n/g, '<br>');
                chatInput.innerHTML = htmlContent;
                await this.sendMessage(chatId);
            } else {
                const editChat = this.chats.get(chatId);
                if (!editChat || !editChat.isSubChat) {
                    // Only log error for main chats - sub-chats don't have input elements
                    console.error('[editUserMessage.save] ERROR: Could not find input for chat', chatId);
                }
                // Fall back to sending message directly with the new content
                await this.sendMessage(chatId, newContent);
            }
        };
        
        buttonsDiv.querySelector('.btn-primary').onclick = save;
        buttonsDiv.querySelector('.btn-secondary').onclick = cancel;
        
        contentDiv.addEventListener('keydown', keyHandler);
        setTimeout(() => document.addEventListener('click', clickOutside), 0);
    }
    
    addToolCall(toolName, args, chatId, turn, _messageIndex, includeInContext, toolId) {
        if (!chatId) {
            console.error('addToolCall called without chatId');
            return;
        }
        
        if (!toolId) {
            console.error(`addToolCall called without toolId for tool: ${toolName}`);
            return;
        }
        
        // If we have a current assistant group, use it as target
        let targetContainer = this.getCurrentAssistantGroup(chatId);
        let needsAppendToChat = false;
        
        if (!targetContainer) {
            // Create a new standalone container for the tool
            targetContainer = document.createElement('div');
            needsAppendToChat = true;
        }
        
        const toolDiv = document.createElement('div');
        toolDiv.className = 'tool-block';
        toolDiv.dataset.toolId = toolId;
        toolDiv.dataset.toolName = toolName;
        toolDiv.dataset.included = String(includeInContext);
        toolDiv.style.position = 'relative';
        
        // Store the turn for auto mode
        const chat = this.chats.get(chatId);
        if (turn !== null) {
            toolDiv.dataset.turn = turn;
        } else if (chat) {
            toolDiv.dataset.turn = chat.currentTurn || 0;
        }
        
        // Extract metadata fields from args if present
        const metadataFields = ['tool_purpose', 'expected_format', 'key_information', 'success_indicators', 'context_for_interpretation'];
        const metadata = {};
        const cleanedArgs = { ...args };
        
        // Extract and remove metadata fields from args
        for (const field of metadataFields) {
            if (args && field in args) {
                metadata[field] = args[field];
                delete cleanedArgs[field];
            }
        }
        
        // Store metadata for later use
        toolDiv.dataset.metadata = JSON.stringify(metadata);
        
        // Don't display metadata in tool header - it will be shown in sub-chat
        const metadataHtml = '';
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
            ${metadataHtml}
            <span class="tool-toggle">▶</span>
            <span class="tool-label"><i class="fas fa-wrench"></i> ${toolName}</span>
            <span class="tool-info">
                <span class="tool-status"><i class="fas fa-hourglass-half"></i> Calling...</span>
            </span>
        `;
        
        const toolContent = document.createElement('div');
        toolContent.className = 'tool-content collapsed';
        
        // Add request section
        const requestSection = document.createElement('div');
        requestSection.className = 'tool-request-section';
        
        // Create controls div
        const controlsDiv = document.createElement('div');
        controlsDiv.className = 'tool-section-controls';
        controlsDiv.innerHTML = '<span class="tool-section-label">📤 REQUEST</span>';
        
        // Create copy button using standardized method
        const requestCopyBtn = this.createCopyButton({
            buttonClass: 'tool-section-copy',
            tooltip: 'Copy request',
            onCopy: () => JSON.stringify(cleanedArgs, null, 2)
        });
        
        controlsDiv.appendChild(requestCopyBtn);
        requestSection.appendChild(controlsDiv);
        
        // Add the request content (without metadata fields)
        const preElement = document.createElement('pre');
        preElement.textContent = JSON.stringify(cleanedArgs, null, 2);
        requestSection.appendChild(preElement);
        
        toolContent.appendChild(requestSection);
        
        // Add separator (will be visible when response is added)
        const separator = document.createElement('div');
        separator.className = 'tool-separator';
        separator.style.display = 'none';
        toolContent.appendChild(separator);
        
        // Placeholder for response
        const responseSection = document.createElement('div');
        responseSection.className = 'tool-response-section';
        responseSection.style.display = 'none';
        toolContent.appendChild(responseSection);
        
        toolHeader.addEventListener('click', (_e) => {
            // Don't toggle if clicking on checkbox, label, or toggle switch
            // No need to check for tool inclusion elements - they've been removed
            const isCollapsed = toolContent.classList.contains('collapsed');
            toolContent.classList.toggle('collapsed');
            toolHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
        });
        
        // Tool inclusion checkboxes have been removed
        // No checkbox event handlers needed
        
        toolDiv.appendChild(toolHeader);
        toolDiv.appendChild(toolContent);
        targetContainer.appendChild(toolDiv);
        
        // Store reference for later update - use toolId as key for proper matching
        if (chat && chat.pendingToolCalls) {
            chat.pendingToolCalls.set(toolId, { toolName, toolDiv });
        }
        
        // Only append to chat if we created a standalone container
        if (needsAppendToChat) {
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                container._elements.messages.appendChild(targetContainer);
            }
        }
        
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
    }

    addToolResult(toolName, result, chatId, responseTime, responseSize, includeInContext, _messageIndex, toolCallId, _subChatId) {
        if (!chatId) {
            console.error('addToolResult called without chatId');
            return;
        }
        
        if (!toolCallId) {
            console.error(`addToolResult called without toolCallId for tool: ${toolName}`);
            return;
        }
        
        // Find the pending tool call using toolCallId from the chat
        const chat = this.chats.get(chatId);
        if (!chat || !chat.pendingToolCalls) {
            console.error(`addToolResult: Chat ${chatId} not found or has no pendingToolCalls`);
            return;
        }
        
        let toolInfo = null;
        let toolDiv = null;
        
        if (chat.pendingToolCalls.has(toolCallId)) {
            toolInfo = chat.pendingToolCalls.get(toolCallId);
            toolDiv = toolInfo.toolDiv;
        } else {
            console.error(`addToolResult: No pending tool call found with ID ${toolCallId} for tool ${toolName} in chat ${chatId}`);
            return;
        }
        
        if (toolDiv) {
            // Update existing tool block
            const statusSpan = toolDiv.querySelector('.tool-status');
            const infoSpan = toolDiv.querySelector('.tool-info');
            
            // Use provided size or calculate it
            const resultSize = responseSize !== null ? responseSize : 
                typeof result === 'string' 
                    ? result.length 
                    : JSON.stringify(result).length
            ;
            
            // Format size info
            let sizeInfo;
            if (resultSize < 1024) {
                sizeInfo = `${resultSize} bytes`;
            } else if (resultSize < 1024 * 1024) {
                sizeInfo = `${(resultSize / 1024).toFixed(1)} KB`;
            } else {
                sizeInfo = `${(resultSize / (1024 * 1024)).toFixed(1)} MB`;
            }
            
            // Format response time
            const timeInfo = responseTime > 0 ? `${(responseTime / 1000).toFixed(2)}s` : '0s';
            
            // Update status
            if (statusSpan) {
                statusSpan.innerHTML = result.error ? '<i class="fas fa-times-circle"></i> Error' : '<i class="fas fa-check-circle"></i> Complete';
            }
            
            // Update metrics while preserving checkbox
            if (infoSpan) {
                // Save checkbox state and reference
                infoSpan.innerHTML = `
                    <span class="tool-status">${result.error ? '<i class="fas fa-times-circle"></i> Error' : '<i class="fas fa-check-circle"></i> Complete'}</span>
                    <span class="tool-metric"><i class="fas fa-clock"></i> ${timeInfo}</span>
                    <span class="tool-metric"><i class="fas fa-box"></i> ${sizeInfo}</span>
                `;
                
                // Tool inclusion checkboxes have been removed
            }
            
            // Update response section
            const responseSection = toolDiv.querySelector('.tool-response-section');
            const separator = toolDiv.querySelector('.tool-separator');
            
            if (responseSection) {
                let formattedResult;
                let isMarkdown = false;
                
                if (typeof result === 'object') {
                    if (result.error) {
                        formattedResult = `<span style="color: var(--danger-color);">${result.error}</span>`;
                    } else {
                        formattedResult = `<pre>${JSON.stringify(result, null, 2)}</pre>`;
                    }
                } else {
                    // For string responses, check if it looks like markdown
                    const textResult = String(result);
                    if (this.isMarkdownContent(textResult)) {
                        formattedResult = marked.parse(textResult, {
                            breaks: true,
                            gfm: true,
                            sanitize: false
                        });
                        isMarkdown = true;
                    } else {
                        formattedResult = `<pre>${textResult}</pre>`;
                    }
                }
                
                // Clear and rebuild response section
                responseSection.innerHTML = '';
                
                // Create controls div
                const responseControlsDiv = document.createElement('div');
                responseControlsDiv.className = 'tool-section-controls';
                responseControlsDiv.innerHTML = '<span class="tool-section-label">📥 RESPONSE</span>';
                
                // Create copy button using standardized method
                const responseCopyBtn = this.createCopyButton({
                    buttonClass: 'tool-section-copy',
                    tooltip: 'Copy response',
                    onCopy: () => typeof result === 'object' ? JSON.stringify(result, null, 2) : String(result)
                });
                
                responseControlsDiv.appendChild(responseCopyBtn);
                responseSection.appendChild(responseControlsDiv);
                
                // Add the formatted result
                const resultDiv = document.createElement('div');
                if (isMarkdown) {
                    resultDiv.className = 'message-content';
                }
                resultDiv.innerHTML = formattedResult;
                responseSection.appendChild(resultDiv);
                
                responseSection.style.display = 'block';
                
                if (separator) {
                    separator.style.display = 'block';
                }
                
                // Sub-chats are rendered as separate chat items via renderSubChatAsItem
                // No need for expandable UI here
            }
            
            // Remove from pending using the toolCallId
            if (toolCallId && chat.pendingToolCalls) {
                chat.pendingToolCalls.delete(toolCallId);
            }
        }
        
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
    }
    
    /**
     * Check if a sub-chat should be created for tool response
     */
    async shouldCreateSubChat(chat, responseSize, _toolCall) {
        // Never create sub-chats within sub-chats
        if (chat.isSubChat) {
            return false;
        }
        
        // Check if tool summarization is enabled
        if (!chat.config?.optimisation?.toolSummarisation?.enabled) {
            return false;
        }
        
        // Get threshold in bytes (convert from KiB)
        const thresholdKiB = chat.config.optimisation.toolSummarisation.thresholdKiB ?? 20;
        const thresholdBytes = thresholdKiB * 1024;
        
        // If threshold is 0, process all tools
        if (thresholdKiB === 0) {
            return true;
        }
        
        // Check if response exceeds threshold
        return responseSize > thresholdBytes;
    }
    
    /**
     * Create a sub-chat for processing tool response
     */
    async createSubChatForTool(parentChat, toolCall, toolResult) {
        // Try to find the tool div in the current chat's container
        const chatContainer = this.chatContainers.get(parentChat.id);
        let toolDiv = null;
        
        if (chatContainer) {
            toolDiv = chatContainer.querySelector(`[data-tool-id="${toolCall.id}"]`);
        }
        
        // Fallback: search globally
        if (!toolDiv) {
            toolDiv = document.querySelector(`[data-tool-id="${toolCall.id}"]`);
        }
        
        // Debug: check all tool divs in current chat
        if (chatContainer) {
            const _allToolDivs = chatContainer.querySelectorAll('[data-tool-id]');
        }
        
        const metadata = toolDiv ? JSON.parse(toolDiv.dataset.metadata || '{}') : {};

        // Determine sub-chat model
        const subChatModel = parentChat.config.optimisation.toolSummarisation.model || {
            provider: parentChat.model.provider || 'anthropic',
            id: parentChat.model.id || 'claude-3-haiku-20240307'
        };
        
        // Create sub-chat with minimal config - no optimizations
        const subChatOptions = {
            mcpServerId: parentChat.mcpServerId,
            llmProviderId: parentChat.llmProviderId,
            model: ChatConfig.modelConfigToString(subChatModel),
            title: `Processing: ${toolCall.name}`,
            parentChatId: parentChat.id,
            parentToolCallId: toolCall.id,
            toolMetadata: metadata,
            config: {
                model: subChatModel,
                optimisation: {
                    // Disable ALL optimizations for sub-chats
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
                    cacheControl: parentChat.config.optimisation.cacheControl || 'all-off', // Inherit parent's cache control
                    titleGeneration: {
                        enabled: false,
                        model: null
                    }
                },
                mcpServer: parentChat.mcpServerId
            }
        };
        
        const subChatId = await this.createNewChat(subChatOptions);
        if (!subChatId) {
            console.error('[createSubChatForTool] Failed to create sub-chat');
            return null;
        }
        
        const subChat = this.chats.get(subChatId);
        if (!subChat) {
            console.error('[createSubChatForTool] Sub-chat not found after creation');
            return null;
        }
        
        // Sub-chat visibility will be handled when it's rendered as a separate item
        
        // Initialize sub-chat with system prompt and tool result
        await this.initializeSubChat(subChatId, toolCall, toolResult, metadata);
        
        // Sub-chat will be processed synchronously in processSingleLLMResponse
        
        return subChatId;
    }
    
    /**
     * Update sub-chat section visibility in tool result
     */
    updateSubChatVisibility(toolCallId, subChatId, subChat) {
        // Find the parent chat ID from the sub-chat
        const parentChatId = subChat.parentChatId;
        if (!parentChatId) {
            console.error('[updateSubChatVisibility] Sub-chat missing parentChatId');
            return;
        }
        
        const container = this.getChatContainer(parentChatId);
        if (!container) {
            console.error(`[updateSubChatVisibility] Container not found for parent chat: ${parentChatId}`);
            return;
        }
        
        const subChatSection = container.querySelector(`.sub-chat-section[data-tool-call-id="${toolCallId}"]`);

        if (subChatSection) {
            subChatSection.dataset.subChatId = subChatId;
            subChatSection.style.display = 'block';
            
            // Update stats
            const statsSpan = subChatSection.querySelector('.sub-chat-stats');
            if (statsSpan) {
                const messageCount = subChat.messages.length;
                const modelName = ChatConfig.getModelDisplayName(subChat.model);
                statsSpan.textContent = `${messageCount} messages • ${modelName}`;
            }
        } else {
            console.error(`[updateSubChatVisibility] Sub-chat section not found for toolCallId: ${toolCallId}`);
            // List all existing sub-chat sections for debugging
            const allSubChatSections = container.querySelectorAll('.sub-chat-section');
            allSubChatSections.forEach((_section, _index) => {  });
        }
    }
    
    /**
     * Initialize sub-chat with system prompt and tool result
     */
    async initializeSubChat(subChatId, toolCall, toolResult, metadata) {
        const subChat = this.chats.get(subChatId);
        if (!subChat) {
            console.error(`[initializeSubChat] Sub-chat not found for subChatId: ${subChatId}, toolCallId: ${toolCall?.id}`);
            return;
        }
        
        // Create the sub-chat system prompt with full MCP capabilities
        const systemPrompt = SystemMsg.createSpecializedSystemPrompt('subchat');
        
        // Update system message
        if (subChat.messages.length > 0 && subChat.messages[0].role === 'system') {
            subChat.messages[0].content = systemPrompt;
        }
        
        // Clean the tool arguments by removing metadata fields
        const metadataFields = ['tool_purpose', 'expected_format', 'key_information', 'success_indicators', 'context_for_interpretation'];
        const cleanedArgs = { ...toolCall.arguments };
        
        // Remove metadata fields from the arguments
        for (const field of metadataFields) {
            if (cleanedArgs && field in cleanedArgs) {
                delete cleanedArgs[field];
            }
        }
        
        // Create a user message that combines the instructions from the primary LLM
        let userMessage = 'I am an AI assistant and I need your help to answer a broader question I am asked.';
        
        if (metadata.tool_purpose) {
            userMessage += `\n\nYour task is: ${metadata.tool_purpose}. `;
        }

        userMessage += `\n\nThis is usually done by executing the ${toolCall.name} tool. `;
        userMessage += `\n\nHere are the arguments I would use:\n\`\`\`json\n${JSON.stringify(cleanedArgs, null, 2)}\n\`\`\``;
        userMessage += `\n\nMy assumption that this tool and parameters will provide the desired result, may be wrong. `;
        userMessage += `In that case, come up with your own plan to answer the question.`;

        if (metadata.key_information) {
            userMessage += `\n\nFocus on: ${metadata.key_information}`;
        }
        
        if (metadata.expected_format) {
            userMessage += `\n\n**CRITICAL**: The expected format is: ${metadata.expected_format}`;
        }
        
        if (metadata.success_indicators) {
            userMessage += `\n\nSuccess indicators to look for: ${metadata.success_indicators}`;
        }
        
        if (metadata.context_for_interpretation) {
            userMessage += `\n\nAdditional context you may need during your investigation: ${metadata.context_for_interpretation}`;
        }

        if (metadata.tool_purpose) {
            userMessage += `\n\nPlease use any of your tools, and adapt to ${metadata.tool_purpose}. `;
        }
        else {
            userMessage += '\n\nPlease use any of your tools, and adapt to provide the answer I seek. ';
        }
        userMessage += '\n\nImportant: Do not ask me any question back, or provide explanations on tool usage, or give up on the first try. ';
        userMessage += 'Check your tools available, adapt to the issues you face (wrong parameters, empty responses, wrong tool chosen, etc), ';
        userMessage += 'and provide an authoritative answer. ';
        userMessage += '\n\n**CRITICAL**: If you encounter large datasets or lists, process EVERY single item. ';
        userMessage += 'Never sample, never use "..." or "among others". Process all items and explicitly state how many you analyzed. ';
        userMessage += 'Your thoroughness is essential for accurate analysis.';
        userMessage += '\n\n**CRITICAL**: If tools return errors or empty data, DO NOT give up! ';
        userMessage += 'Try different parameters, broader time ranges, different filters, or alternative approaches. ';
        userMessage += 'Make multiple attempts before concluding no data exists. The primary assistant is counting on you to be persistent and thorough.';
        userMessage += '\n\n**IF TASK CANNOT BE COMPLETED**: Use the ESCALATION protocol to document your attempts, ';
        userMessage += 'provide any partial data you found, and suggest specific alternatives for the primary assistant to try.';

        // Add the formatted user request
        subChat.messages.push({
            role: 'user',
            content: userMessage,
            timestamp: new Date().toISOString()
        });
        
        // Simulate the assistant requesting the tool
        const toolRequestContent = [{
            type: 'text',
            text: `Let me first call the ${toolCall.name} tool to get some data.`
        }, {
            type: 'tool_use',
            id: toolCall.id,
            name: toolCall.name,
            input: cleanedArgs || {}
        }];
        
        subChat.messages.push({
            role: 'assistant',
            content: toolRequestContent,
            timestamp: new Date().toISOString()
        });
        
        // Add the tool result
        subChat.messages.push({
            role: 'tool-results',
            toolResults: [{
                toolCallId: toolCall.id,
                toolName: toolCall.name,
                result: toolResult,
                isError: false
            }],
            timestamp: new Date().toISOString()
        });
        
        // Save sub-chat
        this.autoSave(subChatId);
    }
    
    /**
     * Process sub-chat and update parent
     */
    async processSubChat(subChatId, parentChatId, _toolCallId) {
        const subChat = this.chats.get(subChatId);
        const parentChat = this.chats.get(parentChatId);
        if (!subChat || !parentChat) {
            console.error('[processSubChat] Missing chats:', { subChat: !!subChat, parentChat: !!parentChat });
            return;
        }
        
        try {
            // Send message to process the tool result
            // Use a dummy message to trigger processing since sub-chat already has the tool result as a user message
            await this.sendMessage(subChatId, 'process', true); // isResume = true to skip adding new user message
            
            // CRITICAL: Wait for the sub-chat to be completely done processing
            // The sub-chat might make multiple tool calls, so we need to wait until it's no longer processing
            while (subChat.isProcessing) {
                console.log(`[processSubChat] Sub-chat ${subChatId} is still processing, waiting...`);
                // eslint-disable-next-line no-await-in-loop
                await new Promise(resolve => { setTimeout(resolve, 100); }); // Check every 100ms
            }
            
            // Find the latest assistant response (skip title and other special messages)
            let assistantMessage = null;
            for (let i = subChat.messages.length - 1; i >= 0; i--) {
                const msg = subChat.messages[i];
                if (msg.role === 'assistant') {
                    assistantMessage = msg;
                    break;
                }
            }
            
            if (assistantMessage) {
                // NOTE: The assistant message has already been rendered by sendMessage
                // We don't need to render it again here
                
                // Extract text content from assistant response
                let textContent = '';
                if (typeof assistantMessage.content === 'string') {
                    textContent = assistantMessage.content;
                } else if (Array.isArray(assistantMessage.content)) {
                    // Extract text from content blocks
                    const textBlocks = assistantMessage.content
                        .filter(block => block.type === 'text')
                        .map(block => block.text)
                        .join('\n\n');
                    textContent = textBlocks;
                } else {
                    textContent = JSON.stringify(assistantMessage.content);
                }
                
                // Note: Parent tool result is now updated directly in executeToolCalls
                // during interleaved execution, so we don't need to update it here
                
                // Update sub-chat stats display
                // Sub-chat DOM status will be updated through renderSubChatAsItem
                
                // NOTE: Don't update parent tool-result costs here - it's too early
                // The parent chat's tool-results message hasn't been added yet
                // Cost accumulation happens later in executeToolCalls after tool-results are added
                
                // Return the summarized result for the promise chain
                return textContent;
            } else {
                console.error('[processSubChat] No valid assistant response found');
                return null;
            }
        } catch (error) {
            console.error('[processSubChat] Error processing sub-chat:', error);
            return null;
        }
    }
    
    /**
     * Update parent chat's tool result with summarized version
     */
    updateParentToolResult(parentChatId, toolCallId, summarizedResult) {
        const parentChat = this.chats.get(parentChatId);
        if (!parentChat) {
            console.error('[updateParentToolResult] Parent chat not found');
            return;
        }
        
        // Get sub-chat costs by finding the sub-chat and using its existing totals
        let subChatCosts = null;
        for (const [_chatId, chat] of this.chats.entries()) {
            if (chat.isSubChat && chat.parentToolCallId === toolCallId) {
                // Ensure the sub-chat has up-to-date pricing
                this.updateChatTokenPricing(chat);
                
                // Copy the totals
                subChatCosts = {
                    totalTokens: { ...chat.totalTokensPrice },
                    perModel: {}
                };
                
                // Deep copy per-model data
                for (const [model, costs] of Object.entries(chat.perModelTokensPrice)) {
                    subChatCosts.perModel[model] = { ...costs };
                }
                break;
            }
        }
        
        // Find and update the tool result in parent messages
        let found = false;
        for (const message of parentChat.messages) {
            if (message.role === 'tool-results' && message.toolResults) {
                const toolResult = message.toolResults.find(tr => tr.toolCallId === toolCallId);
                if (!toolResult) {
                    console.warn(`[updateParentToolResult] Tool result not found in message for toolCallId: ${toolCallId}, parentChatId: ${parentChatId}`);
                    continue;
                }
                if (toolResult) {
                    toolResult.result = summarizedResult;
                    toolResult.wasProcessedBySubChat = true;
                    
                    // Add sub-chat cost information
                    toolResult.subChatCosts = subChatCosts;
                    
                    found = true;
                    break;
                }
            }
        }
        
        if (!found) {
            console.error(`[updateParentToolResult] Tool result not found in parent messages for toolCallId: ${toolCallId}, parentChatId: ${parentChatId}`);
        }
        
        // Update the DOM display
        const container = this.getChatContainer(parentChatId);
        if (container) {
            const toolDiv = container.querySelector(`[data-tool-id="${toolCallId}"]`);
            if (toolDiv) {
                const responseSection = toolDiv.querySelector('.tool-response-section');
                if (responseSection) {
                    // Find the actual result content div (not the controls or indicator)
                    const resultDivs = responseSection.querySelectorAll('div');
                    let actualResultDiv = null;
                    
                    for (const div of resultDivs) {
                        // Skip controls div and sub-chat indicator
                        if (!div.classList.contains('tool-section-controls') && 
                            !div.classList.contains('sub-chat-indicator') &&
                            !div.querySelector('.sub-chat-indicator')) {
                            actualResultDiv = div;
                            break;
                        }
                    }
                    
                    if (actualResultDiv) {
                        // Format as markdown if the content looks like markdown
                        let formattedContent;
                        if (this.isMarkdownContent(summarizedResult)) {
                            formattedContent = marked.parse(summarizedResult, {
                                breaks: true,
                                gfm: true,
                                sanitize: false
                            });
                        } else {
                            formattedContent = this.escapeHtml(summarizedResult);
                        }
                        actualResultDiv.innerHTML = `<div class="tool-summarized-result message-content">${formattedContent}</div>`;
                    }
                }
            }
        }
        
        // Update parent chat token pricing to include new sub-chat costs
        if (found && subChatCosts) {
            this.updateChatTokenPricing(parentChat);
            this.updateCumulativeTokenDisplay(parentChatId);
        }
        
        // Save parent chat
        this.autoSave(parentChatId);
    }
    
    /**
     * Update the status of an existing sub-chat item
     */
    updateSubChatStatus(parentChatId, toolCallId, newStatus) {
        const container = this.getChatContainer(parentChatId);
        if (!container) {
            console.error(`[updateSubChatStatus] Container not found for parent chat ${parentChatId}`);
            return;
        }
        
        // Find the sub-chat item
        const subChatItem = container.querySelector(`[data-tool-call-id="${toolCallId}"]`);
        if (!subChatItem) {
            console.error(`[updateSubChatStatus] Sub-chat item not found for tool ${toolCallId}`);
            return;
        }
        
        // Find the status element
        const statusElement = subChatItem.querySelector('.tool-status');
        if (!statusElement) {
            console.error(`[updateSubChatStatus] Status element not found in sub-chat`);
            return;
        }
        
        // Update status based on newStatus
        let iconClass, text;
        if (newStatus === 'processing') {
            iconClass = 'fas fa-hourglass-half';
            text = 'Processing...';
        } else if (newStatus === 'success') {
            iconClass = 'fas fa-check-circle';
            text = 'Summarized';
        } else if (newStatus === 'failed') {
            iconClass = 'fas fa-times-circle';
            text = 'Failed';
        } else {
            console.error(`[updateSubChatStatus] Unknown status: ${newStatus}`);
            return;
        }
        
        // Update the status element
        statusElement.innerHTML = `<i class="${iconClass}"></i> ${text}`;
    }
    
    /**
     * Render sub-chat as a separate chat item in the main chat flow
     */
    renderSubChatAsItem(parentChatId, subChatId, toolCallId, status = 'success') {
        const parentContainer = this.getChatContainer(parentChatId);
        const subChat = this.chats.get(subChatId);
        
        if (!parentContainer || !subChat) {
            console.error(`[renderSubChatAsItem] Missing required data - parentContainer: ${!!parentContainer}, subChat: ${!!subChat}, parentChatId: ${parentChatId}, subChatId: ${subChatId}, toolCallId: ${toolCallId}`);
            return;
        }
        
        // Find the tool result to insert sub-chat after
        let toolDiv = parentContainer.querySelector(`[data-tool-id="${toolCallId}"]`);
        
        // For loaded chats, we may need to find the tool div differently or create a placeholder
        if (!toolDiv) {
            // Try to find by searching all tool blocks and matching the content
            const allToolBlocks = parentContainer.querySelectorAll('.tool-block');
            for (const block of allToolBlocks) {
                // Check if this tool block might be our target
                // This is a fallback for loaded chats where tool IDs might not match
                if (block.textContent.includes(subChat.parentToolName || '')) {
                    toolDiv = block;
                    break;
                }
            }
            
            if (!toolDiv) {
                // If we still can't find it, create a standalone sub-chat display
                // This ensures sub-chats are visible even if we can't find the parent tool
                const messagesContainer = parentContainer._elements.messages;
                const standaloneSubChat = document.createElement('div');
                standaloneSubChat.className = 'tool-block sub-chat-standalone';
                standaloneSubChat.dataset.subChatId = subChatId;
                messagesContainer.appendChild(standaloneSubChat);
                toolDiv = standaloneSubChat; // Use this as our insertion point
            }
        }
        
        // Create sub-chat item container - use tool-block class for consistent styling
        const subChatItem = document.createElement('div');
        subChatItem.className = 'tool-block sub-chat-item';
        subChatItem.dataset.subChatId = subChatId;
        subChatItem.dataset.toolCallId = toolCallId;
        
        // Get tool metadata from the tool div
        let toolMetadata = {};
        try {
            const metadataStr = toolDiv.dataset.metadata;
            if (metadataStr) {
                toolMetadata = JSON.parse(metadataStr);
            }
        } catch (e) {
            console.warn('[renderSubChatAsItem] Failed to parse tool metadata:', e);
        }
        
        // Create header for expandable section - use tool-header class
        const subChatHeader = document.createElement('div');
        subChatHeader.className = 'tool-header sub-chat-header';
        
        // Handle different status types with appropriate icons and colors
        let statusIcon, statusLabel;
        if (status === 'processing') {
            statusIcon = 'fa-hourglass-half';
            statusLabel = 'Processing...';
        } else if (status === 'success') {
            statusIcon = 'fa-check-circle';
            statusLabel = 'Summarized';
        } else { // 'failed' or any other status
            statusIcon = 'fa-times-circle';
            statusLabel = 'Failed';
        }
        
        subChatHeader.innerHTML = `
            <span class="tool-toggle">▶</span>
            <span class="tool-label"><i class="fas fa-robot"></i> Tool Summarization </span>
            <span class="tool-info">
                <span class="tool-status"><i class="fas ${statusIcon}"></i> ${statusLabel}</span>
                <span class="tool-metric"><i class="fas fa-comments"></i> ${subChat.messages.length}</span>
                ${toolMetadata.tool_purpose ? `<span class="tool-metric" title="${this.escapeHtml(toolMetadata.tool_purpose)}"><i class="fas fa-info-circle"></i> Purpose</span>` : ''}
            </span>
        `;
        
        // Create content area - EXACTLY like tool-content
        const subChatContent = document.createElement('div');
        subChatContent.className = 'tool-content collapsed'; // Use tool-content class with collapsed
        
        // CRITICAL CHANGE: Create permanent messages container immediately
        // This ensures sub-chat has a real DOM container from the start
        const messagesDiv = document.createElement('div');
        messagesDiv.className = 'chat-messages';
        messagesDiv.style.maxHeight = '400px'; // Limit height for sub-chats
        messagesDiv.style.overflowY = 'auto';
        messagesDiv.style.backgroundColor = 'var(--sub-chat-bg, rgba(0, 0, 0, 0.02))'; // Theme-aware background
        messagesDiv.style.borderRadius = '8px';
        messagesDiv.style.padding = '10px';
        messagesDiv.style.margin = '10px';
        
        // Add the messages div to content immediately (even if collapsed)
        subChatContent.appendChild(messagesDiv);
        
        // Create permanent container structure for the sub-chat
        const permanentContainer = {
            _elements: {
                messages: messagesDiv
            },
            _isSubChatContainer: true,
            _parentChatId: parentChatId
        };
        
        // Store the permanent container in regular chatContainers
        // This ensures getChatContainer will return the permanent container
        this.chatContainers.set(subChatId, permanentContainer);
        
        // Add click handler for expand/collapse - EXACTLY like tools
        subChatHeader.addEventListener('click', () => {
            const isCollapsed = subChatContent.classList.contains('collapsed');
            subChatContent.classList.toggle('collapsed');
            subChatHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
            
            // Always load full sub-chat history when expanding
            // This ensures both live and loaded chats show complete history
            if (isCollapsed) {
                this.loadSubChatMessagesIntoContainer(subChatId, messagesDiv);
            }
        });
        
        subChatItem.appendChild(subChatHeader);
        subChatItem.appendChild(subChatContent);
        
        // Insert sub-chat item after the tool result
        toolDiv.parentNode.insertBefore(subChatItem, toolDiv.nextSibling);
    }
    
    /**
     * Load and render sub-chat messages into an existing messages container
     */
    loadSubChatMessagesIntoContainer(subChatId, messagesDiv) {
        const subChat = this.chats.get(subChatId);
        if (!subChat) {
            console.error(`[loadSubChatMessagesIntoContainer] Sub-chat not found for subChatId: ${subChatId}`);
            return;
        }
        
        // Clear existing content
        messagesDiv.innerHTML = '';
        
        // Initialize rendering state for sub-chat if needed
        if (!subChat.renderingState) {
            subChat.renderingState = {
                lastDisplayedTurn: 0,
                currentStepInTurn: 1
            };
        }
        
        // Add Processing Guidance as a system message at the beginning
        const parentChat = this.chats.get(subChat.parentChatId);
        if (parentChat) {
            const container = this.getChatContainer(subChat.parentChatId);
            if (container) {
                const toolDiv = container.querySelector(`[data-tool-id="${subChat.parentToolCallId}"]`);
                if (toolDiv && toolDiv.dataset.metadata) {
                    try {
                        const metadata = JSON.parse(toolDiv.dataset.metadata);
                        if (Object.keys(metadata).length > 0) {
                            // Create a guidance div with proper chat message styling
                            const guidanceDiv = document.createElement('div');
                            guidanceDiv.className = 'message-wrapper system';
                            guidanceDiv.innerHTML = `
                                <div class="message">
                                    <div class="message-content">
                                        <div class="processing-guidance">
                                            <h4>
                                                <i class="fas fa-info-circle"></i> Processing Guidance
                                            </h4>
                                            ${metadata.tool_purpose ? `<div class="guidance-item"><i class="fas fa-lightbulb"></i><strong>Purpose:</strong> ${metadata.tool_purpose}</div>` : ''}
                                            ${metadata.expected_format ? `<div class="guidance-item"><i class="fas fa-file-alt"></i><strong>Expected:</strong> ${metadata.expected_format}</div>` : ''}
                                            ${metadata.key_information ? `<div class="guidance-item"><i class="fas fa-search"></i><strong>Looking for:</strong> ${metadata.key_information}</div>` : ''}
                                            ${metadata.success_indicators ? `<div class="guidance-item"><i class="fas fa-check-circle"></i><strong>Success:</strong> ${metadata.success_indicators}</div>` : ''}
                                            ${metadata.context_for_interpretation ? `<div class="guidance-item"><i class="fas fa-book"></i><strong>Context:</strong> ${metadata.context_for_interpretation}</div>` : ''}
                                        </div>
                                    </div>
                                </div>
                            `;
                            messagesDiv.appendChild(guidanceDiv);
                        }
                    } catch (e) {
                        console.warn('[loadSubChatMessagesIntoContainer] Failed to parse metadata:', e);
                    }
                }
            }
        }
        
        // Use the regular chat rendering system for each message
        for (let i = 0; i < subChat.messages.length; i++) {
            const message = subChat.messages[i];
            
            // Special handling for system messages to make them collapsible
            if (message.role === 'system') {
                this.displaySystemPrompt(message.content, subChatId);
            } else {
                this.displayStoredMessage(message, i, subChatId);
            }
        }
    }
    
    /**
     * Render sub-chats for loaded chat history
     */
    renderSubChatsForLoadedChat(parentChatId) {
        const parentChat = this.chats.get(parentChatId);
        if (!parentChat) {
            console.error(`[renderSubChatsForLoadedChat] Parent chat not found for parentChatId: ${parentChatId}`);
            return;
        }
        
        // Find all sub-chats for this parent
        this.chats.forEach((chat, chatId) => {
            if (chat.isSubChat && chat.parentChatId === parentChatId) {
                // Determine status based on whether sub-chat has valid assistant response
                let status = 'failed';
                const lastMessage = chat.messages[chat.messages.length - 1];
                if (lastMessage && lastMessage.role === 'assistant') {
                    status = 'success';
                }
                
                // Render the sub-chat item
                this.renderSubChatAsItem(parentChatId, chatId, chat.parentToolCallId, status);
            }
        });
    }
    
    /**
     * Load sub-chat content into a container
     */
    
    /**
     * Update the tool result display in the DOM to show actual results instead of "Processing"
     */
    updateToolResultDisplay(chatId, toolCallId, toolResult) {
        // Skip updating DOM if this tool result was already processed by sub-chat
        // The sub-chat processing already updated the DOM with summarized content
        if (toolResult.wasProcessedBySubChat) {
            return;
        }
        
        const container = this.getChatContainer(chatId);
        if (!container) {
            console.error('[updateToolResultDisplay] Chat container not found');
            return;
        }
        
        const toolDiv = container.querySelector(`[data-tool-id="${toolCallId}"]`);
        if (!toolDiv) {
            console.error('[updateToolResultDisplay] Tool div not found');
            return;
        }
        
        const responseSection = toolDiv.querySelector('.tool-response-section');
        if (responseSection) {
            // Update the processing message with actual result
            const processingDiv = responseSection.querySelector('.sub-chat-processing');
            if (processingDiv) {
                // Replace processing message with actual result
                const resultContent = typeof toolResult.result === 'string' 
                    ? toolResult.result 
                    : JSON.stringify(toolResult.result, null, 2);
                    
                responseSection.innerHTML = `
                    <div class="tool-result-content">
                        <pre>${this.escapeHtml(resultContent.substring(0, 1000))}${resultContent.length > 1000 ? '...' : ''}</pre>
                    </div>
                `;
            }
        }
    }
    
    /**
     * Recalculate total tokens price for a chat
     */
    recalculateTotalTokensPrice(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.perModelTokensPrice) return;
        
        const total = {
            input: 0,
            output: 0,
            cacheRead: 0,
            cacheCreation: 0,
            totalCost: 0
        };
        
        for (const data of Object.values(chat.perModelTokensPrice)) {
            total.input += data.input || 0;
            total.output += data.output || 0;
            total.cacheRead += data.cacheRead || 0;
            total.cacheCreation += data.cacheCreation || 0;
            total.totalCost += data.totalCost || 0;
        }
        
        chat.totalTokensPrice = total;
    }
    
    scrollToBottom(chatId, force = false) {
        if (!chatId) {
            console.error('scrollToBottom called without chatId');
            return;
        }
        
        // Use chat-specific messages container
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements || !container._elements.messages) {return;}
        
        const chatMessages = container._elements.messages;
        
        // Store scroll state before any DOM changes
        const chat = this.chats.get(chatId);
        if (chat && !chat.scrollState) {
            chat.scrollState = {};
        }
        
        // Only scroll if user is already near the bottom, unless forced
        const threshold = 100; // pixels from bottom to consider "at bottom"
        const distanceFromBottom = chatMessages.scrollHeight - chatMessages.scrollTop - chatMessages.clientHeight;
        const isAtBottom = distanceFromBottom < threshold;
        
        // Store the current "at bottom" state for this chat
        if (chat) {
            chat.scrollState.wasAtBottom = isAtBottom;
            chat.scrollState.lastCheck = Date.now();
        }
        
        if (isAtBottom || force) {
            // Use requestAnimationFrame to ensure DOM has updated
            requestAnimationFrame(() => {
                chatMessages.scrollTop = chatMessages.scrollHeight;
            });
        }
    }
    
    // Add turn separator between conversation turns
    addTurnSeparator(turnNumber, chatId) {
        if (!chatId) {
            console.error('addTurnSeparator called without chatId');
            return;
        }
        
        const separator = document.createElement('div');
        separator.className = 'turn-separator';
        
        const turnLabel = document.createElement('span');
        turnLabel.className = 'turn-number';
        turnLabel.textContent = `Turn ${turnNumber}`;
        
        separator.appendChild(turnLabel);
        
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.messages) {
            container._elements.messages.appendChild(separator);
        }
    }
    
    // Add step number to the last message element
    addStepNumber(turn, step, chatId, element = null) {
        if (!chatId) {
            console.error('addStepNumber called without chatId');
            return;
        }
        
        let target = element;
        if (!target) {
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                target = container._elements.messages.lastElementChild;
            }
        }
        if (!target) {return;}
        
        // Don't add step numbers to separators, spinners, or metrics
        if (target.classList.contains('turn-separator') || 
            target.classList.contains('loading-spinner') || 
            target.classList.contains('assistant-metrics-footer')) {
            return;
        }
        
        // Check if step number already exists
        if (target.querySelector('.step-number')) {
            return;
        }
        
        const stepLabel = document.createElement('span');
        stepLabel.className = 'step-number';
        stepLabel.textContent = `${turn}.${step}`;
        stepLabel.title = `Turn ${turn}, Step ${step}`;
        
        target.appendChild(stepLabel);
    }

    // Helper method to clean content before sending to API
    cleanContentForAPI(content) {
        // Handle array content (with tool calls)
        if (Array.isArray(content)) {
            // Extract text blocks and concatenate them
            const textBlocks = content.filter(block => block.type === 'text');
            const textContent = textBlocks.map(block => block.text || '').join('\n\n').trim();
            // Remove thinking tags from the extracted text
            return textContent.replace(/<thinking>[\s\S]*?<\/thinking>/g, '').trim();
        }
        
        // Handle string content
        if (typeof content === 'string') {
            // Remove thinking tags and their content
            return content.replace(/<thinking>[\s\S]*?<\/thinking>/g, '').trim();
        }
        
        // Fallback for other types
        return '';
    }

    // Helper method to parse tool results from MCP
    parseToolResult(result) {
        // If result has a content property with type and text, extract all text content
        if (result && result.content && Array.isArray(result.content)) {
            const textContents = [];
            
            for (const item of result.content) {
                if (item.type === 'text' && item.text) {
                    // Try to parse the text as JSON if it looks like JSON
                    try {
                        const parsed = JSON.parse(item.text);
                        textContents.push(parsed);
                    } catch {
                        textContents.push(item.text);
                    }
                } else if (item.type === 'image' && item.data) {
                    // Handle image content
                    textContents.push({
                        type: 'image',
                        data: item.data,
                        mimeType: item.mimeType || 'image/png'
                    });
                } else if (item.type === 'resource' && item.resource) {
                    // Handle resource references
                    textContents.push({
                        type: 'resource',
                        uri: item.resource.uri,
                        mimeType: item.resource.mimeType,
                        text: item.resource.text
                    });
                }
            }
            
            // If we only have one text content, return it directly
            if (textContents.length === 1) {
                return textContents[0];
            } else if (textContents.length > 1) {
                // If multiple contents, return them as an array
                return textContents;
            }
        }
        return result;
    }

    // ===== NEW CENTRALIZED SPINNER AND STATUS MANAGEMENT =====
    
    /**
     * Shows spinner for assistant thinking state
     */
    showAssistantThinking(chatId) {
        this.setSpinnerState(chatId, 'thinking', 'Thinking...');
    }
    
    /**
     * Hides assistant thinking spinner
     */
    hideAssistantThinking(chatId) {
        this.clearSpinnerState(chatId);
    }
    
    /**
     * Shows spinner for tool execution
     */
    showToolExecuting(chatId, toolName) {
        this.setSpinnerState(chatId, 'tool', `Executing ${toolName}...`);
    }
    
    /**
     * Hides tool execution spinner
     */
    hideToolExecuting(chatId) {
        this.clearSpinnerState(chatId);
    }
    
    /**
     * Shows waiting countdown spinner
     */
    showWaitingCountdown(chatId, seconds) {
        this.setSpinnerState(chatId, 'waiting', `Waiting... ${seconds}s`);
    }
    
    /**
     * Updates waiting countdown text
     */
    updateWaitingCountdown(chatId, seconds) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.spinnerState || chat.spinnerState.type !== 'waiting') return;
        
        chat.spinnerState.text = `Waiting... ${seconds}s`;
        this.updateSpinnerText(chatId, chat.spinnerState.text);
    }
    
    /**
     * Hides waiting countdown spinner
     */
    hideWaitingCountdown(chatId) {
        this.clearSpinnerState(chatId);
    }
    
    /**
     * Shows secondary assistant waiting spinner
     */
    showSecondaryAssistantWaiting(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        this.setSpinnerState(chatId, 'secondary-waiting', 'Waiting secondary assistant...');
    }
    
    
    /**
     * Hides secondary assistant waiting spinner
     */
    hideSecondaryAssistantWaiting(chatId) {
        this.clearSpinnerState(chatId);
    }
    
    /**
     * Central method to set spinner state
     */
    setSpinnerState(chatId, type, text) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[setSpinnerState] Chat not found: ${chatId}`);
            return;
        }
        
        // Special handling for rate limit countdown
        if (chat.isInRateLimitCountdown && type !== 'waiting') {
            return;
        }
        
        // Clear any existing spinner first
        this.clearSpinnerState(chatId);
        
        // Set new spinner state
        chat.spinnerState = {
            type, // 'thinking', 'tool', 'waiting'
            text,
            startTime: Date.now()
        };
        
        // Update only this chat's tile status
        this.updateChatTileStatus(chatId);
        
        // Show spinner in message area
        this.displaySpinner(chatId, text);
    }
    
    /**
     * Clear spinner state and remove spinner
     */
    clearSpinnerState(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[clearSpinnerState] Chat not found for chatId: ${chatId}`);
            return;
        }
        
        // Don't clear spinner if we're in a rate limit countdown
        if (chat.isInRateLimitCountdown) {
            return;
        }
        
        // Clear state
        chat.spinnerState = null;
        
        // Update only this chat's tile status
        this.updateChatTileStatus(chatId);
        
        // Remove spinner from message area
        this.removeSpinner(chatId);
    }
    
    /**
     * Display spinner in message area
     */
    displaySpinner(chatId, text) {
        // Remove any existing spinner for this chat
        this.removeSpinner(chatId);
        
        // Create spinner element
        const spinnerDiv = document.createElement('div');
        spinnerDiv.id = `spinner-${chatId}`;
        spinnerDiv.className = 'message assistant loading-spinner';
        
        // Store start time
        const startTime = Date.now();
        
        // Create initial HTML
        spinnerDiv.innerHTML = `
            <div class="spinner-container">
                <div class="spinner"></div>
                <span class="spinner-text">${text} <span class="spinner-time">(0s)</span></span>
            </div>
        `;
        
        // Initialize spinner intervals Map if not exists
        if (!this.spinnerIntervals) {
            this.spinnerIntervals = new Map();
        }
        
        // Clear any existing interval for this chat
        if (this.spinnerIntervals.has(chatId)) {
            clearInterval(this.spinnerIntervals.get(chatId));
        }
        
        // Update time every 100ms
        const interval = setInterval(() => {
            const elapsed = Math.floor((Date.now() - startTime) / 1000);
            const timeElement = spinnerDiv.querySelector('.spinner-time');
            if (timeElement) {
                timeElement.textContent = `(${elapsed}s)`;
            }
        }, 100);
        
        // Store interval for this chat
        this.spinnerIntervals.set(chatId, interval);
        
        // Add to message area
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.messages) {
            const chatMessages = container._elements.messages;
            const chat = this.chats.get(chatId);
            
            // Check if we should auto-scroll BEFORE adding the spinner
            const threshold = 100;
            const wasAtBottom = chatMessages.scrollHeight - chatMessages.scrollTop - chatMessages.clientHeight < threshold;
            
            // Add the spinner
            chatMessages.appendChild(spinnerDiv);
            
            // Only auto-scroll if user was already at bottom or if we recently checked and they were at bottom
            const shouldScroll = wasAtBottom || 
                (chat && chat.scrollState && chat.scrollState.wasAtBottom && 
                 (Date.now() - chat.scrollState.lastCheck) < 1000);
            
            if (shouldScroll) {
                requestAnimationFrame(() => {
                    chatMessages.scrollTop = chatMessages.scrollHeight;
                });
            }
        }
    }
    
    /**
     * Update spinner text without recreating it
     */
    updateSpinnerText(chatId, text) {
        const spinner = document.getElementById(`spinner-${chatId}`);
        if (spinner) {
            const textElement = spinner.querySelector('.spinner-text');
            if (textElement) {
                const timeSpan = textElement.querySelector('.spinner-time');
                const currentTime = timeSpan ? timeSpan.textContent : '(0s)';
                textElement.innerHTML = `${text} <span class="spinner-time">${currentTime}</span>`;
            }
        }
    }
    
    /**
     * Remove spinner from message area
     */
    removeSpinner(chatId) {
        // Clear interval if exists
        if (this.spinnerIntervals && this.spinnerIntervals.has(chatId)) {
            clearInterval(this.spinnerIntervals.get(chatId));
            this.spinnerIntervals.delete(chatId);
        }
        
        // Get current scroll position before removing spinner
        const container = this.getChatContainer(chatId);
        let wasAtBottom = false;
        
        if (container && container._elements && container._elements.messages) {
            const chatMessages = container._elements.messages;
            const threshold = 100;
            wasAtBottom = chatMessages.scrollHeight - chatMessages.scrollTop - chatMessages.clientHeight < threshold;
        }
        
        // Remove spinner element
        const spinner = document.getElementById(`spinner-${chatId}`);
        if (spinner) {
            spinner.remove();
            
            // If user was at bottom before spinner removal, keep them at bottom
            if (wasAtBottom && container && container._elements && container._elements.messages) {
                requestAnimationFrame(() => {
                    container._elements.messages.scrollTop = container._elements.messages.scrollHeight;
                });
            }
        }
    }
    
    /**
     * Update only a specific chat tile's status icon
     */
    updateChatTileStatus(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        // Find the specific tile
        const tile = document.querySelector(`.chat-session-item[data-chat-id="${chatId}"]`);
        if (!tile) return;
        
        const statusIcon = tile.querySelector('.status-icon');
        if (!statusIcon) return;
        
        // Determine status based on state
        let iconHtml;
        let statusClass;
        
        if (chat.hasError) {
            iconHtml = '<i class="fas fa-exclamation-triangle"></i>';
            statusClass = 'status-error';
        } else if (chat.spinnerState) {
            switch (chat.spinnerState.type) {
                case 'thinking':
                    iconHtml = '<i class="fas fa-robot"></i>';
                    statusClass = 'status-llm-active';
                    break;
                case 'tool':
                    iconHtml = '<i class="fas fa-plug"></i>';
                    statusClass = 'status-mcp-active';
                    break;
                case 'waiting':
                    iconHtml = '<i class="fas fa-clock"></i>';
                    statusClass = 'status-waiting';
                    break;
                default:
                    iconHtml = '<i class="fas fa-spinner fa-spin"></i>';
                    statusClass = 'status-processing';
            }
        } else if (chat.wasWaitingOnLoad) {
            iconHtml = '<i class="fas fa-chain-broken"></i>';
            statusClass = 'status-broken';
        } else if (chat.messages && chat.messages.length > 1) {
            iconHtml = '<i class="fas fa-check-circle"></i>';
            statusClass = 'status-ready';
        } else {
            iconHtml = '<i class="fas fa-pause-circle"></i>';
            statusClass = 'status-idle';
        }
        
        // Update icon
        statusIcon.innerHTML = iconHtml;
        statusIcon.className = 'status-icon ' + statusClass;
    }
    
    /**
     * Clear error state
     */
    clearError(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        chat.hasError = false;
        chat.lastError = null;
        
        // Update tile
        this.updateChatTileStatus(chatId);
    }
    
    // ===== DEPRECATED: Old spinner methods - TO BE REMOVED =====
    
    showLoadingSpinner(chatId, text = 'Thinking...') {
        console.warn('[DEPRECATED] showLoadingSpinner is deprecated. Use showAssistantThinking, showToolExecuting, or showWaitingCountdown instead.');
        // For backwards compatibility during transition
        this.setSpinnerState(chatId, 'thinking', text);
    }

    hideLoadingSpinner(chatId) {
        console.warn('[DEPRECATED] hideLoadingSpinner is deprecated. Use hideAssistantThinking, hideToolExecuting, or hideWaitingCountdown instead.');
        // For backwards compatibility during transition
        this.clearSpinnerState(chatId);
    }
    
    // Helper to ensure spinner stays at bottom
    moveSpinnerToBottom(chatId) {
        if (!chatId) {
            console.error('moveSpinnerToBottom called without chatId');
            return;
        }
        
        const spinner = document.getElementById(`llm-loading-spinner-${chatId}`);
        if (spinner && spinner.parentNode) {
            // Remove and re-append to ensure it's at the bottom
            spinner.parentNode.removeChild(spinner);
            
            const container = this.getChatContainer(chatId);
            if (container && container._elements && container._elements.messages) {
                container._elements.messages.appendChild(spinner);
            }
            
            // Force scroll when moving spinner to bottom
            requestAnimationFrame(() => {
                this.scrollToBottom(chatId, true);
            });
        }
    }

    async reconnectMcpServer(mcpServerId) {
        if (!mcpServerId) {
            console.error('[reconnectMcpServer] Called without mcpServerId');
            return;
        }
        
        const server = this.mcpServers.get(mcpServerId);
        if (!server) {
            console.error(`[reconnectMcpServer] Server ${mcpServerId} not found`);
            return;
        }
        
        // Update button state if it exists
        if (this.reconnectMcpBtn) {
            this.reconnectMcpBtn.disabled = true;
            this.reconnectMcpBtn.textContent = 'Reconnecting...';
        }
        
        try {
            const mcpConnection = new MCPClient();
            mcpConnection.onLog = (logEntry) => {
                const prefix = logEntry.direction === 'sent' ? 'mcp-request' : 'mcp-response';
                this.addLogEntry(`${prefix}: ${server.name}`, logEntry);
            };
            await mcpConnection.connect(server.url);
            
            // Store the connection
            this.mcpConnections.set(mcpServerId, mcpConnection);
            
            // Update server status
            server.connected = true;
            this.saveSettings();
            this.updateMcpServersList();
            
            // Find all chats using this MCP server and update their UI
            for (const [chatId, chat] of this.chats) {
                if (chat.mcpServerId === mcpServerId) {
                    // Update the chat's UI to reflect the reconnection
                    const container = this.chatContainers.get(chatId);
                    if (container) {
                        // Hide the reconnect button for this chat
                        const reconnectBtn = container._elements.reconnectBtn;
                        if (reconnectBtn) {
                            reconnectBtn.style.display = 'none';
                        }
                        
                        // Update MCP dropdown to show connection status
                        this.populateMCPDropdown(chatId, null);
                    }
                }
            }
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `MCP server "${server.name}" reconnected successfully`
            });
            
        } catch (error) {
            console.error(`[reconnectMcpServer] Failed to reconnect to MCP server: ${error.message}`);
            
            // Find any chat using this server to show the error
            let targetChatId = null;
            for (const [chatId, chat] of this.chats) {
                if (chat.mcpServerId === mcpServerId) {
                    targetChatId = chatId;
                    break;
                }
            }
            
            if (targetChatId) {
                this.showError(`Failed to reconnect to MCP server: ${error.message}`, targetChatId);
            }
            
            if (this.reconnectMcpBtn) {
                this.reconnectMcpBtn.disabled = false;
                this.reconnectMcpBtn.textContent = 'Reconnect MCP Server';
            }
        }
    }

    // Also add auto-reconnect on send if disconnected
    handleMcpConnectionStateChange(mcpServerId, stateChange, chatId = null) {
        const { newState, details } = stateChange;
        
        // Update the server status in the list
        this.updateMcpServersList();
        
        // If we have a chatId, update the specific chat's UI
        if (chatId) {
            const chat = this.chats.get(chatId);
            if (chat) {
                this.updateChatConnectionUI(chatId, newState, details);
            }
        }
        
        // Update all chats using this MCP server
        for (const [cId, chat] of this.chats) {
            if (chat.mcpServerId === mcpServerId) {
                this.updateChatConnectionUI(cId, newState, details);
                
                // Handle cleanup for disconnected/failed states
                if (newState === 'DISCONNECTED' || newState === 'FAILED') {
                    this.cleanupPendingToolCalls(cId, details);
                }
            }
        }
    }
    
    updateChatConnectionUI(chatId, connectionState, details = {}) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const chatContainer = document.querySelector(`.chat-container[data-chat-id="${chatId}"]`);
        if (!chatContainer) {
            // Chat UI not ready yet, store the state for later
            chat.pendingConnectionState = { state: connectionState, details };
            return;
        }
        
        const messagesArea = chatContainer.querySelector('.chat-messages');
        const userInput = chatContainer.querySelector('.user-input');
        const sendButton = chatContainer.querySelector('.send-button');
        
        // Check if elements exist
        if (!messagesArea || !userInput || !sendButton) {
            // UI elements not ready, store state for later
            chat.pendingConnectionState = { state: connectionState, details };
            return;
        }
        
        // Remove any existing connection overlay
        const existingOverlay = chatContainer.querySelector('.connection-overlay');
        if (existingOverlay) {
            existingOverlay.remove();
        }
        
        // Handle different connection states
        switch (connectionState) {
            case 'CONNECTING':
            case 'RECONNECTING':
            case 'HANDSHAKING':
            case 'INITIALIZING':
                // Create connection progress overlay
                const overlay = document.createElement('div');
                overlay.className = 'connection-overlay';
                overlay.innerHTML = `
                    <div class="connection-progress">
                        <div class="spinner-border spinner-border-sm" role="status">
                            <span class="sr-only">Connecting...</span>
                        </div>
                        <div class="connection-status">
                            ${this.getConnectionStatusMessage(connectionState, details)}
                        </div>
                        ${details.reconnectAttempts > 0 ? `<div class="reconnect-attempts">Attempt ${details.reconnectAttempts}</div>` : ''}
                    </div>
                `;
                messagesArea.appendChild(overlay);
                
                // Disable input and send button
                userInput.disabled = true;
                userInput.placeholder = this.getConnectionStatusMessage(connectionState, details);
                sendButton.disabled = true;
                break;
                
            case 'CONNECTED':
                // Enable input and send button
                userInput.disabled = false;
                userInput.placeholder = 'Ask about your Netdata metrics...';
                sendButton.disabled = false;
                
                // Show success message briefly
                const successOverlay = document.createElement('div');
                successOverlay.className = 'connection-overlay success';
                successOverlay.innerHTML = `
                    <div class="connection-progress">
                        <i class="fas fa-check-circle"></i>
                        <div class="connection-status">Connected successfully</div>
                    </div>
                `;
                messagesArea.appendChild(successOverlay);
                setTimeout(() => successOverlay.remove(), 2000);
                break;
                
            case 'FAILED':
            case 'DISCONNECTED':
                // Show error state
                userInput.disabled = true;
                userInput.placeholder = details.error || 'MCP server disconnected - click Reconnect';
                sendButton.disabled = true;
                
                // Show reconnect button if not already showing
                const reconnectBtn = chatContainer.querySelector('.btn-reconnect-mcp');
                if (!reconnectBtn) {
                    const errorOverlay = document.createElement('div');
                    errorOverlay.className = 'connection-overlay error';
                    errorOverlay.innerHTML = `
                        <div class="connection-progress">
                            <i class="fas fa-exclamation-circle"></i>
                            <div class="connection-status">${details.error || 'Connection lost'}</div>
                            <button class="btn btn-sm btn-primary btn-reconnect-mcp mt-2" onclick="app.reconnectMcpServer('${chat.mcpServerId}')">
                                <i class="fas fa-plug"></i> Reconnect
                            </button>
                        </div>
                    `;
                    messagesArea.appendChild(errorOverlay);
                }
                break;
                
            default:
                // For any other states, just log them
                console.warn(`Unhandled connection state: ${connectionState}`, details);
                break;
        }
    }
    
    getConnectionStatusMessage(state, details) {
        switch (state) {
            case 'CONNECTING':
                return '🔄 Establishing connection...';
            case 'RECONNECTING':
                return '🔄 Reconnecting to MCP server...';
            case 'HANDSHAKING':
                return '🤝 Performing MCP handshake...';
            case 'INITIALIZING':
                if (details.phase === 'loading_tools') {
                    return '📋 Loading available tools...';
                }
                return '⚙️ Initializing MCP session...';
            case 'CONNECTED':
                return '✅ Connected successfully';
            case 'FAILED':
                return `❌ Connection failed: ${details.error || 'Unknown error'}`;
            case 'DISCONNECTED':
                return '🔌 Disconnected from MCP server';
            default:
                return 'Connecting...';
        }
    }
    
    cleanupPendingToolCalls(chatId, details = {}) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.pendingToolCalls || chat.pendingToolCalls.size === 0) return;
        
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements || !container._elements.messages) return;
        
        // Clear any spinner state
        this.clearSpinnerState(chatId);
        
        // Process each pending tool call
        for (const [_toolCallId, toolInfo] of chat.pendingToolCalls) {
            const { toolDiv } = toolInfo;
            
            if (toolDiv) {
                // Find the response section
                const responseSection = toolDiv.querySelector('.tool-response-section');
                if (responseSection && !responseSection.querySelector('.tool-response-content')) {
                    // Add error message to the response section
                    const errorContent = document.createElement('div');
                    errorContent.className = 'tool-response-content error';
                    errorContent.innerHTML = `
                        <div class="error-message">
                            <i class="fas fa-exclamation-circle"></i>
                            Tool execution failed: MCP server disconnected
                            ${details.error ? `<br><small>${details.error}</small>` : ''}
                        </div>
                    `;
                    responseSection.appendChild(errorContent);
                    
                    // Update the header to show error state
                    const responseLabel = responseSection.querySelector('.tool-section-label');
                    if (responseLabel) {
                        responseLabel.innerHTML = '<i class="fas fa-exclamation-circle" style="color: var(--danger-color);"></i> RESPONSE';
                    }
                }
            }
        }
        
        // Count pending tools before clearing
        const pendingCount = chat.pendingToolCalls.size;
        
        // Clear all pending tool calls for this chat
        chat.pendingToolCalls.clear();
        
        // Add a system message about the disconnection if there were pending tools
        if (pendingCount > 0) {
            this.addSystemMessage(
                `MCP server disconnected. ${pendingCount} pending tool execution(s) were cancelled.`,
                chatId,
                'fas fa-plug'
            );
        }
    }

    async ensureMcpConnection(mcpServerId, chatId = null) {
        if (this.mcpConnections.has(mcpServerId)) {
            const connection = this.mcpConnections.get(mcpServerId);
            if (connection.isReady()) {
                return connection;
            }
            // If connection exists but not ready, wait a bit and check again
            // This prevents creating duplicate connections during initialization
            await new Promise(resolve => { setTimeout(resolve, 100); });
            if (connection.isReady()) {
                return connection;
            }
        }
        
        // Only create new connection if none exists or existing one is truly disconnected
        const existingConnection = this.mcpConnections.get(mcpServerId);
        if (existingConnection && existingConnection.ws && existingConnection.ws.readyState === WebSocket.CONNECTING) {
            // Connection is still connecting, wait for it
            let attempts = 0;
            while (attempts < 50 && !existingConnection.isReady()) { // Max 5 seconds
                // eslint-disable-next-line no-await-in-loop
                await new Promise(resolve => { setTimeout(resolve, 100); });
                attempts++;
            }
            if (existingConnection.isReady()) {
                return existingConnection;
            }
        }
        
        // Try to reconnect
        const server = this.mcpServers.get(mcpServerId);
        if (!server) {
            throw new Error('MCP server configuration not found');
        }
        
        const mcpConnection = new MCPClient();
        mcpConnection.onLog = (logEntry) => {
            const prefix = logEntry.direction === 'sent' ? 'mcp-request' : 'mcp-response';
            this.addLogEntry(`${prefix}: ${server.name}`, logEntry);
        };
        
        // Set up connection state handler
        mcpConnection.onConnectionStateChange = (stateChange) => {
            this.handleMcpConnectionStateChange(mcpServerId, stateChange, chatId);
        };
        
        const isReconnect = existingConnection !== undefined;
        await mcpConnection.connect(server.url, isReconnect);
        
        this.mcpConnections.set(mcpServerId, mcpConnection);
        return mcpConnection;
    }
    
    // Token usage tracking methods
    calculateContextWindowTokens(chatId) {
        if (!chatId) {
            console.error('calculateContextWindowTokens called without chatId');
            return 0;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {return 0;}
        
        // Find the last summary checkpoint
        let lastSummaryIndex = -1;
        for (let i = chat.messages.length - 1; i >= 0; i--) {
            if (chat.messages[i].role === 'summary') {
                lastSummaryIndex = i;
                break;
            }
        }
        
        // If we have a summary, calculate tokens only from after it
        if (lastSummaryIndex >= 0) {
            const summaryMsg = chat.messages[lastSummaryIndex];
            let contextTokens = 0;
            
            // Start with the summary's completion tokens (this will be in the system prompt)
            if (summaryMsg.usage && summaryMsg.usage.completionTokens) {
                contextTokens = summaryMsg.usage.completionTokens;
            }
            
            // Find the latest assistant message AFTER the summary
            for (let i = chat.messages.length - 1; i > lastSummaryIndex; i--) {
                const message = chat.messages[i];
                if (message.role === 'assistant' && message.usage) {
                    // Skip title responses
                    if (i > 0 && chat.messages[i-1].role === 'system-title') {
                        continue;
                    }
                    
                    // For assistant messages after summary, include all tokens from that request
                    const usage = message.usage;
                    return (usage.promptTokens || 0) + 
                           (usage.cacheReadInputTokens || 0) + 
                           (usage.cacheCreationInputTokens || 0) + 
                           (usage.completionTokens || 0);
                }
            }
            
            // If no assistant message after summary, return just the summary tokens
            return contextTokens;
        }
        
        // No summary found - find the latest assistant message
        for (let i = chat.messages.length - 1; i >= 0; i--) {
            const message = chat.messages[i];
            if (message.role === 'assistant' && message.usage) {
                // Skip title and summary responses
                if (i > 0 && ['system-title', 'system-summary'].includes(chat.messages[i-1].role)) {
                    continue;
                }
                
                // Calculate total tokens for this message
                const usage = message.usage;
                return (usage.promptTokens || 0) + 
                       (usage.cacheReadInputTokens || 0) + 
                       (usage.cacheCreationInputTokens || 0) + 
                       (usage.completionTokens || 0);
            }
        }
        
        // No assistant message found
        return 0;
    }
    
    updateTokenUsage(chatId, usage, model) {
        if (!this.tokenUsageHistory.has(chatId)) {
            this.tokenUsageHistory.set(chatId, {
                requests: [],
                model
            });
        }
        
        const history = this.tokenUsageHistory.get(chatId);
        
        // Add this request to history with the model
        history.requests.push({
            timestamp: new Date().toISOString(),
            model,
            promptTokens: usage.promptTokens,
            completionTokens: usage.completionTokens,
            totalTokens: usage.totalTokens,
            cacheCreationInputTokens: usage.cacheCreationInputTokens || 0,
            cacheReadInputTokens: usage.cacheReadInputTokens || 0
        });
        
        // Update context window indicator
        this.updateContextWindowIndicator(chatId);
        
        // Update all token displays (context window, cumulative tokens, and cost)
        this.updateAllTokenDisplays(chatId);
        
        // Update any pending conversation total displays
        const pendingTotals = document.querySelectorAll('[id^="conv-total-"]');
        pendingTotals.forEach(el => {
            if (el.textContent === 'Calculating...' || el.textContent.match(/^\d/)) {
                el.textContent = contextTokens.toLocaleString();
            }
        });
    }
    
    updateContextWindowIndicator(chatId) {
        if (!chatId) {
            console.error('updateContextWindowIndicator called without chatId');
            return;
        }
        
        const container = this.chatContainers.get(chatId);
        if (!container) {
            console.error(`[updateContextWindowIndicator] Container not found for chatId: ${chatId}`);
            return;
        }
        
        const elements = container._elements;
        if (!elements.contextFill || !elements.contextStats) {return;}
        
        // Get the chat to use effective context window
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[updateContextWindowIndicator] Chat not found for chatId: ${chatId}`);
            return;
        }
        
        // Calculate the current context window tokens
        const totalTokens = this.calculateContextWindowTokens(chatId);
        
        // Use the effective context window (user-configured or model limit)
        const limit = this.getEffectiveContextWindow(chat);
        const percentUsed = Math.min(totalTokens / limit * 100, 100);
        
        // Update stats - show as "X / Y" or "Xk / Yk"
        if (totalTokens >= 1000 || limit >= 1000) {
            const totalDisplay = totalTokens >= 1000 ? `${(totalTokens / 1000).toFixed(1)}k` : totalTokens.toString();
            const limitDisplay = limit >= 1000 ? `${(limit / 1000).toFixed(0)}k` : limit.toString();
            elements.contextStats.textContent = `${totalDisplay} / ${limitDisplay}`;
        } else {
            elements.contextStats.textContent = `${totalTokens} / ${limit}`;
        }
        
        // Update bar
        elements.contextFill.style.width = percentUsed + '%';
        
        // Update color based on usage
        elements.contextFill.classList.remove('warning', 'danger');
        if (percentUsed >= 90) {
            elements.contextFill.classList.add('danger');
        } else if (percentUsed >= 75) {
            elements.contextFill.classList.add('warning');
        }
    }
    
    getTokenUsageForChat(chatId) {
        const history = this.tokenUsageHistory.get(chatId);
        if (!history || history.requests.length === 0) {
            return { totalTokens: 0 };
        }
        
        // Get the latest request's tokens
        const latestRequest = history.requests[history.requests.length - 1];
        
        // Context window includes:
        // - All input tokens (which already contains the full conversation history)
        // - The completion tokens (which will be part of the next request's input)
        const totalContextTokens = latestRequest.promptTokens + 
                                   (latestRequest.cacheReadInputTokens || 0) + 
                                   (latestRequest.cacheCreationInputTokens || 0) +
                                   (latestRequest.completionTokens || 0);
        
        return { totalTokens: totalContextTokens };
    }
    
    // Add cumulative tokens with model-based pricing lookup
    addCumulativeTokens(chatId, model, promptTokens, completionTokens, _cacheReadTokens = 0, _cacheCreationTokens = 0) {
        if (!chatId) {
            console.error('addCumulativeTokens called without chatId');
            return;
        }
        
        // This function is now deprecated - token tracking happens in addMessage()
        // Keeping it for backward compatibility but it just updates displays
        this.updateAllTokenDisplays(chatId);
    }
    
    // Get cumulative token usage for the entire chat - now just reads from stored data
    getCumulativeTokenUsage(chatId) {
        if (!chatId) {
            console.error('getCumulativeTokenUsage called without chatId');
            return { 
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            return {
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        if (!chat.totalTokensPrice) {
            return {
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        // Use stored token counts
        return { 
            inputTokens: chat.totalTokensPrice.input || 0, 
            outputTokens: chat.totalTokensPrice.output || 0,
            cacheCreationTokens: chat.totalTokensPrice.cacheCreation || 0,
            cacheReadTokens: chat.totalTokensPrice.cacheRead || 0
        };
    }
    
    // Centralized method to update all token-related displays
    updateAllTokenDisplays(chatId) {
        if (!chatId) {
            console.error('updateAllTokenDisplays called without chatId');
            return;
        }
        
        // Check if container exists before trying to update
        const container = this.chatContainers.get(chatId);
        if (!container) {
            // Container doesn't exist yet, skip updates
            return;
        }
        
        // Update context window
        const chat = this.chats.get(chatId);
        if (chat) {
            this.updateContextWindowIndicator(chatId);
        }
        
        // Update cumulative tokens
        this.updateCumulativeTokenDisplay(chatId);
    }
    
    // Update cumulative token display
    updateCumulativeTokenDisplay(chatId) {
        if (!chatId) {
            console.error('updateCumulativeTokenDisplay called without chatId');
            return;
        }
        
        // Skip token display updates for sub-chats - they don't have token counter UI
        const chat = this.chats.get(chatId);
        if (chat && chat.isSubChat) {
            return;
        }
        
        const cumulative = this.getCumulativeTokenUsage(chatId);
        // Get the chat-specific DOM elements
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements) {
            console.error('[updateCumulativeTokenDisplay] Container or elements not found for chat:', chatId);
            return;
        }
        
        const inputElement = container._elements.cumulativeInputTokens;
        const outputElement = container._elements.cumulativeOutputTokens;
        const cacheReadElement = container._elements.cumulativeCacheReadTokens;
        const cacheCreationElement = container._elements.cumulativeCacheCreationTokens;
        
        // Format numbers with k suffix for thousands
        const formatTokens = (num) => {
            if (num >= 1000) {
                const thousands = num / 1000;
                const formatted = thousands >= 1000 ? 
                    thousands.toFixed(1).replace(/\B(?=(\d{3})+(?!\d))/g, ',') : 
                    thousands.toFixed(1);
                return `${formatted}<span style="color: var(--text-tertiary); font-size: 10px;">k</span>`;
            }
            return num.toString();
        };
        
        if (inputElement) {
            inputElement.innerHTML = formatTokens(cumulative.inputTokens);
        }
        
        if (outputElement) {
            outputElement.innerHTML = formatTokens(cumulative.outputTokens);
        }
        
        // Always show cache token displays, even if zero
        if (cacheReadElement) {
            cacheReadElement.innerHTML = formatTokens(cumulative.cacheReadTokens);
        }
        
        if (cacheCreationElement) {
            cacheCreationElement.innerHTML = formatTokens(cumulative.cacheCreationTokens);
        }
        
        // Calculate and display total cost
        const cost = this.calculateTokenCost(chatId);
        const costElement = container._elements.cumulativeCost;
        if (costElement) {
            if (cost !== null && cost >= 0) {
                costElement.textContent = `$${cost.toFixed(4)}`;
                costElement.style.display = '';
            } else {
                // Show $0.0000 for empty chats
                costElement.textContent = '$0.0000';
                costElement.style.display = '';
            }
        }
        
        // Update tooltips with individual costs
        this.updateTokenCostTooltips(chatId);
    }
    
    // Calculate token cost for a chat - now just reads from stored data
    calculateTokenCost(chatId) {
        if (!chatId) {
            console.error('calculateTokenCost called without chatId');
            return 0;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {return 0;} // Return 0 for no chat
        
        // Use stored total cost
        if (chat.totalTokensPrice && chat.totalTokensPrice.totalCost !== undefined) {
            return chat.totalTokensPrice.totalCost;
        }
        
        return 0;
    }
    
    // Update token cost tooltips
    updateTokenCostTooltips(chatId) {
        if (!chatId) {
            console.error('updateTokenCostTooltips called without chatId');
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat || !chat.perModelTokensPrice) {return;}
        
        // Get the chat-specific token counters section
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements) {return;}
        
        const tokenCountersSection = container.querySelector('.token-counters-section');
        if (tokenCountersSection) {
            tokenCountersSection.removeAttribute('data-tooltip');
            
            // Add hover handlers if not already added
            if (!tokenCountersSection.hasHoverHandlers) {
                tokenCountersSection.hasHoverHandlers = true;
                
                // Mouse enter - show the breakdown
                tokenCountersSection.addEventListener('mouseenter', (e) => {
                    // Use current chat ID dynamically
                    this.showTokenBreakdownHover(e, chatId);
                });
                
                // Mouse leave - hide the breakdown
                tokenCountersSection.addEventListener('mouseleave', () => {
                    this.hideTokenBreakdownHover();
                });
            }
        }
    }
    
    showTokenBreakdownHover(event, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.perModelTokensPrice) {return;}
        
        // Remove any existing hover element
        this.hideTokenBreakdownHover();
        
        // Create the hover element
        const hoverDiv = document.createElement('div');
        hoverDiv.id = 'tokenBreakdownHover';
        hoverDiv.className = 'token-breakdown-hover';
        hoverDiv.style.cssText = `
            position: absolute;
            background: var(--background-color);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 16px;
            box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
            z-index: 10000;
            font-size: 12px;
            min-width: 600px;
        `;
        
        // Build the content
        const html = [];
        html.push('<div style="font-weight: 600; margin-bottom: 12px; font-size: 13px;">TOKEN USAGE AND COST BREAKDOWN</div>');
        html.push('<div style="display: flex; padding: 6px 0; font-weight: 600; border-bottom: 2px solid var(--border-color); font-size: 11px; color: var(--text-secondary);">');
        html.push('<div style="flex: 1;">Model</div>');
        html.push('<div style="width: 70px; text-align: right;">Input</div>');
        html.push('<div style="width: 70px; text-align: right;">Cache R</div>');
        html.push('<div style="width: 70px; text-align: right;">Cache W</div>');
        html.push('<div style="width: 70px; text-align: right;">Output</div>');
        html.push('<div style="width: 80px; text-align: right;">Cost</div>');
        html.push('</div>');
        
        // Add each model's data
        let hasData = false;
        for (const [model, data] of Object.entries(chat.perModelTokensPrice)) {
            const modelName = ChatConfig.getModelDisplayName(model);
            
            // Skip models with no usage
            if (data.input === 0 && data.output === 0 && data.cacheRead === 0 && data.cacheCreation === 0) {
                continue;
            }
            
            hasData = true;
            html.push('<div style="display: flex; padding: 6px 0; border-bottom: 1px solid var(--hover-color);">');
            html.push(`<div style="flex: 1; font-family: monospace;">${modelName}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${data.input.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${data.cacheRead.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${data.cacheCreation.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${data.output.toLocaleString()}</div>`);
            html.push(`<div style="width: 80px; text-align: right; font-family: monospace; color: #4CAF50;">$${data.totalCost.toFixed(4)}</div>`);
            html.push('</div>');
        }
        
        // Add totals
        if (chat.totalTokensPrice && hasData) {
            html.push('<div style="display: flex; padding: 8px 0 4px 0; border-top: 2px solid var(--border-color); font-weight: 600;">');
            html.push(`<div style="flex: 1;">TOTAL</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${chat.totalTokensPrice.input.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${chat.totalTokensPrice.cacheRead.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${chat.totalTokensPrice.cacheCreation.toLocaleString()}</div>`);
            html.push(`<div style="width: 70px; text-align: right; font-family: monospace;">${chat.totalTokensPrice.output.toLocaleString()}</div>`);
            html.push(`<div style="width: 80px; text-align: right; font-family: monospace; color: #4CAF50;">$${chat.totalTokensPrice.totalCost.toFixed(4)}</div>`);
            html.push('</div>');
        }
        
        if (!hasData) {
            html.push('<div style="padding: 20px; text-align: center; color: var(--text-tertiary);">No token usage data available</div>');
        }
        
        hoverDiv.innerHTML = html.join('');
        document.body.appendChild(hoverDiv);
        
        // Position the hover element
        const rect = event.target.getBoundingClientRect();
        const hoverRect = hoverDiv.getBoundingClientRect();
        
        // Position below the token counters, aligned to the right edge
        let top = rect.bottom + 8;
        let left = rect.right - hoverRect.width;
        
        // Ensure it doesn't go off the left edge
        if (left < 8) {
            left = 8;
        }
        
        // Ensure it doesn't go off the bottom
        if (top + hoverRect.height > window.innerHeight - 8) {
            // Position above instead
            top = rect.top - hoverRect.height - 8;
        }
        
        hoverDiv.style.top = `${top}px`;
        hoverDiv.style.left = `${left}px`;
    }
    
    hideTokenBreakdownHover() {
        const hoverDiv = document.getElementById('tokenBreakdownHover');
        if (hoverDiv) {
            hoverDiv.remove();
        }
    }
    
    showModelTooltip(event, chat) {
        // Remove any existing tooltip
        this.hideModelTooltip();
        
        // Create the tooltip element
        const tooltipDiv = document.createElement('div');
        tooltipDiv.id = 'modelTooltipHover';
        tooltipDiv.className = 'model-tooltip-hover';
        tooltipDiv.style.cssText = `
            position: absolute;
            background: var(--background-color);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0;
            box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2);
            z-index: 10000;
            font-size: 12px;
        `;
        
        tooltipDiv.innerHTML = this.createModelTooltip(chat);
        document.body.appendChild(tooltipDiv);
        
        // Position the tooltip
        const rect = event.target.getBoundingClientRect();
        const tooltipRect = tooltipDiv.getBoundingClientRect();
        
        // Check if this is a chat list item (has .session-model class)
        const isChatListItem = event.target.classList.contains('session-model');
        
        let top, left;
        
        if (isChatListItem) {
            // For chat list items, position to the right
            top = rect.top;
            left = rect.right + 8;
            
            // Adjust if it goes off the right edge
            if (left + tooltipRect.width > window.innerWidth - 8) {
                // Position to the left instead
                left = rect.left - tooltipRect.width - 8;
            }
            
            // Adjust if it goes off the bottom
            if (top + tooltipRect.height > window.innerHeight - 8) {
                // Align with bottom of viewport
                top = window.innerHeight - tooltipRect.height - 8;
            }
        } else {
            // For other elements (header), position below
            top = rect.bottom + 8;
            left = rect.left;
            
            // Adjust if it goes off the right edge
            if (left + tooltipRect.width > window.innerWidth - 8) {
                left = window.innerWidth - tooltipRect.width - 8;
            }
            
            // Adjust if it goes off the bottom
            if (top + tooltipRect.height > window.innerHeight - 8) {
                // Position above instead
                top = rect.top - tooltipRect.height - 8;
            }
        }
        
        tooltipDiv.style.top = `${top}px`;
        tooltipDiv.style.left = `${left}px`;
    }
    
    hideModelTooltip() {
        const tooltipDiv = document.getElementById('modelTooltipHover');
        if (tooltipDiv) {
            tooltipDiv.remove();
        }
    }
    
    // Copy conversation metrics to clipboard
    async copyConversationMetrics(chatId) {
        if (!chatId) {
            this.showError('No active chat to copy metrics from', chatId);
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat || !this.hasUserContent(chat)) {
            this.showError('No messages to copy', chatId);
            return;
        }
        
        // Create a copy of the messages array with empty payloads
        const sanitizedMessages = chat.messages.map((message, index) => {
            // Start with a shallow copy of the message
            const sanitized = { ...message, index };
            
            // Clean up: remove 'type' field if 'role' exists
            if (sanitized.role && sanitized.type) {
                delete sanitized.type;
            }
            
            // Remove the actual content/payload
            delete sanitized.content;
            
            // Add content size for debugging
            if (message.content) {
                sanitized.contentSize = typeof message.content === 'string' 
                    ? message.content.length 
                    : JSON.stringify(message.content).length;
            }
            
            
            // Handle toolResults field
            if (message.toolResults && Array.isArray(message.toolResults)) {
                sanitized.toolResults = message.toolResults.map(r => {
                    const resultCopy = { ...r };
                    if (r.result) {
                        resultCopy.resultSize = typeof r.result === 'string' 
                            ? r.result.length 
                            : JSON.stringify(r.result).length;
                        resultCopy.result = '<tool response omitted>';  // Replace with placeholder
                    }
                    return resultCopy;
                });
                // Add total size for all tool results
                sanitized.totalToolResultsSize = sanitized.toolResults.reduce((sum, r) => sum + (r.resultSize || 0), 0);
            }
            
            return sanitized;
        });
        
        const output = {
            title: chat.title,
            model: ChatConfig.getChatModelString(chat),
            messageCount: chat.messages.length,
            messages: sanitizedMessages,
            configuration: chat.config  // Include the entire config as-is
        };
        
        try {
            await this.writeToClipboard(JSON.stringify(output, null, 2));
            
            // Show success feedback
            const btn = this.copyMetricsBtn;
            const originalHTML = btn.innerHTML;
            btn.innerHTML = '<i class="fas fa-check"></i> Copied JSON!';
            btn.classList.add('btn-success');
            
            setTimeout(() => {
                btn.innerHTML = originalHTML;
                btn.classList.remove('btn-success');
            }, 2000);
        } catch (error) {
            this.showError('Failed to copy to clipboard: ' + error.message, chatId);
        }
    }
    
    getCurrentTemperature(chatId) {
        if (!chatId) {
            throw new Error('chatId is required for getCurrentTemperature');
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            throw new Error(`Chat ${chatId} not found`);
        }
        
        return chat.config.model.params.temperature;
    }
    
    // Handle generate title button click
    async handleGenerateTitleClick(chatId) {
        if (!chatId) {
            this.showError('Please select or create a chat first', chatId);
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Check if there are any messages to generate a title from
        const userMessages = chat.messages.filter(m => m.role === 'user');
        const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
        
        if (userMessages.length === 0 || assistantMessages.length === 0) {
            this.showError('Need at least one user message and one assistant response to generate a title', chatId);
            return;
        }
        
        // Disable the button to prevent multiple clicks
        this.generateTitleBtn.disabled = true;
        
        try {
            // Get MCP connection
            const mcpConnection = await this.ensureMcpConnection(chat.mcpServerId);
            
            // Get proxy provider configuration
            const proxyProvider = this.llmProviders.get(chat.llmProviderId);
            if (!proxyProvider) {
                throw new Error('LLM provider not found');
            }
            
            // Get the appropriate provider for title generation
            const titleProvider = TitleGenerator.getTitleGenerationProvider(
                chat,
                proxyProvider,
                null, // No default provider, will be created
                createLLMProvider
            );
            
            // If no title provider (no model configured), create one from chat's primary model
            const provider = titleProvider || (() => {
                const providerType = chat.config?.model?.provider;
                const modelName = chat.config?.model?.id;
                if (!providerType || !modelName) {
                    throw new Error('Invalid model configuration in chat');
                }
                // Get the API type from the provider configuration
                const providerApiType = proxyProvider.availableProviders?.[providerType]?.type || providerType;
                const p = createLLMProvider(providerApiType, proxyProvider.proxyUrl, modelName, chat.config.model);
                p.onLog = (logEntry) => {
                    const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
                    const providerName = providerType.charAt(0).toUpperCase() + providerType.slice(1);
                    this.addLogEntry(`${prefix}: ${providerName}`, logEntry);
                };
                return p;
            })();
            
            // Generate the title (force=true for manual generation)
            await TitleGenerator.generateChatTitle(
                chat, 
                mcpConnection, 
                provider, 
                false, 
                true,
                this.getTitleGenerationCallbacks()
            );
        } catch (error) {
            this.showError(`Failed to generate title: ${error.message}`, chatId);
        } finally {
            this.generateTitleBtn.disabled = false;
        }
    }
    
    // Unified summary generation method
    async generateChatSummary(chat, mcpConnection, provider, isAutomatic = false) {
        try {
            // User request that clearly explains the purpose
            const summaryRequest = 'Please create a comprehensive summary of our discussion that I can save and later provide back to you so we can continue from where we left off. Include all the context, findings, and details needed to resume our conversation.';
            
            // Use centralized system prompt for summaries
            const summarySystemPrompt = SystemMsg.createSpecializedSystemPrompt('summary');
            
            // CRITICAL: Save summary request BEFORE displaying
            this.addMessage(chat.id, { 
                role: 'system-summary',
                content: summaryRequest,
                timestamp: new Date().toISOString()
            });
            
            // Now display it
            this.processRenderEvent({ type: 'system-summary-message', content: summaryRequest }, chat.id);
            
            // Show thinking spinner
            this.showAssistantThinking(chat.id);
            
            // Build conversational messages (no tools) for summary generation
            // Don't include the original system prompt - we'll use a custom one
            const conversationalMessages = this.buildConversationalMessages(chat.messages, false);
            
            // Create messages array with custom summary system prompt
            const messages = [{
                role: 'system',
                content: summarySystemPrompt
            }];
            
            // Add all conversational messages
            messages.push(...conversationalMessages);
            
            // IMPORTANT: Ensure the conversation ends properly for the API
            // If the last message is from the user (orphaned due to MAX_TOKENS or other error),
            // we need to handle it specially
            if (messages.length > 1 && messages[messages.length - 1].role === 'user') {
                // Add a placeholder assistant message to complete the conversation
                messages.push({
                    role: 'assistant',
                    content: '[Response truncated due to token limit]'
                });
            }
            
            // IMPORTANT: The summary request should be added AFTER building messages
            // but BEFORE the system-summary message is added to chat history
            // This way it's not included in the conversation being summarized
            
            // Add the simple summary request as a user message
            messages.push({ role: 'user', content: summaryRequest });
            
            // For summaries, we can use a simple cache control on the last conversational message
            const cacheControlIndex = messages.length - 2; // Before the summary request
            
            // Send request with low temperature for consistent summaries
            const temperature = 0.5;
            const response = await this.callAssistant({
                chatId: chat.id,
                provider,
                messages,
                tools: [],
                temperature,
                cacheControlIndex,
                context: 'Summary request'
            });
            
            // Check if rate limit was handled automatically
            if (response._rateLimitHandled) {
                return { rateLimitHandled: true };
            }
            
            // Extract response time from the response object
            const llmResponseTime = response._responseTime || 0;
            
            // Update metrics
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, ChatConfig.getChatModelString(chat) || provider.model);
            }
            
            // Process the summary response
            if (response.content) {
                // Use the full model string from chat config or construct it with provider prefix
                const fullModelString = ChatConfig.getChatModelString(chat) || 
                                       (provider.type ? `${provider.type}:${provider.model}` : provider.model);
                
                // CRITICAL: Save summary response BEFORE displaying
                this.addMessage(chat.id, { 
                    role: 'summary', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: fullModelString,
                    timestamp: new Date().toISOString(),
                    cacheControlIndex // Store the frozen cache position
                });
                
                // Display the response as a summary message WITH metrics
                this.processRenderEvent({ 
                    type: 'summary-message', 
                    content: response.content,
                    usage: response.usage,
                    responseTime: llmResponseTime,
                    model: fullModelString
                }, chat.id);
                
                // Update context window display
                this.updateContextWindowIndicator(chat.id);
                
                // Update all token displays including cost
                this.updateAllTokenDisplays(chat.id);
                
                // Mark that summary was generated
                chat.summaryGenerated = true;
                if (!isAutomatic) {
                    this.saveChatToStorage(chat.id);
                }
            }
            
        } catch (error) {
            console.error('Failed to summarize conversation:', error);
            
            // Hide spinner on error
            this.hideAssistantThinking(chat.id);
            
            if (!isAutomatic) {
                this.showError(`Failed to summarize: ${error.message}`, chat.id);
            }
            
            // Remove the system-summary message if it failed
            const lastMsg = chat.messages[chat.messages.length - 1];
            if (lastMsg && lastMsg.role === 'system-summary') {
                this.removeLastMessage(chat.id);
            }
            
            throw error; // Re-throw for caller to handle
        }
    }
    
    // Manual summary button handler
    async summarizeConversation(chatId) {
        if (!chatId) {
            this.showError('No chat ID provided for summarization', null);
            return;
        }
        
        const chat = this.chats.get(chatId);
        if (!chat) {
            this.showError('No active chat to summarize', chatId);
            return;
        }
        
        // Check if there are any user messages to summarize
        const userMessages = chat.messages.filter(m => m.role === 'user' && !['system-title', 'system-summary'].includes(m.role));
        const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
        if (userMessages.length === 0 || assistantMessages.length === 0) {
            this.showError('Need at least one complete conversation exchange to summarize', chatId);
            return;
        }
        
        // Disable button to prevent multiple requests
        this.summarizeBtn.disabled = true;
        this.summarizeBtn.innerHTML = '<span><i class="fas fa-hourglass-half"></i></span><span>Summarizing...</span>';
        
        try {
            // Get MCP connection and LLM provider
            const mcpConnection = await this.ensureMcpConnection(chat.mcpServerId);
            const llmProviderConfig = this.llmProviders.get(chat.llmProviderId);
            
            // Get model from config
            const providerType = chat.config?.model?.provider;
            const modelName = chat.config?.model?.id;
            if (!providerType || !modelName) {
                throw new Error('Invalid model configuration in chat');
            }
            
            // Get the API type from the provider configuration
            const providerApiType = llmProviderConfig.availableProviders?.[providerType]?.type || providerType;
            
            const provider = window.createLLMProvider(
                providerApiType,
                llmProviderConfig.proxyUrl,
                modelName,
                chat.config.model
            );
            provider.onLog = (logEntry) => {
                const prefix = logEntry.direction === 'sent' ? 'llm-request' : 'llm-response';
                const providerName = providerType.charAt(0).toUpperCase() + providerType.slice(1);
                this.addLogEntry(`${prefix}: ${providerName}`, logEntry);
            };
            
            // Use the unified method
            await this.generateChatSummary(chat, mcpConnection, provider, false);
            
        } catch (error) {
            console.error('Failed to summarize conversation:', error);
            
            // Hide spinner on error
            this.hideAssistantThinking(chatId);
            
            this.showError(`Failed to summarize: ${error.message}`, chatId);
            
            // Remove the system-summary message if it failed
            const lastMsg = chat.messages[chat.messages.length - 1];
            if (lastMsg && lastMsg.role === 'system-summary') {
                this.removeLastMessage(chat.id);
            }
        } finally {
            // Re-enable button
            this.summarizeBtn.disabled = false;
            this.summarizeBtn.innerHTML = '<span><i class="fas fa-compress-alt"></i></span><span>Summarize</span>';
        }
    }
    
    // Delete summary messages and replace with accounting record
    deleteSummaryMessages(messageIndex, chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        // Find the system-summary message at or near the given index
        let systemSummaryIndex = -1;
        let summaryIndex = -1;
        
        // Look for system-summary message around the given index
        for (let i = Math.max(0, messageIndex - 1); i < Math.min(chat.messages.length, messageIndex + 2); i++) {
            if (chat.messages[i]?.role === 'system-summary') {
                systemSummaryIndex = i;
                break;
            }
        }
        
        if (systemSummaryIndex === -1) {
            console.error('Could not find system-summary message near index', messageIndex);
            return;
        }
        
        // Look for the corresponding summary response after the system-summary
        for (let i = systemSummaryIndex + 1; i < chat.messages.length; i++) {
            if (chat.messages[i]?.role === 'summary') {
                summaryIndex = i;
                break;
            }
        }
        
        // Extract tokens from the summary message's usage data
        const summaryTokens = {
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
            cacheCreationTokens: 0
        };
        let summaryModel = null;
        
        if (summaryIndex !== -1) {
            const summaryMsg = chat.messages[summaryIndex];
            if (summaryMsg.usage) {
                summaryTokens.inputTokens = summaryMsg.usage.promptTokens || 0;
                summaryTokens.outputTokens = summaryMsg.usage.completionTokens || 0;
                summaryTokens.cacheReadTokens = summaryMsg.usage.cacheReadInputTokens || 0;
                summaryTokens.cacheCreationTokens = summaryMsg.usage.cacheCreationInputTokens || 0;
            }
            // Get the model used for the summary
            summaryModel = summaryMsg.model || ChatConfig.getChatModelString(chat);
        }
        
        // Count messages to be deleted
        let messagesToDelete = 1; // At least the system-summary
        if (summaryIndex !== -1) {
            messagesToDelete = 2; // Both system-summary and summary
        }
        
        // Create accounting node with the tokens that were spent on the summary
        const accountingNode = {
            role: 'accounting',
            timestamp: new Date().toISOString(),
            cumulativeTokens: summaryTokens,
            reason: 'Summary deleted',
            discardedMessages: messagesToDelete,
            model: summaryModel
        };
        
        // Delete messages and insert accounting node
        if (summaryIndex !== -1) {
            // Delete both messages, starting from the later index
            this.removeMessage(chat.id, summaryIndex, 1);
            this.removeMessage(chat.id, systemSummaryIndex, 1);
        } else {
            // Delete only system-summary
            this.removeMessage(chat.id, systemSummaryIndex, 1);
        }
        
        // Insert accounting node at the position of the deleted system-summary
        this.insertMessage(chat.id, systemSummaryIndex, accountingNode);
        
        // Mark that summary is no longer generated
        chat.summaryGenerated = false;
        this.saveChatToStorage(chat.id);
        
        // Reload the chat to update the display (force refresh after deleting messages)
        this.loadChat(chatId, true);
    }
    
    // Check if automatic summary should be generated
    shouldGenerateSummary(chat) {
        // Check if auto-summarization is enabled
        if (!chat.config?.optimisation?.autoSummarisation?.enabled) {
            return false;
        }
        
        // Don't summarize if we recently created a summary (within 10 minutes)
        const recentSummary = chat.messages.findLast(m => m.role === 'system-summary');
        if (recentSummary) {
            const summaryAge = Date.now() - new Date(recentSummary.timestamp).getTime();
            if (summaryAge < 10 * 60 * 1000) {
                console.log('[Auto-summarize] Skipping - recent summary exists', {
                    summaryAge: Math.round(summaryAge / 1000 / 60) + ' minutes'
                });
                return false;
            }
        }
        
        // Need at least a few exchanges before summarizing
        const userMessages = chat.messages.filter(m => m.role === 'user');
        const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
        if (userMessages.length < 3 || assistantMessages.length < 3) {
            return false;
        }
        
        // Calculate current context window usage
        const contextTokens = this.calculateContextWindowTokens(chat.id);
        const effectiveLimit = this.getEffectiveContextWindow(chat);
        
        const percentUsed = Math.round((contextTokens / effectiveLimit) * 100);
        const triggerPercent = chat.config.optimisation.autoSummarisation.triggerPercent || 50;
        
        console.log('[Auto-summarize] Context check', {
            contextTokens,
            effectiveLimit,
            percentUsed: percentUsed + '%',
            triggerPercent: triggerPercent + '%',
            willTrigger: percentUsed >= triggerPercent
        });
        
        return percentUsed >= triggerPercent;
    }
    
    
    // Simple helper methods for assistant group management
    setCurrentAssistantGroup(chatId, groupElement) {
        const chat = this.chats.get(chatId);
        if (chat) {
            chat.currentAssistantGroup = groupElement;
        }
    }
    
    getCurrentAssistantGroup(chatId) {
        const chat = this.chats.get(chatId);
        return chat ? chat.currentAssistantGroup : null;
    }
    
    clearCurrentAssistantGroup(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            console.error(`[clearCurrentAssistantGroup] Chat not found for chatId: ${chatId}`);
            return;
        }
        chat.currentAssistantGroup = null;
    }
    
    // Get callbacks for title generation
    getTitleGenerationCallbacks() {
        return {
            addMessage: (chatId, message) => this.addMessage(chatId, message),
            processRenderEvent: (event, chatId) => this.processRenderEvent(event, chatId),
            updateChatSessions: () => this.updateChatSessions(),
            updateChatTitle: (chatId, title) => {
                const container = this.getChatContainer(chatId);
                if (container && container._elements && container._elements.title) {
                    container._elements.title.textContent = title;
                }
            },
            saveChatToStorage: (chatId) => this.saveChatToStorage(chatId),
            showError: (message, chatId) => this.showError(message, chatId),
            removeLastMessage: (chatId) => this.removeLastMessage(chatId),
            clearCurrentAssistantGroup: (chatId) => this.clearCurrentAssistantGroup(chatId)
        };
    }
    
    // Reset global state when switching chats
    resetGlobalChatState() {
        // isProcessing is now per-chat, no need to reset globally
        // shouldStopProcessing is now per-chat, no need to reset globally
        
        // currentContextWindow is now per-chat, no need to reset globally
        
        // Clear UI state
        if (this.spinnerInterval) {
            clearInterval(this.spinnerInterval);
            this.spinnerInterval = null;
        }
        
        // Note: pendingToolCalls are now per-chat, so they don't need global cleanup
        
        // Clear selection state
        this.userHasSelectedChat = false;
    }
}

// Initialize the application
document.addEventListener('DOMContentLoaded', () => {
    window.app = new NetdataMCPChat();
    
    // Expose migration function for testing
    window.migrateCurrentChat = () => {
        const activeChatId = window.app.getActiveChatId();
        if (!activeChatId) {
            return;
        }
        const chat = window.app.chats.get(activeChatId);
        if (!chat) {
            return;
        }
        window.app.migrateTokenPricing(chat);
        // Update displays
        window.app.updateAllTokenDisplays(activeChatId);
    };
});

