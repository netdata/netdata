#!/usr/bin/env node

/**
 * @typedef {Object} OpenAIUsage
 * @property {number} prompt_tokens - Number of input tokens
 * @property {number} completion_tokens - Number of output tokens
 * @property {Object} [prompt_tokens_details] - Details about prompt tokens
 * @property {number} [prompt_tokens_details.cached_tokens] - Number of cached tokens
 */

/**
 * @typedef {Object} OpenAIResponse
 * @property {OpenAIUsage} [usage] - Token usage information
 */

/**
 * @typedef {Object} AnthropicUsage
 * @property {number} input_tokens - Number of input tokens
 * @property {number} output_tokens - Number of output tokens
 * @property {number} [cache_read_input_tokens] - Number of cached tokens read
 * @property {number} [cache_creation_input_tokens] - Number of tokens used to create cache
 */

/**
 * @typedef {Object} AnthropicResponse
 * @property {AnthropicUsage} [usage] - Token usage information
 */

/**
 * @typedef {Object} GoogleUsageMetadata
 * @property {number} promptTokenCount - Number of input tokens
 * @property {number} candidatesTokenCount - Number of output tokens
 */

/**
 * @typedef {Object} GoogleResponse
 * @property {GoogleUsageMetadata} [usageMetadata] - Token usage information
 */

/**
 * @typedef {Object} GoogleModel
 * @property {string} name - Model name with prefix
 * @property {string[]} [supportedGenerationMethods] - Supported generation methods
 * @property {number} [inputTokenLimit] - Input token limit
 */

