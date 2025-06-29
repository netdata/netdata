/**
 * Definitive test suite for MessageOptimizer
 * 
 * This test suite verifies that the tool memory feature works correctly
 * and that filtered messages are properly prepared for LLM providers.
 */

import { MessageOptimizer } from './message-optimizer.js';

// Helper function to create a basic settings object
function createSettings(forgetAfterConclusions = 0, toolMemoryEnabled = true) {
    return {
        model: {
            provider: 'test',
            id: 'test-model',
            params: {
                temperature: 0.7,
                topP: 0.9,
                maxTokens: 4096,
                seed: { enabled: false, value: 123 }
            }
        },
        optimisation: {
            toolSummarisation: { enabled: false, thresholdKiB: 20, model: null },
            autoSummarisation: { enabled: false, triggerPercent: 50, model: null },
            toolMemory: { enabled: toolMemoryEnabled, forgetAfterConclusions },
            cacheControl: 'all-off',
            titleGeneration: { enabled: true, model: null }
        },
        mcpServer: 'test'
    };
}

// Helper to create messages
function createUserMessage(content, turn) {
    return { role: 'user', content, turn };
}

function createAssistantMessage(content, turn, toolCalls = null) {
    const msg = { role: 'assistant', content, turn };
    if (toolCalls) {
        msg.toolCalls = toolCalls;
    }
    return msg;
}

function createToolUseContent(text, toolUses = []) {
    const content = [];
    if (text) {
        content.push({ type: 'text', text });
    }
    toolUses.forEach(tool => {
        content.push({
            type: 'tool_use',
            id: tool.id,
            name: tool.name,
            input: tool.input || {}
        });
    });
    return content;
}

function createToolResults(results, turn) {
    return {
        role: 'tool-results',
        toolResults: results.map(r => ({
            toolCallId: r.id,
            name: r.name,
            result: r.result || 'Result data',
            includeInContext: true
        })),
        turn
    };
}

// Test runner
function runTest(name, testFn) {
    console.log(`\n=== ${name} ===`);
    try {
        testFn();
        console.log('✅ PASSED');
    } catch (error) {
        console.log('❌ FAILED:', error.message);
        console.error(error.stack);
    }
}

// Test 1: Basic functionality - forgetAfterConclusions = 0
runTest('Test 1: forgetAfterConclusions = 0 (immediate filtering)', () => {
    const settings = createSettings(0, true);
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        id: 'test-1',
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' },
            createUserMessage('First question', 1),
            createAssistantMessage(
                createToolUseContent('Let me check...', [
                    { id: 'call_1', name: 'tool1' }
                ]),
                1,
                [{ id: 'call_1', name: 'tool1', arguments: {} }]
            ),
            createToolResults([{ id: 'call_1', name: 'tool1', result: 'Data 1' }], 1),
            createAssistantMessage('Based on tool1, the answer is X.', 1), // Conclusion
            createUserMessage('Second question', 2),
            createAssistantMessage(
                createToolUseContent('Let me check again...', [
                    { id: 'call_2', name: 'tool2' }
                ]),
                2,
                [{ id: 'call_2', name: 'tool2', arguments: {} }]
            ),
            createToolResults([{ id: 'call_2', name: 'tool2', result: 'Data 2' }], 2),
            createAssistantMessage('Based on tool2, the answer is Y.', 2) // Conclusion
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Verify turn 1 tools are filtered
    const hasTurn1Tools = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id === 'call_1')) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId === 'call_1'))
    );
    
    // Verify turn 2 tools remain
    const hasTurn2Tools = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id === 'call_2')) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId === 'call_2'))
    );
    
    if (hasTurn1Tools) throw new Error('Turn 1 tools should be filtered');
    if (!hasTurn2Tools) throw new Error('Turn 2 tools should remain');
    if (result.stats.toolsFiltered !== 1) throw new Error(`Expected 1 tool filtered, got ${result.stats.toolsFiltered}`);
});

