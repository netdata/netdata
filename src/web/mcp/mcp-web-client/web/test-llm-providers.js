#!/usr/bin/env node

/**
 * Unit tests for llm-providers.js message conversion
 * Tests all providers (OpenAI, Anthropic, Google) with various message formats
 */

import { strict as assert } from 'assert';
import { readFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import vm from 'vm';

// Get the directory of this script
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Create a mock browser-like environment
const mockWindow = {
    createLLMProvider: null
};

const mockDocument = {};

// Mock fetch for testing
const mockFetch = async () => {
    throw new Error('Network calls should be mocked in tests');
};

// Create a context with browser-like globals
const context = vm.createContext({
    window: mockWindow,
    document: mockDocument,
    fetch: mockFetch,
    console,
    setTimeout,
    clearTimeout,
    Date,
    JSON,
    Object,
    Array,
    String,
    Number,
    Boolean,
    Map,
    Set,
    Promise,
    Error,
    TypeError,
    ReferenceError
});

// Load and execute llm-providers.js in the context
const llmProvidersCode = readFileSync(join(__dirname, 'llm-providers.js'), 'utf8');

// Add class definitions to the global scope in the code
const wrappedCode = `
${llmProvidersCode}

// Export classes to global scope for testing
this.OpenAIProvider = OpenAIProvider;
this.AnthropicProvider = AnthropicProvider;
this.GoogleProvider = GoogleProvider;
this.MODEL_ENDPOINT_CONFIG = MODEL_ENDPOINT_CONFIG;
`;

vm.runInContext(wrappedCode, context);

// Extract the classes we need
const { OpenAIProvider, AnthropicProvider, GoogleProvider } = context;

// Run tests
(async () => {
    let testsPassed = 0;
    let testsFailed = 0;
    
    function test(name, fn) {
        try {
            fn();
            console.log(`✓ ${name}`);
            testsPassed++;
        } catch (error) {
            console.error(`✗ ${name}`);
            console.error(`  ${error.message}`);
            console.error(`  ${error.stack}`);
            testsFailed++;
        }
    }
    
    function deepEqual(actual, expected, message) {
        // Use JSON comparison for deep equality
        const actualJson = JSON.stringify(actual);
        const expectedJson = JSON.stringify(expected);
        
        if (actualJson !== expectedJson) {
            console.error('Actual:', JSON.stringify(actual, null, 2));
            console.error('Expected:', JSON.stringify(expected, null, 2));
            throw new Error(message || 'Objects are not deeply equal');
        }
    }
    
    console.log('Testing LLM Providers Message Conversion...\n');
    
    // Test data
    const testMessages = {
        // Basic conversation
        basic: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Hello!' },
            { role: 'assistant', content: 'Hi there! How can I help you?' }
        ],
        
        // Assistant with tool calls (Anthropic format)
        withToolCalls: [
            { role: 'system', content: 'You are a helpful weather assistant.' },
            { role: 'user', content: 'What is the weather?' },
            { 
                role: 'assistant', 
                content: [
                    { type: 'text', text: 'I\'ll check the weather for you.' },
                    { 
                        type: 'tool_use', 
                        id: 'tool_123',
                        name: 'get_weather',
                        input: { location: 'New York' }
                    }
                ]
            },
            {
                role: 'tool-results',
                toolResults: [{
                    toolCallId: 'tool_123',
                    toolName: 'get_weather',
                    result: 'Sunny, 72°F'
                }]
            },
            { role: 'assistant', content: 'The weather in New York is sunny and 72°F.' }
        ],
        
        // Edge case: assistant with array content but only text
        assistantArrayText: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Hi' },
            { 
                role: 'assistant', 
                content: [{ type: 'text', text: 'Hello!' }]
            }
        ],
        
        // OpenAI multi_tool_use format
        multiToolUseFormat: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Get infrastructure health summary' },
            { 
                role: 'assistant', 
                content: 'I\'ll gather that information for you.\n\n<multi_tool_use.parallel  tool_uses={[\n  {\n    recipient_name: "functions.list_nodes",\n    parameters: {}\n  },\n  {\n    recipient_name: "functions.list_alert_transitions",\n    parameters: {\n      after: "-604800", // 7 days ago\n      status: ["CRITICAL", "WARNING", "CLEAR"],\n      cardinality_limit: 300\n    }\n  }\n]}/>'
            }
        ],
        
        // OpenAI parallel format
        parallelFormat: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Get the weather and time' },
            { 
                role: 'assistant', 
                content: 'I\'ll check both for you.\n\n<|parallel|>\n{\n  "tool_uses": [\n    {\n      "recipient_name": "functions.get_weather",\n      "parameters": {\n        "location": "NYC"\n      }\n    },\n    {\n      "recipient_name": "functions.get_time",\n      "parameters": {\n        "timezone": "EST"\n      }\n    }\n  ]\n}\n</|parallel|>'
            }
        ],
        
        // Edge case: nested array in text (the bug we fixed)
        nestedArrayBug: [
            { role: 'system', content: 'You are a test assistant.' },
            { role: 'user', content: 'Test' },
            { 
                role: 'assistant', 
                content: [{
                    type: 'text',
                    text: [
                        { type: 'text', text: 'This is nested' },
                        { type: 'tool_use', id: 'ignored', name: 'test', input: {} }
                    ]
                }]
            }
        ],
        
        // Empty or null content
        emptyContent: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: '' },
            { role: 'assistant', content: null },
            { role: 'user', content: 'Continue' }
        ],
        
        // Multiple tool calls
        multipleTools: [
            { role: 'system', content: 'You are a helpful assistant with access to weather and time tools.' },
            { role: 'user', content: 'Get weather and time' },
            { 
                role: 'assistant', 
                content: [
                    { type: 'text', text: 'I\'ll check both for you.' },
                    { type: 'tool_use', id: 'tool_1', name: 'get_weather', input: { location: 'NYC' } },
                    { type: 'tool_use', id: 'tool_2', name: 'get_time', input: { timezone: 'EST' } }
                ]
            },
            {
                role: 'tool-results',
                toolResults: [
                    { toolCallId: 'tool_1', toolName: 'get_weather', result: 'Rainy' },
                    { toolCallId: 'tool_2', toolName: 'get_time', result: '3:30 PM' }
                ]
            }
        ],
        
        // System message with summary
        withSummary: [
            { role: 'summary', content: 'Previous conversation about coding.' },
            { role: 'system', content: 'You are a coding assistant.' },
            { role: 'user', content: 'Continue our discussion' }
        ]
    };
    
    // Test OpenAI Provider (standard completion)
    console.log('=== Testing OpenAI Provider (Completion API) ===\n');
    
    test('OpenAI: Basic conversation', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        const converted = provider.convertMessages(testMessages.basic);
        
        deepEqual(converted, [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Hello!' },
            { role: 'assistant', content: 'Hi there! How can I help you?' }
        ]);
    });
    
    test('OpenAI: Assistant with tool calls', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        const converted = provider.convertMessages(testMessages.withToolCalls);
        
        assert.equal(converted.length, 5); // system + user + assistant + tool + assistant
        assert.equal(converted[0].role, 'system');
        assert.equal(converted[0].content, 'You are a helpful weather assistant.');
        assert.equal(converted[1].role, 'user');
        assert.equal(converted[2].role, 'assistant');
        assert.equal(converted[2].content, 'I\'ll check the weather for you.');
        assert.equal(converted[2].tool_calls.length, 1);
        assert.equal(converted[2].tool_calls[0].id, 'tool_123');
        assert.equal(converted[2].tool_calls[0].type, 'function');
        assert.equal(converted[2].tool_calls[0].function.name, 'get_weather');
        assert.equal(converted[2].tool_calls[0].function.arguments, '{"location":"New York"}');
        assert.equal(converted[3].role, 'tool');
        assert.equal(converted[3].tool_call_id, 'tool_123');
        assert.equal(converted[4].role, 'assistant');
    });
    
    test('OpenAI: Assistant with array text only', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        const converted = provider.convertMessages(testMessages.assistantArrayText);
        
        deepEqual(converted, [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Hi' },
            { role: 'assistant', content: 'Hello!' }
        ]);
    });
    
    test('OpenAI: Empty content handling', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        const converted = provider.convertMessages(testMessages.emptyContent);
        
        deepEqual(converted, [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: '' },
            { role: 'assistant', content: null },
            { role: 'user', content: 'Continue' }
        ]);
    });
    
    test('OpenAI: Multiple tool calls', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        const converted = provider.convertMessages(testMessages.multipleTools);
        
        // system[0] + user[1] + assistant with tools[2] + tool results[3,4]
        assert.equal(converted.length, 5);
        assert.equal(converted[2].role, 'assistant');
        assert.equal(converted[2].tool_calls.length, 2);
        assert.equal(converted[2].tool_calls[0].function.name, 'get_weather');
        assert.equal(converted[2].tool_calls[1].function.name, 'get_time');
        // Should have 2 tool result messages
        assert.equal(converted.filter(m => m.role === 'tool').length, 2);
    });
    
    test('OpenAI: Parallel format parsing', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        
        // Test parsing legacy tool calls
        const content = testMessages.parallelFormat[2].content;
        const toolCalls = provider.parseLegacyToolCalls(content);
        
        // Should have extracted 2 tool calls
        assert.equal(toolCalls.length, 2);
        assert.equal(toolCalls[0].name, 'get_weather');
        deepEqual(toolCalls[0].arguments, { location: 'NYC' });
        assert.equal(toolCalls[1].name, 'get_time');
        deepEqual(toolCalls[1].arguments, { timezone: 'EST' });
        
        // Test cleaning content
        const cleaned = provider.cleanContentFromToolCalls(content);
        assert.equal(cleaned.includes('<|parallel|>'), false);
        assert.equal(cleaned.includes('tool_uses'), false);
        assert.equal(cleaned.trim(), 'I\'ll check both for you.');
    });
    
    test('OpenAI: multi_tool_use format parsing', () => {
        const provider = new OpenAIProvider('http://localhost', 'gpt-4');
        
        // Test parsing multi_tool_use format
        const content = testMessages.multiToolUseFormat[2].content;
        const toolCalls = provider.parseLegacyToolCalls(content);
        
        // Should have extracted 2 tool calls
        assert.equal(toolCalls.length, 2);
        assert.equal(toolCalls[0].name, 'list_nodes');
        deepEqual(toolCalls[0].arguments, {});
        assert.equal(toolCalls[1].name, 'list_alert_transitions');
        deepEqual(toolCalls[1].arguments, {
            after: '-604800',
            status: ['CRITICAL', 'WARNING', 'CLEAR'],
            cardinality_limit: 300
        });
        
        // Test cleaning content
        const cleaned = provider.cleanContentFromToolCalls(content);
        assert.equal(cleaned.includes('<multi_tool_use.parallel'), false);
        assert.equal(cleaned.includes('tool_uses'), false);
        assert.equal(cleaned.trim(), 'I\'ll gather that information for you.');
    });
    
    // Test OpenAI Provider (responses endpoint - o1/o3 models)
    console.log('\n=== Testing OpenAI Provider (Response API - o1/o3) ===\n');
    
    test('OpenAI Response API: Basic conversation', () => {
        const provider = new OpenAIProvider('http://localhost', 'o1-preview');
        const converted = provider.convertMessages(testMessages.basic);
        
        // Response API returns messages as-is for further processing
        deepEqual(converted, testMessages.basic);
    });
    
    test('OpenAI Response API: Array content extraction', () => {
        const provider = new OpenAIProvider('http://localhost', 'o1-preview');
        // The sendMessage method would process this, but convertMessages returns as-is
        const converted = provider.convertMessages(testMessages.assistantArrayText);
        deepEqual(converted, testMessages.assistantArrayText);
    });
    
    test('OpenAI: o3 tool call format processing', () => {
        const _provider = new OpenAIProvider('http://localhost', 'gpt-4');
        
        // Simulate o3 response processing
        const o3Response = {
            choices: [{
                message: {
                    content: null,
                    tool_calls: [
                        {
                            id: 'call_RvhlCCHxOOD1r5zwAvEqrwnW',
                            name: 'list_nodes',
                            arguments: {after: -604800, nodes: '*'}
                        }
                    ]
                }
            }]
        };
        
        // Process the response (simulating what parseResponse does)
        const choice = o3Response.choices[0];
        const contentArray = [];
        
        if (choice.message.content) {
            contentArray.push({ type: 'text', text: choice.message.content });
        }
        
        if (choice.message.tool_calls) {
            for (const tc of choice.message.tool_calls) {
                // This should not throw an error even without tc.function
                let toolCallId, toolCallName, toolCallArgs;
                
                if (tc.function) {
                    toolCallId = tc.id;
                    toolCallName = tc.function.name;
                    toolCallArgs = tc.function.arguments;
                } else {
                    toolCallId = tc.id;
                    toolCallName = tc.name;
                    toolCallArgs = tc.arguments;
                }
                
                let parsedArgs;
                if (typeof toolCallArgs === 'string') {
                    try {
                        parsedArgs = JSON.parse(toolCallArgs);
                    } catch (_e) {
                        parsedArgs = {};
                    }
                } else {
                    parsedArgs = toolCallArgs || {};
                }
                
                contentArray.push({
                    type: 'tool_use',
                    id: toolCallId,
                    name: toolCallName,
                    input: parsedArgs
                });
            }
        }
        
        // Verify the result
        assert.equal(contentArray.length, 1);
        assert.equal(contentArray[0].type, 'tool_use');
        assert.equal(contentArray[0].id, 'call_RvhlCCHxOOD1r5zwAvEqrwnW');
        assert.equal(contentArray[0].name, 'list_nodes');
        deepEqual(contentArray[0].input, {after: -604800, nodes: '*'});
    });
    
    test('OpenAI Response API: Null content handling for o3', () => {
        const provider = new OpenAIProvider('http://localhost', 'o3-2025-04-16');
        
        // Test messages with null content
        const messagesWithNull = [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: null },
            { role: 'assistant', content: [
                { type: 'tool_use', id: 'tool_1', name: 'test_tool', input: {} }
            ]},
            { role: 'tool', tool_call_id: 'tool_1', content: null }
        ];
        
        // Mock the sendMessage method to capture the request
        let _capturedRequest;
        const originalFetch = global.fetch;
        global.fetch = async (url, options) => {
            capturedRequest = JSON.parse(options.body);
            return { ok: true, json: async () => ({ output: [] }) };
        };
        
        // Convert messages (simulating what sendMessage does)
        const openaiMessages = provider.convertMessages(messagesWithNull);
        
        // Build input messages for o3 (simulating sendMessage logic)
        const inputMessages = [];
        for (const msg of openaiMessages) {
            if (msg.role === 'system') continue;
            
            if (msg.role === 'user') {
                inputMessages.push({
                    role: 'user',
                    content: msg.content || ''
                });
            } else if (msg.role === 'assistant') {
                let textContent = msg.content;
                if (Array.isArray(msg.content)) {
                    const textBlocks = msg.content.filter(block => block.type === 'text');
                    textContent = textBlocks.map(block => block.text || '').join('\n\n').trim() || '';
                }
                if (textContent === null || textContent === undefined) {
                    textContent = '';
                }
                inputMessages.push({
                    role: 'assistant',
                    content: textContent
                });
            } else if (msg.role === 'tool') {
                inputMessages.push({
                    role: 'tool',
                    tool_call_id: msg.tool_call_id,
                    content: msg.content || ''
                });
            }
        }
        
        // Verify no null content
        assert.equal(inputMessages.length, 3); // user, assistant, tool
        assert.equal(inputMessages[0].role, 'user');
        assert.equal(inputMessages[0].content, ''); // null converted to empty string
        assert.equal(inputMessages[1].role, 'assistant');
        assert.equal(inputMessages[1].content, ''); // no text blocks, converted to empty string
        assert.equal(inputMessages[2].role, 'tool');
        assert.equal(inputMessages[2].content, ''); // null converted to empty string
        
        // Restore global fetch
        global.fetch = originalFetch;
    });
    
    // Test Anthropic Provider
    console.log('\n=== Testing Anthropic Provider ===\n');
    
    test('Anthropic: Basic conversation', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const result = provider.convertMessagesWithCaching(testMessages.basic);
        const converted = result.converted;
        
        // Anthropic filters out system messages and converts them differently
        assert.equal(converted.length, 2);
        assert.equal(converted[0].role, 'user');
        assert.equal(converted[0].content[0].type, 'text');
        assert.equal(converted[0].content[0].text, 'Hello!');
        assert.equal(converted[1].role, 'assistant');
        assert.equal(converted[1].content[0].type, 'text');
        assert.equal(converted[1].content[0].text, 'Hi there! How can I help you?');
    });
    
    test('Anthropic: Tool calls preserved', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const result = provider.convertMessagesWithCaching(testMessages.withToolCalls);
        const converted = result.converted;
        
        assert.equal(converted.length, 4);
        assert.equal(converted[0].role, 'user');
        assert.equal(converted[1].role, 'assistant');
        assert.equal(converted[1].content.length, 2);
        assert.equal(converted[1].content[0].type, 'text');
        assert.equal(converted[1].content[1].type, 'tool_use');
        assert.equal(converted[2].role, 'user'); // Tool results go in user message
        assert.equal(converted[2].content[0].type, 'tool_result');
        assert.equal(converted[3].role, 'assistant');
    });
    
    test('Anthropic: Nested array bug handling', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const result = provider.convertMessagesWithCaching(testMessages.nestedArrayBug);
        const converted = result.converted;
        
        // Should flatten the nested array
        assert.equal(converted[1].content[0].type, 'text');
        assert.equal(converted[1].content[0].text, 'This is nested');
    });
    
    test('Anthropic: Empty content handling', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const result = provider.convertMessagesWithCaching(testMessages.emptyContent);
        const converted = result.converted;
        
        assert.equal(converted.length, 2); // Empty assistant message filtered out
        assert.equal(converted[0].content[0].text, '');
        assert.equal(converted[1].content[0].text, 'Continue');
    });
    
    test('Anthropic: Cache control positioning', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const messages = [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'First' },
            { role: 'assistant', content: 'Second' },
            { role: 'user', content: 'Third' }
        ];
        
        // Test with specific cache position
        const cachedResult = provider.convertMessagesWithCaching(messages, 1);
        const cached = cachedResult.converted;
        assert.equal(cached[1].content[0].cache_control?.type, 'ephemeral');
        
        // Test with default (last position)
        const defaultCachedResult = provider.convertMessagesWithCaching(messages);
        const defaultCached = defaultCachedResult.converted;
        const lastMsg = defaultCached[defaultCached.length - 1];
        const lastContent = lastMsg.content[lastMsg.content.length - 1];
        assert.equal(lastContent.cache_control?.type, 'ephemeral');
    });
    
    // Test Google Provider
    console.log('\n=== Testing Google Provider ===\n');
    
    test('Google: Basic conversation', () => {
        const provider = new GoogleProvider('http://localhost', 'gemini-pro');
        const { contents } = provider.convertMessages(testMessages.basic);
        
        deepEqual(contents, [
            {
                role: 'user',
                parts: [{ text: 'Hello!' }]
            },
            {
                role: 'model',
                parts: [{ text: 'Hi there! How can I help you?' }]
            }
        ]);
    });
    
    test('Google: Tool calls conversion', () => {
        const provider = new GoogleProvider('http://localhost', 'gemini-pro');
        const { contents } = provider.convertMessages(testMessages.withToolCalls);
        
        assert.equal(contents.length, 4);
        assert.equal(contents[1].role, 'model');
        assert.equal(contents[1].parts.length, 2); // text + functionCall
        assert.equal(contents[1].parts[1].functionCall.name, 'get_weather');
        assert.equal(contents[2].role, 'user');
        assert.equal(contents[2].parts[0].functionResponse.name, 'get_weather');
    });
    
    test('Google: System message handling', () => {
        const provider = new GoogleProvider('http://localhost', 'gemini-pro');
        const { contents, systemInstruction } = provider.convertMessages(testMessages.basic);
        
        assert.equal(systemInstruction, 'You are a helpful assistant.');
        assert.equal(contents[0].parts[0].text, 'Hello!'); // No system in contents
    });
    
    test('Google: Empty content filtering', () => {
        const provider = new GoogleProvider('http://localhost', 'gemini-pro');
        const { contents } = provider.convertMessages(testMessages.emptyContent);
        
        // Google includes user messages even with empty content
        assert.equal(contents.length, 2);
        assert.equal(contents[0].parts[0].text, '');
        assert.equal(contents[1].parts[0].text, 'Continue');
    });
    
    test('Google: Orphaned tool response detection', () => {
        const provider = new GoogleProvider('http://localhost', 'gemini-pro');
        
        // Create messages with orphaned tool response
        const orphanedMessages = [
            { role: 'user', content: 'Test' },
            {
                role: 'tool-results',
                toolResults: [{
                    toolCallId: 'orphan',
                    toolName: 'orphaned_tool',
                    result: 'This should fail'
                }]
            }
        ];
        
        assert.throws(() => {
            provider.convertMessages(orphanedMessages);
        }, /Tool "orphaned_tool" response found without a preceding function call/);
    });
    
    // Test Anthropic convertMessages (non-caching version)
    console.log('\n=== Testing Anthropic convertMessages (non-caching) ===\n');
    
    test('Anthropic convertMessages: Tool calls handling', () => {
        const provider = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const result = provider.convertMessages(testMessages.withToolCalls);
        const { messages, system } = result;
        
        // System should be extracted
        assert.equal(system, 'You are a helpful weather assistant.');
        
        // Should have user, assistant with tools, tool results as user, final assistant
        assert.equal(messages.length, 4);
        assert.equal(messages[0].role, 'user');
        assert.equal(messages[0].content[0].text, 'What is the weather?');
        
        // Assistant message should have tool_use blocks
        assert.equal(messages[1].role, 'assistant');
        assert.equal(messages[1].content.length, 2); // text + tool_use
        assert.equal(messages[1].content[0].type, 'text');
        assert.equal(messages[1].content[0].text, 'I\'ll check the weather for you.');
        assert.equal(messages[1].content[1].type, 'tool_use');
        assert.equal(messages[1].content[1].id, 'tool_123');
        assert.equal(messages[1].content[1].name, 'get_weather');
        deepEqual(messages[1].content[1].input, { location: 'New York' });
        
        // Tool results as user message
        assert.equal(messages[2].role, 'user');
        assert.equal(messages[2].content[0].type, 'tool_result');
        
        // Final assistant
        assert.equal(messages[3].role, 'assistant');
    });
    
    // Test edge cases
    console.log('\n=== Testing Edge Cases ===\n');
    
    test('Handle undefined/null messages array', () => {
        const openai = new OpenAIProvider('http://localhost', 'gpt-4');
        
        assert.throws(() => {
            openai.convertMessages(null);
        });
        
        assert.throws(() => {
            openai.convertMessages(undefined);
        });
    });
    
    test('Handle messages with missing role', () => {
        const anthropic = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const badMessages = [
            { content: 'No role specified' }
        ];
        
        // Should be filtered out
        const result = anthropic.convertMessagesWithCaching(badMessages);
        const converted = result.converted;
        assert.equal(converted.length, 0);
    });
    
    test('Summary message handling', () => {
        const anthropic = new AnthropicProvider('http://localhost', 'claude-3-opus-20240229');
        const messages = testMessages.withSummary;
        
        // Summary should be captured but not included in converted messages
        const result = anthropic.convertMessagesWithCaching(messages);
        const converted = result.converted;
        assert.equal(converted.length, 1); // Only user message
        assert.equal(converted[0].content[0].text, 'Continue our discussion');
    });
    
    test('Tool result formatting', () => {
        const openai = new OpenAIProvider('http://localhost', 'gpt-4');
        
        // Test formatToolResponse method
        const formatted = openai.formatToolResponse('tool_123', 'Result data', 'test_tool');
        deepEqual(formatted, {
            role: 'tool',
            tool_call_id: 'tool_123',
            content: 'Result data'
        });
        
        // Test with complex result
        const complexResult = { status: 'ok', data: { value: 42 } };
        const complexFormatted = openai.formatToolResponse('tool_456', complexResult, 'complex_tool');
        assert.equal(complexFormatted.content, JSON.stringify(complexResult, null, 2));
    });
    
    console.log('\n=== Test Summary ===');
    console.log(`Tests passed: ${testsPassed}`);
    console.log(`Tests failed: ${testsFailed}`);
    
    if (testsFailed > 0) {
        process.exit(1);
    }
})();