// ============================================================================
// MODEL DEFINITIONS TABLE - Easy to find and edit
// ============================================================================
// Pricing is per million tokens (MTok)
// For Anthropic models: input, cacheWrite, cacheRead, output
// For OpenAI models: input, cacheRead, output  
// For Google models: input, output
const MODEL_DEFINITIONS = {
  // ==================== OPENAI MODELS ====================
  
  // GPT-5 Series (Latest Models)
  'gpt-5': { 
    contextWindow: 128000, 
    pricing: { input: 1.25, cacheRead: 0.125, output: 10.00 } 
  },
  'gpt-5-mini': { 
    contextWindow: 128000, 
    pricing: { input: 0.25, cacheRead: 0.025, output: 2.00 } 
  },
  'gpt-5-nano': {
    contextWindow: 128000,
    pricing: { input: 0.05, cacheRead: 0.005, output: 0.40 }
  },
  'gpt-5-chat-latest': {
    contextWindow: 128000,
    pricing: { input: 1.25, cacheRead: 0.125, output: 10.00 }
  },
  'gpt-5-pro': {
    contextWindow: 128000,
    pricing: { input: 15.00, cacheRead: 0.00, output: 120.00 }
  },
  'gpt-5-codex': {
    contextWindow: 128000,
    pricing: { input: 1.25, cacheRead: 0.125, output: 10.00 }
  },

  // GPT-4o Series
  'gpt-4o': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 1.25, output: 10.00 } 
  },
  'gpt-4o-mini': { 
    contextWindow: 128000, 
    pricing: { input: 0.15, cacheRead: 0.075, output: 0.60 } 
  },
  'gpt-4o-2024-05-13': { 
    contextWindow: 128000, 
    pricing: { input: 5.00, cacheRead: 0.00, output: 15.00 } 
  },
  'gpt-4o-mini-2024-07-18': { 
    contextWindow: 128000, 
    pricing: { input: 0.15, cacheRead: 0.075, output: 0.60 } 
  },
  'gpt-4o-2024-08-06': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 1.25, output: 10.00 } 
  },
  'gpt-4o-2024-11-20': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 1.25, output: 10.00 } 
  },
  'chatgpt-4o-latest': { 
    contextWindow: 128000, 
    pricing: { input: 5.00, cacheRead: 0.00, output: 15.00 }  // Legacy model
  },
  
  // GPT-4 Turbo Series (Legacy)
  'gpt-4-turbo': { 
    contextWindow: 128000, 
    pricing: { input: 10.00, cacheRead: 0.00, output: 30.00 } 
  },
  'gpt-4-turbo-preview': { 
    contextWindow: 128000, 
    pricing: { input: 10.00, cacheRead: 0.00, output: 30.00 } 
  },
  'gpt-4-turbo-2024-04-09': { 
    contextWindow: 128000, 
    pricing: { input: 10.00, cacheRead: 0.00, output: 30.00 } 
  },
  'gpt-4-0125-preview': { 
    contextWindow: 128000, 
    pricing: { input: 10.00, cacheRead: 0.00, output: 30.00 } 
  },
  'gpt-4-1106-preview': { 
    contextWindow: 128000, 
    pricing: { input: 10.00, cacheRead: 0.00, output: 30.00 } 
  },
  
  // GPT-4 Original (Legacy)
  'gpt-4': { 
    contextWindow: 8192, 
    pricing: { input: 30.00, cacheRead: 0.00, output: 60.00 } 
  },
  'gpt-4-0613': { 
    contextWindow: 8192, 
    pricing: { input: 30.00, cacheRead: 0.00, output: 60.00 } 
  },
  'gpt-4-0314': { 
    contextWindow: 8192, 
    pricing: { input: 30.00, cacheRead: 0.00, output: 60.00 } 
  },
  // GPT-4-32k models - Legacy (very expensive)
  // 'gpt-4-32k': { 
  //   contextWindow: 32768, 
  //   pricing: { input: 60.00, cacheRead: 0.00, output: 120.00 } 
  // },
  // 'gpt-4-32k-0613': { 
  //   contextWindow: 32768, 
  //   pricing: { input: 60.00, cacheRead: 0.00, output: 120.00 } 
  // },
  
  // GPT-4.5 Series - DEPRECATED (Not in current pricing)
  // 'gpt-4.5-preview': { 
  //   contextWindow: 128000, 
  //   pricing: { input: 75.00, cacheRead: 37.50, output: 150.00 } 
  // },
  // 'gpt-4.5-preview-2025-02-27': { 
  //   contextWindow: 128000, 
  //   pricing: { input: 75.00, cacheRead: 37.50, output: 150.00 } 
  // },
  
  // GPT-4.1 Series (New models with 1M context)
  'gpt-4.1': { 
    contextWindow: 1000000, 
    pricing: { input: 2.00, cacheRead: 0.50, output: 8.00 } 
  },
  'gpt-4.1-2025-04-14': { 
    contextWindow: 1000000, 
    pricing: { input: 2.00, cacheRead: 0.50, output: 8.00 } 
  },
  'gpt-4.1-mini': { 
    contextWindow: 1000000, 
    pricing: { input: 0.40, cacheRead: 0.10, output: 1.60 } 
  },
  'gpt-4.1-mini-2025-04-14': { 
    contextWindow: 1000000, 
    pricing: { input: 0.40, cacheRead: 0.10, output: 1.60 } 
  },
  'gpt-4.1-nano': { 
    contextWindow: 1000000, 
    pricing: { input: 0.10, cacheRead: 0.025, output: 0.40 } 
  },
  'gpt-4.1-nano-2025-04-14': { 
    contextWindow: 1000000, 
    pricing: { input: 0.10, cacheRead: 0.025, output: 0.40 } 
  },
  
  // GPT-3.5 Turbo Series
  'gpt-3.5-turbo': { 
    contextWindow: 16384, 
    pricing: { input: 0.50, cacheRead: 0.00, output: 1.50 } 
  },
  'gpt-3.5-turbo-16k': { 
    contextWindow: 16384, 
    pricing: { input: 3.00, cacheRead: 0.00, output: 4.00 }  // Legacy
  },
  'gpt-3.5-turbo-16k-0613': { 
    contextWindow: 16384, 
    pricing: { input: 3.00, cacheRead: 0.00, output: 4.00 }  // Legacy
  },
  'gpt-3.5-turbo-0125': { 
    contextWindow: 16384, 
    pricing: { input: 0.50, cacheRead: 0.00, output: 1.50 } 
  },
  'gpt-3.5-turbo-1106': { 
    contextWindow: 16384, 
    pricing: { input: 1.00, cacheRead: 0.00, output: 2.00 } 
  },
  'gpt-3.5-turbo-0613': { 
    contextWindow: 4096, 
    pricing: { input: 1.50, cacheRead: 0.00, output: 2.00 }  // Legacy
  },
  'gpt-3.5-turbo-0301': { 
    contextWindow: 4096, 
    pricing: { input: 1.50, cacheRead: 0.00, output: 2.00 }  // Legacy
  },
  'gpt-3.5-turbo-instruct': { 
    contextWindow: 4096, 
    pricing: { input: 1.50, cacheRead: 0.00, output: 2.00 } 
  },
  'gpt-3.5-turbo-instruct-0914': { 
    contextWindow: 4096, 
    pricing: { input: 1.50, cacheRead: 0.00, output: 2.00 } 
  },
  
  
  // Search Models
  'gpt-4o-search-preview': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 0.00, output: 10.00 } 
  },
  'gpt-4o-search-preview-2025-03-11': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 0.00, output: 10.00 } 
  },
  'gpt-4o-mini-search-preview': { 
    contextWindow: 128000, 
    pricing: { input: 0.15, cacheRead: 0.00, output: 0.60 } 
  },
  'gpt-4o-mini-search-preview-2025-03-11': { 
    contextWindow: 128000, 
    pricing: { input: 0.15, cacheRead: 0.00, output: 0.60 } 
  },
  
  // Specialized Models
  
  // Legacy Models
  'davinci-002': { 
    contextWindow: 16384, 
    pricing: { input: 2.00, cacheRead: 0.00, output: 2.00 } 
  },
  'babbage-002': { 
    contextWindow: 16384, 
    pricing: { input: 0.40, cacheRead: 0.00, output: 0.40 } 
  },
  
  // o1 Series (Reasoning Models) - use /v1/responses endpoint, no tool support
  'o1': { 
    contextWindow: 200000, 
    pricing: { input: 15.00, cacheRead: 7.50, output: 60.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-2024-12-17': { 
    contextWindow: 200000, 
    pricing: { input: 15.00, cacheRead: 7.50, output: 60.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-preview': { 
    contextWindow: 128000, 
    pricing: { input: 15.00, cacheRead: 7.50, output: 60.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-preview-2024-09-12': { 
    contextWindow: 128000, 
    pricing: { input: 15.00, cacheRead: 7.50, output: 60.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-mini': { 
    contextWindow: 128000, 
    pricing: { input: 1.10, cacheRead: 0.55, output: 4.40 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-mini-2024-09-12': { 
    contextWindow: 128000, 
    pricing: { input: 1.10, cacheRead: 0.55, output: 4.40 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-pro': { 
    contextWindow: 200000, 
    pricing: { input: 150.00, cacheRead: 0.00, output: 600.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  'o1-pro-2025-03-19': { 
    contextWindow: 200000, 
    pricing: { input: 150.00, cacheRead: 0.00, output: 600.00 },
    endpoint: 'responses',
    supportsTools: false
  },
  
  // o3 Series - use /v1/responses endpoint, with tool support
  'o3': { 
    contextWindow: 200000, 
    pricing: { input: 2.00, cacheRead: 0.50, output: 8.00 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-2025-04-16': { 
    contextWindow: 200000, 
    pricing: { input: 2.00, cacheRead: 0.50, output: 8.00 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-deep-research': { 
    contextWindow: 200000, 
    pricing: { input: 10.00, cacheRead: 2.50, output: 40.00 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-mini': { 
    contextWindow: 200000, 
    pricing: { input: 1.10, cacheRead: 0.55, output: 4.40 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-mini-2025-01-31': { 
    contextWindow: 200000, 
    pricing: { input: 1.10, cacheRead: 0.55, output: 4.40 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-pro': { 
    contextWindow: 200000, 
    pricing: { input: 20.00, cacheRead: 0.00, output: 80.00 },
    endpoint: 'responses',
    supportsTools: true
  },
  'o3-pro-2025-06-10': { 
    contextWindow: 200000, 
    pricing: { input: 20.00, cacheRead: 0.00, output: 80.00 },
    endpoint: 'responses',
    supportsTools: true
  },
  
  // o4 Series
  'o4-mini': { 
    contextWindow: 200000, 
    pricing: { input: 1.10, cacheRead: 0.275, output: 4.40 } 
  },
  'o4-mini-2025-04-16': { 
    contextWindow: 200000, 
    pricing: { input: 1.10, cacheRead: 0.275, output: 4.40 } 
  },
  'o4-mini-deep-research': { 
    contextWindow: 200000, 
    pricing: { input: 2.00, cacheRead: 0.50, output: 8.00 } 
  },
  
  // Specialized Models
  'codex-mini-latest': { 
    contextWindow: 128000, 
    pricing: { input: 1.50, cacheRead: 0.375, output: 6.00 } 
  },
  'computer-use-preview': { 
    contextWindow: 128000, 
    pricing: { input: 3.00, cacheRead: 0.00, output: 12.00 } 
  },
  'computer-use-preview-2025-03-11': { 
    contextWindow: 128000, 
    pricing: { input: 3.00, cacheRead: 0.00, output: 12.00 } 
  },
  
  // Audio Models
  'gpt-4o-audio-preview': { 
    contextWindow: 128000, 
    pricing: { input: 2.50, cacheRead: 0.00, output: 10.00 } 
  },
  'gpt-4o-realtime-preview': { 
    contextWindow: 128000, 
    pricing: { input: 5.00, cacheRead: 2.50, output: 20.00 } 
  },
  'gpt-4o-mini-audio-preview': { 
    contextWindow: 128000, 
    pricing: { input: 0.15, cacheRead: 0.00, output: 0.60 } 
  },
  'gpt-4o-mini-realtime-preview': { 
    contextWindow: 128000, 
    pricing: { input: 0.60, cacheRead: 0.30, output: 2.40 } 
  },
  
  // Image Model
  'gpt-image-1': { 
    contextWindow: 128000, 
    pricing: { input: 5.00, cacheRead: 1.25, output: 0.00 } 
  },
  
  // ==================== ANTHROPIC MODELS ====================
  
  // Claude Opus 4
  'claude-opus-4-20250514': { 
    contextWindow: 200000, 
    pricing: { input: 15.00, cacheWrite: 18.75, cacheRead: 1.50, output: 75.00 } 
  },
  
  // Claude Sonnet 4
  'claude-sonnet-4-20250514': {
    contextWindow: 200000,
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 }
  },

  // Claude 4.5 Series
  'claude-sonnet-4-5': {
    contextWindow: 200000,
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 }
  },
  'claude-haiku-4-5': {
    contextWindow: 200000,
    pricing: { input: 1.00, cacheWrite: 1.25, cacheRead: 0.10, output: 5.00 }
  },

  // Claude Sonnet 3.7
  'claude-3-7-sonnet-20250219': { 
    contextWindow: 200000, 
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 } 
  },
  
  // Claude 3.5 Series
  'claude-3-5-haiku-20241022': { 
    contextWindow: 200000, 
    pricing: { input: 0.80, cacheWrite: 1.00, cacheRead: 0.08, output: 4.00 } 
  },
  'claude-3-5-sonnet-20241022': { 
    contextWindow: 200000, 
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 } 
  },
  'claude-3-5-sonnet-20240620': { 
    contextWindow: 200000, 
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 } 
  },
  
  // Claude 3 Series
  'claude-3-opus-20240229': { 
    contextWindow: 200000, 
    pricing: { input: 15.00, cacheWrite: 18.75, cacheRead: 1.50, output: 75.00 } 
  },
  'claude-3-sonnet-20240229': { 
    contextWindow: 200000, 
    pricing: { input: 3.00, cacheWrite: 3.75, cacheRead: 0.30, output: 15.00 } 
  },
  'claude-3-haiku-20240307': { 
    contextWindow: 200000, 
    pricing: { input: 0.25, cacheWrite: 0.30, cacheRead: 0.03, output: 1.25 } 
  },
  
  // ==================== GOOGLE GEMINI MODELS ====================
  
  // Gemini 2.0 Series - DEPRECATED (Not in current pricing)
  // 'gemini-2.0-flash': { 
  //   contextWindow: 1048576, 
  //   pricing: { input: 0.10, output: 0.40 } 
  // },
  // 'gemini-2.0-flash-001': { 
  //   contextWindow: 1048576, 
  //   pricing: { input: 0.10, output: 0.40 } 
  // },
  // 'gemini-2.0-flash-lite': { 
  //   contextWindow: 1048576, 
  //   pricing: { input: 0.075, output: 0.30 } 
  // },
  // 'gemini-2.0-flash-lite-001': { 
  //   contextWindow: 1048576, 
  //   pricing: { input: 0.075, output: 0.30 } 
  // },
  'gemini-2.0-flash-exp': { 
    contextWindow: 1000000, 
    pricing: { input: 0.00, output: 0.00 } // Free experimental
  },
  'gemini-2.0-flash-thinking-exp-1219': { 
    contextWindow: 1000000, 
    pricing: { input: 0.00, output: 0.00 } // Free experimental
  },
  'gemini-2.0-flash-thinking-exp-01-21': { 
    contextWindow: 1000000, 
    pricing: { input: 0.00, output: 0.00 } // Free experimental
  },
  
  // Gemini 2.5 Series (Latest Models)
  'gemini-2.5-pro': { 
    contextWindow: 2000000, 
    pricing: { input: 1.25, output: 10.00 }  // For prompts <= 200k tokens
  },
  'gemini-2.5-flash': { 
    contextWindow: 1000000, 
    pricing: { input: 0.30, output: 2.50 }  // Text/image/video pricing
  },
  'gemini-2.5-flash-lite': { 
    contextWindow: 1000000, 
    pricing: { input: 0.10, output: 0.40 }  // Text/image/video pricing
  },
  
  // Gemini 1.5 Series
  'gemini-1.5-pro': { 
    contextWindow: 2000000, 
    pricing: { input: 1.25, output: 5.00 } 
  },
  'gemini-1.5-pro-001': { 
    contextWindow: 2000000, 
    pricing: { input: 1.25, output: 5.00 } 
  },
  'gemini-1.5-pro-002': { 
    contextWindow: 2000000, 
    pricing: { input: 1.25, output: 5.00 } 
  },
  'gemini-1.5-flash': { 
    contextWindow: 1000000, 
    pricing: { input: 0.075, output: 0.30 } 
  },
  'gemini-1.5-flash-001': { 
    contextWindow: 1000000, 
    pricing: { input: 0.075, output: 0.30 } 
  },
  'gemini-1.5-flash-002': { 
    contextWindow: 1000000, 
    pricing: { input: 0.075, output: 0.30 } 
  },
  // 'gemini-1.5-flash-8b': {  // DEPRECATED - Not in current pricing
  //   contextWindow: 1000000, 
  //   pricing: { input: 0.0375, output: 0.15 } // Pricing for prompts <= 128k
  // },
  // 'gemini-1.5-flash-8b-001': {  // DEPRECATED - Not in current pricing
  //   contextWindow: 1000000, 
  //   pricing: { input: 0.0375, output: 0.15 } 
  // },
  // 'gemini-1.5-flash-001-tuning': {  // DEPRECATED - Not in current pricing
  //   contextWindow: 16384, 
  //   pricing: { input: 0.00, output: 0.00 } // Free for tuning
  // },
  
  // Gemini 1.0 Series - DEPRECATED (Not in current pricing)
  // 'gemini-pro': { 
  //   contextWindow: 32760, 
  //   pricing: { input: 0.50, output: 1.50 } 
  // },
  // 'gemini-pro-vision': { 
  //   contextWindow: 32760, 
  //   pricing: { input: 0.50, output: 1.50 } 
  // },
  // 'gemini-1.0-pro': { 
  //   contextWindow: 32760, 
  //   pricing: { input: 0.50, output: 1.50 } 
  // },
  // 'gemini-1.0-pro-vision-latest': { 
  //   contextWindow: 12288, 
  //   pricing: { input: 0.50, output: 1.50 } 
  // }
  
  // ==================== DEEPSEEK MODELS ====================
  // DeepSeek uses OpenAI-compatible API with cache support
  // New pricing starting Sept 5, 2025 at 16:00 UTC
  
  'deepseek-chat': {
    contextWindow: 128000,
    pricing: { 
      input: 0.56,      // cache miss
      cacheRead: 0.07,  // cache hit
      output: 1.68 
    }
  },
  'deepseek-reasoner': {
    contextWindow: 128000,
    pricing: { 
      input: 0.56,      // cache miss  
      cacheRead: 0.07,  // cache hit (same for both models)
      output: 1.68      // same price as deepseek-chat
    }
  }
};

const http = require('http');
const https = require('https');
const url = require('url');
const fs = require('fs');
const path = require('path');
const os = require('os');
const zlib = require('zlib');

// Display startup banner
console.log('='.repeat(60));
console.log('LLM Proxy Server & MCP Web Client');
console.log('='.repeat(60));

// Accounting log file path
const ACCOUNTING_DIR = path.join(process.cwd(), 'logs');
const ACCOUNTING_FILE = path.join(ACCOUNTING_DIR, `llm-accounting-${new Date().toISOString().split('T')[0]}.jsonl`);

// Configuration file path in current working directory
const CONFIG_FILE = path.join(process.cwd(), 'llm-proxy-config.json');

// Helper function to generate models list from MODEL_DEFINITIONS
function generateModelsForProvider(provider) {
  const models = [];
  
  // Define which models belong to which provider
  // Since MODEL_DEFINITIONS is organized by provider sections, we'll check model prefixes
  const providerPrefixes = {
    openai: ['gpt', 'o1', 'o3', 'o4', 'davinci', 'chatgpt', 'codex', 'computer-use'],
    anthropic: ['claude'],
    google: ['gemini'],
    deepseek: ['deepseek']
  };
  
  const prefixes = providerPrefixes[provider.toLowerCase()];
  if (!prefixes) return models;
  
  Object.entries(MODEL_DEFINITIONS).forEach(([modelId, definition]) => {
    // Check if this model belongs to the requested provider
    const belongsToProvider = prefixes.some(prefix => 
      modelId.startsWith(prefix) || modelId.includes('-' + prefix + '-')
    );
    
    if (belongsToProvider) {
      models.push({
        id: modelId,
        contextWindow: definition.contextWindow,
        pricing: definition.pricing,
        endpoint: definition.endpoint || 'completions',
        supportsTools: definition.supportsTools !== false // Default to true unless explicitly false
      });
    }
  });
  
  return models;
}

const DEFAULT_CONFIG = {
  port: 8081,
  allowedOrigins: '*',
  providers: {
    openai: {
      apiKey: '',
      baseUrl: 'https://api.openai.com',
      type: 'openai',
      models: generateModelsForProvider('openai').slice(0, 7) // Include a subset for initial config
    },
    anthropic: {
      apiKey: '',
      baseUrl: 'https://api.anthropic.com',
      type: 'anthropic',
      models: generateModelsForProvider('anthropic')
    },
    google: {
      apiKey: '',
      baseUrl: 'https://generativelanguage.googleapis.com',
      type: 'google',
      models: generateModelsForProvider('google').slice(0, 10) // Include a subset for initial config
    },
    deepseek: {
      apiKey: '',
      baseUrl: 'https://api.deepseek.com',
      type: 'openai', // DeepSeek uses OpenAI-compatible API
      models: generateModelsForProvider('deepseek')
    },
    ollama: {
      apiKey: '',  // Not used, but kept for config consistency
      baseUrl: 'http://localhost:11434',
      type: 'ollama',
      models: [] // Configure using --update-config --sync --check-availability
    }
  },
  mcpServers: [
    // Example MCP server configuration
    // { id: 'local_netdata', name: 'Local Netdata', url: 'ws://localhost:19999/mcp?api_key=YOUR_API_KEY' }
  ]
};

// LLM Provider configurations
const LLM_PROVIDERS = {
  anthropic: {
    baseUrl: 'https://api.anthropic.com',
    authHeader: 'x-api-key'
  },
  openai: {
    baseUrl: 'https://api.openai.com',
    authHeader: 'Authorization',
    authPrefix: 'Bearer '
  },
  google: {
    baseUrl: 'https://generativelanguage.googleapis.com',
    authHeader: null // Google uses API key in URL
  },
  deepseek: {
    baseUrl: 'https://api.deepseek.com',
    authHeader: 'Authorization',
    authPrefix: 'Bearer ' // DeepSeek uses OpenAI-compatible auth
  },
  ollama: {
    baseUrl: 'http://localhost:11434',
    authHeader: null // Ollama typically runs locally without authentication
  }
};

// Load or create configuration
function loadConfig() {
  console.log('\n📁 Configuration:');
  console.log(`   Config file: ${CONFIG_FILE}`);

  if (!fs.existsSync(CONFIG_FILE)) {
    console.log('\n🆕 First time setup detected!');
    console.log('   Creating default configuration file...');
    fs.writeFileSync(CONFIG_FILE, JSON.stringify(DEFAULT_CONFIG, null, 2));
    console.log('\n✅ Configuration file created successfully!');
    console.log('\n📝 Next steps:');
    console.log(`   1. Edit the configuration file: ${CONFIG_FILE}`);
    console.log('   2. Add your API keys for the LLM providers you want to use:');
    console.log('      - OpenAI: Add your API key starting with "sk-"');
    console.log('      - Anthropic: Add your API key starting with "sk-ant-"');
    console.log('      - Google: Add your API key for Gemini');
    console.log('      - DeepSeek: Add your API key from api.deepseek.com');
    console.log('   3. (Optional) Add MCP servers to the mcpServers array:');
    console.log('      - id: Unique identifier for the server');
    console.log('      - name: Display name for the server');
    console.log('      - url: WebSocket URL (e.g., ws://localhost:19999/mcp?api_key=...)');
    console.log('   4. Customize pricing if needed (prices are per million tokens):');
    console.log('      - input: Regular input token cost');
    console.log('      - cacheRead: Cached input token cost (discounted)');
    console.log('      - cacheWrite: Cache creation cost (Anthropic only, 25% surcharge)');
    console.log('      - output: Output token cost');
    console.log('   5. Save the file and restart this server');
    console.log('\n💡 Tip: You can use any text editor to edit the configuration file');
    console.log('   Example: nano ' + CONFIG_FILE);
    console.log('\n');
    process.exit(0);
  }

  try {
    console.log('   Loading configuration...');
    const config = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
    
    // Validate all models in configuration
    let hasValidModels = false;
    const validationErrors = [];
    
    Object.entries(config.providers).forEach(([provider, settings]) => {
      if (settings.apiKey && settings.apiKey.length > 0) {
        if (!settings.models || !Array.isArray(settings.models)) {
          validationErrors.push(`${provider}: No models array defined`);
          return;
        }
        
        settings.models.forEach((model, index) => {
          if (typeof model === 'string') {
            validationErrors.push(`${provider} model[${index}]: String format not supported, must be object with id, contextWindow, and pricing`);
            return;
          }
          
          const modelId = model?.id || `index ${index}`;
          const providerType = settings.type || provider;
          const error = validateModelConfig(provider, model, providerType);
          if (error) {
            validationErrors.push(`${provider} model "${modelId}": ${error}`);
          } else {
            hasValidModels = true;
          }
        });
      }
    });
    
    if (validationErrors.length > 0) {
      console.error('\n❌ Configuration validation errors:');
      validationErrors.forEach(error => console.error(`   - ${error}`));
      
      if (!hasValidModels) {
        console.error('\n❌ No valid models found in configuration!');
        console.error('   Please fix the errors above and restart the server.');
        process.exit(1);
      } else {
        console.error('\n⚠️  Some models have errors and will be unavailable.');
      }
    }
    
    // Check if any API keys are configured
    const configuredProviders = [];
    let totalValidModels = 0;
    
    Object.entries(config.providers).forEach(([provider, settings]) => {
      // Get provider type to determine API key requirements
      const providerType = settings.type || provider;
      const requiresApiKey = providerType !== 'ollama';
      
      if (!requiresApiKey || (settings.apiKey && settings.apiKey.length > 0)) {
        const validModels = (settings.models || []).filter((model) => {
          if (typeof model === 'string') return false;
          const modelProviderType = settings.type || provider;
          return !validateModelConfig(provider, model, modelProviderType);
        });
        
        if (validModels.length > 0) {
          configuredProviders.push(`${provider} (${validModels.length} valid models)`);
          totalValidModels += validModels.length;
        } else if (provider === 'ollama') {
          // Ollama with no models configured
          configuredProviders.push(`${provider} (0 models - use --update-config to discover)`);
        }
      }
    });
    
    // Skip validation if we're updating config, showing models, or showing help
    const isUpdatingConfig = process.argv.includes('--update-config');
    const isShowingModels = process.argv.includes('--show-models');
    const isShowingHelp = process.argv.includes('--help') || process.argv.includes('-h');
    if (!isUpdatingConfig && !isShowingModels && !isShowingHelp && (configuredProviders.length === 0 || totalValidModels === 0)) {
      console.error('\n❌ Error: No valid models configured!');
      console.error('\n📝 Please edit the configuration file and add valid models:');
      console.error(`   ${CONFIG_FILE}`);
      console.error('\nExample model configuration:');
      console.error('```json');
      console.error('{');
      console.error('  "providers": {');
      console.error('    "openai": {');
      console.error('      "apiKey": "sk-YOUR-API-KEY-HERE",');
      console.error('      "models": [');
      console.error('        {');
      console.error('          "id": "gpt-4",');
      console.error('          "contextWindow": 8192,');
      console.error('          "pricing": {');
      console.error('            "input": 30.0,');
      console.error('            "output": 60.0,');
      console.error('            "cacheRead": 30.0');
      console.error('          }');
      console.error('        }');
      console.error('      ]');
      console.error('    }');
      console.error('  }');
      console.error('}');
      console.error('```\n');
      process.exit(1);
    }
    
    console.log('   ✅ Configuration loaded successfully!');
    console.log(`   Configured providers: ${configuredProviders.join(', ')}`);
    console.log(`   Total valid models available: ${totalValidModels}`);

    return config;
  } catch (error) {
    console.error('\n❌ Error reading configuration file:', error.message);
    console.error('   Please check that the file is valid JSON format');
    process.exit(1);
  }
}

// Validate model configuration
function validateModelConfig(provider, model, providerType = null) {
  // Check if model has required structure
  if (typeof model !== 'object' || !model.id || !model.contextWindow) {
    return `Missing required fields (id, contextWindow)`;
  }
  
  // Determine the provider type (use providerType if provided, otherwise use provider)
  const type = providerType || provider.toLowerCase();
  
  // Ollama models may have null pricing
  if (type === 'ollama' && model.pricing === null) {
    return null; // Valid for Ollama
  }
  
  if (!model.pricing) {
    return `Missing required field: pricing`;
  }
  
  // Validate context window
  if (typeof model.contextWindow !== 'number' || model.contextWindow <= 0) {
    return `Invalid contextWindow: must be a positive number`;
  }
  
  // Validate pricing structure
  if (typeof model.pricing !== 'object') {
    return `Invalid pricing: must be an object`;
  }
  
  const pricing = model.pricing;
  
  // Check for required pricing fields based on provider type
  switch (type) {
    case 'google':
      // Google requires only input and output
      if (typeof pricing.input !== 'number' || pricing.input < 0) {
        return `Invalid pricing.input: must be a number >= 0`;
      }
      if (typeof pricing.output !== 'number' || pricing.output < 0) {
        return `Invalid pricing.output: must be a number >= 0`;
      }
      // Google should NOT have cache fields
      if ('cacheRead' in pricing || 'cacheWrite' in pricing) {
        return `Invalid pricing: Google models should not have cacheRead or cacheWrite`;
      }
      break;
      
    case 'openai':
      // OpenAI and DeepSeek (OpenAI-compatible) require input, output, and cacheRead
      if (typeof pricing.input !== 'number' || pricing.input < 0) {
        return `Invalid pricing.input: must be a number >= 0`;
      }
      if (typeof pricing.output !== 'number' || pricing.output < 0) {
        return `Invalid pricing.output: must be a number >= 0`;
      }
      if (typeof pricing.cacheRead !== 'number' || pricing.cacheRead < 0) {
        return `Invalid pricing.cacheRead: must be a number >= 0`;
      }
      // OpenAI should NOT have cacheWrite
      if ('cacheWrite' in pricing) {
        return `Invalid pricing: OpenAI models should not have cacheWrite`;
      }
      break;
      
    case 'anthropic':
      // Anthropic requires all four pricing fields
      if (typeof pricing.input !== 'number' || pricing.input < 0) {
        return `Invalid pricing.input: must be a number >= 0`;
      }
      if (typeof pricing.output !== 'number' || pricing.output < 0) {
        return `Invalid pricing.output: must be a number >= 0`;
      }
      if (typeof pricing.cacheRead !== 'number' || pricing.cacheRead < 0) {
        return `Invalid pricing.cacheRead: must be a number >= 0`;
      }
      if (typeof pricing.cacheWrite !== 'number' || pricing.cacheWrite < 0) {
        return `Invalid pricing.cacheWrite: must be a number >= 0`;
      }
      break;
      
    default:
      return `Unknown provider: ${provider}`;
  }
  
  return null; // Valid
}


// Load configuration
const config = loadConfig();
const PROXY_PORT = config.port || 8081;
const ALLOWED_ORIGINS = config.allowedOrigins || '*';

// Ensure accounting directory exists
if (!fs.existsSync(ACCOUNTING_DIR)) {
  try {
    fs.mkdirSync(ACCOUNTING_DIR, { recursive: true });
    console.log(`\n📊 Created accounting directory: ${ACCOUNTING_DIR}`);
  } catch (error) {
    console.error(`\n❌ Failed to create accounting directory: ${ACCOUNTING_DIR}`);
    console.error(`   Error: ${error.message}`);
    process.exit(1);
  }
}

// Function to write accounting entry
function writeAccountingEntry(entry) {
  try {
    fs.appendFileSync(ACCOUNTING_FILE, JSON.stringify(entry) + '\n');
  } catch (error) {
    console.error('❌ Failed to write accounting entry:', error.message);
    
    // Try to write to stderr as a fallback
    console.error('📊 ACCOUNTING_FALLBACK:', JSON.stringify(entry));
    
    // Optionally, try to write to a backup location
    try {
      const backupFile = path.join(os.tmpdir(), 'llm-accounting-backup.jsonl');
      fs.appendFileSync(backupFile, JSON.stringify(entry) + '\n');
      console.error(`   ✓ Written to backup file: ${backupFile}`);
    } catch (backupError) {
      console.error('   ✗ Backup write also failed:', backupError.message);
    }
  }
}

/**
 * Function to extract token usage from provider response
 * @param {string} provider - The LLM provider name
 * @param {OpenAIResponse|AnthropicResponse|GoogleResponse} responseData - Response data from the provider
 * @returns {Object} Token usage information
 */
function extractTokenUsage(provider, responseData) {
  try {
    switch (provider.toLowerCase()) {
      case 'openai':
        // Handle both standard OpenAI format and o3/o1 reasoning model format
        return {
          promptTokens: responseData.usage?.prompt_tokens || responseData.usage?.input_tokens || 0,
          completionTokens: responseData.usage?.completion_tokens || responseData.usage?.output_tokens || 0,
          cachedTokens: responseData.usage?.prompt_tokens_details?.cached_tokens || responseData.usage?.input_tokens_details?.cached_tokens || 0,
          // OpenAI doesn't report cache creation separately
          cacheCreationTokens: 0,
          // Add reasoning tokens for o3/o1 models (included in output_tokens)
          reasoningTokens: responseData.usage?.output_tokens_details?.reasoning_tokens || 0
        };
      
      case 'anthropic':
        return {
          promptTokens: responseData.usage?.input_tokens || 0,
          completionTokens: responseData.usage?.output_tokens || 0,
          cachedTokens: responseData.usage?.cache_read_input_tokens || 0,
          cacheCreationTokens: responseData.usage?.cache_creation_input_tokens || 0
        };
      
      case 'google':
        // Google Gemini token reporting
        return {
          promptTokens: responseData.usageMetadata?.promptTokenCount || 0,
          completionTokens: responseData.usageMetadata?.candidatesTokenCount || 0,
          // Google doesn't have cache tokens
          cachedTokens: 0,
          cacheCreationTokens: 0
        };
      
      case 'ollama':
        // Ollama token reporting
        return {
          promptTokens: responseData.prompt_eval_count || 0,
          completionTokens: responseData.eval_count || 0,
          // Ollama doesn't have cache tokens
          cachedTokens: 0,
          cacheCreationTokens: 0
        };
      
      default:
        return {
          promptTokens: 0,
          completionTokens: 0,
          cachedTokens: 0,
          cacheCreationTokens: 0
        };
    }
  } catch (error) {
    console.error('❌ Failed to extract token usage:', error.message);
    return {
      promptTokens: 0,
      completionTokens: 0,
      cachedTokens: 0,
      cacheCreationTokens: 0
    };
  }
}

// Function to calculate costs based on tokens and pricing
function calculateCosts(tokens, pricing) {
  if (!pricing) {
    return {
      inputCost: 0,
      outputCost: 0,
      cacheReadCost: 0,
      cacheWriteCost: 0,
      totalCost: 0
    };
  }
  
  // Costs = tokens * price per million / 1,000,000
  const inputCost = (tokens.promptTokens * (pricing.input || 0)) / 1000000;
  const outputCost = (tokens.completionTokens * (pricing.output || 0)) / 1000000;
  const cacheReadCost = (tokens.cachedTokens * (pricing.cacheRead || pricing.input || 0)) / 1000000;
  const cacheWriteCost = (tokens.cacheCreationTokens * (pricing.cacheWrite || pricing.input || 0)) / 1000000;
  
  return {
    inputCost,
    outputCost,
    cacheReadCost,
    cacheWriteCost,
    totalCost: inputCost + outputCost + cacheReadCost + cacheWriteCost
  };
}

/**
 * Fetch available models from provider APIs
 * @param {string} provider - The provider name (openai, anthropic, google)
 * @param {string} apiKey - API key for the provider
 * @param {Object} providerConfig - Provider configuration including baseUrl
 * @returns {Promise<Array|null>} Array of available models or null
 */
async function fetchAvailableModels(provider, apiKey, providerConfig = null) {
  // Get provider type from config or use provider as fallback
  const providerType = providerConfig?.type || provider;
  
  // Ollama doesn't require an API key
  if (!apiKey && providerType !== 'ollama') return null;
  
  console.log(`   🔍 Fetching available models from ${provider}...`);
  
  try {
    switch (providerType) {
      case 'openai':
      case 'openai-responses': {
        // Use provider's base URL from config
        const baseUrl = providerConfig?.baseUrl || LLM_PROVIDERS[provider]?.baseUrl || 'https://api.openai.com';
        const modelsUrl = new URL('/v1/models', baseUrl);
        
        // Build headers using the same logic as chat endpoints
        const headers = {
          'Content-Type': 'application/json'
        };
        
        // Add authentication headers based on provider type (matching chat endpoint logic)
        const providerUrlConfig = LLM_PROVIDERS[provider];
        if (providerUrlConfig && providerUrlConfig.authHeader) {
          if (providerUrlConfig.authPrefix) {
            headers[providerUrlConfig.authHeader] = providerUrlConfig.authPrefix + apiKey;
          } else {
            headers[providerUrlConfig.authHeader] = apiKey;
          }
        } else if (providerType === 'openai' || providerType === 'openai-responses') {
          // Default OpenAI-style authentication
          headers.Authorization = 'Bearer ' + apiKey;
        } else if (providerType === 'anthropic') {
          // Default Anthropic-style authentication
          headers['x-api-key'] = apiKey;
        }
        
        const options = {
          hostname: modelsUrl.hostname,
          port: modelsUrl.port || (modelsUrl.protocol === 'https:' ? 443 : 80),
          path: modelsUrl.pathname,
          method: 'GET',
          headers
        };
        
        return new Promise((resolve) => {
          const req = https.request(options, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
              if (res.statusCode !== 200) {
                console.log(`   ⚠️  Failed to fetch models from ${provider}: HTTP ${res.statusCode}`);
                if (data) {
                  try {
                    const error = JSON.parse(data);
                    if (error.error?.message) {
                      console.log(`      Error: ${error.error.message}`);
                    }
                  } catch (_e) {
                    // Ignore JSON parse errors for error response
                  }
                }
                resolve(null);
                return;
              }
              
              try {
                const response = JSON.parse(data);
                const models = response.data
                  .filter(model => {
                    // Filter for chat/completion models based on provider
                    if (provider === 'deepseek') {
                      // For DeepSeek, include their specific models
                      return model.id.includes('deepseek');
                    } else {
                      // For OpenAI and other OpenAI-compatible providers
                      return model.id.includes('gpt') || 
                             model.id.includes('davinci') ||
                             model.id.includes('turbo') ||
                             model.id.includes('o1') ||
                             model.id.includes('o3') ||
                             model.id.includes('o4');
                    }
                  })
                  .map(model => ({
                    id: model.id,
                    contextWindow: MODEL_DEFINITIONS[model.id]?.contextWindow || 4096,
                    pricing: MODEL_DEFINITIONS[model.id]?.pricing || null
                  }));
                
                console.log(`   ✅ Found ${models.length} available models from ${provider}`);
                resolve(models);
              } catch (e) {
                console.log(`   ⚠️  Error parsing response from ${provider}: ${e.message}`);
                resolve(null);
              }
            });
          });
          
          req.on('error', (e) => {
            console.log(`   ⚠️  Error fetching models from ${provider}: ${e.message}`);
            resolve(null);
          });
          
          req.end();
        });
      }
      
      case 'anthropic': {
        // Anthropic doesn't have a models endpoint, return null
        console.log(`   ℹ️  ${provider} doesn't provide a models endpoint`);
        return null;
      }
      
      case 'google': {
        // Google Gemini models endpoint
        const baseUrl = providerConfig?.baseUrl || LLM_PROVIDERS[provider]?.baseUrl || 'https://generativelanguage.googleapis.com';
        const targetUrl = new URL('/v1/models', baseUrl);
        targetUrl.searchParams.append('key', apiKey);
        
        const options = {
          hostname: targetUrl.hostname,
          port: 443,
          path: targetUrl.pathname + targetUrl.search,
          method: 'GET',
          headers: {
            'Content-Type': 'application/json'
          }
        };
        
        return new Promise((resolve) => {
          const req = https.request(options, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
              if (res.statusCode !== 200) {
                console.log(`   ⚠️  Failed to fetch models from ${provider}: ${res.statusCode}`);
                resolve(null);
                return;
              }
              
              try {
                const response = JSON.parse(data);
                const models = response.models
                  .filter(/** @param {GoogleModel} model */ model => {
                    // Filter for generative models
                    return model.supportedGenerationMethods && 
                           model.supportedGenerationMethods.includes('generateContent');
                  })
                  .map(/** @param {GoogleModel} model */ model => {
                    const modelId = model.name.replace('models/', '');
                    return {
                      id: modelId,
                      contextWindow: MODEL_DEFINITIONS[modelId]?.contextWindow || 
                                     (model.inputTokenLimit || 4096),
                      pricing: MODEL_DEFINITIONS[modelId]?.pricing || null
                    };
                  });
                
                console.log(`   ✅ Found ${models.length} available models from ${provider}`);
                resolve(models);
              } catch (e) {
                console.log(`   ⚠️  Error parsing response from ${provider}: ${e.message}`);
                resolve(null);
              }
            });
          });
          
          req.on('error', (e) => {
            console.log(`   ⚠️  Error fetching models from ${provider}: ${e.message}`);
            resolve(null);
          });
          
          req.end();
        });
      }

      case 'ollama': {
        // Ollama doesn't need an API key
        // First fetch all available models
        const baseUrl = providerConfig?.baseUrl || LLM_PROVIDERS.ollama.baseUrl;
        const tagsUrl = new URL('/api/tags', baseUrl);
        
        return new Promise((resolve) => {
          const tagsReq = http.request({
            hostname: tagsUrl.hostname,
            port: tagsUrl.port || 80,
            path: tagsUrl.pathname,
            method: 'GET',
            headers: {
              'Content-Type': 'application/json'
            }
          }, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', async () => {
              if (res.statusCode !== 200) {
                console.log(`   ⚠️  Failed to fetch models from ${provider}: HTTP ${res.statusCode}`);
                resolve(null);
                return;
              }
              
              try {
                const response = JSON.parse(data);
                
                // Fetch all model details in parallel
                const modelPromises = (response.models || []).map(model => {
                  return new Promise((resolveModel) => {
                    const showUrl = new URL('/api/show', baseUrl);
                    const showData = JSON.stringify({ name: model.name });
                    
                    const showReq = http.request({
                      hostname: showUrl.hostname,
                      port: showUrl.port || 80,
                      path: showUrl.pathname,
                      method: 'POST',
                      headers: {
                        'Content-Type': 'application/json',
                        'Content-Length': Buffer.byteLength(showData)
                      }
                    }, (showRes) => {
                      let showResData = '';
                      showRes.on('data', chunk => showResData += chunk);
                      showRes.on('end', () => {
                        if (showRes.statusCode === 200) {
                          try {
                            const info = JSON.parse(showResData);
                            
                            if (info && info.model_info) {
                              // Extract context length from the model info
                              // Ollama uses "num_ctx" for context window size
                              // The keys are flat strings like "llama.num_ctx" or just "num_ctx"
                              let contextWindow = 4096; // default
                              
                              // Look for num_ctx or any key ending with "num_ctx"
                              const contextKey = Object.keys(info.model_info).find(key => 
                                key.endsWith('.num_ctx') || key === 'num_ctx' || 
                                key.endsWith('.context_length') || key === 'context_length'
                              );
                              
                              if (contextKey && info.model_info[contextKey]) {
                                contextWindow = info.model_info[contextKey];
                              }
                              
                              // Check for tool support in capabilities
                              const supportsTools = info.capabilities && 
                                                  Array.isArray(info.capabilities) && 
                                                  info.capabilities.includes('tools');
                              
                              resolveModel({
                                id: model.name,
                                contextWindow,
                                supportsTools,
                                pricing: null // Ollama models are free (local)
                              });
                            } else {
                              // If we can't get model info, add with default context window
                              resolveModel({
                                id: model.name,
                                contextWindow: 4096,
                                supportsTools: false, // default to false if we can't check
                                pricing: null
                              });
                            }
                          } catch (e) {
                            console.log(`   ⚠️  Error parsing model info for ${model.name}: ${e.message}`);
                            resolveModel({
                              id: model.name,
                              contextWindow: 4096,
                              supportsTools: false,
                              pricing: null
                            });
                          }
                        } else {
                          resolveModel({
                            id: model.name,
                            contextWindow: 4096,
                            supportsTools: false,
                            pricing: null
                          });
                        }
                      });
                    });
                    
                    showReq.on('error', (e) => {
                      console.log(`   ⚠️  Error fetching model info for ${model.name}: ${e.message}`);
                      resolveModel({
                        id: model.name,
                        contextWindow: 4096,
                        supportsTools: false,
                        pricing: null
                      });
                    });
                    
                    showReq.write(showData);
                    showReq.end();
                  });
                });
                
                // Wait for all model info fetches to complete
                const models = await Promise.all(modelPromises);
                
                console.log(`   ✅ Found ${models.length} available models from ${provider}`);
                resolve(models);
              } catch (e) {
                console.log(`   ⚠️  Error parsing response from ${provider}: ${e.message}`);
                resolve(null);
              }
            });
          });
          
          tagsReq.on('error', (e) => {
            console.log(`   ⚠️  Error fetching models from ${provider}: ${e.message}`);
            resolve(null);
          });
          
          tagsReq.end();
        });
      }
      
      default:
        console.log(`   ℹ️  Unknown provider ${provider}, skipping model fetch`);
        return null;
    }
  } catch (error) {
    console.log(`   ⚠️  Error fetching models from ${provider}: ${error.message}`);
    return null;
  }
}

