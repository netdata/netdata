/**
 * Main application logic for the Netdata MCP LLM Client
 */

import {MessageOptimizer} from './message-optimizer.js';
import * as ChatConfig from './chat-config.js';
import * as TitleGenerator from './title.js';
import * as SystemMsg from './system-msg.js';
import {SafetyChecker, SafetyLimitError} from './safety-limits.js';

class NetdataMCPChat {
    constructor() {
        this.mcpServers = new Map(); // Multiple MCP servers
        this.mcpConnections = new Map(); // Active MCP connections
        this.llmProviders = new Map(); // Multiple LLM providers
        this.chats = new Map(); // Chat sessions
        this.communicationLog = []; // Universal log (not saved)
        this.tokenUsageHistory = new Map(); // Track token usage per chat
        this.toolInclusionStates = new Map(); // Track which tools are included/excluded per chat
        this.currentContextWindow = 0; // Running total for delta calculation during rendering
        this.shouldStopProcessing = false; // Flag to stop processing between requests
        this.isProcessing = false; // Track if we're currently processing messages
        this.modelPricing = {}; // Initialize model pricing storage
        this.modelLimits = {}; // Initialize model context limits storage
        this.copiedModel = null; // Track copied model for paste functionality
        
        // Safety protections
        this.safetyChecker = new SafetyChecker();
        
        // Per-chat DOM management
        this.chatContainers = new Map(); // Map of chatId -> DOM container

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
        
        // Initialize providers and then create default chat
        Promise.all([
            this.initializeDefaultLLMProvider(),
            this.initializeDefaultMCPServer()
        ]).then(async () => {
            // Update chat sessions after providers are loaded
            this.updateChatSessions();
            
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
                        this.loadChat(newChatId);
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
        
        chat.messages.pop();
        chat.updatedAt = new Date().toISOString();
        
        // Update cumulative token pricing
        this.updateChatTokenPricing(chat);
        
        // Update the cumulative token display
        this.updateCumulativeTokenDisplay(chatId);
        
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
        this.chatInput = null;
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
        if (!chat) {return;}
        
        const provider = this.llmProviders.get(chat.llmProviderId);
        if (!provider || !provider.availableProviders) {return;}
        
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
            
            <div style="display: flex; align-items: center; gap: 4px; margin-left: auto;">
                <label style="font-size: 12px; color: var(--text-secondary);">max output tokens:</label>
                <select id="maxTokens_${chatId}" 
                        style="padding: 2px 6px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); font-size: 12px;">
                    <option value="1024" ${currentMaxTokens === 1024 ? 'selected' : ''}>1k</option>
                    <option value="2048" ${currentMaxTokens === 2048 ? 'selected' : ''}>2k</option>
                    <option value="4096" ${currentMaxTokens === 4096 ? 'selected' : ''}>4k</option>
                    <option value="8192" ${currentMaxTokens === 8192 ? 'selected' : ''}>8k</option>
                    <option value="16384" ${currentMaxTokens === 16384 ? 'selected' : ''}>16k</option>
                    <option value="32768" ${currentMaxTokens === 32768 ? 'selected' : ''}>32k</option>
                    <option value="65536" ${currentMaxTokens === 65536 ? 'selected' : ''}>64k</option>
                    <option value="131072" ${currentMaxTokens === 131072 ? 'selected' : ''}>128k</option>
                </select>
            </div>
        `;
        section.appendChild(chatModelDiv);
        
        // Tool Summarization Option
        const toolSumDiv = document.createElement('div');
        const isEnabled = config.optimisation.toolSummarisation.enabled;
        toolSumDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!isEnabled ? 'opacity: 0.5;' : ''}`;
        
        const currentThreshold = config.optimisation.toolSummarisation.thresholdKiB || 20; // Default 20KB
        const toolSumModel = ChatConfig.modelConfigToString(config.optimisation.toolSummarisation.model) || ChatConfig.getChatModelString(chat);
        
        toolSumDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="toolSummarization_${chatId}" ${isEnabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Summarize tool responses of at least</span>
            </label>
            <select id="toolThreshold_${chatId}" 
                    style="width: 70px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;"
                    ${!isEnabled ? 'disabled' : ''}>
                <option value="0">0 (all)</option>
                <option value="5">5</option>
                <option value="10">10</option>
                <option value="20" ${currentThreshold === 20 ? 'selected' : ''}>20</option>
                <option value="30">30</option>
                <option value="40">40</option>
                <option value="50">50</option>
                <option value="60">60</option>
                <option value="70">70</option>
                <option value="80">80</option>
                <option value="90">90</option>
                <option value="100">100</option>
            </select>
            <span>KiB size, with</span>
            <div class="model-select-wrapper" style="position: relative; display: inline-block;">
                <button class="model-select-btn" id="toolSumModel_${chatId}" 
                        style="padding: 2px 8px; border: 1px solid var(--border-color); 
                               border-radius: 4px; background: var(--background-color); 
                               color: var(--text-primary); cursor: pointer;
                               display: flex; align-items: center; gap: 4px;"
                        ${!isEnabled ? 'disabled' : ''}>
                    <span class="model-name">${toolSumModel || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
        `;
        
        section.appendChild(toolSumDiv);
        
        // Auto-summarization Option (second)
        const autoSumDiv = document.createElement('div');
        const autoSumEnabled = config.optimisation.autoSummarisation.enabled;
        autoSumDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!autoSumEnabled ? 'opacity: 0.5;' : ''}`;
        
        const currentPercent = config.optimisation.autoSummarisation.triggerPercent || 50;
        const autoSumModel = ChatConfig.modelConfigToString(config.optimisation.autoSummarisation.model) || ChatConfig.getChatModelString(chat);
        
        autoSumDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: pointer;">
                <input type="checkbox" id="autoSummarization_${chatId}" ${autoSumEnabled ? 'checked' : ''}
                       style="margin-right: 6px;">
                <span>Summarize conversation when context window above</span>
            </label>
            <select id="autoSumThreshold_${chatId}" 
                    style="width: 70px; padding: 2px 4px; border: 1px solid var(--border-color); 
                           border-radius: 4px; background: var(--background-color); color: var(--text-primary);
                           cursor: pointer;"
                    ${!autoSumEnabled ? 'disabled' : ''}>
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
                               display: flex; align-items: center; gap: 4px;"
                        ${!autoSumEnabled ? 'disabled' : ''}>
                    <span class="model-name">${autoSumModel || 'Select model'}</span>
                    <i class="fas fa-chevron-down" style="font-size: 10px;"></i>
                </button>
            </div>
        `;
        
        section.appendChild(autoSumDiv);
        
        // Title Generation Option
        const titleGenDiv = document.createElement('div');
        const titleGenEnabled = config.optimisation.titleGeneration?.enabled !== false; // Default to true
        titleGenDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!titleGenEnabled ? 'opacity: 0.5;' : ''}`;
        
        const titleGenModel = ChatConfig.modelConfigToString(config.optimisation.titleGeneration?.model);
        
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
        const toolMemoryEnabled = config.optimisation.toolMemory.enabled;
        toolMemoryDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${!toolMemoryEnabled ? 'opacity: 0.5;' : ''}`;
        
        const forgetAfterConclusions = config.optimisation.toolMemory.forgetAfterConclusions;
        
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
        
        // Cache Control Option (only for Anthropic provider)
        const isAnthropicProvider = config.model && config.model.provider === 'anthropic';
        const cacheControlDiv = document.createElement('div');
        const cacheControlEnabled = config.optimisation.cacheControl.enabled;
        const cacheControlDisabled = !isAnthropicProvider || toolMemoryEnabled;
        cacheControlDiv.style.cssText = `display: flex; align-items: center; gap: 8px; margin-bottom: 8px; ${cacheControlDisabled ? 'opacity: 0.5;' : ''}`;
        
        cacheControlDiv.innerHTML = `
            <label style="display: flex; align-items: center; cursor: ${cacheControlDisabled ? 'default' : 'pointer'};">
                <input type="checkbox" id="cacheControl_${chatId}" ${cacheControlEnabled ? 'checked' : ''}
                       style="margin-right: 6px;"
                       ${cacheControlDisabled ? 'disabled' : ''}>
                <span>Enable Anthropic's cache control${toolMemoryEnabled ? ' (disabled: tool memory is on)' : ''}</span>
            </label>
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
            ChatConfig.saveChatConfig(chatId, chat.config);
            this.autoSave(chatId);
        });
        
        topPSlider.addEventListener('input', (e) => {
            const value = parseFloat(e.target.value);
            topPValueLabel.textContent = value.toFixed(2);
        });
        
        topPSlider.addEventListener('change', (e) => {
            chat.config.model.params.topP = parseFloat(e.target.value);
            ChatConfig.saveChatConfig(chatId, chat.config);
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
            const kbValue = parseInt(e.target.value, 10) || 20;
            const byteValue = kbValue * 1024;
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
            
            // Update cache control state for Anthropic (mutually exclusive with tool memory)
            if (isAnthropicProvider) {
                const cacheControlCheckbox = section.querySelector(`#cacheControl_${chatId}`);
                const cacheControlLabel = cacheControlCheckbox.closest('label');
                const cacheControlSpan = cacheControlLabel.querySelector('span');
                
                if (enabled) {
                    // Disable cache control when tool memory is enabled
                    cacheControlCheckbox.disabled = true;
                    cacheControlCheckbox.closest('div').style.opacity = '0.5';
                    cacheControlSpan.textContent = 'Enable Anthropic\'s cache control (disabled: tool memory is on)';
                    if (cacheControlCheckbox.checked) {
                        cacheControlCheckbox.checked = false;
                        this.updateOptimizationSetting(chatId, 'cacheControl', false);
                    }
                } else {
                    // Re-enable cache control when tool memory is disabled
                    cacheControlCheckbox.disabled = false;
                    cacheControlCheckbox.closest('div').style.opacity = '1';
                    cacheControlSpan.textContent = 'Enable Anthropic\'s cache control';
                }
            }
            
            this.updateOptimizationSetting(chatId, 'toolMemory', enabled);
        });
        
        toolMemorySelect.addEventListener('change', (e) => {
            e.stopPropagation();
            const newForgetAfterConclusions = parseInt(e.target.value, 10);
            this.updateToolMemoryThreshold(chatId, newForgetAfterConclusions);
        });
        
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
                            ${status(config.optimisation.cacheControl.enabled)} 
                            ${config.optimisation.cacheControl.enabled ? `Strategy: ${config.optimisation.cacheControl.strategy}` : 'Disabled'}
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
                                    contextWindow: contextWindow || 128000, // Default context
                                    pricing: pricing || null
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
                const hasModel = modelName && modelName !== 'Select model';
                
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
                        this.copiedModel = modelName;
                        document.body.removeChild(menu);
                        this.showToast(`Copied model: ${modelName}`, 'success-toast');
                    });
                    menu.appendChild(copyItem);
                }
                
