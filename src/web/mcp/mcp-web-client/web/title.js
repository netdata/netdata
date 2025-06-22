/**
 * Title Generation Module
 * 
 * Handles automatic and manual title generation for chats
 * with configurable model selection
 */

import * as ChatConfig from './chat-config.js';
import * as SystemMsg from './system-msg.js';

// Title generation prompt constant
const TITLE_REQUEST_PROMPT = 
    'Please provide a short, descriptive title (max 50 characters) for this conversation.\n' +
    'Respond with ONLY the title text, no quotes, no explanation.';

/**
 * Generate a title for the chat conversation
 * @param {Object} chat - The chat object
 * @param {Object} mcpConnection - MCP connection (not used but kept for consistency)
 * @param {Object} provider - LLM provider to use for generation
 * @param {boolean} isAutomatic - Whether this is automatic generation
 * @param {boolean} force - Force generation even if disabled in config
 * @param {Object} callbacks - Callback functions for UI updates
 * @returns {Promise<string|null>} - Generated title or null if skipped/failed
 */
export async function generateChatTitle(chat, mcpConnection, provider, isAutomatic = false, force = false, callbacks = {}) {
    // Check if title generation is enabled (unless forced)
    if (!force && !chat.config?.optimisation?.titleGeneration?.enabled) {
        console.log('Title generation is disabled in config, skipping');
        return null;
    }
    
    try {
        // Create a system message for title generation
        
        // Save title request message
        if (callbacks.addMessage) {
            callbacks.addMessage(chat.id, { 
                role: 'system-title', 
                content: TITLE_REQUEST_PROMPT,
                timestamp: new Date().toISOString()
            });
        }
        
        // Display the system-title request as a collapsible node
        if (callbacks.processRenderEvent) {
            callbacks.processRenderEvent({ type: 'system-title-message', content: TITLE_REQUEST_PROMPT }, chat.id);
            // Show loading spinner
            callbacks.processRenderEvent({ type: 'show-spinner' }, chat.id);
        }
        
        // Build conversational messages (no tools) for title generation
        const cleanMessages = buildConversationalMessages(chat.messages, false);
        
        // Create messages array with specialized system prompt for title generation
        const titleSystemPrompt = SystemMsg.createSpecializedSystemPrompt('title');
        const messages = [{
            role: 'system', 
            content: titleSystemPrompt
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
        messages.push({ role: 'user', content: TITLE_REQUEST_PROMPT });
        
        // Send request with low temperature for consistent titles
        const temperature = 0.3;
        const llmStartTime = Date.now();
        const response = await provider.sendMessage(messages, [], temperature, 'all-off', null, null);
        const llmResponseTime = Date.now() - llmStartTime;
        
        // Process the title response
        if (response.content) {
            // Store the title response with the 'title' role
            if (callbacks.addMessage) {
                callbacks.addMessage(chat.id, { 
                    role: 'title', 
                    content: response.content,
                    usage: response.usage || null,
                    responseTime: llmResponseTime || null,
                    model: provider.model || ChatConfig.getChatModelString(chat),
                    timestamp: new Date().toISOString()
                });
            }
            
            // Display the response as a title message WITH metrics
            if (callbacks.processRenderEvent) {
                callbacks.processRenderEvent({ 
                    type: 'title-message', 
                    content: response.content,
                    usage: response.usage,
                    responseTime: llmResponseTime,
                    model: provider.model || ChatConfig.getChatModelString(chat)
                }, chat.id);
            }
            
            // Extract text content from response
            let titleText = '';
            if (typeof response.content === 'string') {
                titleText = response.content;
            } else if (Array.isArray(response.content)) {
                // Extract text from content array
                const textBlocks = response.content.filter(block => block.type === 'text');
                titleText = textBlocks.map(block => block.text || '').join(' ').trim();
            }
            
            // Extract and clean the title
            const newTitle = titleText.trim()
                .replace(/^["']|["']$/g, '') // Remove quotes
                .replace(/^Title:\s*/i, '') // Remove "Title:" prefix if present
                .substring(0, 65); // Allow some tolerance beyond 50 chars
            
            // Update the chat title
            if (newTitle && newTitle.length > 0) {
                chat.title = newTitle;
                chat.updatedAt = new Date().toISOString();
                
                // Mark that title was generated
                chat.titleGenerated = true;
                
                // Trigger UI updates
                if (callbacks.updateChatSessions) {
                    callbacks.updateChatSessions();
                }
                
                if (callbacks.updateChatTitle) {
                    callbacks.updateChatTitle(chat.id, newTitle);
                }
                
                // Save changes (unless automatic)
                if (!isAutomatic && callbacks.saveChatToStorage) {
                    callbacks.saveChatToStorage(chat.id);
                }
                
                return newTitle;
            }
        }
        
        return null;
        
    } catch (error) {
        console.error('Title generation failed:', error);
        
        // Hide spinner on error
        if (callbacks.processRenderEvent) {
            callbacks.processRenderEvent({ type: 'hide-spinner' }, chat.id);
        }
        
        if (!isAutomatic && callbacks.showError) {
            callbacks.showError(`Failed to generate title: ${error.message}`, chat.id);
        }
        
        // Remove the system-title message if it failed
        if (callbacks.removeLastMessage) {
            const lastMsg = chat.messages[chat.messages.length - 1];
            if (lastMsg && lastMsg.role === 'system-title') {
                callbacks.removeLastMessage(chat.id);
            }
        }
        
        throw error; // Re-throw for caller to handle
    } finally {
        // Hide spinner
        if (callbacks.processRenderEvent) {
            callbacks.processRenderEvent({ type: 'hide-spinner' }, chat.id);
        }
        // Clear assistant group
        if (callbacks.clearCurrentAssistantGroup) {
            callbacks.clearCurrentAssistantGroup(chat.id);
        }
    }
}

/**
 * Determine which provider to use for title generation based on config
 * @param {Object} chat - The chat object
 * @param {Object} llmProvider - The LLM provider configuration
 * @param {Object} defaultProvider - The default provider to fall back to
 * @param {Function} createLLMProvider - Factory function to create providers
 * @returns {Object} - The provider to use for title generation
 */
export function getTitleGenerationProvider(chat, llmProvider, defaultProvider, createLLMProvider) {
    // If title generation is enabled and has a model configured, use that
    if (chat.config?.optimisation?.titleGeneration?.enabled && 
        chat.config.optimisation.titleGeneration.model) {
        const titleModel = chat.config.optimisation.titleGeneration.model;
        const provider = createLLMProvider(
            titleModel.provider,
            llmProvider.proxyUrl,
            titleModel.id
        );
        provider.onLog = llmProvider.onLog;
        return provider;
    }
    
    // Otherwise use the default provider (chat's primary model)
    console.warn('Using chat primary model for title generation - no dedicated title model configured');
    return defaultProvider;
}

/**
 * Check if automatic title generation should run
 * @param {Object} chat - The chat object
 * @returns {boolean} - Whether to generate title automatically
 */
export function shouldGenerateTitleAutomatically(chat) {
    // Check basic conditions
    if (!chat || chat.titleGenerated) {
        return false;
    }
    
    // Check if title generation is enabled
    if (!chat.config?.optimisation?.titleGeneration?.enabled) {
        return false;
    }
    
    // Check if there are messages to generate from
    const userMessages = chat.messages.filter(m => m.role === 'user');
    const assistantMessages = chat.messages.filter(m => m.role === 'assistant');
    
    // Need at least one exchange
    return userMessages.length > 0 && assistantMessages.length > 0;
}

/**
 * Build messages for LLM context, stripping tool-related content
 * This creates a clean conversation flow without tool calls/results
 * Used for title generation to reduce costs
 * @param {Array} messages - Raw chat messages
 * @param {boolean} includeSystemPrompt - Whether to include the system prompt
 * @returns {Array} Clean messages array with only conversational content
 */
function buildConversationalMessages(messages, includeSystemPrompt = true) {
    const cleanMessages = [];
    
    // Add system prompt if requested and it exists
    if (includeSystemPrompt && messages.length > 0 && messages[0].role === 'system') {
        cleanMessages.push({ 
            role: 'system', 
            content: messages[0].content 
        });
    }
    
    
    // Process messages, skipping tool-related content
    for (let i = includeSystemPrompt && messages[0]?.role === 'system' ? 1 : 0; i < messages.length; i++) {
        const msg = messages[i];
        
        if (msg.role === 'user' && msg.content) {
            // Always include user messages
            cleanMessages.push({ role: 'user', content: msg.content });
        } else if (msg.role === 'assistant') {
            // Process assistant messages - extract text content
            let textContent = '';
            
            if (typeof msg.content === 'string') {
                textContent = msg.content;
            } else if (Array.isArray(msg.content)) {
                // Handle structured content
                const textParts = [];
                for (const block of msg.content) {
                    if (block.type === 'text' && block.text) {
                        textParts.push(block.text);
                    }
                }
                textContent = textParts.join('\n\n');
            }
            
            // Only add if there's actual text content
            if (textContent.trim()) {
                cleanMessages.push({ role: 'assistant', content: textContent });
            }
        }
    }
    
    return cleanMessages;
}