// MIME types for static file serving
const MIME_TYPES = {
  '.html': 'text/html',
  '.js': 'application/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.gif': 'image/gif',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon'
};

// Serve static files
function serveStaticFile(req, res) {
  // Parse URL to get pathname only (ignore query strings)
  const parsedUrl = url.parse(req.url);
  let requestPath = parsedUrl.pathname || '/';
  
  // Default to index.html for root
  if (requestPath === '/') {
    requestPath = '/index.html';
  }
  
  // Remove leading slash for path.join to work correctly
  if (requestPath.startsWith('/')) {
    requestPath = requestPath.substring(1);
  }
  
  // Normalize the path and remove any directory traversal attempts
  requestPath = path.normalize(requestPath).replace(/^(\.\.[\\/\\])+/, '');
  
  // Define the web root directory
  const webRoot = path.join(__dirname, 'web');
  
  // Resolve the full file path
  const filePath = path.resolve(webRoot, requestPath);
  
  // Debug logging for troubleshooting
  if (req.url !== '/favicon.ico') {  // Skip favicon requests in logs
    console.log(`[${new Date().toISOString()}] 📁 Static file request:`, {
      url: req.url,
      requestPath,
      __dirname,
      webRoot,
      filePath,
      resolvedWebRoot: path.resolve(webRoot),
      startsWith: filePath.startsWith(path.resolve(webRoot))
    });
  }
  
  // Security: ensure the resolved path is within the web directory
  // This prevents directory traversal attacks like ../../etc/passwd
  if (!filePath.startsWith(path.resolve(webRoot))) {
    console.error(`[${new Date().toISOString()}] ❌ Security check failed: ${filePath} not in ${path.resolve(webRoot)}`);
    res.writeHead(403, { 'Content-Type': 'text/plain' });
    res.end('Forbidden');
    return;
  }
  
  // Additional security: ensure the file exists and is a file (not a directory)
  fs.stat(filePath, (err, stats) => {
    if (err || !stats.isFile()) {
      res.writeHead(404, { 'Content-Type': 'text/plain' });
      res.end('Not Found');
      return;
    }
    
    // Read and serve the file
    fs.readFile(filePath, (readErr, content) => {
      if (readErr) {
        res.writeHead(500, { 'Content-Type': 'text/plain' });
        res.end('Internal Server Error');
        return;
      }
      
      const ext = path.extname(filePath);
      const mimeType = MIME_TYPES[ext] || 'application/octet-stream';
      
      res.writeHead(200, {
        'Content-Type': mimeType,
        'Access-Control-Allow-Origin': ALLOWED_ORIGINS,
        'Cache-Control': 'no-cache'
      });
      res.end(content);
    });
  });
}

