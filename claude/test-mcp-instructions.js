#!/usr/bin/env node

/**
 * Test script to verify MCP instruction handling
 */

import { writeFileSync } from 'node:fs';

import { AIAgent } from './dist/ai-agent.js';

async function testMCPInstructions() {
  process.stdout.write('Testing MCP Instruction Handling...\n\n');
  
  try {
    // Create AI agent with minimal configuration
    // Create a simpler test configuration
    const testConfig = {
      providers: {
        ollama: {
          apiKey: "ollama",
          baseUrl: "http://localhost:11434/v1"
        }
      },
      mcpServers: {
        "test-mcp": {
          type: "stdio",
          command: "node",
          args: ["test-mcp-server.js"],
          description: "Test MCP server with instructions"
        }
      },
      defaults: {
        llmTimeout: 30000,
        toolTimeout: 10000,
        parallelToolCalls: true,
        temperature: 0.7,
        topP: 1.0
      }
    };

    // Write temporary config file
    writeFileSync('/tmp/test-config.json', JSON.stringify(testConfig, null, 2));

    const agent = new AIAgent({
      configPath: '/tmp/test-config.json',
      callbacks: {
        onLog: (level, message) => process.stderr.write(`[${level.toUpperCase()}] ${message}\n`),
        onOutput: (text) => process.stdout.write(text),
      }
    });

    // Test with a simple prompt that would benefit from MCP instructions
    const result = await agent.run({
      systemPrompt: "You are a helpful AI assistant with access to tools.",
      userPrompt: "Can you see any instructions from the MCP server? What tools are available?", 
      providers: ['ollama'],
      models: ['llama3.2:3b'],
      tools: ['test-mcp'], // Using our test MCP server
    });

    process.stdout.write('\n--- TEST RESULTS ---\n');
    process.stdout.write(`Success: ${result.success}\n`);
    process.stdout.write(`Error: ${result.error}\n`);
    process.stdout.write(`Conversation: ${JSON.stringify(result.conversation, null, 2)}\n`);
    
    // Save results to file for inspection
    writeFileSync('/tmp/mcp-instructions-test.json', JSON.stringify({
      success: result.success,
      error: result.error,
      conversation: result.conversation,
      timestamp: new Date().toISOString()
    }, null, 2));
    
    process.stdout.write('\nResults saved to /tmp/mcp-instructions-test.json\n');
    
  } catch (error) {
    process.stderr.write(`Test failed: ${error.message}\n`);
    process.stderr.write(`Stack: ${error.stack}\n`);
  }
}

// Run the test
testMCPInstructions();