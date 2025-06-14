/**
 * Unit Tests for MessageOptimizer
 * 
 * Comprehensive test suite covering all code paths and edge cases.
 * Run with: node message-optimizer.test.js
 */

import { MessageOptimizer } from './message-optimizer.js';

class TestRunner {
    constructor() {
        this.passed = 0;
        this.failed = 0;
        this.tests = [];
    }

    test(name, fn) {
        this.tests.push({ name, fn });
    }

    async run() {
        console.log('🧪 Running MessageOptimizer Tests\n');
        
        for (const { name, fn } of this.tests) {
            try {
                // eslint-disable-next-line no-await-in-loop
                await fn();
                console.log(`✅ ${name}`);
                this.passed++;
            } catch (error) {
                console.log(`❌ ${name}`);
                console.log(`   Error: ${error.message}`);
                this.failed++;
            }
        }
        
        console.log(`\n📊 Results: ${this.passed} passed, ${this.failed} failed`);
        
        if (this.failed > 0) {
            process.exit(1);
        }
    }

    assertEqual(actual, expected, message = '') {
        if (JSON.stringify(actual) !== JSON.stringify(expected)) {
            throw new Error(`${message}\nExpected: ${JSON.stringify(expected)}\nActual: ${JSON.stringify(actual)}`);
        }
    }

    assertThrows(fn, expectedMessage = '') {
        try {
            fn();
            throw new Error(`Expected function to throw${expectedMessage ? ': ' + expectedMessage : ''}`);
        } catch (error) {
            if (expectedMessage && !error.message.includes(expectedMessage)) {
                throw new Error(`Expected error message to contain "${expectedMessage}", got: ${error.message}`);
            }
        }
    }

    assertTrue(value, message = '') {
        if (!value) {
            throw new Error(`Expected true, got false. ${message}`);
        }
    }

    assertFalse(value, message = '') {
        if (value) {
            throw new Error(`Expected false, got true. ${message}`);
        }
    }
}

const test = new TestRunner();

// Test data factories
function createBasicChat() {
    return {
        id: 'test-chat',
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'Hello' },
            { role: 'assistant', content: 'Hi there!' }
        ],
        toolInclusionMode: 'auto',
        currentTurn: 0
    };
}

function createChatWithTools() {
    return {
        id: 'test-chat-tools',
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' },
            { role: 'user', content: 'List files' },
            { 
                role: 'assistant', 
                content: [
                    { type: 'text', text: 'I\'ll list the files for you.' },
                    { type: 'tool_use', id: 'call_1', name: 'list_files', input: {} }
                ]
            },
            {
                role: 'tool-results',
                toolResults: [
                    { toolCallId: 'call_1', toolName: 'list_files', result: { files: ['a.txt', 'b.txt'] } }
                ]
            },
            { role: 'assistant', content: 'Here are the files: a.txt, b.txt. Task completed!' }
        ]
    };
}

function createDefaultSettings() {
    return {
        model: {
            provider: 'anthropic',
            id: 'claude-3-sonnet',
            params: {
                temperature: 0.7,
                topP: 0.9,
                maxTokens: 4096,
                seed: { enabled: false, value: 123456 }
            }
        },
        optimisation: {
            toolSummarisation: { enabled: false, thresholdKiB: 20, model: null },
            toolMemory: { enabled: false, forgetAfterConclusions: 1 },
            cacheControl: { enabled: false, strategy: 'smart' },
            autoSummarisation: { enabled: false, triggerPercent: 50, model: null },
            titleGeneration: { enabled: true, model: null }
        },
        mcpServer: null
    };
}

// Constructor Tests
test.test('Constructor - Valid settings', () => {
    const settings = createDefaultSettings();
    const optimizer = new MessageOptimizer(settings);
    test.assertEqual(optimizer.settings.model.id, 'claude-3-sonnet');
    test.assertEqual(optimizer.settings.model.provider, 'anthropic');
});

test.test('Constructor - Invalid settings type', () => {
    test.assertThrows(() => new MessageOptimizer(null), '[MessageOptimizer] settings must be a valid object');
    test.assertThrows(() => new MessageOptimizer('invalid'), '[MessageOptimizer] settings must be a valid object');
});