// Check for command line arguments
if (process.argv.includes('--help') || process.argv.includes('-h')) {
  console.log('\n📚 Usage:');
  console.log('   node llm-proxy.js [options]');
  console.log('\n🎯 Options:');
  console.log('   --help, -h                   Show this help message');
  console.log('   --show-models                Display all models from all providers with their pricing information');
  console.log('   --update-config              Update the configuration file with latest model definitions');
  console.log('                                while preserving your API keys and custom settings');
  console.log('   --sync                       When used with --update-config, sync configuration with');
  console.log('                                MODEL_DEFINITIONS from code (optionally filtered by API availability)');
  console.log('   --sync-prices                When used with --update-config, update prices and settings');
  console.log('                                for models that exist in both config and code');
  console.log('   --check-availability         When used with --sync and --update-config, filter models by');
  console.log('                                actual availability from provider APIs (requires valid API keys)');
  console.log('\n📖 Description:');
  console.log('   This server provides:');
  console.log('   • A proxy for LLM API calls (OpenAI, Anthropic, Google)');
  console.log('   • A web interface for the MCP (Model Context Protocol) client');
  console.log('   • Model configuration and context window information');
  console.log('   • Cost accounting and usage tracking');
  console.log('   • For Ollama: use --update-config --sync --check-availability to discover models');
  console.log('\n🌐 Endpoints:');
  console.log('   • Web UI:         http://localhost:' + (config.port || 8081) + '/');
  console.log('   • Models API:     http://localhost:' + (config.port || 8081) + '/models');
  console.log('   • MCP Servers:    http://localhost:' + (config.port || 8081) + '/mcp-servers');
  console.log('   • Proxy API:      http://localhost:' + (config.port || 8081) + '/proxy/<provider>/<path>');
  console.log('\n📊 Accounting:');
  console.log('   • Log files:   ' + ACCOUNTING_DIR);
  console.log('   • Format:      JSON Lines (one JSON object per line)');
  console.log('   • Rotation:    Daily (new file each day)');
  console.log('\n');
  process.exit(0);
}

