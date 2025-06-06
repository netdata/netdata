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
        this.pendingAssistantMetrics = null; // Store metrics to add at end of assistant message
        
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
        this.defaultSystemPrompt = `You are a helpful assistant with access to Netdata monitoring data through MCP (Model Context Protocol) tools. 
You can query metrics, check alerts, analyze system performance, and help users understand their infrastructure health.
When users ask about their systems, use the available MCP tools to fetch real data and provide insights.`;
        
        // Load last used system prompt from localStorage or use default
        this.lastSystemPrompt = localStorage.getItem('lastSystemPrompt') || this.defaultSystemPrompt;
        
        this.initializeUI();
        this.initializeResizable();
        this.loadSettings();
    }
    
    // Get available models for a provider type
    getModelsForProviderType(providerType) {
        // Use the single source of truth for models
        return this.models[providerType] || [];
    }

    initializeUI() {
        // Chat sidebar
        this.newChatBtn = document.getElementById('newChatBtn');
        this.newChatBtn.addEventListener('click', () => this.showNewChatModal());
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
        
        this.sendMessageBtn.addEventListener('click', () => this.sendMessage());
        this.reconnectMcpBtn.addEventListener('click', () => this.reconnectCurrentMcp());
        this.chatInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendMessage();
            }
        });
        
        // Log panel
        this.logPanel = document.getElementById('logPanel');
        this.toggleLogBtn = document.getElementById('toggleLogBtn');
        this.clearLogBtn = document.getElementById('clearLogBtn');
        this.downloadLogBtn = document.getElementById('downloadLogBtn');
        this.logContent = document.getElementById('logContent');
        
        this.toggleLogBtn.addEventListener('click', () => this.toggleLog());
        this.clearLogBtn.addEventListener('click', () => this.clearLog());
        this.downloadLogBtn.addEventListener('click', () => this.downloadLog());
        
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
        this.setupTabs();
        
        // Settings lists
        this.mcpServersList = document.getElementById('mcpServersList');
        this.llmProvidersList = document.getElementById('llmProvidersList');
        this.addMcpServerBtn = document.getElementById('addMcpServerBtn');
        this.addLlmProviderBtn = document.getElementById('addLlmProviderBtn');
        
        this.addMcpServerBtn.addEventListener('click', () => this.showModal('addMcpModal'));
        this.addLlmProviderBtn.addEventListener('click', () => this.showModal('addLlmModal'));
        
        // New chat modal
        this.setupModal('newChatModal', 'newChatBackdrop', 'closeNewChatBtn');
        this.newChatMcpServer = document.getElementById('newChatMcpServer');
        this.newChatLlmProvider = document.getElementById('newChatLlmProvider');
        this.newChatModelGroup = document.getElementById('newChatModelGroup');
        this.newChatModel = document.getElementById('newChatModel');
        this.newChatTitle = document.getElementById('newChatTitle');
        this.createChatBtn = document.getElementById('createChatBtn');
        this.cancelNewChatBtn = document.getElementById('cancelNewChatBtn');
        
        this.newChatLlmProvider.addEventListener('change', () => this.updateNewChatModels());
        this.createChatBtn.addEventListener('click', () => this.createNewChat());
        this.cancelNewChatBtn.addEventListener('click', () => this.hideModal('newChatModal'));
        
        // Add MCP server modal
        this.setupModal('addMcpModal', 'addMcpBackdrop', 'closeAddMcpBtn');
        this.mcpServerUrl = document.getElementById('mcpServerUrl');
        this.mcpServerName = document.getElementById('mcpServerName');
        this.saveMcpServerBtn = document.getElementById('saveMcpServerBtn');
        this.cancelAddMcpBtn = document.getElementById('cancelAddMcpBtn');
        
        this.saveMcpServerBtn.addEventListener('click', () => this.addMcpServer());
        this.cancelAddMcpBtn.addEventListener('click', () => this.hideModal('addMcpModal'));
        
        // Add LLM provider modal
        this.setupModal('addLlmModal', 'addLlmBackdrop', 'closeAddLlmBtn');
        this.llmProxyUrl = document.getElementById('llmProxyUrl');
        this.llmProviderName = document.getElementById('llmProviderName');
        this.llmProvidersStatus = document.getElementById('llmProvidersStatus');
        this.llmProvidersInfo = document.getElementById('llmProvidersInfo');
        this.saveLlmProviderBtn = document.getElementById('saveLlmProviderBtn');
        this.cancelAddLlmBtn = document.getElementById('cancelAddLlmBtn');
        
        this.llmProxyUrl.addEventListener('blur', () => this.testProxyConnection());
        this.saveLlmProviderBtn.addEventListener('click', () => this.addLlmProvider());
        this.cancelAddLlmBtn.addEventListener('click', () => this.hideModal('addLlmModal'));
        
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
    }

    setupModal(modalId, backdropId, closeId) {
        const modal = document.getElementById(modalId);
        const backdrop = document.getElementById(backdropId);
        const closeBtn = document.getElementById(closeId);
        
        backdrop.addEventListener('click', () => this.hideModal(modalId));
        closeBtn.addEventListener('click', () => this.hideModal(modalId));
    }

    setupTabs() {
        const tabBtns = document.querySelectorAll('.tab-btn');
        tabBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                const tabName = btn.getAttribute('data-tab');
                
                // Update active button
                tabBtns.forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                
                // Update active content
                document.querySelectorAll('.tab-content').forEach(content => {
                    content.classList.remove('active');
                });
                document.getElementById(`${tabName}-tab`).classList.add('active');
            });
        });
    }

    showModal(modalId) {
        document.getElementById(modalId).classList.add('show');
    }

    hideModal(modalId) {
        document.getElementById(modalId).classList.remove('show');
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
    }

    initializeResizable() {
        // Chat sidebar resize
        const chatSidebar = document.getElementById('chatSidebar');
        const chatSidebarResize = document.getElementById('chatSidebarResize');
        
        this.setupResize(chatSidebarResize, 'horizontal', (delta) => {
            const currentWidth = chatSidebar.offsetWidth;
            const newWidth = Math.max(200, Math.min(400, currentWidth + delta));
            chatSidebar.style.width = newWidth + 'px';
            this.savePaneSizes();
        });

        // Log panel resize
        const logPanel = document.getElementById('logPanel');
        const logPanelResize = document.getElementById('logPanelResize');
        
        this.setupResize(logPanelResize, 'horizontal', (delta) => {
            // First, ensure the panel is not collapsed
            if (logPanel.classList.contains('collapsed')) {
                // Expand it first
                logPanel.classList.remove('collapsed');
                this.toggleLogBtn.textContent = '◀';
                localStorage.setItem('logCollapsed', 'false');
                // Set initial width when expanding
                logPanel.style.width = '300px';
            }
            
            const currentWidth = logPanel.offsetWidth;
            // For right panel, dragging left (negative delta) should increase width
            const newWidth = Math.max(200, Math.min(650, currentWidth + (-delta)));
            logPanel.style.width = newWidth + 'px';
            console.log('Log panel resize:', { currentWidth, delta, newWidth, offsetWidth: logPanel.offsetWidth });
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
                
                if (sizes.chatSidebar) {
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
        this.logPanel.classList.toggle('collapsed');
        this.toggleLogBtn.textContent = this.logPanel.classList.contains('collapsed') ? '▶' : '◀';
        localStorage.setItem('logCollapsed', this.logPanel.classList.contains('collapsed'));
        this.savePaneSizes();
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
            messageDiv.textContent = `❌ ${message}`;
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
                <button class="btn-copy-log" title="Copy to clipboard" data-entry-id="${entryId}">📋</button>
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
            button.textContent = '✓';
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
            this.toggleLogBtn.textContent = '▶';
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
                    this.chats.set(chat.id, chat);
                });
                this.updateChatSessions();
                
                // Load last active chat
                const lastChatId = localStorage.getItem('currentChatId');
                if (lastChatId && this.chats.has(lastChatId)) {
                    this.loadChat(lastChatId);
                }
            } catch (e) {
                console.error('Failed to load chats:', e);
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
        
        // Save chats
        const chatsToSave = Array.from(this.chats.values());
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
            let html = '<div style="color: var(--color-success);">✓ Connected successfully</div>';
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

    async addLlmProvider() {
        const proxyUrl = this.llmProxyUrl.value.trim();
        const name = this.llmProviderName.value.trim();
        
        if (!proxyUrl || !name) {
            this.showError('Please fill in all fields');
            return;
        }
        
        // Make sure we have tested the connection and have available providers
        if (!this.availableProviders) {
            await this.testProxyConnection();
            if (!this.availableProviders) {
                this.showError('Please test the proxy connection first');
                return;
            }
        }
        
        try {
            const providerId = `llm_${Date.now()}`;
            
            // Create a proxy provider object that holds all available providers
            const provider = {
                id: providerId,
                name: name,
                proxyUrl: proxyUrl,
                availableProviders: this.availableProviders,
                onLog: (logEntry) => this.addLogEntry(name, logEntry)
            };
            
            this.llmProviders.set(providerId, provider);
            this.saveSettings();
            this.updateLlmProvidersList();
            this.updateNewChatSelectors();
            
            // Clear form
            this.llmProxyUrl.value = 'http://localhost:8081';
            this.llmProviderName.value = '';
            this.llmProvidersStatus.style.display = 'none';
            this.availableProviders = null;
            this.hideModal('addLlmModal');
            
            this.addLogEntry('SYSTEM', {
                timestamp: new Date().toISOString(),
                direction: 'info',
                message: `LLM proxy "${name}" added successfully`
            });
            
        } catch (error) {
            this.showError(`Failed to add LLM proxy: ${error.message}`);
        }
    }

    updateLlmProvidersList() {
        this.llmProvidersList.innerHTML = '';
        
        if (this.llmProviders.size === 0) {
            this.llmProvidersList.innerHTML = '<div class="text-center text-muted">No LLM proxies configured</div>';
            return;
        }
        
        for (const [id, provider] of this.llmProviders) {
            const providerCount = Object.keys(provider.availableProviders || {}).length;
            const modelCount = Object.values(provider.availableProviders || {})
                .reduce((sum, p) => sum + (p.models || []).length, 0);
            
            const providerDiv = document.createElement('div');
            providerDiv.className = 'config-item';
            providerDiv.innerHTML = `
                <div class="config-item-info">
                    <div class="config-item-name">🔗 ${provider.name}</div>
                    <div class="config-item-details">${provider.proxyUrl} - ${providerCount} providers, ${modelCount} models</div>
                </div>
                <div class="config-item-actions">
                    <button class="btn btn-small btn-danger" onclick="app.removeLlmProvider('${id}')">Remove</button>
                </div>
            `;
            this.llmProvidersList.appendChild(providerDiv);
        }
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
            case 'openai': return '🤖';
            case 'anthropic': return '🧠';
            case 'google': return '🔮';
            default: return '💬';
        }
    }

    // Chat Management
    showNewChatModal() {
        this.updateNewChatSelectors();
        this.showModal('newChatModal');
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
            
            config.models.forEach(modelName => {
                const option = document.createElement('option');
                // Store both provider type and model in the value
                option.value = `${providerType}:${modelName}`;
                option.textContent = modelName;
                optgroup.appendChild(option);
            });
            
            this.newChatModel.appendChild(optgroup);
        });
    }

    async createNewChat() {
        const mcpServerId = this.newChatMcpServer.value;
        const llmProviderId = this.newChatLlmProvider.value;
        const selectedModel = this.newChatModel.value;
        let title = this.newChatTitle.value.trim();
        
        if (!mcpServerId || !llmProviderId) {
            this.showError('Please select both MCP server and LLM provider');
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
            updatedAt: new Date().toISOString()
        };
        
        this.chats.set(chatId, chat);
        this.currentChatId = chatId;
        
        // Initialize token usage history for new chat
        this.tokenUsageHistory.set(chatId, {
            requests: [],
            model: selectedModel
        });
        
        this.saveSettings();
        this.updateChatSessions();
        this.loadChat(chatId);
        
        // Clear form and close modal
        this.newChatTitle.value = '';
        this.newChatModel.value = '';
        this.newChatModelGroup.style.display = 'none';
        this.hideModal('newChatModal');
    }

    updateChatSessions() {
        this.chatSessions.innerHTML = '';
        
        if (this.chats.size === 0) {
            this.chatSessions.innerHTML = '<div class="text-center text-muted mt-2">No chats yet</div>';
            return;
        }
        
        const sortedChats = Array.from(this.chats.values()).sort((a, b) => 
            new Date(b.updatedAt) - new Date(a.updatedAt)
        );
        
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
            
            // Calculate context usage percentage if available
            let contextInfo = '';
            if (chat.contextUsage && chat.lastModel) {
                // Extract model name from format "provider:model-name" if needed
                let modelName = chat.lastModel;
                if (modelName && modelName.includes(':')) {
                    modelName = modelName.split(':')[1];
                }
                const limit = this.modelLimits[modelName] || 4096;
                const percentage = Math.round((chat.contextUsage / limit) * 100);
                const contextK = (chat.contextUsage / 1000).toFixed(1);
                contextInfo = ` • ${contextK}k/${(limit/1000).toFixed(0)}k`;
            }
            
            sessionDiv.innerHTML = `
                <div class="session-content" onclick="app.loadChat('${chat.id}')">
                    <div class="session-title">${chat.title}</div>
                    <div class="session-meta">
                        <span>${modelDisplay}${contextInfo}</span>
                        <span>${new Date(chat.updatedAt).toLocaleDateString()}</span>
                    </div>
                </div>
                <button class="btn-delete-chat" onclick="event.stopPropagation(); app.deleteChat('${chat.id}')" title="Delete chat">
                    🗑️
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
        
        // Display system prompt as first message
        this.displaySystemPrompt(chat.systemPrompt || this.defaultSystemPrompt);
        
        for (const msg of chat.messages) {
            if (msg.role === 'system') continue;
            this.displayStoredMessage(msg);
        }
        // Clear current group after loading
        this.currentAssistantGroup = null;
        
        // Update context window indicator
        const tokenHistory = this.getTokenUsageForChat(chatId);
        const model = chat.model || (provider ? provider.model : null);
        
        // Also check if we have saved context usage in the chat
        if (chat.contextUsage && chat.contextUsage > 0) {
            // Use the actual model from the chat, not lastModel which might be in wrong format
            this.updateContextWindowIndicator(chat.contextUsage, model || chat.model || 'unknown');
        } else if (tokenHistory.totalTokens > 0 && model) {
            this.updateContextWindowIndicator(tokenHistory.totalTokens, model);
        } else {
            // Show empty context window with the correct model
            this.updateContextWindowIndicator(0, model || chat.model || 'unknown');
        }
        
        this.scrollToBottom();
    }

    displayStoredMessage(msg) {
        switch(msg.type) {
            case 'user':
                // User messages reset the assistant group
                this.currentAssistantGroup = null;
                this.addMessage('user', msg.content);
                break;
                
            case 'assistant':
                // Create new assistant group for this message
                this.currentAssistantGroup = null;
                // Display assistant message with saved statistics
                if (msg.content) {
                    this.addMessage('assistant', msg.content, msg.usage, msg.responseTime);
                }
                // Display any tool calls in the same group
                if (msg.toolCalls && msg.toolCalls.length > 0) {
                    // Ensure we have a group even if there was no content
                    if (!this.currentAssistantGroup) {
                        this.addMessage('assistant', '', msg.usage, msg.responseTime);
                    }
                    for (const toolCall of msg.toolCalls) {
                        this.addToolCall(toolCall.name, toolCall.arguments);
                    }
                }
                break;
                
            case 'tool-results':
                // Display all tool results in the current group
                for (const result of msg.results) {
                    this.addToolResult(result.name, result.result);
                }
                break;
                
            // Handle old format for backward compatibility
            case 'tool-call':
                this.addToolCall(msg.toolName, msg.args);
                break;
            case 'tool-result':
                this.addToolResult(msg.toolName, msg.result, 0, null);
                break;
                
            case 'system':
                this.addSystemMessage(msg.content);
                break;
                
            case 'error':
                // Just display the error in chat, don't trigger full error handling
                const messageDiv = document.createElement('div');
                messageDiv.className = 'message error';
                messageDiv.textContent = `❌ [Previous session error] ${msg.content}`;
                this.chatMessages.appendChild(messageDiv);
                break;
        }
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
                
                // Show empty context window indicator
                const indicator = document.getElementById('contextWindowIndicator');
                if (indicator) {
                    indicator.style.display = 'flex';
                    this.updateContextWindowIndicator(0, 'unknown');
                }
            }
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
        
        // Disable input
        this.chatInput.value = '';
        this.chatInput.disabled = true;
        this.sendMessageBtn.disabled = true;
        
        // Add user message
        this.addMessage('user', message);
        chat.messages.push({ type: 'user', role: 'user', content: message });
        
        // Show loading spinner
        this.showLoadingSpinner();
        
        try {
            await this.processMessageWithTools(chat, mcpConnection, provider, message);
        } catch (error) {
            this.showError(`Error: ${error.message}`);
            chat.messages.push({ type: 'error', content: error.message });
        } finally {
            // Remove loading spinner
            this.hideLoadingSpinner();
            
            // Finalize assistant group with metrics at bottom
            this.finalizeAssistantGroup();
            
            // Clear current assistant group after processing is complete
            this.currentAssistantGroup = null;
            
            chat.updatedAt = new Date().toISOString();
            this.saveSettings();
            this.chatInput.disabled = false;
            this.sendMessageBtn.disabled = false;
            this.chatInput.focus();
        }
    }

    async processMessageWithTools(chat, mcpConnection, provider, userMessage) {
        // Build conversation history
        const messages = [];
        
        // Add system prompt if first message
        if (chat.messages.filter(m => m.role === 'user').length === 1) {
            messages.push({ role: 'system', content: chat.systemPrompt || this.defaultSystemPrompt });
        }
        
        // Add conversation history with our simplified structure
        for (let i = 0; i < chat.messages.length; i++) {
            const msg = chat.messages[i];
            
            if (msg.type === 'user') {
                // Simple user message
                messages.push({ role: 'user', content: msg.content });
                
            } else if (msg.type === 'assistant') {
                // Assistant message with potential tool calls
                const cleanedContent = msg.content ? this.cleanContentForAPI(msg.content) : '';
                
                if (msg.toolCalls && msg.toolCalls.length > 0) {
                    // Assistant with tool calls
                    if (provider.type === 'anthropic') {
                        // Anthropic format with content blocks
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
                        // OpenAI format
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
                        // Google format (will be handled by their convertMessages)
                        messages.push({
                            role: 'assistant',
                            content: cleanedContent,
                            toolCalls: msg.toolCalls
                        });
                    }
                } else if (cleanedContent) {
                    // Assistant without tool calls
                    messages.push({ role: 'assistant', content: cleanedContent });
                }
                
            } else if (msg.type === 'tool-results') {
                // Tool results
                if (provider.type === 'anthropic') {
                    // Anthropic wants tool results in a user message
                    messages.push({
                        role: 'user',
                        content: msg.results.map(tr => ({
                            type: 'tool_result',
                            tool_use_id: tr.toolCallId,
                            content: typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result)
                        }))
                    });
                } else {
                    // OpenAI and others want individual tool messages
                    for (const tr of msg.results) {
                        messages.push(provider.formatToolResponse(
                            tr.toolCallId,
                            tr.result,
                            tr.name
                        ));
                    }
                }
            }
            // Note: We skip old format messages (tool-call, tool-result) as they should not exist in new chats
        }
        
        // Get available tools
        const tools = Array.from(mcpConnection.tools.values());
        
        let attempts = 0;
        const maxAttempts = 10;
        
        // Create assistant group at the start of processing
        // We'll add all content from this conversation turn to this single group
        this.currentAssistantGroup = null;
        
        while (attempts < maxAttempts) {
            attempts++;
            
            // Send to LLM with current temperature
            const temperature = this.getCurrentTemperature();
            const llmStartTime = Date.now();
            const response = await provider.sendMessage(messages, tools, temperature);
            const llmResponseTime = Date.now() - llmStartTime;
            
            // Track token usage
            if (response.usage) {
                this.updateTokenUsage(chat.id, response.usage, chat.model || provider.model);
            }
            
            // If no tool calls, display response and finish
            if (!response.toolCalls || response.toolCalls.length === 0) {
                if (response.content) {
                    // Create assistant group on first response with content
                    if (!this.currentAssistantGroup) {
                        this.addMessage('assistant', response.content, response.usage, llmResponseTime);
                    } else {
                        // Add content to existing group
                        this.addContentToAssistantGroup(response.content);
                    }
                    chat.messages.push({ 
                        type: 'assistant', 
                        role: 'assistant', 
                        content: response.content,
                        usage: response.usage || null,
                        responseTime: llmResponseTime || null
                    });
                    // Clean content before sending back to API
                    const cleanedContent = this.cleanContentForAPI(response.content);
                    if (cleanedContent && cleanedContent.trim()) {
                        messages.push({ role: 'assistant', content: cleanedContent });
                    }
                }
                break;
            }
            
            // Store assistant message with tool calls if any
            if (response.content || response.toolCalls) {
                // Display the message
                if (response.content) {
                    // Create assistant group on first response with content
                    if (!this.currentAssistantGroup) {
                        this.addMessage('assistant', response.content, response.usage, llmResponseTime);
                    } else {
                        // Add content to existing group
                        this.addContentToAssistantGroup(response.content);
                    }
                }
                
                // Store in our improved internal format
                const assistantMessage = {
                    type: 'assistant',
                    role: 'assistant',
                    content: response.content || '',
                    toolCalls: response.toolCalls || [],
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null
                };
                chat.messages.push(assistantMessage);
                
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
                    this.addMessage('assistant', '', response.usage, llmResponseTime);
                }
                
                const toolResults = [];
                
                for (const toolCall of response.toolCalls) {
                    try {
                        // Show tool call in UI
                        this.addToolCall(toolCall.name, toolCall.arguments);
                        
                        // Execute tool and track timing
                        const toolStartTime = Date.now();
                        const rawResult = await mcpConnection.callTool(toolCall.name, toolCall.arguments);
                        const toolResponseTime = Date.now() - toolStartTime;
                        
                        // Parse the result to handle MCP's response format
                        const result = this.parseToolResult(rawResult);
                        
                        // Calculate response size
                        const responseSize = typeof result === 'string' 
                            ? result.length 
                            : JSON.stringify(result).length;
                        
                        // Show result in UI with timing and size
                        this.addToolResult(toolCall.name, result, toolResponseTime, responseSize);
                        
                        // Collect result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result: result
                        });
                        
                    } catch (error) {
                        const errorMsg = `Tool error (${toolCall.name}): ${error.message}`;
                        this.addToolResult(toolCall.name, { error: errorMsg }, 0, errorMsg.length);
                        
                        // Collect error result
                        toolResults.push({
                            toolCallId: toolCall.id,
                            name: toolCall.name,
                            result: { error: errorMsg }
                        });
                    }
                }
                
                // Store all tool results together
                if (toolResults.length > 0) {
                    chat.messages.push({
                        type: 'tool-results',
                        results: toolResults
                    });
                    
                    // Add to conversation based on provider
                    if (provider.type === 'anthropic') {
                        // Anthropic wants tool results in a user message
                        messages.push({
                            role: 'user',
                            content: toolResults.map(tr => ({
                                type: 'tool_result',
                                tool_use_id: tr.toolCallId,
                                content: typeof tr.result === 'string' ? tr.result : JSON.stringify(tr.result)
                            }))
                        });
                    } else {
                        // OpenAI and others want individual tool messages
                        for (const tr of toolResults) {
                            messages.push(provider.formatToolResponse(
                                tr.toolCallId,
                                tr.result,
                                tr.name
                            ));
                        }
                    }
                }
            }
        }
        
        if (attempts >= maxAttempts) {
            this.showError('Maximum tool call attempts reached');
        }
    }

    addMessage(role, content, usage = null, responseTime = null) {
        let messageDiv;
        
        if (role === 'assistant') {
            // For assistant messages, we create or use the current group
            if (!this.currentAssistantGroup) {
                // Create new assistant group
                const groupDiv = document.createElement('div');
                groupDiv.className = 'assistant-group';
                
                // Store metrics to add later at the bottom
                if (usage || responseTime) {
                    this.pendingAssistantMetrics = { usage, responseTime };
                }
                
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
                    thinkingContent.textContent = part.content;
                    
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
        } else {
            // Regular message without thinking tags
            const contentDiv = document.createElement('div');
            contentDiv.className = 'message-content';
            
            if (role === 'assistant') {
                // Use marked to render markdown for assistant messages
                contentDiv.innerHTML = marked.parse(content);
            } else {
                // Keep user messages as plain text
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
        editBalloon.style.display = 'none';
        wrapper.appendChild(editBalloon);
        
        // Show balloon on hover
        contentDiv.addEventListener('mouseenter', () => {
            if (!contentDiv.classList.contains('editing')) {
                editBalloon.style.display = 'block';
            }
        });
        
        wrapper.addEventListener('mouseleave', () => {
            editBalloon.style.display = 'none';
        });
        
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
            <button class="btn btn-small btn-primary" title="Save & Restart Chat (Enter)">✓</button>
            <button class="btn btn-small btn-secondary" title="Cancel (Escape)">✗</button>
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
    
    // Finalize assistant group by adding metrics at the bottom
    finalizeAssistantGroup() {
        if (!this.currentAssistantGroup || !this.pendingAssistantMetrics) return;
        
        const { usage, responseTime } = this.pendingAssistantMetrics;
        
        const metricsFooter = document.createElement('div');
        metricsFooter.className = 'assistant-metrics-footer';
        
        let metricsHtml = '';
        
        // Add response time
        if (responseTime !== null) {
            const timeSeconds = (responseTime / 1000).toFixed(1);
            metricsHtml += `<span class="metric-item">⏱️ ${timeSeconds}s</span>`;
        }
        
        // Add token usage
        if (usage) {
            const formatNumber = (num) => num.toLocaleString();
            metricsHtml += `
                <span class="metric-item">📥 ${formatNumber(usage.promptTokens)}</span>
                <span class="metric-item">📤 ${formatNumber(usage.completionTokens)}</span>
                <span class="metric-item">📊 ${formatNumber(usage.totalTokens)}</span>
            `;
        }
        
        metricsFooter.innerHTML = metricsHtml;
        this.currentAssistantGroup.appendChild(metricsFooter);
        
        // Clear pending metrics
        this.pendingAssistantMetrics = null;
    }
    
    editUserMessage(contentDiv, originalContent) {
        const chat = this.chats.get(this.currentChatId);
        if (!chat) return;
        
        // Prevent multiple edit sessions
        if (contentDiv.classList.contains('editing')) return;
        
        // Find the message index
        let messageIndex = -1;
        for (let i = 0; i < chat.messages.length; i++) {
            if (chat.messages[i].role === 'user' && chat.messages[i].content === originalContent) {
                messageIndex = i;
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
            <button class="btn btn-small btn-primary" title="Save & Resend (Enter)">✓</button>
            <button class="btn btn-small btn-secondary" title="Cancel (Escape)">✗</button>
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
            
            if (newContent === originalText) {
                // No change, just cancel
                cancel();
                return;
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
        const cancel = () => {
            contentDiv.contentEditable = false;
            contentDiv.classList.remove('editing');
            contentDiv.textContent = originalText;
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
                thinkingContent.textContent = match[1].trim();
                
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

    addToolCall(toolName, args) {
        // If we have a current assistant group, append to it
        const targetContainer = this.currentAssistantGroup || this.chatMessages;
        
        // Create tool container with unique ID
        const toolId = `tool-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
        
        const toolDiv = document.createElement('div');
        toolDiv.className = 'tool-block';
        toolDiv.dataset.toolId = toolId;
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
            <span class="tool-toggle">▶</span>
            <span class="tool-label">🔧 ${toolName}</span>
            <span class="tool-info">
                <span class="tool-status">⏳ Calling...</span>
            </span>
        `;
        
        const toolContent = document.createElement('div');
        toolContent.className = 'tool-content collapsed';
        
        // Add request section
        const requestSection = document.createElement('div');
        requestSection.className = 'tool-request-section';
        requestSection.innerHTML = `
            <div class="tool-section-header">📤 Request</div>
            <pre>${JSON.stringify(args, null, 2)}</pre>
        `;
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
        
        toolHeader.addEventListener('click', () => {
            const isCollapsed = toolContent.classList.contains('collapsed');
            toolContent.classList.toggle('collapsed');
            toolHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
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

    addToolResult(toolName, result, responseTime = 0, responseSize = null) {
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
                const timeInfo = responseTime > 0 ? `${(responseTime / 1000).toFixed(2)}s` : '';
                
                // Update status
                if (statusSpan) {
                    statusSpan.textContent = result.error ? '❌ Error' : '✅ Complete';
                }
                
                // Add metrics
                if (infoSpan) {
                    infoSpan.innerHTML = `
                        <span class="tool-status">${result.error ? '❌ Error' : '✅ Complete'}</span>
                        <span class="tool-metric">⏱️ ${timeInfo}</span>
                        <span class="tool-metric">📦 ${sizeInfo}</span>
                    `;
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
                        <div class="tool-section-header">📥 Response</div>
                        ${formattedResult}
                    `;
                    responseSection.style.display = 'block';
                    
                    if (separator) {
                        separator.style.display = 'block';
                    }
                }
                
                // Remove from pending
                this.pendingToolCalls.delete(toolName);
            } else {
                // Fallback: create new block if not found
                this.createStandaloneToolResult(toolName, result, responseTime, responseSize);
            }
        } else {
            // No pending call found, create standalone result
            this.createStandaloneToolResult(toolName, result, responseTime, responseSize);
        }
        
        this.scrollToBottom();
        this.moveSpinnerToBottom();
    }
    
    createStandaloneToolResult(toolName, result, responseTime = 0, responseSize = null) {
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
        
        const toolHeader = document.createElement('div');
        toolHeader.className = 'tool-header';
        toolHeader.innerHTML = `
            <span class="tool-toggle">▶</span>
            <span class="tool-label">📊 Tool result: ${toolName}</span>
            <span class="tool-info">
                <span class="tool-metric">⏱️ ${timeInfo}</span>
                <span class="tool-metric">📦 ${sizeInfo}</span>
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
        
        toolHeader.addEventListener('click', () => {
            const isCollapsed = toolContent.classList.contains('collapsed');
            toolContent.classList.toggle('collapsed');
            toolHeader.querySelector('.tool-toggle').textContent = isCollapsed ? '▼' : '▶';
        });
        
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

    showLoadingSpinner() {
        // Remove any existing spinner first
        this.hideLoadingSpinner();
        
        // Create spinner element
        const spinnerDiv = document.createElement('div');
        spinnerDiv.id = 'llm-loading-spinner';
        spinnerDiv.className = 'message assistant loading-spinner';
        spinnerDiv.innerHTML = `
            <div class="spinner-container">
                <div class="spinner"></div>
                <span class="spinner-text">Thinking...</span>
            </div>
        `;
        this.chatMessages.appendChild(spinnerDiv);
        this.scrollToBottom();
    }

    hideLoadingSpinner() {
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
            totalTokens: usage.totalTokens
        });
        
        // The prompt tokens of the latest request include the entire conversation
        // So we use the latest prompt tokens as the true total
        const latestTotalTokens = usage.promptTokens;
        
        
        // Update context window indicator with the actual conversation size
        this.updateContextWindowIndicator(latestTotalTokens, model);
        
        // Save context usage in the chat
        const chat = this.chats.get(chatId);
        if (chat) {
            chat.contextUsage = latestTotalTokens;
            chat.lastModel = model;
            this.saveSettings();
        }
        
        // Update any pending conversation total displays
        const pendingTotals = document.querySelectorAll('[id^="conv-total-"]');
        pendingTotals.forEach(el => {
            if (el.textContent === 'Calculating...' || el.textContent.match(/^\d/)) {
                el.textContent = latestTotalTokens.toLocaleString();
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
        
        // Get the latest request's prompt tokens as the total
        const latestRequest = history.requests[history.requests.length - 1];
        return { totalTokens: latestRequest.promptTokens };
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
}

// Initialize the application
document.addEventListener('DOMContentLoaded', () => {
    window.app = new NetdataMCPChat();
});