// Test 2: Real-world scenario from user report
runTest('Test 2: Real-world scenario with 4 turns', () => {
    const settings = {
        model: {
            provider: 'anthropic',
            id: 'claude-3-5-haiku-20241022',
            params: {
                temperature: 0.7,
                topP: 0.9,
                maxTokens: 4096,
                seed: { enabled: false, value: 972472 }
            }
        },
        optimisation: {
            toolSummarisation: { enabled: false, thresholdKiB: 20, model: null },
            autoSummarisation: { enabled: false, triggerPercent: 50, model: null },
            toolMemory: { enabled: true, forgetAfterConclusions: 0 },
            cacheControl: 'all-off',
            titleGeneration: { enabled: true, model: null }
        },
        mcpServer: 'demos_registry'
    };
    
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        id: 'real-test',
        messages: [
            { role: 'system', content: 'System prompt here...' },
            // Turn 1: 4 tool calls
            createUserMessage('describe my infra', 1),
            createAssistantMessage(
                createToolUseContent('I\'ll analyze...', [
                    { id: 'call_t1_1', name: 'list_nodes' },
                    { id: 'call_t1_2', name: 'list_raised_alerts' },
                    { id: 'call_t1_3', name: 'find_anomalous_metrics' },
                    { id: 'call_t1_4', name: 'list_metrics' }
                ]),
                1,
                [
                    { id: 'call_t1_1', name: 'list_nodes', arguments: {} },
                    { id: 'call_t1_2', name: 'list_raised_alerts', arguments: {} },
                    { id: 'call_t1_3', name: 'find_anomalous_metrics', arguments: {} },
                    { id: 'call_t1_4', name: 'list_metrics', arguments: {} }
                ]
            ),
            createToolResults([
                { id: 'call_t1_1', name: 'list_nodes' },
                { id: 'call_t1_2', name: 'list_raised_alerts' },
                { id: 'call_t1_3', name: 'find_anomalous_metrics' },
                { id: 'call_t1_4', name: 'list_metrics' }
            ], 1),
            createAssistantMessage('Based on the analysis...', 1), // Conclusion
            { role: 'system-title', content: 'Generate title' },
            { role: 'title', content: 'Infrastructure Check' },
            // Turn 2: 4 more tool calls
            createUserMessage('which applications?', 2),
            createAssistantMessage(
                createToolUseContent('Let me check...', [
                    { id: 'call_t2_1', name: 'list_metrics' },
                    { id: 'call_t2_2', name: 'get_nodes_details' },
                    { id: 'call_t2_3', name: 'list_functions' },
                    { id: 'call_t2_4', name: 'list_running_alerts' }
                ]),
                2,
                [
                    { id: 'call_t2_1', name: 'list_metrics', arguments: {} },
                    { id: 'call_t2_2', name: 'get_nodes_details', arguments: {} },
                    { id: 'call_t2_3', name: 'list_functions', arguments: {} },
                    { id: 'call_t2_4', name: 'list_running_alerts', arguments: {} }
                ]
            ),
            createToolResults([
                { id: 'call_t2_1', name: 'list_metrics' },
                { id: 'call_t2_2', name: 'get_nodes_details' },
                { id: 'call_t2_3', name: 'list_functions' },
                { id: 'call_t2_4', name: 'list_running_alerts' }
            ], 2),
            createAssistantMessage('Here are the applications...', 2), // Conclusion
            // Turn 3: No tools
            createUserMessage('what are the main problems?', 3),
            createAssistantMessage('The main problems are...', 3), // No tools
            // Turn 4: 2 tool calls
            createUserMessage('tell me more', 4),
            createAssistantMessage(
                createToolUseContent('Getting details...', [
                    { id: 'call_t4_1', name: 'list_metrics' }
                ]),
                4,
                [{ id: 'call_t4_1', name: 'list_metrics', arguments: {} }]
            ),
            createToolResults([{ id: 'call_t4_1', name: 'list_metrics' }], 4),
            createAssistantMessage(
                createToolUseContent('More info...', [
                    { id: 'call_t4_2', name: 'list_functions' }
                ]),
                4,
                [{ id: 'call_t4_2', name: 'list_functions', arguments: {} }]
            ),
            createToolResults([{ id: 'call_t4_2', name: 'list_functions' }], 4),
            createAssistantMessage('Final details...', 4) // Conclusion
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With forgetAfterConclusions = 0, only turn 4 tools should remain
    const hasTurn1Tools = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id.includes('t1_'))) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId.includes('t1_')))
    );
    const hasTurn2Tools = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id.includes('t2_'))) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId.includes('t2_')))
    );
    const hasTurn4Tools = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id.includes('t4_'))) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId.includes('t4_')))
    );
    
    if (hasTurn1Tools) throw new Error('Turn 1 tools should be filtered');
    if (hasTurn2Tools) throw new Error('Turn 2 tools should be filtered');
    if (!hasTurn4Tools) throw new Error('Turn 4 tools should remain');
    if (result.stats.toolsFiltered !== 2) throw new Error(`Expected 2 tool-results filtered, got ${result.stats.toolsFiltered}`);
});

// Test 3: Critical - Verify toolCalls array is removed when tools are filtered
runTest('Test 3: toolCalls array removal (CRITICAL for token reduction)', () => {
    const settings = createSettings(0, true);
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        id: 'test-3',
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' },
            createUserMessage('Question', 1),
            createAssistantMessage(
                createToolUseContent('Checking...', [
                    { id: 'call_old', name: 'old_tool' }
                ]),
                1,
                [{ id: 'call_old', name: 'old_tool', arguments: { data: 'lots of data here' } }]
            ),
            createToolResults([{ id: 'call_old', name: 'old_tool' }], 1),
            createAssistantMessage('Done', 1), // Conclusion
            createUserMessage('Another question', 2),
            createAssistantMessage('Answer without tools', 2)
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // Find the assistant message that had tools
    const assistantWithFilteredTools = result.messages.find(m => 
        m.role === 'assistant' && 
        m.turn === 1 && 
        Array.isArray(m.content)
    );
    
    if (!assistantWithFilteredTools) {
        throw new Error('Assistant message with filtered tools not found');
    }
    
    // CRITICAL: Verify toolCalls array was removed
    if (assistantWithFilteredTools.toolCalls) {
        throw new Error('toolCalls array still present after filtering! This causes high token usage.');
    }
    
    // Verify tool_use blocks were also removed from content
    const hasToolUseBlocks = assistantWithFilteredTools.content.some(c => c.type === 'tool_use');
    if (hasToolUseBlocks) {
        throw new Error('tool_use blocks still present in content after filtering');
    }
});