if (process.argv.includes('--show-models') || process.argv.includes('--list-models')) {
  console.log('\n📊 All Provider Models and Pricing Information');
  console.log('='.repeat(120));
  
  Object.entries(config.providers).forEach(([provider, providerConfig]) => {
    console.log(`\n🏢 ${provider.toUpperCase()}`);
    if (!providerConfig.apiKey) {
      console.log('   ⚠️  No API key configured');
    }
    console.log('-'.repeat(120));
    
    if (!providerConfig.models || providerConfig.models.length === 0) {
      console.log('   No models configured');
      return;
    }
    
    // Create a map of all models from MODEL_DEFINITIONS for this provider
    const codeModels = new Set();
    Object.keys(MODEL_DEFINITIONS).forEach(modelId => {
      if ((provider === 'openai' && (modelId.startsWith('gpt') || modelId.startsWith('davinci') || modelId === 'gpt-image-1')) ||
          (provider === 'anthropic' && modelId.startsWith('claude')) ||
          (provider === 'google' && modelId.startsWith('gemini'))) {
        codeModels.add(modelId);
      }
    });
    
    // Table header
    console.log('   Model ID'.padEnd(45) + 
                'Status'.padEnd(15) +
                'Context Window'.padEnd(15) +
                'Input $/MTok'.padEnd(13) +
                'Output $/MTok'.padEnd(14) +
                'Cache Read'.padEnd(12) +
                'Cache Write');
    console.log('   ' + '-'.repeat(114));
    
    providerConfig.models.forEach(model => {
      const modelId = typeof model === 'string' ? model : model.id;
      const configDef = typeof model === 'object' ? model : null;
      const codeDef = MODEL_DEFINITIONS[modelId];
      
      // Determine status
      let status = '';
      if (!codeDef) {
        status = 'not in code';
      } else if (configDef) {
        // Compare config with code
        const configCtx = configDef.contextWindow;
        const codeCtx = codeDef.contextWindow;
        const configPricing = configDef.pricing;
        const codePricing = codeDef.pricing;
        
        const ctxSame = configCtx === codeCtx;
        let pricingSame = true;
        
        if (configPricing && codePricing) {
          pricingSame = configPricing.input === codePricing.input &&
                        configPricing.output === codePricing.output &&
                        configPricing.cacheRead === codePricing.cacheRead &&
                        configPricing.cacheWrite === codePricing.cacheWrite;
        } else if (configPricing !== codePricing) {
          pricingSame = false;
        }
        
        if (ctxSame && pricingSame) {
          status = 'same';
        } else {
          status = 'different';
        }
      } else {
        status = 'not in config';
      }
      
      // Remove from codeModels set as we've seen it
      codeModels.delete(modelId);
      
      const contextWindow = configDef?.contextWindow || codeDef?.contextWindow || 'Unknown';
      const pricing = configDef?.pricing || codeDef?.pricing;
        
      let pricingStr = '';
      if (pricing) {
        const input = pricing.input !== undefined ? '$' + pricing.input.toFixed(2) : 'N/A';
        const output = pricing.output !== undefined ? '$' + pricing.output.toFixed(2) : 'N/A';
        const cacheRead = pricing.cacheRead !== undefined ? '$' + pricing.cacheRead.toFixed(2) : '-';
        const cacheWrite = pricing.cacheWrite !== undefined ? '$' + pricing.cacheWrite.toFixed(2) : '-';
        
        pricingStr = input.padEnd(13) + output.padEnd(14) + cacheRead.padEnd(12) + cacheWrite;
      } else {
        pricingStr = 'No pricing data available';
      }
      
      console.log('   ' + modelId.padEnd(45) + 
                  status.padEnd(15) +
                  contextWindow.toString().padEnd(15) +
                  pricingStr);
    });
    
    // Show models that are in code but not in config
    codeModels.forEach(modelId => {
      const codeDef = MODEL_DEFINITIONS[modelId];
      const status = 'not in config';
      
      let pricingStr = '';
      if (codeDef.pricing) {
        const input = codeDef.pricing.input !== undefined ? '$' + codeDef.pricing.input.toFixed(2) : 'N/A';
        const output = codeDef.pricing.output !== undefined ? '$' + codeDef.pricing.output.toFixed(2) : 'N/A';
        const cacheRead = codeDef.pricing.cacheRead !== undefined ? '$' + codeDef.pricing.cacheRead.toFixed(2) : '-';
        const cacheWrite = codeDef.pricing.cacheWrite !== undefined ? '$' + codeDef.pricing.cacheWrite.toFixed(2) : '-';
        
        pricingStr = input.padEnd(13) + output.padEnd(14) + cacheRead.padEnd(12) + cacheWrite;
      } else {
        pricingStr = 'No pricing data available';
      }
      
      console.log('   ' + modelId.padEnd(45) + 
                  status.padEnd(15) +
                  codeDef.contextWindow.toString().padEnd(15) +
                  pricingStr);
    });
  });
  
  console.log('\n' + '='.repeat(120));
  console.log('\n💡 Notes:');
  console.log('   • Prices are per million tokens (MTok)');
  console.log('   • Cache Read: Discounted rate for cached content (OpenAI/Anthropic)');
  console.log('   • Cache Write: Additional cost for creating cache (Anthropic only, 25% surcharge)');
  console.log('   • Status column meanings:');
  console.log('     - "same": Configuration matches code defaults');
  console.log('     - "different": Configuration differs from code defaults (context window or pricing)');
  console.log('     - "not in code": Model exists in config but not in code definitions');
  console.log('     - "not in config": Model exists in code but not in configuration\n');
  
  process.exit(0);
}

