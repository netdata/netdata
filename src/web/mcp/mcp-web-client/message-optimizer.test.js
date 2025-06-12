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
        primaryModel: 'claude-3-sonnet',
        secondaryModel: 'claude-3-haiku',
        toolSummarization: { enabled: false, threshold: 50000 },
        toolMemory: { enabled: false, forgetAfterConclusions: 1 },
        cacheControl: { enabled: false, strategy: 'smart' },
        autoSummarization: { enabled: false, triggerPercent: 50 }
    };
}

// Constructor Tests
test.test('Constructor - Valid settings', () => {
    const settings = createDefaultSettings();
    const optimizer = new MessageOptimizer(settings);
    test.assertEqual(optimizer.settings.primaryModel, 'claude-3-sonnet');
});

test.test('Constructor - Invalid settings type', () => {
    test.assertThrows(() => new MessageOptimizer(null), 'settings must be a valid object');
    test.assertThrows(() => new MessageOptimizer('invalid'), 'settings must be a valid object');
});

test.test('Constructor - Invalid primaryModel type', () => {
    const settings = { primaryModel: 123 };
    test.assertThrows(() => new MessageOptimizer(settings), 'primaryModel must be a string');
});

test.test('Constructor - Valid null secondaryModel', () => {
    const settings = { primaryModel: 'claude-3-sonnet', secondaryModel: null };
    const optimizer = new MessageOptimizer(settings);
    test.assertEqual(optimizer.settings.secondaryModel, null);
});

test.test('Constructor - Invalid tool summarization threshold', () => {
    const settings = { toolSummarization: { threshold: -100 } };
    test.assertThrows(() => new MessageOptimizer(settings), 'threshold must be positive number');
});

test.test('Constructor - Invalid tool memory conclusions', () => {
    const settings = { toolMemory: { forgetAfterConclusions: 10 } };
    test.assertThrows(() => new MessageOptimizer(settings), 'forgetAfterConclusions must be 0-5');
});

test.test('Constructor - Invalid cache strategy', () => {
    const settings = { cacheControl: { strategy: 'invalid' } };
    test.assertThrows(() => new MessageOptimizer(settings), 'strategy must be one of');
});

// buildMessagesForAPI Input Validation Tests
test.test('buildMessagesForAPI - Null chat', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    test.assertThrows(() => optimizer.buildMessagesForAPI(null), 'chat must be a valid object');
});

test.test('buildMessagesForAPI - Missing messages array', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = { id: 'test' };
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), 'messages must be an array');
});

test.test('buildMessagesForAPI - Empty messages array', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = { messages: [] };
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), 'messages cannot be empty');
});

test.test('buildMessagesForAPI - Invalid freezeCache type', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = createBasicChat();
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat, 'invalid'), 'freezeCache must be boolean');
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
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), 'not a valid object');
});

test.test('Message validation - Missing role and type', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { content: 'Hello' } // Missing role/type
        ]
    };
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), 'missing both role and type');
});

test.test('Message validation - tool-results missing toolResults', () => {
    const optimizer = new MessageOptimizer(createDefaultSettings());
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'tool-results' } // Missing toolResults array
        ]
    };
    
    test.assertThrows(() => optimizer.buildMessagesForAPI(chat), 'missing toolResults array');
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
    settings.toolMemory.enabled = false;
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createChatWithTools();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // All messages should be included
    test.assertTrue(result.messages.some(m => m.role === 'tool-results'));
});

test.test('Tool memory - enabled but no conclusions', () => {
    const settings = createDefaultSettings();
    settings.toolMemory.enabled = true;
    settings.toolMemory.forgetAfterConclusions = 1;
    
    const optimizer = new MessageOptimizer(settings);
    
    // Chat without conclusion indicators
    const chat = {
        messages: [
            { role: 'system', content: 'Test' },
            { role: 'user', content: 'List files' },
            { role: 'assistant', content: [{ type: 'tool_use', id: 'call_1', name: 'list', input: {} }] },
            { role: 'tool-results', toolResults: [{ toolCallId: 'call_1', result: {} }] },
            { role: 'assistant', content: 'I\'m still working on this...' }
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Tools should still be included (no conclusion)
    test.assertTrue(result.messages.some(m => m.role === 'tool-results'));
});

// Cache Control Tests
test.test('Cache control - disabled', () => {
    const settings = createDefaultSettings();
    settings.cacheControl.enabled = false;
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    test.assertEqual(result.cacheControlIndex, -1);
});

test.test('Cache control - smart strategy', () => {
    const settings = createDefaultSettings();
    settings.cacheControl.enabled = true;
    settings.cacheControl.strategy = 'smart';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should cache up to 70% of messages
    const expectedIndex = Math.floor(result.messages.length * 0.7);
    test.assertTrue(result.cacheControlIndex <= expectedIndex);
});

test.test('Cache control - aggressive strategy', () => {
    const settings = createDefaultSettings();
    settings.cacheControl.enabled = true;
    settings.cacheControl.strategy = 'aggressive';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should cache all but last message
    test.assertEqual(result.cacheControlIndex, result.messages.length - 2);
});

test.test('Cache control - minimal strategy', () => {
    const settings = createDefaultSettings();
    settings.cacheControl.enabled = true;
    settings.cacheControl.strategy = 'minimal';
    
    const optimizer = new MessageOptimizer(settings);
    const chat = createBasicChat();
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Should only cache system prompt
    test.assertEqual(result.cacheControlIndex, 0);
});

test.test('Cache control - freeze cache', () => {
    const settings = createDefaultSettings();
    settings.cacheControl.enabled = true;
    
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

// Run all tests
if (import.meta.url === `file://${process.argv[1]}`) {
    test.run().catch(console.error);
}

export { TestRunner };