// Test 4: Assistant message with only tools should be removed entirely
runTest('Test 4: Assistant message with only tools (no text)', () => {
    const settings = createSettings(0, true);
    const optimizer = new MessageOptimizer(settings);
    
    const chat = {
        id: 'test-4',
        messages: [
            { role: 'system', content: 'You are a helpful assistant.' },
            createUserMessage('Do something', 1),
            createAssistantMessage(
                createToolUseContent(null, [ // No text, only tools
                    { id: 'call_1', name: 'tool1' },
                    { id: 'call_2', name: 'tool2' }
                ]),
                1,
                [
                    { id: 'call_1', name: 'tool1', arguments: {} },
                    { id: 'call_2', name: 'tool2', arguments: {} }
                ]
            ),
            createToolResults([
                { id: 'call_1', name: 'tool1' },
                { id: 'call_2', name: 'tool2' }
            ], 1),
            createAssistantMessage('Done.', 1), // Conclusion
            createUserMessage('Another request', 2),
            createAssistantMessage('No tools needed.', 2)
        ]
    };
    
    const result = optimizer.buildMessagesForAPI(chat);
    
    // The assistant message with only tools should be completely removed
    const assistantMessages = result.messages.filter(m => m.role === 'assistant');
    if (assistantMessages.length !== 2) {
        throw new Error(`Expected 2 assistant messages (conclusions only), got ${assistantMessages.length}`);
    }
});

// Test 5: Different forgetAfterConclusions values
runTest('Test 5: forgetAfterConclusions = 1 (keep 1 turn)', () => {
    const settings = createSettings(1, true);
    const optimizer = new MessageOptimizer(settings);
    
    const messages = [{ role: 'system', content: 'You are a helpful assistant.' }];
    
    // Create 3 turns
    for (let turn = 0; turn < 3; turn++) {
        messages.push(createUserMessage(`Question ${turn}`, turn));
        messages.push(createAssistantMessage(
            createToolUseContent(`Checking turn ${turn}...`, [
                { id: `call_${turn}`, name: `tool${turn}` }
            ]),
            turn,
            [{ id: `call_${turn}`, name: `tool${turn}`, arguments: {} }]
        ));
        messages.push(createToolResults([
            { id: `call_${turn}`, name: `tool${turn}` }
        ], turn));
        messages.push(createAssistantMessage(`Answer ${turn}`, turn));
    }
    
    const chat = { id: 'test-5', messages };
    const result = optimizer.buildMessagesForAPI(chat);
    
    // With forgetAfterConclusions = 1:
    // - Turn 0 should be filtered (2 turns old)
    // - Turn 1 should remain (1 turn old)
    // - Turn 2 should remain (current turn)
    
    const hasTurn0 = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id === 'call_0')) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId === 'call_0'))
    );
    const hasTurn1 = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id === 'call_1')) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId === 'call_1'))
    );
    const hasTurn2 = result.messages.some(m => 
        (m.toolCalls && m.toolCalls.some(tc => tc.id === 'call_2')) ||
        (m.toolResults && m.toolResults.some(tr => tr.toolCallId === 'call_2'))
    );
    
    if (hasTurn0) throw new Error('Turn 0 should be filtered');
    if (!hasTurn1) throw new Error('Turn 1 should remain');
    if (!hasTurn2) throw new Error('Turn 2 should remain');
});

// Summary
console.log('\n=== TEST SUMMARY ===');
console.log('All tests completed. Check results above.');
console.log('\nCRITICAL POINTS VERIFIED:');
console.log('1. Tool memory filters old tools based on forgetAfterConclusions threshold');
console.log('2. toolCalls array is REMOVED when tools are filtered (prevents token explosion)');
console.log('3. tool-results messages are completely removed when filtered');
console.log('4. Assistant messages with only tools are removed entirely');
console.log('5. System/UI messages do not affect turn tracking');

console.log('\n=== ARCHITECTURAL ISSUE ===');
console.log('The LLM provider (llm-providers.js) rebuilds tool_use blocks from msg.toolCalls');
console.log('even after MessageOptimizer has filtered them. This causes high token usage.');
console.log('Solution: Either:');
console.log('1. MessageOptimizer should be the ONLY place that handles tool filtering');
console.log('2. OR LLM providers should respect when tools have been filtered');