if (process.argv.includes('--update-config')) {
  console.log('\n🔄 Configuration Update Mode');
  console.log('   This will update your configuration file with the latest model definitions');
  console.log('   while preserving your API keys and custom settings.\n');
  
  const syncModels = process.argv.includes('--sync');
  const syncPrices = process.argv.includes('--sync-prices');
  if (syncModels) {
    console.log('   🔄 Will sync configuration with MODEL_DEFINITIONS from code\n');
  }
  if (syncPrices) {
    console.log('   💰 Will update prices for models that exist in both config and code\n');
  }
  
  (async () => {
    try {
      // Read existing config to preserve API keys
      let existingConfig = {};
      if (fs.existsSync(CONFIG_FILE)) {
        const configContent = fs.readFileSync(CONFIG_FILE, 'utf8');
        existingConfig = JSON.parse(configContent);
      }
      
      // Create new config with updated model lists and context windows
      const updatedConfig = {
        ...DEFAULT_CONFIG,
        providers: {}
      };
      
      // Prepare promises for parallel execution
      const providerProcessingPromises = Object.entries(DEFAULT_CONFIG.providers).map(async ([provider, defaultProviderConfig]) => {
        const existingProvider = existingConfig.providers?.[provider];
        
        // Get ALL models from MODEL_DEFINITIONS for this provider
        const allProviderModels = generateModelsForProvider(provider);
        
        // Create a map of all models from MODEL_DEFINITIONS
        const modelDefsMap = new Map();
        allProviderModels.forEach(model => {
          modelDefsMap.set(model.id, model);
        });
        
        let mergedModels;
        
        if (syncPrices) {
          // When syncing prices, only update models that exist in both config and code
          mergedModels = [];
          
          if (existingProvider?.models) {
            existingProvider.models.forEach(existingModel => {
              const id = typeof existingModel === 'string' ? existingModel : existingModel.id;
              
              if (modelDefsMap.has(id)) {
                // Model exists in both - update with code pricing and settings
                const codeModel = modelDefsMap.get(id);
                mergedModels.push(codeModel);
                console.log(`   💰 Updated ${id} with latest pricing from code`);
              } else {
                // Model only in config - preserve as-is
                mergedModels.push(existingModel);
                console.log(`   ✨ Preserved ${id} (not in code)`);
              }
            });
            
            // Report models in code but not in config
            allProviderModels.forEach(codeModel => {
              if (!existingProvider.models.find(m => (typeof m === 'string' ? m : m.id) === codeModel.id)) {
                console.log(`   ℹ️  ${codeModel.id} exists in code but not in config (skipping)`);
              }
            });
          } else {
            // No existing models - use defaults from code
            mergedModels = [...allProviderModels];
            console.log(`   ✅ No existing ${provider} models - using defaults from code`);
          }
        } else if (syncModels) {
          // When syncing, use exactly what's in MODEL_DEFINITIONS
          mergedModels = [...allProviderModels];
          console.log(`   ✅ Syncing ${provider} with ${mergedModels.length} models from MODEL_DEFINITIONS`);
          
          // IMPORTANT: Preserve user-modified pricing
          if (existingProvider?.models) {
            const userPricingMap = new Map();
            existingProvider.models.forEach(existingModel => {
              if (typeof existingModel === 'object' && existingModel.pricing) {
                userPricingMap.set(existingModel.id, existingModel.pricing);
              }
            });
            
            // Apply user pricing to merged models
            mergedModels = mergedModels.map(model => {
              if (userPricingMap.has(model.id)) {
                console.log(`   💰 Preserving user-modified pricing for ${model.id}`);
                return { ...model, pricing: userPricingMap.get(model.id) };
              }
              return model;
            });
            
            // ALSO preserve custom models not in MODEL_DEFINITIONS
            existingProvider.models.forEach(existingModel => {
              const id = typeof existingModel === 'string' ? existingModel : existingModel.id;
              if (!modelDefsMap.has(id)) {
                // This is a custom model not in our definitions, preserve it
                mergedModels.push(existingModel);
                console.log(`   🌟 Preserving custom API model not in code: ${id}`);
              }
            });
          }
        } else {
          // Normal update: preserve custom models
          mergedModels = [...allProviderModels];
          
          // Add any custom models from existing config that aren't in MODEL_DEFINITIONS
          if (existingProvider?.models) {
            existingProvider.models.forEach(existingModel => {
              const id = typeof existingModel === 'string' ? existingModel : existingModel.id;
              if (!modelDefsMap.has(id)) {
                // This is a custom model not in our definitions, preserve it
                mergedModels.push(existingModel);
                console.log(`   ⚠️  Preserving custom model not in code: ${id}`);
              }
            });
          }
        }
        
        // If sync is requested and we have an API key (or it's Ollama), optionally filter by availability
        const isOllama = provider === 'ollama';
        if ((syncModels || syncPrices) && (existingProvider?.apiKey || isOllama) && process.argv.includes('--check-availability')) {
          const availableModels = await fetchAvailableModels(provider, existingProvider?.apiKey || '', existingProvider);
          
          if (availableModels) {
            // For Ollama, completely replace the models list with discovered ones
            if (isOllama) {
              mergedModels = availableModels;
              console.log(`   ✅ Discovered ${mergedModels.length} Ollama models from API`);
            } else {
            // Create a set of available model IDs for faster lookup
            const availableModelIds = new Set(availableModels.map(m => m.id));
            
            // Filter merged models to only include available ones
            const beforeCount = mergedModels.length;
            const filteredModels = [];
            
            const excludedModels = [];
            
            mergedModels.forEach(model => {
              const id = typeof model === 'string' ? model : model.id;
              if (availableModelIds.has(id)) {
                // Model is available, keep it with data from MODEL_DEFINITIONS
                if (modelDefsMap.has(id)) {
                  filteredModels.push(modelDefsMap.get(id));
                } else {
                  filteredModels.push(model);
                }
              } else {
                // Model not available
                excludedModels.push(id);
              }
            });
            
            // Log excluded models
            if (excludedModels.length > 0) {
              console.log(`   🚫 Excluded ${provider} models not available to your API key:`);
              excludedModels.forEach(modelId => {
                console.log(`      - ${modelId}`);
              });
            }
            
            mergedModels = filteredModels;
            
            const removedCount = beforeCount - mergedModels.length;
            if (removedCount > 0) {
              console.log(`   🗑️  Removed ${removedCount} unavailable models from ${provider}`);
            }
            
            // Also add any new models from the API that aren't in our MODEL_DEFINITIONS
            availableModels.forEach(apiModel => {
              if (!mergedModels.find(m => (typeof m === 'string' ? m : m.id) === apiModel.id)) {
                // Check if we have this model in MODEL_DEFINITIONS
                if (modelDefsMap.has(apiModel.id)) {
                  mergedModels.push(modelDefsMap.get(apiModel.id));
                  console.log(`   ✅ Added model from API with code defaults: ${apiModel.id}`);
                } else {
                  mergedModels.push(apiModel);
                  console.log(`   ➕ Added new model from API (not in code): ${apiModel.id}`);
                }
              }
            });
            }
          }
        }
        
        return {
          provider,
          config: {
            ...defaultProviderConfig,
            // Preserve existing API key if available
            apiKey: existingProvider?.apiKey || '',
            // Preserve existing baseUrl if available
            baseUrl: existingProvider?.baseUrl || defaultProviderConfig.baseUrl,
            // Preserve existing type if available
            type: existingProvider?.type || defaultProviderConfig.type || provider,
            // Use merged models list
            models: mergedModels
          }
        };
      });
      
      // Wait for all promises to complete
      const results = await Promise.all(providerProcessingPromises);
      results.forEach(result => {
        updatedConfig.providers[result.provider] = result.config;
      });
      
      // Also preserve any custom providers not in defaults
      if (existingConfig.providers) {
        Object.entries(existingConfig.providers).forEach(([provider, providerConfig]) => {
          if (!updatedConfig.providers[provider]) {
            updatedConfig.providers[provider] = providerConfig;
          }
        });
      }
      
      // Preserve any custom settings
      if (existingConfig.port) updatedConfig.port = existingConfig.port;
      if (existingConfig.allowedOrigins) updatedConfig.allowedOrigins = existingConfig.allowedOrigins;
      
      // Preserve MCP servers
      if (existingConfig.mcpServers) {
        updatedConfig.mcpServers = existingConfig.mcpServers;
      }
      
      // Write updated config
      fs.writeFileSync(CONFIG_FILE, JSON.stringify(updatedConfig, null, 2));
      
      console.log('   ✅ Configuration updated successfully!');
      console.log('\n📊 Update Summary:');
      console.log('   • API keys: Preserved');
      console.log('   • MCP servers: Preserved');
      console.log('   • Custom models: Preserved');
      console.log('   • Latest model definitions: Added');
      console.log('   • Context window sizes: Updated');
      console.log('   • Pricing information: Added/Updated');
      
      if (syncModels) {
        console.log('   • Synced with MODEL_DEFINITIONS from code');
      }
      
      // Count total models
      let totalModels = 0;
      Object.values(updatedConfig.providers).forEach(provider => {
        if (provider.models) totalModels += provider.models.length;
      });
      console.log(`   • Total models available: ${totalModels}`);
      
      console.log('\n✨ Your configuration is now up to date!');
      console.log('   You can start the server normally to use the new models.\n');
      
      process.exit(0);
    } catch (error) {
      console.error('\n❌ Error updating configuration:', error.message);
      console.error('   Please check that your configuration file is valid JSON');
      process.exit(1);
    }
  })();
}