test.test('Constructor - Invalid model structure', () => {
    const settings = { model: { provider: 'anthropic' } }; // Missing id
    test.assertThrows(() => new MessageOptimizer(settings), '[MessageOptimizer] model must have provider and id');
});

test.test('Constructor - Valid null model in optimisation', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.model = null;
    const optimizer = new MessageOptimizer(settings);
    test.assertEqual(optimizer.settings.optimisation.toolSummarisation.model, null);
});

test.test('Constructor - Invalid tool summarization threshold', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.thresholdKiB = -100;
    test.assertThrows(() => new MessageOptimizer(settings), '[MessageOptimizer] toolSummarisation.thresholdKiB must be positive number');
});

test.test('Constructor - Invalid tool memory conclusions', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.forgetAfterConclusions = 10;
    test.assertThrows(() => new MessageOptimizer(settings), '[MessageOptimizer] toolMemory.forgetAfterConclusions must be 0-5');
});

test.test('Constructor - Invalid cache strategy', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.strategy = 'invalid';
    test.assertThrows(() => new MessageOptimizer(settings), '[MessageOptimizer] cacheControl.strategy must be one of');
});

// buildMessagesForAPI Input Validation Tests
test.test('buildMessagesForAPI - Null chat', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    test.assertThrows(() => optimizer.buildMessagesForAPI(null), '[MessageOptimizer.buildMessagesForAPI] chat must be a valid object');
});

test.test('buildMessagesForAPI - Missing messages array', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = { id: 'test' };
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), '[MessageOptimizer.buildMessagesForAPI] chat.messages must be an array');
});

test.test('buildMessagesForAPI - Empty messages array', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = { messages: [] };
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), '[MessageOptimizer.buildMessagesForAPI] chat.messages cannot be empty');
});

test.test('buildMessagesForAPI - Invalid freezeCache type', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat, 'invalid'), '[MessageOptimizer.buildMessagesForAPI] freezeCache must be boolean');
});

// Message Processing Tests
test.test('Basic message processing', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.messages.length, 3);
    test.assertEqual(result.messages[0].role, 'system');
    test.assertEqual(result.messages[1].role, 'user');
    test.assertEqual(result.messages[2].role, 'assistant');
    test.assertEqual(result.toolInclusionMode, 'auto');
});

test.test('Message validation - Invalid message structure', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            null, // Invalid message
            { role: 'user', content: 'Hello' }
        ]
    };
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), '[MessageOptimizer.buildMessagesForAPI] Message at index 0 is not a valid object');
});

test.test('Message validation - Missing role and type', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { content: 'Hello' } // Missing role/type
        ]
    };
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), '[MessageOptimizer] Message at index 0 missing both role and type');
});

test.test('Message validation - tool-results missing toolResults', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'tool-results' } // Missing toolResults array
        ]
    };
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), '[MessageOptimizer] tool-results message at index 1 missing toolResults array');
});

// Skip Message Tests
test.test('Skip system roles and UI messages', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Hello' },
            { role: 'system-title', content: 'Generate title' }, // Should be skipped
            { role: 'title', content: 'Test Title' }, // Should be skipped
            { role: 'error', content: 'Error message' }, // Should be skipped
            { type: 'tool-summary-request', content: 'Summarizing...' }, // Should be skipped
            { role: 'assistant', content: 'Response' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should only have system, user, and assistant
    test.assertEqual(result.messages.length, 3);
    test.assertEqual(result.messages[0].role, 'system');
    test.assertEqual(result.messages[1].role, 'user');
    test.assertEqual(result.messages[2].role, 'assistant');
});

// Summary Checkpoint Tests
test.test('Summary checkpoint detection', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Old message' },
            { role: 'assistant', content: 'Old response' },
            { role: 'summary', content: 'This is a summary of previous conversation.' },
            { role: 'user', content: 'New message' },
            { role: 'assistant', content: 'New response' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should include: system, summary, new user, new assistant
    test.assertEqual(result.messages.length, 4);
    test.assertEqual(result.messages[0].role, 'system');
    test.assertEqual(result.messages[1].role, 'summary');
    test.assertEqual(result.messages[2].role, 'user');
    test.assertEqual(result.messages[2].content, 'New message');
    test.assertEqual(result.messages[3].role, 'assistant');
});

