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
    maxInputBytes: 10240,    // 10KB max input size
    maxHistoryPairs: 5,      // Send last 5 user-assistant pairs
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
  // UI Text Constants
  // ---------------------------------------------------------------------------

  const TEXT_WORKING = 'Working...';
  const TEXT_TYPE_MESSAGE = 'Type a message...';

  // ---------------------------------------------------------------------------
  // Icons (SVG) - Static trusted content
  // ---------------------------------------------------------------------------

  const icons = {
    chat: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>`,
    close: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`,
    send: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="22" y1="2" x2="11" y2="13"></line><polygon points="22 2 15 22 11 13 2 9 22 2"></polygon></svg>`,
    stop: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="12" height="12" rx="2" ry="2"></rect></svg>`,
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
    // Stats icons (monochrome)
    clock: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>`,
    brain: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.5 2A2.5 2.5 0 0 1 12 4.5v15a2.5 2.5 0 0 1-4.96.44 2.5 2.5 0 0 1-2.96-3.08 3 3 0 0 1-.34-5.58 2.5 2.5 0 0 1 1.32-4.24 2.5 2.5 0 0 1 1.98-3A2.5 2.5 0 0 1 9.5 2Z"></path><path d="M14.5 2A2.5 2.5 0 0 0 12 4.5v15a2.5 2.5 0 0 0 4.96.44 2.5 2.5 0 0 0 2.96-3.08 3 3 0 0 0 .34-5.58 2.5 2.5 0 0 0-1.32-4.24 2.5 2.5 0 0 0-1.98-3A2.5 2.5 0 0 0 14.5 2Z"></path></svg>`,
    output: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" y1="3" x2="12" y2="15"></line></svg>`,
    file: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line></svg>`,
    wrench: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"></path></svg>`,
    users: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle><path d="M23 21v-2a4 4 0 0 0-3-3.87"></path><path d="M16 3.13a4 4 0 0 1 0 7.75"></path></svg>`,
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
      this.mdUser = null;
      this.mermaidInitialized = false;
    }

    async init() {
      // Wait for markdown-it to load
      if (!window.markdownit) {
        await this.waitFor(() => window.markdownit, 5000);
      }

      const highlightFn = (str, lang) => {
        // Use escapeHtml to prevent XSS in code blocks
        const escapedCode = escapeHtml(str);
        const escapedLang = escapeHtml(lang || 'text');
        return `<pre class="ai-agent-code-block" data-lang="${escapedLang}"><code>${escapedCode}</code></pre>`;
      };

      // Both assistant and user messages: html:false for security
      // SVG is handled separately via ```svg code blocks
      // typographer:false prevents (C) -> © conversion (problematic for DevOps)
      this.md = window.markdownit({
        html: false,
        linkify: true,
        typographer: false,
        highlight: highlightFn,
      });

      // User messages use same config
      this.mdUser = window.markdownit({
        html: false,
        linkify: true,
        typographer: false,
        highlight: highlightFn,
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

    renderUser(markdown) {
      if (!this.mdUser) {
        return `<p>${escapeHtml(markdown)}</p>`;
      }
      return this.mdUser.render(markdown);
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

    renderSvg(container) {
      const svgBlocks = container.querySelectorAll('pre.ai-agent-code-block[data-lang="svg"]');
      for (const block of svgBlocks) {
        const code = block.textContent.trim();
        try {
          const sanitized = this.sanitizeSvg(code);
          if (sanitized) {
            const wrapper = document.createElement('div');
            wrapper.className = 'ai-agent-svg-diagram';
            wrapper.innerHTML = sanitized;
            block.replaceWith(wrapper);
          }
        } catch (err) {
          // eslint-disable-next-line no-console -- client-side debug logging
          console.warn('SVG rendering failed:', err);
        }
      }
    }

    sanitizeSvg(svgString) {
      // Parse as XML to validate structure
      const parser = new DOMParser();
      const doc = parser.parseFromString(svgString, 'image/svg+xml');

      // Check for parse errors
      const parseError = doc.querySelector('parsererror');
      if (parseError) {
        // eslint-disable-next-line no-console -- client-side debug logging
        console.warn('SVG parse error:', parseError.textContent);
        return null;
      }

      const svg = doc.documentElement;
      if (svg.tagName.toLowerCase() !== 'svg') {
        return null;
      }

      // Remove dangerous elements
      const dangerousTags = ['script', 'foreignObject', 'iframe', 'object', 'embed', 'use'];
      for (const tag of dangerousTags) {
        const elements = svg.querySelectorAll(tag);
        for (const el of elements) {
          el.remove();
        }
      }

      // Remove dangerous attributes from all elements
      const allElements = svg.querySelectorAll('*');
      for (const el of [svg, ...allElements]) {
        // Remove event handlers (on*)
        const attrs = [...el.attributes];
        for (const attr of attrs) {
          const name = attr.name.toLowerCase();
          if (name.startsWith('on') || name === 'href' && attr.value.trim().toLowerCase().startsWith('javascript:')) {
            el.removeAttribute(attr.name);
          }
        }
        // Remove xlink:href with javascript:
        const xlinkHref = el.getAttributeNS('http://www.w3.org/1999/xlink', 'href');
        if (xlinkHref && xlinkHref.trim().toLowerCase().startsWith('javascript:')) {
          el.removeAttributeNS('http://www.w3.org/1999/xlink', 'href');
        }
      }

      return svg.outerHTML;
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
      this.statusMessage = 'Ready';
      this.spinnerVisible = false;
      this.metricsData = this.buildEmptyMetrics();
      this.requestCompleted = false;

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
        maxHistoryPairs: this.config.maxHistoryPairs,
        maxInputBytes: this.config.maxInputBytes,
        onEvent: (event) => this.handleChatEvent(event),
      });

      this.updateStatus('Ready');
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
      // Send/Stop button - sends message or aborts request
      this.sendBtn.addEventListener('click', () => {
        if (this.isLoading) {
          this.abortRequest();
        } else {
          this.sendMessage();
        }
      });

      // Input handling
      this.inputEl.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault();
          if (!this.isLoading) {
            this.sendMessage();
          }
        }
      });

      // Auto-expand textarea
      this.inputEl.addEventListener('input', () => this.autoExpandInput());

      // Paste handling - convert HTML to markdown, enforce size limit
      this.inputEl.addEventListener('paste', (e) => {
        e.preventDefault();
        const html = e.clipboardData.getData('text/html');
        const plain = e.clipboardData.getData('text/plain');
        let pasteContent = html ? htmlToMarkdown(html) : plain;

        const start = this.inputEl.selectionStart;
        const end = this.inputEl.selectionEnd;
        const before = this.inputEl.value.substring(0, start);
        const after = this.inputEl.value.substring(end);

        // Enforce max input size
        const maxBytes = this.config.maxInputBytes || 10240;
        const encoder = new TextEncoder();
        const existingBytes = encoder.encode(before + after).length;
        const availableBytes = Math.max(0, maxBytes - existingBytes);
        const pasteBytes = encoder.encode(pasteContent).length;

        if (pasteBytes > availableBytes) {
          // Truncate paste content to fit within limit
          pasteContent = this.truncateToBytes(pasteContent, availableBytes);
          this.showInputLimitWarning();
        }

        this.inputEl.value = before + pasteContent + after;
        this.inputEl.selectionStart = this.inputEl.selectionEnd = start + pasteContent.length;
        this.autoExpandInput();
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
      const maxHeight = Math.floor(window.innerHeight / 3); // 1/3 of screen height
      this.inputEl.style.height = Math.min(this.inputEl.scrollHeight, maxHeight) + 'px';
    }

    truncateToBytes(str, maxBytes) {
      const encoder = new TextEncoder();
      const decoder = new TextDecoder();
      const bytes = encoder.encode(str);
      if (bytes.length <= maxBytes) return str;
      // Truncate and decode - this handles multi-byte chars safely
      return decoder.decode(bytes.slice(0, maxBytes));
    }

    showInputLimitWarning() {
      // Brief visual feedback that input was truncated
      this.inputEl.classList.add('ai-agent-input-limit-warning');
      setTimeout(() => {
        this.inputEl.classList.remove('ai-agent-input-limit-warning');
      }, 1500);
    }

    updateInputState() {
      // Update input disabled state and placeholder
      this.inputEl.disabled = this.isLoading;
      this.inputEl.classList.toggle('ai-agent-input-disabled', this.isLoading);
      this.inputEl.placeholder = this.isLoading ? TEXT_WORKING : TEXT_TYPE_MESSAGE;

      // Update button icon and title
      if (this.isLoading) {
        this.sendBtn.innerHTML = icons.stop;
        this.sendBtn.title = 'Stop';
        this.sendBtn.classList.add('ai-agent-send-stop');
      } else {
        this.sendBtn.innerHTML = icons.send;
        this.sendBtn.title = 'Send';
        this.sendBtn.classList.remove('ai-agent-send-stop');
      }
    }

    abortRequest() {
      if (!this.isLoading || !this.chat) return;
      this.chat.abort();
      this.requestCompleted = true;
      this.isLoading = false;
      this.clearInactivityTimer();
      this.hideSpinner();
      this.updateInputState();
      this.updateStatus('Stopped');

      // Finalize the streaming message with stop indicator
      if (this.streamingMessageId) {
        const msg = this.messages.find(m => m.id === this.streamingMessageId);
        if (msg) {
          // Append stop indicator so the model knows the response was cut off
          const stopIndicator = '\n\n[OUTPUT STOPPED BY THE USER]';
          msg.content = this.streamingContent + stopIndicator;
          msg.isStreaming = false;
        }
        this.renderMessage(this.streamingMessageId);
        this.streamingMessageId = null;
        this.streamingContent = '';
        this.saveState();
      }
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
      this.requestCompleted = false;
      this.currentStatus = '';
      this.inactivityTimer = null;
      this.updateInputState();
      this.updateStatus(TEXT_WORKING);
      this.resetMetrics();
      this.showSpinner();

      try {
        await this.chat.ask(message);
        if (this.isLoading && !this.requestCompleted) {
          this.handleStreamError('Error: Connection closed');
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        // Ignore abort errors - they're expected when user clicks stop
        if (err instanceof Error && err.name === 'AbortError') {
          return;
        }
        if (!this.requestCompleted) {
          this.handleStreamError(`Error: ${msg}`);
        }
      }
    }

    handleChatEvent(event) {
      switch (event.type) {
        case 'client':
          this.clientId = event.clientId;
          this.saveState();
          break;

        case 'meta':
          break;

        case 'status':
          // Use only the "now" field from simplified status updates
          this.currentStatus = event.data.now || '';
          // Show spinner when receiving status updates (model is working)
          this.showSpinner();
          this.updateSpinnerStatus(this.currentStatus);
          break;

        case 'metrics':
          this.updateMetrics(event.data);
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
          // Start inactivity timer - show spinner again if no content for 2s
          this.startInactivityTimer();
          break;

        case 'done':
          this.requestCompleted = true;
          this.isLoading = false;
          this.clearInactivityTimer();
          this.hideSpinner();
          this.updateInputState();

          // Finalize the message
          if (this.streamingMessageId) {
            const msg = this.messages.find(m => m.id === this.streamingMessageId);
            if (msg) {
              msg.content = this.streamingContent;
              msg.isStreaming = false;
            }
            this.renderMessage(this.streamingMessageId);
            this.renderer.renderMermaid(this.messagesEl);
            this.renderer.renderSvg(this.messagesEl);

            // Add stats footer to the completed message (not saved in history)
            const msgEl = this.messagesEl.querySelector(`[data-message-id="${this.streamingMessageId}"]`);
            if (msgEl) {
              this.addMessageFooter(msgEl, this.streamingMessageId, { ...this.metricsData });
            }
          }

          this.streamingMessageId = null;
          this.streamingContent = '';
          this.updateStatus('Ready');
          this.saveState();
          break;

        case 'error':
          this.requestCompleted = true;
          this.clearInactivityTimer();
          this.handleStreamError(`Error: ${event.error.message}`);
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
        // Add footer for assistant messages from history (no stats available)
        if (msg.role === 'assistant') {
          const el = this.messagesEl.querySelector(`[data-message-id="${msg.id}"]`);
          if (el) this.addMessageFooter(el, msg.id, null);
        }
      }
      this.renderer.renderMermaid(this.messagesEl);
      this.renderer.renderSvg(this.messagesEl);
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
        // User messages: render markdown without HTML support (plain markdown only)
        const contentDiv = createElement('div', 'ai-agent-message-content');
        contentDiv.innerHTML = this.renderer.renderUser(msg.content);
        el.appendChild(contentDiv);
        // User message actions (copy button on right)
        const actionsDiv = createElement('div', 'ai-agent-message-actions');
        const copyBtn = createElement('button', 'ai-agent-copy-btn', {
          'data-copy-type': 'message',
          'data-message-id': id,
          title: 'Copy'
        });
        copyBtn.innerHTML = icons.copy;
        actionsDiv.appendChild(copyBtn);
        el.appendChild(actionsDiv);
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
        // Footer is added separately: with stats in 'done' event, without stats for history
      }
    }

    addMessageFooter(el, messageId, metrics) {
      // Remove existing footer if any
      const existingFooter = el.querySelector('.ai-agent-message-footer');
      if (existingFooter) existingFooter.remove();

      const footer = createElement('div', 'ai-agent-message-footer');

      // Add stats if provided (not saved in history, only shown for current session)
      if (metrics) {
        const statsDiv = createElement('div', 'ai-agent-message-stats');
        const formatNumber = (value) => (Number.isFinite(value) ? value.toLocaleString() : '0');
        const formatTime = (ms) => {
          const totalSeconds = ms / 1000;
          if (totalSeconds >= 60) {
            const minutes = Math.floor(totalSeconds / 60);
            const seconds = (totalSeconds % 60).toFixed(1);
            return `${minutes}:${seconds.padStart(4, '0')}`;
          }
          return totalSeconds.toFixed(1) + 's';
        };

        const statItems = [
          { value: formatTime(metrics.elapsed), icon: icons.clock, title: 'The duration since this request started' },
          { value: formatNumber(metrics.reasoningChars), icon: icons.brain, title: 'The size of models\' reasoning/thinking' },
          { value: formatNumber(metrics.outputChars), icon: icons.output, title: 'The size of models\' output' },
          { value: formatNumber(metrics.documentsChars), icon: icons.file, title: 'The size of documents read by the models' },
          { value: formatNumber(metrics.tools), icon: icons.wrench, title: 'The number of tools executed' },
          { value: formatNumber(metrics.llmCalls), icon: icons.chat, title: 'The number of LLM calls made' },
        ];

        statItems.forEach(item => {
          const statEl = createElement('span', 'ai-agent-stat-item', { title: item.title });
          statEl.innerHTML = item.icon;
          const valueEl = createElement('span', 'ai-agent-stat-value');
          valueEl.textContent = item.value;
          statEl.appendChild(valueEl);
          statsDiv.appendChild(statEl);
        });

        footer.appendChild(statsDiv);
      }

      // Copy button
      const copyBtn = createElement('button', 'ai-agent-copy-btn', {
        'data-copy-type': 'message',
        'data-message-id': messageId,
        title: 'Copy response'
      });
      copyBtn.innerHTML = icons.copy;
      footer.appendChild(copyBtn);

      el.appendChild(footer);
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
      this.updateStatus('Ready');

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

    buildEmptyMetrics() {
      return {
        elapsed: 0,
        reasoningChars: 0,
        outputChars: 0,
        documentsChars: 0,
        tools: 0,
        llmCalls: 0,
      };
    }

    resetMetrics() {
      this.metricsData = this.buildEmptyMetrics();
      this.renderStatusBar();
    }

    updateMetrics(payload) {
      this.metricsData = {
        elapsed: typeof payload.elapsed === 'number' ? payload.elapsed : 0,
        reasoningChars: typeof payload.reasoningChars === 'number' ? payload.reasoningChars : 0,
        outputChars: typeof payload.outputChars === 'number' ? payload.outputChars : 0,
        documentsChars: typeof payload.documentsChars === 'number' ? payload.documentsChars : 0,
        tools: typeof payload.tools === 'number' ? payload.tools : 0,
        llmCalls: typeof payload.llmCalls === 'number' ? payload.llmCalls : 0,
      };
      // Update spinner stats if visible
      if (this.spinnerVisible) {
        this.updateSpinnerStats();
      }
      this.renderStatusBar();
    }

    renderStatusBar() {
      if (!this.statusText) return;
      // Stats are shown in spinner when visible, so just show status message
      this.statusText.textContent = this.statusMessage;
    }

    updateStatus(text) {
      this.statusMessage = text;
      this.renderStatusBar();
    }

    handleStreamError(message) {
      this.isLoading = false;
      this.hideSpinner();
      this.updateInputState();
      this.updateStatus(message);
    }

    showSpinner() {
      let spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (!spinner) {
        spinner = createElement('div', 'ai-agent-spinner');

        const workingSpan = createElement('span', 'ai-agent-spinner-working');
        workingSpan.textContent = TEXT_WORKING;

        // Stats row with SVG icons (static trusted content from icons object)
        const statsDiv = createElement('div', 'ai-agent-spinner-stats');
        const statItems = [
          { key: 'elapsed', icon: icons.clock, title: 'The duration since this request started' },
          { key: 'reasoningChars', icon: icons.brain, title: 'The size of models\' reasoning/thinking' },
          { key: 'outputChars', icon: icons.output, title: 'The size of models\' output' },
          { key: 'documentsChars', icon: icons.file, title: 'The size of documents read by the models' },
          { key: 'tools', icon: icons.wrench, title: 'The number of tools executed' },
          { key: 'llmCalls', icon: icons.chat, title: 'The number of LLM calls made' },
        ];
        statItems.forEach(item => {
          const statEl = createElement('span', 'ai-agent-stat-item', { 'data-stat': item.key, title: item.title });
          // Static trusted SVG from icons object (not user input)
          statEl.innerHTML = item.icon;
          const valueEl = createElement('span', 'ai-agent-stat-value');
          valueEl.textContent = '0';
          statEl.appendChild(valueEl);
          statsDiv.appendChild(statEl);
        });

        const statusSpan = createElement('span', 'ai-agent-spinner-status');

        spinner.appendChild(workingSpan);
        spinner.appendChild(statsDiv);
        spinner.appendChild(statusSpan);
      }
      // Always move spinner to the end (after the last message)
      this.messagesEl.appendChild(spinner);
      const statusEl = spinner.querySelector(CSS_SPINNER_STATUS);
      const hasStatus = this.currentStatus && this.currentStatus.length > 0;
      statusEl.textContent = hasStatus ? this.currentStatus : '';
      statusEl.classList.toggle(CSS_HIDDEN, !hasStatus);
      spinner.classList.remove(CSS_HIDDEN);
      this.spinnerVisible = true;
      // Update stats in spinner
      this.updateSpinnerStats();
      this.renderStatusBar();
      this.scrollToBottom();
    }

    updateSpinnerStatus(status) {
      const spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (spinner) {
        const statusEl = spinner.querySelector(CSS_SPINNER_STATUS);
        if (statusEl) {
          const hasStatus = status && status.length > 0;
          statusEl.textContent = hasStatus ? status : '';
          statusEl.classList.toggle(CSS_HIDDEN, !hasStatus);
        }
      }
    }

    updateSpinnerStats() {
      const spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (!spinner) return;

      const formatNumber = (value) => (Number.isFinite(value) ? value.toLocaleString() : '0');
      const metrics = this.metricsData;

      // Update elapsed time (M:SS.S when >= 60s, otherwise SS.Ss)
      const elapsedEl = spinner.querySelector('[data-stat="elapsed"] .ai-agent-stat-value');
      if (elapsedEl) {
        const totalSeconds = metrics.elapsed / 1000;
        if (totalSeconds >= 60) {
          const minutes = Math.floor(totalSeconds / 60);
          const seconds = (totalSeconds % 60).toFixed(1);
          elapsedEl.textContent = `${minutes}:${seconds.padStart(4, '0')}`;
        } else {
          elapsedEl.textContent = totalSeconds.toFixed(1) + 's';
        }
      }

      // Update other stats
      const statMappings = [
        { key: 'reasoningChars', suffix: '' },
        { key: 'outputChars', suffix: '' },
        { key: 'documentsChars', suffix: '' },
        { key: 'tools', suffix: '' },
        { key: 'llmCalls', suffix: '' },
      ];
      statMappings.forEach(mapping => {
        const el = spinner.querySelector(`[data-stat="${mapping.key}"] .ai-agent-stat-value`);
        if (el) {
          el.textContent = formatNumber(metrics[mapping.key]) + mapping.suffix;
        }
      });
    }

    hideSpinner() {
      const spinner = this.messagesEl.querySelector(CSS_SPINNER);
      if (spinner) {
        spinner.classList.add(CSS_HIDDEN);
      }
      this.spinnerVisible = false;
      this.renderStatusBar();
    }

    startInactivityTimer() {
      // Clear any existing timer
      this.clearInactivityTimer();
      // Show spinner again after 2s of no content
      this.inactivityTimer = setTimeout(() => {
        if (this.isLoading && !this.requestCompleted) {
          this.showSpinner();
        }
      }, 2000);
    }

    clearInactivityTimer() {
      if (this.inactivityTimer) {
        clearTimeout(this.inactivityTimer);
        this.inactivityTimer = null;
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