// Create proxy server
const server = http.createServer(async (req, res) => {
  // Handle CORS preflight
  if (req.method === 'OPTIONS') {
    res.writeHead(200, {
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS,
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization, x-api-key, x-goog-api-key, anthropic-version, anthropic-beta',
      'Access-Control-Max-Age': '86400'
    });
    res.end();
    return;
  }

  // Parse request URL
  const parsedUrl = url.parse(req.url, true);
  const pathParts = parsedUrl.pathname.split('/').filter(p => p);

  // Serve static files for the web client
  if (!parsedUrl.pathname.startsWith('/proxy/') && 
      !parsedUrl.pathname.startsWith('/models') && 
      !parsedUrl.pathname.startsWith('/mcp-servers')) {
    // Log static file requests only for main pages
    if (req.url === '/' || req.url.endsWith('.html')) {
      console.log(`[${new Date().toISOString()}] 📄 Web UI request: ${req.url}`);
    }
    serveStaticFile(req, res);
    return;
  }

  // Handle /models endpoint
  if (pathParts.length === 1 && pathParts[0] === 'models') {
    console.log(`[${new Date().toISOString()}] 🔍 Models API request`);
    const availableProviders = {};
    
    // Process each provider
    await Promise.all(Object.entries(config.providers).map(async ([provider, providerConfig]) => {
      const providerType = providerConfig.type || provider;
      const requiresApiKey = providerType !== 'ollama';
      
      if (!requiresApiKey || (providerConfig.apiKey && providerConfig.apiKey.length > 0)) {
        availableProviders[provider] = {
          type: providerConfig.type || provider, // Include the provider type
          models: (providerConfig.models || []).map(model => {
              // Skip string format models - not supported
              if (typeof model === 'string') return null;
              
              const modelId = model.id;
              if (!modelId) return null;
              
              // Validate model configuration
              const modelProviderType = providerConfig.type || provider;
              const validationError = validateModelConfig(provider, model, modelProviderType);
              if (validationError) {
                // Silently skip invalid models (these are typically audio/video models)
                return null;
              }
              
              return {
                id: modelId,
                contextWindow: model.contextWindow,
                pricing: model.pricing,
                endpoint: model.endpoint || 'completions',
                supportsTools: model.supportsTools !== false
              };
            }).filter(Boolean)
          };
      }
    }));

    res.writeHead(200, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ providers: availableProviders }));
    return;
  }
  
  // Handle /mcp-servers endpoint
  if (pathParts.length === 1 && pathParts[0] === 'mcp-servers') {
    console.log(`[${new Date().toISOString()}] 🔍 MCP Servers API request`);
    
    // Get configured MCP servers or provide default
    let mcpServers = config.mcpServers || [];
    
    // If no servers configured, provide a default localhost server
    if (mcpServers.length === 0) {
      mcpServers = [{
        id: 'local_netdata',
        name: 'Local Netdata',
        url: 'ws://localhost:19999/mcp'
      }];
      console.log('   ℹ️  No MCP servers configured, returning default localhost server');
    }
    
    res.writeHead(200, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ servers: mcpServers }));
    return;
  }
  
  // Expected format: /proxy/<provider>/<rest-of-path>
  if (pathParts.length < 2 || pathParts[0] !== 'proxy') {
    res.writeHead(404, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ error: 'Invalid proxy path. Expected /proxy/<provider>/<path> or /models' }));
    return;
  }

  const provider = pathParts[1];
  const apiPath = '/' + pathParts.slice(2).join('/');

  // Check if provider is configured
  const providerConfig = config.providers[provider.toLowerCase()];
  
  if (!providerConfig) {
    res.writeHead(400, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ error: `Provider '${provider}' is not configured` }));
    return;
  }

  // Get the provider type to determine auth requirements
  const providerType = providerConfig.type || provider.toLowerCase();
  
  // Check if API key is required based on provider type
  const requiresApiKey = providerType !== 'ollama';
  
  if (requiresApiKey && !providerConfig.apiKey) {
    res.writeHead(400, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ error: `Provider '${provider}' requires an API key` }));
    return;
  }

  // Get provider URL configuration based on type
  const providerUrlConfig = LLM_PROVIDERS[providerType];
  
  // If no hardcoded config exists for this type, use sensible defaults
  const baseUrl = providerConfig.baseUrl || (providerUrlConfig && providerUrlConfig.baseUrl) || 'https://api.example.com';
  
  // Build target URL
  let targetUrl;
  
  // Special handling for Google - need to adjust the path format
  if (providerType === 'google') {
    // Google expects the full path including model name
    const adjustedPath = apiPath.replace('/generateContent', ':generateContent');
    targetUrl = new URL(baseUrl + adjustedPath);
    // Add API key to URL for Google
    targetUrl.searchParams.append('key', providerConfig.apiKey);
  } else {
    targetUrl = new URL(baseUrl + apiPath);
  }
  
  // Copy query parameters from original request (except Google's key)
  Object.keys(parsedUrl.query).forEach(key => {
    if (!(providerType === 'google' && key === 'key')) {
      const value = parsedUrl.query[key];
      // Handle both string and string[] cases
      if (Array.isArray(value)) {
        // If multiple values, append each one
        value.forEach(v => targetUrl.searchParams.append(key, v));
      } else {
        targetUrl.searchParams.append(key, value);
      }
    }
  });

  // Prepare headers
  const headers = {
    'Content-Type': req.headers['content-type'] || 'application/json',
    'Accept': req.headers.accept || 'application/json',
    'User-Agent': 'MCP-LLM-Proxy/1.0'
  };

  // Add authentication headers based on provider type
  if (providerUrlConfig && providerUrlConfig.authHeader) {
    if (providerUrlConfig.authPrefix) {
      headers[providerUrlConfig.authHeader] = providerUrlConfig.authPrefix + providerConfig.apiKey;
    } else {
      headers[providerUrlConfig.authHeader] = providerConfig.apiKey;
    }
  } else if (providerType === 'openai' || providerType === 'openai-responses') {
    // Default OpenAI-style authentication
    headers.Authorization = 'Bearer ' + providerConfig.apiKey;
  } else if (providerType === 'anthropic') {
    // Default Anthropic-style authentication
    headers['x-api-key'] = providerConfig.apiKey;
  }

  // Forward anthropic-version header if present
  if (req.headers['anthropic-version']) {
    headers['anthropic-version'] = req.headers['anthropic-version'];
  }
  
  // Forward anthropic-beta header if present (for caching and other beta features)
  if (req.headers['anthropic-beta']) {
    headers['anthropic-beta'] = req.headers['anthropic-beta'];
  }

  // Forward other relevant headers
  ['content-length', 'accept-encoding'].forEach(header => {
    if (req.headers[header]) {
      headers[header] = req.headers[header];
    }
  });

  // Collect request body using Buffer for proper handling of large payloads
  const chunks = [];
  req.on('data', chunk => {
    chunks.push(chunk);
  });

  req.on('end', () => {
    const body = Buffer.concat(chunks).toString();
    // Prepare options for the outgoing request
    const options = {
      hostname: targetUrl.hostname,
      port: targetUrl.port || (targetUrl.protocol === 'https:' ? 443 : 80),
      path: targetUrl.pathname + targetUrl.search,
      method: req.method,
      headers
    };

    // Choose http or https module
    const protocol = targetUrl.protocol === 'https:' ? https : http;

    // Extract model from request body for better logging and accounting
    let modelInfo = '';
    let requestModel = '';
    let requestData = {};
    try {
      requestData = JSON.parse(body);
      if (requestData.model) {
        modelInfo = ` (model: ${requestData.model})`;
        requestModel = requestData.model;
      }
    } catch (_e) {
      // Ignore JSON parse errors
    }
    
    // For Google, extract model from URL path
    if (providerType === 'google' && !requestModel) {
      // Path format: /v1beta/models/gemini-1.5-pro/generateContent
      const pathMatch = apiPath.match(/\/models\/([^/]+)\//);
      if (pathMatch && pathMatch[1]) {
        requestModel = pathMatch[1];
        modelInfo = ` (model: ${requestModel})`;
      }
    }
    
    // Get client IP
    const clientIp = req.headers['x-forwarded-for'] || req.socket.remoteAddress || 'unknown';
    
    // Get pricing for the model from configuration (the only source of truth)
    let modelPricing = null;
    let modelConfig = null;
    const modelProviderConfig = config.providers[provider.toLowerCase()];
    if (modelProviderConfig && modelProviderConfig.models && requestModel) {
      modelConfig = modelProviderConfig.models.find(m => 
        (typeof m === 'string' ? m : m.id) === requestModel
      );
      if (modelConfig && typeof modelConfig === 'object') {
        modelPricing = modelConfig.pricing || null;
        
        // Validate the model configuration
        const validationError = validateModelConfig(provider.toLowerCase(), modelConfig, providerType);
        if (validationError) {
          console.error(`[${new Date().toISOString()}] ❌ Invalid model configuration for ${requestModel}: ${validationError}`);
          res.writeHead(400, {
            'Content-Type': 'application/json',
            'Access-Control-Allow-Origin': ALLOWED_ORIGINS
          });
          res.end(JSON.stringify({ 
            error: `Model ${requestModel} has invalid configuration: ${validationError}. Please fix the configuration and restart the server.` 
          }));
          return;
        }
      }
    }
    
    // Check if model exists in configuration
    if (!modelConfig) {
      console.error(`[${new Date().toISOString()}] ❌ Model ${requestModel} not found in configuration for ${provider}`);
      res.writeHead(400, {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': ALLOWED_ORIGINS
      });
      res.end(JSON.stringify({ 
        error: `Model ${requestModel} is not configured for ${provider}. Available models must be defined in the configuration file.` 
      }));
      return;
    }
    
    // For Ollama, if model is not in config, create a minimal config
    if (!modelConfig && providerType === 'ollama') {
      modelConfig = {
        id: requestModel,
        contextWindow: 4096, // Default, actual value would be from auto-discovery
        pricing: null // Ollama is free (local)
      };
    }
    
    console.log(`[${new Date().toISOString()}] 🔄 Proxy ${req.method} to ${provider}: ${targetUrl.pathname}${modelInfo}`);
    
    // Track request start time
    const requestStartTime = Date.now();
    
    // Make the request to the LLM provider
    const proxyReq = protocol.request(options, (proxyRes) => {
      const statusEmoji = proxyRes.statusCode >= 200 && proxyRes.statusCode < 300 ? '✅' : '❌';
      console.log(`[${new Date().toISOString()}] ${statusEmoji} Response: ${proxyRes.statusCode} from ${provider}`);
      
      // Set CORS headers
      const responseHeaders = {
        'Access-Control-Allow-Origin': ALLOWED_ORIGINS,
        'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
        'Access-Control-Allow-Headers': 'Content-Type, Authorization, x-api-key, x-goog-api-key, anthropic-version, anthropic-beta'
      };

      // Forward relevant response headers
      ['content-type', 'content-length', 'content-encoding'].forEach(header => {
        if (proxyRes.headers[header]) {
          responseHeaders[header] = proxyRes.headers[header];
        }
      });

      res.writeHead(proxyRes.statusCode, responseHeaders);

      // Collect response data for accounting
      let responseBody = '';
      let responseBuffer = Buffer.alloc(0);
      const isStreaming = requestData.stream === true;
      const contentEncoding = proxyRes.headers['content-encoding'];
      
      // Debug log
      if (isStreaming) {
        console.log(`[${new Date().toISOString()}] 📡 Streaming response detected`);
      }
      if (contentEncoding) {
        console.log(`[${new Date().toISOString()}] 🗜️  Response encoding: ${contentEncoding}`);
      }
      
      // Handle streaming response
      proxyRes.on('data', (chunk) => {
        res.write(chunk);
        // Capture response for accounting - keep as buffer for compressed responses
        responseBuffer = Buffer.concat([responseBuffer, chunk]);
      });

      proxyRes.on('end', () => {
        res.end();
        
        // Decompress response if needed
        if (contentEncoding && responseBuffer.length > 0) {
          try {
            if (contentEncoding === 'gzip') {
              responseBody = zlib.gunzipSync(responseBuffer).toString('utf8');
            } else if (contentEncoding === 'deflate') {
              responseBody = zlib.inflateSync(responseBuffer).toString('utf8');
            } else if (contentEncoding === 'br') {
              responseBody = zlib.brotliDecompressSync(responseBuffer).toString('utf8');
            } else {
              // Unknown encoding, use raw buffer
              responseBody = responseBuffer.toString('utf8');
            }
          } catch (decompressError) {
            console.error(`❌ Failed to decompress response: ${decompressError.message}`);
            responseBody = responseBuffer.toString('utf8');
          }
        } else {
          responseBody = responseBuffer.toString('utf8');
        }
        
        // Debug logging
        console.log(`[${new Date().toISOString()}] 📊 Response complete - Raw size: ${responseBuffer.length} bytes, Decompressed size: ${Buffer.byteLength(responseBody, 'utf8')} bytes, Streaming: ${isStreaming}`);
        
        // Process accounting for all responses (including errors)
        try {
          let finalResponse = null;
          let errorResponse = null;
          
          // Handle successful responses
          if (proxyRes.statusCode >= 200 && proxyRes.statusCode < 300) {
            if (isStreaming) {
              // For streaming responses, find the last data line with usage info
              const lines = responseBody.split('\n');
              console.log(`   📡 Processing ${lines.length} streaming lines`);
              
              for (let i = lines.length - 1; i >= 0; i--) {
                const line = lines[i].trim();
                if (line.startsWith('data: ')) {
                  const data = line.substring(6);
                  if (data !== '[DONE]') {
                    try {
                      const parsed = JSON.parse(data);
                      if (parsed.usage) {
                        finalResponse = parsed;
                        console.log(`   ✅ Found usage in line ${i}: ${JSON.stringify(parsed.usage)}`);
                        break;
                      }
                    } catch (_e) {
                      // Continue searching
                    }
                  }
                }
              }
              
              if (!finalResponse) {
                console.log(`   ⚠️  No usage data found in streaming response`);
              }
            } else if (responseBody && responseBody.trim()) {
              // Non-streaming response
              try {
                finalResponse = JSON.parse(responseBody);
                if (finalResponse && finalResponse.usage) {
                  console.log(`   ✅ Found usage in non-streaming response: ${JSON.stringify(finalResponse.usage)}`);
                }
              } catch (e) {
                console.error(`❌ Failed to parse non-streaming response: ${e.message}`);
                console.error(`   Response preview: ${responseBody.substring(0, 200)}...`);
              }
            } else {
              console.log(`   ⚠️  Empty response body`);
            }
          } else if (responseBody && responseBody.trim()) {
            // Handle error responses
            try {
              errorResponse = JSON.parse(responseBody);
            } catch (_e) {
              errorResponse = { error: responseBody || 'Unknown error' };
            }
          }
          
          // Extract token usage (may be present even in error responses)
          const tokens = finalResponse ? extractTokenUsage(provider, finalResponse) : {
            promptTokens: 0,
            completionTokens: 0,
            cachedTokens: 0,
            cacheCreationTokens: 0
          };
          
          // Use pricing that was already looked up
          const pricing = modelPricing;
          
          if (!pricing && requestModel) {
            console.log(`   ⚠️  No pricing found for model: ${requestModel}`);
          }
          
          // Calculate costs
          const costs = calculateCosts(tokens, pricing);
          
          // Create accounting entry
          const accountingEntry = {
            timestamp: new Date().toISOString(),
            clientIp,
            provider,
            model: requestModel,
            endpoint: apiPath,
            statusCode: proxyRes.statusCode,
            duration: Date.now() - requestStartTime,
            requestBytes: Buffer.byteLength(body || '', 'utf8'),
            responseBytes: responseBuffer.length,  // Raw response size (compressed)
            decompressedBytes: Buffer.byteLength(responseBody || '', 'utf8'),  // Decompressed size
            tokens: {
              prompt: tokens.promptTokens,
              completion: tokens.completionTokens,
              cachedRead: tokens.cachedTokens,
              cacheCreation: tokens.cacheCreationTokens,
              reasoning: tokens.reasoningTokens || 0
            },
            unitPricing: pricing ? {
              input: pricing.input || 0,
              output: pricing.output || 0,
              cacheRead: pricing.cacheRead || pricing.input || 0,
              cacheWrite: pricing.cacheWrite || pricing.input || 0
            } : null,
            costs: {
              input: costs.inputCost,
              output: costs.outputCost,
              cacheRead: costs.cacheReadCost,
              cacheWrite: costs.cacheWriteCost
            },
            totalCost: costs.totalCost
          };
          
          // Add error information if present
          if (errorResponse) {
            accountingEntry.error = errorResponse;
          }
          
          // Write to accounting log
          writeAccountingEntry(accountingEntry);
          
          // Log to console
          if (costs.totalCost > 0) {
            console.log(`[${new Date().toISOString()}] 💰 Cost: $${costs.totalCost.toFixed(6)} for ${requestModel}`);
          }
          if (errorResponse) {
            console.log(`[${new Date().toISOString()}] ⚠️  Error logged to accounting for ${requestModel}`);
          }
        } catch (error) {
          console.error('❌ Accounting error:', error.message);
          // Debug: log response body length and first 100 chars
          console.error(`   Response body length: ${responseBody.length}`);
          if (responseBody) {
            console.error(`   First 100 chars: ${responseBody.substring(0, 100)}...`);
          }
        }
      });
    });

    proxyReq.on('error', (error) => {
      console.error(`[${new Date().toISOString()}] ❌ Proxy error for ${provider}: ${error.message}`);
      
      // Log failed request attempts to accounting
      const accountingEntry = {
        timestamp: new Date().toISOString(),
        clientIp,
        provider,
        model: requestModel,
        endpoint: apiPath,
        statusCode: 0, // 0 indicates network/connection failure
        duration: Date.now() - requestStartTime,
        requestBytes: Buffer.byteLength(body || '', 'utf8'),
        responseBytes: 0,
        tokens: {
          prompt: 0,
          completion: 0,
          cachedRead: 0,
          cacheCreation: 0,
          reasoning: 0
        },
        unitPricing: modelPricing,
        costs: {
          input: 0,
          output: 0,
          cacheRead: 0,
          cacheWrite: 0
        },
        totalCost: 0,
        error: {
          type: 'proxy_error',
          message: error.message,
          code: error.code || 'UNKNOWN'
        }
      };
      
      writeAccountingEntry(accountingEntry);
      console.log(`[${new Date().toISOString()}] ⚠️  Network error logged to accounting for ${requestModel || 'unknown model'}`);
      
      
      res.writeHead(502, {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': ALLOWED_ORIGINS
      });
      res.end(JSON.stringify({ error: 'Failed to connect to LLM provider: ' + error.message }));
    });

    // Write request body if present
    if (body) {
      proxyReq.write(body);
    }
    
    proxyReq.end();
  });
});