// Tool Memory Tests  
test.test('Tool memory - disabled', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = false;
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createChatWithTools();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // All messages should be included
    test.assertTrue(result.messages.some(m => m.role === 'tool-results'));
});

test.test('Tool memory - enabled but no conclusions', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 1;
    
    const optimizer = new MessageOptimizer(settings);
    
    // Chat without conclusions (assistant still using tools)
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'List files' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'list', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: {} }] },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 'list', input: {} }] }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Tools should still be included (no conclusion - assistant still using tools)
    test.assertTrue(result.messages.some(m => m.role === 'tool-results'));
});

// Cache Control Tests
test.test('Cache control - disabled', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = false;
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.cacheControlIndex, -1);
});

test.test('Cache control - smart strategy', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = true;
    settings.optimisation.cacheControl.strategy = 'smart';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should cache up to 70% of messages
    const expectedIndex = Math.floor(result.messages.length * 0.7);
    test.assertTrue(result.cacheControlIndex <= expectedIndex);
});

test.test('Cache control - aggressive strategy', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = true;
    settings.optimisation.cacheControl.strategy = 'aggressive';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should cache all but last message
    test.assertEqual(result.cacheControlIndex, result.messages.length - 2);
});

test.test('Cache control - minimal strategy', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = true;
    settings.optimisation.cacheControl.strategy = 'minimal';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should only cache system prompt
    test.assertEqual(result.cacheControlIndex, 0);
});

test.test('Cache control - freeze cache', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = true;
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    chat.lastCacheControlIndex = 1;
    
    const result = optimizer.buildMessagesForAPI(chat, true);
    
    // Should use frozen cache index
    test.assertEqual(result.cacheControlIndex, 1);
});

// AssistantStateTracker Tests
test.test('AssistantStateTracker - tool usage detection', () => {
    const settings = createDefaultSettings();
    const optimizer = new MessageOptimizer(settings);
    
    // Access the tracker indirectly by testing behavior
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Hello' },
            { 
                role: 'assistant', 
                content: [{ type: 'tool_use', id: 'call_1', name: 'test', input: {} }]
            }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should process without errors
    test.assertTrue(result.messages.length > 0);
});

test.test('AssistantStateTracker - conclusion detection', () => {
    const settings = createDefaultSettings();
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Hello' },
            { role: 'assistant', content: 'Task completed successfully!' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should process without errors
    test.assertTrue(result.messages.length > 0);
});

// Statistics Tests
test.test('Statistics collection', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertTrue(typeof result.stats === 'object');
    test.assertTrue(typeof result.stats.originalMessages === 'number');
    test.assertTrue(typeof result.stats.optimizedMessages === 'number');
    test.assertTrue(result.stats.originalMessages > 0);
    test.assertTrue(result.stats.optimizedMessages > 0);
});

// MCP Instructions Tests
test.test('buildMessagesForAPI with MCP instructions', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    const mcpInstructions = 'These are MCP server instructions.';
    
    const result = optimizer.buildMessagesForAPI(chat, false, mcpInstructions);
    
    // System message should be enhanced with MCP instructions
    test.assertTrue(result.messages[0].content.includes(mcpInstructions));
});

// Edge Cases
test.test('Edge case - single system message', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.messages.length, 1);
    test.assertEqual(result.messages[0].role, 'system');
});

test.test('Edge case - no system message', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'user', content: 'Hello' },
            { role: 'assistant', content: 'Hi!' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.messages.length, 2);
    test.assertEqual(result.messages[0].role, 'user');
});

test.test('Edge case - message with both type and role', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'user', type: 'user', content: 'Hello' },
            { role: 'assistant', type: 'assistant', content: 'Hi!' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.messages.length, 2);
});

// Tool Summarization Tests
test.test('Tool summarization - Disabled by default', () => {
    const settings = createDefaultSettings();
    const optimizer = new MessageOptimizer(settings);
    test.assertEqual(optimizer.toolSummarizer, null);
});

