/**
 * Main application logic for the Netdata MCP LLM Client
 */

class NetdataMCPChat {
    constructor() {
        this.mcpServers = new Map(); // Multiple MCP servers
        this.mcpConnections = new Map(); // Active MCP connections
        this.llmProviders = new Map(); // Multiple LLM providers
        this.chats = new Map(); // Chat sessions
        this.currentChatId = null;
        this.communicationLog = []; // Universal log (not saved)
        this.tokenUsageHistory = new Map(); // Track token usage per chat
        this.toolInclusionStates = new Map(); // Track which tools are included/excluded per chat
        this.currentContextWindow = 0; // Running total for delta calculation during rendering
        this.shouldStopProcessing = false; // Flag to stop processing between requests
        this.isProcessing = false; // Track if we're currently processing messages
        this.pendingSaveTimeout = null; // Debounce saves for performance
        this.modelPricing = {}; // Initialize model pricing storage
        this.modelLimits = {}; // Initialize model context limits storage
        
        // Models will be loaded dynamically from the proxy server
        // No hardcoded model list needed
        
        // Default system prompt
        this.defaultSystemPrompt = `You are the Netdata assistant, with access to Netdata monitoring data via your tools. 

For ANY request involving data analysis, troubleshooting, or complex queries, you MUST use <thinking> tags to show your complete reasoning process.
In your <thinking> section, always include:

- Your interpretation of the user's request and what they're trying to accomplish
- Your strategy for approaching the problem (which tools to use and why)
- Analysis of each piece of data you retrieve
- Connections you're making between different metrics/nodes/alerts
- Any assumptions or limitations in your analysis
- Your reasoning for conclusions or recommendations

For each request, follow this structured process:

1. IDENTIFY WHAT IS RELEVANT
  - Identify which of the tools may be relevant to the user's request to provide insights or data.
  - Each tool has a description and parameters. Read them carefully to understand what data they can provide.
  - Once you have a plan, use all the relevant tools to gather data, AT ONCE (return an array of tools to execute).
  
  Usually your entry point will be:
  
  - If the user specified "what" is interested in, use:
    - list_metrics: full text search on everything related to metrics
    - list_nodes: search by hostname, machine guide, node id
    
  - If the user specified "when" is interested in, use:
    - find_anomalous_metrics: search for anomalies in metrics
      This will provide metric names, or labels, or nodes that are anomalous, which you can then use to find more data.
      Netdata ML is real-time. The anomalies are detected in real-time during data collection and are stored in the database.
      The percentage given represents the % of samples collected found anomalous. Depending on the time range queried, this may be very low,
      but still it may indicate strong anomalies when narrowed down to a specific event.
    - list_alert_transitions: search for alert transitions in the given time range
      You may need to expand the time range to the past, to find already raised alerts during the time range.
  
2. FIND DATA TO ANSWER THE QUESTION
  - Once you have identified which part of the infrastructure is relevant, use more tools to gather data.
  - Pay attention to the parameters of each tool, as they may require specific inputs.
  - For live information (processes, containers, sockets, etc) use Netdata functions.
  - For logs queries, use Netdata functions.

## FORMATTING GUIDELINES
**CRITICAL**: Always use proper markdown formatting in your responses:

- Use **bold** and *italic* for emphasis
- Use proper markdown lists with dashes or numbers for structured information
- For tree structures, node hierarchies, or ASCII diagrams, ALWAYS wrap them in code blocks with triple backticks
- Use inline code formatting for technical terms, commands, and values
- Use > blockquotes for important notes or warnings
- Use tables when presenting structured data
- Use headings (##, ###) to organize your response

**Example of proper tree formatting:**
Use triple backticks to wrap tree structures:
📡 Streaming Path:
└── parent (root)
    ├── child1
    └── child2

## RESPONSE STYLE
After your <thinking> analysis, provide your response following your existing style guidelines and the formatting guidelines above.

CRITICAL: Never skip the <thinking> section. Even for simple queries, show your reasoning process. This transparency helps users understand your analysis methodology and builds confidence in your conclusions.`;
        
        // Load last used system prompt from localStorage or use default
        this.lastSystemPrompt = localStorage.getItem('lastSystemPrompt') || this.defaultSystemPrompt;
        
        this.initializeUI();
        
        // Delay resizable initialization to ensure DOM is ready
        setTimeout(() => {
            this.initializeResizable();
        }, 0);
        
        // Clear current chat ID to always start fresh
        localStorage.removeItem('currentChatId');
        
        this.loadSettings();
        this.initializeDefaultLLMProvider();
        this.initializeDefaultMCPServer();
        
        // Track if user has interacted with chat selection
        this.userHasSelectedChat = false;
        
        // After all initialization, create default chat if needed
        // Use a longer timeout to give user time to select a chat
        this.defaultChatTimeout = setTimeout(() => {
            // Only create default chat if user hasn't selected one
            if (!this.userHasSelectedChat && !this.currentChatId) {
                this.createDefaultChatIfNeeded();
            }
        }, 500); // Increased from 100ms to 500ms
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
     *     this.autoSave(this.currentChatId);
     * }
     */
    addMessage(chatId, message) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
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
        