                if (this.copiedModel && this.copiedModel !== modelName) {
                    const pasteItem = document.createElement('div');
                    pasteItem.style.cssText = `
                        padding: 6px 12px;
                        cursor: pointer;
                        font-size: 13px;
                    `;
                    pasteItem.textContent = `Paste "${this.copiedModel}"`;
                    pasteItem.addEventListener('mouseenter', () => {
                        pasteItem.style.background = 'var(--hover-color)';
                    });
                    pasteItem.addEventListener('mouseleave', () => {
                        pasteItem.style.background = '';
                    });
                    pasteItem.addEventListener('click', () => {
                        button.querySelector('.model-name').textContent = this.copiedModel;
                        onSelect(this.copiedModel);
                        document.body.removeChild(menu);
                        this.showToast(`Pasted model: ${this.copiedModel}`, 'success-toast');
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
                    if (!menu.contains(evt.target)) {
                        document.body.removeChild(menu);
                        document.removeEventListener('click', removeMenu);
                    }
                };
                setTimeout(() => {
                    document.addEventListener('click', removeMenu);
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
                    tr.style.cssText = `
                        cursor: pointer;
                        transition: background 0.1s;
                        border-bottom: 1px solid var(--border-subtle, var(--border-color));
                    `;
                    tr.addEventListener('mouseenter', () => {
                        tr.style.background = 'var(--hover-color)';
                    });
                    tr.addEventListener('mouseleave', () => {
                        tr.style.background = '';
                    });
                    
                    // Capture the full model string in the closure
                    const fullModelId = `${model.providerId}:${model.id}`;
                    
                    // Add click handler directly here
                    tr.addEventListener('click', () => {
                        const modelName = ChatConfig.getModelDisplayName(fullModelId);
                        button.querySelector('.model-name').textContent = modelName;
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
                
                // Add search functionality
                searchInput.addEventListener('input', (e) => {
                    const searchTerm = e.target.value.toLowerCase().trim();
                    
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
                    }
                });
                
                // Handle keyboard navigation
                searchInput.addEventListener('keydown', (e) => {
                    if (e.key === 'Escape') {
                        dropdown.remove();
                        button.removeAttribute('data-dropdown-open');
                    } else if (e.key === 'ArrowDown') {
                        e.preventDefault();
                        const firstRow = tbody.querySelector('tr[style*="cursor: pointer"]');
                        if (firstRow) {
                            firstRow.focus();
                            firstRow.style.background = 'var(--hover-color)';
                        }
                    }
                });
                
                // Define functions before they're used
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
        if (!chat) return;

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
            case 'cacheControl':
                config.optimisation.cacheControl.enabled = enabled;
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
        ChatConfig.saveChatConfig(chatId, config);
        
        // Auto-save chat
        this.autoSave(chatId);
    }

    
    updateChatModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        // Store the old model before updating
        const oldModelString = ChatConfig.modelConfigToString(chat.config?.model);
        
        // Update config
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        const modelConfig = ChatConfig.modelConfigFromString(model);
        if (modelConfig) {
            // Preserve existing params
            modelConfig.params = config.model?.params || modelConfig.params;
            config.model = modelConfig;
        }
        
        // Update models in optimization features if they were using the old model
        if (oldModelString && config.optimisation.toolSummarisation.model && 
            ChatConfig.modelConfigToString(config.optimisation.toolSummarisation.model) === oldModelString) {
            config.optimisation.toolSummarisation.model = { ...config.model };
        }
        if (oldModelString && config.optimisation.autoSummarisation.model && 
            ChatConfig.modelConfigToString(config.optimisation.autoSummarisation.model) === oldModelString) {
            config.optimisation.autoSummarisation.model = { ...config.model };
        }
        if (oldModelString && config.optimisation.titleGeneration.model && 
            ChatConfig.modelConfigToString(config.optimisation.titleGeneration.model) === oldModelString) {
            config.optimisation.titleGeneration.model = { ...config.model };
        }
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
        
        // Update displays
        this.updateChatHeader(chatId);
        this.updateChatSessions();
    }
    
    updateToolSummarizationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.toolSummarisation.model = ChatConfig.modelConfigFromString(model);
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
    }
    
    updateAutoSummarizationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.autoSummarisation.model = ChatConfig.modelConfigFromString(model);
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
    }
    
    updateTitleGenerationModel(chatId, model) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.titleGeneration.model = ChatConfig.modelConfigFromString(model);
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
    }
    