test.test('Tool summarization - Enabled requires llmProviderFactory', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = true;
    
    test.assertThrows(
        () => new MessageOptimizer(settings),
        '[MessageOptimizer] llmProviderFactory required when tool summarisation is enabled'
    );
});

test.test('Tool summarization - Creates summarizer when enabled', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = true;
    settings.llmProviderFactory = () => {}; // Mock factory
    
    const optimizer = new MessageOptimizer(settings);
    test.assertTrue(optimizer.toolSummarizer !== null);
});

test.test('Tool summarization - Marks large results', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = true;
    settings.optimisation.toolSummarisation.thresholdKiB = 0.1; // 100 bytes for testing
    settings.llmProviderFactory = () => {};
    
    const optimizer = new MessageOptimizer(settings);
    const largeResult = 'x'.repeat(200); // 200 bytes
    
    const msg = {
        role: 'tool-results',
        toolResults: [
            { toolCallId: 'call_1', toolName: 'test', result: largeResult },
            { toolCallId: 'call_2', toolName: 'test2', result: 'small' }
        ]
    };
    
    const stats = { toolsSummarized: 0 };
    const processed = optimizer.maybeSummarizeToolResults(msg, stats);
    
    test.assertEqual(stats.toolsSummarized, 1);
    test.assertTrue(processed._hasPendingSummarization);
    test.assertTrue(processed.toolResults[0]._needsSummarization);
    test.assertFalse(processed.toolResults[1]._needsSummarization === true);
});

// MCP Instructions Edge Cases
test.test('buildMessagesForAPI with null MCP instructions', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat, false, null);
    
    // System message should not be modified
    test.assertEqual(result.messages[0].content, chat.messages[0].content);
});

test.test('buildMessagesForAPI with empty MCP instructions', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat, false, '');
    
    // System message should not be modified
    test.assertEqual(result.messages[0].content, chat.messages[0].content);
});

test.test('MCP instructions with summary checkpoint', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test system prompt' },
            { role: 'user', content: 'Old message' },
            { role: 'summary', content: 'Summary of previous conversation' },
            { role: 'user', content: 'New message' }
        ]
    };
    const mcpInstructions = 'MCP server instructions';
    
    const result = optimizer.buildMessagesForAPI(chat, false, mcpInstructions);
    
    // System message should include MCP instructions
    test.assertTrue(result.messages[0].content.includes(mcpInstructions));
    test.assertTrue(result.messages[0].content.includes('Test system prompt'));
});

// Tool Memory Advanced Scenarios
test.test('Tool memory - filters after multiple conclusions', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 0; // Filter immediately
    
    const optimizer = new MessageOptimizer(settings);
    
    // Create a scenario where tool filtering is guaranteed
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Task' },
            { role: 'assistant', content: 'Working on it' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'tool1', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: 'result1' }] },
            { role: 'assistant', content: 'Task completed successfully!' }, // Conclusion
            // New user message resets the state
            { role: 'user', content: 'Another task' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 'tool2', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: 'result2' }] },
            { role: 'assistant', content: 'This is also done!' } // Another conclusion
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With forgetAfterConclusions = 0, all tools from previous turns should be filtered
    // Turn 0 had call_1 and result1, these should be filtered when processing turn 1
    test.assertTrue(result.stats.toolsFiltered >= 2);
    
    // Check that tool-results were filtered
    const toolResults = result.messages.filter(m => m.role === 'tool-results');
    test.assertEqual(toolResults.length, 1); // Only the last one should remain
    
    // Check that tool calls were removed from assistant messages in old turns
    const assistantMessages = result.messages.filter(m => m.role === 'assistant');
    const toolCallMessages = assistantMessages.filter(m => 
        Array.isArray(m.content) && m.content.some(c => c.type === 'tool_use')
    );
    test.assertEqual(toolCallMessages.length, 1); // Only the last tool call should remain
});