// Start the server only if not running config management commands
const isConfigCommand = process.argv.includes('--update-config') || 
                       process.argv.includes('--show-models') || 
                       process.argv.includes('--list-models') ||
                       process.argv.includes('--help') || 
                       process.argv.includes('-h');

if (!isConfigCommand) {
  server.listen(PROXY_PORT, () => {
  console.log('\n🚀 Server Started Successfully!');
  console.log('='.repeat(60));
  console.log('\n🌐 Available Services:');
  console.log(`   • Web UI:          http://localhost:${PROXY_PORT}/`);
  console.log(`   • Models API:      http://localhost:${PROXY_PORT}/models`);
  console.log(`   • MCP Servers:     http://localhost:${PROXY_PORT}/mcp-servers`);
  console.log(`   • Proxy Endpoint:  http://localhost:${PROXY_PORT}/proxy/<provider>/<path>`);
  
  console.log('\n📊 Accounting:');
  console.log(`   • Log directory:   ${ACCOUNTING_DIR}`);
  console.log(`   • Today's log:     ${path.basename(ACCOUNTING_FILE)}`);
  console.log('   • Format:          JSON Lines (JSONL)');
  
  console.log('\n🔌 MCP Connection:');
  console.log('   The web client will automatically try to connect to:');
  console.log('   • MCP Server: ws://localhost:19999/mcp');
  console.log('   • If the MCP server is not running, you can start it separately');
  
  console.log('\n📝 Quick Start:');
  console.log(`   1. Open your browser to: http://localhost:${PROXY_PORT}/`);
  console.log('   2. Create a new chat');
  console.log('   3. Start asking questions about your infrastructure!');
  
  console.log('\n💡 Tips:');
  console.log('   • Press Ctrl+C to stop the server');
  console.log('   • Logs are displayed here in real-time');
  console.log('   • Check the Communication Log in the web UI for detailed debugging');
  console.log('   • Cost tracking is automatic for all LLM requests');
  
  console.log('\n' + '='.repeat(60));
  console.log('Server is ready and waiting for connections...\n');
  });
}