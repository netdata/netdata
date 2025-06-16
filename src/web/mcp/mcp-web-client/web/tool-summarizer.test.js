/**
 * Unit Tests for ToolSummarizer
 * 
 * Run with: node tool-summarizer.test.js
 */

import { ToolSummarizer } from './tool-summarizer.js';
import { TestRunner } from './message-optimizer.test.js';

const test = new TestRunner();

// Mock LLM provider
class MockLLMProvider {
    constructor(model) {
        this.model = model;
        this.calls = [];
    }
    
    async sendMessage(messages, tools, temperature, mode) {
        this.calls.push({ messages, tools, temperature, mode });
        
        // Return mock summary based on input
        const userMessage = messages.find(m => m.role === 'user');
        if (userMessage && userMessage.content.includes('Tool Response to Summarize')) {
            return {
                content: 'This is a concise summary of the tool response focusing on key data points.',
                usage: {
                    promptTokens: 1000,
                    completionTokens: 50,
                    totalTokens: 1050
                }
            };
        }
        
        throw new Error('Unexpected message format');
    }
}

// Mock provider factory
function createMockProviderFactory() {
    const providers = new Map();
    
    return function(provider, url, model) {
        const key = `${provider}/${model}/${url}`;
        if (!providers.has(key)) {
            providers.set(key, new MockLLMProvider(model));
        }
        return providers.get(key);
    };
}

// Test data factories
function createValidConfig() {
    return {
        llmProviderFactory: createMockProviderFactory(),
        primaryModel: 'anthropic/claude-3-sonnet',
        secondaryModel: 'anthropic/claude-3-haiku',
        threshold: 50000,
        useSecondaryModel: true
    };
}

function createLargeToolResult() {
    // Create a result larger than 50KB
    const largeData = 'x'.repeat(60000);
    return {
        toolCallId: 'call_123',
        toolName: 'list_files',
        result: largeData
    };
}

function createSmallToolResult() {
    return {
        toolCallId: 'call_456',
        toolName: 'get_time',
        result: 'Current time: 2024-01-15 10:30:00'
    };
}

// Constructor Tests
test.test('Constructor - Valid configuration', () => {
    const config = createValidConfig();
    const summarizer = new ToolSummarizer(config);
    test.assertEqual(summarizer.primaryModel, 'anthropic/claude-3-sonnet');
    test.assertEqual(summarizer.secondaryModel, 'anthropic/claude-3-haiku');
    test.assertEqual(summarizer.threshold, 50000);
    test.assertTrue(summarizer.useSecondaryModel);
});

test.test('Constructor - Invalid config object', () => {
    test.assertThrows(() => new ToolSummarizer(null), 'config must be a valid object');
    test.assertThrows(() => new ToolSummarizer('invalid'), 'config must be a valid object');
});

test.test('Constructor - Missing llmProviderFactory', () => {
    const config = createValidConfig();
    delete config.llmProviderFactory;
    test.assertThrows(() => new ToolSummarizer(config), 'llmProviderFactory must be a function');
});

test.test('Constructor - Invalid primaryModel', () => {
    const config = createValidConfig();
    config.primaryModel = 123;
    test.assertThrows(() => new ToolSummarizer(config), 'primaryModel must be a string');
});

test.test('Constructor - Invalid secondaryModel', () => {
    const config = createValidConfig();
    config.secondaryModel = 123;
    test.assertThrows(() => new ToolSummarizer(config), 'secondaryModel must be a string or null');
});

test.test('Constructor - Valid null secondaryModel', () => {
    const config = createValidConfig();
    config.secondaryModel = null;
    const summarizer = new ToolSummarizer(config);
    test.assertEqual(summarizer.secondaryModel, null);
});

test.test('Constructor - Invalid threshold', () => {
    const config = createValidConfig();
    config.threshold = -100;
    test.assertThrows(() => new ToolSummarizer(config), 'threshold must be a positive number');
});

test.test('Constructor - Invalid useSecondaryModel', () => {
    const config = createValidConfig();
    config.useSecondaryModel = 'yes';
    test.assertThrows(() => new ToolSummarizer(config), 'useSecondaryModel must be boolean');
});

