#!/usr/bin/env node
/**
 * Command Line Interface for AI Agent
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import { Command } from 'commander';

import type { AIAgentOptions, AIAgentRunOptions, AIAgentCallbacks, ConversationMessage } from './types.js';

import { AIAgent } from './ai-agent.js';
import { loadConfiguration } from './config.js';

const program = new Command();

/**
 * Read prompt from file, stdin, or return as-is
 */
async function readPrompt(prompt: string): Promise<string> {
  if (prompt === '-') {
    // Read from stdin
    return new Promise((resolve, reject) => {
      let data = '';
      process.stdin.setEncoding('utf-8');
      process.stdin.on('data', (chunk: string) => { data += chunk; });
      process.stdin.on('end', () => { resolve(data.trim()); });
      process.stdin.on('error', reject);
    });
  }

  if (prompt.startsWith('@')) {
    // Read from file
    const filename = prompt.slice(1);
    try {
      return fs.readFileSync(filename, 'utf-8').trim();
    } catch (error) {
      throw new Error(`Failed to read prompt file ${filename}: ${String(error)}`);
    }
  }

  return prompt;
}

program
  .name('ai-agent')
  .description('Universal LLM Tool Calling Interface with MCP support')
  .version('1.0.0')
  .argument('<providers>', 'Comma-separated list of LLM providers')
  .argument('<models>', 'Comma-separated list of model names')
  .argument('<mcp-tools>', 'Comma-separated list of MCP tools')
  .argument('<system-prompt>', 'System prompt (string, @filename, or - for stdin)')
  .argument('<user-prompt>', 'User prompt (string, @filename, or - for stdin)')
  .option('--config <filename>', 'Configuration file path')
  .option('--llm-timeout <ms>', 'Timeout for LLM responses (ms)', '30000')
  .option('--tool-timeout <ms>', 'Timeout for tool execution (ms)', '10000')
  .option('--max-parallel-tools <n>', 'Max tools to accept from LLM', parseInt)
  .option('--max-concurrent-tools <n>', 'Max tools to run concurrently', parseInt)
  .option('--temperature <n>', 'LLM temperature (0.0-2.0)', '0.7')
  .option('--top-p <n>', 'LLM top-p sampling (0.0-1.0)', '1.0')
  .option('--parallel-tool-calls', 'Enable parallel tool calls (default)', true)
  .option('--no-parallel-tool-calls', 'Disable parallel tool calls')
  .option('--save <filename>', 'Save conversation to JSON file')
  .option('--load <filename>', 'Load conversation from JSON file')
  .option('--accounting <filename>', 'Override accounting file from config')
  .action(async (providers: string, models: string, mcpTools: string | undefined, systemPrompt: string, userPrompt: string, options: Record<string, string | boolean | number | undefined>) => {
    try {
      // Parse command line arguments
      const providerList = providers.split(',').map((s: string) => s.trim()).filter((s: string) => s.length > 0);
      const modelList = models.split(',').map((s: string) => s.trim()).filter((s: string) => s.length > 0);
      const toolList = mcpTools !== undefined ? mcpTools.split(',').map((s: string) => s.trim()).filter((s: string) => s.length > 0) : [];

      // Validate arguments
      if (providerList.length === 0) {
        console.error('Error: At least one provider must be specified');
        process.exit(4);
      }
      if (modelList.length === 0) {
        console.error('Error: At least one model must be specified');
        process.exit(4);
      }
      // MCP tools are optional - can run with no tools for simple LLM chat

      // Parse numeric options
      const llmTimeout = parseInt(options['llmTimeout'] as string);
      const toolTimeout = parseInt(options['toolTimeout'] as string);
      const maxParallelTools = typeof options['maxParallelTools'] === 'string' ? parseInt(options['maxParallelTools']) : undefined;
      const maxConcurrentTools = typeof options['maxConcurrentTools'] === 'string' ? parseInt(options['maxConcurrentTools']) : undefined;
      const temperature = parseFloat(options['temperature'] as string);
      const topP = parseFloat(options['topP'] as string);

      // Validate numeric options
      if (isNaN(llmTimeout) || llmTimeout <= 0) {
        console.error('Error: --llm-timeout must be a positive number');
        process.exit(4);
      }
      if (isNaN(toolTimeout) || toolTimeout <= 0) {
        console.error('Error: --tool-timeout must be a positive number');
        process.exit(4);
      }
      if (isNaN(temperature) || temperature < 0 || temperature > 2) {
        console.error('Error: --temperature must be between 0.0 and 2.0');
        process.exit(4);
      }
      if (isNaN(topP) || topP < 0 || topP > 1) {
        console.error('Error: --top-p must be between 0.0 and 1.0');
        process.exit(4);
      }
      if (maxParallelTools !== undefined && (isNaN(maxParallelTools) || maxParallelTools <= 0)) {
        console.error('Error: --max-parallel-tools must be a positive number');
        process.exit(4);
      }
      if (maxConcurrentTools !== undefined && (isNaN(maxConcurrentTools) || maxConcurrentTools <= 0)) {
        console.error('Error: --max-concurrent-tools must be a positive number');
        process.exit(4);
      }

      // Handle parallel tool calls flag
      const parallelToolCalls = options['parallelToolCalls'] !== false;

      // Resolve prompts from files/stdin at CLI level
      const resolvedSystemPrompt = await readPrompt(systemPrompt);
      const resolvedUserPrompt = await readPrompt(userPrompt);
      
      // Load conversation history if specified
      let conversationHistory: ConversationMessage[] | undefined;
      if (typeof options['load'] === 'string') {
        try {
          const content = fs.readFileSync(options['load'], 'utf-8');
          conversationHistory = JSON.parse(content) as ConversationMessage[];
        } catch (error) {
          console.error(`Failed to load conversation from ${options['load']}: ${String(error)}`);
          process.exit(1);
        }
      }

      // Load config to get accounting file if not overridden
      const config = loadConfiguration(options['config'] as string | undefined);
      
      // Set up callbacks for accounting and output
      const accountingFile = options['accounting'] ?? config.accounting?.file;
      const callbacks: AIAgentCallbacks = {
        onLog: (level, message) => { console.error(`[${level.toUpperCase()}] ${message}`); },
        onOutput: (text) => process.stdout.write(text),
        onAccounting: typeof accountingFile === 'string' ? (entry) => {
          try {
            // Ensure directory exists
            const dir = path.dirname(accountingFile);
            if (!fs.existsSync(dir)) {
              fs.mkdirSync(dir, { recursive: true });
            }
            // Append to JSONL file
            const line = JSON.stringify(entry) + '\n';
            fs.appendFileSync(accountingFile, line, 'utf-8');
          } catch (error) {
            console.error(`Failed to write accounting entry to ${accountingFile}:`, error);
          }
        } : undefined,
      };

      // Create AI Agent options
      const agentOptions: AIAgentOptions = {
        configPath: options['config'] as string | undefined,
        llmTimeout,
        toolTimeout,
        parallelToolCalls,
        temperature,
        topP,
        callbacks,
      };

      // Create run options
      const runOptions: AIAgentRunOptions = {
        providers: providerList,
        models: modelList,
        tools: toolList,
        systemPrompt: resolvedSystemPrompt,
        userPrompt: resolvedUserPrompt,
        conversationHistory,
      };

      // Create and run AI Agent
      const agent = new AIAgent(agentOptions);
      const result = await agent.run(runOptions);

      if (!result.success) {
        const errorMessage = result.error ?? 'Unknown error';
        console.error(`Error: ${errorMessage}`);
        
        // Determine appropriate exit code based on error type
        if (errorMessage.includes('Configuration')) {
          process.exit(1);
        } else if (errorMessage.includes('provider') || errorMessage.includes('model')) {
          process.exit(2);
        } else if (errorMessage.includes('tool')) {
          process.exit(3);
        } else {
          process.exit(1);
        }
      }

      // Save conversation if specified
      if (typeof options['save'] === 'string') {
        try {
          fs.writeFileSync(options['save'], JSON.stringify(result.conversation, null, 2), 'utf-8');
        } catch (error) {
          console.error(`Failed to save conversation to ${options['save']}: ${String(error)}`);
          process.exit(1);
        }
      }

      // Success - exit with 0 (stdout already contains LLM output)
      process.exit(0);

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      console.error(`Fatal error: ${errorMessage}`);
      
      // Determine exit code based on error type
      if (errorMessage.includes('Configuration') || errorMessage.includes('config')) {
        process.exit(1);
      } else if (errorMessage.includes('command line') || errorMessage.includes('argument')) {
        process.exit(4);
      } else if (errorMessage.includes('tool')) {
        process.exit(3);
      } else {
        process.exit(1);
      }
    }
  });

// Handle uncaught exceptions
process.on('uncaughtException', (error) => {
  console.error(`Uncaught exception: ${error.message}`);
  process.exit(1);
});

process.on('unhandledRejection', (reason) => {
  console.error(`Unhandled rejection: ${String(reason)}`);
  process.exit(1);
});

// Parse command line arguments
program.parse();