test.test('Tool memory - conclusion detection patterns', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 1;
    
    const optimizer = new MessageOptimizer(settings);
    
    // Simpler test case with clearer conclusion pattern
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'First task' },
            { role: 'assistant', content: 'Let me help with that' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'check', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: 'data' }] },
            { role: 'assistant', content: 'Task successfully completed!' }, // Clear conclusion
            { role: 'assistant', content: 'Now let me do more' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 'check2', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: 'data2' }] },
            { role: 'assistant', content: 'All done!' } // Another conclusion
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With forgetAfterConclusions = 1:
    // First set of tools (call_1) should be kept since we're only 1 turn away
    // Both tool results should be present
    const toolResults = result.messages.filter(m => m.role === 'tool-results');
    test.assertEqual(toolResults.length, 2);
    
    // Both tool calls should be present in assistant messages
    const assistantMessages = result.messages.filter(m => m.role === 'assistant');
    const toolCallMessages = assistantMessages.filter(m => 
        Array.isArray(m.content) && m.content.some(c => c.type === 'tool_use')
    );
    test.assertEqual(toolCallMessages.length, 2);
    
    // No tools should be filtered with threshold of 1
    test.assertEqual(result.stats.toolsFiltered, 0);
});

// Test rolling window behavior
test.test('Tool memory - rolling window across multiple turns', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 1;
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Start' },
            // Turn 0
            { role: 'assistant', content: [{ type: 'tool_use', id: 'tool_0', name: 'search', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'tool_0', result: 'turn 0 data' }] },
            { role: 'assistant', content: 'Turn 0 complete' }, // Conclusion -> Turn 1
            // Turn 1
            { role: 'assistant', content: [{ type: 'tool_use', id: 'tool_1', name: 'fetch', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'tool_1', result: 'turn 1 data' }] },
            { role: 'assistant', content: 'Turn 1 complete' }, // Conclusion -> Turn 2
            // Turn 2
            { role: 'assistant', content: [{ type: 'tool_use', id: 'tool_2', name: 'analyze', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'tool_2', result: 'turn 2 data' }] },
            { role: 'assistant', content: 'Turn 2 complete' }, // Conclusion -> Turn 3
            // Now in turn 3
            { role: 'user', content: 'What have you found?' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With forgetAfterConclusions = 1, in turn 3:
    // - Turn 0 tools: (3 - 0 = 3) > 1 -> FILTERED
    // - Turn 1 tools: (3 - 1 = 2) > 1 -> FILTERED  
    // - Turn 2 tools: (3 - 2 = 1) <= 1 -> KEPT
    
    // Should have filtered 4 items (2 tool calls + 2 tool results from turns 0 and 1)
    test.assertEqual(result.stats.toolsFiltered, 4);
    
    // Only turn 2 tools should remain
    const toolResults = result.messages.filter(m => m.role === 'tool-results');
    test.assertEqual(toolResults.length, 1);
    test.assertTrue(toolResults[0].toolResults[0].result === 'turn 2 data');
    
    // Only turn 2 tool call should remain in assistant messages
    const assistantWithTools = result.messages.filter(m => 
        m.role === 'assistant' && 
        Array.isArray(m.content) && 
        m.content.some(c => c.type === 'tool_use')
    );
    test.assertEqual(assistantWithTools.length, 1);
    test.assertTrue(assistantWithTools[0].content[0].id === 'tool_2');
});

// New Tool Memory Rolling Window Tests
test.test('Tool memory - forgetAfterConclusions = 0 (immediate filtering)', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 0;
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'First question' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'weather', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: '72F sunny' }] },
            { role: 'assistant', content: 'It is 72F and sunny' }, // Turn 0 ends
            { role: 'user', content: 'What about tomorrow?' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 'weather', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: '68F cloudy' }] },
            { role: 'assistant', content: 'Tomorrow will be 68F' } // Turn 1 ends
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    
    // With forgetAfterConclusions = 0, Turn 0 tools should be filtered when we're in Turn 1
    test.assertTrue(!result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '72F sunny'
    ), 'Turn 0 tools should be filtered');
    
    // But Turn 1 tools should still be present
    test.assertTrue(result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '68F cloudy'
    ), 'Turn 1 tools should be present');
    
    test.assertEqual(result.stats.toolsFiltered, 2); // 1 tool call + 1 tool result from Turn 0
});

