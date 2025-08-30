#!/usr/bin/env node

/**
 * Example: Using AI Agent programmatically
 */

import { AIAgent } from '../dist/ai-agent.js';

const LLAMA_MODEL = 'llama3.2:3b';
const CONVERSATION_FILE = 'conversation-example.json';

async function main() {
  process.stdout.write('=== AI Agent Programmatic Usage ===\n\n');

  try {
    // Create AI Agent instance
    const agent = new AIAgent({
      configPath: 'config.example.json',
      temperature: 0.7,
      callbacks: {
        onOutput: (text) => process.stdout.write(text),
        onLog: (level, message) => process.stderr.write(`[${level.toUpperCase()}] ${message}\n`)
      }
    });

    // Simple conversation
    process.stdout.write('1. Simple conversation:\n');
    const result1 = await agent.run({
      providers: ['ollama'],
      models: [LLAMA_MODEL],
      tools: [],
      systemPrompt: 'You are a helpful assistant.',
      userPrompt: 'Explain what Node.js is in one sentence.'
    });

    if (result1.success) {
      process.stdout.write('\n✓ Conversation completed successfully\n\n');
    } else {
      process.stdout.write(`✗ Conversation failed: ${result1.error}\n\n`);
    }

    // Conversation with tools
    process.stdout.write('2. Conversation with MCP tools:\n');
    const result2 = await agent.run({
      providers: ['openai'],
      models: ['gpt-4o-mini'],
      tools: ['filesystem'],
      systemPrompt: 'You can read and write files to help the user.',
      userPrompt: 'List the files in the current directory.'
    });

    if (result2.success) {
      process.stdout.write('\n✓ Tool-assisted conversation completed\n\n');
    } else {
      process.stdout.write(`✗ Tool conversation failed: ${result2.error}\n\n`);
    }

    // Save conversation
    process.stdout.write('3. Conversation with history:\n');
    const result3 = await agent.run({
      providers: ['ollama'],
      models: [LLAMA_MODEL],  
      tools: [],
      systemPrompt: 'You are a coding tutor.',
      userPrompt: 'What is TypeScript?',
      saveConversation: CONVERSATION_FILE
    });

    if (result3.success) {
      process.stdout.write(`\n✓ Conversation saved to ${CONVERSATION_FILE}\n\n`);
      
      // Continue conversation
      const result4 = await agent.run({
        providers: ['ollama'],
        models: [LLAMA_MODEL],
        tools: [],
        systemPrompt: 'You are a coding tutor.',
        userPrompt: 'Give me a simple example.',
        loadConversation: CONVERSATION_FILE,
        saveConversation: CONVERSATION_FILE
      });

      if (result4.success) {
        process.stdout.write('\n✓ Conversation continued and updated\n\n');
      }
    }

  } catch (error) {
    process.stderr.write(`Fatal error: ${error.message}\n`);
    process.exit(1);
  }
}

main().catch((error) => {
  process.stderr.write(`Unhandled error: ${error.message}\n`);
  process.exit(1);
});