// Size Calculation Tests
test.test('calculateSize - String content', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const size = summarizer.calculateSize('Hello, world!');
    test.assertEqual(size, 13); // 13 bytes for "Hello, world!"
});

test.test('calculateSize - Object content', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const obj = { name: 'test', value: 42 };
    const size = summarizer.calculateSize(obj);
    test.assertTrue(size > 0);
});

test.test('calculateSize - Array content', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const arr = [1, 2, 3, 'test'];
    const size = summarizer.calculateSize(arr);
    test.assertTrue(size > 0);
});

// Should Summarize Tests
test.test('shouldSummarize - Large result', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const result = createLargeToolResult();
    test.assertTrue(summarizer.shouldSummarize(result));
});

test.test('shouldSummarize - Small result', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const result = createSmallToolResult();
    test.assertFalse(summarizer.shouldSummarize(result));
});

test.test('shouldSummarize - Null result', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    test.assertFalse(summarizer.shouldSummarize(null));
});

test.test('shouldSummarize - Missing result field', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    test.assertFalse(summarizer.shouldSummarize({ toolCallId: '123' }));
});

// Model Selection Tests
test.test('Model selection - Use secondary model', () => {
    const config = createValidConfig();
    const summarizer = new ToolSummarizer(config);
    
    const modelId = summarizer.useSecondaryModel && summarizer.secondaryModel 
        ? summarizer.secondaryModel 
        : summarizer.primaryModel;
        
    test.assertEqual(modelId, 'anthropic/claude-3-haiku');
});

test.test('Model selection - Use primary when secondary disabled', () => {
    const config = createValidConfig();
    config.useSecondaryModel = false;
    const summarizer = new ToolSummarizer(config);
    
    const modelId = summarizer.useSecondaryModel && summarizer.secondaryModel 
        ? summarizer.secondaryModel 
        : summarizer.primaryModel;
        
    test.assertEqual(modelId, 'anthropic/claude-3-sonnet');
});

test.test('Model selection - Use primary when secondary is null', () => {
    const config = createValidConfig();
    config.secondaryModel = null;
    const summarizer = new ToolSummarizer(config);
    
    const modelId = summarizer.useSecondaryModel && summarizer.secondaryModel 
        ? summarizer.secondaryModel 
        : summarizer.primaryModel;
        
    test.assertEqual(modelId, 'anthropic/claude-3-sonnet');
});

// Parse Model Identifier Tests
test.test('parseModelIdentifier - Valid identifier', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const parsed = summarizer.parseModelIdentifier('anthropic/claude-3-haiku');
    test.assertEqual(parsed.provider, 'anthropic');
    test.assertEqual(parsed.modelName, 'claude-3-haiku');
});

test.test('parseModelIdentifier - Invalid identifier', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    test.assertThrows(
        () => summarizer.parseModelIdentifier('invalid-format'),
        'Invalid model identifier'
    );
});

// Prompt Building Tests
test.test('buildSummarizationPrompt - Complete context', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const prompt = summarizer.buildSummarizationPrompt({
        toolResult: { result: 'Large data here' },
        toolName: 'list_files',
        toolSchema: { description: 'Lists files in directory' },
        userQuestion: 'What files are in /tmp?',
        assistantReasoning: 'I need to check the /tmp directory'
    });
    
    test.assertTrue(prompt.includes('What files are in /tmp?'));
    test.assertTrue(prompt.includes('I need to check the /tmp directory'));
    test.assertTrue(prompt.includes('list_files'));
    test.assertTrue(prompt.includes('Lists files in directory'));
    test.assertTrue(prompt.includes('Large data here'));
});

// Summarization Tests
test.test('summarizeToolResult - Successful summarization', async () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    const result = await summarizer.summarizeToolResult({
        toolResult: createLargeToolResult(),
        toolName: 'list_files',
        toolSchema: { description: 'Lists files' },
        userQuestion: 'What files exist?',
        assistantReasoning: 'Checking files',
        providerInfo: { url: 'http://localhost:8080' }
    });
    
    test.assertEqual(result.summary, 'This is a concise summary of the tool response focusing on key data points.');
    test.assertEqual(result.model, 'anthropic/claude-3-haiku');
    test.assertTrue(result.originalSize > 50000);
    test.assertTrue(result.summarizedSize < result.originalSize);
    test.assertTrue(result.compressionRatio < 1);
    test.assertTrue(result.usage !== null);
    test.assertTrue(result.timestamp !== null);
});