// Test with string content for assistant messages
test.test('Tool memory - with string content assistant messages', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 0;
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'First question' },
            // Assistant with tool call but string content (common in actual usage)
            { role: 'assistant', content: 'Let me check the weather' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'weather', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: '72F sunny' }] },
            { role: 'assistant', content: 'It is 72F and sunny' }, // Turn 0 ends
            { role: 'user', content: 'What about tomorrow?' },
            { role: 'assistant', content: 'Checking tomorrow\'s weather' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 'weather', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: '68F cloudy' }] },
            { role: 'assistant', content: 'Tomorrow will be 68F' } // Turn 1 ends
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Turn 0 tools should be filtered
    test.assertTrue(!result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '72F sunny'
    ), 'Turn 0 tools should be filtered');
    
    // Turn 1 tools should be present
    test.assertTrue(result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '68F cloudy'
    ), 'Turn 1 tools should be present');
});

// Test mixed content format (text + tool calls in same message)
test.test('Tool memory - mixed content format', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 0;
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'First question' },
            // Mixed content: text + tool call in same message
            { role: 'assistant', content: [
                { type: 'text', text: 'Let me check the weather' },
                { type: 'tool_use', id: 'call_1', name: 'weather', input: {} }
            ]},
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: '72F sunny' }] },
            { role: 'assistant', content: 'It is 72F and sunny' }, // Turn 0 ends
            { role: 'user', content: 'What about tomorrow?' },
            { role: 'assistant', content: [
                { type: 'text', text: 'Checking tomorrow\'s forecast' },
                { type: 'tool_use', id: 'call_2', name: 'weather', input: {} }
            ]},
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: '68F cloudy' }] },
            { role: 'assistant', content: 'Tomorrow will be 68F' } // Turn 1 ends
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Turn 0 tools should be filtered
    test.assertTrue(!result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '72F sunny'
    ), 'Turn 0 tools should be filtered');
    
    // Turn 1 tools should be present
    test.assertTrue(result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === '68F cloudy'
    ), 'Turn 1 tools should be present');
    
    // Check that tool_use blocks were filtered from assistant messages
    const turn0Assistant = result.messages.find(m => 
        m.role === 'assistant' && Array.isArray(m.content) && 
        m.content.some(c => c.type === 'text' && c.text === 'Let me check the weather')
    );
    if (turn0Assistant) {
        test.assertTrue(!turn0Assistant.content.some(c => c.type === 'tool_use'), 
            'Turn 0 tool_use blocks should be filtered from assistant message');
    }
});

test.test('Tool memory - forgetAfterConclusions = 1 (keep 1 turn)', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = true;
    settings.optimisation.toolMemory.forgetAfterConclusions = 1;
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Q1' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 't1', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: 'r1' }] },
            { role: 'assistant', content: 'Answer 1' }, // Turn 0 ends
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_2', name: 't2', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_2', result: 'r2' }] },
            { role: 'assistant', content: 'Answer 2' }, // Turn 1 ends
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_3', name: 't3', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_3', result: 'r3' }] },
            { role: 'assistant', content: 'Answer 3' } // Turn 2 ends
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // We're in Turn 2, so:
    // - Turn 0 tools should be filtered (2 - 0 = 2 > 1)
    // - Turn 1 tools should be kept (2 - 1 = 1 <= 1)
    // - Turn 2 tools should be kept (2 - 2 = 0 <= 1)
    
    test.assertTrue(!result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === 'r1'
    ), 'Turn 0 tools should be filtered');
    
    test.assertTrue(result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === 'r2'
    ), 'Turn 1 tools should be present');
    
    test.assertTrue(result.messages.some(m => 
        m.role === 'tool-results' && m.toolResults && m.toolResults[0].result === 'r3'
    ), 'Turn 2 tools should be present');
    
    test.assertEqual(result.stats.toolsFiltered, 2); // Turn 0: 1 tool call + 1 tool result
});

// Statistics Accuracy Tests
test.test('Statistics - accurate message counting', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Hello' },
            { role: 'system-title', content: 'Title request' }, // Skipped
            { role: 'title', content: 'Chat Title' }, // Skipped
            { role: 'assistant', content: 'Response' },
            { role: 'error', content: 'Error occurred' }, // Skipped
            { role: 'user', content: 'Another message' },
            { role: 'tool-summary-request', content: 'Summary request' }, // Skipped
            { role: 'assistant', content: 'Another response' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.stats.originalMessages, 9);
    test.assertEqual(result.stats.optimizedMessages, 5); // Only non-skipped messages
});

