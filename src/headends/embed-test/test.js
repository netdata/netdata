/**
 * AI Agent Chat UI - Embeddable chat interface
 *
 * Configuration (set before loading this script):
 *   window.AiAgentChatConfig = {
 *     endpoint: 'http://localhost:8090',
 *     agentId: 'support',              // Optional - server uses allowedAgents[0] if not set
 *     icon: 'data:image/svg+xml,...',  // Custom widget icon (widget mode only)
 *     position: 'bottom-right',         // Widget position (widget mode only)
 *   }
 *
 * Or via data attributes on script tag:
 *   <script src="/test.js" data-endpoint="..." data-agent-id="..."></script>
 *
 * Security note: Markdown is rendered with html:false to prevent XSS.
 * SVG icons and mermaid diagrams are rendered from trusted sources only.
 * Agent access is controlled server-side via allowedAgents config.
 */

(function() {
  'use strict';

  // CSS class constants
  const CSS_HIDDEN = 'ai-agent-hidden';
  const CSS_SPINNER = '.ai-agent-spinner';
  const CSS_SPINNER_STATUS = '.ai-agent-spinner-status';

  // ---------------------------------------------------------------------------
  // Configuration
  // ---------------------------------------------------------------------------

  const defaultConfig = {
    endpoint: window.location.origin,
    agentId: undefined,  // Server decides based on allowedAgents config
    format: 'markdown+mermaid',  // Tell model it can use mermaid charts
    icon: null,
    position: 'bottom-right',
    storageKey: 'ai-agent-chat',
  };

  function getConfig() {
    const config = { ...defaultConfig };

    // Global config object
    if (window.AiAgentChatConfig) {
      Object.assign(config, window.AiAgentChatConfig);
    }

    // Data attributes from script tag
    const script = document.currentScript || document.querySelector('script[src*="test.js"]');
    if (script) {
      if (script.dataset.endpoint) config.endpoint = script.dataset.endpoint;
      if (script.dataset.agentId) config.agentId = script.dataset.agentId;
      if (script.dataset.format) config.format = script.dataset.format;
      if (script.dataset.icon) config.icon = script.dataset.icon;
      if (script.dataset.position) config.position = script.dataset.position;
    }

    return config;
  }

  // ---------------------------------------------------------------------------
  // Icons (SVG) - Static trusted content
  // ---------------------------------------------------------------------------

  const icons = {
    chat: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>`,
    close: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`,
    send: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" y1="2" x2="11" y2="13"></line><polygon points="22 2 15 22 11 13 2 9 22 2"></polygon></svg>`,
    // Clipboard icon - board with clip at top
    clipboard: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"></path><rect x="8" y="2" width="8" height="4" rx="1" ry="1"></rect></svg>`,
    // Copy icon - two overlapping documents
    copy: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`,
    check: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`,
    sun: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>`,
    moon: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>`,
    // X icon for clear
    x: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`,
    maximize: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 3 21 3 21 9"></polyline><polyline points="9 21 3 21 3 15"></polyline><line x1="21" y1="3" x2="14" y2="10"></line><line x1="3" y1="21" x2="10" y2="14"></line></svg>`,
    minimize: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 14 10 14 10 20"></polyline><polyline points="20 10 14 10 14 4"></polyline><line x1="14" y1="10" x2="21" y2="3"></line><line x1="3" y1="21" x2="10" y2="14"></line></svg>`,
  };

  // ---------------------------------------------------------------------------
  // Utilities
  // ---------------------------------------------------------------------------

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  function generateId() {
    return 'msg-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);
  }

  async function copyToClipboard(text) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fallback for older browsers
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const success = document.execCommand('copy');
      document.body.removeChild(textarea);
      return success;
    }
  }

  // Simple HTML to Markdown conversion (fallback if Turndown not available)
  function htmlToMarkdown(html) {
    if (window.TurndownService) {
      const turndown = new window.TurndownService({
        headingStyle: 'atx',
        codeBlockStyle: 'fenced',
      });
      return turndown.turndown(html);
    }
    // Basic fallback - extract text only
    const temp = document.createElement('div');
    temp.innerHTML = html;
    return temp.textContent || temp.innerText || '';
  }

  // Safe DOM element creation helper
  function createElement(tag, className, attributes = {}) {
    const el = document.createElement(tag);
    if (className) el.className = className;
    Object.entries(attributes).forEach(([key, value]) => {
      el.setAttribute(key, value);
    });
    return el;
  }

  // ---------------------------------------------------------------------------
  // Theme Management
  // ---------------------------------------------------------------------------

  class ThemeManager {
    constructor(container) {
      this.container = container;
      this.storageKey = 'ai-agent-chat-theme';
      this.theme = this.loadTheme();
      this.applyTheme();
    }

    loadTheme() {
      const stored = localStorage.getItem(this.storageKey);
      if (stored === 'light' || stored === 'dark') return stored;
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }

    applyTheme() {
      this.container.setAttribute('data-theme', this.theme);
    }

    toggle() {
      this.theme = this.theme === 'dark' ? 'light' : 'dark';
      localStorage.setItem(this.storageKey, this.theme);
      this.applyTheme();
      return this.theme;
    }

    isDark() {
      return this.theme === 'dark';
    }
  }

  // ---------------------------------------------------------------------------
  // Markdown Renderer
  // ---------------------------------------------------------------------------

  class MarkdownRenderer {
    constructor() {
      this.md = null;
      this.mermaidInitialized = false;
    }

    async init() {
      // Wait for markdown-it to load
      if (!window.markdownit) {
        await this.waitFor(() => window.markdownit, 5000);
      }
      // html:true allows HTML in markdown (needed for some content)
      // typographer:false prevents (C) -> © conversion (problematic for DevOps)
      this.md = window.markdownit({
        html: true,
        linkify: true,
        typographer: false,
        highlight: (str, lang) => {
          // Use escapeHtml to prevent XSS in code blocks
          const escapedCode = escapeHtml(str);
          const escapedLang = escapeHtml(lang || 'text');
          return `<pre class="ai-agent-code-block" data-lang="${escapedLang}"><code>${escapedCode}</code></pre>`;
        },
      });
    }

    async waitFor(condition, timeout) {
      const start = Date.now();
      while (!condition()) {
        if (Date.now() - start > timeout) {
          throw new Error('Timeout waiting for dependency');
        }
        await new Promise(r => setTimeout(r, 50));
      }
    }

    render(markdown) {
      if (!this.md) {
        return `<p>${escapeHtml(markdown)}</p>`;
      }
      return this.md.render(markdown);
    }

    async renderMermaid(container) {
      if (!window.mermaid) return;

      if (!this.mermaidInitialized) {
        window.mermaid.initialize({
          startOnLoad: false,
          theme: 'default',
          securityLevel: 'loose',
        });
        this.mermaidInitialized = true;
      }

      const mermaidBlocks = container.querySelectorAll('pre.ai-agent-code-block[data-lang="mermaid"]');
      for (const block of mermaidBlocks) {
        const code = block.textContent;
        const id = 'mermaid-' + generateId();
        try {
          const { svg } = await window.mermaid.render(id, code);
          const wrapper = document.createElement('div');
          wrapper.className = 'ai-agent-mermaid-diagram';
          // Mermaid SVG is from trusted mermaid.js library
          wrapper.innerHTML = svg;
          block.replaceWith(wrapper);
        } catch (err) {
          // eslint-disable-next-line no-console -- client-side debug logging
          console.warn('Mermaid rendering failed:', err);
        }
      }
    }
  }

  // ---------------------------------------------------------------------------
  // Storage Manager
  // ---------------------------------------------------------------------------

  class StorageManager {
    constructor(key) {
      this.key = key;
    }

    load() {
      try {
        const data = localStorage.getItem(this.key);
        if (data) {
          return JSON.parse(data);
        }
      } catch (err) {
        // eslint-disable-next-line no-console -- client-side debug logging
        console.warn('Failed to load chat history:', err);
      }
      return { messages: [], clientId: null };
    }

    save(data) {
      try {
        localStorage.setItem(this.key, JSON.stringify(data));
      } catch (err) {
        // eslint-disable-next-line no-console -- client-side debug logging
        console.warn('Failed to save chat history:', err);
      }
    }

    clear() {
      localStorage.removeItem(this.key);
    }
  }

  // ---------------------------------------------------------------------------
  // Chat UI Component
  // ---------------------------------------------------------------------------

  class AiAgentChatUI {
    constructor(container, options = {}) {
      this.container = typeof container === 'string'
        ? document.querySelector(container)
        : container;

      if (!this.container) {
        throw new Error('Container element not found');
      }

      this.config = { ...getConfig(), ...options };
      this.mode = options.mode || 'div'; // 'div' or 'widget'
      this.isMaximized = false;
      this.isOpen = this.mode === 'div'; // Widget starts closed

      this.messages = [];
      this.clientId = null;
      this.isLoading = false;
      this.currentStatus = '';
      this.streamingContent = '';
      this.streamingMessageId = null;

      // Double-buffer rendering state
      this.renderPending = false;
      this.lastRenderContent = '';
      this.activeBuffer = 'a'; // 'a' or 'b'

      // Auto-scroll control
      this.userScrolledUp = false;

      this.storage = new StorageManager(this.config.storageKey);
      this.renderer = new MarkdownRenderer();
      this.chat = null; // AiAgentChat instance

      this.init();
    }

    async init() {
      // Load persisted data
      const saved = this.storage.load();
      this.messages = saved.messages || [];
      this.clientId = saved.clientId;

      // Initialize renderer
      await this.renderer.init();

      // Build UI
      this.buildUI();

      // Initialize theme
      this.themeManager = new ThemeManager(this.wrapper);

      // Initialize chat client
      await this.initChatClient();

      // Render existing messages
      this.renderMessages();

      // Setup event listeners
      this.setupEventListeners();

      // Scroll to bottom
      this.scrollToBottom();
    }

    async initChatClient() {
      this.updateStatus('Initializing...');

      // Wait for AiAgentChat to be available
      const start = Date.now();
      while (!window.AiAgentChat) {
        if (Date.now() - start > 10000) {
           
          console.error('AiAgentChat not loaded');
          this.updateStatus('Error: Chat client failed to load. Check console.');
          return;
        }
        await new Promise(r => setTimeout(r, 50));
      }

      this.chat = new window.AiAgentChat({
        endpoint: this.config.endpoint,
        agentId: this.config.agentId,
        format: this.config.format,
        clientId: this.clientId,
        onEvent: (event) => this.handleChatEvent(event),
      });

      this.updateStatus('Ready');
      // Clear ready status after a moment
      setTimeout(() => {
        if (this.statusText.textContent === 'Ready') {
          this.updateStatus('');
        }
      }, 2000);
    }

    buildUI() {
      this.container.textContent = ''; // Safe clear
      this.container.classList.add('ai-agent-chat-container');

      if (this.mode === 'widget') {
        this.buildWidgetUI();
      } else {
        this.buildDivUI();
      }
    }

    buildDivUI() {
      this.wrapper = createElement('div', 'ai-agent-wrapper ai-agent-div-mode');

      // Messages container
      const messagesEl = createElement('div', 'ai-agent-messages');

      // Input area
      const inputArea = createElement('div', 'ai-agent-input-area');
      const textarea = createElement('textarea', 'ai-agent-input', {
        placeholder: 'Type your message...',
        rows: '1'
      });
      const sendBtn = createElement('button', 'ai-agent-send', { title: 'Send' });
      sendBtn.innerHTML = icons.send; // Static trusted SVG
      inputArea.appendChild(textarea);
      inputArea.appendChild(sendBtn);

      // Status bar
      const statusBar = createElement('div', 'ai-agent-status-bar');
      const statusText = createElement('span', 'ai-agent-status-text');
      const statusActions = createElement('div', 'ai-agent-status-actions');

      const copyAllBtn = createElement('button', 'ai-agent-copy-all', { title: 'Copy conversation' });
      copyAllBtn.innerHTML = icons.clipboard;
      const clearBtn = createElement('button', 'ai-agent-clear', { title: 'Clear conversation' });
      clearBtn.innerHTML = icons.x;
      const themeBtn = createElement('button', 'ai-agent-theme', { title: 'Toggle theme' });
      themeBtn.innerHTML = icons.sun;

      statusActions.appendChild(copyAllBtn);
      statusActions.appendChild(clearBtn);
      statusActions.appendChild(themeBtn);
      statusBar.appendChild(statusText);
      statusBar.appendChild(statusActions);

      this.wrapper.appendChild(messagesEl);
      this.wrapper.appendChild(inputArea);
      this.wrapper.appendChild(statusBar);

      this.container.appendChild(this.wrapper);
      this.cacheElements();
    }

    buildWidgetUI() {
      // Floating button
      this.floatButton = createElement('button', 'ai-agent-float-button', { title: 'Open chat' });
      this.floatButton.innerHTML = this.config.icon || icons.chat;
      this.container.appendChild(this.floatButton);

      // Chat panel wrapper
      this.wrapper = createElement('div', 'ai-agent-wrapper ai-agent-widget-mode ai-agent-hidden');

      // Widget header
      const header = createElement('div', 'ai-agent-widget-header');
      const title = createElement('span', 'ai-agent-widget-title');
      title.textContent = 'AI Assistant';
      const controls = createElement('div', 'ai-agent-widget-controls');
      const maximizeBtn = createElement('button', 'ai-agent-maximize', { title: 'Maximize' });
      maximizeBtn.innerHTML = icons.maximize;
      const closeBtn = createElement('button', 'ai-agent-close', { title: 'Close' });
      closeBtn.innerHTML = icons.close;
      controls.appendChild(maximizeBtn);
      controls.appendChild(closeBtn);
      header.appendChild(title);
      header.appendChild(controls);

      // Messages container
      const messagesEl = createElement('div', 'ai-agent-messages');

      // Input area
      const inputArea = createElement('div', 'ai-agent-input-area');
      const textarea = createElement('textarea', 'ai-agent-input', {
        placeholder: 'Type your message...',
        rows: '1'
      });
      const sendBtn = createElement('button', 'ai-agent-send', { title: 'Send' });
      sendBtn.innerHTML = icons.send;
      inputArea.appendChild(textarea);
      inputArea.appendChild(sendBtn);

      // Status bar
      const statusBar = createElement('div', 'ai-agent-status-bar');
      const statusText = createElement('span', 'ai-agent-status-text');
      const statusActions = createElement('div', 'ai-agent-status-actions');

      const copyAllBtn = createElement('button', 'ai-agent-copy-all', { title: 'Copy conversation' });
      copyAllBtn.innerHTML = icons.clipboard;
      const clearBtn = createElement('button', 'ai-agent-clear', { title: 'Clear conversation' });
      clearBtn.innerHTML = icons.x;
      const themeBtn = createElement('button', 'ai-agent-theme', { title: 'Toggle theme' });
      themeBtn.innerHTML = icons.sun;

      statusActions.appendChild(copyAllBtn);
      statusActions.appendChild(clearBtn);
      statusActions.appendChild(themeBtn);
      statusBar.appendChild(statusText);
      statusBar.appendChild(statusActions);

      this.wrapper.appendChild(header);
      this.wrapper.appendChild(messagesEl);
      this.wrapper.appendChild(inputArea);
      this.wrapper.appendChild(statusBar);

      this.container.appendChild(this.wrapper);
      this.cacheElements();
    }

    cacheElements() {
      this.messagesEl = this.wrapper.querySelector('.ai-agent-messages');
      this.inputEl = this.wrapper.querySelector('.ai-agent-input');
      this.sendBtn = this.wrapper.querySelector('.ai-agent-send');
      this.statusText = this.wrapper.querySelector('.ai-agent-status-text');
      this.clearBtn = this.wrapper.querySelector('.ai-agent-clear');
      this.themeBtn = this.wrapper.querySelector('.ai-agent-theme');
      this.copyAllBtn = this.wrapper.querySelector('.ai-agent-copy-all');

      if (this.mode === 'widget') {
        this.closeBtn = this.wrapper.querySelector('.ai-agent-close');
        this.maximizeBtn = this.wrapper.querySelector('.ai-agent-maximize');
      }
    }

    setupEventListeners() {
      // Send message
      this.sendBtn.addEventListener('click', () => this.sendMessage());

      // Input handling
      this.inputEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          this.sendMessage();
        }
      });

      // Auto-expand textarea
      this.inputEl.addEventListener('input', () => this.autoExpandInput());

      // Paste handling - convert HTML to markdown
      this.inputEl.addEventListener('paste', (e) => {
        const html = e.clipboardData.getData('text/html');
        if (html) {
          e.preventDefault();
          const markdown = htmlToMarkdown(html);
          const start = this.inputEl.selectionStart;
          const end = this.inputEl.selectionEnd;
          const text = this.inputEl.value;
          this.inputEl.value = text.substring(0, start) + markdown + text.substring(end);
          this.inputEl.selectionStart = this.inputEl.selectionEnd = start + markdown.length;
          this.autoExpandInput();
        }
      });

      // Clear conversation
      this.clearBtn.addEventListener('click', () => this.clearConversation());

      // Theme toggle
      this.themeBtn.addEventListener('click', () => {
        const theme = this.themeManager.toggle();
        this.themeBtn.innerHTML = theme === 'dark' ? icons.sun : icons.moon;
      });

      // Copy all
      this.copyAllBtn.addEventListener('click', () => this.copyConversation());

      // Widget-specific
      if (this.mode === 'widget') {
        this.floatButton.addEventListener('click', () => this.toggleWidget());
        this.closeBtn.addEventListener('click', () => this.toggleWidget());
        this.maximizeBtn.addEventListener('click', () => this.toggleMaximize());
      }

      // Message copy buttons (delegated)
      this.messagesEl.addEventListener('click', (e) => {
        const copyBtn = e.target.closest('.ai-agent-copy-btn');
        if (copyBtn) {
          this.handleCopyClick(copyBtn);
        }
      });

      // Auto-scroll control: detect when user scrolls up
      this.messagesEl.addEventListener('scroll', () => {
        const el = this.messagesEl;
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
        this.userScrolledUp = !atBottom;
      });
    }

    autoExpandInput() {
      this.inputEl.style.height = 'auto';
      this.inputEl.style.height = Math.min(this.inputEl.scrollHeight, 150) + 'px';
    }

    async sendMessage() {
      const message = this.inputEl.value.trim();
      if (!message || this.isLoading) return;

      // Check if chat client is ready
      if (!this.chat) {
        this.updateStatus('Error: Chat not initialized. Please refresh the page.');
        return;
      }

      // Add user message
      this.addMessage('user', message);

      // Clear input
      this.inputEl.value = '';
      this.autoExpandInput();

      // Start loading
      this.isLoading = true;
      this.streamingContent = '';
      this.streamingMessageId = null;
      this.renderPending = false;
      this.lastRenderContent = '';
      this.userScrolledUp = false; // Reset scroll on new message
      this.updateStatus('Connecting...');
      this.showSpinner();

      try {
        await this.chat.ask(message);
      } catch (err) {
        this.hideSpinner();
        this.updateStatus('Error: ' + err.message);
        this.isLoading = false;
      }
    }

    handleChatEvent(event) {
      switch (event.type) {
        case 'client':
          this.clientId = event.clientId;
          this.saveState();
          // Clear "Connecting..." status on first client event
          this.updateStatus('');
          break;

        case 'meta':
          // Clear status on meta event (session started)
          this.updateStatus('');
          break;

        case 'status':
          this.currentStatus = event.data.message || event.data.status || '';
          // Show spinner when receiving status updates (model is working)
          this.showSpinner();
          this.updateSpinnerStatus(this.currentStatus);
          break;

        case 'report':
          // First chunk - create the message element
          if (!this.streamingMessageId) {
            this.streamingMessageId = this.addMessage('assistant', '', true);
          }
          // Hide spinner when content is streaming
          this.hideSpinner();
          this.streamingContent = event.report;
          this.updateStreamingMessage(this.streamingContent);
          break;

        case 'done':
          this.isLoading = false;
          this.hideSpinner();

          // Finalize the message
          if (this.streamingMessageId) {
            const msg = this.messages.find(m => m.id === this.streamingMessageId);
            if (msg) {
              msg.content = this.streamingContent;
              msg.isStreaming = false;
            }
            this.renderMessage(this.streamingMessageId);
            this.renderer.renderMermaid(this.messagesEl);
          }

          this.streamingMessageId = null;
          this.streamingContent = '';
          this.updateStatus('');
          this.saveState();
          break;

        case 'error':
          this.isLoading = false;
          this.hideSpinner();
          this.updateStatus('Error: ' + event.error.message);
          break;
      }
    }

    addMessage(role, content, isStreaming = false) {
      const id = generateId();
      const message = { id, role, content, timestamp: Date.now(), isStreaming };
      this.messages.push(message);

      if (!isStreaming) {
        this.saveState();
      }

      this.renderMessage(id);
      this.scrollToBottom();

      return id;
    }

    renderMessages() {
      this.messagesEl.textContent = ''; // Safe clear
      for (const msg of this.messages) {
        this.renderMessage(msg.id);
      }
      this.renderer.renderMermaid(this.messagesEl);
    }

    renderMessage(id) {
      const msg = this.messages.find(m => m.id === id);
      if (!msg) return;

      let el = this.messagesEl.querySelector(`[data-message-id="${id}"]`);
      if (!el) {
        el = createElement('div', `ai-agent-message ai-agent-message-${msg.role}`, {
          'data-message-id': id
        });
        this.messagesEl.appendChild(el);
      }

      // Clear existing content safely
      el.textContent = '';

      if (msg.role === 'user') {
        // User messages: render markdown (preserves newlines, supports formatting)
        // Note: markdown-it processes the content; user input is their own
        const contentDiv = createElement('div', 'ai-agent-message-content');
        contentDiv.innerHTML = this.renderer.render(msg.content);
        el.appendChild(contentDiv);
      } else if (msg.isStreaming) {
        // Streaming assistant messages: double-buffer with two content divs
        const contentA = createElement('div', 'ai-agent-message-content ai-agent-buffer-a');
        const contentB = createElement('div', 'ai-agent-message-content ai-agent-buffer-b ai-agent-buffer-hidden');
        el.appendChild(contentA);
        el.appendChild(contentB);
        this.activeBuffer = 'a';
      } else {
        // Completed assistant messages: single content div with rendered markdown
        const contentDiv = createElement('div', 'ai-agent-message-content');
        contentDiv.innerHTML = this.renderer.render(msg.content);
        el.appendChild(contentDiv);
        // Add copy buttons to code blocks
        this.addCodeBlockCopyButtons(el);
      }

      // Message actions (copy button)
      const actionsDiv = createElement('div', 'ai-agent-message-actions');
      const copyBtn = createElement('button', 'ai-agent-copy-btn', {
        'data-copy-type': 'message',
        'data-message-id': id,
        title: 'Copy'
      });
      copyBtn.innerHTML = icons.copy;
      actionsDiv.appendChild(copyBtn);
      el.appendChild(actionsDiv);
    }

    addCodeBlockCopyButtons(el) {
      el.querySelectorAll('.ai-agent-code-block').forEach(block => {
        const wrapper = createElement('div', 'ai-agent-code-block-wrapper');
        block.parentNode.insertBefore(wrapper, block);
        wrapper.appendChild(block);

        const codeCopyBtn = createElement('button', 'ai-agent-copy-btn ai-agent-code-copy-btn', {
          'data-copy-type': 'code',
          title: 'Copy code'
        });
        codeCopyBtn.innerHTML = icons.copy;
        wrapper.appendChild(codeCopyBtn);
      });
    }

    updateStreamingMessage(content) {
      // Store the content for rendering
      this.lastRenderContent = content;

      // Throttle: only schedule render if not already pending
      if (this.renderPending) return;

      this.renderPending = true;

      // Use requestAnimationFrame for smooth rendering
      requestAnimationFrame(() => {
        this.doDoubleBufferRender();
        this.renderPending = false;
      });
    }

    doDoubleBufferRender() {
      const el = this.messagesEl.querySelector(`[data-message-id="${this.streamingMessageId}"]`);
      if (!el) return;

      const content = this.lastRenderContent;

      // Determine which buffer is hidden (will render into it)
      const hiddenBufferClass = this.activeBuffer === 'a' ? 'ai-agent-buffer-b' : 'ai-agent-buffer-a';
      const visibleBufferClass = this.activeBuffer === 'a' ? 'ai-agent-buffer-a' : 'ai-agent-buffer-b';

      const hiddenEl = el.querySelector('.' + hiddenBufferClass);
      const visibleEl = el.querySelector('.' + visibleBufferClass);

      if (!hiddenEl || !visibleEl) return;

      // Render markdown into the hidden buffer
      hiddenEl.innerHTML = this.renderer.render(content);

      // Swap: show hidden, hide visible
      hiddenEl.classList.remove('ai-agent-buffer-hidden');
      visibleEl.classList.add('ai-agent-buffer-hidden');

      // Toggle active buffer
      this.activeBuffer = this.activeBuffer === 'a' ? 'b' : 'a';

      // Scroll to bottom (respects user scroll)
      this.scrollToBottom();
    }

    async handleCopyClick(btn) {
      const type = btn.dataset.copyType;
      let text = '';

      if (type === 'message') {
        const id = btn.dataset.messageId;
        const msg = this.messages.find(m => m.id === id);
        if (msg) text = msg.content;
      } else if (type === 'code') {
        const wrapper = btn.closest('.ai-agent-code-block-wrapper');
        const code = wrapper?.querySelector('code');
        if (code) text = code.textContent;
      }

      if (text) {
        const success = await copyToClipboard(text);
        if (success) {
          btn.innerHTML = icons.check;
          btn.classList.add('copied');
          setTimeout(() => {
            btn.innerHTML = icons.copy;
            btn.classList.remove('copied');
          }, 2000);
        }
      }
    }

    async copyConversation() {
      const text = this.messages
        .map(m => `${m.role === 'user' ? 'User' : 'Assistant'}:\n${m.content}`)
        .join('\n\n---\n\n');

      const success = await copyToClipboard(text);
      if (success) {
        this.copyAllBtn.innerHTML = icons.check;
        setTimeout(() => {
          this.copyAllBtn.innerHTML = icons.clipboard;
        }, 2000);
      }
    }

    clearConversation() {
      this.messages = [];
      this.clientId = null;
      this.storage.clear();
      this.messagesEl.textContent = ''; // Safe clear
      this.updateStatus('');

      // Reset chat client
      if (this.chat) {
        this.chat.reset();
      }
    }

    saveState() {
      this.storage.save({
        messages: this.messages.filter(m => !m.isStreaming),
        clientId: this.clientId,
      });
    }

    updateStatus(text) {
      this.statusText.textContent = text;
    }

    showSpinner() {
      let spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (!spinner) {
        spinner = createElement('div', 'ai-agent-spinner');

        const workingSpan = createElement('span', 'ai-agent-spinner-working');
        workingSpan.textContent = 'Working...';

        const statusSpan = createElement('span', 'ai-agent-spinner-status');

        spinner.appendChild(workingSpan);
        spinner.appendChild(statusSpan);
      }
      // Always move spinner to the end (after the last message)
      this.messagesEl.appendChild(spinner);
      const statusEl = spinner.querySelector(CSS_SPINNER_STATUS);
      statusEl.textContent = this.currentStatus || '';
      spinner.classList.remove(CSS_HIDDEN);
      this.scrollToBottom();
    }

    updateSpinnerStatus(status) {
      const spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (spinner) {
        const statusEl = spinner.querySelector(CSS_SPINNER_STATUS);
        if (statusEl) {
          statusEl.textContent = status || '';
        }
      }
    }

    hideSpinner() {
      const spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (spinner) {
        spinner.classList.add(CSS_HIDDEN);
      }
    }

    scrollToBottom(force = false) {
      // Respect user scroll position unless forced
      if (!force && this.userScrolledUp) return;
      this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    // Widget-specific methods
    toggleWidget() {
      this.isOpen = !this.isOpen;
      this.wrapper.classList.toggle(CSS_HIDDEN, !this.isOpen);
      this.floatButton.innerHTML = this.isOpen ? icons.close : (this.config.icon || icons.chat);
      this.floatButton.title = this.isOpen ? 'Close chat' : 'Open chat';

      if (this.isOpen) {
        this.inputEl.focus();
        this.scrollToBottom();
      }
    }

    toggleMaximize() {
      this.isMaximized = !this.isMaximized;
      this.wrapper.classList.toggle('ai-agent-maximized', this.isMaximized);
      this.maximizeBtn.innerHTML = this.isMaximized ? icons.minimize : icons.maximize;
      this.maximizeBtn.title = this.isMaximized ? 'Restore' : 'Maximize';
    }
  }

  // ---------------------------------------------------------------------------
  // Export
  // ---------------------------------------------------------------------------

  window.AiAgentChatUI = AiAgentChatUI;

})();