test.test('summarizeToolResult - Missing parameters', async () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    try {
        await summarizer.summarizeToolResult({
            toolResult: createLargeToolResult()
            // Missing other required params
        });
        test.assertTrue(false, 'Should have thrown error');
    } catch (error) {
        test.assertTrue(error.message.includes('Missing required parameter'));
    }
});

// Compression Ratio Tests
test.test('calculateCompressionRatio - Normal compression', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const ratio = summarizer.calculateCompressionRatio(
        'x'.repeat(1000), // 1000 bytes
        'summary'.repeat(10) // ~70 bytes
    );
    test.assertTrue(ratio < 0.1);
});

test.test('calculateCompressionRatio - Zero original size', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const ratio = summarizer.calculateCompressionRatio('', 'summary');
    test.assertEqual(ratio, 1);
});

// Multiple Tools Tests
test.test('summarizeMultipleTools - Mixed sizes', async () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    const toolResults = [
        createLargeToolResult(),
        createSmallToolResult(),
        { ...createLargeToolResult(), toolCallId: 'call_789', toolName: 'read_file' }
    ];
    
    const context = {
        toolSchemas: new Map([
            ['list_files', { description: 'Lists files' }],
            ['get_time', { description: 'Gets time' }],
            ['read_file', { description: 'Reads file' }]
        ]),
        userQuestion: 'Test question',
        assistantReasoning: 'Test reasoning',
        providerInfo: { url: 'http://localhost:8080' }
    };
    
    const results = await summarizer.summarizeMultipleTools(toolResults, context);
    
    // Should summarize 2 large results, skip the small one
    test.assertEqual(results.size, 2);
    test.assertTrue(results.has('call_123'));
    test.assertTrue(results.has('call_789'));
    test.assertFalse(results.has('call_456'));
});

test.test('summarizeMultipleTools - Invalid input', async () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    try {
        await summarizer.summarizeMultipleTools('not-an-array', {});
        test.assertTrue(false, 'Should have thrown error');
    } catch (error) {
        test.assertTrue(error.message.includes('must be an array'));
    }
});

// Provider Caching Tests
test.test('getProvider - Caches providers', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    const provider1 = summarizer.getProvider('anthropic/claude-3-haiku', { url: 'http://localhost:8080' });
    const provider2 = summarizer.getProvider('anthropic/claude-3-haiku', { url: 'http://localhost:8080' });
    
    test.assertTrue(provider1 === provider2, 'Should return same cached instance');
});

test.test('getProvider - Different URLs get different providers', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    
    const provider1 = summarizer.getProvider('anthropic/claude-3-haiku', { url: 'http://localhost:8080' });
    const provider2 = summarizer.getProvider('anthropic/claude-3-haiku', { url: 'http://localhost:8081' });
    
    test.assertTrue(provider1 !== provider2, 'Should return different instances');
});

// System Prompt Test
test.test('getSystemPrompt - Returns valid prompt', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const prompt = summarizer.getSystemPrompt();
    
    test.assertTrue(prompt.includes('summarizes tool responses'));
    test.assertTrue(prompt.includes('reduce token usage'));
});

// Response Parsing Tests
test.test('parseResponse - Valid response', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    const parsed = summarizer.parseResponse('  This is a summary.  \n');
    test.assertEqual(parsed, 'This is a summary.');
});

test.test('parseResponse - Invalid response', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    test.assertThrows(() => summarizer.parseResponse(null), 'Invalid response format');
    test.assertThrows(() => summarizer.parseResponse(123), 'Invalid response format');
});

test.test('parseResponse - Empty response', () => {
    const summarizer = new ToolSummarizer(createValidConfig());
    test.assertThrows(() => summarizer.parseResponse('   \n\n  '), 'Empty summary response');
});

// Run all tests
if (import.meta.url === `file://${process.argv[1]}`) {
    test.run().catch(console.error);
}

export { TestRunner };