        this.autoSave(chatId);
    }
    
    insertMessage(chatId, index, message) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        chat.messages.splice(index, 0, message);
        chat.updatedAt = new Date().toISOString();
        this.autoSave(chatId);
    }
    
    removeMessage(chatId, index, count = 1) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        chat.messages.splice(index, count);
        chat.updatedAt = new Date().toISOString();
        this.autoSave(chatId);
    }
    
    removeLastMessage(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || !this.hasUserContent(chat)) return;
        
        chat.messages.pop();
        chat.updatedAt = new Date().toISOString();
        this.autoSave(chatId);
    }
    
    /**
     * Check if a chat has any real user content (excluding system messages)
     */
    hasUserContent(chat) {
        if (!chat || !chat.messages) return false;
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
        if (!chat || !chat.messages) return false;
        const userMessages = chat.messages.filter(m => 
            m.type === 'user' && 
            !m.isTitleRequest
        );
        return userMessages.length === 1;
    }
    
    /**
     * Count real user messages (excluding title requests)
     */
    countUserMessages(chat) {
        if (!chat || !chat.messages) return 0;
        return chat.messages.filter(m => 
            m.type === 'user' && 
            !m.isTitleRequest
        ).length;
    }
    
    /**
     * Count real assistant messages (excluding title responses)
     */
    countAssistantMessages(chat) {
        if (!chat || !chat.messages) return 0;
        return chat.messages.filter(m => 
            m.type === 'assistant' && 
            !m.isTitleRequest
        ).length;
    }
    
    /**
     * Auto-save with debouncing for performance
     * Saves only the specific chat that was modified
     */
    autoSave(chatId) {
        if (!chatId) return;
        
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
    
    // Get available models for a provider type from the current LLM provider
    getModelsForProviderType(providerType) {
        // Get models from the first available LLM provider (usually there's only one)
        const provider = Array.from(this.llmProviders.values())[0];
        if (!provider || !provider.availableProviders || !provider.availableProviders[providerType]) {
            return [];
        }
        
        // Convert models to the expected format
        const models = provider.availableProviders[providerType].models || [];
        return models.map(model => {
            if (typeof model === 'string') {
                return {
                    value: model,
                    text: model,
                    contextLimit: this.modelLimits[model] || 4096
                };
            } else if (typeof model === 'object' && model.id) {
                return {
                    value: model.id,
                    text: model.id,
                    contextLimit: model.contextWindow || this.modelLimits[model.id] || 4096
                };
            }
            return null;
        }).filter(Boolean);
    }
    
    // Get model info by model ID from dynamic provider data
    getModelInfo(modelId) {
        if (!modelId) return null;
        
        // Search through all LLM providers' available models
        for (const provider of this.llmProviders.values()) {
            if (!provider.availableProviders) continue;
            
            for (const [providerType, config] of Object.entries(provider.availableProviders)) {
                if (!config.models) continue;
                
                for (const model of config.models) {
                    const id = typeof model === 'string' ? model : model.id;
                    if (id === modelId) {
                        return {
                            value: id,
                            text: id,
                            contextLimit: (typeof model === 'object' && model.contextWindow) 
                                ? model.contextWindow 
                                : this.modelLimits[id] || 4096
                        };
                    }
                }
            }
        }
        return null;
    }

    // Calculate price for a single message based on its model and usage
    calculateMessagePrice(model, usage) {
        if (!usage || !model) return null;
        
        // Extract model name from format "provider:model-name"
        let modelName = model;
        if (modelName.includes(':')) {
            modelName = modelName.split(':')[1];
        }
        
        const pricing = this.modelPricing[modelName];
        if (!pricing) return null;
        
        let totalCost = 0;
        
        const promptTokens = usage.promptTokens || 0;
        const completionTokens = usage.completionTokens || 0;
        const cacheReadTokens = usage.cacheReadInputTokens || 0;
        const cacheCreationTokens = usage.cacheCreationInputTokens || 0;
        
        // For Anthropic models with cache pricing
        if (pricing.cacheWrite !== undefined && pricing.cacheRead !== undefined) {
            totalCost += (promptTokens / 1_000_000) * pricing.input;
            totalCost += (cacheReadTokens / 1_000_000) * pricing.cacheRead;
            totalCost += (cacheCreationTokens / 1_000_000) * pricing.cacheWrite;
            totalCost += (completionTokens / 1_000_000) * pricing.output;
        }
        // For OpenAI models with cache pricing
        else if (pricing.cacheRead !== undefined) {
            const cachedInputTokens = cacheReadTokens + cacheCreationTokens;
            totalCost += (promptTokens / 1_000_000) * pricing.input;
            totalCost += (cachedInputTokens / 1_000_000) * pricing.cacheRead;
            totalCost += (completionTokens / 1_000_000) * pricing.output;
        }
        // For models without cache pricing
        else {
            const allInputTokens = promptTokens + cacheReadTokens + cacheCreationTokens;
            totalCost += (allInputTokens / 1_000_000) * pricing.input;
            totalCost += (completionTokens / 1_000_000) * pricing.output;
        }
        
        return totalCost;
    }

    // Update the chat's cumulative token pricing
    updateChatTokenPricing(chat) {
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
                const model = message.model || chat.model; // Fallback to chat model for old messages
                if (!model) continue;
                
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
                    message.model = chat.model;
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
        
        // Sidebar footer controls
        this.themeToggle = document.getElementById('themeToggle');
        this.themeToggle.addEventListener('click', () => this.toggleTheme());
        this.settingsBtn = document.getElementById('settingsBtn');
        this.settingsBtn.addEventListener('click', () => this.showModal('settingsModal'));
        
        // Chat area
        this.chatTitle = document.getElementById('chatTitle');
        this.chatMcp = document.getElementById('chatMcp');
        this.chatLlm = document.getElementById('chatLlm');
        this.chatMessages = document.getElementById('chatMessages');
        this.chatInput = document.getElementById('chatInput');
        this.sendMessageBtn = document.getElementById('sendMessageBtn');
        this.reconnectMcpBtn = document.getElementById('reconnectMcpBtn');
        
        this.sendMessageBtn.addEventListener('click', () => this.handleSendButtonClick());
        this.reconnectMcpBtn.addEventListener('click', () => this.reconnectCurrentMcp());
        
        // Copy metrics button
        this.copyMetricsBtn = document.getElementById('copyMetricsBtn');
        this.copyMetricsBtn.addEventListener('click', () => this.copyConversationMetrics());
        
        // Summarize conversation button
        this.summarizeBtn = document.getElementById('summarizeBtn');
        this.summarizeBtn.addEventListener('click', () => this.summarizeConversation());
        
        // Include all tools toggle button - three state: all on, all off, mixed
        this.globalToolToggleBtn = document.getElementById('globalToolToggleBtn');
        this.globalToolToggleBtn.addEventListener('click', () => {
            this.cycleGlobalToolToggle();
        });
        
        // Generate title button
        this.generateTitleBtn = document.getElementById('generateTitleBtn');
        this.generateTitleBtn.addEventListener('click', () => this.handleGenerateTitleClick());
        
        // Model and MCP server dropdowns
        this.llmModelBtn = document.getElementById('llmModelBtn');
        this.llmModelDropdown = document.getElementById('llmModelDropdown');
        this.currentModelText = document.getElementById('currentModelText');
        this.mcpServerBtn = document.getElementById('mcpServerBtn');
        this.mcpServerDropdown = document.getElementById('mcpServerDropdown');
        this.currentMcpText = document.getElementById('currentMcpText');
        
        this.llmModelBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleDropdown('llmModelDropdown');
        });
        
        this.mcpServerBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleDropdown('mcpServerDropdown');
        });
        
        // Close dropdowns when clicking outside
        document.addEventListener('click', () => {
            this.llmModelDropdown.style.display = 'none';
            this.mcpServerDropdown.style.display = 'none';
            if (this.temperatureDropdown) {
                this.temperatureDropdown.style.display = 'none';
            }
        });
        
        this.chatInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage();
            }
        });
        
        // Update send button state when input changes
        this.chatInput.addEventListener('input', () => {
            this.updateSendButton();
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
        
        // Temperature control dropdown
        this.temperatureBtn = document.getElementById('temperatureBtn');
        this.temperatureDropdown = document.getElementById('temperatureDropdown');
        this.temperatureSlider = document.getElementById('temperatureSlider');
        this.currentTempText = document.getElementById('currentTempText');
        this.tempValueLabel = document.getElementById('tempValueLabel');
        this.temperatureControl = this.temperatureBtn; // For compatibility
        this.temperatureValue = this.currentTempText; // For compatibility
        
        // Initialize temperature display
        this.updateTemperatureDisplay(0.7);
        
        // Temperature button click
        this.temperatureBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.toggleDropdown('temperatureDropdown');
        });
        
        // Temperature slider events
        this.temperatureSlider.addEventListener('input', (e) => {
            const temp = parseFloat(e.target.value);
            this.updateTemperatureDisplay(temp);
        });
        
        this.temperatureSlider.addEventListener('change', (e) => {
            const temp = parseFloat(e.target.value);
            this.updateTemperatureDisplay(temp);
            this.saveTemperatureForChat(temp);
        });
        
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
        this.systemPromptTextarea = document.getElementById('systemPromptTextarea');
        this.closeSystemPromptBtn = document.getElementById('closeSystemPromptBtn');
        this.systemPromptBackdrop = document.getElementById('systemPromptBackdrop');
        this.cancelSystemPromptBtn = document.getElementById('cancelSystemPromptBtn');
        this.saveSystemPromptBtn = document.getElementById('saveSystemPromptBtn');
        this.resetToDefaultPromptBtn = document.getElementById('resetToDefaultPromptBtn');
        
        this.closeSystemPromptBtn.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.systemPromptBackdrop.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.cancelSystemPromptBtn.addEventListener('click', () => this.hideModal('systemPromptModal'));
        this.saveSystemPromptBtn.addEventListener('click', () => this.saveSystemPrompt());
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
        // Get default model for fallback
        let defaultModel = null;
        try {
            const lastConfig = localStorage.getItem('lastChatConfig');
            if (lastConfig) {
                const config = JSON.parse(lastConfig);
                defaultModel = config.model;
            }
        } catch {
            // Ignore
        }
        
        // Validate each chat's model
        for (const [chatId, chat] of this.chats) {
            if (chat.model && chat.llmProviderId) {
                const provider = this.llmProviders.get(chat.llmProviderId);
                if (provider && provider.availableProviders) {
                    // Check if the model exists
                    let modelExists = false;
                    const [providerType, modelName] = chat.model.includes(':') ? 
                        chat.model.split(':') : ['', chat.model];
                    
                    if (providerType && provider.availableProviders[providerType]) {
                        const models = provider.availableProviders[providerType].models || [];
                        modelExists = models.some(m => {
                            const mId = typeof m === 'string' ? m : m.id;
                            return mId === modelName;
                        });
                    }
                    
                    if (!modelExists) {
                        // Use default or first available
                        let fallbackModel = defaultModel;
                        if (!fallbackModel || !this.isModelValid(fallbackModel, provider)) {
                            const firstProvider = Object.keys(provider.availableProviders)[0];
                            const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                            if (firstModel) {
                                const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                                fallbackModel = `${firstProvider}:${modelId}`;
                            }
                        }
                        
                        console.warn(`Chat ${chatId} has invalid model ${chat.model}, resetting to ${fallbackModel || 'none (no valid model found)'}`);
                        
                        if (fallbackModel) {
                            chat.model = fallbackModel;
                            
                            // Save the updated chat
                            this.saveChatToStorage(chatId);
                            
                            // Update UI if this is the current chat
                            if (chatId === this.currentChatId) {
                                this.updateModelDisplay(chat);
                            }
                        }
                    }
                }
            }
        }
    }
    
    isModelValid(model, provider) {
        if (!model || !provider || !provider.availableProviders) return false;
        
        const [providerType, modelName] = model.includes(':') ? 
            model.split(':') : ['', model];
        
        if (!providerType || !provider.availableProviders[providerType]) return false;
        
        const models = provider.availableProviders[providerType].models || [];
        return models.some(m => {
            const mId = typeof m === 'string' ? m : m.id;
            return mId === modelName;
        });
    }
    
    toggleDropdown(dropdownId) {
        const dropdown = document.getElementById(dropdownId);
        const isVisible = dropdown.style.display === 'block';
        
        // Close all dropdowns
        this.llmModelDropdown.style.display = 'none';
        this.mcpServerDropdown.style.display = 'none';
        if (this.temperatureDropdown) {
            this.temperatureDropdown.style.display = 'none';
        }
        
        // Toggle the requested dropdown
        if (!isVisible) {
            dropdown.style.display = 'block';
            
            // Populate the dropdown based on which one it is
            if (dropdownId === 'llmModelDropdown') {
                this.populateModelDropdown();
            } else if (dropdownId === 'mcpServerDropdown') {
                this.populateMcpDropdown();
            }
        }
    }
    
    populateModelDropdown() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        const provider = this.llmProviders.get(chat.llmProviderId);
        if (!provider || !provider.availableProviders) return;
        
        this.llmModelDropdown.innerHTML = '';
        this.llmModelDropdown.style.width = '600px'; // Make dropdown wider for pricing table with 4 columns
        
        // Add models from all available providers
        Object.entries(provider.availableProviders).forEach(([providerType, config]) => {
            // Add provider header
            const header = document.createElement('div');
            header.className = 'dropdown-header';
            header.textContent = providerType.charAt(0).toUpperCase() + providerType.slice(1);
            this.llmModelDropdown.appendChild(header);
            
            // Add table header for pricing
            const tableHeader = document.createElement('div');
            tableHeader.style.cssText = 'display: flex; padding: 5px 10px; font-size: 11px; color: var(--text-tertiary); border-bottom: 1px solid var(--border-color);';
            tableHeader.innerHTML = `
                <div style="flex: 1;">Model</div>
                <div style="width: 70px; text-align: right;">Input</div>
                <div style="width: 70px; text-align: right;">Cache R</div>
                <div style="width: 70px; text-align: right;">Cache W</div>
                <div style="width: 70px; text-align: right;">Output</div>
            `;
            this.llmModelDropdown.appendChild(tableHeader);
            
            // Add models
            config.models.forEach(model => {
                const modelId = typeof model === 'string' ? model : model.id;
                // Get pricing from the model object itself or from this.modelPricing
                const pricing = (typeof model === 'object' && model.pricing) ? model.pricing : this.modelPricing[modelId];
                
                const item = document.createElement('button');
                item.className = 'dropdown-item';
                item.style.cssText = 'display: flex; align-items: center; padding: 8px 10px; width: 100%; text-align: left;';
                
                // Build pricing display
                let pricingHTML = `<div style="flex: 1; font-weight: 500;">${modelId}</div>`;
                
                if (pricing) {
                    const formatPrice = (price) => price !== undefined && price !== null ? `$${price.toFixed(2)}` : 'N/A';
                    
                    // Input price
                    pricingHTML += `<div style="width: 70px; text-align: right; font-size: 12px; color: var(--text-secondary);">${formatPrice(pricing.input)}</div>`;
                    
                    // Cache read price
                    pricingHTML += `<div style="width: 70px; text-align: right; font-size: 12px; color: var(--text-secondary);">${formatPrice(pricing.cacheRead)}</div>`;
                    
                    // Cache write price (Anthropic only)
                    pricingHTML += `<div style="width: 70px; text-align: right; font-size: 12px; color: var(--text-secondary);">${formatPrice(pricing.cacheWrite)}</div>`;
                    
                    // Output price
                    pricingHTML += `<div style="width: 70px; text-align: right; font-size: 12px; color: var(--text-secondary);">${formatPrice(pricing.output)}</div>`;
                } else {
                    // No pricing info
                    pricingHTML += `<div style="width: 280px; text-align: right; font-size: 12px; color: var(--text-tertiary); font-style: italic;">No pricing data</div>`;
                }
                
                item.innerHTML = pricingHTML;
                
                // Mark active model
                if (chat.model === `${providerType}:${modelId}`) {
                    item.classList.add('active');
                }
                
                item.onclick = () => {
                    this.switchModel(`${providerType}:${modelId}`);
                    this.llmModelDropdown.style.display = 'none';
                };
                
                this.llmModelDropdown.appendChild(item);
            });
            
            // Add divider after each provider (except the last)
            const divider = document.createElement('div');
            divider.className = 'dropdown-divider';
            this.llmModelDropdown.appendChild(divider);
        });
        
        // Remove the last divider
        const lastChild = this.llmModelDropdown.lastChild;
        if (lastChild && lastChild.className === 'dropdown-divider') {
            this.llmModelDropdown.removeChild(lastChild);
        }
    }
    
    populateMcpDropdown() {
        this.mcpServerDropdown.innerHTML = '';
        
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
            
            const chat = this.chats.get(this.currentChatId);
            if (chat && chat.mcpServerId === id) {
                item.classList.add('active');
            }
            
            item.onclick = () => {
                this.switchMcpServer(id);
                this.mcpServerDropdown.style.display = 'none';
            };
            
            this.mcpServerDropdown.appendChild(item);
        }
    }
    
    async switchModel(newModel) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat || chat.model === newModel) return;
        
        // Extract model name for context window lookup
        const modelName = newModel.includes(':') ? newModel.split(':')[1] : newModel;
        
        // Update model limits if we have context window info
        const provider = this.llmProviders.get(chat.llmProviderId);
        if (provider && provider.availableProviders) {
            const [providerType] = newModel.split(':');
            const providerConfig = provider.availableProviders[providerType];
            if (providerConfig) {
                const modelInfo = providerConfig.models.find(m => 
                    (typeof m === 'string' ? m : m.id) === modelName
                );
                if (modelInfo && typeof modelInfo === 'object' && modelInfo.contextWindow) {
                    this.modelLimits[modelName] = modelInfo.contextWindow;
                }
            }
        }
        
        chat.model = newModel;
        this.autoSave(chat.id);
        
        // Update UI
        this.currentModelText.textContent = modelName;
        
        // Update context window indicator
        const { totalTokens } = this.getTokenUsageForChat(this.currentChatId);
        this.updateContextWindowIndicator(totalTokens, newModel);
        
        // Update all token displays including cost
        this.updateAllTokenDisplays();
        
        // Save as default for new chats
        const lastConfig = localStorage.getItem('lastChatConfig');
        let config = {};
        if (lastConfig) {
            try {
                config = JSON.parse(lastConfig);
            } catch {
                // Ignore parse errors
            }
        }
        config.model = newModel;
        config.llmProviderId = chat.llmProviderId;
        config.mcpServerId = chat.mcpServerId;
        localStorage.setItem('lastChatConfig', JSON.stringify(config));
        
        this.addLogEntry('SYSTEM', {
            timestamp: new Date().toISOString(),
            direction: 'info',
            message: `Switched model to ${modelName}`
        });
    }
    
    async switchMcpServer(newServerId) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat || chat.mcpServerId === newServerId) return;
        
        try {
            // Ensure connection to new MCP server
            await this.ensureMcpConnection(newServerId);
            
            chat.mcpServerId = newServerId;
            this.autoSave(chat.id);
            
            // Update UI
            const server = this.mcpServers.get(newServerId);
            this.currentMcpText.textContent = server.name;
            
            // Clear tool inclusion states for the new server (will be populated on next use)
            const chatToolStates = this.toolInclusionStates.get(this.currentChatId);
            if (chatToolStates) {
                chatToolStates.clear();
            }
            
            // Save as default for new chats
            const lastConfig = localStorage.getItem('lastChatConfig');
            let config = {};
            if (lastConfig) {
                try {
                    config = JSON.parse(lastConfig);
                } catch {
                    // Ignore parse errors
                }
            }
            config.mcpServerId = newServerId;
            config.llmProviderId = chat.llmProviderId;
            config.model = chat.model;
            localStorage.setItem('lastChatConfig', JSON.stringify(config));
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `Switched to MCP server: ${server.name}`
            });
            
        } catch (error) {
            this.showError(`Failed to switch MCP server: ${error.message}`);
        }
    }

    showSystemPromptModal() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Load the current system prompt for this chat
        this.systemPromptTextarea.value = chat.systemPrompt || this.defaultSystemPrompt;
        this.showModal('systemPromptModal');
    }

    saveSystemPrompt() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        const newPrompt = this.systemPromptTextarea.value.trim();
        if (!newPrompt) {
            this.showError('System prompt cannot be empty');
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
        this.tokenUsageHistory.set(this.currentChatId, {
            requests: [],
            model: chat.model
        });
        
        // Save settings
        this.saveSettings();
        
        // Reload the chat
        this.loadChat(this.currentChatId);
        
        // Hide modal
        this.hideModal('systemPromptModal');
        
        // Show notification
        this.addSystemMessage('System prompt updated. Conversation has been reset.');
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
            const newWidth = Math.max(200, Math.min(650, currentWidth + (-delta)));
            logPanel.style.width = newWidth + 'px';
            this.savePaneSizes();
        }, logPanel);

        // Chat input resize
        const chatInputContainer = document.getElementById('chatInputContainer');
        const chatInputResize = document.getElementById('chatInputResize');
        
        this.setupResize(chatInputResize, 'vertical', (delta) => {
            const currentHeight = chatInputContainer.offsetHeight;
            const newHeight = Math.max(80, Math.min(300, currentHeight - delta));
            chatInputContainer.style.height = newHeight + 'px';
            
            // No need to manually adjust textarea height anymore since it uses flexbox
            
            this.savePaneSizes();
        });
    }

    setupResize(handle, direction, onResize, element) {
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
            if (!isResizing) return;
            
            const currentPos = direction === 'horizontal' ? e.clientX : e.clientY;
            const delta = currentPos - startPos;
            startPos = currentPos;
            
            onResize(delta);
        };
        
        const stopResize = () => {
            if (!isResizing) return;
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

    savePaneSizes() {
        const logPanel = document.getElementById('logPanel');
        const sizes = {
            chatSidebar: document.getElementById('chatSidebar').offsetWidth,
            logPanel: logPanel.classList.contains('collapsed') ? 40 : logPanel.offsetWidth,
            logPanelCollapsed: logPanel.classList.contains('collapsed'),
            chatInput: document.getElementById('chatInputContainer').offsetHeight
        };
        localStorage.setItem('paneSizes', JSON.stringify(sizes));
    }

    loadPaneSizes() {
        const savedSizes = localStorage.getItem('paneSizes');
        if (savedSizes) {
            try {
                const sizes = JSON.parse(savedSizes);
                
                if (sizes.chatSidebar && !this.chatSidebar.classList.contains('collapsed')) {
                    document.getElementById('chatSidebar').style.width = sizes.chatSidebar + 'px';
                }
                
                if (sizes.logPanel) {
                    const logPanel = document.getElementById('logPanel');
                    // Only set width if not collapsed, or if we have a saved non-collapsed state
                    if (!logPanel.classList.contains('collapsed') || !sizes.logPanelCollapsed) {
                        logPanel.style.width = sizes.logPanel + 'px';
                    }
                }
                
                if (sizes.chatInput) {
                    const container = document.getElementById('chatInputContainer');
                    container.style.height = sizes.chatInput + 'px';
                    
                    // No need to manually adjust textarea height anymore since it uses flexbox
                }
            } catch (e) {
                console.error('Failed to load pane sizes:', e);
            }
        }
    }

    toggleLog() {
        const isCollapsed = this.logPanel.classList.toggle('collapsed');
        this.toggleLogBtn.innerHTML = isCollapsed ? '<i class="fas fa-chevron-left"></i>' : '<i class="fas fa-chevron-right"></i>';
        this.expandLogBtn.style.display = isCollapsed ? 'block' : 'none';
        localStorage.setItem('logCollapsed', isCollapsed);
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
                localStorage.setItem('chatSidebarWidth', currentWidth);
            }
            // Override any inline width when collapsed
            this.chatSidebar.style.width = '';
        } else {
            // Restore previous width
            const savedWidth = localStorage.getItem('chatSidebarWidth') || '280';
            this.chatSidebar.style.width = savedWidth + 'px';
        }
        
        localStorage.setItem('chatSidebarCollapsed', isCollapsed);
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

    showError(message) {
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
            message: message
        });
        
        // Also show in chat if there's an active chat
        if (this.currentChatId) {
            const messageDiv = document.createElement('div');
            messageDiv.className = 'message error';
            messageDiv.innerHTML = `<i class="fas fa-times-circle"></i> ${message}`;
            this.chatMessages.appendChild(messageDiv);
            this.scrollToBottom();
        }
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
    
    showErrorWithRetry(message, retryCallback, buttonLabel = 'Retry') {
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
            message: message
        });
        
        // Also show in chat with retry button if there's an active chat
        if (this.currentChatId) {
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
            
            this.chatMessages.appendChild(messageDiv);
            this.scrollToBottom();
        }
    }

    addLogEntry(source, entry) {
        const logEntry = {
            ...entry,
            source: source
        };
        this.communicationLog.push(logEntry);
        this.updateLogDisplay(logEntry);
    }

    updateLogDisplay(entry) {
        const entryDiv = document.createElement('div');
        entryDiv.className = 'log-entry';
        
        let directionClass = entry.direction;
        let directionSymbol = '';
        switch(entry.direction) {
            case 'sent': directionSymbol = '→'; break;
            case 'received': directionSymbol = '←'; break;
            case 'error': directionSymbol = '⚠'; break;
            case 'info': directionSymbol = 'ℹ'; break;
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
        const entryId = `log-entry-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
        
        entryDiv.innerHTML = `
            <div class="log-entry-header">
                <div class="log-entry-info">
                    <span class="log-timestamp">${new Date(entry.timestamp).toLocaleTimeString()}</span>
                    <span class="log-source">[${entry.source}]</span>
                    <span class="log-direction ${directionClass}">${directionSymbol}</span>
                </div>
                <button class="btn-copy-log" title="Copy to clipboard" data-entry-id="${entryId}"><i class="fas fa-clipboard"></i></button>
            </div>
            ${metadataHtml}
            <div class="log-message" id="${entryId}">${this.formatLogMessage(entry.message)}</div>
        `;
        
        // Add click handler for copy button
        const copyBtn = entryDiv.querySelector('.btn-copy-log');
        copyBtn.addEventListener('click', () => {
            const messageElement = document.getElementById(entryId);
            const textToCopy = messageElement.textContent || messageElement.innerText;
            this.copyToClipboard(textToCopy, copyBtn);
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
    async redoFromMessage(messageIndex) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Find the message to redo from
        const message = chat.messages[messageIndex];
        if (!message) return;
        
        // Get the MCP connection and provider
        const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
        const proxyProvider = this.llmProviders.get(chat.llmProviderId);
        
        if (!mcpConnection || !proxyProvider || !chat.model) {
            this.showError('Cannot redo: MCP server or LLM provider not available');
            return;
        }
        
        // Parse model format
        const [providerType, modelName] = chat.model.split(':');
        if (!providerType || !modelName) {
            this.showError('Invalid model format in chat');
            return;
        }
        
        // Create the LLM provider
        const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
        provider.onLog = proxyProvider.onLog;
        
        try {
            if (message.type === 'user') {
                // Redo from user message - truncate everything AFTER this message
                // Keep all history up to and including this message
                chat.messages = chat.messages.slice(0, messageIndex + 1);
                this.autoSave(chat.id);
                this.loadChat(this.currentChatId);
                
                // Show loading spinner AFTER loadChat to prevent it from being cleared
                this.processRenderEvent({ type: 'show-spinner' });
                
                // Resend the user message with full prior context
                await this.processMessageWithTools(chat, mcpConnection, provider, message.content);
            } else if (message.type === 'assistant') {
                // Redo from assistant message - find the user message that triggered it
                let triggeringUserMessage = null;
                let triggeringUserIndex = -1;
                
                // Find the most recent user message before this assistant message
                for (let i = messageIndex - 1; i >= 0; i--) {
                    if (chat.messages[i].type === 'user' && !chat.messages[i].isTitleRequest) {
                        triggeringUserMessage = chat.messages[i];
                        triggeringUserIndex = i;
                        break;
                    }
                }
                
                if (triggeringUserMessage) {
                    // Truncate to remove this assistant message and everything after
                    // Keep everything up to the triggering user message
                    chat.messages = chat.messages.slice(0, triggeringUserIndex + 1);
                    this.autoSave(chat.id);
                    this.loadChat(this.currentChatId);
                    
                    // Show loading spinner AFTER loadChat to prevent it from being cleared
                    this.processRenderEvent({ type: 'show-spinner' });
                    
                    // Resend the triggering user message with full prior context
                    await this.processMessageWithTools(chat, mcpConnection, provider, triggeringUserMessage.content);
                } else {
                    this.showError('Cannot find the user message that triggered this response');
                }
            } else {
                // For any other message type, show error (shouldn't happen with our button logic)
                this.showError('Redo is only available for user and assistant messages');
            }
        } catch (error) {
            this.showError(`Redo failed: ${error.message}`);
        } finally {
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' });
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
        
        // Always create a new chat on startup instead of loading the last one
        this.shouldCreateDefaultChat = true;
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
                        this.validateAndAddChat(chatData);
                    }
                } catch (e) {
                    console.error(`Failed to load chat from key ${key}:`, e);
                }
            }
        }
        
        this.updateChatSessions();
    }
    
    
    validateAndAddChat(chat) {
        // Validate that the chat's model still exists
        if (chat.model && chat.llmProviderId) {
            const provider = this.llmProviders.get(chat.llmProviderId);
            if (provider && provider.availableProviders) {
                // Check if the model exists in the provider's available models
                let modelExists = false;
                const [providerType, modelName] = chat.model.includes(':') ? 
                    chat.model.split(':') : ['', chat.model];
                
                if (providerType && provider.availableProviders[providerType]) {
                    const models = provider.availableProviders[providerType].models || [];
                    modelExists = models.some(m => {
                        const mId = typeof m === 'string' ? m : m.id;
                        return mId === modelName;
                    });
                }
                
                if (!modelExists) {
                    // Try to get default model from lastChatConfig
                    let defaultModel = null;
                    try {
                        const lastConfig = localStorage.getItem('lastChatConfig');
                        if (lastConfig) {
                            const config = JSON.parse(lastConfig);
                            if (config.model && config.llmProviderId === chat.llmProviderId) {
                                defaultModel = config.model;
                            }
                        }
                    } catch {
                        // Ignore
                    }
                    
                    // If no default, use first available model
                    if (!defaultModel) {
                        const firstProvider = Object.keys(provider.availableProviders)[0];
                        const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                        if (firstModel) {
                            const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                            defaultModel = `${firstProvider}:${modelId}`;
                        }
                    }
                    
                    console.warn(`Chat ${chat.id} has invalid model ${chat.model}, resetting to ${defaultModel || 'none (no valid model found)'}`);
                    
                    if (defaultModel) {
                        chat.model = defaultModel;
                    }
                }
            }
        }
        
        this.chats.set(chat.id, chat);
    }
    
    saveAllChatsToSplitStorage() {
        // Save all chats to individual keys
        for (const [chatId, chat] of this.chats) {
            if (chat.isSaved !== false) {
                const key = `mcp_chat_${chatId}`;
                localStorage.setItem(key, JSON.stringify(chat));
            }
        }
    }

    async initializeDefaultLLMProvider() {
        // Always fetch models even if we have providers (to get fresh model list)
        // Check if we already have LLM providers
        const hasProviders = this.llmProviders.size > 0;

        // Auto-detect the proxy URL from the current origin
        const proxyUrl = window.location.origin;
        
        console.log('Fetching models from:', `${proxyUrl}/models`);
        
        try {
            // Fetch available models from the same origin
            const response = await fetch(`${proxyUrl}/models`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const data = await response.json();
            const providers = data.providers || {};
            
            console.log('Received providers data from proxy:', providers);
            
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
                    proxyUrl: proxyUrl,
                    availableProviders: providers,
                    onLog: (logEntry) => this.addLogEntry('Local LLM Proxy', logEntry)
                };
                this.llmProviders.set(providerId, provider);
            }
            
            // Always populate modelLimits and pricing from fresh data
            this.modelPricing = {};
            Object.entries(providers).forEach(([providerType, config]) => {
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
            this.updateNewChatSelectors();
            
            // Validate all existing chats have valid models
            this.validateChatModels();
            
            // Update token displays for the current chat now that pricing is loaded
            if (this.currentChatId) {
                this.updateAllTokenDisplays();
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
        // Only create if we should and have the necessary components
        if (!this.shouldCreateDefaultChat) return;
        
        // Check if there's already an unsaved chat
        const hasUnsavedChat = Array.from(this.chats.values()).some(chat => chat.isSaved === false);
        if (hasUnsavedChat) return;
        
        // Make sure we have at least one MCP server and LLM provider
        if (this.mcpServers.size === 0 || this.llmProviders.size === 0) return;
        
        // Get the last used configuration or use defaults
        let mcpServerId, llmProviderId, model;
        
        try {
            const lastConfig = localStorage.getItem('lastChatConfig');
            if (lastConfig) {
                const config = JSON.parse(lastConfig);
                // Verify these still exist
                if (this.mcpServers.has(config.mcpServerId) && this.llmProviders.has(config.llmProviderId)) {
                    mcpServerId = config.mcpServerId;
                    llmProviderId = config.llmProviderId;
                    model = config.model;
                }
            }
        } catch (e) {
            // Ignore errors
        }
        
        // Use defaults if needed
        if (!mcpServerId) mcpServerId = this.mcpServers.keys().next().value;
        if (!llmProviderId) llmProviderId = this.llmProviders.keys().next().value;
        
        // Get first available model if not specified
        if (!model) {
            const provider = this.llmProviders.get(llmProviderId);
            if (provider && provider.availableProviders) {
                const firstProvider = Object.keys(provider.availableProviders)[0];
                const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                if (firstModel) {
                    const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                    model = `${firstProvider}:${modelId}`;
                }
            }
        }
        
        if (!model) return;
        
        // Create an unsaved chat
        await this.createNewChat({
            mcpServerId: mcpServerId,
            llmProviderId: llmProviderId,
            model: model,
            title: 'New Chat',
            isSaved: false
        });
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
                this.updateNewChatSelectors();
                
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
                this.updateNewChatSelectors();
                
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
        if (this.currentChatId) {
            localStorage.setItem('currentChatId', this.currentChatId);
        }
    }
    
    saveChatToStorage(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || chat.isSaved === false) return;
        
        const key = `mcp_chat_${chatId}`;
        try {
            localStorage.setItem(key, JSON.stringify(chat));
        } catch (e) {
            console.error(`Failed to save chat ${chatId}:`, e);
            if (e.name === 'QuotaExceededError') {
                this.showError('Storage quota exceeded. Consider deleting old chats.');
            }
        }
    }

    // MCP Server Management
    async addMcpServer() {
        const url = this.mcpServerUrl.value.trim();
        const name = this.mcpServerName.value.trim();
        
        if (!url || !name) {
            this.showError('Please fill in all fields');
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
                name: name,
                url: url,
                connected: true
            };
            
            this.mcpServers.set(serverId, server);
            this.mcpConnections.set(serverId, testClient);
            
            this.saveSettings();
            this.updateMcpServersList();
            this.updateNewChatSelectors();
            
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
            this.showError(`Failed to connect to MCP server: ${error.message}`);
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
            this.updateNewChatSelectors();
            
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
    async testProxyConnection() {
        const proxyUrl = this.llmProxyUrl.value.trim();
        if (!proxyUrl) {
            this.llmProvidersStatus.style.display = 'none';
            return;
        }
        
        this.llmProvidersInfo.innerHTML = '<div style="color: var(--text-muted);">Connecting to proxy...</div>';
        this.llmProvidersStatus.style.display = 'block';
        
        try {
            const response = await fetch(`${proxyUrl}/models`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const data = await response.json();
            const providers = data.providers || {};
            
            if (Object.keys(providers).length === 0) {
                this.llmProvidersInfo.innerHTML = '<div style="color: var(--color-error);">No providers configured in proxy. Please configure API keys in llm-proxy-config.json</div>';
                return;
            }
            
            // Display available providers and models
            let html = '<div style="color: var(--color-success);"><i class="fas fa-check"></i> Connected successfully</div>';
            html += '<div style="margin-top: 8px; font-size: 0.9em;">';
            
            Object.entries(providers).forEach(([provider, config]) => {
                html += `<div style="margin-bottom: 4px;"><strong>${provider}:</strong> ${config.models.length} models available</div>`;
            });
            
            html += '</div>';
            this.llmProvidersInfo.innerHTML = html;
            
            // Store available providers for later use
            this.availableProviders = providers;
            
        } catch (error) {
            this.llmProvidersInfo.innerHTML = `<div style="color: var(--color-error);">Failed to connect: ${error.message}</div>`;
            this.availableProviders = null;
        }
    }

    // This function is no longer needed since LLM providers are auto-configured
    async addLlmProvider() {
        // Kept for backward compatibility but does nothing
    }

    updateLlmProvidersList() {
        // This function is no longer needed since LLM providers are auto-configured
        // but we'll keep it for backward compatibility
    }

    removeLlmProvider(providerId) {
        if (confirm('Remove this LLM provider?')) {
            this.llmProviders.delete(providerId);
            this.saveSettings();
            this.updateLlmProvidersList();
            this.updateNewChatSelectors();
            
            // Check if any chats use this provider
            for (const chat of this.chats.values()) {
                if (chat.llmProviderId === providerId) {
                    chat.llmProviderId = null;
                    // Note: Chat becomes unusable without LLM provider
                }
            }
            this.saveSettings();
        }
    }

    getProviderIcon(type) {
        switch(type) {
            case 'openai': return '<i class="fas fa-robot"></i>';
            case 'anthropic': return '🧠';
            case 'google': return '🔮';
            default: return '💬';
        }
    }

    // Chat Management
    async createNewChatDirectly() {
        // Check if there's an unsaved chat
        const unsavedChat = Array.from(this.chats.values()).find(chat => chat.isSaved === false);
        if (unsavedChat) {
            // Check if we're already in the unsaved chat
            if (this.currentChatId === unsavedChat.id) {
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
            this.showError('Please configure at least one MCP server and LLM provider');
            return;
        }
        
        // Get the last used configuration or use defaults
        let mcpServerId, llmProviderId, model;
        
        try {
            const lastConfig = localStorage.getItem('lastChatConfig');
            if (lastConfig) {
                const config = JSON.parse(lastConfig);
                // Verify these still exist
                if (this.mcpServers.has(config.mcpServerId) && this.llmProviders.has(config.llmProviderId)) {
                    mcpServerId = config.mcpServerId;
                    llmProviderId = config.llmProviderId;
                    model = config.model;
                }
            }
        } catch (e) {
            // Ignore errors
        }
        
        // Use defaults if needed
        if (!mcpServerId) mcpServerId = this.mcpServers.keys().next().value;
        if (!llmProviderId) llmProviderId = this.llmProviders.keys().next().value;
        
        // Get first available model if not specified
        if (!model) {
            const provider = this.llmProviders.get(llmProviderId);
            if (provider && provider.availableProviders) {
                const firstProvider = Object.keys(provider.availableProviders)[0];
                const firstModel = provider.availableProviders[firstProvider]?.models?.[0];
                if (firstModel) {
                    const modelId = typeof firstModel === 'string' ? firstModel : firstModel.id;
                    model = `${firstProvider}:${modelId}`;
                }
            }
        }
        
        if (!model) {
            this.showError('No models available');
            return;
        }
        
        // Create an unsaved chat
        await this.createNewChat({
            mcpServerId: mcpServerId,
            llmProviderId: llmProviderId,
            model: model,
            title: 'New Chat',
            isSaved: false
        });
    }
    
    // Legacy method kept for compatibility
    showNewChatModal() {
        // Redirect to direct creation
        this.createNewChatDirectly();
    }

    updateNewChatSelectors() {
        // This function is no longer needed since we create chats directly
        // The New Chat Modal has been removed
    }
    
    updateNewChatModels() {
        // This function is no longer needed since we create chats directly
        // The New Chat Modal has been removed
    }

    async createNewChat(options = {}) {
        // Now only supports programmatic creation since we removed the modal
        const mcpServerId = options.mcpServerId;
        const llmProviderId = options.llmProviderId;
        const selectedModel = options.model;
        let title = options.title || '';
        const isSaved = options.isSaved !== undefined ? options.isSaved : true;
        
        if (!mcpServerId || !llmProviderId) {
            this.showError('Cannot create chat: Missing MCP server or LLM provider');
            return;
        }
        
        if (!selectedModel) {
            this.showError('Please select a model');
            return;
        }
        
        // Ensure MCP connection
        try {
            await this.ensureMcpConnection(mcpServerId);
            // Update server list to show connected status
            this.updateMcpServersList();
        } catch (error) {
            this.showError(`Failed to connect to MCP server: ${error.message}`);
            return;
        }
        
        // Generate title if not provided
        if (!title) {
            const server = this.mcpServers.get(mcpServerId);
            const provider = this.llmProviders.get(llmProviderId);
            title = `${server.name} - ${provider.name}`;
        }
        
        const chatId = `chat_${Date.now()}`;
        
        // Prepare system prompt with timestamp and timezone
        let systemPromptWithTimestamp = this.lastSystemPrompt;
        const currentTimestamp = new Date().toISOString();
        const timezoneInfo = this.getTimezoneInfo();
        const currentDate = new Date();
        const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
        const currentDayName = dayNames[currentDate.getDay()];
        
        systemPromptWithTimestamp += `\n\n## CRITICAL DATE/TIME CONTEXT
Current date and time: ${currentTimestamp}
Current day: ${currentDayName}
Current timezone: ${timezoneInfo.name} (${timezoneInfo.offset})
Current year: ${currentDate.getFullYear()}

IMPORTANT DATE/TIME INTERPRETATION RULES FOR MONITORING DATA:
1. When the user mentions dates without a year (e.g., "January 15", "last month"), use ${currentDate.getFullYear()} as the current year
2. When the user mentions times without a timezone (e.g., "10pm", "14:30"), assume ${timezoneInfo.name} timezone
3. ALL relative references refer to the PAST (this is a monitoring system analyzing historical data):
   - "this morning" = earlier today, before noon
   - "this afternoon" = earlier today, after noon
   - "tonight" = earlier today, evening hours
   - "this Thursday" or "on Thursday" = the most recent Thursday (if today is Thursday and it's past the mentioned time, use today; otherwise use last Thursday)
   - "during the weekend" = the most recent Saturday and Sunday
   - "Monday" or "on Monday" = the most recent Monday
4. IMPORTANT: Distinguish between complete time periods and relative offsets:
   - "yesterday" = the complete 24-hour period before today at 00:00 (e.g., if today is Jan 15, yesterday is Jan 14 00:00 to Jan 14 23:59:59)
   - "last week" = the complete previous calendar week (Monday 00:00 to Sunday 23:59:59)
   - "last hour" = the complete previous clock hour (e.g., if it's 14:35, last hour is 13:00 to 13:59:59)
   - "last month" = the complete previous calendar month (e.g., if it's January, last month is December 1-31)
   - BUT: "7 days ago", "3 hours ago", "2 weeks ago" = exactly that amount of time before now
5. Never interpret relative references as future times - users are always asking about historical monitoring data

All date/time interpretations must be based on the current date/time context provided above, NOT on your training data.`;
        
        const chat = {
            id: chatId,
            title: title,
            mcpServerId: mcpServerId,
            llmProviderId: llmProviderId,
            model: selectedModel, // Selected model for this chat
            messages: [
                {
                    role: 'system',
                    content: systemPromptWithTimestamp,
                    timestamp: new Date().toISOString()
                }
            ],
            temperature: 0.7, // Default temperature
            systemPrompt: systemPromptWithTimestamp, // System prompt with timestamp
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            currentTurn: 0, // Track conversation turns
            toolInclusionMode: 'cached', // 'auto', 'all-on', 'all-off', 'manual', or 'cached'
            isSaved: isSaved, // Track whether this chat has been saved to localStorage
            titleGenerated: false // Track whether title has been generated
        };
        
        this.chats.set(chatId, chat);
        this.currentChatId = chatId;
        
        // Initialize token usage history for new chat
        this.tokenUsageHistory.set(chatId, {
            requests: [],
            model: selectedModel
        });
        
        // Save the selections for next time
        localStorage.setItem('lastChatConfig', JSON.stringify({
            mcpServerId: mcpServerId,
            llmProviderId: llmProviderId,
            model: selectedModel
        }));
        
        // Only save settings if this is a saved chat
        if (isSaved) {
            this.saveSettings();
        }
        this.updateChatSessions();
        this.loadChat(chatId);
        
        return chatId;
    }

    updateChatSessions() {
        this.chatSessions.innerHTML = '';
        
        if (this.chats.size === 0) {
            this.chatSessions.innerHTML = '<div class="text-center text-muted mt-2">No chats yet</div>';
            return;
        }
        
        const sortedChats = Array.from(this.chats.values())
            .filter(chat => chat.isSaved !== false) // Only show saved chats
            .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
        
        for (const chat of sortedChats) {
            const sessionDiv = document.createElement('div');
            sessionDiv.className = `chat-session-item ${chat.id === this.currentChatId ? 'active' : ''}`;
            
            // Get model display name
            let modelDisplay = 'No model';
            if (chat.model) {
                // Extract model name from format "provider:model-name"
                const parts = chat.model.split(':');
                modelDisplay = parts.length > 1 ? parts[1] : chat.model;
            }
            
            // Calculate context usage from token history
            let contextInfo = '';
            const tokenHistory = this.getTokenUsageForChat(chat.id);
            if (tokenHistory.totalTokens > 0 && chat.model) {
                // Extract model name from format "provider:model-name" if needed
                let modelName = chat.model;
                if (modelName && modelName.includes(':')) {
                    modelName = modelName.split(':')[1];
                }
                const limit = this.modelLimits[modelName] || 4096;
                const percentage = Math.round((tokenHistory.totalTokens / limit) * 100);
                const contextK = (tokenHistory.totalTokens / 1000).toFixed(1);
                contextInfo = ` • ${contextK}k/${(limit/1000).toFixed(0)}k`;
            }
            
            sessionDiv.innerHTML = `
                <div class="session-content" onclick="app.loadChat('${chat.id}')">
                    <div class="session-title" title="${chat.title}">${chat.title}</div>
                    <div class="session-meta">
                        <span>${modelDisplay}${contextInfo}</span>
                        <span>${new Date(chat.updatedAt).toLocaleDateString()}</span>
                    </div>
                </div>
                <button class="btn-delete-chat" onclick="event.stopPropagation(); app.deleteChat('${chat.id}')" title="Delete chat">
                    <i class="fas fa-trash-alt"></i>
                </button>
            `;
            
            this.chatSessions.appendChild(sessionDiv);
        }
    }

    loadChat(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        // Mark that user has selected a chat
        this.userHasSelectedChat = true;
        
        // Cancel any pending default chat creation
        if (this.defaultChatTimeout) {
            clearTimeout(this.defaultChatTimeout);
            this.defaultChatTimeout = null;
        }
        
        this.currentChatId = chatId;
        this.updateChatSessions();
        
        // Migrate old chat data if needed
        if (!chat.totalTokensPrice || !chat.perModelTokensPrice) {
            this.migrateTokenPricing(chat);
        }
        
        // Initialize token usage history for this chat if it doesn't exist
        if (!this.tokenUsageHistory.has(chatId)) {
            this.tokenUsageHistory.set(chatId, {
                requests: [],
                model: chat.model
            });
            
            // Rebuild token history from saved messages
            let reconstructedRequests = [];
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
        
        const server = this.mcpServers.get(chat.mcpServerId);
        const provider = this.llmProviders.get(chat.llmProviderId);
        
        // Update UI
        this.chatTitle.textContent = chat.title;
        this.chatMcp.textContent = server ? server.name : 'MCP: Not found';
        if (provider) {
            // Parse the model format "provider:model"
            const modelDisplay = chat.model ? chat.model.split(':')[1] || chat.model : 'No model selected';
            this.chatLlm.textContent = modelDisplay;
        } else {
            this.chatLlm.textContent = 'Model: Not found';
        }
        
        // Update dropdown buttons
        if (chat.model) {
            const modelName = chat.model.includes(':') ? chat.model.split(':')[1] : chat.model;
            this.currentModelText.textContent = modelName;
        } else {
            this.currentModelText.textContent = 'Select Model';
        }
        
        if (server) {
            this.currentMcpText.textContent = server.name;
        } else {
            this.currentMcpText.textContent = 'Select MCP';
        }
        
        // Enable/disable input based on server and provider availability
        if (server && provider && this.mcpConnections.has(chat.mcpServerId)) {
            this.chatInput.disabled = false;
            this.sendMessageBtn.disabled = false;
            this.chatInput.placeholder = 'Ask about your Netdata metrics...';
            // Hide reconnect button if shown
            const reconnectBtn = document.getElementById('reconnectMcpBtn');
            if (reconnectBtn) {
                reconnectBtn.style.display = 'none';
            }
            // Show temperature control and context window
            this.temperatureControl.style.display = 'flex';
            const indicator = document.getElementById('contextWindowIndicator');
            if (indicator) {
                indicator.style.display = 'flex';
            }
            const temp = chat.temperature || 0.7;
            this.temperatureSlider.value = temp;
            this.updateTemperatureDisplay(temp);
        } else {
            this.chatInput.disabled = true;
            this.sendMessageBtn.disabled = true;
            this.temperatureControl.style.display = 'flex';
            if (!server) {
                this.chatInput.placeholder = 'MCP server not found';
            } else if (!provider) {
                this.chatInput.placeholder = 'LLM provider not found';
            } else if (!this.mcpConnections.has(chat.mcpServerId)) {
                this.chatInput.placeholder = 'MCP server disconnected - click Reconnect';
                // Show reconnect button
                this.showReconnectButton(chat.mcpServerId);
            } else {
                this.chatInput.placeholder = 'MCP server or LLM provider not available';
            }
            // Still show context window even when disabled
            const indicator = document.getElementById('contextWindowIndicator');
            if (indicator) {
                indicator.style.display = 'flex';
            }
        }
        
        // Load messages
        this.chatMessages.innerHTML = '';
        this.currentAssistantGroup = null; // Reset any current group
        this.currentStepInTurn = 1; // Initialize step counter
        this.lastDisplayedTurn = 0; // Track last displayed turn for separators
        this.currentContextWindow = 0; // Reset context window counter for delta calculation
        
        // Update global toggle UI based on chat's tool inclusion mode
        this.updateGlobalToggleUI();
        
        // Display system prompt as first message
        const systemPromptToDisplay = chat.systemPrompt || this.defaultSystemPrompt;
        this.displaySystemPrompt(systemPromptToDisplay);
        
        // Track previous prompt tokens for delta calculation
        let previousPromptTokens = 0;
        
        let inTitleGeneration = false;
        let inSummaryGeneration = false;
        
        for (let i = 0; i < chat.messages.length; i++) {
            const msg = chat.messages[i];
            if (msg.role === 'system') continue;
            
            // Check if this is the start of title generation
            if (msg.isTitleRequest && msg.type === 'user' && !inTitleGeneration) {
                inTitleGeneration = true;
                // Add the separator for title generation
                const separatorDiv = document.createElement('div');
                separatorDiv.className = 'message system';
                separatorDiv.style.cssText = 'margin: 20px auto; padding: 8px 16px; background: var(--info-color); color: white; text-align: center; border-radius: 4px; max-width: 80%;';
                separatorDiv.innerHTML = '<i class="fas fa-edit"></i> Generating chat title...';
                this.chatMessages.appendChild(separatorDiv);
            }
            
            // Check if this is the start of summary generation
            if (msg.role === 'system-summary' && !inSummaryGeneration) {
                inSummaryGeneration = true;
            }
            
            // Store previous tokens before displaying
            this.previousPromptTokensForDisplay = previousPromptTokens;
            
            // Pass the message index to displayStoredMessage
            this.displayStoredMessage(msg, i);
            
            // Update previous tokens if this message had usage
            if (msg.usage && msg.usage.promptTokens) {
                previousPromptTokens = msg.usage.promptTokens;
            }
            
            // Check if title generation completed
            if (inTitleGeneration && msg.isTitleRequest && msg.type === 'assistant') {
                inTitleGeneration = false;
                // Add ACTION section to the existing Chat Title block
                const titleBlock = this.chatMessages.querySelector('.system-block[data-type="title"]');
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
                const summaryBlocks = this.chatMessages.querySelectorAll('.system-block[data-type="summary"]');
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
                                this.copyToClipboard(msg.content, copyBtn);
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
                this.currentAssistantGroup = null;
            }
        }
        // Clear current group after loading
        this.currentAssistantGroup = null;
        
        // Update global toggle UI to reflect loaded tools state
        this.updateGlobalToggleUI();
        
        // Update tool checkboxes based on mode
        if (chat.toolInclusionMode === 'auto') {
            this.updateToolCheckboxesForAutoMode();
        } else if (chat.toolInclusionMode === 'cached') {
            // In cached mode, all tools are on and disabled
            this.setAllToolStates(true);
            this.disableAllToolCheckboxes();
        }
        
        // Update context window indicator
        const contextTokens = this.calculateContextWindowTokens(chatId);
        const model = chat.model || (provider ? provider.model : null);
        
        // Always calculate token usage properly (respecting summaries)
        if (contextTokens > 0 && model) {
            this.updateContextWindowIndicator(contextTokens, model);
        } else {
            // Show empty context window with the correct model
            this.updateContextWindowIndicator(0, model || chat.model || 'unknown');
        }
        
        // Update cumulative token counters
        const tokenCounters = document.getElementById('tokenCounters');
        if (tokenCounters) {
            tokenCounters.style.display = 'flex';
        }
        this.updateAllTokenDisplays();
        
        // Force scroll to bottom when loading chat (ignore the isAtBottom check)
        requestAnimationFrame(() => {
            this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
        });
        
        // Focus the chat input so user can start typing immediately
        if (this.chatInput && !this.chatInput.disabled) {
            this.chatInput.focus();
        }
    }

    displayStoredMessage(msg, messageIndex) {
        // Handle turn tracking for stored messages
        if (msg.type === 'user' && !msg.isTitleRequest) {
            const chat = this.chats.get(this.currentChatId);
            const msgTurn = msg.turn || 0;
            
            // Add turn separator if we're entering a new turn
            if (msgTurn > 0 && msgTurn !== this.lastDisplayedTurn) {
                this.addTurnSeparator(msgTurn);
                this.currentStepInTurn = 1;
                this.lastDisplayedTurn = msgTurn;
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
            this.processRenderEvent(event);
        }
    }
    
    // Convert a stored message into a sequence of rendering events
    convertMessageToEvents(msg) {
        const events = [];
        
        // Handle messages by role (new system) or type (legacy)
        const messageRole = msg.role || msg.type;
        
        switch(messageRole) {
            case 'user':
                if (msg.isTitleRequest) {
                    events.push({ type: 'user-message', content: `[System Request] ${msg.content}` });
                } else {
                    events.push({ type: 'user-message', content: msg.content });
                }
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
                        events.push({ 
                            type: 'tool-call', 
                            name: toolCall.name, 
                            arguments: toolCall.arguments,
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
                    events.push({ 
                        type: 'tool-result', 
                        name: result.name || result.toolName, 
                        result: result.result,
                        includeInContext: result.includeInContext
                    });
                }
                // Reset assistant group after tool results
                events.push({ type: 'reset-assistant-group' });
                break;
                
            // Handle old format for backward compatibility
            case 'tool-call':
                events.push({ type: 'tool-call', name: msg.toolName, arguments: msg.args });
                break;
            case 'tool-result':
                events.push({ type: 'tool-result', name: msg.toolName, result: msg.result });
                break;
                
            case 'system':
                events.push({ type: 'system-message', content: msg.content });
                break;
                
            case 'error':
                events.push({ type: 'error-message', content: msg.content });
                break;
        }
        
        return events;
    }
    
    // Process a single rendering event
    processRenderEvent(event) {
        switch(event.type) {
            case 'user-message':
                this.currentAssistantGroup = null;
                const chat = this.chats.get(this.currentChatId);
                const turn = event.turn !== undefined ? event.turn : (chat ? chat.currentTurn : 0);
                
                // For live messages, add turn separator if this is a new turn
                if (event.turn === undefined && chat && chat.currentTurn > 0 && chat.currentTurn !== this.lastDisplayedTurn) {
                    this.addTurnSeparator(chat.currentTurn);
                    this.currentStepInTurn = 1; // Reset step counter
                    this.lastDisplayedTurn = chat.currentTurn;
                }
                
                this.renderMessage('user', event.content, event.messageIndex);
                if (turn > 0) {
                    this.addStepNumber(turn, this.currentStepInTurn++);
                }
                break;
                
            case 'assistant-metrics':
                // Simply append metrics to the chat
                this.appendMetricsToChat(event.usage, event.responseTime, event.model);
                break;
                
            case 'assistant-message':
                this.renderMessage('assistant', event.content, event.messageIndex);
                const chat2 = this.chats.get(this.currentChatId);
                const turn2 = event.turn !== undefined ? event.turn : (chat2 ? chat2.currentTurn : 0);
                if (turn2 > 0 && this.currentAssistantGroup) {
                    this.addStepNumber(turn2, this.currentStepInTurn++, this.currentAssistantGroup);
                }
                break;
                
            case 'tool-call':
                // Ensure we have a group
                if (!this.currentAssistantGroup) {
                    this.renderMessage('assistant', '');
                }
                this.addToolCall(event.name, event.arguments, event.includeInContext !== false, event.turn, event.messageIndex);
                break;
                
            case 'tool-result':
                this.addToolResult(event.name, event.result, event.responseTime || 0, event.responseSize || null, event.includeInContext, event.messageIndex);
                break;
                
                
            case 'system-message':
                this.addSystemMessage(event.content);
                break;
                
            case 'system-title-message':
                this.renderMessage('system-title', event.content, event.messageIndex);
                break;
                
            case 'system-summary-message':
                this.renderMessage('system-summary', event.content, event.messageIndex);
                break;
                
            case 'title-message':
                this.renderMessage('title', event.content, event.messageIndex);
                // For title/summary, append metrics AFTER rendering the message
                if (event.usage) {
                    this.appendMetricsToChat(event.usage, event.responseTime, event.model, 'title');
                }
                break;
                
            case 'summary-message':
                this.renderMessage('summary', event.content, event.messageIndex);
                // For title/summary, append metrics AFTER rendering the message
                if (event.usage) {
                    this.appendMetricsToChat(event.usage, event.responseTime, event.model, 'summary');
                }
                break;
                
            case 'accounting-message':
                this.addAccountingNode(event.data);
                break;
                
            case 'error-message':
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                messageDiv.innerHTML = `
                    <div><i class="fas fa-times-circle"></i> ${event.content}</div>
                    <button class="btn btn-warning btn-small" style="margin-top: 8px;">
                        <i class="fas fa-redo"></i> Retry
                    </button>
                `;
                
                const retryBtn = messageDiv.querySelector('button');
                retryBtn.onclick = async () => {
                    retryBtn.disabled = true;
                    retryBtn.textContent = 'Retrying...';
                    
                    // Get the current chat and find the last user message
                    const chat = this.chats.get(this.currentChatId);
                    if (!chat) return;
                    
                    const lastUserMessage = chat.messages.filter(m => m.type === 'user' && !m.isTitleRequest).pop();
                    if (lastUserMessage) {
                        // ATOMIC OPERATION: Remove messages from the error onwards
                        const errorIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === event.content);
                        if (errorIndex !== -1) {
                            this.batchMode = true;
                            try {
                                // Check if we're actually discarding any conversation messages (not just the error)
                                const messagesToDiscard = chat.messages.length - errorIndex - 1; // -1 to exclude the error itself
                                
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
                                this.batchMode = false;
                                this.autoSave(this.currentChatId);
                            }
                            
                            // Reload chat to show cleaned history
                            this.loadChat(this.currentChatId);
                            
                            // Hide any existing spinner before retry
                            this.hideLoadingSpinner();
                            
                            // Retry processing
                            try {
                                const mcpConnection = this.mcpConnections.get(chat.mcpServerId);
                                const proxyProvider = this.llmProviders.get(chat.llmProviderId);
                                
                                if (mcpConnection && proxyProvider && chat.model) {
                                    // Parse model format: "provider:model-name"
                                    const [providerType, modelName] = chat.model.split(':');
                                    if (!providerType || !modelName) {
                                        this.showError('Invalid model format in chat');
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
                                    this.showError('Cannot retry: MCP server or LLM provider not available');
                                }
                            } catch (retryError) {
                                this.showError(`Retry failed: ${retryError.message}`);
                            }
                        }
                    }
                };
                
                this.chatMessages.appendChild(messageDiv);
                break;
                
            case 'show-spinner':
                this.showLoadingSpinner(event.text || 'Thinking...');
                break;
                
            case 'hide-spinner':
                this.hideLoadingSpinner();
                break;
                
            case 'reset-assistant-group':
                // Reset the current assistant group so next content starts fresh
                this.currentAssistantGroup = null;
                break;
        }
        
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }

    deleteChat(chatId) {
        if (!chatId) return;
        
        const chat = this.chats.get(chatId);
        if (!chat) return;
        
        if (confirm(`Delete chat "${chat.title}"?`)) {
            this.chats.delete(chatId);
            
            // Remove from split storage
            const key = `mcp_chat_${chatId}`;
            localStorage.removeItem(key);
            
            this.updateChatSessions();
            
            // If this was the current chat, clear the display
            if (chatId === this.currentChatId) {
                this.currentChatId = null;
                this.chatTitle.textContent = 'Select or create a chat';
                this.chatMcp.textContent = '';
                this.chatLlm.textContent = '';
                this.chatMessages.innerHTML = '';
                this.chatInput.disabled = true;
                this.sendMessageBtn.disabled = true;
                this.reconnectMcpBtn.style.display = 'none';
                this.temperatureControl.style.display = 'flex';
                
                // Reset dropdown buttons
                this.currentModelText.textContent = 'Model';
                this.currentMcpText.textContent = 'MCP Server';
                
                // Show empty context window indicator
                const indicator = document.getElementById('contextWindowIndicator');
                if (indicator) {
                    indicator.style.display = 'flex';
                    this.updateContextWindowIndicator(0, 'unknown');
                }
                // Update all token displays (will show zeros)
                this.updateAllTokenDisplays();
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
    
    // Show system message in chat
    showSystemMessage(message) {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message system';
        messageDiv.textContent = message;
        this.chatMessages.appendChild(messageDiv);
        this.scrollToBottom();
    }
    
    // Handle send/stop button click
    handleSendButtonClick() {
        if (this.isProcessing) {
            // Set flag to stop processing
            this.shouldStopProcessing = true;
            this.showSystemMessage('⏹️ Stopping after current request completes...');
        } else {
            this.sendMessage();
        }
    }
    
    // Messaging
    async sendMessage() {
        const message = this.chatInput.value.trim();
        if (!message) return;
        
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        let mcpConnection;
        try {
            // Try to ensure MCP connection (will reconnect if needed)
            mcpConnection = await this.ensureMcpConnection(chat.mcpServerId);
        } catch (error) {
            this.showError(`Failed to connect to MCP server: ${error.message}`);
            return;
        }
        
        const proxyProvider = this.llmProviders.get(chat.llmProviderId);
        if (!proxyProvider) {
            this.showError('LLM proxy not available');
            return;
        }
        
        // Parse the model selection (format: "provider:model")
        const [providerType, modelName] = chat.model.split(':');
        if (!providerType || !modelName) {
            this.showError('Invalid model selection');
            return;
        }
        
        // Create the actual LLM provider instance
        const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
        provider.onLog = proxyProvider.onLog;
        
        // Clear any current assistant group since we're starting a new conversation turn
        this.currentAssistantGroup = null;
        
        // Disable input and update button to Stop
        this.chatInput.value = '';
        this.chatInput.disabled = true;
        this.isProcessing = true;
        this.shouldStopProcessing = false;
        this.updateSendButton();
        
        // Increment turn counter for new user message
        chat.currentTurn = (chat.currentTurn || 0) + 1;
        
        // CRITICAL: Save message BEFORE displaying it to prevent data loss
        this.addMessage(chat.id, { type: 'user', role: 'user', content: message, turn: chat.currentTurn });
        
        // Now display it - if save failed, user won't see a message that wasn't saved
        this.processRenderEvent({ type: 'user-message', content: message });
        
        // Force scroll to bottom after DOM update
        requestAnimationFrame(() => {
            this.scrollToBottom(true);
        });
        
        // If this is the first message in an unsaved chat, mark it as saved
        if (chat.isSaved === false && this.isFirstUserMessage(chat)) {
            chat.isSaved = true;
            this.updateChatSessions();
        }
        
        // Save this specific chat after adding message
        this.autoSave(this.currentChatId);
        
        // Show loading spinner event
        this.processRenderEvent({ type: 'show-spinner' });
        
        try {
            await this.processMessageWithTools(chat, mcpConnection, provider, message);
            
            // Check if we should generate a title (first user message and got a response)
            if (this.isFirstUserMessage(chat) && this.countAssistantMessages(chat) > 0 && !chat.titleGenerated) {
                // Generate title automatically
                await this.generateChatTitle(chat, mcpConnection, provider, true);
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
                        
                        // Reload chat to remove the error from display
                        this.loadChat(this.currentChatId);
                    }
                    
                    // Continue with a simple continuation prompt
                    this.chatInput.value = 'Please continue';
                    await this.sendMessage();
                }, 'Continue');
                this.addMessage(chat.id, { type: 'error', content: continueMessage });
            } else {
                // Regular error handling
                this.showErrorWithRetry(`Error: ${error.message}`, async () => {
                    // Find and remove only the error message
                    const errorIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === error.message);
                    if (errorIndex !== -1) {
                        this.removeMessage(chat.id, errorIndex, 1);
                        
                        // Reload chat to remove the error from display
                        this.loadChat(this.currentChatId);
                    }
                    
                    // Retry processing without removing any other messages
                    try {
                        // Continue the conversation from where it left off
                        await this.processMessageWithTools(chat, mcpConnection, provider, message);
                        
                        // Save the chat after successful retry
                        chat.updatedAt = new Date().toISOString();
                        this.autoSave(chat.id);
                    } catch (retryError) {
                        this.showError(`Retry failed: ${retryError.message}`);
                    }
                });
                this.addMessage(chat.id, { type: 'error', content: error.message });
            }
        } finally {
            // Remove loading spinner event
            this.processRenderEvent({ type: 'hide-spinner' });
            
            // Clear current assistant group after processing is complete
            this.currentAssistantGroup = null;
            
            chat.updatedAt = new Date().toISOString();
            this.autoSave(chat.id);
            this.chatInput.disabled = false;
            this.isProcessing = false;
            this.shouldStopProcessing = false;
            this.updateSendButton();
            this.chatInput.focus();
            
            // Update tool checkboxes if in auto mode
            if (chat.toolInclusionMode === 'auto') {
                this.updateToolCheckboxesForAutoMode();
            }
        }
    }

    // Get timezone information including name and UTC offset
    getTimezoneInfo() {
        const date = new Date();
        
        // Get UTC offset in minutes
        const offsetMinutes = -date.getTimezoneOffset();
        const offsetHours = Math.floor(Math.abs(offsetMinutes) / 60);
        const offsetMins = Math.abs(offsetMinutes) % 60;
        const offsetSign = offsetMinutes >= 0 ? '+' : '-';
        const offsetString = `UTC${offsetSign}${offsetHours.toString().padStart(2, '0')}:${offsetMins.toString().padStart(2, '0')}`;
        
        // Try to get timezone name
        let timezoneName;
        try {
            // This returns something like "America/New_York"
            timezoneName = Intl.DateTimeFormat().resolvedOptions().timeZone;
        } catch (e) {
            // Fallback to basic timezone string
            timezoneName = date.toString().match(/\(([^)]+)\)/)?.[1] || offsetString;
        }
        
        return {
            name: timezoneName,
            offset: offsetString
        };
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
        for (let i = (includeSystemPrompt && messages[0]?.role === 'system' ? 1 : 0); i < messages.length; i++) {
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

    buildMessagesForAPI(chat, freezeCache = false) {
        const messages = [];
        let cacheControlIndex = null;
        let lastCacheControlIndex = null;
        
        // Basic input validation
        if (!chat.messages || chat.messages.length === 0) {
            const error = 'Chat has no messages';
            console.error('[buildMessagesForAPI] ' + error);
            this.showError(error);
            throw new Error(error);
        }
        
        // Find the last summary checkpoint
        let startIndex = 0;
        let summaryMessage = null;
        for (let i = chat.messages.length - 1; i >= 0; i--) {
            if (chat.messages[i].role === 'summary') {
                summaryMessage = chat.messages[i];
                startIndex = i + 1;  // Start after summary (we'll add it separately)
                break;
            }
        }
        
        // Find the last cache control index if freezing
        if (freezeCache) {
            for (let i = chat.messages.length - 1; i >= startIndex; i--) {
                const msg = chat.messages[i];
                if (msg.cacheControlIndex !== undefined) {
                    lastCacheControlIndex = msg.cacheControlIndex;
                    break;
                }
            }
        }
        
        // Always include the system prompt if it exists (it should be the first message)
        if (chat.messages.length > 0 && chat.messages[0].role === 'system') {
            messages.push(chat.messages[0]);
        }
        
        // If we have a summary, include it in the messages
        // The provider will decide how to handle it (modify system prompt, add as user message, etc.)
        if (summaryMessage) {
            messages.push(summaryMessage);
        }
        
        // Build messages from checkpoint onwards
        let apiMessageIndex = messages.length; // Start after system message if present
        
        for (let i = startIndex; i < chat.messages.length; i++) {
            const msg = chat.messages[i];
            
            // Skip system roles and error messages - they should never be sent to API
            if (['system-title', 'system-summary', 'title', 'summary', 'accounting', 'error'].includes(msg.role)) {
                continue;
            }
            
            // Process based on type/role
            const messageType = msg.type || msg.role;
            
            if (messageType === 'system') {
                // Skip system message if it's the first message (we already added it)
                if (i === 0) continue;
                // Otherwise include it
                messages.push(msg);
                apiMessageIndex++;
            } else if (messageType === 'user') {
                // Pass through the original user message to preserve all its properties
                messages.push(msg);
                apiMessageIndex++;
            } else if (messageType === 'assistant') {
                // Pass through the original assistant message to preserve all its properties
                // The provider will handle content cleaning and formatting
                messages.push(msg);
                apiMessageIndex++;
            } else if (messageType === 'tool-results') {
                // Pass through the original tool-results message
                // The provider will handle formatting and filtering
                messages.push(msg);
                apiMessageIndex++;
            }
        }
        
        // Determine cache control position
        if (freezeCache && lastCacheControlIndex !== null) {
            cacheControlIndex = lastCacheControlIndex;
        } else {
            // Normal behavior - cache control goes on the last message
            cacheControlIndex = messages.length - 1;
        }
        
        // Return messages with tool inclusion context
        return { 
            messages, 
            cacheControlIndex,
            toolInclusionMode: chat.toolInclusionMode || 'auto',
            currentTurn: chat.currentTurn || 0
        };
    }

    async processMessageWithTools(chat, mcpConnection, provider, userMessage) {
        // Build conversation history using the new function
        const { messages, cacheControlIndex, toolInclusionMode, currentTurn } = this.buildMessagesForAPI(chat);
        
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
        
        let attempts = 0;
        // No limit on attempts - let the LLM decide when it's done
        
        // Create assistant group at the start of processing
        // We'll add all content from this conversation turn to this single group
        this.currentAssistantGroup = null;
        
        while (true) {
            attempts++;
            
            // Check if we should stop processing
            if (this.shouldStopProcessing) {
                // Don't show a system message here - we'll handle it in the catch block
                break;
            }
            
            
            // Send to LLM with current temperature
            const temperature = this.getCurrentTemperature();
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, tools, temperature, chat.toolInclusionMode, cacheControlIndex);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Track token usage
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, chat.model || provider.model);
            }
            
            // If no tool calls, display response and finish
            if (!response.toolCalls || response.toolCalls.length === 0) {
                // Emit metrics event first (just like loaded chats)
                this.processRenderEvent({ 
                    type: 'assistant-metrics', 
                    usage: response.usage, 
                    responseTime: llmResponseTime, 
                    model: chat.model || provider.model
                });
                
                if (response.content) {
                    // Then emit assistant message event
                    this.processRenderEvent({ type: 'assistant-message', content: response.content });
                    const assistantMsg = { 
                        type: 'assistant', 
                        role: 'assistant', 
                        content: response.content,
                        usage: response.usage || null,
                        responseTime: llmResponseTime || null,
                        model: chat.model || provider.model,
                        turn: chat.currentTurn
                    };
                    // Use addMessage to calculate price and update cumulative totals
                    this.addMessage(chat.id, assistantMsg);
                    
                    // Track token usage with model information
                    if (response.usage) {
                        const modelUsed = chat.model || provider.model;
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
                        includeInContext: true  // Default to included
                    })),
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: chat.model || provider.model,
                    turn: chat.currentTurn,
                    cacheControlIndex: cacheControlIndex  // Store where cache control was placed
                };
                this.addMessage(chat.id, assistantMessage);
                
                // Track token usage with model information
                if (response.usage) {
                    const modelUsed = chat.model || provider.model;
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
                // Emit metrics event first (just like loaded chats)
                this.processRenderEvent({ 
                    type: 'assistant-metrics', 
                    usage: response.usage, 
                    responseTime: llmResponseTime, 
                    model: chat.model || provider.model
                });
                
                if (response.content) {
                    this.processRenderEvent({ type: 'assistant-message', content: response.content });
                }
                assistantMessageIndex = chat.messages.length - 1;
                
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
                if (cleanedContent || (response.toolCalls && response.toolCalls.length > 0)) {
                    messages.push(assistantMsg);
                }
            }
            
            // Execute tool calls and collect results
            if (response.toolCalls && response.toolCalls.length > 0) {
                // Ensure we have an assistant group even if there was no content
                if (!this.currentAssistantGroup) {
                    this.processRenderEvent({ type: 'assistant-message', content: '' });
                }
                
                const toolResults = [];
                
                for (const toolCall of response.toolCalls) {
                    try {
                        // Show tool call in UI
                        this.processRenderEvent({ 
                            type: 'tool-call', 
                            name: toolCall.name, 
                            arguments: toolCall.arguments,
                            includeInContext: toolCall.includeInContext !== false 
                        });
                        
                        // Show spinner while executing tool
                        this.processRenderEvent({ type: 'show-spinner', text: `Executing ${toolCall.name}...` });
                        
                        // Execute tool and track timing
                        const toolStartTime = Date.now();
                        const rawResult = await mcpConnection.callTool(toolCall.name, toolCall.arguments);
                        const toolResponseTime = Date.now() - toolStartTime;
                        
                        // Hide spinner after tool execution
                        this.processRenderEvent({ type: 'hide-spinner' });
                        
                        // Parse the result to handle MCP's response format
                        const result = this.parseToolResult(rawResult);
                        
                        // Calculate response size
                        const responseSize = typeof result === 'string' 
                            ? result.length 
                            : JSON.stringify(result).length;
                        
                        // Show result in UI with timing and size
                        this.processRenderEvent({ type: 'tool-result', name: toolCall.name, result: result, responseTime: toolResponseTime, responseSize: responseSize, messageIndex: assistantMessageIndex });
                        
                        // Collect result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result: result,
                            includeInContext: true  // Will be updated based on checkbox state
                        });
                        
                    } catch (error) {
                        const errorMsg = `Tool error (${toolCall.name}): ${error.message}`;
                        this.processRenderEvent({ type: 'tool-result', name: toolCall.name, result: { error: errorMsg }, responseTime: 0, responseSize: errorMsg.length, messageIndex: assistantMessageIndex });
                        
                        // Collect error result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result: { error: errorMsg },
                            includeInContext: true  // Will be updated based on checkbox state
                        });
                    }
                }
                
                // Update tool results with inclusion state from checkboxes
                if (toolResults.length > 0) {
                    // Check current checkbox states
                    if (this.currentChatId) {
                        const chat = this.chats.get(this.currentChatId);
                        const chatToolStates = this.toolInclusionStates.get(this.currentChatId) || new Map();
                        
                        // In cached mode, all tools are always included
                        if (chat && chat.toolInclusionMode === 'cached') {
                            toolResults.forEach(tr => {
                                tr.includeInContext = true;
                            });
                        } else {
                            toolResults.forEach(tr => {
                                // Find the tool div by tool name
                                const toolDivs = document.querySelectorAll(`.tool-block[data-tool-name="${tr.name}"]`);
                                for (const toolDiv of toolDivs) {
                                    const checkbox = toolDiv.querySelector('.tool-include-checkbox');
                                    if (checkbox) {
                                        tr.includeInContext = checkbox.checked;
                                        break;
                                    }
                                }
                            });
                        }
                    }
                    
                    // Store all tool results together
                    // NOTE: Tool results are displayed during execution for user feedback,
                    // but the consolidated results are saved here to ensure consistency
                    this.addMessage(chat.id, {
                        type: 'tool-results',
                        toolResults: toolResults,
                        turn: chat.currentTurn
                    });
                    
                    // Filter included results for API
                    const includedResults = toolResults.filter(tr => tr.includeInContext !== false);
                    
                    if (includedResults.length > 0) {
                        // Add standard tool-results message
                        // Providers will convert to their specific format
                        messages.push({
                            role: 'tool-results',
                            toolResults: includedResults.map(tr => ({
                                toolCallId: tr.toolCallId,
                                toolName: tr.name,
                                result: tr.result
                            }))
                        });
                    }
                    
                    // Reset assistant group after tool results so next metrics appears separately
                    this.processRenderEvent({ type: 'reset-assistant-group' });
                    
                    // Show thinking spinner again before next LLM call
                    this.processRenderEvent({ type: 'show-spinner', text: 'Thinking...' });
                }
            }
        }
    }

    renderMessage(role, content, messageIndex) {
        let messageDiv;
        
        if (role === 'assistant') {
            // For assistant messages, we create or use the current group
            if (!this.currentAssistantGroup) {
                // Create new assistant group
                const groupDiv = document.createElement('div');
                groupDiv.className = 'assistant-group';
                
                this.currentAssistantGroup = groupDiv;
                this.chatMessages.appendChild(groupDiv);
            }
            
            // Use the current group as our target
            messageDiv = this.currentAssistantGroup;
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
            redoBtn.onclick = () => this.redoFromMessage(messageIndex);
            messageDiv.style.position = 'relative';
            messageDiv.appendChild(redoBtn);
        }
        
        // Check if content has thinking tags
        const thinkingRegex = /<thinking>([\s\S]*?)<\/thinking>/g;
        const hasThinking = thinkingRegex.test(content);
        
        // Make user messages editable on click
        if (role === 'user' && this.currentChatId) {
            messageDiv.classList.add('editable-message');
        }
        
        // Process content
        if (hasThinking && role === 'assistant') {
            // Reset regex for actual processing
            content.match(/<thinking>([\s\S]*?)<\/thinking>/g);
            
            // Split content into parts
            let parts = [];
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
            parts.forEach((part, index) => {
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
            let systemBlock = this.chatMessages.querySelector('.system-block:last-child');
            const isTitle = role === 'system-title';
            const blockType = isTitle ? 'title' : 'summary';
            
            if (!systemBlock || systemBlock.dataset.type !== blockType) {
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
                        const chat = this.chats.get(this.currentChatId);
                        if (chat) {
                            // Find the most recent summary message
                            for (let i = chat.messages.length - 1; i >= 0; i--) {
                                if (chat.messages[i]?.role === 'summary') {
                                    if (confirm('Delete this summary and replace with accounting record?')) {
                                        this.deleteSummaryMessages(i);
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
                this.chatMessages.appendChild(systemBlock);
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
            
            // Find the LAST system block of this type (most recent)
            const systemBlocks = this.chatMessages.querySelectorAll(`.system-block[data-type="${blockType}"]`);
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
                    this.copyToClipboard(content, copyBtn);
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
            } else {
                console.warn(`No ${blockType} block found for response:`, content);
            }
        } else {
            // Regular message without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            
            if (role === 'assistant' || role === 'user') {
                // Use marked to render markdown for assistant and user messages
                // Configure marked to preserve line breaks and handle whitespace properly
                contentDiv.innerHTML = marked.parse(content, {
                    breaks: true,        // Convert line breaks to <br>
                    gfm: true,          // GitHub Flavored Markdown
                    sanitize: false     // Allow HTML (needed for thinking tags)
                });
            } else {
                // Other messages (system, error) as plain text
                contentDiv.textContent = content;
            }
            
            messageDiv.appendChild(contentDiv);
        }
        
        // Only append non-assistant messages to chat (assistant groups are already appended)
        if (role !== 'assistant') {
            this.chatMessages.appendChild(messageDiv);
        }
        
        // Add edit trigger after element is in DOM
        if (role === 'user' && this.currentChatId && content) {
            const contentDiv = messageDiv.querySelector('.message-content');
            if (contentDiv) {
                this.addEditTrigger(contentDiv, content, 'user');
            }
        }
        
        // Scroll to bottom after DOM update
        // For user messages, force scroll since user just sent it
        requestAnimationFrame(() => {
            if (role === 'user') {
                this.scrollToBottom(true);  // Force scroll for user messages
            } else {
                this.scrollToBottom();  // Normal scroll behavior for other messages
            }
            this.moveSpinnerToBottom();
        });
    }

    addSystemMessage(content) {
        const messageDiv = document.createElement('div');
        messageDiv.className = 'message system';
        messageDiv.textContent = content;
        this.chatMessages.appendChild(messageDiv);
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }
    
    addAccountingNode(data) {
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
                <span>${data.model.includes(':') ? data.model.split(':')[1] : data.model}</span>
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
        
        this.chatMessages.appendChild(messageDiv);
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }
    
    createAccountingNodes(reason, messagesToDiscard = 0) {
        // Calculate tokens ONLY from messages that will be discarded
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return [];
        
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
                model: model,
                cumulativeTokens: tokens,
                reason: reason,
                discardedMessages: tokens.messageCount
            });
        }
        
        return accountingNodes;
    }
    
    displaySystemPrompt(prompt) {
        const promptDiv = document.createElement('div');
        promptDiv.className = 'system-prompt-display';
        
        // Create collapsible header (similar to thinking blocks)
        const headerDiv = document.createElement('div');
        headerDiv.className = 'system-prompt-header';
        headerDiv.style.cursor = 'pointer';
        headerDiv.style.userSelect = 'none';
        
        const toggle = document.createElement('span');
        toggle.className = 'system-prompt-toggle';
        toggle.textContent = '▼';
        toggle.style.fontSize = '12px';
        toggle.style.fontFamily = 'monospace';
        toggle.style.transition = 'transform 0.2s';
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
                toggle.style.transform = 'rotate(0deg)';
                toggle.textContent = '▼';
            } else {
                contentDiv.classList.remove('collapsed');
                toggle.style.transform = 'rotate(90deg)';
                toggle.textContent = '▶';
            }
        };
        
        promptDiv.appendChild(headerDiv);
        promptDiv.appendChild(contentDiv);
        
        this.chatMessages.appendChild(promptDiv);
        
        // Add edit trigger after element is in DOM (only when expanded)
        const originalAddEditTrigger = () => this.addEditTrigger(contentDiv, prompt, 'system');
        
        // Override the header click to also handle edit trigger
        const originalClick = headerDiv.onclick;
        headerDiv.onclick = () => {
            originalClick();
            // Add edit trigger when expanding for the first time
            if (!isCollapsed && !contentDiv.dataset.editTriggerAdded) {
                setTimeout(originalAddEditTrigger, 300); // Wait for animation
                contentDiv.dataset.editTriggerAdded = 'true';
            }
        };
    }
    
    addEditTrigger(contentDiv, originalContent, type) {
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
                this.editUserMessage(contentDiv, originalContent);
            } else if (type === 'system') {
                this.editSystemPromptInline(contentDiv, originalContent);
            }
            editBalloon.style.display = 'none';
        };
    }
    
    editSystemPromptInline(contentDiv, originalPrompt) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Prevent multiple edit sessions
        if (contentDiv.classList.contains('editing')) return;
        
        // Make content editable
        contentDiv.contentEditable = true;
        contentDiv.classList.add('editing');
        
        // Just focus, don't select all - let user position cursor
        contentDiv.focus();
        
        // Create floating save/cancel buttons
        const buttonsDiv = document.createElement('div');
        buttonsDiv.className = 'edit-actions-floating';
        buttonsDiv.innerHTML = `
            <button class="btn btn-small btn-primary" title="Save & Restart Chat (Enter)"><i class="fas fa-check"></i></button>
            <button class="btn btn-small btn-secondary" title="Cancel (Escape)"><i class="fas fa-times"></i></button>
        `;
        contentDiv.parentElement.appendChild(buttonsDiv);
        
        // Position buttons
        const rect = contentDiv.getBoundingClientRect();
        buttonsDiv.style.top = (rect.bottom - contentDiv.parentElement.getBoundingClientRect().top + 4) + 'px';
        
        // Handle save
        const save = () => {
            const newPrompt = contentDiv.textContent.trim();
            if (!newPrompt) {
                this.showError('System prompt cannot be empty');
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
            this.tokenUsageHistory.set(this.currentChatId, {
                requests: [],
                model: chat.model
            });
            
            // Save settings
            this.saveSettings();
            
            // Reload the chat
            this.loadChat(this.currentChatId);
            
            // Show notification
            this.addSystemMessage('System prompt updated. Conversation has been reset.');
        };
        
        // Handle cancel
        const cancel = () => {
            contentDiv.contentEditable = false;
            contentDiv.classList.remove('editing');
            contentDiv.textContent = originalPrompt;
            buttonsDiv.remove();
            // Restore the edit trigger
            this.addEditTrigger(contentDiv, originalPrompt, 'system');
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
    appendMetricsToChat(usage, responseTime, model, messageType = 'assistant') {
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        
        const formatNumber = (num) => num.toLocaleString();
        let metricsHtml = '';
        
        // 1. MODEL
        if (model) {
            const modelDisplay = model.includes(':') ? model.split(':')[1] : model;
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
            
            // 7. TOTAL
            const totalTokens = (usage.promptTokens || 0) + 
                               (usage.cacheReadInputTokens || 0) + 
                               (usage.cacheCreationInputTokens || 0) + 
                               (usage.completionTokens || 0);
            metricsHtml += `<span class="metric-item" data-tooltip="Total tokens"><i class="fas fa-chart-bar"></i> ${formatNumber(totalTokens)}</span>`;
            
            // 8. DELTA - calculate based on message type
            let deltaTokens = 0;
            
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
            
            // 9. PRICE
            if (model) {
                const price = this.calculateMessagePrice(model, usage) || 0;
                metricsHtml += `<span class="metric-item" data-tooltip="Cost"><i class="fas fa-dollar-sign"></i> ${price.toFixed(4)}</span>`;
            }
        }
        
        metricsFooter.innerHTML = metricsHtml;
        
        // Simply append to the chat messages container
        this.chatMessages.appendChild(metricsFooter);
    }
    
    
    
    editUserMessage(contentDiv, originalContent) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Prevent multiple edit sessions
        if (contentDiv.classList.contains('editing')) return;
        
        // Find the message index by searching through all user message divs
        let messageIndex = -1;
        const allUserMessages = this.chatMessages.querySelectorAll('.message.user .message-content');
        
        for (let i = 0; i < allUserMessages.length; i++) {
            if (allUserMessages[i] === contentDiv) {
                // Count user messages up to this point
                let userMessageCount = 0;
                for (let j = 0; j < chat.messages.length; j++) {
                    if (chat.messages[j].type === 'user' && !chat.messages[j].isTitleRequest) {
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
            this.showError('Message not found in chat history');
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
            <button class="btn btn-small btn-primary" title="Save & Resend (Enter)"><i class="fas fa-check"></i></button>
            <button class="btn btn-small btn-secondary" title="Cancel (Escape)"><i class="fas fa-times"></i></button>
        `;
        contentDiv.parentElement.appendChild(buttonsDiv);
        
        // Position buttons
        const rect = contentDiv.getBoundingClientRect();
        buttonsDiv.style.top = (rect.bottom - contentDiv.parentElement.getBoundingClientRect().top + 4) + 'px';
        
        // Handle save
        const save = async () => {
            const newContent = contentDiv.textContent.trim();
            if (!newContent) {
                this.showError('Message cannot be empty');
                return;
            }
            
            // Always proceed even if content is the same - user may want to regenerate response
            
            // ATOMIC OPERATION: Batch mode to prevent partial saves
            this.batchMode = true;
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
                this.batchMode = false;
                this.autoSave(this.currentChatId);
            }
            
            // Reload the chat to show clipped history
            this.loadChat(this.currentChatId);
            
            // Send the new message
            this.chatInput.value = newContent;
            await this.sendMessage();
        };
        
        // Handle cancel
        // Handle keyboard shortcuts with a named function so we can remove it
        const keyHandler = (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                save();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancel();
            }
        };
        
        // Handle click outside
        const clickOutside = (e) => {
            if (!contentDiv.contains(e.target) && !buttonsDiv.contains(e.target)) {
                cancel();
            }
        };
        
        const cancel = () => {
            contentDiv.contentEditable = false;
            contentDiv.classList.remove('editing');
            contentDiv.textContent = originalText;
            buttonsDiv.remove();
            // Clean up event listeners
            contentDiv.removeEventListener('keydown', keyHandler);
            document.removeEventListener('click', clickOutside);
            // Restore the edit trigger
            this.addEditTrigger(contentDiv, originalText, 'user');
        };
        
        buttonsDiv.querySelector('.btn-primary').onclick = save;
        buttonsDiv.querySelector('.btn-secondary').onclick = cancel;
        
        contentDiv.addEventListener('keydown', keyHandler);
        setTimeout(() => document.addEventListener('click', clickOutside), 0);
    }
    
    // Helper to add content to the current assistant group
    addContentToAssistantGroup(content) {
        if (!this.currentAssistantGroup) {
            // Create a new group if we don't have one
            this.renderMessage('assistant', '');
        }
        
        // Check if content has thinking tags
        const thinkingRegex = /<thinking>([\s\S]*?)<\/thinking>/g;
        const hasThinking = thinkingRegex.test(content);
        
        if (hasThinking) {
            console.log('Processing content with thinking tags:', content);
            // Process thinking content
            let lastIndex = 0;
            let match;
            const regex = /<thinking>([\s\S]*?)<\/thinking>/g;
            
            while ((match = regex.exec(content)) !== null) {
                // Add text before thinking
                if (match.index > lastIndex) {
                    const textContent = content.substring(lastIndex, match.index).trim();
                    if (textContent) {
                        const textDiv = document.createElement('div');
                        textDiv.className = 'message-content';
                        textDiv.innerHTML = marked.parse(textContent);
                        this.currentAssistantGroup.appendChild(textDiv);
                    }
                }
                
                // Add thinking block
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
                contentWrapper.innerHTML = marked.parse(match[1].trim());
                
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
                this.currentAssistantGroup.appendChild(thinkingDiv);
                
                lastIndex = regex.lastIndex;
            }
            
            // Add remaining text
            if (lastIndex < content.length) {
                const remaining = content.substring(lastIndex).trim();
                if (remaining) {
                    const textDiv = document.createElement('div');
                    textDiv.className = 'message-content';
                    textDiv.innerHTML = marked.parse(remaining);
                    this.currentAssistantGroup.appendChild(textDiv);
                }
            }
        } else if (content.trim()) {
            // Regular content without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            contentDiv.innerHTML = marked.parse(content);
            this.currentAssistantGroup.appendChild(contentDiv);
        }
        
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }

    addToolCall(toolName, args, includeInContext = true, turn = null, messageIndex) {
        // If we have a current assistant group, append to it
        const targetContainer = this.currentAssistantGroup || this.chatMessages;
        
        // Create tool container with unique ID
        const toolId = `tool-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
        
        const toolDiv = document.createElement('div');
        toolDiv.className = 'tool-block';
        toolDiv.dataset.toolId = toolId;
        toolDiv.dataset.toolName = toolName;
        toolDiv.dataset.included = includeInContext;
        toolDiv.style.position = 'relative';
        
        // Store the turn for auto mode
        const chat = this.chats.get(this.currentChatId);
        if (turn !== null) {
            toolDiv.dataset.turn = turn;
        } else if (chat) {
            toolDiv.dataset.turn = chat.currentTurn || 0;
        }
        
        // Check if we're in cached mode
        const isCachedMode = chat && chat.toolInclusionMode === 'cached';
        const checkboxDisabled = isCachedMode ? 'disabled' : '';
        const labelTitle = isCachedMode ? 'Tool inclusion is locked in cached mode' : 'Include this tool\'s result in the LLM context';
        const labelOpacity = isCachedMode ? 'style="opacity: 0.5;"' : '';
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
            <span class="tool-toggle">▶</span>
            <span class="tool-label"><i class="fas fa-wrench"></i> ${toolName}</span>
            <span class="tool-info">
                <span class="tool-status"><i class="fas fa-hourglass-half"></i> Calling...</span>
                <label class="tool-include-label" title="${labelTitle}" ${labelOpacity}>
                    <input type="checkbox" class="tool-include-checkbox" ${includeInContext || isCachedMode ? 'checked' : ''} ${checkboxDisabled}>
                    <span class="tool-include-toggle"></span>
                    <span class="tool-include-text">Include</span>
                </label>
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
                <button class="tool-section-copy" title="Copy request"><i class="fas fa-clipboard"></i></button>
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
        
        toolHeader.addEventListener('click', (e) => {
            // Don't toggle if clicking on checkbox, label, or toggle switch
            if (e.target.matches('.tool-include-checkbox, .tool-include-label, .tool-include-text, .tool-include-toggle') || 
                e.target.closest('.tool-include-label')) {
                e.stopPropagation();
                return;
            }
            const isCollapsed = toolContent.classList.contains('collapsed');
            toolContent.classList.toggle('collapsed');
            toolHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
        });
        
        // Handle checkbox change
        const checkbox = toolHeader.querySelector('.tool-include-checkbox');
        checkbox.addEventListener('change', () => {
            // Update the tool's included state
            toolDiv.dataset.included = checkbox.checked;
            
            // If this is part of a chat, track the inclusion state
            if (this.currentChatId) {
                const chatToolStates = this.toolInclusionStates.get(this.currentChatId) || new Map();
                chatToolStates.set(toolId, checkbox.checked);
                this.toolInclusionStates.set(this.currentChatId, chatToolStates);
            }
            
            // Update global toggle state
            this.updateGlobalToggleUI();
        });
        
        toolDiv.appendChild(toolHeader);
        toolDiv.appendChild(toolContent);
        targetContainer.appendChild(toolDiv);
        
        // Store reference for later update
        this.pendingToolCalls = this.pendingToolCalls || new Map();
        this.pendingToolCalls.set(toolName, toolId);
        
        // Only append to chat if we're not in a group
        if (!this.currentAssistantGroup) {
            this.chatMessages.appendChild(targetContainer);
        }
        
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }

    addToolResult(toolName, result, responseTime = 0, responseSize = null, includeInContext = true, messageIndex = null) {
        // Try to find the pending tool call
        const toolId = this.pendingToolCalls?.get(toolName);
        
        if (toolId) {
            // Update existing tool block
            const toolDiv = document.querySelector(`[data-tool-id="${toolId}"]`);
            if (toolDiv) {
                // Update header status
                const statusSpan = toolDiv.querySelector('.tool-status');
                const infoSpan = toolDiv.querySelector('.tool-info');
                
                // Use provided size or calculate it
                const resultSize = responseSize !== null ? responseSize : (
                    typeof result === 'string' 
                        ? result.length 
                        : JSON.stringify(result).length
                );
                
                // Format size info
                let sizeInfo = '';
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
                    const oldCheckbox = infoSpan.querySelector('.tool-include-checkbox');
                    const wasChecked = oldCheckbox ? oldCheckbox.checked : true;
                    
                    // Check if we're in cached mode
                    const chat = this.chats.get(this.currentChatId);
                    const isCachedMode = chat && chat.toolInclusionMode === 'cached';
                    const checkboxDisabled = isCachedMode ? 'disabled' : '';
                    const labelTitle = isCachedMode ? 'Tool inclusion is locked in cached mode' : 'Include this tool\'s result in the LLM context';
                    const labelOpacity = isCachedMode ? 'style="opacity: 0.5;"' : '';
                    
                    infoSpan.innerHTML = `
                        <span class="tool-status">${result.error ? '<i class="fas fa-times-circle"></i> Error' : '<i class="fas fa-check-circle"></i> Complete'}</span>
                        <span class="tool-metric"><i class="fas fa-clock"></i> ${timeInfo}</span>
                        <span class="tool-metric"><i class="fas fa-box"></i> ${sizeInfo}</span>
                        <label class="tool-include-label" title="${labelTitle}" ${labelOpacity}>
                            <input type="checkbox" class="tool-include-checkbox" ${wasChecked || isCachedMode ? 'checked' : ''} ${checkboxDisabled}>
                            <span class="tool-include-toggle"></span>
                            <span class="tool-include-text">Include</span>
                        </label>
                    `;
                    
                    // Re-attach checkbox event handler if it exists
                    const newCheckbox = infoSpan.querySelector('.tool-include-checkbox');
                    if (newCheckbox) {
                        newCheckbox.addEventListener('change', () => {
                            // Update the tool's included state
                            toolDiv.dataset.included = newCheckbox.checked;
                            
                            // If this is part of a chat, track the inclusion state
                            if (this.currentChatId) {
                                const chatToolStates = this.toolInclusionStates.get(this.currentChatId) || new Map();
                                chatToolStates.set(toolId, newCheckbox.checked);
                                this.toolInclusionStates.set(this.currentChatId, chatToolStates);
                            }
                            
                            // Update global toggle state
                            this.updateGlobalToggleUI();
                        });
                    }
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
                            <button class="tool-section-copy" title="Copy response"><i class="fas fa-clipboard"></i></button>
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
                
                // Remove from pending
                this.pendingToolCalls.delete(toolName);
            } else {
                // Fallback: create new block if not found
                this.createStandaloneToolResult(toolName, result, responseTime, responseSize, includeInContext);
            }
        } else {
            // No pending call found, create standalone result
            this.createStandaloneToolResult(toolName, result, responseTime, responseSize, includeInContext);
        }
        
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }
    
    createStandaloneToolResult(toolName, result, responseTime = 0, responseSize = null, includeInContext = true) {
        const targetContainer = this.currentAssistantGroup || this.chatMessages;
        
        const toolDiv = document.createElement('div');
        toolDiv.className = 'tool-block tool-result-block';
        
        // Use provided size or calculate it
        const resultSize = responseSize !== null ? responseSize : (
            typeof result === 'string' 
                ? result.length 
                : JSON.stringify(result).length
        );
        
        // Format size info
        let sizeInfo = '';
        if (resultSize < 1024) {
            sizeInfo = `${resultSize} bytes`;
        } else if (resultSize < 1024 * 1024) {
            sizeInfo = `${(resultSize / 1024).toFixed(1)} KB`;
        } else {
            sizeInfo = `${(resultSize / (1024 * 1024)).toFixed(1)} MB`;
        }
        
        // Format response time
        const timeInfo = responseTime > 0 ? `${(responseTime / 1000).toFixed(2)}s` : '';
        
        // Generate unique ID for this tool result
        const toolId = `tool-result-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
        toolDiv.dataset.toolId = toolId;
        toolDiv.dataset.toolName = toolName;
        toolDiv.dataset.included = includeInContext ? 'true' : 'false';
        
        // Check if we're in cached mode
        const chat = this.chats.get(this.currentChatId);
        const isCachedMode = chat && chat.toolInclusionMode === 'cached';
        const checkboxDisabled = isCachedMode ? 'disabled' : '';
        const labelTitle = isCachedMode ? 'Tool inclusion is locked in cached mode' : 'Include this tool\'s result in the LLM context';
        const labelOpacity = isCachedMode ? 'style="opacity: 0.5;"' : '';
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
            <span class="tool-toggle">▶</span>
            <span class="tool-label"><i class="fas fa-chart-bar"></i> Tool result: ${toolName}</span>
            <span class="tool-info">
                <span class="tool-metric"><i class="fas fa-clock"></i> ${timeInfo}</span>
                <span class="tool-metric"><i class="fas fa-box"></i> ${sizeInfo}</span>
                <label class="tool-include-label" title="${labelTitle}" ${labelOpacity}>
                    <input type="checkbox" class="tool-include-checkbox" ${includeInContext || isCachedMode ? 'checked' : ''} ${checkboxDisabled}>
                    <span class="tool-include-toggle"></span>
                    <span class="tool-include-text">Include</span>
                </label>
            </span>
        `;
        
        const toolContent = document.createElement('div');
        toolContent.className = 'tool-content collapsed';
        
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
        
        toolContent.innerHTML = formattedResult;
        
        toolHeader.addEventListener('click', (e) => {
            // Don't toggle if clicking on checkbox, label, or toggle switch
            if (e.target.matches('.tool-include-checkbox, .tool-include-label, .tool-include-text, .tool-include-toggle') || 
                e.target.closest('.tool-include-label')) {
                e.stopPropagation();
                return;
            }
            const isCollapsed = toolContent.classList.contains('collapsed');
            toolContent.classList.toggle('collapsed');
            toolHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
        });
        
        // Handle checkbox change
        const checkbox = toolHeader.querySelector('.tool-include-checkbox');
        if (checkbox) {
            checkbox.addEventListener('change', () => {
                // Update the tool's included state
                toolDiv.dataset.included = checkbox.checked;
                
                // If this is part of a chat, track the inclusion state
                if (this.currentChatId) {
                    const chatToolStates = this.toolInclusionStates.get(this.currentChatId) || new Map();
                    chatToolStates.set(toolId, checkbox.checked);
                    this.toolInclusionStates.set(this.currentChatId, chatToolStates);
                    
                    // If we're in manual mode, update it when user changes checkboxes
                    const chat = this.chats.get(this.currentChatId);
                    if (chat && chat.toolInclusionMode === 'manual') {
                        // Stay in manual mode when user manually changes checkboxes
                        this.updateGlobalToggleUI();
                    } else if (chat) {
                        // User changed a checkbox, switch to manual mode
                        chat.toolInclusionMode = 'manual';
                        this.autoSave(chat.id);
                        this.updateGlobalToggleUI();
                    }
                }
            });
        }
        
        toolDiv.appendChild(toolHeader);
        toolDiv.appendChild(toolContent);
        targetContainer.appendChild(toolDiv);
        
        // Only append to chat if we're not in a group
        if (!this.currentAssistantGroup) {
            this.chatMessages.appendChild(targetContainer);
        }
    }

    scrollToBottom(force = false) {
        // Only scroll if user is already near the bottom, unless forced
        const threshold = 100; // pixels from bottom to consider "at bottom"
        const isAtBottom = this.chatMessages.scrollHeight - this.chatMessages.scrollTop - this.chatMessages.clientHeight < threshold;
        
        if (isAtBottom || force) {
            this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
        }
    }
    
    // Add turn separator between conversation turns
    addTurnSeparator(turnNumber) {
        const separator = document.createElement('div');
        separator.className = 'turn-separator';
        
        const turnLabel = document.createElement('span');
        turnLabel.className = 'turn-number';
        turnLabel.textContent = `Turn ${turnNumber}`;
        
        separator.appendChild(turnLabel);
        this.chatMessages.appendChild(separator);
    }
    
    // Add step number to the last message element
    addStepNumber(turn, step, element = null) {
        const target = element || this.chatMessages.lastElementChild;
        if (!target) return;
        
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

    showLoadingSpinner(text = 'Thinking...') {
        // Remove any existing spinner first
        this.hideLoadingSpinner();
        
        // Create spinner element
        const spinnerDiv = document.createElement('div');
        spinnerDiv.id = 'llm-loading-spinner';
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
        
        // Update time every 100ms
        this.spinnerInterval = setInterval(() => {
            const elapsed = Math.floor((Date.now() - startTime) / 1000);
            const timeElement = spinnerDiv.querySelector('.spinner-time');
            if (timeElement) {
                timeElement.textContent = `(${elapsed}s)`;
            }
        }, 100);
        
        this.chatMessages.appendChild(spinnerDiv);
        // Force scroll to bottom when showing spinner
        requestAnimationFrame(() => {
            this.scrollToBottom(true);
        });
    }

    hideLoadingSpinner() {
        // Clear the interval
        if (this.spinnerInterval) {
            clearInterval(this.spinnerInterval);
            this.spinnerInterval = null;
        }
        
        const spinner = document.getElementById('llm-loading-spinner');
        if (spinner) {
            spinner.remove();
        }
    }
    
    // Helper to ensure spinner stays at bottom
    moveSpinnerToBottom() {
        const spinner = document.getElementById('llm-loading-spinner');
        if (spinner && spinner.parentNode) {
            // Remove and re-append to ensure it's at the bottom
            spinner.parentNode.removeChild(spinner);
            this.chatMessages.appendChild(spinner);
            // Force scroll when moving spinner to bottom
            requestAnimationFrame(() => {
                this.scrollToBottom(true);
            });
        }
    }

    showReconnectButton(mcpServerId) {
        this.reconnectMcpBtn.style.display = 'block';
        this.reconnectMcpBtn.dataset.mcpServerId = mcpServerId;
    }

    async reconnectCurrentMcp() {
        const mcpServerId = this.reconnectMcpBtn.dataset.mcpServerId;
        if (!mcpServerId) return;
        
        const server = this.mcpServers.get(mcpServerId);
        if (!server) {
            this.showError('MCP server configuration not found');
            return;
        }
        
        this.reconnectMcpBtn.disabled = true;
        this.reconnectMcpBtn.textContent = 'Reconnecting...';
        
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
            
            // Reload current chat to update UI
            if (this.currentChatId) {
                this.loadChat(this.currentChatId);
            }
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `MCP server "${server.name}" reconnected successfully`
            });
            
        } catch (error) {
            this.showError(`Failed to reconnect to MCP server: ${error.message}`);
            this.reconnectMcpBtn.disabled = false;
            this.reconnectMcpBtn.textContent = 'Reconnect MCP Server';
        }
    }

    // Also add auto-reconnect on send if disconnected
    async ensureMcpConnection(mcpServerId) {
        if (this.mcpConnections.has(mcpServerId)) {
            const connection = this.mcpConnections.get(mcpServerId);
            if (connection.isReady()) {
                return connection;
            }
        }
        
        // Try to reconnect
        const server = this.mcpServers.get(mcpServerId);
        if (!server) {
            throw new Error('MCP server configuration not found');
        }
        
        const mcpConnection = new MCPClient();
        mcpConnection.onLog = (logEntry) => this.addLogEntry(`MCP-${server.name}`, logEntry);
        await mcpConnection.connect(server.url);
        
        this.mcpConnections.set(mcpServerId, mcpConnection);
        return mcpConnection;
    }
    
    // Token usage tracking methods
    calculateContextWindowTokens(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return 0;
        
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
                model: model
            });
        }
        
        const history = this.tokenUsageHistory.get(chatId);
        
        // Add this request to history with the model
        history.requests.push({
            timestamp: new Date().toISOString(),
            model: model,
            promptTokens: usage.promptTokens,
            completionTokens: usage.completionTokens,
            totalTokens: usage.totalTokens,
            cacheCreationInputTokens: usage.cacheCreationInputTokens || 0,
            cacheReadInputTokens: usage.cacheReadInputTokens || 0
        });
        
        // Calculate context window based on message type
        const contextTokens = this.calculateContextWindowTokens(chatId);
        
        // Update context window indicator
        this.updateContextWindowIndicator(contextTokens, model);
        
        // Update all token displays (context window, cumulative tokens, and cost)
        this.updateAllTokenDisplays();
        
        // Update any pending conversation total displays
        const pendingTotals = document.querySelectorAll('[id^="conv-total-"]');
        pendingTotals.forEach(el => {
            if (el.textContent === 'Calculating...' || el.textContent.match(/^\d/)) {
                el.textContent = contextTokens.toLocaleString();
            }
        });
    }
    
    updateContextWindowIndicator(totalTokens, model) {
        const indicator = document.getElementById('contextWindowIndicator');
        const stats = document.getElementById('contextWindowStats');
        const fill = document.getElementById('contextWindowFill');
        const percentage = document.getElementById('contextWindowPercentage');
        
        if (!indicator) return;
        
        // Extract model name from format "provider:model-name" if needed
        let modelName = model;
        if (model && model.includes(':')) {
            modelName = model.split(':')[1];
        }
        
        // Get model info
        const limit = this.modelLimits[modelName] || 4096;
        const percentUsed = Math.min((totalTokens / limit) * 100, 100);
        
        // Show indicator
        indicator.style.display = 'flex';
        
        // Update stats - show as "X / Y" or "Xk / Yk"
        if (totalTokens >= 1000 || limit >= 1000) {
            const totalDisplay = totalTokens >= 1000 ? `${(totalTokens / 1000).toFixed(1)}k` : totalTokens.toString();
            const limitDisplay = limit >= 1000 ? `${(limit / 1000).toFixed(0)}k` : limit.toString();
            stats.textContent = `${totalDisplay} / ${limitDisplay}`;
        } else {
            stats.textContent = `${totalTokens} / ${limit}`;
        }
        
        // Update bar
        fill.style.width = percentUsed + '%';
        
        // Only show percentage text in non-compact view
        if (percentage) {
            percentage.textContent = Math.round(percentUsed) + '%';
        }
        
        // Update color based on usage
        fill.classList.remove('warning', 'danger');
        if (percentUsed >= 90) {
            fill.classList.add('danger');
        } else if (percentUsed >= 75) {
            fill.classList.add('warning');
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
    addCumulativeTokens(chatId, model, promptTokens, completionTokens, cacheReadTokens = 0, cacheCreationTokens = 0) {
        // This function is now deprecated - token tracking happens in addMessage()
        // Keeping it for backward compatibility but it just updates displays
        this.updateAllTokenDisplays();
    }
    
    // Get cumulative token usage for the entire chat - now just reads from stored data
    getCumulativeTokenUsage(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.totalTokensPrice) {
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
    updateAllTokenDisplays(chatId = null) {
        const targetChatId = chatId || this.currentChatId;
        if (!targetChatId) {
            // Clear displays when no chat is selected
            const costElement = document.getElementById('cumulativeCost');
            if (costElement) {
                costElement.textContent = '$0.00';
            }
            return;
        }
        
        // Store current chat ID temporarily if using a different one
        const originalChatId = this.currentChatId;
        if (chatId && chatId !== this.currentChatId) {
            this.currentChatId = chatId;
        }
        
        // Update context window
        const chat = this.chats.get(this.currentChatId);
        if (chat) {
            const contextTokens = this.calculateContextWindowTokens(this.currentChatId);
            this.updateContextWindowIndicator(contextTokens, chat.model || 'unknown');
        }
        
        // Update cumulative tokens
        this.updateCumulativeTokenDisplay();
        
        // Restore original chat ID if we changed it
        if (chatId && chatId !== originalChatId) {
            this.currentChatId = originalChatId;
        }
    }
    
    // Update cumulative token display
    updateCumulativeTokenDisplay() {
        if (!this.currentChatId) return;
        
        const cumulative = this.getCumulativeTokenUsage(this.currentChatId);
        const formatNumber = (num) => num.toLocaleString();
        
        const inputElement = document.getElementById('cumulativeInputTokens');
        const outputElement = document.getElementById('cumulativeOutputTokens');
        const cacheReadElement = document.getElementById('cumulativeCacheReadTokens');
        const cacheCreationElement = document.getElementById('cumulativeCacheCreationTokens');
        
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
        const cost = this.calculateTokenCost(this.currentChatId);
        const costElement = document.getElementById('cumulativeCost');
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
        this.updateTokenCostTooltips(this.currentChatId);
    }
    
    // Calculate token cost for a chat - now just reads from stored data
    calculateTokenCost(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) return 0; // Return 0 for no chat
        
        // Use stored total cost
        if (chat.totalTokensPrice && chat.totalTokensPrice.totalCost !== undefined) {
            return chat.totalTokensPrice.totalCost;
        }
        
        return 0;
    }
    
    // Update token cost tooltips
    updateTokenCostTooltips(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat || !chat.perModelTokensPrice) return;
        
        // Remove old data-tooltip attribute
        const tokenCountersSection = document.getElementById('tokenCounters');
        if (tokenCountersSection) {
            tokenCountersSection.removeAttribute('data-tooltip');
            
            // Add hover handlers if not already added
            if (!tokenCountersSection.hasHoverHandlers) {
                tokenCountersSection.hasHoverHandlers = true;
                
                // Mouse enter - show the breakdown
                tokenCountersSection.addEventListener('mouseenter', (e) => {
                    // Use current chat ID dynamically
                    this.showTokenBreakdownHover(e, this.currentChatId);
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
        if (!chat || !chat.perModelTokensPrice) return;
        
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
        let html = [];
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
            const modelName = model.includes(':') ? model.split(':')[1] : model;
            
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
    
    // Get previous prompt tokens for delta calculation
    getPreviousPromptTokens() {
        if (!this.currentChatId) return 0;
        
        const history = this.tokenUsageHistory.get(this.currentChatId);
        if (!history || history.requests.length === 0) {
            return 0;
        }
        
        // If we have previous requests, get the last one's prompt tokens
        if (history.requests.length > 0) {
            const lastRequest = history.requests[history.requests.length - 1];
            return lastRequest.promptTokens || 0;
        }
        
        return 0;
    }
    
    // Add a turn separator with turn number
    addTurnSeparator(turnNumber) {
        const separator = document.createElement('div');
        separator.className = 'turn-separator';
        
        const turnLabel = document.createElement('span');
        turnLabel.className = 'turn-number';
        turnLabel.textContent = `Turn ${turnNumber}`;
        
        separator.appendChild(turnLabel);
        this.chatMessages.appendChild(separator);
    }
    
    // Add step number to the last added element
    addStepNumber(turn, step, element = null) {
        const target = element || this.chatMessages.lastElementChild;
        if (!target) return;
        
        const stepLabel = document.createElement('span');
        stepLabel.className = 'step-number';
        stepLabel.textContent = `${turn}.${step}`;
        stepLabel.title = `Turn ${turn}, Step ${step}`;
        
        // Position the step number on the left of the element
        target.appendChild(stepLabel);
    }
    
    // Tooltips are now implemented with pure CSS
    // No JavaScript initialization required
    
    // Copy conversation metrics to clipboard
    async copyConversationMetrics() {
        if (!this.currentChatId) {
            this.showError('No active chat to copy metrics from');
            return;
        }
        
        const chat = this.chats.get(this.currentChatId);
        if (!chat || !this.hasUserContent(chat)) {
            this.showError('No messages to copy');
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
                    toolCallId: tc.toolCallId,
                    // Don't include args or result
                }));
            }
            
            if (message.results && Array.isArray(message.results)) {
                sanitized.results = message.results.map(r => ({
                    name: r.name,
                    toolCallId: r.toolCallId,
                    resultSize: r.result 
                        ? (typeof r.result === 'string' 
                            ? r.result.length 
                            : JSON.stringify(r.result).length)
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
            model: chat.model,
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
            this.showError('Failed to copy to clipboard: ' + error.message);
        }
    }
    
    // Temperature control methods
    updateTemperatureDisplay(temperature) {
        const formattedValue = temperature.toFixed(1);
        
        // Update all temperature displays
        if (this.currentTempText) {
            this.currentTempText.textContent = formattedValue;
        }
        if (this.tempValueLabel) {
            this.tempValueLabel.textContent = formattedValue;
        }
        if (this.temperatureValue) {
            this.temperatureValue.textContent = formattedValue;
        }
        
        // Set title attribute as hint for compact view
        let hint = '';
        if (temperature === 0) {
            hint = 'Deterministic';
        } else if (temperature <= 0.3) {
            hint = 'Very focused';
        } else if (temperature <= 0.5) {
            hint = 'Focused';
        } else if (temperature <= 0.7) {
            hint = 'Balanced';
        } else if (temperature <= 1.0) {
            hint = 'Creative';
        } else if (temperature <= 1.5) {
            hint = 'Very creative';
        } else {
            hint = 'Experimental';
        }
        
        if (this.temperatureBtn) {
            this.temperatureBtn.setAttribute('data-tooltip', `Temperature ${formattedValue}: ${hint}`);
        }
        if (this.temperatureControl) {
            this.temperatureControl.title = `Temperature: ${hint}`;
        }
    }
    
    saveTemperatureForChat(temperature) {
        if (!this.currentChatId) return;
        
        const chat = this.chats.get(this.currentChatId);
        if (chat) {
            chat.temperature = temperature;
            chat.updatedAt = new Date().toISOString();
            this.autoSave(chat.id);
        }
    }
    
    getCurrentTemperature() {
        if (!this.currentChatId) return 0.7;
        
        const chat = this.chats.get(this.currentChatId);
        return chat ? (chat.temperature || 0.7) : 0.7;
    }
    
    // Handle generate title button click
    async handleGenerateTitleClick() {
        if (!this.currentChatId) {
            this.showError('Please select or create a chat first');
            return;
        }
        
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Check if there are any messages to generate a title from
        const userMessages = chat.messages.filter(m => m.type === 'user' && !m.isTitleRequest);
        const assistantMessages = chat.messages.filter(m => m.type === 'assistant' && !m.isTitleRequest);
        
        if (userMessages.length === 0 || assistantMessages.length === 0) {
            this.showError('Need at least one user message and one assistant response to generate a title');
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
            
            // Parse the model selection (format: "provider:model")
            const [providerType, modelName] = chat.model.split(':');
            if (!providerType || !modelName) {
                throw new Error('Invalid model selection in chat');
            }
            
            // Create the actual LLM provider instance
            console.log('Creating provider with proxyUrl:', proxyProvider.proxyUrl);
            const provider = createLLMProvider(providerType, proxyProvider.proxyUrl, modelName);
            provider.onLog = proxyProvider.onLog;
            
            // Generate the title
            await this.generateChatTitle(chat, mcpConnection, provider);
        } catch (error) {
            this.showError(`Failed to generate title: ${error.message}`);
        } finally {
            this.generateTitleBtn.disabled = false;
        }
    }
    
    // Unified summary generation method
    async generateChatSummary(chat, mcpConnection, provider, isAutomatic = false) {
        try {
            // User request that clearly explains the purpose
            const summaryRequest = 'Please create a comprehensive summary of our discussion that I can save and later provide back to you so we can continue from where we left off. Include all the context, findings, and details needed to resume our conversation.';
            
            // System prompt that frames the task as creating a reusable context
            const summarySystemPrompt = `You are a helpful assistant that creates conversation summaries designed to be provided back to an AI assistant to continue discussions.

When asked to summarize, you are creating a "conversation checkpoint" that captures the complete state of the discussion so far. This summary will be given to you (or another AI assistant) in a future conversation to provide full context.

CRITICAL: You are summarizing the conversation that happened BEFORE the summary request. The conversation consists of:
1. User messages (questions, requests, information provided)
2. Assistant responses (analysis, findings, answers, data retrieved)
3. Any tool usage or data collection that occurred

Create a summary with these sections:

## CONVERSATION SUMMARY (CHECKPOINT)

### USER'S ORIGINAL REQUESTS
Quote each user message exactly as written (excluding the summary request itself).

### ASSISTANT'S FINDINGS AND RESPONSES
Summarize what the assistant discovered, analyzed, and explained, including:
- Complete responses provided
- Data retrieved using tools
- Analysis performed
- Conclusions reached
- Any specific numbers, configurations, or technical details mentioned

### CURRENT UNDERSTANDING
- What has been established about the user's environment/situation
- Key facts and data points discovered
- Current state of any investigations or analysis

### CONTEXT FOR CONTINUATION
- Where the conversation left off
- Any pending questions or next steps
- Relevant details that would be needed to continue the discussion

Remember: This summary will be the ONLY context available when resuming the conversation, so include all important details, findings, and the current state of discussion.`;
            
            // CRITICAL: Save summary request BEFORE displaying
            this.addMessage(chat.id, { 
                role: 'system-summary',
                content: summaryRequest,
                timestamp: new Date().toISOString()
            });
            
            // Now display it
            this.processRenderEvent({ type: 'system-summary-message', content: summaryRequest });
            
            // Show loading spinner
            this.processRenderEvent({ type: 'show-spinner' });
            
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
            const response = await provider.sendMessage(messages, [], temperature, chat.toolInclusionMode, cacheControlIndex);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' });
            
            // Update metrics
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, chat.model || provider.model);
            }
            
            // Process the summary response
            if (response.content) {
                // CRITICAL: Save summary response BEFORE displaying
                this.addMessage(chat.id, { 
                    role: 'summary', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: chat.model || provider.model,
                    timestamp: new Date().toISOString(),
                    cacheControlIndex: cacheControlIndex  // Store the frozen cache position
                });
                
                // Display the response as a summary message WITH metrics
                this.processRenderEvent({ 
                    type: 'summary-message', 
                    content: response.content,
                    usage: response.usage,
                    responseTime: llmResponseTime,
                    model: chat.model || provider.model
                });
                
                // Update context window display
                const contextTokens = this.calculateContextWindowTokens(chat.id);
                this.updateContextWindowIndicator(contextTokens, chat.model || provider.model);
                
                // Update all token displays including cost
                this.updateAllTokenDisplays();
                
                // Mark that summary was generated
                chat.summaryGenerated = true;
                if (!isAutomatic) {
                    this.saveChatToStorage(chat.id);
                }
            }
            
        } catch (error) {
            console.error('Failed to summarize conversation:', error);
            
            // Hide spinner on error
            this.processRenderEvent({ type: 'hide-spinner' });
            
            if (!isAutomatic) {
                this.showError(`Failed to summarize: ${error.message}`);
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
    async summarizeConversation() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) {
            this.showError('No active chat to summarize');
            return;
        }
        
        // Check if there are any user messages to summarize
        const userMessages = chat.messages.filter(m => m.role === 'user' && !['system-title', 'system-summary'].includes(m.role));
        const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
        if (userMessages.length === 0 || assistantMessages.length === 0) {
            this.showError('Need at least one complete conversation exchange to summarize');
            return;
        }
        
        // Disable button to prevent multiple requests
        this.summarizeBtn.disabled = true;
        this.summarizeBtn.innerHTML = '<span><i class="fas fa-hourglass-half"></i></span><span>Summarizing...</span>';
        
        try {
            // Get MCP connection and LLM provider
            const mcpConnection = await this.ensureMcpConnection(chat.mcpServerId);
            const llmProviderConfig = this.llmProviders.get(chat.llmProviderId);
            
            // Parse the model selection (format: "provider:model")
            const [providerType, modelName] = chat.model.split(':');
            if (!providerType || !modelName) {
                throw new Error('Invalid model selection in chat');
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
            this.processRenderEvent({ type: 'hide-spinner' });
            
            this.showError(`Failed to summarize: ${error.message}`);
            
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
    deleteSummaryMessages(messageIndex) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
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
        let summaryTokens = {
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
            summaryModel = summaryMsg.model || chat.model;
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
        
        // Reload the chat to update the display
        this.loadChat(this.currentChatId);
    }
    
    // Check if automatic summary should be generated
    shouldGenerateSummary(chat) {
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
        const modelLimit = this.getModelContextLimit(chat.model);
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
    
    // Unified title generation method
    async generateChatTitle(chat, mcpConnection, provider, isAutomatic = false) {
        try {
            // Create a system message for title generation
            const titleRequest = 'Please provide a short, descriptive title (max 50 characters) for this conversation. Respond with ONLY the title text, no quotes, no explanation.';
            
            // CRITICAL: Save title request BEFORE displaying
            this.addMessage(chat.id, { 
                role: 'system-title', 
                content: titleRequest,
                timestamp: new Date().toISOString()
            });
            
            // Now display it
            this.processRenderEvent({ type: 'system-title-message', content: titleRequest });
            
            // Show loading spinner
            this.processRenderEvent({ type: 'show-spinner' });
            
            // Build conversational messages (no tools) for title generation
            const cleanMessages = this.buildConversationalMessages(chat.messages, false);
            
            // Create messages array with custom system prompt for title generation
            const messages = [{
                role: 'system', 
                content: 'You are a helpful assistant that generates concise, descriptive titles for conversations. Respond with ONLY the title text, no quotes, no explanation, no markdown.' 
            }];
            
            // Add clean conversational messages, truncating assistant responses
            for (const msg of cleanMessages) {
                if (msg.role === 'user') {
                    messages.push(msg);
                } else if (msg.role === 'assistant' && msg.content) {
                    // Truncate long assistant messages for title context
                    const truncated = msg.content.length > 500 ? msg.content.substring(0, 500) + '...' : msg.content;
                    messages.push({ role: 'assistant', content: truncated });
                }
            }
            
            // Add the title request
            messages.push({ role: 'user', content: titleRequest });
            
            // Send request with low temperature for consistent titles
            const temperature = 0.3;
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, [], temperature, chat.toolInclusionMode);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Don't update token usage history for title generation
            // as it shouldn't affect the context window
            
            // Process the title response
            if (response.content) {
                // Store the title response with the 'title' role
                this.addMessage(chat.id, { 
                    role: 'title', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: chat.model || provider.model,
                    timestamp: new Date().toISOString()
                });
                
                // Display the response as a title message WITH metrics
                this.processRenderEvent({ 
                    type: 'title-message', 
                    content: response.content,
                    usage: response.usage,
                    responseTime: llmResponseTime,
                    model: chat.model || provider.model
                });
                
                // Extract and clean the title
                const newTitle = response.content.trim()
                    .replace(/^["']|["']$/g, '') // Remove quotes
                    .replace(/^Title:\s*/i, '') // Remove "Title:" prefix if present
                    .substring(0, 65); // Allow some tolerance beyond 50 chars
                
                // Update the chat title
                if (newTitle && newTitle.length > 0) {
                    chat.title = newTitle;
                    chat.updatedAt = new Date().toISOString();
                    
                    // Mark that title was generated
                    chat.titleGenerated = true;
                    
                    // Update UI
                    this.updateChatSessions();
                    if (chat.id === this.currentChatId) {
                        document.getElementById('chatTitle').textContent = newTitle;
                    }
                    
                    // Save changes (unless automatic)
                    if (!isAutomatic) {
                        this.saveChatToStorage(chat.id);
                    }
                }
            }
            
        } catch (error) {
            console.error('Failed to generate title:', error);
            
            // Hide spinner on error
            this.processRenderEvent({ type: 'hide-spinner' });
            
            if (!isAutomatic) {
                // Don't show error to user for manual requests, just log it
            }
            
            // Remove the system-title message if it failed
            const lastMsg = chat.messages[chat.messages.length - 1];
            if (lastMsg && lastMsg.role === 'system-title') {
                this.removeLastMessage(chat.id);
            }
            
            throw error; // Re-throw for caller to handle
        } finally {
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' });
            // Clear assistant group
            this.currentAssistantGroup = null;
        }
    }
    
    // Tool inclusion toggle methods
    cycleGlobalToolToggle() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        const currentMode = chat.toolInclusionMode || 'auto';
        let newMode;
        
        // Cycle through states: auto -> all-on -> all-off -> manual -> cached -> auto
        switch (currentMode) {
            case 'auto':
                newMode = 'all-on';
                this.enableAllToolCheckboxes();
                this.setAllToolStates(true);
                break;
            case 'all-on':
                newMode = 'all-off';
                this.setAllToolStates(false);
                break;
            case 'all-off':
                newMode = 'manual';
                // Don't change individual states in manual mode
                break;
            case 'manual':
                newMode = 'cached';
                // In cached mode, all tools are on and disabled
                this.setAllToolStates(true);
                this.disableAllToolCheckboxes();
                break;
            case 'cached':
                newMode = 'auto';
                // In auto mode, we'll dynamically determine which tools to include
                this.enableAllToolCheckboxes();
                this.updateToolCheckboxesForAutoMode();
                break;
            default:
                newMode = 'auto';
                this.updateToolCheckboxesForAutoMode();
        }
        
        chat.toolInclusionMode = newMode;
        this.autoSave(chat.id);
        this.updateGlobalToggleUI();
    }
    
    getGlobalToolToggleState() {
        const checkboxes = document.querySelectorAll('.tool-block .tool-include-checkbox');
        if (checkboxes.length === 0) return 'all-on';
        
        let checkedCount = 0;
        checkboxes.forEach(cb => {
            if (cb.checked) checkedCount++;
        });
        
        if (checkedCount === checkboxes.length) return 'all-on';
        if (checkedCount === 0) return 'all-off';
        return 'mixed';
    }
    
    setAllToolStates(checked) {
        const checkboxes = document.querySelectorAll('.tool-block .tool-include-checkbox');
        checkboxes.forEach(checkbox => {
            checkbox.checked = checked;
            // Trigger change event to update data attributes and states
            checkbox.dispatchEvent(new Event('change', { bubbles: true }));
        });
    }
    
    enableAllToolCheckboxes() {
        const checkboxes = document.querySelectorAll('.tool-block .tool-include-checkbox');
        checkboxes.forEach(checkbox => {
            checkbox.disabled = false;
            const label = checkbox.closest('.tool-include-label');
            if (label) {
                label.style.opacity = '1';
                label.title = 'Include this tool\'s result in the LLM context';
            }
        });
    }
    
    disableAllToolCheckboxes() {
        const checkboxes = document.querySelectorAll('.tool-block .tool-include-checkbox');
        checkboxes.forEach(checkbox => {
            checkbox.disabled = true;
            const label = checkbox.closest('.tool-include-label');
            if (label) {
                label.style.opacity = '0.5';
                label.title = 'Tool inclusion is locked in cached mode';
            }
        });
    }
    
    updateToolCheckboxesForAutoMode() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat || chat.toolInclusionMode !== 'auto') return;
        
        const currentTurn = chat.currentTurn || 0;
        
        // Find all tool blocks in the chat
        const toolBlocks = this.chatMessages.querySelectorAll('.tool-block');
        
        toolBlocks.forEach(toolBlock => {
            // Get the turn from the dataset
            const toolTurn = parseInt(toolBlock.dataset.turn || '0');
            
            // Update checkbox based on whether this tool is from current turn
            const checkbox = toolBlock.querySelector('.tool-include-checkbox');
            if (checkbox) {
                const shouldInclude = toolTurn === currentTurn;
                checkbox.checked = shouldInclude;
                checkbox.disabled = true; // Disable in auto mode
                toolBlock.dataset.included = shouldInclude;
                
                // Update visual state
                const label = toolBlock.querySelector('.tool-include-label');
                if (label) {
                    label.style.opacity = '0.6';
                    label.title = `Auto mode: ${shouldInclude ? 'Included (current turn)' : 'Excluded (previous turn)'}`;
                }
            }
        });
    }
    
    shouldIncludeToolsForMessage(msg, toolMode, currentTurn) {
        // For non-tool messages, always include
        if (msg.type !== 'assistant' && msg.type !== 'tool-results') {
            return true;
        }
        
        // Check the inclusion mode
        switch (toolMode) {
            case 'all-on':
                return true;
            case 'all-off':
                return false;
            case 'manual':
                // In manual mode, respect individual checkbox states
                return true; // The filtering happens later based on includeInContext
            case 'auto':
                // In auto mode, only include tools from the current turn
                return msg.turn === currentTurn;
            default:
                return true;
        }
    }
    
    updateGlobalToggleUI() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        const mode = chat.toolInclusionMode || 'auto';
        const button = this.globalToolToggleBtn;
        const icon = button.querySelector('.tool-toggle-icon');
        const text = button.querySelector('.tool-toggle-text');
        
        // Update button appearance based on mode
        switch (mode) {
            case 'auto':
                button.classList.remove('btn-warning', 'btn-danger', 'btn-secondary');
                button.classList.add('btn-success');
                icon.innerHTML = '<i class="fas fa-sync-alt"></i>';
                text.textContent = 'Auto (current turn only)';
                break;
            case 'all-on':
                button.classList.remove('btn-warning', 'btn-danger', 'btn-success');
                button.classList.add('btn-secondary');
                icon.textContent = '✓';
                text.textContent = 'All tools included';
                break;
            case 'all-off':
                button.classList.remove('btn-secondary', 'btn-warning', 'btn-success');
                button.classList.add('btn-danger');
                icon.textContent = '✗';
                text.textContent = 'No tools included';
                break;
            case 'manual':
                button.classList.remove('btn-secondary', 'btn-danger', 'btn-success');
                button.classList.add('btn-warning');
                const state = this.getGlobalToolToggleState();
                icon.innerHTML = '<i class="fas fa-cog"></i>';
                text.textContent = state === 'mixed' ? 'Manual (mixed)' : `Manual (${state})`;
                break;
            case 'cached':
                button.classList.remove('btn-warning', 'btn-danger', 'btn-success');
                button.classList.add('btn-secondary');
                icon.innerHTML = '<i class="fas fa-lock"></i>';
                text.textContent = 'Cached (all locked)';
                break;
        }
    }
}

// Initialize the application
document.addEventListener('DOMContentLoaded', () => {
    window.app = new NetdataMCPChat();
    
    // Expose migration function for testing
    window.migrateCurrentChat = () => {
        if (!window.app.currentChatId) {
            console.log('No chat is currently loaded');
            return;
        }
        const chat = window.app.chats.get(window.app.currentChatId);
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
        window.app.updateAllTokenDisplays();
        console.log('Migration complete!');
    };
});