test.test('Statistics - tool filtering counts', () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolMemory.enabled = false; // Disable to test basic stats
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Task' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'tool1', input: {} }] },
            { 
                role: 'tool-results', 
                toolResults: [
                    { toolCallId: 'call_1', result: 'result1' },
                    { toolCallId: 'call_2', result: 'result2' },
                    { toolCallId: 'call_3', result: 'result3' }
                ]
            },
            { role: 'assistant', content: 'Task completed!' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With tool memory disabled, no tools should be filtered
    test.assertEqual(result.stats.toolsFiltered, 0);
    test.assertEqual(result.stats.originalMessages, 5);
    test.assertEqual(result.stats.optimizedMessages, 5);
});

// performToolSummarization Tests
test.test('performToolSummarization - validates inputs', async () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = true;
    settings.llmProviderFactory = () => {};
    
    const optimizer = new MessageOptimizer(settings);
    
    // Test null messages
    try {
        await optimizer.performToolSummarization(null, {});
        test.assertTrue(false, 'Should have thrown error');
    } catch (error) {
        test.assertTrue(error.message.includes('messages must be an array'));
    }
    
    // Test null context
    try {
        await optimizer.performToolSummarization([], null);
        test.assertTrue(false, 'Should have thrown error');
    } catch (error) {
        test.assertTrue(error.message.includes('context must be a valid object'));
    }
});

test.test('performToolSummarization - returns messages unchanged when disabled', async () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = false;
    
    const optimizer = new MessageOptimizer(settings);
    const messages = [{ role: 'user', content: 'test' }];
    
    const result = await optimizer.performToolSummarization(messages, {});
    
    test.assertEqual(result, messages);
});

test.test('performToolSummarization - returns messages unchanged when no pending summarization', async () => {
    const settings = createDefaultSettings();
    settings.optimisation.toolSummarisation.enabled = true;
    settings.llmProviderFactory = () => {};
    
    const optimizer = new MessageOptimizer(settings);
    const messages = [
        { role: 'user', content: 'test' },
        { role: 'tool-results', toolResults: [{ toolCallId: '1', result: 'small' }] }
    ];
    
    const result = await optimizer.performToolSummarization(messages, {});
    
    test.assertEqual(result.length, messages.length);
});

// Cache Control Edge Cases
test.test('Cache control - smart strategy avoids tool-results', () => {
    const settings = createDefaultSettings();
    settings.optimisation.cacheControl.enabled = true;
    settings.optimisation.cacheControl.strategy = 'smart';
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'Message 1' },
            { role: 'assistant', content: 'Response 1' },
            { role: 'tool-results', toolResults: [] }, // Should not be cached
            { role: 'assistant', content: 'Response 2' },
            { role: 'user', content: 'Message 2' },
            { role: 'assistant', content: 'Response 3' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should cache up to 70% but avoid tool-results
    const seventyPercent = Math.floor(result.messages.length * 0.7);
    test.assertTrue(result.cacheControlIndex <= seventyPercent);
    test.assertTrue(result.messages[result.cacheControlIndex].role !== 'tool-results');
});

// Mixed Type/Role Messages
test.test('Process message with type instead of role', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    // Add a system message at start then user messages with type
    const chat = {
        messages: [
            { role: 'system', content: 'Test system' }, // Use role for system
            { type: 'user', content: 'Hello' },
            { type: 'assistant', content: 'Hi' },
            { type: 'tool-results', toolResults: [{ toolCallId: '1', result: 'data' }] }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // All messages should be included
    test.assertEqual(result.messages.length, 4);
    test.assertEqual(result.messages[0].role, 'system');
    test.assertEqual(result.messages[1].type, 'user');
    test.assertEqual(result.messages[2].type, 'assistant');
});

// Run all tests
if (import.meta.url === `file://${process.argv[1]}`) {
    test.run().catch(console.error);
}

export { TestRunner };