    updateToolThreshold(chatId, threshold) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.toolSummarisation.thresholdKiB = Math.floor(threshold / 1024);
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
    }
    
    updateAutoSumThreshold(chatId, percent) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.autoSummarisation.triggerPercent = percent;
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
        this.autoSave(chatId);
    }
    
    updateToolMemoryThreshold(chatId, forgetAfterConclusions) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        const config = chat.config || ChatConfig.loadChatConfig(chatId);
        config.optimisation.toolMemory.forgetAfterConclusions = forgetAfterConclusions;
        
        chat.config = config;
        this.recreateMessageOptimizer(chat, config);
        ChatConfig.saveChatConfig(chatId, config);
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
        
        for (const [id, server] of this.mcpServers) {
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
                    this.showError('Failed to switch MCP server', targetChatId);
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
            this.showError(`Failed to switch MCP server: ${error.message}`, chatId);
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

    showError(message, chatId) {
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
        
        entryDiv.innerHTML = `
            <div class="log-entry-header">
                <div class="log-entry-info">
                    <span class="log-timestamp">${new Date(entry.timestamp).toLocaleTimeString()}</span>
                    <span class="log-source">[${entry.source}]</span>
                    <span class="log-direction ${directionClass}">${directionSymbol}</span>
                </div>
                <button class="btn-copy-log" data-tooltip="Copy to clipboard" data-entry-id="${entryId}"><i class="fas fa-clipboard"></i></button>
            </div>
            ${metadataHtml}
            <div class="log-message" id="${entryId}">${this.formatLogMessage(entry.message)}</div>
        `;
        
        // Add click handler for copy button
        const copyBtn = entryDiv.querySelector('.btn-copy-log');
        copyBtn.addEventListener('click', () => {
            const messageElement = document.getElementById(entryId);
            const textToCopy = messageElement.textContent || messageElement.innerText;
            this.copyToClipboard(textToCopy, copyBtn).catch(error => {
                console.error('Failed to copy to clipboard:', error);
            });
        });
        
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

    async copyToClipboard(text, button) {
        try {
            await navigator.clipboard.writeText(text);
            
            // Show success feedback
            const originalText = button.textContent;
            button.innerHTML = '<i class="fas fa-check"></i>';
            button.style.color = 'var(--success-color)';
            
            setTimeout(() => {
                button.textContent = originalText;
                button.style.color = '';
            }, 1500);
        } catch (err) {
            console.error('Failed to copy to clipboard:', err);
            
            // Show error feedback
            const originalText = button.textContent;
            button.textContent = '✗';
            button.style.color = 'var(--danger-color)';
            
            setTimeout(() => {
                button.textContent = originalText;
                button.style.color = '';
            }, 1500);
        }
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
        
        // Create the LLM provider
        const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
        provider.onLog = proxyProvider.onLog;
        
        try {
            if (message.type === 'user') {
                // Redo from user message - truncate everything AFTER this message
                // Keep all history up to and including this message
                // noinspection JSDeprecatedSymbols (false positive - slice is not deprecated)
                chat.messages = chat.messages.slice(0, messageIndex + 1);
                this.autoSave(chat.id);
                this.loadChat(chatId, true);
                
                // Show loading spinner AFTER loadChat to prevent it from being cleared
                this.processRenderEvent({ type: 'show-spinner' }, chatId);
                
                // Get fresh chat object after loadChat
                const freshChat = this.chats.get(chatId);
                if (!freshChat) {
                    throw new Error('Chat not found after reload');
                }
                
                // Resend the user message with full prior context
                await this.processMessageWithTools(freshChat, mcpConnection, provider, message.content);
            } else if (message.type === 'assistant') {
                // Redo from assistant message - find the user message that triggered it
                let triggeringUserMessage = null;
                let triggeringUserIndex = -1;
                
                // Find the most recent user message before this assistant message
                for (let i = messageIndex - 1; i >= 0; i--) {
                    if (chat.messages[i].role === 'user') {
                        triggeringUserMessage = chat.messages[i];
                        triggeringUserIndex = i;
                        break;
                    }
                }
                
                if (triggeringUserMessage) {
                    // Truncate to remove this assistant message and everything after
                    // Keep everything up to the triggering user message
                    // noinspection JSDeprecatedSymbols (false positive - slice is not deprecated)
                    chat.messages = chat.messages.slice(0, triggeringUserIndex + 1);
                    this.autoSave(chat.id);
                    this.loadChat(chatId, true);
                    
                    // Show loading spinner AFTER loadChat to prevent it from being cleared
                    this.processRenderEvent({ type: 'show-spinner' }, chatId);
                    
                    // Get fresh chat object after loadChat
                    const freshChat = this.chats.get(chatId);
                    if (!freshChat) {
                        throw new Error('Chat not found after reload');
                    }
                    
                    // Resend the triggering user message with full prior context
                    await this.processMessageWithTools(freshChat, mcpConnection, provider, triggeringUserMessage.content);
                } else {
                    this.showError('Cannot find the user message that triggered this response', chatId);
                }
            } else {
                // For any other message type, show error (shouldn't happen with our button logic)
                this.showError('Redo is only available for user and assistant messages', chatId);
            }
        } catch (error) {
            this.showError(`Redo failed: ${error.message}`, chatId);
        } finally {
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' }, chatId);
        }
    }

    clearLog() {
        if (confirm('Clear all communication logs?')) {
            this.communicationLog = [];
            this.logContent.innerHTML = '';
        }
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
        const savedTheme = localStorage.getItem('theme') || 'light';
        document.documentElement.setAttribute('data-theme', savedTheme);
        
        // Load log collapsed state
        const logCollapsed = localStorage.getItem('logCollapsed') === 'true';
        if (logCollapsed) {
            this.logPanel.classList.add('collapsed');
            this.toggleLogBtn.innerHTML = '<i class="fas fa-chevron-left"></i>';
            this.expandLogBtn.style.display = 'block';
        }
        
        // Load pane sizes
        this.loadPaneSizes();
        
        // Load MCP servers
        const savedMcpServers = localStorage.getItem('mcpServers');
        if (savedMcpServers) {
            try {
                const servers = JSON.parse(savedMcpServers);
                servers.forEach(server => {
                    this.mcpServers.set(server.id, server);
                });
                this.updateMcpServersList();
            } catch (e) {
                console.error('Failed to load MCP servers:', e);
            }
        }
        
        // LLM providers will be fetched from proxy on demand
        
        // Load chats - first try split storage, then fall back to legacy
        this.loadChatsFromStorage();
    }

    loadChatsFromStorage() {
        // Load from split storage (mcp_chat_* keys)
        const chatKeyPrefix = 'mcp_chat_';
        
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
        if (!ChatConfig.validateConfig(chat.config)) {
            console.error(`Chat ${chat.id} has invalid config, creating default`);
            chat.config = ChatConfig.createDefaultConfig();
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
        
        // Ensure currentAssistantGroup exists for loaded chats
        if (!Object.prototype.hasOwnProperty.call(chat, 'currentAssistantGroup')) {
            chat.currentAssistantGroup = null;
        }
        
        // Ensure pendingToolCalls is a Map (it gets serialized as {} in localStorage)
        if (!chat.pendingToolCalls || !(chat.pendingToolCalls instanceof Map)) {
            chat.pendingToolCalls = new Map();
        }
        
        // Check if the chat was saved while waiting for a response (broken state)
        if (chat.waitingForLLM || chat.waitingForMCP) {
            chat.wasWaitingOnLoad = true;
            // Clear the waiting states since we're not actually waiting anymore
            chat.waitingForLLM = false;
            chat.waitingForMCP = false;
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
        
        // Add to memory - DO NOT SAVE!
        // The chat is now in the correct format in memory
        // It will only be saved when the user actually modifies it
        this.chats.set(chat.id, chat);
    }
    
    async initializeDefaultLLMProvider() {
        // Always fetch models even if we have providers (to get fresh model list)

        // Auto-detect the proxy URL from the current origin
        const proxyUrl = window.location.origin;
        
        // console.log('Fetching models from:', `${proxyUrl}/models`);
        
        try {
            // Fetch available models from the same origin
            const response = await fetch(`${proxyUrl}/models`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const data = await response.json();
            const providers = data.providers || {};
            
            // console.log('Received providers data from proxy:', providers);
            
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
                    name: 'Local LLM Proxy',
                    proxyUrl,
                    availableProviders: providers,
                    onLog: (logEntry) => this.addLogEntry('Local LLM Proxy', logEntry)
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
            mcpServerId = this.mcpServers.keys().next().value;
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

    async initializeDefaultMCPServer() {
        // Check if we already have MCP servers or if the default one exists
        const defaultUrl = 'ws://localhost:19999/mcp';
        const defaultName = 'Local Netdata';
        
        // Check if this URL already exists
        for (const [, server] of this.mcpServers) {
            if (server.url === defaultUrl) {
                // Already exists, no need to add
                return;
            }
        }
        
        // If we have no MCP servers, add the default one
        if (this.mcpServers.size === 0) {
            try {
                // Try to connect to the default MCP server
                const testClient = new MCPClient();
                testClient.onLog = (logEntry) => this.addLogEntry(`MCP-${defaultName}`, logEntry);
                
                await testClient.connect(defaultUrl);
                
                // Connection successful, save server
                const serverId = 'default_mcp_server';
                const server = {
                    id: serverId,
                    name: defaultName,
                    url: defaultUrl,
                    connected: true,
                    lastConnected: new Date().toISOString()
                };
                
                this.mcpServers.set(serverId, server);
                this.mcpConnections.set(serverId, testClient);
                this.saveSettings();
                this.updateMcpServersList();

                this.addLogEntry('SYSTEM', {
                    timestamp: new Date().toISOString(),
                    direction: 'info',
                    message: `Auto-connected to default MCP server at ${defaultUrl}`
                });
                
            } catch {
                // Connection failed, but still add the server for manual connection later
                const serverId = 'default_mcp_server';
                const server = {
                    id: serverId,
                    name: defaultName,
                    url: defaultUrl,
                    connected: false
                };
                
                this.mcpServers.set(serverId, server);
                this.saveSettings();
                this.updateMcpServersList();

                this.addLogEntry('SYSTEM', {
                    timestamp: new Date().toISOString(),
                    direction: 'warning',
                    message: `Added default MCP server at ${defaultUrl} (not connected)`
                });
            }
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
        
        const key = `mcp_chat_${chatId}`;
        try {
            localStorage.setItem(key, JSON.stringify(chat));
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
            testClient.onLog = (logEntry) => this.addLogEntry(`MCP-${name}`, logEntry);
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
        
        for (const [id, server] of this.mcpServers) {
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
        if (confirm('Remove this MCP server?')) {
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
        // Check if there's an unsaved chat
        const unsavedChat = Array.from(this.chats.values()).find(chat => chat.isSaved === false);
        if (unsavedChat) {
            const activeChatId = this.getActiveChatId();
            console.log('[createNewChatDirectly] Found unsaved chat:', unsavedChat.id, 'Active chat:', activeChatId);
            
            // Check if we're already in the unsaved chat
            if (activeChatId === unsavedChat.id) {
                // Already in the unsaved chat, just show toast
                this.showToast('Please use the current chat or save it by sending a message before creating a new one.');
            } else {
                // Switch to the unsaved chat instead of creating a new one
                console.log('[createNewChatDirectly] Switching to unsaved chat:', unsavedChat.id);
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
            mcpServerId = this.mcpServers.keys().next().value;
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
        if (!this.chatContainers.has(chatId)) {
            const container = this.createChatDOM(chatId);
            if (container) {
                this.chatContainersEl.appendChild(container);
                this.chatContainers.set(chatId, container);
                
                // Apply any pending connection state
                const chat = this.chats.get(chatId);
                if (chat && chat.pendingConnectionState) {
                    this.updateChatConnectionUI(chatId, chat.pendingConnectionState.state, chat.pendingConnectionState.details);
                    delete chat.pendingConnectionState;
                }
            }
        }
        return this.chatContainers.get(chatId);
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
                        <textarea 
                            class="chat-input" 
                            placeholder="Ask about your Netdata metrics..."
                            rows="3"
                        ></textarea>
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
            this.sendMessage(chatId).catch(error => {
                console.error('Failed to send message:', error);
                this.showError('Failed to send message', chatId);
            });
        });
        
        // Input field
        elements.input.addEventListener('input', (e) => {
            // Save draft in memory only - don't update UI or save to storage on every keystroke
            chat.draftMessage = e.target.value;
            
            // Update send button state
            elements.sendBtn.disabled = !e.target.value.trim();
            
            // Debounce saving to storage - save after 2 seconds of no typing
            if (this.draftSaveTimeout) {
                clearTimeout(this.draftSaveTimeout);
            }
            this.draftSaveTimeout = setTimeout(() => {
                this.autoSave(chatId);
                // Still don't update UI here - just save to storage
            }, 2000);
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
                if (elements.llmModelDropdown !== dropdownEl) {
                    elements.llmModelDropdown.style.display = 'none';
                }
                if (elements.mcpServerDropdown !== dropdownEl) {
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
                console.log('Blocking DOM switch to new chat - user already selected:', activeChatId);
                return;
            }
        }
        
        // Hide welcome screen
        if (this.welcomeScreen) {
            this.welcomeScreen.style.display = 'none';
        }
        
        // Hide all chat containers
        this.chatContainers.forEach((container, id) => {
            container.classList.remove('active');
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
                
                // Update global references for compatibility
                this.chatInput = container._elements.input;
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
            
            // Create tooltip content
            const tooltipLines = [];
            modelsInUse.forEach((model, purpose) => {
                tooltipLines.push(`${purpose.padEnd(10)} | ${model}`);
            });
            
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
               !chat.waitingForLLM &&
               !chat.waitingForMCP;
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
                if (!chat.isProcessing && !chat.waitingForLLM && !chat.waitingForMCP) {
                    elements.input.disabled = false;
                    elements.sendBtn.disabled = false;
                    elements.input.placeholder = 'Ask about your Netdata metrics...';
                    elements.reconnectBtn.style.display = 'none';
                    
                    // Focus input if this is the active chat
                    if (this.getActiveChatId() === chatId) {
                        elements.input.focus();
                    }
                } else {
                    // Chat is busy - keep input disabled but hide reconnect button
                    elements.input.disabled = true;
                    elements.sendBtn.disabled = true;
                    elements.input.placeholder = 'Processing...';
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
            pendingToolCalls: new Map()
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
        
        // Save the config for next time
        ChatConfig.saveLastConfig(chatConfig);
        
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
            .sort((a, b) => {
                // Show unsaved chats first
                if (a.isSaved === false && b.isSaved !== false) {return -1;}
                if (b.isSaved === false && a.isSaved !== false) {return 1;}
                // Then sort by updated date
                return new Date(b.updatedAt) - new Date(a.updatedAt);
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
            } else if (chat.isProcessing) {
                // Check if waiting for LLM or MCP
                if (chat.waitingForLLM) {
                    statusIcon = '<i class="fas fa-robot"></i>';
                    statusClass = 'status-llm-active';
                } else if (chat.waitingForMCP) {
                    statusIcon = '<i class="fas fa-plug"></i>';
                    statusClass = 'status-mcp-active';
                } else {
                    // Generic processing
                    statusIcon = '<i class="fas fa-spinner fa-spin"></i>';
                    statusClass = 'status-processing';
                }
            } else if (chat.hasError) {
                statusIcon = '<i class="fas fa-exclamation-triangle"></i>';
                statusClass = 'status-error';
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
                    const modelName = chat.config.model.id;
                    const limit = this.modelLimits[modelName] || 4096;
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
        
        // If user has already selected a chat and this is the auto-created new chat trying to load, ignore it
        if (this.userHasSelectedChat && this.pendingNewChatId === chatId && this.getActiveChatId() !== chatId) {
            console.log('Blocking auto-load of new chat because user already selected a different chat');
            return;
        }
        
        // Mark that user has selected a chat
        this.userHasSelectedChat = true;
        
        // Cancel any pending new chat load if this is a different chat
        if (this.pendingNewChatId && this.pendingNewChatId !== chatId) {
            console.log('User selected different chat, cancelling new chat load');
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
            // console.log('[loadChat] Migrating token pricing for chat:', chatId);
            this.migrateTokenPricing(chat);
        }
        
        // Log the totalTokensPrice after migration
        // console.log('[loadChat] totalTokensPrice after migration:', chat.totalTokensPrice);
        
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
                    if (!chat.isProcessing && !chat.waitingForLLM && !chat.waitingForMCP) {
                        elements.input.disabled = false;
                        elements.sendBtn.disabled = false;
                        elements.input.placeholder = 'Ask about your Netdata metrics...';
                        elements.reconnectBtn.style.display = 'none';
                    } else {
                        // Chat is busy - keep input disabled
                        elements.input.disabled = true;
                        elements.sendBtn.disabled = true;
                        elements.input.placeholder = 'Processing...';
                        elements.reconnectBtn.style.display = 'none';
                    }
                    
                } else {
                    // Connection exists but not ready yet
                    elements.input.disabled = true;
                    elements.sendBtn.disabled = true;
                    elements.input.placeholder = 'Connecting to MCP server...';
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
                elements.input.disabled = true;
                elements.sendBtn.disabled = true;
                elements.input.placeholder = 'Connecting to MCP server...';
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
                            elements.input.placeholder = 'MCP server connection failed - click Reconnect';
                            elements.reconnectBtn.style.display = 'block';
                        }
                    });
            }
        } else {
            elements.input.disabled = true;
            elements.sendBtn.disabled = true;
            
            if (!server) {
                elements.input.placeholder = 'MCP server not found';
            } else if (!provider) {
                elements.input.placeholder = 'LLM provider not found';
            } else {
                elements.input.placeholder = 'MCP server or LLM provider not available';
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
        this.currentContextWindow = 0; // Reset context window counter for delta calculation
        
        // Check if we need to re-render messages
        // Re-render if: 1) Never rendered before, 2) DOM is empty (switched from another chat), 3) Force render requested
        const needsRender = !chat.hasBeenRendered || elements.messages.children.length === 0 || forceRender;
        
        if (needsRender) {
            // Clear and re-render messages
            elements.messages.innerHTML = '';
            
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
                if (msg.type === 'tool-results') {
                    this.clearCurrentAssistantGroup(chatId);
                }
            }
            // Clear current group after loading
            this.clearCurrentAssistantGroup(chatId);
            
            // Mark chat as rendered  
            chat.hasBeenRendered = true;
        }
        
        // Update global toggle UI based on chat's tool inclusion mode
        
        // Update context window indicator
        const contextTokens = this.calculateContextWindowTokens(chatId);
        const model = ChatConfig.getChatModelString(chat) || (provider ? provider.model : null);
        
        // Always update the context window and cumulative tokens
        this.updateContextWindowIndicator(contextTokens, model, chatId);
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
                    if (chatInput && !chatInput.disabled) {
                        chatInput.focus();
                    }
                    
                    // Restore draft message if exists
                    if (chatInput && chat.draftMessage) {
                        chatInput.value = chat.draftMessage;
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
        const messageRole = msg.role || msg.type;
        
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
                
                // Then add tool calls
                if (msg.toolCalls && msg.toolCalls.length > 0) {
                    for (const toolCall of msg.toolCalls) {
                        if (!toolCall.id) {
                            console.error('[convertMessageToEvents] Tool call missing required id:', toolCall);
                            continue; // Skip invalid tool calls
                        }
                        // Use destructuring to avoid direct 'arguments' reference
                        const { arguments: toolArgs } = toolCall || {};
                        events.push({ 
                            type: 'tool-call', 
                            name: toolCall.name, 
                            arguments: toolArgs,
                            id: toolCall.id,  // Required tool call ID for matching
                            includeInContext: toolCall.includeInContext !== false,
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
                        includeInContext: result.includeInContext
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
                events.push({ type: 'error-message', content: msg.content, errorMessageIndex: msg.errorMessageIndex });
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
                this.addToolResult(event.name, event.result, chatId, event.responseTime || 0, event.responseSize || null, event.includeInContext, event.messageIndex, event.toolCallId);
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
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                messageDiv.style.position = 'relative';
                
                // Show the error message
                messageDiv.innerHTML = `<div><i class="fas fa-times-circle"></i> ${event.content}</div>`;
                
                // Only show retry/redo buttons for retryable errors (not safety limit errors)
                if (event.errorType !== 'safety_limit' && (event.errorMessageIndex !== undefined && event.errorMessageIndex >= 0)) {
                    // This error has context - use redo button
                    const redoBtn = document.createElement('button');
                    redoBtn.className = 'redo-button';
                    redoBtn.textContent = 'Redo';
                    redoBtn.onclick = () => this.redoFromMessage(event.errorMessageIndex, chatId);
                    messageDiv.appendChild(redoBtn);
                } else {
                    // No errorMessageIndex - create a retry button
                    const retryBtn = document.createElement('button');
                    retryBtn.className = 'btn btn-warning btn-small';
                    retryBtn.style.marginTop = '8px';
                    retryBtn.innerHTML = '<i class="fas fa-redo"></i> Retry';
                    
                    // Check if we have a pending retry callback for this error
                    if (this.pendingRetryCallbacks && this.pendingRetryCallbacks.has(event.content)) {
                        const callback = this.pendingRetryCallbacks.get(event.content);
                        retryBtn.onclick = async () => {
                            retryBtn.disabled = true;
                            retryBtn.textContent = 'Retrying...';
                            this.pendingRetryCallbacks.delete(event.content);
                            await callback();
                        };
                    } else {
                        // Fallback to old retry logic for backwards compatibility
                        retryBtn.onclick = async () => {
                            retryBtn.disabled = true;
                            retryBtn.textContent = 'Retrying...';
                            
                            // Get the current chat and find the last user message
                            const currentChat = this.chats.get(chatId);
                            if (!currentChat) {return;}
                            
                            const lastUserMessage = currentChat.messages.filter(m => m.role === 'user').pop();
                            if (lastUserMessage) {
                        // ATOMIC OPERATION: Remove messages from the error onwards
                        const errorIndex = currentChat.messages.findIndex(m => m.type === 'error' && m.content === event.content);
                        if (errorIndex !== -1) {
                            try {
                                // Check if we're actually discarding any conversation messages (not just the error)
                                const messagesToDiscard = currentChat.messages.length - errorIndex - 1; // -1 to exclude the error itself
                                
                                // Only create accounting node if we're discarding actual conversation messages
                                if (messagesToDiscard > 0) {
                                    const accountingNodes = this.createAccountingNodes('Retry after error', messagesToDiscard);
                                    // Insert all accounting nodes (one per model)
                                    let insertIndex = errorIndex;
                                    for (const accountingNode of accountingNodes) {
                                        this.insertMessage(chat.id, insertIndex, accountingNode);
                                        insertIndex++;
                                    }
                                    // Remove all messages after the accounting nodes
                                    const messagesToRemove = chat.messages.length - insertIndex;
                                    if (messagesToRemove > 0) {
                                        this.removeMessage(chat.id, insertIndex, messagesToRemove);
                                    }
                                } else {
                                    // Just remove the error message, no accounting needed
                                    this.removeMessage(chat.id, errorIndex, 1);
                                }
                            } finally {
                                // Exit batch mode and save atomically
                                this.autoSave(chatId);
                            }
                            
                            // Reload chat to show cleaned history (force render to refresh UI)
                            this.loadChat(chatId, true);
                            
                            // Hide any existing spinner before retry
                            this.hideLoadingSpinner(chatId);
                            
                            // Retry processing
                            try {
                                const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
                                const proxyProvider = this.llmProviders.get(chat.llmProviderId);
                                
                                if (mcpConnection && proxyProvider && chat.config?.model) {
                                    // Get model config
                                    const providerType = chat.config.model.provider;
                                    const modelName = chat.config.model.id;
                                    if (!providerType || !modelName) {
                                        this.showError('Invalid model configuration in chat', chatId);
                                        return;
                                    }
                                    
                                    // Create the actual LLM provider instance
                                    const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
                                    provider.onLog = proxyProvider.onLog;
                                    
                                    await this.processMessageWithTools(chat, mcpConnection, provider, lastUserMessage.content);
                                    
                                    // Save the chat after successful retry
                                    chat.updatedAt = new Date().toISOString();
                                    this.autoSave(chat.id);
                                } else {
                                    this.showError('Cannot retry: MCP server or LLM provider not available', chatId);
                                }
                            } catch (retryError) {
                                // Show error with retry button
                                const retryErrorMessage = `Retry failed: ${retryError.message}`;
                                this.showErrorWithRetry(retryErrorMessage, async () => {
                                    // Remove the retry error message
                                    const retryErrorIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === retryErrorMessage);
                                    if (retryErrorIndex !== -1) {
                                        this.removeMessage(chat.id, retryErrorIndex, 1);
                                        this.loadChat(chatId, true);
                                    }
                                    // Try again - trigger a click event on the original retry button
                                    if (retryBtn) {
                                        retryBtn.click();
                                    }
                                }, 'Retry', chatId);
                                // Find the last user message index for retry functionality
                        const lastUserIdx = chat.messages.findLastIndex(m => m.role === 'user');
                        this.addMessage(chat.id, { type: 'error', content: retryErrorMessage, errorMessageIndex: lastUserIdx });
                            }
                        }
                    }
                };
                    }
                    
                    messageDiv.appendChild(retryBtn);
                }
                
                // Use chat-specific messages container
                const container = this.getChatContainer(chatId);
                if (container && container._elements && container._elements.messages) {
                    container._elements.messages.appendChild(messageDiv);
                }
                break;
                
            case 'show-spinner':
                this.showLoadingSpinner(chatId, event.text || 'Thinking...');
                break;
                
            case 'hide-spinner':
                this.hideLoadingSpinner(chatId);
                // Clear waiting states
                const chatForSpinner = this.chats.get(chatId);
                if (chatForSpinner) {
                    chatForSpinner.waitingForLLM = false;
                    chatForSpinner.waitingForMCP = false;
                    this.updateChatSessions();
                }
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

    deleteChat(chatId) {
        if (!chatId) {return;}
        
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
        if (confirm(`Delete chat "${chat.title}"?`)) {
            this.chats.delete(chatId);
            
            // Remove from split storage
            const key = `mcp_chat_${chatId}`;
            localStorage.removeItem(key);
            
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
    updateSendButton() {
        if (this.isProcessing) {
            this.sendMessageBtn.textContent = 'Stop';
            this.sendMessageBtn.classList.remove('btn-send');
            this.sendMessageBtn.classList.add('btn-danger');
            this.sendMessageBtn.disabled = false;
        } else {
            this.sendMessageBtn.textContent = 'Send';
            this.sendMessageBtn.classList.remove('btn-danger');
            this.sendMessageBtn.classList.add('btn-send');
            this.sendMessageBtn.disabled = !this.chatInput.value.trim();
        }
    }
    
    // Messaging
    async sendMessage(chatId) {
        const message = this.chatInput.value.trim();
        if (!message) {return;}
        
        const chat = this.chats.get(chatId);
        if (!chat) {return;}
        
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
        this.chatInput.value = '';
        this.chatInput.disabled = true;
        this.isProcessing = true;
        this.shouldStopProcessing = false;
        this.updateSendButton();
        
        // Increment turn counter for new user message
        chat.currentTurn = (chat.currentTurn || 0) + 1;
        
        // CRITICAL: Add and display the user's message immediately for better UX
        this.addMessage(chat.id, { type: 'user', role: 'user', content: message, turn: chat.currentTurn });
        const userMessageIndex = chat.messages.length - 1;
        this.processRenderEvent({ type: 'user-message', content: message, messageIndex: userMessageIndex }, chat.id);
        
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
            this.chatInput.disabled = false;
            this.isProcessing = false;
            this.updateSendButton();
            return;
        }
        
        // Create the actual LLM provider instance
        const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
        provider.onLog = proxyProvider.onLog;
        
        // Show loading spinner event
        this.processRenderEvent({ type: 'show-spinner' }, chat.id);
        
        try {
            await this.processMessageWithTools(chat, mcpConnection, provider, message);
            
            // Check if we should generate a title automatically
            if (this.isFirstUserMessage(chat) && TitleGenerator.shouldGenerateTitleAutomatically(chat)) {
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
        } catch (error) {
            // Check if the user stopped processing
            if (this.shouldStopProcessing) {
                // User clicked Stop - show a friendly message with Continue option
                const continueMessage = '⏸️ Processing paused. You can continue the conversation by sending a new message or clicking Continue.';
                this.showErrorWithRetry(continueMessage, async () => {
                    // Find and remove the pause message
                    const errorIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === continueMessage);
                    if (errorIndex !== -1) {
                        this.removeMessage(chat.id, errorIndex, 1);
                        
                        // Reload chat to remove the error from display (force render to refresh UI)
                        this.loadChat(chat.id, true);
                    }
                    
                    // Continue with a simple continuation prompt
                    this.chatInput.value = 'Please continue';
                    await this.sendMessage(chat.id);
                }, 'Continue', chat.id);
                // Find the last user message index for retry functionality
                const lastUserMsgIdx = chat.messages.findLastIndex(m => m.role === 'user');
                this.addMessage(chat.id, { type: 'error', content: continueMessage, errorMessageIndex: lastUserMsgIdx });
                
                // Display the continue message
                this.processRenderEvent({ type: 'error-message', content: continueMessage, errorMessageIndex: lastUserMsgIdx }, chat.id);
            } else {
                // Regular error handling
                const errorMessage = `Error: ${error.message}`;
                // Don't use showErrorWithRetry as it creates a duplicate - just add the error message
                // The error message will be rendered with a retry button when displayed
                const lastUserMessageIndex = chat.messages.findLastIndex(m => m.role === 'user');
                this.addMessage(chat.id, { type: 'error', content: errorMessage, errorMessageIndex: lastUserMessageIndex });
                
                // Display the error message
                this.processRenderEvent({ type: 'error-message', content: errorMessage, errorMessageIndex: lastUserMessageIndex }, chat.id);
                
                // Create the retry callback for when the retry button is clicked
                this.pendingRetryCallbacks = this.pendingRetryCallbacks || new Map();
                this.pendingRetryCallbacks.set(errorMessage, async () => {
                    // Find and remove only the error message
                    const errorMessageIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === errorMessage);
                    if (errorMessageIndex !== -1) {
                        this.removeMessage(chat.id, errorMessageIndex, 1);
                        
                        // Reload chat to remove the error from display (force render to refresh UI)
                        this.loadChat(chat.id, true);
                    }
                    
                    // Retry processing without removing any other messages
                    try {
                        // Continue the conversation from where it left off
                        await this.processMessageWithTools(chat, mcpConnection, provider, message);
                        
                        // Save the chat after successful retry
                        chat.updatedAt = new Date().toISOString();
                        this.autoSave(chat.id);
                    } catch (retryError) {
                        // Show error with retry button
                        const retryErrorMessage = `Retry failed: ${retryError.message}`;
                        this.showErrorWithRetry(retryErrorMessage, async () => {
                            // Remove the retry error message
                            const retryErrIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === retryErrorMessage);
                            if (retryErrIndex !== -1) {
                                this.removeMessage(chat.id, retryErrIndex, 1);
                                this.loadChat(chat.id, true);
                            }
                            // Try again - resend the original message
                            await this.processMessageWithTools(chat, mcpConnection, provider, message);
                        }, 'Retry', chat.id);
                        // Find the last user message index for retry functionality
                        const lastUserIdx = chat.messages.findLastIndex(m => m.role === 'user');
                        this.addMessage(chat.id, { type: 'error', content: retryErrorMessage, errorMessageIndex: lastUserIdx });
                        
                        // Display the retry error message
                        this.processRenderEvent({ type: 'error-message', content: retryErrorMessage, errorMessageIndex: lastUserIdx }, chat.id);
                    }
                });
            }
        } finally {
            // Remove loading spinner event
            this.processRenderEvent({ type: 'hide-spinner' }, chat.id);
            
            // Clear current assistant group after processing is complete
            this.clearCurrentAssistantGroup(chat.id);
            
            chat.updatedAt = new Date().toISOString();
            this.autoSave(chat.id);
            this.chatInput.disabled = false;
            this.isProcessing = false;
            this.shouldStopProcessing = false;
            this.updateSendButton();
            this.chatInput.focus();
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
            
            const msgRole = msg.role || msg.type;
            
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
                if (!msg.toolCalls || msg.toolCalls.length === 0) {
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
            const result = chat.messageOptimizer.buildMessagesForAPI(chat, freezeCache, mcpInstructions);
            
            // Log optimization stats if available
            if (result.stats) {
                // console.log(`[buildMessagesForAPI] Optimization stats for chat ${chat.id}:`, result.stats);
            }
            
            return result;
        } catch (error) {
            console.error('[buildMessagesForAPI] MessageOptimizer failed:', error);
            this.showError(`Message optimization failed: ${error.message}`, chat.id);
            throw error;
        }
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
            
            // Check if we should stop processing
            if (this.shouldStopProcessing) {
                // Don't show a system message here - we'll handle it in the catch block
                break;
            }
            
            
            // SAFETY CHECK: Validate request before sending to LLM
            try {
                // Increment iteration count
                this.safetyChecker.incrementIterations(chat.id);
                
                // Prepare request data for size validation
                const requestData = {
                    messages,
                    tools,
                    temperature: this.getCurrentTemperature(chat.id)
                };
                
                // Validate all safety limits
                this.safetyChecker.validateRequest(chat.id, requestData, []);
            } catch (error) {
                if (error instanceof SafetyLimitError) {
                    // Show safety error with no retry option
                    this.addMessage(chat.id, { 
                        type: 'error', 
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
            const llmStartTime = Date.now();
            // eslint-disable-next-line no-await-in-loop
            const response = await provider.sendMessage(messages, tools, temperature, cacheControlIndex);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Track token usage
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, ChatConfig.getChatModelString(chat) || provider.model);
            }
            
            // If no tool calls, display response and finish
            if (!response.toolCalls || response.toolCalls.length === 0) {
                // Emit metrics event first (just like loaded chats)
                this.processRenderEvent({ 
                    type: 'assistant-metrics', 
                    usage: response.usage, 
                    responseTime: llmResponseTime, 
                    model: ChatConfig.getChatModelString(chat) || provider.model
                }, chat.id);
                
                if (response.content) {
                    // Create the assistant message first
                    const assistantMsg = { 
                        type: 'assistant', 
                        role: 'assistant', 
                        content: response.content,
                        usage: response.usage || null,
                        responseTime: llmResponseTime || null,
                        model: provider.model || ChatConfig.getChatModelString(chat),
                        turn: chat.currentTurn
                    };
                    // Use addMessage to calculate price and update cumulative totals
                    this.addMessage(chat.id, assistantMsg);
                    
                    // Then emit assistant message event with messageIndex
                    const messageIndex = chat.messages.length - 1;
                    this.processRenderEvent({ 
                        type: 'assistant-message', 
                        content: response.content,
                        messageIndex
                    }, chat.id);
                    
                    // Track token usage with model information
                    if (response.usage) {
                        const modelUsed = ChatConfig.getChatModelString(chat) || provider.model;
                        this.addCumulativeTokens(
                            chat.id,
                            modelUsed,
                            response.usage.promptTokens || 0,
                            response.usage.completionTokens || 0,
                            response.usage.cacheReadInputTokens || 0,
                            response.usage.cacheCreationInputTokens || 0
                        );
                    }
                    // Clean content before sending back to API
                    const cleanedContent = this.cleanContentForAPI(response.content);
                    if (cleanedContent && cleanedContent.trim()) {
                        messages.push({ role: 'assistant', content: cleanedContent });
                    }
                }
                break;
            }
            
            // Initialize assistantMessageIndex here so it's available for tool execution
            let assistantMessageIndex = null;
            
            // Store assistant message with tool calls if any
            if (response.content || response.toolCalls) {
                // CRITICAL: Save assistant message BEFORE displaying
                const assistantMessage = {
                    type: 'assistant',
                    role: 'assistant',
                    content: response.content || '',
                    toolCalls: (response.toolCalls || []).map(tc => ({
                        ...tc,
                        includeInContext: true // Default to included
                    })),
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: provider.model || ChatConfig.getChatModelString(chat),
                    turn: chat.currentTurn,
                    cacheControlIndex // Store where cache control was placed
                };
                this.addMessage(chat.id, assistantMessage);
                
                // Track token usage with model information
                if (response.usage) {
                    const modelUsed = ChatConfig.getChatModelString(chat) || provider.model;
                    this.addCumulativeTokens(
                        chat.id,
                        modelUsed,
                        response.usage.promptTokens || 0,
                        response.usage.completionTokens || 0,
                        response.usage.cacheReadInputTokens || 0,
                        response.usage.cacheCreationInputTokens || 0
                    );
                }
                
                // Now display the message - after it's safely saved
                // Get the message index after adding
                assistantMessageIndex = chat.messages.length - 1;
                
                // Emit metrics event first (just like loaded chats)
                this.processRenderEvent({ 
                    type: 'assistant-metrics', 
                    usage: response.usage, 
                    responseTime: llmResponseTime, 
                    model: ChatConfig.getChatModelString(chat) || provider.model
                }, chat.id);
                
                if (response.content) {
                    this.processRenderEvent({ 
                        type: 'assistant-message', 
                        content: response.content,
                        messageIndex: assistantMessageIndex
                    }, chat.id);
                }
                
                // Build the message for the API
                const cleanedContent = response.content ? this.cleanContentForAPI(response.content) : '';
                
                // Create standard assistant message
                const assistantMsg = {
                    role: 'assistant',
                    content: cleanedContent
                };
                
                // Add tool calls if present (providers will convert to their format)
                if (response.toolCalls && response.toolCalls.length > 0) {
                    assistantMsg.toolCalls = response.toolCalls;
                }
                
                // Only add message if it has content or tool calls
                if (cleanedContent || response.toolCalls && response.toolCalls.length > 0) {
                    messages.push(assistantMsg);
                }
            }
            
            // Execute tool calls and collect results
            if (response.toolCalls && response.toolCalls.length > 0) {
                // SAFETY CHECK: Validate concurrent tools limit
                try {
                    this.safetyChecker.checkConcurrentToolsLimit(response.toolCalls);
                } catch (error) {
                    if (error instanceof SafetyLimitError) {
                        // Show safety error with no retry option
                        this.addMessage(chat.id, { 
                            type: 'error', 
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
                
                // Ensure we have an assistant group even if there was no content
                if (!this.getCurrentAssistantGroup(chat.id)) {
                    this.processRenderEvent({ 
                        type: 'assistant-message', 
                        content: '',
                        messageIndex: assistantMessageIndex
                    }, chat.id);
                }
                
                const toolResults = [];
                
                for (const toolCall of response.toolCalls) {
                    if (!toolCall.id) {
                        console.error('[processMessageWithTools] Tool call missing required id:', toolCall);
                        continue; // Skip invalid tool calls
                    }
                    try {
                        // Use destructuring to avoid direct 'arguments' reference
                        const { arguments: toolArgs } = toolCall || {};
                        // Show tool call in UI
                        this.processRenderEvent({ 
                            type: 'tool-call', 
                            name: toolCall.name, 
                            arguments: toolArgs,
                            id: toolCall.id,  // Required tool ID for proper matching
                            includeInContext: toolCall.includeInContext !== false 
                        }, chat.id);
                        
                        // Show spinner while executing tool
                        this.processRenderEvent({ type: 'show-spinner', text: `Executing ${toolCall.name}...` }, chat.id);
                        
                        // Set MCP waiting state
                        chat.waitingForMCP = true;
                        chat.waitingForLLM = false;
                        this.updateChatSessions();
                        
                        // Execute tool and track timing
                        const toolStartTime = Date.now();
                        // eslint-disable-next-line no-await-in-loop
                        const rawResult = await mcpConnection.callTool(toolCall.name, toolArgs);
                        const toolResponseTime = Date.now() - toolStartTime;
                        
                        // Hide spinner after tool execution
                        this.processRenderEvent({ type: 'hide-spinner' }, chat.id);
                        
                        // Parse the result to handle MCP's response format
                        const result = this.parseToolResult(rawResult);
                        
                        // Calculate response size
                        const responseSize = typeof result === 'string' 
                            ? result.length 
                            : JSON.stringify(result).length;
                        
                        // Show result in UI with timing and size
                        this.processRenderEvent({ 
                            type: 'tool-result', 
                            name: toolCall.name, 
                            result, 
                            toolCallId: toolCall.id,  // Pass tool call ID for proper matching
                            responseTime: toolResponseTime, 
                            responseSize, 
                            messageIndex: assistantMessageIndex 
                        }, chat.id);
                        
                        // Collect result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result,
                            includeInContext: true // Will be updated based on checkbox state
                        });
                        
                    } catch (error) {
                        const errorMsg = `Tool error (${toolCall.name}): ${error.message}`;
                        this.processRenderEvent({ type: 'tool-result', name: toolCall.name, result: { error: errorMsg }, responseTime: 0, responseSize: errorMsg.length, messageIndex: assistantMessageIndex, toolCallId: toolCall.id }, chat.id);
                        
                        // Collect error result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result: { error: errorMsg },
                            includeInContext: true // Will be updated based on checkbox state
                        });
                    }
                }
                
                // Update tool results with inclusion state from checkboxes
                if (toolResults.length > 0) {
                    // Check current checkbox states
                    toolResults.forEach(tr => {
                        // Tool inclusion checkboxes have been removed
                        // Always include tool results in context
                        tr.includeInContext = true;
                    });
                    }
                    
                    // Store all tool results together
                    // NOTE: Tool results are displayed during execution for user feedback,
                    // but the consolidated results are saved here to ensure consistency
                    this.addMessage(chat.id, {
                        type: 'tool-results',
                        toolResults,
                        turn: chat.currentTurn
                    });
                    
                    // Filter included results for API
                    const includedResults = toolResults.filter(tr => tr.includeInContext !== false);
                    
                    if (includedResults.length > 0) {
                        // Add standard tool-results message
                        // Providers will convert to their specific format
                        const toolResultsMessage = {
                            role: 'tool-results',
                            toolResults: includedResults.map(tr => ({
                                toolCallId: tr.toolCallId,
                                toolName: tr.name,
                                result: tr.result
                            }))
                        };
                        
                        messages.push(toolResultsMessage);
                        
                        // Check if we need to perform tool summarization
                        if (chat.messageOptimizer && chat.config?.optimisation?.toolSummarisation?.enabled) {
                            try {
                                // Get tool schemas for context
                                const toolSchemas = new Map();
                                for (const tool of tools) {
                                    toolSchemas.set(tool.name, tool);
                                }
                                
                                // Get provider info
                                const providerInfo = {
                                    url: provider.proxyUrl
                                };
                                
                                // Perform async summarization
                                // eslint-disable-next-line no-await-in-loop
                                const summarizedMessages = await chat.messageOptimizer.performToolSummarization(
                                    messages,
                                    {
                                        toolSchemas,
                                        providerInfo
                                    }
                                );
                                
                                // Replace messages array with summarized version
                                messages.length = 0;
                                messages.push(...summarizedMessages);
                                
                                // Log summarization stats
                                const lastToolMessage = messages[messages.length - 1];
                                if (lastToolMessage && lastToolMessage.toolResults) {
                                    const summarizedCount = lastToolMessage.toolResults.filter(
                                        tr => tr.result && tr.result._type === 'summarized'
                                    ).length;
                                    if (summarizedCount > 0) {
                                        console.log(`[Tool Summarization] Summarized ${summarizedCount} large tool responses`);
                                    }
                                }
                            } catch (error) {
                                console.error('[Tool Summarization] Failed:', error);
                                // Continue with original messages on error
                            }
                        }
                    }
                    
                    // Reset assistant group after tool results so next metrics appears separately
                    this.processRenderEvent({ type: 'reset-assistant-group' }, chat.id);
                    
                    // Show thinking spinner again before next LLM call
                    this.processRenderEvent({ type: 'show-spinner', text: 'Thinking...' }, chat.id);
                }
            }
        } catch (error) {
            // Set error state
            chat.hasError = true;
            chat.lastError = error.message;
            throw error;
        } finally {
            // Clear processing state when done
            chat.isProcessing = false;
            chat.waitingForLLM = false;
            chat.waitingForMCP = false;
            this.updateChatSessions(); // Update sidebar to remove activity indicator
        }
    }

    renderMessage(role, content, messageIndex, chatId) {
        if (!chatId) {
            console.error('renderMessage called without chatId');
            return;
        }
        
        // Ensure content is a string
        if (typeof content !== 'string') {
            console.error('renderMessage: content is not a string', { role, content, messageIndex });
            content = String(content || '');
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
        
        // Add redo button only for user and assistant messages
        if (messageIndex !== undefined && (role === 'user' || role === 'assistant')) {
            const redoBtn = document.createElement('button');
            redoBtn.className = 'redo-button';
            redoBtn.textContent = 'Redo';
            redoBtn.onclick = () => this.redoFromMessage(messageIndex, chatId);
            messageDiv.style.position = 'relative';
            messageDiv.appendChild(redoBtn);
        }
        
        // Check if content has thinking tags
        const thinkingRegex = /<thinking>([\s\S]*?)<\/thinking>/g;
        const hasThinking = thinkingRegex.test(content);
        
        // Make user messages editable on click
        if (role === 'user' && chatId) {
            messageDiv.classList.add('editable-message');
        }
        
        // Process content
        if (hasThinking && role === 'assistant') {
            // Reset regex for actual processing
            content.match(/<thinking>([\s\S]*?)<\/thinking>/g);
            
            // Split content into parts
            const parts = [];
            let lastIndex = 0;
            let match;
            const regex = /<thinking>([\s\S]*?)<\/thinking>/g;
            
            while ((match = regex.exec(content)) !== null) {
                // Add text before thinking
                if (match.index > lastIndex) {
                    parts.push({
                        type: 'text',
                        content: content.substring(lastIndex, match.index).trim()
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
            if (lastIndex < content.length) {
                const remaining = content.substring(lastIndex).trim();
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
                        const chat = this.chats.get(chatId);
                        if (chat) {
                            // Find the most recent summary message
                            for (let i = chat.messages.length - 1; i >= 0; i--) {
                                if (chat.messages[i]?.role === 'summary') {
                                    if (confirm('Delete this summary and replace with accounting record?')) {
                                        this.deleteSummaryMessages(i, chatId);
                                    }
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
                    responseContent.innerHTML = marked.parse(content, {
                        breaks: true, gfm: true, sanitize: false
                    });
                    responseSection.appendChild(responseContent);
                } else {
                    // For title, use plain text
                    const responseContent = document.createElement('pre');
                    responseContent.textContent = content;
                    responseSection.appendChild(responseContent);
                }
                
                systemContent.appendChild(responseSection);
                
                return; // Don't append messageDiv to chat
            } 
                console.warn(`No ${blockType} block found for response:`, content);
            
        } else {
            // Regular message without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            
            if (role === 'assistant' || role === 'user') {
                // Use marked to render markdown for assistant and user messages
                // Configure marked to preserve line breaks and handle whitespace properly
                contentDiv.innerHTML = marked.parse(content, {
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
                deltaTokens = (usage.completionTokens || 0) - this.currentContextWindow;
                // Reset context window to just the summary's output
                this.currentContextWindow = usage.completionTokens || 0;
            } else {
                // Regular assistant messages
                deltaTokens = totalTokens - this.currentContextWindow;
                // Update the running total
                this.currentContextWindow = totalTokens;
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
        
        // Make content editable
        contentDiv.contentEditable = true;
        contentDiv.classList.add('editing');
        const originalText = contentDiv.textContent;
        
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
            contentDiv.textContent = originalText;
            buttonsDiv.remove();
            // Clean up event listeners
            contentDiv.removeEventListener('keydown', keyHandler);
            document.removeEventListener('click', clickOutside);
            // Restore the edit trigger
            this.addEditTrigger(contentDiv, originalText, 'user', chatId);
        };
        
        save = async () => {
            const newContent = contentDiv.textContent.trim();
            if (!newContent) {
                this.showError('Message cannot be empty', chatId);
                return;
            }
            
            // Always proceed even if content is the same - user may want to regenerate response
            
            // ATOMIC OPERATION: Batch mode to prevent partial saves
            try {
                // Create accounting node before discarding messages
                const messagesToDiscard = chat.messages.length - messageIndex;
                if (messagesToDiscard > 0) {
                    const accountingNodes = this.createAccountingNodes('Message edited', messagesToDiscard);
                    // Insert all accounting nodes (one per model)
                    for (const accountingNode of accountingNodes) {
                        this.insertMessage(chat.id, messageIndex, accountingNode);
                        messageIndex++; // Adjust index after each insertion
                    }
                }
                
                // Clip history at this point - remove all messages after messageIndex
                const toRemove = chat.messages.length - messageIndex;
                if (toRemove > 0) {
                    this.removeMessage(chat.id, messageIndex, toRemove);
                }
            } finally {
                // Exit batch mode and save immediately
                this.autoSave(chatId);
            }
            
            // Reload the chat to show clipped history (force render to refresh UI)
            this.loadChat(chatId, true);
            
            // Send the new message
            this.chatInput.value = newContent;
            await this.sendMessage(chatId);
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
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
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
        requestSection.innerHTML = `
            <div class="tool-section-controls">
                <span class="tool-section-label">📤 REQUEST</span>
                <button class="tool-section-copy" data-tooltip="Copy request"><i class="fas fa-clipboard"></i></button>
            </div>
            <pre>${JSON.stringify(args, null, 2)}</pre>
        `;
        toolContent.appendChild(requestSection);
        
        // Add copy functionality for request
        const requestCopyBtn = requestSection.querySelector('.tool-section-copy');
        requestCopyBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            const text = JSON.stringify(args, null, 2);
            navigator.clipboard.writeText(text).then(() => {
                requestCopyBtn.innerHTML = '<i class="fas fa-check"></i>';
                setTimeout(() => { requestCopyBtn.innerHTML = '<i class="fas fa-clipboard"></i>'; }, 1000);
            });
        });
        
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

    addToolResult(toolName, result, chatId, responseTime, responseSize, includeInContext, _messageIndex, toolCallId) {
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
                if (typeof result === 'object') {
                    if (result.error) {
                        formattedResult = `<span style="color: var(--danger-color);">${result.error}</span>`;
                    } else {
                        formattedResult = `<pre>${JSON.stringify(result, null, 2)}</pre>`;
                    }
                } else {
                    formattedResult = result;
                }
                
                responseSection.innerHTML = `
                    <div class="tool-section-controls">
                        <span class="tool-section-label">📥 RESPONSE</span>
                        <button class="tool-section-copy" data-tooltip="Copy response"><i class="fas fa-clipboard"></i></button>
                    </div>
                    ${formattedResult}
                `;
                responseSection.style.display = 'block';
                
                // Add copy functionality for response
                const responseCopyBtn = responseSection.querySelector('.tool-section-copy');
                responseCopyBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    const text = typeof result === 'object' ? JSON.stringify(result, null, 2) : String(result);
                    navigator.clipboard.writeText(text).then(() => {
                        responseCopyBtn.innerHTML = '<i class="fas fa-check"></i>';
                        setTimeout(() => { responseCopyBtn.innerHTML = '<i class="fas fa-clipboard"></i>'; }, 1000);
                    });
                });
                
                if (separator) {
                    separator.style.display = 'block';
                }
            }
            
            // Remove from pending using the toolCallId
            if (toolCallId && chat.pendingToolCalls) {
                chat.pendingToolCalls.delete(toolCallId);
            }
        }
        
        this.scrollToBottom(chatId);
        this.moveSpinnerToBottom(chatId);
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
        
        // Only scroll if user is already near the bottom, unless forced
        const threshold = 100; // pixels from bottom to consider "at bottom"
        const isAtBottom = chatMessages.scrollHeight - chatMessages.scrollTop - chatMessages.clientHeight < threshold;
        
        if (isAtBottom || force) {
            chatMessages.scrollTop = chatMessages.scrollHeight;
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
        // Remove thinking tags and their content
        return content.replace(/<thinking>[\s\S]*?<\/thinking>/g, '').trim();
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

    showLoadingSpinner(chatId, text = 'Thinking...') {
        if (!chatId) {
            console.error('showLoadingSpinner called without chatId');
            return;
        }
        
        // Set chat state to waiting for LLM
        const chat = this.chats.get(chatId);
        if (chat) {
            chat.waitingForLLM = true;
            this.updateChatSessions();
        }
        
        // CRITICAL FIX: Remove any existing spinners from ALL chats first
        // This prevents orphaned spinners from appearing in wrong chats
        this.chatContainers.forEach((container, id) => {
            this.hideLoadingSpinner(id);
        });
        
        // Create spinner element
        const spinnerDiv = document.createElement('div');
        spinnerDiv.id = `llm-loading-spinner-${chatId}`;
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
        
        const container = this.getChatContainer(chatId);
        if (container && container._elements && container._elements.messages) {
            container._elements.messages.appendChild(spinnerDiv);
        }
        
        // Force scroll to bottom when showing spinner
        requestAnimationFrame(() => {
            this.scrollToBottom(chatId, true);
        });
    }

    hideLoadingSpinner(chatId) {
        if (chatId) {
            // Clear chat-specific interval
            if (this.spinnerIntervals && this.spinnerIntervals.has(chatId)) {
                clearInterval(this.spinnerIntervals.get(chatId));
                this.spinnerIntervals.delete(chatId);
            }
            
            // Remove spinner for specific chat
            const spinner = document.getElementById(`llm-loading-spinner-${chatId}`);
            if (spinner) {
                spinner.remove();
            }
        } else {
            // Fallback: clear global interval and remove any spinner with old global ID (for backwards compatibility)
            if (this.spinnerInterval) {
                clearInterval(this.spinnerInterval);
                this.spinnerInterval = null;
            }
            
            const spinner = document.getElementById('llm-loading-spinner');
            if (spinner) {
                spinner.remove();
            }
        }
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
            mcpConnection.onLog = (logEntry) => this.addLogEntry(`MCP-${server.name}`, logEntry);
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
                console.log(`Unhandled connection state: ${connectionState}`, details);
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
        
        // Hide any loading spinners
        this.hideLoadingSpinner(chatId);
        
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
        mcpConnection.onLog = (logEntry) => this.addLogEntry(`MCP-${server.name}`, logEntry);
        
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
        
        // Calculate context window based on message type
        const contextTokens = this.calculateContextWindowTokens(chatId);
        
        // Update context window indicator
        this.updateContextWindowIndicator(contextTokens, model, chatId);
        
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
    
    updateContextWindowIndicator(totalTokens, model, chatId) {
        if (!chatId) {
            console.error('updateContextWindowIndicator called without chatId');
            return;
        }
        
        const container = this.chatContainers.get(chatId);
        if (!container) {
            console.error('updateContextWindowIndicator: container not found for chat', chatId);
            return;
        }
        
        const elements = container._elements;
        if (!elements.contextFill || !elements.contextStats) {return;}
        
        // Extract model name from format "provider:model-name" if needed
        const modelName = ChatConfig.getModelDisplayName(model);
        
        // Get model info
        const limit = this.modelLimits[modelName] || 4096;
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
            // console.log('[getCumulativeTokenUsage] Chat not found:', chatId);
            return { 
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        if (!chat.totalTokensPrice) {
            // console.log('[getCumulativeTokenUsage] No totalTokensPrice for chat:', chatId);
            return { 
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        // console.log('[getCumulativeTokenUsage] totalTokensPrice:', chat.totalTokensPrice);
        
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
            const contextTokens = this.calculateContextWindowTokens(chatId);
            this.updateContextWindowIndicator(contextTokens, ChatConfig.getChatModelString(chat) || 'unknown', chatId);
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
        
        const cumulative = this.getCumulativeTokenUsage(chatId);
        // console.log('[updateCumulativeTokenDisplay] Cumulative tokens for chat', chatId, cumulative);

        // Get the chat-specific DOM elements
        const container = this.getChatContainer(chatId);
        if (!container || !container._elements) {
            console.error('[updateCumulativeTokenDisplay] Container or elements not found for chat:', chatId);
            return;
        }
        
        // console.log('[updateCumulativeTokenDisplay] Container elements:', container._elements);
        
        const inputElement = container._elements.cumulativeInputTokens;
        const outputElement = container._elements.cumulativeOutputTokens;
        const cacheReadElement = container._elements.cumulativeCacheReadTokens;
        const cacheCreationElement = container._elements.cumulativeCacheCreationTokens;
        
        // console.log('[updateCumulativeTokenDisplay] Elements found:', {
        //     inputElement: Boolean(inputElement),
        //     outputElement: Boolean(outputElement),
        //     cacheReadElement: Boolean(cacheReadElement),
        //     cacheCreationElement: Boolean(cacheCreationElement)
        // });
        
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
        
        // Create a sanitized copy of the messages without payloads
        const sanitizedMessages = chat.messages.map((message, index) => {
            const sanitized = {
                index,
                type: message.type,
                role: message.role
            };
            
            // Add metadata fields but not content
            if (message.usage) {
                sanitized.usage = message.usage;
            }
            
            if (message.responseTime) {
                sanitized.responseTime = message.responseTime;
            }
            
            if (message.toolCalls && message.toolCalls.length > 0) {
                sanitized.toolCalls = message.toolCalls.map(tc => ({
                    name: tc.name,
                    toolCallId: tc.toolCallId
                    // Don't include args or result
                }));
            }
            
            if (message.results && Array.isArray(message.results)) {
                sanitized.results = message.results.map(r => ({
                    name: r.name,
                    toolCallId: r.toolCallId,
                    resultSize: r.result 
                        ? typeof r.result === 'string' 
                            ? r.result.length 
                            : JSON.stringify(r.result).length
                        : 0
                    // Don't include actual result
                }));
            }
            
            // Add content size but not content itself
            if (message.content) {
                sanitized.contentSize = typeof message.content === 'string' 
                    ? message.content.length 
                    : JSON.stringify(message.content).length;
            }
            
            return sanitized;
        });
        
        const output = {
            title: chat.title,
            model: ChatConfig.getChatModelString(chat),
            messageCount: chat.messages.length,
            messages: sanitizedMessages
        };
        
        try {
            await navigator.clipboard.writeText(JSON.stringify(output, null, 2));
            
            // Show success feedback
            const btn = this.copyMetricsBtn;
            const originalText = btn.textContent;
            btn.innerHTML = '<i class="fas fa-check"></i> Copied JSON!';
            btn.classList.add('btn-success');
            
            setTimeout(() => {
                btn.textContent = originalText;
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
                const p = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
                p.onLog = proxyProvider.onLog;
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
            
            // Show loading spinner
            this.processRenderEvent({ type: 'show-spinner' }, chat.id);
            
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
            
            console.log('Messages before adding summary request:', messages.length, messages.map(m => ({ role: m.role, contentLength: m.content?.length || 0 })));
            
            // IMPORTANT: The summary request should be added AFTER building messages
            // but BEFORE the system-summary message is added to chat history
            // This way it's not included in the conversation being summarized
            
            // Add the simple summary request as a user message
            messages.push({ role: 'user', content: summaryRequest });
            
            // For summaries, we can use a simple cache control on the last conversational message
            const cacheControlIndex = messages.length - 2; // Before the summary request
            
            console.log('Total messages being sent:', messages.length);
            
            // Send request with low temperature for consistent summaries
            const temperature = 0.5;
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, [], temperature, cacheControlIndex);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' }, chat.id);
            
            // Update metrics
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, ChatConfig.getChatModelString(chat) || provider.model);
            }
            
            // Process the summary response
            if (response.content) {
                // CRITICAL: Save summary response BEFORE displaying
                this.addMessage(chat.id, { 
                    role: 'summary', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: provider.model || ChatConfig.getChatModelString(chat),
                    timestamp: new Date().toISOString(),
                    cacheControlIndex // Store the frozen cache position
                });
                
                // Display the response as a summary message WITH metrics
                this.processRenderEvent({ 
                    type: 'summary-message', 
                    content: response.content,
                    usage: response.usage,
                    responseTime: llmResponseTime,
                    model: ChatConfig.getChatModelString(chat) || provider.model
                }, chat.id);
                
                // Update context window display
                const contextTokens = this.calculateContextWindowTokens(chat.id);
                this.updateContextWindowIndicator(contextTokens, ChatConfig.getChatModelString(chat) || provider.model, chat.id);
                
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
            this.processRenderEvent({ type: 'hide-spinner' }, chat.id);
            
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
            
            const provider = window.createLLMProvider(
                providerType,
                llmProviderConfig.proxyUrl,
                modelName
            );
            provider.onLog = llmProviderConfig.onLog;
            
            // Use the unified method
            await this.generateChatSummary(chat, mcpConnection, provider, false);
            
        } catch (error) {
            console.error('Failed to summarize conversation:', error);
            
            // Hide spinner on error
            this.processRenderEvent({ type: 'hide-spinner' }, chatId);
            
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
    shouldGenerateSummary(_chat) {
        // Placeholder implementation - customize conditions as needed
        // Examples of conditions you might want:
        // - After X messages
        // - After Y tokens used
        // - After Z time elapsed
        // - When context window is X% full
        // - Every N user messages
        
        // For now, return false - no automatic summaries
        return false;
        
        // Example implementation (uncomment and customize):
        /*
        // Don't summarize if already has a summary
        if (chat.summaryGenerated) return false;
        
        // Check message count (e.g., after 20 exchanges)
        const userMessages = chat.messages.filter(m => m.role === 'user' && !['system-title', 'system-summary'].includes(m.role));
        const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
        if (userMessages.length < 10 || assistantMessages.length < 10) return false;
        
        // Check context window usage (e.g., when 80% full)
        const contextTokens = this.calculateContextWindowTokens(chat.id);
        const modelLimit = this.getModelContextLimit(ChatConfig.getChatModelString(chat));
        if (contextTokens < modelLimit * 0.8) return false;
        
        // Check time elapsed (e.g., after 30 minutes)
        const firstMessage = chat.messages.find(m => m.timestamp);
        if (firstMessage) {
            const elapsed = Date.now() - new Date(firstMessage.timestamp).getTime();
            if (elapsed < 30 * 60 * 1000) return false;
        }
        
        return true;
        */
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
        if (chat) {
            chat.currentAssistantGroup = null;
        }
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
        // Clear processing state
        this.isProcessing = false;
        this.shouldStopProcessing = false;
        
        // Clear token display state
        this.currentContextWindow = 0;
        
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
            console.log('No chat is currently loaded');
            return;
        }
        const chat = window.app.chats.get(activeChatId);
        if (!chat) {
            console.log('Chat not found');
            return;
        }
        console.log('Migrating chat:', chat.title);
        console.log('Before migration:', {
            totalTokensPrice: chat.totalTokensPrice,
            perModelTokensPrice: chat.perModelTokensPrice
        });
        window.app.migrateTokenPricing(chat);
        console.log('After migration:', {
            totalTokensPrice: chat.totalTokensPrice,
            perModelTokensPrice: chat.perModelTokensPrice
        });
        // Update displays
        window.app.updateAllTokenDisplays(activeChatId);
        console.log('Migration complete!');
    };
});

