/**
 * Accounting Manager - Production Implementation
 * Handles token and tool usage tracking according to IMPLEMENTATION.md
 */

import * as fs from 'node:fs';
import * as path from 'node:path';

import type { AccountingEntry, AIAgentCallbacks, LLMAccountingEntry, TokenUsage, ToolAccountingEntry } from './types.js';

export class AccountingManager {
  private callbacks?: AIAgentCallbacks;
  private accountingFile?: string;

  constructor(callbacks?: AIAgentCallbacks, accountingFile?: string) {
    this.callbacks = callbacks;
    this.accountingFile = accountingFile;
  }

  /**
   * Log accounting entry using callback or JSONL file
   */
  private logEntry(entry: AccountingEntry): void {
    // Use callback if available per spec
    if (this.callbacks?.onAccounting !== undefined) {
      this.callbacks.onAccounting(entry);
      return;
    }

    // Otherwise write to JSONL file if configured
    if (this.accountingFile !== undefined) {
      try {
        // Ensure directory exists
        const dir = path.dirname(this.accountingFile);
        if (!fs.existsSync(dir)) {
          fs.mkdirSync(dir, { recursive: true });
        }

        // Append JSONL entry
        const jsonLine = JSON.stringify(entry) + '\n';
        fs.appendFileSync(this.accountingFile, jsonLine, 'utf-8');
      } catch (error: unknown) {
        const errorMessage = error instanceof Error ? error.message : String(error);
        console.error(`[accounting] Failed to write to ${this.accountingFile}: ${errorMessage}`);
      }
    }
  }

  /**
   * Log LLM usage per spec line 94
   */
  logLLMUsage(
    provider: string,
    model: string,
    tokens: TokenUsage,
    latency: number,
    status: 'ok' | 'failed',
    error?: string
  ): void {
    const entry: LLMAccountingEntry = {
      type: 'llm',
      timestamp: Date.now(),
      provider,
      model,
      tokens,
      latency,
      status,
      error,
    };

    this.logEntry(entry);
  }

  /**
   * Log tool usage per spec line 95
   */
  logToolUsage(
    toolName: string,
    mcpServer: string,
    _command: string,
    usage: {
      latency: number;
      charactersIn: number;
      charactersOut: number;
      success: boolean;
    },
    error?: string
  ): void {
    const entry: ToolAccountingEntry = {
      type: 'tool',
      timestamp: Date.now(),
      mcpServer,
      command: toolName, // Use toolName as command
      charactersIn: usage.charactersIn,
      charactersOut: usage.charactersOut,
      latency: usage.latency,
      status: usage.success ? 'ok' : 'failed',
      error,
    };

    this.logEntry(entry);
  }

}