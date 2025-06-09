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
        this.metricsIdCounter = 0; // Auto-increment ID for metrics placeholders
        this.toolInclusionStates = new Map(); // Track which tools are included/excluded per chat
        this.shouldStopProcessing = false; // Flag to stop processing between requests
        this.isProcessing = false; // Track if we're currently processing messages
        
        // Single source of truth for all model information
        this.models = {
            openai: [
                // Currently available reasoning models
                { value: 'o1-preview', text: 'o1 Preview (Reasoning)', category: 'reasoning', contextLimit: 128000 },
                { value: 'o1-mini', text: 'o1 Mini (Reasoning)', category: 'reasoning', contextLimit: 128000 },
                // GPT-4o series (Multimodal)
                { value: 'gpt-4o', text: 'GPT-4o (Latest)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4o-2024-11-20', text: 'GPT-4o (2024-11-20)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4o-2024-08-06', text: 'GPT-4o (2024-08-06)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4o-2024-05-13', text: 'GPT-4o (2024-05-13)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4o-mini', text: 'GPT-4o Mini', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4o-mini-2024-07-18', text: 'GPT-4o Mini (2024-07-18)', category: 'gpt-4', contextLimit: 128000 },
                // GPT-4 Turbo
                { value: 'gpt-4-turbo', text: 'GPT-4 Turbo', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4-turbo-2024-04-09', text: 'GPT-4 Turbo (2024-04-09)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4-turbo-preview', text: 'GPT-4 Turbo Preview', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4-0125-preview', text: 'GPT-4 (0125 Preview)', category: 'gpt-4', contextLimit: 128000 },
                { value: 'gpt-4-1106-preview', text: 'GPT-4 (1106 Preview)', category: 'gpt-4', contextLimit: 128000 },
                // Standard GPT-4
                { value: 'gpt-4', text: 'GPT-4', category: 'gpt-4', contextLimit: 8192 },
                { value: 'gpt-4-0613', text: 'GPT-4 (0613)', category: 'gpt-4', contextLimit: 8192 },
                // GPT-3.5
                { value: 'gpt-3.5-turbo', text: 'GPT-3.5 Turbo', category: 'gpt-3.5', contextLimit: 16385 },
                { value: 'gpt-3.5-turbo-0125', text: 'GPT-3.5 Turbo (0125)', category: 'gpt-3.5', contextLimit: 16385 },
                { value: 'gpt-3.5-turbo-1106', text: 'GPT-3.5 Turbo (1106)', category: 'gpt-3.5', contextLimit: 16385 }
            ],
            anthropic: [
                // Claude 4 series (Latest - May 2025)
                { value: 'claude-opus-4-20250514', text: 'Claude Opus 4', category: 'claude-4', contextLimit: 200000 },
                { value: 'claude-sonnet-4-20250514', text: 'Claude Sonnet 4', category: 'claude-4', contextLimit: 200000 },
                // Claude 3.7
                { value: 'claude-3-7-sonnet-20250219', text: 'Claude 3.7 Sonnet', category: 'claude-3.7', contextLimit: 200000 },
                { value: 'claude-3-7-sonnet-latest', text: 'Claude 3.7 Sonnet (Latest alias)', category: 'claude-3.7', contextLimit: 200000 },
                // Claude 3.5 series
                { value: 'claude-3-5-sonnet-20241022', text: 'Claude 3.5 Sonnet v2', category: 'claude-3.5', contextLimit: 200000 },
                { value: 'claude-3-5-sonnet-latest', text: 'Claude 3.5 Sonnet (Latest alias)', category: 'claude-3.5', contextLimit: 200000 },
                { value: 'claude-3-5-sonnet-20240620', text: 'Claude 3.5 Sonnet v1', category: 'claude-3.5', contextLimit: 200000 },
                { value: 'claude-3-5-haiku-20241022', text: 'Claude 3.5 Haiku', category: 'claude-3.5', contextLimit: 200000 },
                { value: 'claude-3-5-haiku-latest', text: 'Claude 3.5 Haiku (Latest alias)', category: 'claude-3.5', contextLimit: 200000 },
                // Claude 3 series
                { value: 'claude-3-opus-20240229', text: 'Claude 3 Opus', category: 'claude-3', contextLimit: 200000 },
                { value: 'claude-3-opus-latest', text: 'Claude 3 Opus (Latest alias)', category: 'claude-3', contextLimit: 200000 },
                { value: 'claude-3-sonnet-20240229', text: 'Claude 3 Sonnet', category: 'claude-3', contextLimit: 200000 },
                { value: 'claude-3-haiku-20240307', text: 'Claude 3 Haiku', category: 'claude-3', contextLimit: 200000 }
            ],
            google: [
                // Gemini 1.5 series (Currently available)
                { value: 'gemini-1.5-pro', text: 'Gemini 1.5 Pro', category: 'current', contextLimit: 2000000 },
                { value: 'gemini-1.5-pro-latest', text: 'Gemini 1.5 Pro Latest', category: 'current', contextLimit: 2000000 },
                { value: 'gemini-1.5-pro-002', text: 'Gemini 1.5 Pro 002', category: 'current', contextLimit: 2000000 },
                { value: 'gemini-1.5-pro-001', text: 'Gemini 1.5 Pro 001', category: 'current', contextLimit: 2000000 },
                { value: 'gemini-1.5-flash', text: 'Gemini 1.5 Flash', category: 'current', contextLimit: 1000000 },
                { value: 'gemini-1.5-flash-latest', text: 'Gemini 1.5 Flash Latest', category: 'current', contextLimit: 1000000 },
                { value: 'gemini-1.5-flash-002', text: 'Gemini 1.5 Flash 002', category: 'current', contextLimit: 1000000 },
                { value: 'gemini-1.5-flash-001', text: 'Gemini 1.5 Flash 001', category: 'current', contextLimit: 1000000 },
                { value: 'gemini-1.5-flash-8b', text: 'Gemini 1.5 Flash 8B', category: 'current', contextLimit: 1000000 },
                { value: 'gemini-1.5-flash-8b-latest', text: 'Gemini 1.5 Flash 8B Latest', category: 'current', contextLimit: 1000000 },
                // Gemini 1.0 (Legacy)
                { value: 'gemini-1.0-pro', text: 'Gemini 1.0 Pro', category: 'legacy', contextLimit: 32768 },
                { value: 'gemini-1.0-pro-latest', text: 'Gemini 1.0 Pro Latest', category: 'legacy', contextLimit: 32768 },
                { value: 'gemini-1.0-pro-001', text: 'Gemini 1.0 Pro 001', category: 'legacy', contextLimit: 32768 },
                { value: 'gemini-pro', text: 'Gemini Pro (Legacy)', category: 'legacy', contextLimit: 32768 }
            ]
        };
        
        // Build modelLimits from the models data for backwards compatibility
        this.modelLimits = {};
        for (const provider in this.models) {
            for (const model of this.models[provider]) {
                this.modelLimits[model.value] = model.contextLimit;
            }
        }
        
        // Default system prompt
        this.defaultSystemPrompt = `You are the Netdata assistant, with access to Netdata monitoring data via your tools. 

For ANY request involving data analysis, troubleshooting, or complex queries, you MUST use <thinking> tags to show your complete reasoning process. In your <thinking> section, always include:

- Your interpretation of the user's request and what they're trying to accomplish
- Your strategy for approaching the problem (which tools to use and why)
- Analysis of each piece of data you retrieve
- Connections you're making between different metrics/nodes/alerts
- Any assumptions or limitations in your analysis
- Your reasoning for conclusions or recommendations

## STEP 1: IDENTIFY WHAT IS RELEVANT
<thinking>
The user is asking about [summarize request]. To answer this, I need to:
1. Identify relevant infrastructure components by...
2. My search strategy will be...
3. I expect to find...
</thinking>

[Continue with your existing process]

## STEP 2: FIND DATA TO ANSWER THE QUESTION
<thinking>
Based on Step 1 results, I found [summarize findings]. Now I need to:
1. Query specific metrics because...
2. The time range should be X because...
3. I'm looking for patterns that indicate...
</thinking>

[Continue with targeted queries]

## RESPONSE STYLE
After your <thinking> analysis, provide your response following your existing style guidelines.

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
        
        // After all initialization, create default chat if needed
        setTimeout(() => this.createDefaultChatIfNeeded(), 100);
    }
    
    // Get available models for a provider type
    getModelsForProviderType(providerType) {
        // Use the single source of truth for models
        return this.models[providerType] || [];
    }
    
    // Get model info by model ID
    getModelInfo(modelId) {
        if (!modelId) return null;
        
        // Search through all providers
        for (const [provider, models] of Object.entries(this.models)) {
            const model = models.find(m => m.value === modelId);
            if (model) {
                return model;
            }
        }
        return null;
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
        
        // Temperature control
        this.temperatureControl = document.getElementById('temperatureControl');
        this.temperatureSlider = document.getElementById('temperatureSlider');
        this.temperatureValue = document.getElementById('temperatureValue');
        
        // Initialize temperature display
        this.updateTemperatureDisplay(0.7);
        
        this.temperatureSlider.addEventListener('input', (e) => {
            this.updateTemperatureDisplay(parseFloat(e.target.value));
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
                } catch (e) {
                    // Invalid URL, ignore
                }
            }
        });
        
        // Initialize tooltips for all elements
        this.initializeTooltips(document.body);
        
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
        const modal = document.getElementById(modalId);
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
        } catch (e) {
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
                        console.warn(`Chat ${chatId} has invalid model ${chat.model}, resetting to default`);
                        
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
                        
                        if (fallbackModel) {
                            chat.model = fallbackModel;
                            
                            // Update UI if this is the current chat
                            if (chatId === this.currentChatId) {
                                this.updateModelDisplay(chat);
                            }
                        }
                    }
                }
            }
        }
        
        // Save any changes
        this.saveSettings();
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
        
        // Add models from all available providers
        Object.entries(provider.availableProviders).forEach(([providerType, config]) => {
            // Add provider header
            const header = document.createElement('div');
            header.className = 'dropdown-header';
            header.textContent = providerType.charAt(0).toUpperCase() + providerType.slice(1);
            this.llmModelDropdown.appendChild(header);
            
            // Add models
            config.models.forEach(model => {
                const modelId = typeof model === 'string' ? model : model.id;
                const item = document.createElement('button');
                item.className = 'dropdown-item';
                item.textContent = modelId;
                
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
            item.innerHTML = `
                ${server.connected ? '🟢' : '🔴'} ${server.name}
                <small style="display: block; color: var(--text-secondary); font-size: 12px;">${server.url}</small>
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
        this.saveSettings();
        
        // Update UI
        this.currentModelText.textContent = modelName;
        
        // Update context window indicator
        const { totalTokens } = this.getTokenUsageForChat(this.currentChatId);
        this.updateContextWindowIndicator(totalTokens, newModel);
        
        // Save as default for new chats
        const lastConfig = localStorage.getItem('lastChatConfig');
        let config = {};
        if (lastConfig) {
            try {
                config = JSON.parse(lastConfig);
            } catch (e) {
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
            const connection = await this.ensureMcpConnection(newServerId);
            
            chat.mcpServerId = newServerId;
            this.saveSettings();
            
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
                } catch (e) {
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
        
        // Update Tippy theme for existing tooltips
        const isDarkTheme = newTheme === 'dark';
        const allTippyInstances = document.querySelectorAll('[data-tooltip]');
        allTippyInstances.forEach(element => {
            if (element._tippy) {
                element._tippy.setProps({
                    theme: isDarkTheme ? 'light-border' : 'light'
                });
            }
        });
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
    
    showErrorWithRetry(message, retryCallback) {
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
            messageDiv.innerHTML = `
                <div><i class="fas fa-times-circle"></i> ${message}</div>
                <button class="btn btn-warning btn-small" style="margin-top: 8px;">
                    <i class="fas fa-redo"></i> Retry
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
        this.logContent.scrollTop = this.logContent.scrollHeight;
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
        
        // Remove all messages after this point
        chat.messages = chat.messages.slice(0, messageIndex + 1);
        this.saveSettings();
        
        // Reload the chat to show the truncated history
        this.loadChat(this.currentChatId);
        
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
        
        // Show loading spinner
        this.processRenderEvent({ type: 'show-spinner' });
        
        try {
            if (message.type === 'user') {
                // Redo from user message - resend it
                await this.processMessageWithTools(chat, mcpConnection, provider, message.content);
            } else if (message.type === 'assistant') {
                // Redo from assistant message - find previous user message and resend
                let lastUserMessage = null;
                for (let i = messageIndex - 1; i >= 0; i--) {
                    if (chat.messages[i].type === 'user' && !chat.messages[i].isTitleRequest) {
                        lastUserMessage = chat.messages[i];
                        break;
                    }
                }
                
                if (lastUserMessage) {
                    // Remove the assistant message we're redoing from
                    chat.messages = chat.messages.slice(0, messageIndex);
                    this.saveSettings();
                    this.loadChat(this.currentChatId);
                    
                    await this.processMessageWithTools(chat, mcpConnection, provider, lastUserMessage.content);
                } else {
                    this.showError('Cannot find previous user message to redo from');
                }
            } else if (message.type === 'tool-results') {
                // Redo from tool results - continue conversation
                // This essentially just continues the conversation from this point
                const fakeUserMessage = "[Continue from tool results]";
                await this.processMessageWithTools(chat, mcpConnection, provider, fakeUserMessage);
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
        
        // Load LLM providers (proxy configurations)
        const savedLlmProviders = localStorage.getItem('llmProviders');
        if (savedLlmProviders) {
            try {
                const providers = JSON.parse(savedLlmProviders);
                providers.forEach(p => {
                    const provider = {
                        id: p.id,
                        name: p.name,
                        proxyUrl: p.proxyUrl,
                        availableProviders: p.availableProviders,
                        onLog: (logEntry) => this.addLogEntry(p.name, logEntry)
                    };
                    this.llmProviders.set(p.id, provider);
                    
                    // Populate modelLimits from available providers
                    if (p.availableProviders) {
                        Object.entries(p.availableProviders).forEach(([providerType, config]) => {
                            if (config.models) {
                                config.models.forEach(model => {
                                    if (typeof model === 'object' && model.id && model.contextWindow) {
                                        this.modelLimits[model.id] = model.contextWindow;
                                    }
                                });
                            }
                        });
                    }
                });
                this.updateLlmProvidersList();
            } catch (e) {
                console.error('Failed to load LLM providers:', e);
            }
        }
        
        // Load chats
        const savedChats = localStorage.getItem('chats');
        if (savedChats) {
            try {
                const chats = JSON.parse(savedChats);
                chats.forEach(chat => {
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
                                console.warn(`Chat ${chat.id} has invalid model ${chat.model}, resetting to default`);
                                
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
                                } catch (e) {
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
                                
                                if (defaultModel) {
                                    chat.model = defaultModel;
                                }
                            }
                        }
                    }
                    
                    this.chats.set(chat.id, chat);
                });
                this.updateChatSessions();
                
                // Always create a new chat on startup instead of loading the last one
                this.shouldCreateDefaultChat = true;
            } catch (e) {
                console.error('Failed to load chats:', e);
            }
        } else {
            // No saved chats, will create one after initialization
            this.shouldCreateDefaultChat = true;
        }
    }

    async initializeDefaultLLMProvider() {
        // Check if we already have LLM providers
        if (this.llmProviders.size > 0) {
            return;
        }

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
            
            // Create the default LLM provider
            const providerId = 'default_llm_provider';
            const provider = {
                id: providerId,
                name: 'Local LLM Proxy',
                proxyUrl: proxyUrl,
                availableProviders: providers,
                onLog: (logEntry) => this.addLogEntry('Local LLM Proxy', logEntry)
            };
            
            // Populate modelLimits from available providers
            Object.entries(providers).forEach(([providerType, config]) => {
                if (config.models) {
                    config.models.forEach(model => {
                        if (typeof model === 'object' && model.id && model.contextWindow) {
                            this.modelLimits[model.id] = model.contextWindow;
                        }
                    });
                }
            });
            
            this.llmProviders.set(providerId, provider);
            this.saveSettings();
            this.updateLlmProvidersList();
            this.updateNewChatSelectors();
            
            // Validate all existing chats have valid models
            this.validateChatModels();
            
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
        for (const [id, server] of this.mcpServers) {
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
                
            } catch (error) {
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
        
        // Save LLM providers (proxy configurations only, no API keys)
        const providersToSave = Array.from(this.llmProviders.entries()).map(([id, provider]) => ({
            id: id,
            name: provider.name,
            proxyUrl: provider.proxyUrl,
            availableProviders: provider.availableProviders
        }));
        localStorage.setItem('llmProviders', JSON.stringify(providersToSave));
        
        // Save only chats that have been marked as saved
        const chatsToSave = Array.from(this.chats.values()).filter(chat => chat.isSaved !== false);
        localStorage.setItem('chats', JSON.stringify(chatsToSave));
        
        // Save current chat ID
        if (this.currentChatId) {
            localStorage.setItem('currentChatId', this.currentChatId);
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
        // Update MCP server selector
        this.newChatMcpServer.innerHTML = '<option value="">Select MCP Server</option>';
        for (const [id, server] of this.mcpServers) {
            const option = document.createElement('option');
            option.value = id;
            option.textContent = server.name;
            this.newChatMcpServer.appendChild(option);
        }
        
        // Update LLM provider selector
        this.newChatLlmProvider.innerHTML = '<option value="">Select LLM Provider</option>';
        for (const [id, provider] of this.llmProviders) {
            const option = document.createElement('option');
            option.value = id;
            option.textContent = provider.name;
            this.newChatLlmProvider.appendChild(option);
        }
        
        // Reset model selector
        this.newChatModelGroup.style.display = 'none';
        this.newChatModel.innerHTML = '<option value="">Select Model</option>';
        
        // If only one MCP server, auto-select it
        if (this.mcpServers.size === 1) {
            const [id] = this.mcpServers.keys();
            this.newChatMcpServer.value = id;
        }
        
        // If only one LLM provider, auto-select it and load models
        if (this.llmProviders.size === 1) {
            const [id] = this.llmProviders.keys();
            this.newChatLlmProvider.value = id;
            // Trigger model update
            this.updateNewChatModels();
        }
    }
    
    updateNewChatModels() {
        const providerId = this.newChatLlmProvider.value;
        if (!providerId) {
            this.newChatModelGroup.style.display = 'none';
            return;
        }
        
        const provider = this.llmProviders.get(providerId);
        if (!provider || !provider.availableProviders) {
            this.newChatModelGroup.style.display = 'none';
            return;
        }
        
        // Show model selector
        this.newChatModelGroup.style.display = 'block';
        this.newChatModel.innerHTML = '<option value="">Select Model</option>';
        
        // Add models from all available providers
        Object.entries(provider.availableProviders).forEach(([providerType, config]) => {
            const optgroup = document.createElement('optgroup');
            optgroup.label = providerType.charAt(0).toUpperCase() + providerType.slice(1);
            
            config.models.forEach(model => {
                const option = document.createElement('option');
                // Handle both object format (new) and string format (legacy)
                const modelId = typeof model === 'string' ? model : model.id;
                const contextWindow = typeof model === 'object' ? model.contextWindow : null;
                
                // Store both provider type and model in the value
                option.value = `${providerType}:${modelId}`;
                option.textContent = modelId;
                
                // Store context window in data attribute if available
                if (contextWindow) {
                    option.setAttribute('data-context-window', contextWindow);
                }
                
                optgroup.appendChild(option);
            });
            
            this.newChatModel.appendChild(optgroup);
        });
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
        let mcpConnection;
        try {
            mcpConnection = await this.ensureMcpConnection(mcpServerId);
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
        
        // Extract model name from format "provider:model-name"
        const modelName = selectedModel.includes(':') ? selectedModel.split(':')[1] : selectedModel;
        
        const chatId = `chat_${Date.now()}`;
        const chat = {
            id: chatId,
            title: title,
            mcpServerId: mcpServerId,
            llmProviderId: llmProviderId,
            model: selectedModel, // Selected model for this chat
            messages: [],
            temperature: 0.7, // Default temperature
            systemPrompt: this.lastSystemPrompt, // Use the last system prompt
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
            currentTurn: 0, // Track conversation turns
            toolInclusionMode: 'cached', // 'auto', 'all-on', 'all-off', 'manual', or 'cached'
            isSaved: isSaved // Track whether this chat has been saved to localStorage
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
            
            const server = this.mcpServers.get(chat.mcpServerId);
            const provider = this.llmProviders.get(chat.llmProviderId);
            
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
        
        this.currentChatId = chatId;
        this.updateChatSessions();
        
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
        this.chatMcp.textContent = server ? `MCP: ${server.name}` : 'MCP: Not found';
        if (provider) {
            // Parse the model format "provider:model"
            const modelDisplay = chat.model ? chat.model.split(':')[1] || chat.model : 'No model selected';
            this.chatLlm.textContent = `LLM: ${provider.name} (${modelDisplay})`;
        } else {
            this.chatLlm.textContent = 'LLM: Not found';
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
            this.chatInput.placeholder = "Ask about your Netdata metrics...";
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
                this.chatInput.placeholder = "MCP server not found";
            } else if (!provider) {
                this.chatInput.placeholder = "LLM provider not found";
            } else if (!this.mcpConnections.has(chat.mcpServerId)) {
                this.chatInput.placeholder = "MCP server disconnected - click Reconnect";
                // Show reconnect button
                this.showReconnectButton(chat.mcpServerId);
            } else {
                this.chatInput.placeholder = "MCP server or LLM provider not available";
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
        this.metricsIdMapping = {}; // Reset metrics ID mapping
        this.currentStepInTurn = 1; // Initialize step counter
        this.lastDisplayedTurn = 0; // Track last displayed turn for separators
        
        // Update global toggle UI based on chat's tool inclusion mode
        this.updateGlobalToggleUI();
        
        // Display system prompt as first message
        this.displaySystemPrompt(chat.systemPrompt || this.defaultSystemPrompt);
        
        // Track previous prompt tokens for delta calculation
        let previousPromptTokens = 0;
        
        let inTitleGeneration = false;
        
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
                // Add the success message
                const successDiv = document.createElement('div');
                successDiv.className = 'message system';
                successDiv.style.cssText = 'margin: 10px auto 20px; padding: 8px 16px; background: var(--success-color); color: white; text-align: center; border-radius: 4px; max-width: 80%;';
                successDiv.innerHTML = `<i class="fas fa-check-circle"></i> Chat title updated: "${chat.title}"`;
                this.chatMessages.appendChild(successDiv);
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
        this.updateCumulativeTokenDisplay();
        
        this.scrollToBottom();
        
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
                events.push({ type: 'system-title-message', content: msg.content });
                break;
                
            case 'system-summary':
                events.push({ type: 'system-summary-message', content: msg.content });
                break;
                
            case 'title':
                events.push({ type: 'title-message', content: msg.content });
                break;
                
            case 'summary':
                events.push({ type: 'summary-message', content: msg.content });
                break;
                
            case 'accounting':
                events.push({ type: 'accounting-message', data: msg });
                break;
                
            case 'assistant':
                // Add metrics placeholder event (like we do in live chats)
                const metricsId = `metrics-stored-${this.metricsIdCounter++}`;
                events.push({ type: 'metrics-placeholder', id: metricsId });
                
                // Add assistant message event
                if (msg.content) {
                    events.push({ type: 'assistant-message', content: msg.content });
                }
                
                // Add tool calls
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
                
                // Update metrics event
                if (msg.usage || msg.responseTime) {
                    events.push({ 
                        type: 'update-metrics', 
                        id: metricsId,
                        usage: msg.usage, 
                        responseTime: msg.responseTime,
                        previousPromptTokens: this.previousPromptTokensForDisplay
                    });
                }
                break;
                
            case 'tool-results':
                // Add tool results with their inclusion state
                for (const result of msg.results) {
                    events.push({ 
                        type: 'tool-result', 
                        name: result.name, 
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
                
                this.addMessage('user', event.content, event.messageIndex);
                if (turn > 0) {
                    this.addStepNumber(turn, this.currentStepInTurn++);
                }
                break;
                
            case 'metrics-placeholder':
                const placeholder = this.addMetricsPlaceholder();
                // Store mapping of event ID to actual DOM ID
                if (event.id) {
                    this.metricsIdMapping = this.metricsIdMapping || {};
                    this.metricsIdMapping[event.id] = placeholder;
                }
                break;
                
            case 'assistant-message':
                this.addMessage('assistant', event.content, event.messageIndex);
                const chat2 = this.chats.get(this.currentChatId);
                const turn2 = event.turn !== undefined ? event.turn : (chat2 ? chat2.currentTurn : 0);
                if (turn2 > 0 && this.currentAssistantGroup) {
                    this.addStepNumber(turn2, this.currentStepInTurn++, this.currentAssistantGroup);
                }
                break;
                
            case 'tool-call':
                // Ensure we have a group
                if (!this.currentAssistantGroup) {
                    this.addMessage('assistant', '');
                }
                this.addToolCall(event.name, event.arguments, event.includeInContext !== false, event.turn, event.messageIndex);
                break;
                
            case 'tool-result':
                this.addToolResult(event.name, event.result, event.responseTime || 0, event.responseSize || null, event.includeInContext, event.messageIndex);
                break;
                
            case 'update-metrics':
                // Find the placeholder by mapped ID
                const domId = this.metricsIdMapping?.[event.id] || event.id;
                this.updateMetricsPlaceholder(domId, event.usage, event.responseTime, event.previousPromptTokens);
                break;
                
            case 'system-message':
                this.addSystemMessage(event.content);
                break;
                
            case 'system-title-message':
                this.addMessage('system-title', event.content, event.messageIndex);
                break;
                
            case 'system-summary-message':
                this.addMessage('system-summary', event.content, event.messageIndex);
                break;
                
            case 'title-message':
                this.addMessage('title', event.content, event.messageIndex);
                break;
                
            case 'summary-message':
                this.addMessage('summary', event.content, event.messageIndex);
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
                        // Remove messages from the error onwards
                        const errorIndex = chat.messages.findIndex(m => m.type === 'error' && m.content === event.content);
                        if (errorIndex !== -1) {
                            // Create accounting node before discarding messages
                            const messagesToDiscard = chat.messages.length - errorIndex;
                            if (messagesToDiscard > 0) {
                                const accountingNode = this.createAccountingNode('Retry after error', messagesToDiscard);
                                chat.messages.splice(errorIndex, 0, accountingNode);
                            }
                            
                            chat.messages = chat.messages.slice(0, errorIndex + 1); // Keep accounting node
                            this.saveSettings();
                            
                            // Reload chat to show cleaned history
                            this.loadChat(this.currentChatId);
                            
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
            this.saveSettings();
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
        
        // Process user message event
        this.processRenderEvent({ type: 'user-message', content: message });
        chat.messages.push({ type: 'user', role: 'user', content: message, turn: chat.currentTurn });
        
        // If this is the first message in an unsaved chat, mark it as saved
        if (chat.isSaved === false && chat.messages.filter(m => m.type === 'user').length === 1) {
            chat.isSaved = true;
            this.saveSettings();
            this.updateChatSessions();
        }
        
        // Show loading spinner event
        this.processRenderEvent({ type: 'show-spinner' });
        
        try {
            await this.processMessageWithTools(chat, mcpConnection, provider, message);
            
            // Check if we should generate a title (first user message and got a response)
            const userMessages = chat.messages.filter(m => m.type === 'user' && !m.isTitleRequest);
            const assistantMessages = chat.messages.filter(m => m.type === 'assistant' && !m.isTitleRequest);
            
            if (userMessages.length === 1 && assistantMessages.length > 0 && !chat.titleGenerated) {
                // Generate title automatically
                await this.generateChatTitle(chat, mcpConnection, provider);
            }
        } catch (error) {
            // Show error with retry button
            this.showErrorWithRetry(`Error: ${error.message}`, async () => {
                // Retry the last message
                const lastUserMessage = chat.messages.filter(m => m.type === 'user').pop();
                if (lastUserMessage) {
                    // Remove the error message and the failed attempt's messages
                    const lastUserIndex = chat.messages.lastIndexOf(lastUserMessage);
                    chat.messages = chat.messages.slice(0, lastUserIndex + 1);
                    this.saveSettings();
                    
                    // Reload chat and retry
                    this.loadChat(this.currentChatId);
                    try {
                        await this.processMessageWithTools(chat, mcpConnection, provider, lastUserMessage.content);
                    } catch (retryError) {
                        this.showError(`Retry failed: ${retryError.message}`);
                    }
                }
            });
            chat.messages.push({ type: 'error', content: error.message });
        } finally {
            // Remove loading spinner event
            this.processRenderEvent({ type: 'hide-spinner' });
            
            // Clear current assistant group after processing is complete
            this.currentAssistantGroup = null;
            
            chat.updatedAt = new Date().toISOString();
            this.saveSettings();
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

    buildMessagesForAPI(chat, provider, freezeCache = false) {
        const messages = [];
        let cacheControlIndex = null;
        let lastCacheControlIndex = null;
        
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
        
        // Add system prompt (always needed at start of conversation)
        // If we have a summary, include it in the system prompt
        const userMessagesAfterCheckpoint = chat.messages.slice(startIndex).filter(m => m.role === 'user').length;
        if (userMessagesAfterCheckpoint === 1 || (startIndex === 0 && chat.messages.filter(m => m.role === 'user').length === 1)) {
            let systemContent = chat.systemPrompt || this.defaultSystemPrompt;
            
            // If there's a summary, prepend it to the system prompt
            if (summaryMessage) {
                systemContent = `Previous Conversation Summary:\n${summaryMessage.content}\n\n${systemContent}`;
            }
            
            messages.push({ role: 'system', content: systemContent });
        }
        
        // Build messages from checkpoint onwards
        let apiMessageIndex = messages.length; // Start after system message if present
        
        for (let i = startIndex; i < chat.messages.length; i++) {
            const msg = chat.messages[i];
            
            // Skip system roles - they should never be sent to API
            if (['system-title', 'system-summary', 'title', 'summary', 'accounting'].includes(msg.role)) {
                continue;
            }
            
            // Process based on type/role
            const messageType = msg.type || msg.role;
            
            if (messageType === 'user') {
                messages.push({ role: 'user', content: msg.content });
                apiMessageIndex++;
            } else if (messageType === 'assistant') {
                // Process assistant message (existing logic)
                const cleanedContent = msg.content ? this.cleanContentForAPI(msg.content) : '';
                
                // Check if we should include tool calls
                const toolMode = chat.toolInclusionMode || 'auto';
                const shouldIncludeTools = this.shouldIncludeToolsForMessage(msg, toolMode, msg.turn || 0);
                
                if (msg.toolCalls && msg.toolCalls.length > 0 && shouldIncludeTools) {
                    // Add message with tool calls (provider-specific format)
                    if (provider.type === 'anthropic') {
                        messages.push({
                            role: 'assistant',
                            content: [
                                ...(cleanedContent ? [{ type: 'text', text: cleanedContent }] : []),
                                ...msg.toolCalls.map(tc => ({
                                    type: 'tool_use',
                                    id: tc.id,
                                    name: tc.name,
                                    input: tc.arguments
                                }))
                            ]
                        });
                    } else if (provider.type === 'openai') {
                        messages.push({
                            role: 'assistant',
                            content: cleanedContent,
                            tool_calls: msg.toolCalls.map(tc => ({
                                id: tc.id,
                                type: 'function',
                                function: {
                                    name: tc.name,
                                    arguments: JSON.stringify(tc.arguments)
                                }
                            }))
                        });
                    } else {
                        messages.push({
                            role: 'assistant',
                            content: cleanedContent,
                            toolCalls: msg.toolCalls
                        });
                    }
                } else if (cleanedContent) {
                    messages.push({ role: 'assistant', content: cleanedContent });
                }
                apiMessageIndex++;
            } else if (messageType === 'tool-results') {
                // Process tool results (existing logic)
                const toolMode = chat.toolInclusionMode || 'auto';
                const shouldIncludeTools = this.shouldIncludeToolsForMessage(msg, toolMode, msg.turn || 0);
                
                if (shouldIncludeTools) {
                    const includedResults = msg.results.filter(tr => tr.includeInContext !== false);
                    
                    if (includedResults.length > 0) {
                        if (provider.type === 'anthropic') {
                            messages.push({
                                role: 'user',
                                content: includedResults.map(tr => ({
                                    type: 'tool_result',
                                    tool_use_id: tr.toolCallId,
                                    content: typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result)
                                }))
                            });
                            apiMessageIndex++;
                        } else {
                            for (const tr of includedResults) {
                                messages.push(provider.formatToolResponse(tr.toolCallId, tr.result, tr.name));
                                apiMessageIndex++;
                            }
                        }
                    }
                }
            }
        }
        
        // Determine cache control position
        if (freezeCache && lastCacheControlIndex !== null) {
            cacheControlIndex = lastCacheControlIndex;
        } else {
            // Normal behavior - cache control goes on the last message
            cacheControlIndex = messages.length - 1;
        }
        
        return { messages, cacheControlIndex };
    }

    async processMessageWithTools(chat, mcpConnection, provider, userMessage, firstMetricsId = null) {
        // Build conversation history using the new function
        const { messages, cacheControlIndex } = this.buildMessagesForAPI(chat, provider);
        
        
        // Get available tools
        const tools = Array.from(mcpConnection.tools.values());
        
        let attempts = 0;
        // No limit on attempts - let the LLM decide when it's done
        
        // Create assistant group at the start of processing
        // We'll add all content from this conversation turn to this single group
        this.currentAssistantGroup = null;
        
        while (true) {
            attempts++
            
            // Check if we should stop processing
            if (this.shouldStopProcessing) {
                this.showSystemMessage('⏹️ Stopped by user');
                break;
            }
            
            let metricsId;
            if (attempts === 1 && firstMetricsId) {
                // First iteration - use the metrics placeholder already added after user message
                metricsId = firstMetricsId;
            } else {
                // Subsequent iterations - add new metrics placeholder
                const eventId = `metrics-live-${this.metricsIdCounter++}`;
                this.processRenderEvent({ type: 'metrics-placeholder', id: eventId });
                metricsId = this.metricsIdMapping?.[eventId] || eventId;
            }
            
            // Send to LLM with current temperature
            const temperature = this.getCurrentTemperature();
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, tools, temperature, chat.toolInclusionMode, cacheControlIndex);
            console.log('LLM response:', { 
                content: response.content?.substring(0, 200), 
                hasThinking: response.content?.includes('<thinking>'),
                toolCalls: response.toolCalls?.length 
            });
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Track token usage
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, chat.model || provider.model);
            }
            
            // Update the metrics placeholder with actual data
            this.processRenderEvent({ 
                type: 'update-metrics', 
                id: metricsId,
                usage: response.usage, 
                responseTime: llmResponseTime,
                previousPromptTokens: this.getPreviousPromptTokens()
            });
            
            // If no tool calls, display response and finish
            if (!response.toolCalls || response.toolCalls.length === 0) {
                if (response.content) {
                    // Process assistant message event
                    this.processRenderEvent({ type: 'assistant-message', content: response.content });
                    chat.messages.push({ 
                        type: 'assistant', 
                        role: 'assistant', 
                        content: response.content,
                        usage: response.usage || null,
                        responseTime: llmResponseTime || null,
                        turn: chat.currentTurn
                    });
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
                // Display the message
                if (response.content) {
                    // Process assistant message event
                    this.processRenderEvent({ type: 'assistant-message', content: response.content });
                }
                
                // Store in our improved internal format
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
                    turn: chat.currentTurn,
                    cacheControlIndex: cacheControlIndex  // Store where cache control was placed
                };
                chat.messages.push(assistantMessage);
                assistantMessageIndex = chat.messages.length - 1;
                
                // Build the message for the API
                const cleanedContent = response.content ? this.cleanContentForAPI(response.content) : '';
                
                if (provider.type === 'anthropic' && response.toolCalls && response.toolCalls.length > 0) {
                    // Anthropic format with content blocks
                    messages.push({
                        role: 'assistant',
                        content: [
                            ...(cleanedContent ? [{ type: 'text', text: cleanedContent }] : []),
                            ...response.toolCalls.map(tc => ({
                                type: 'tool_use',
                                id: tc.id,
                                name: tc.name,
                                input: tc.arguments
                            }))
                        ]
                    });
                } else if (response.toolCalls && response.toolCalls.length > 0) {
                    // Other providers (OpenAI, Google)
                    messages.push({
                        role: 'assistant',
                        content: cleanedContent,
                        tool_calls: response.toolCalls.map(tc => ({
                            id: tc.id,
                            type: 'function',
                            function: {
                                name: tc.name,
                                arguments: JSON.stringify(tc.arguments)
                            }
                        }))
                    });
                } else if (cleanedContent) {
                    // Just text, no tool calls
                    messages.push({ role: 'assistant', content: cleanedContent });
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
                    chat.messages.push({
                        type: 'tool-results',
                        results: toolResults,
                        turn: chat.currentTurn
                    });
                    
                    // Filter included results for API
                    const includedResults = toolResults.filter(tr => tr.includeInContext !== false);
                    
                    if (includedResults.length > 0) {
                        // Add to conversation based on provider
                        if (provider.type === 'anthropic') {
                            // Anthropic wants tool results in a user message
                            messages.push({
                                role: 'user',
                                content: includedResults.map(tr => ({
                                    type: 'tool_result',
                                    tool_use_id: tr.toolCallId,
                                    content: typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result)
                                }))
                            });
                        } else {
                            // OpenAI and others want individual tool messages
                            for (const tr of includedResults) {
                                messages.push(provider.formatToolResponse(
                                    tr.toolCallId,
                                    tr.result,
                                    tr.name
                                ));
                            }
                        }
                    }
                    
                    // Reset assistant group after tool results so next metrics appears separately
                    this.processRenderEvent({ type: 'reset-assistant-group' });
                    
                    // Show thinking spinner again before next LLM call
                    this.processRenderEvent({ type: 'show-spinner', text: 'Thinking...' });
                }
            }
        }
    }

    addMessage(role, content, messageIndex) {
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
        
        // Add redo button if we have a message index
        if (messageIndex !== undefined && role !== 'system') {
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
            console.log('Found thinking tags in assistant message:', content);
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
                    textDiv.innerHTML = marked.parse(part.content);
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
                    contentWrapper.innerHTML = marked.parse(part.content);
                    
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
            console.log('Creating system message block for role:', role, 'content:', content);
            
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
            
            // Store reference for when response comes
            systemBlock.pendingResponse = true;
            
            return; // Don't append messageDiv to chat
        } else if (role === 'title' || role === 'summary') {
            // Handle system responses - add to existing system block
            console.log('Adding system response for role:', role, 'content:', content);
            
            const isTitle = role === 'title';
            const blockType = isTitle ? 'title' : 'summary';
            let systemBlock = this.chatMessages.querySelector(`.system-block[data-type="${blockType}"]`);
            
            if (systemBlock && systemBlock.pendingResponse) {
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
                
                responseControls.appendChild(responseLabel);
                responseSection.appendChild(responseControls);
                
                const responseContent = document.createElement('pre');
                responseContent.textContent = content;
                responseSection.appendChild(responseContent);
                
                systemContent.appendChild(responseSection);
                
                // Clear pending response flag
                delete systemBlock.pendingResponse;
                
                return; // Don't append messageDiv to chat
            }
        } else {
            // Regular message without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            
            if (role === 'assistant' || role === 'user') {
                // Use marked to render markdown for assistant and user messages
                contentDiv.innerHTML = marked.parse(content);
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
        this.scrollToBottom();
        this.moveSpinnerToBottom();
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
        
        messageDiv.innerHTML = `
            <div class="accounting-line">
                <div class="accounting-content">
                    <span class="accounting-icon"><i class="fas fa-coins"></i></span>
                    <div class="accounting-tokens">
                        <span class="accounting-token-item">
                            <i class="fas fa-download"></i>
                            <span>${formatNumber(tokens.inputTokens || 0)}</span>
                        </span>
                        <span class="accounting-token-item">
                            <i class="fas fa-upload"></i>
                            <span>${formatNumber(tokens.outputTokens || 0)}</span>
                        </span>
                        ${tokens.cacheReadTokens ? `
                        <span class="accounting-token-item">
                            <i class="fas fa-memory"></i>
                            <span>${formatNumber(tokens.cacheReadTokens)}</span>
                        </span>` : ''}
                        ${tokens.cacheCreationTokens ? `
                        <span class="accounting-token-item">
                            <i class="fas fa-save"></i>
                            <span>${formatNumber(tokens.cacheCreationTokens)}</span>
                        </span>` : ''}
                    </div>
                    <span class="accounting-reason">${data.reason || 'Token checkpoint'}</span>
                </div>
            </div>
        `;
        
        this.chatMessages.appendChild(messageDiv);
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }
    
    createAccountingNode(reason, messagesToDiscard = 0) {
        // Calculate current cumulative tokens before discarding messages
        const cumulative = this.getCumulativeTokenUsage(this.currentChatId);
        
        return {
            role: 'accounting',
            timestamp: new Date().toISOString(),
            cumulativeTokens: {
                inputTokens: cumulative.inputTokens,
                outputTokens: cumulative.outputTokens,
                cacheReadTokens: cumulative.cacheReadTokens,
                cacheCreationTokens: cumulative.cacheCreationTokens
            },
            reason: reason,
            discardedMessages: messagesToDiscard
        };
    }
    
    displaySystemPrompt(prompt) {
        const promptDiv = document.createElement('div');
        promptDiv.className = 'system-prompt-display';
        
        const headerDiv = document.createElement('div');
        headerDiv.className = 'system-prompt-header';
        headerDiv.innerHTML = `<span class="system-prompt-label">System Prompt</span>`;
        
        const contentDiv = document.createElement('div');
        contentDiv.className = 'system-prompt-content';
        contentDiv.textContent = prompt;
        
        promptDiv.appendChild(headerDiv);
        promptDiv.appendChild(contentDiv);
        
        this.chatMessages.appendChild(promptDiv);
        
        // Add edit trigger after element is in DOM
        this.addEditTrigger(contentDiv, prompt, 'system');
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
    
    // Add a placeholder for metrics that will be filled later
    addMetricsPlaceholder() {
        const metricsId = `metrics-${this.metricsIdCounter++}`;
        
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        metricsFooter.id = metricsId;
        metricsFooter.innerHTML = '<span class="metric-item"><i class="fas fa-hourglass-half"></i> Waiting for response...</span>';
        
        // Always append at the end - the spinner will be moved after if needed
        this.chatMessages.appendChild(metricsFooter);
        
        // Move spinner to bottom if it exists
        this.moveSpinnerToBottom();
        
        return metricsId;
    }
    
    // Update a metrics placeholder with actual data
    updateMetrics(metricsId, usage, responseTime) {
        const metricsFooter = document.getElementById(metricsId);
        if (!metricsFooter) return;
        
        let metricsHtml = '';
        
        // Add response time
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item" data-tooltip="Time taken for the assistant to respond"><i class="fas fa-clock"></i> ${timeSeconds}s</span>`;
        }
        
        // Add token usage
        if (usage) {
            const formatNumber = (num) => num.toLocaleString();
            
            // Calculate delta (tokens added in this step)
            let deltaTokens = 0;
            
            if (this.currentChatId) {
                const history = this.tokenUsageHistory.get(this.currentChatId);
                if (history && history.requests.length > 1) {
                    // Get previous request's prompt tokens
                    const previousRequest = history.requests[history.requests.length - 2];
                    deltaTokens = usage.promptTokens - previousRequest.promptTokens;
                } else if (history && history.requests.length === 1) {
                    // First request - all tokens are new
                    deltaTokens = usage.promptTokens;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Total tokens in the context window sent to the model"><i class="fas fa-download"></i> ${formatNumber(usage.promptTokens)}</span>`;
            
            // Add delta if there was an increase
            if (deltaTokens > 0) {
                metricsHtml += `<span class="metric-item" style="color: var(--warning-color)" data-tooltip="New tokens added since previous request">+${formatNumber(deltaTokens)}</span>`;
            }
            
            // Add cache information if available (Anthropic)
            if (usage.cacheReadInputTokens > 0 || usage.cacheCreationInputTokens > 0) {
                if (usage.cacheReadInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--success-color)" data-tooltip="Tokens read from cache (90% discount)"><i class="fas fa-memory"></i> ${formatNumber(usage.cacheReadInputTokens)}</span>`;
                }
                if (usage.cacheCreationInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--info-color)" data-tooltip="Tokens written to cache (25% surcharge)"><i class="fas fa-save"></i> ${formatNumber(usage.cacheCreationInputTokens)}</span>`;
                }
            }
            
            // Calculate the true total including cache tokens
            const trueTotal = usage.promptTokens + 
                              (usage.cacheReadInputTokens || 0) + 
                              (usage.cacheCreationInputTokens || 0) + 
                              usage.completionTokens;
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Tokens generated by the assistant"><i class="fas fa-upload"></i> ${formatNumber(usage.completionTokens)}</span>
                <span class="metric-item" data-tooltip="Total tokens (input + cache + output)"><i class="fas fa-chart-bar"></i> ${formatNumber(trueTotal)}</span>
            `;
        }
        
        metricsFooter.innerHTML = metricsHtml;
        
        // Initialize Tippy tooltips for the metric items
        this.initializeTooltips(metricsFooter);
    }
    
    // Update a metrics placeholder with actual data (unified method for both live and stored)
    updateMetricsPlaceholder(metricsId, usage, responseTime, previousPromptTokens) {
        const metricsFooter = document.getElementById(metricsId);
        if (!metricsFooter) return;
        
        let metricsHtml = '';
        
        // Add response time
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item" data-tooltip="Time taken for the assistant to respond"><i class="fas fa-clock"></i> ${timeSeconds}s</span>`;
        }
        
        // Add token usage
        if (usage) {
            const formatNumber = (num) => num.toLocaleString();
            
            // Calculate delta (tokens added in this step)
            let deltaTokens = 0;
            
            // Use provided previous tokens if available (for stored messages)
            if (previousPromptTokens !== undefined && previousPromptTokens !== null) {
                deltaTokens = usage.promptTokens - previousPromptTokens;
            } else if (this.currentChatId) {
                // Otherwise try to calculate from history
                const history = this.tokenUsageHistory.get(this.currentChatId);
                if (history && history.requests.length > 1) {
                    // Get previous request's prompt tokens
                    const previousRequest = history.requests[history.requests.length - 2];
                    deltaTokens = usage.promptTokens - previousRequest.promptTokens;
                } else if (history && history.requests.length === 1) {
                    // First request - all tokens are new
                    deltaTokens = usage.promptTokens;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Total tokens in the context window sent to the model"><i class="fas fa-download"></i> ${formatNumber(usage.promptTokens)}</span>`;
            
            // Add delta if there was an increase
            if (deltaTokens > 0) {
                metricsHtml += `<span class="metric-item" style="color: var(--warning-color)" data-tooltip="New tokens added since previous request">+${formatNumber(deltaTokens)}</span>`;
            }
            
            // Add cache information if available (Anthropic)
            if (usage.cacheReadInputTokens > 0 || usage.cacheCreationInputTokens > 0) {
                if (usage.cacheReadInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--success-color)" data-tooltip="Tokens read from cache (90% discount)"><i class="fas fa-memory"></i> ${formatNumber(usage.cacheReadInputTokens)}</span>`;
                }
                if (usage.cacheCreationInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--info-color)" data-tooltip="Tokens written to cache (25% surcharge)"><i class="fas fa-save"></i> ${formatNumber(usage.cacheCreationInputTokens)}</span>`;
                }
            }
            
            // Calculate the true total including cache tokens
            const trueTotal = usage.promptTokens + 
                              (usage.cacheReadInputTokens || 0) + 
                              (usage.cacheCreationInputTokens || 0) + 
                              usage.completionTokens;
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Tokens generated by the assistant"><i class="fas fa-upload"></i> ${formatNumber(usage.completionTokens)}</span>
                <span class="metric-item" data-tooltip="Total tokens (input + cache + output)"><i class="fas fa-chart-bar"></i> ${formatNumber(trueTotal)}</span>
            `;
        }
        
        metricsFooter.innerHTML = metricsHtml;
        
        // Initialize Tippy tooltips for the metric items
        this.initializeTooltips(metricsFooter);
    }
    
    // Add metrics to the previous message element
    addMetricsToPreviousMessage(usage, responseTime) {
        const allMessages = this.chatMessages.querySelectorAll('.message, .assistant-group');
        if (allMessages.length === 0) return;
        
        // Get the last message element (excluding loading spinner)
        let lastMessage = null;
        for (let i = allMessages.length - 1; i >= 0; i--) {
            const msg = allMessages[i];
            if (!msg.classList.contains('loading-spinner')) {
                lastMessage = msg;
                break;
            }
        }
        
        if (!lastMessage) return;
        
        // Check if it already has metrics
        if (lastMessage.querySelector('.assistant-metrics-footer')) return;
        
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        
        let metricsHtml = '';
        
        // Add response time
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item" data-tooltip="Time taken for the assistant to respond"><i class="fas fa-clock"></i> ${timeSeconds}s</span>`;
        }
        
        // Add token usage
        if (usage) {
            const formatNumber = (num) => num.toLocaleString();
            
            // Calculate delta (tokens added in this step)
            let deltaTokens = 0;
            
            if (this.currentChatId) {
                const history = this.tokenUsageHistory.get(this.currentChatId);
                if (history && history.requests.length > 1) {
                    // Get previous request's prompt tokens
                    const previousRequest = history.requests[history.requests.length - 2];
                    deltaTokens = usage.promptTokens - previousRequest.promptTokens;
                } else if (history && history.requests.length === 1) {
                    // First request - all tokens are new
                    deltaTokens = usage.promptTokens;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Total tokens in the context window sent to the model"><i class="fas fa-download"></i> ${formatNumber(usage.promptTokens)}</span>`;
            
            // Add delta if there was an increase
            if (deltaTokens > 0) {
                metricsHtml += `<span class="metric-item" style="color: var(--warning-color)" data-tooltip="New tokens added since previous request">+${formatNumber(deltaTokens)}</span>`;
            }
            
            // Add cache information if available (Anthropic)
            if (usage.cacheReadInputTokens > 0 || usage.cacheCreationInputTokens > 0) {
                if (usage.cacheReadInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--success-color)" data-tooltip="Tokens read from cache (90% discount)"><i class="fas fa-memory"></i> ${formatNumber(usage.cacheReadInputTokens)}</span>`;
                }
                if (usage.cacheCreationInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--info-color)" data-tooltip="Tokens written to cache (25% surcharge)"><i class="fas fa-save"></i> ${formatNumber(usage.cacheCreationInputTokens)}</span>`;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Tokens generated by the assistant"><i class="fas fa-upload"></i> ${formatNumber(usage.completionTokens)}</span>
                <span class="metric-item" data-tooltip="Total tokens used (prompt + completion)"><i class="fas fa-chart-bar"></i> ${formatNumber(usage.totalTokens)}</span>
            `;
        }
        
        metricsFooter.innerHTML = metricsHtml;
        lastMessage.appendChild(metricsFooter);
        
        // Initialize Tippy tooltips for the metric items
        this.initializeTooltips(metricsFooter);
    }
    
    // Add metrics directly to the current assistant group (for stored messages)
    addMetricsToAssistantGroup(usage, responseTime, previousPromptTokens) {
        if (!this.currentAssistantGroup) return;
        
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        
        let metricsHtml = '';
        
        // Add response time
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item" data-tooltip="Time taken for the assistant to respond"><i class="fas fa-clock"></i> ${timeSeconds}s</span>`;
        }
        
        // Add token usage
        if (usage) {
            const formatNumber = (num) => num.toLocaleString();
            
            // Calculate delta (tokens added in this step)
            let deltaTokens = 0;
            
            // Use provided previous tokens if available (for stored messages)
            if (previousPromptTokens !== undefined && previousPromptTokens !== null) {
                deltaTokens = usage.promptTokens - previousPromptTokens;
            } else if (this.currentChatId) {
                // Otherwise try to calculate from history
                const history = this.tokenUsageHistory.get(this.currentChatId);
                if (history && history.requests.length > 1) {
                    // Get previous request's prompt tokens
                    const previousRequest = history.requests[history.requests.length - 2];
                    deltaTokens = usage.promptTokens - previousRequest.promptTokens;
                } else if (history && history.requests.length === 1) {
                    // First request - all tokens are new
                    deltaTokens = usage.promptTokens;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Total tokens in the context window sent to the model"><i class="fas fa-download"></i> ${formatNumber(usage.promptTokens)}</span>`;
            
            // Add delta if there was an increase
            if (deltaTokens > 0) {
                metricsHtml += `<span class="metric-item" style="color: var(--warning-color)" data-tooltip="New tokens added since previous request">+${formatNumber(deltaTokens)}</span>`;
            }
            
            // Add cache information if available (Anthropic)
            if (usage.cacheReadInputTokens > 0 || usage.cacheCreationInputTokens > 0) {
                if (usage.cacheReadInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--success-color)" data-tooltip="Tokens read from cache (90% discount)"><i class="fas fa-memory"></i> ${formatNumber(usage.cacheReadInputTokens)}</span>`;
                }
                if (usage.cacheCreationInputTokens > 0) {
                    metricsHtml += `<span class="metric-item" style="color: var(--info-color)" data-tooltip="Tokens written to cache (25% surcharge)"><i class="fas fa-save"></i> ${formatNumber(usage.cacheCreationInputTokens)}</span>`;
                }
            }
            
            metricsHtml += `
                <span class="metric-item" data-tooltip="Tokens generated by the assistant"><i class="fas fa-upload"></i> ${formatNumber(usage.completionTokens)}</span>
                <span class="metric-item" data-tooltip="Total tokens used (prompt + completion)"><i class="fas fa-chart-bar"></i> ${formatNumber(usage.totalTokens)}</span>
            `;
        }
        
        metricsFooter.innerHTML = metricsHtml;
        this.currentAssistantGroup.appendChild(metricsFooter);
        
        // Initialize Tippy tooltips for the metric items
        this.initializeTooltips(metricsFooter);
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
            
            // Create accounting node before discarding messages
            const messagesToDiscard = chat.messages.length - messageIndex;
            if (messagesToDiscard > 0) {
                const accountingNode = this.createAccountingNode('Message edited', messagesToDiscard);
                chat.messages.splice(messageIndex, 0, accountingNode);
                messageIndex++; // Adjust index after insertion
            }
            
            // Clip history at this point
            chat.messages = chat.messages.slice(0, messageIndex);
            chat.updatedAt = new Date().toISOString();
            
            // Save the clipped state
            this.saveSettings();
            
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
            this.addMessage('assistant', '');
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
                        this.saveSettings();
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

    scrollToBottom() {
        this.chatMessages.scrollTop = this.chatMessages.scrollHeight;
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
        this.scrollToBottom();
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
            this.scrollToBottom();
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
        
        // For system messages (title, summary requests), return current context
        const lastMessage = chat.messages[chat.messages.length - 1];
        if (lastMessage && ['system-title', 'system-summary', 'title'].includes(lastMessage.role)) {
            // Don't change context window for these messages
            const currentTokens = this.getTokenUsageForChat(chatId);
            return currentTokens.totalTokens;
        }
        
        // Check if there's a summary in the conversation (not just last message)
        const summaryIndex = chat.messages.findIndex(m => m.role === 'summary');
        if (summaryIndex !== -1) {
            // We have a summary - context should be based on messages after it
            const summaryMessage = chat.messages[summaryIndex];
            const messagesAfterSummary = chat.messages.slice(summaryIndex + 1);
            
            // Start with the summary's completion tokens
            let contextTokens = 0;
            if (summaryMessage.usage && summaryMessage.usage.completionTokens) {
                contextTokens = summaryMessage.usage.completionTokens;
            }
            
            // Add tokens from messages after the summary
            const history = this.tokenUsageHistory.get(chatId);
            if (history && history.requests.length > 0) {
                // If we have messages after summary, use the latest token count
                const hasMessagesAfterSummary = messagesAfterSummary.some(m => 
                    ['user', 'assistant'].includes(m.role) && m.usage
                );
                if (hasMessagesAfterSummary) {
                    const latestRequest = history.requests[history.requests.length - 1];
                    return latestRequest.promptTokens + 
                           (latestRequest.cacheReadInputTokens || 0) + 
                           (latestRequest.cacheCreationInputTokens || 0) +
                           (latestRequest.completionTokens || 0);
                }
            }
            
            return contextTokens;
        }
        
        // Otherwise, get the latest token usage
        const tokenUsage = this.getTokenUsageForChat(chatId);
        return tokenUsage.totalTokens;
    }
    
    updateTokenUsage(chatId, usage, model) {
        if (!this.tokenUsageHistory.has(chatId)) {
            this.tokenUsageHistory.set(chatId, {
                requests: [],
                model: model
            });
        }
        
        const history = this.tokenUsageHistory.get(chatId);
        
        // Add this request to history
        history.requests.push({
            timestamp: new Date().toISOString(),
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
        
        // Update cumulative token counters
        this.updateCumulativeTokenDisplay();
        
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
        
        // Update stats - show as "X tokens / Y tokens" or "Xk tokens / Yk tokens"
        if (totalTokens >= 1000 || limit >= 1000) {
            const totalDisplay = totalTokens >= 1000 ? `${(totalTokens / 1000).toFixed(1)}k` : totalTokens.toString();
            const limitDisplay = limit >= 1000 ? `${(limit / 1000).toFixed(0)}k` : limit.toString();
            stats.textContent = `${totalDisplay} tokens / ${limitDisplay} tokens`;
        } else {
            stats.textContent = `${totalTokens} tokens / ${limit} tokens`;
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
    
    // Get cumulative token usage for the entire chat
    getCumulativeTokenUsage(chatId) {
        const chat = this.chats.get(chatId);
        if (!chat) {
            return { 
                inputTokens: 0, 
                outputTokens: 0,
                cacheCreationTokens: 0,
                cacheReadTokens: 0
            };
        }
        
        // Start with tokens from accounting nodes
        let totalInput = 0;
        let totalOutput = 0;
        let totalCacheCreation = 0;
        let totalCacheRead = 0;
        
        // Find the last accounting node
        let lastAccountingIndex = -1;
        for (let i = chat.messages.length - 1; i >= 0; i--) {
            if (chat.messages[i].role === 'accounting') {
                const tokens = chat.messages[i].cumulativeTokens;
                totalInput = tokens.inputTokens || 0;
                totalOutput = tokens.outputTokens || 0;
                totalCacheCreation = tokens.cacheCreationTokens || 0;
                totalCacheRead = tokens.cacheReadTokens || 0;
                lastAccountingIndex = i;
                break;
            }
        }
        
        // Add tokens from requests after the last accounting node
        const history = this.tokenUsageHistory.get(chatId);
        if (history && history.requests.length > 0) {
            // Count how many requests happened after the accounting node
            // This is approximate - we count messages that have token usage
            let requestsAfterAccounting = 0;
            if (lastAccountingIndex >= 0) {
                for (let i = lastAccountingIndex + 1; i < chat.messages.length; i++) {
                    if (chat.messages[i].usage) {
                        requestsAfterAccounting++;
                    }
                }
            } else {
                requestsAfterAccounting = history.requests.length;
            }
            
            // Add the last N requests
            const startIndex = Math.max(0, history.requests.length - requestsAfterAccounting);
            for (let i = startIndex; i < history.requests.length; i++) {
                const request = history.requests[i];
                totalInput += request.promptTokens || 0;
                totalOutput += request.completionTokens || 0;
                totalCacheCreation += request.cacheCreationInputTokens || 0;
                totalCacheRead += request.cacheReadInputTokens || 0;
            }
        }
        
        return { 
            inputTokens: totalInput, 
            outputTokens: totalOutput,
            cacheCreationTokens: totalCacheCreation,
            cacheReadTokens: totalCacheRead
        };
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
        
        if (inputElement) {
            inputElement.innerHTML = `<i class="fas fa-download"></i> ${formatNumber(cumulative.inputTokens)}`;
        }
        
        if (outputElement) {
            outputElement.innerHTML = `<i class="fas fa-upload"></i> ${formatNumber(cumulative.outputTokens)}`;
        }
        
        // Show/hide and update cache token displays
        if (cacheReadElement) {
            if (cumulative.cacheReadTokens > 0) {
                cacheReadElement.style.display = 'inline-block';
                cacheReadElement.innerHTML = `<i class="fas fa-memory"></i> ${formatNumber(cumulative.cacheReadTokens)}`;
            } else {
                cacheReadElement.style.display = 'none';
            }
        }
        
        if (cacheCreationElement) {
            if (cumulative.cacheCreationTokens > 0) {
                cacheCreationElement.style.display = 'inline-block';
                cacheCreationElement.innerHTML = `<i class="fas fa-save"></i> ${formatNumber(cumulative.cacheCreationTokens)}`;
            } else {
                cacheCreationElement.style.display = 'none';
            }
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
    
    // Initialize Tippy tooltips
    initializeTooltips(container) {
        // Find all elements with data-tooltip attribute
        const elements = container.querySelectorAll('[data-tooltip]');
        
        elements.forEach(element => {
            // Get theme based on current theme
            const isDarkTheme = document.documentElement.getAttribute('data-theme') === 'dark';
            
            tippy(element, {
                content: element.getAttribute('data-tooltip'),
                theme: isDarkTheme ? 'light-border' : 'light',
                animation: 'shift-away',
                delay: 0, // No delay
                duration: [200, 150], // Animation duration in/out
                placement: 'top',
                arrow: true,
                inertia: true
            });
        });
    }
    
    // Copy conversation metrics to clipboard
    async copyConversationMetrics() {
        if (!this.currentChatId) {
            this.showError('No active chat to copy metrics from');
            return;
        }
        
        const chat = this.chats.get(this.currentChatId);
        if (!chat || !chat.messages || chat.messages.length === 0) {
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
        this.temperatureValue.textContent = temperature.toFixed(1);
        
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
            this.saveSettings();
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
    
    // Generate chat title using LLM
    async summarizeConversation() {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) {
            this.showError('No active chat to summarize');
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
            
            // Create summary request
            const summaryRequest = `Please provide a comprehensive summary of our conversation so far. Include:
1. The main topics discussed
2. Key decisions or conclusions reached
3. Any important context or background information
4. Current status or next steps if applicable

This summary will be used as context for continuing our conversation.`;
            
            // Process the summary request as a system-summary message
            this.processRenderEvent({ type: 'system-summary-message', content: summaryRequest });
            chat.messages.push({ 
                role: 'system-summary', 
                content: summaryRequest,
                timestamp: new Date().toISOString()
            });
            
            // Add metrics placeholder
            const metricsId = `metrics-summary-${this.metricsIdCounter++}`;
            this.processRenderEvent({ type: 'metrics-placeholder', id: metricsId });
            
            // Show loading spinner
            this.processRenderEvent({ type: 'show-spinner' });
            
            // Build messages for API with frozen cache position
            const { messages, cacheControlIndex } = this.buildMessagesForAPI(chat, provider, true);
            
            // Add the summary request
            messages.push({ role: 'user', content: summaryRequest });
            
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
            
            this.processRenderEvent({ 
                type: 'update-metrics', 
                id: metricsId,
                usage: response.usage, 
                responseTime: llmResponseTime,
                previousPromptTokens: this.getPreviousPromptTokens()
            });
            
            // Process the summary response
            if (response.content) {
                // Display the response as a summary checkpoint
                this.processRenderEvent({ type: 'summary-message', content: response.content });
                
                // Store the summary response with the 'summary' role
                chat.messages.push({ 
                    role: 'summary', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    timestamp: new Date().toISOString(),
                    cacheControlIndex: cacheControlIndex  // Store the frozen cache position
                });
                
                // Save the chat
                this.saveSettings();
                
                // Show success message
                const successDiv = document.createElement('div');
                successDiv.className = 'message system';
                successDiv.style.cssText = 'margin: 20px auto; padding: 8px 16px; background: var(--success-color); color: white; text-align: center; border-radius: 4px; max-width: 80%;';
                successDiv.innerHTML = '<i class="fas fa-check-circle"></i> Conversation summarized! Context window has been reset.';
                this.chatMessages.appendChild(successDiv);
                this.scrollToBottom();
                
                // Update context window display (should show near zero)
                const contextTokens = this.calculateContextWindowTokens(chat.id);
                this.updateContextWindowIndicator(contextTokens, chat.model || provider.model);
            }
            
        } catch (error) {
            console.error('Failed to summarize conversation:', error);
            this.showError(`Failed to summarize: ${error.message}`);
            
            // Remove the system-summary message if it failed
            const lastMsg = chat.messages[chat.messages.length - 1];
            if (lastMsg && lastMsg.role === 'system-summary') {
                chat.messages.pop();
                this.saveSettings();
            }
        } finally {
            // Re-enable button
            this.summarizeBtn.disabled = false;
            this.summarizeBtn.innerHTML = '<span><i class="fas fa-compress-alt"></i></span><span>Summarize Conversation</span>';
        }
    }
    
    async generateChatTitle(chat, mcpConnection, provider) {
        try {
            // Mark that we're generating a title
            chat.titleGenerated = true;
            this.saveSettings();
            
            // Create a system message for title generation
            const titleRequest = "Please provide a short, descriptive title (max 50 characters) for this conversation. Respond with ONLY the title text, no quotes, no explanation.";
            
            // Add visual separator
            const separatorDiv = document.createElement('div');
            separatorDiv.className = 'message system';
            separatorDiv.style.cssText = 'margin: 20px auto; padding: 8px 16px; background: var(--info-color); color: white; text-align: center; border-radius: 4px; max-width: 80%;';
            separatorDiv.innerHTML = '<i class="fas fa-pen"></i> Generating chat title...';
            this.chatMessages.appendChild(separatorDiv);
            this.scrollToBottom();
            
            // Process the title request as a system-title message
            this.processRenderEvent({ type: 'system-title-message', content: titleRequest });
            chat.messages.push({ 
                role: 'system-title', 
                content: titleRequest,
                timestamp: new Date().toISOString()
            });
            
            // Add metrics placeholder
            const metricsId = `metrics-title-${this.metricsIdCounter++}`;
            this.processRenderEvent({ type: 'metrics-placeholder', id: metricsId });
            const domMetricsId = this.metricsIdMapping?.[metricsId] || metricsId;
            
            // Show loading spinner
            this.processRenderEvent({ type: 'show-spinner' });
            
            // Build messages for title generation
            const messages = [];
            
            // Add system prompt for title generation
            messages.push({ 
                role: 'system', 
                content: 'You are a helpful assistant that generates concise, descriptive titles for conversations. Respond with ONLY the title text, no quotes, no explanation, no markdown.' 
            });
            
            // Add the conversation context (excluding system messages)
            for (const msg of chat.messages) {
                // Skip system roles
                if (['system-title', 'system-summary', 'title', 'summary'].includes(msg.role)) continue;
                
                const msgRole = msg.role || msg.type;
                if (msgRole === 'user') {
                    messages.push({ role: 'user', content: msg.content });
                } else if (msgRole === 'assistant' && msg.content) {
                    // Clean content and truncate if needed
                    const cleanedContent = this.cleanContentForAPI(msg.content);
                    const truncated = cleanedContent.length > 500 ? cleanedContent.substring(0, 500) + '...' : cleanedContent;
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
            
            this.processRenderEvent({ 
                type: 'update-metrics', 
                id: metricsId,
                usage: response.usage, 
                responseTime: llmResponseTime,
                previousPromptTokens: this.getPreviousPromptTokens()
            });
            
            // Process the title response
            if (response.content) {
                // Display the response as a title message
                this.processRenderEvent({ type: 'title-message', content: response.content });
                
                // Extract and clean the title
                const newTitle = response.content.trim()
                    .replace(/^["']|["']$/g, '') // Remove quotes
                    .replace(/^Title:\s*/i, '') // Remove "Title:" prefix if present
                    .substring(0, 65); // Allow some tolerance beyond 50 chars
                
                console.log('Generated title:', newTitle);
                console.log('Current chat ID:', this.currentChatId);
                
                // Update the chat title
                if (newTitle && newTitle.length > 0) {
                    chat.title = newTitle;
                    chat.updatedAt = new Date().toISOString();
                    this.saveSettings(); // Save the updated title
                    
                    // Store the title response with the 'title' role
                    chat.messages.push({ 
                        role: 'title', 
                        content: response.content,
                        usage: response.usage || null,
                        responseTime: llmResponseTime || null,
                        timestamp: new Date().toISOString()
                    });
                    
                    // Update UI
                    this.updateChatSessions();
                    document.getElementById('chatTitle').textContent = newTitle;
                    
                    // Also update the sidebar item directly
                    const sessionItem = document.querySelector(`[data-chat-id="${this.currentChatId}"] .session-title`);
                    if (sessionItem) {
                        sessionItem.textContent = newTitle;
                    }
                    
                    // Add success message
                    const successDiv = document.createElement('div');
                    successDiv.className = 'message system';
                    successDiv.style.cssText = 'margin: 10px auto 20px; padding: 8px 16px; background: var(--success-color); color: white; text-align: center; border-radius: 4px; max-width: 80%;';
                    successDiv.innerHTML = `<i class="fas fa-check-circle"></i> Chat title updated: "${newTitle}"`;
                    this.chatMessages.appendChild(successDiv);
                    this.scrollToBottom();
                }
            }
            
        } catch (error) {
            console.error('Failed to generate title:', error);
            // Don't show error to user, just log it
        } finally {
            // Hide spinner
            this.processRenderEvent({ type: 'hide-spinner' });
            // Clear assistant group
            this.currentAssistantGroup = null;
            // Save any changes
            this.saveSettings();
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
        this.saveSettings();
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
});
