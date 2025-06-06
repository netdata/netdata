#!/usr/bin/env node

const http = require('http');
const https = require('https');
const url = require('url');
const fs = require('fs');
const path = require('path');
const os = require('os');

// Configuration file path in user's home directory
const CONFIG_DIR = path.join(os.homedir(), '.config');
const CONFIG_FILE = path.join(CONFIG_DIR, 'llm-proxy-config.json');

// Default configuration template
const DEFAULT_CONFIG = {
  port: 8081,
  allowedOrigins: '*',
  providers: {
    openai: {
      apiKey: '',
      models: [
        'gpt-4o',
        'gpt-4o-mini',
        'gpt-4-turbo',
        'gpt-4-turbo-preview',
        'gpt-4',
        'gpt-3.5-turbo',
        'gpt-3.5-turbo-16k'
      ]
    },
    anthropic: {
      apiKey: '',
      models: [
        'claude-opus-4-20250514',
        'claude-sonnet-4-20250514',
        'claude-3-7-sonnet-20250219',
        'claude-3-5-haiku-20241022',
        'claude-3-5-sonnet-20241022',
        'claude-3-5-sonnet-20240620',
        'claude-3-opus-20240229',
        'claude-3-sonnet-20240229',
        'claude-3-haiku-20240307'
      ]
    },
    google: {
      apiKey: '',
      models: [
        'gemini-2.0-flash-exp',
        'gemini-2.0-flash-thinking-exp',
        'gemini-1.5-pro',
        'gemini-1.5-flash',
        'gemini-pro',
        'gemini-pro-vision'
      ]
    }
  }
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
  }
};

// Load or create configuration
function loadConfig() {
  // Ensure .config directory exists
  if (!fs.existsSync(CONFIG_DIR)) {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }

  if (!fs.existsSync(CONFIG_FILE)) {
    console.log(`Configuration file not found. Creating ${CONFIG_FILE}`);
    fs.writeFileSync(CONFIG_FILE, JSON.stringify(DEFAULT_CONFIG, null, 2));
    console.log('\nPlease edit the configuration file and add your API keys.');
    console.log('Then restart the proxy server.');
    process.exit(0);
  }

  try {
    const config = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
    
    // Check if any API keys are configured
    const hasApiKeys = Object.values(config.providers).some(provider => provider.apiKey && provider.apiKey.length > 0);
    
    if (!hasApiKeys) {
      console.error('\nError: No API keys configured!');
      console.error(`Please edit ${CONFIG_FILE} and add at least one API key.`);
      console.error('\nExample:');
      console.error('  "openai": {');
      console.error('    "apiKey": "sk-...",');
      console.error('    "models": ["gpt-4", "gpt-3.5-turbo"]');
      console.error('  }');
      process.exit(1);
    }

    return config;
  } catch (error) {
    console.error(`Error reading configuration file: ${error.message}`);
    process.exit(1);
  }
}

// Load configuration
const config = loadConfig();
const PROXY_PORT = config.port || 8081;
const ALLOWED_ORIGINS = config.allowedOrigins || '*';

// Create proxy server
const server = http.createServer(async (req, res) => {
  // Handle CORS preflight
  if (req.method === 'OPTIONS') {
    res.writeHead(200, {
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS,
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization, x-api-key, x-goog-api-key, anthropic-version',
      'Access-Control-Max-Age': '86400'
    });
    res.end();
    return;
  }

  // Parse request URL
  const parsedUrl = url.parse(req.url, true);
  const pathParts = parsedUrl.pathname.split('/').filter(p => p);

  // Handle /models endpoint
  if (pathParts.length === 1 && pathParts[0] === 'models') {
    const availableProviders = {};
    
    Object.entries(config.providers).forEach(([provider, providerConfig]) => {
      if (providerConfig.apiKey && providerConfig.apiKey.length > 0) {
        availableProviders[provider] = {
          models: providerConfig.models || []
        };
      }
    });

    res.writeHead(200, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ providers: availableProviders }));
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
  if (!providerConfig || !providerConfig.apiKey) {
    res.writeHead(400, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ error: `Provider '${provider}' is not configured or has no API key` }));
    return;
  }

  // Get provider URL configuration
  const providerUrlConfig = LLM_PROVIDERS[provider.toLowerCase()];
  if (!providerUrlConfig) {
    res.writeHead(400, {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': ALLOWED_ORIGINS
    });
    res.end(JSON.stringify({ error: 'Unknown provider: ' + provider }));
    return;
  }

  // Build target URL
  let targetUrl;
  
  // Special handling for Google - need to adjust the path format
  if (provider.toLowerCase() === 'google') {
    // Google expects the full path including model name
    const adjustedPath = apiPath.replace('/generateContent', ':generateContent');
    targetUrl = new URL(providerUrlConfig.baseUrl + adjustedPath);
    // Add API key to URL for Google
    targetUrl.searchParams.append('key', providerConfig.apiKey);
  } else {
    targetUrl = new URL(providerUrlConfig.baseUrl + apiPath);
  }
  
  // Copy query parameters from original request (except Google's key)
  Object.keys(parsedUrl.query).forEach(key => {
    if (!(provider.toLowerCase() === 'google' && key === 'key')) {
      targetUrl.searchParams.append(key, parsedUrl.query[key]);
    }
  });

  // Prepare headers
  const headers = {
    'Content-Type': req.headers['content-type'] || 'application/json',
    'Accept': req.headers['accept'] || 'application/json',
    'User-Agent': 'MCP-LLM-Proxy/1.0'
  };

  // Add authentication headers from config
  if (providerUrlConfig.authHeader) {
    if (providerUrlConfig.authPrefix) {
      headers[providerUrlConfig.authHeader] = providerUrlConfig.authPrefix + providerConfig.apiKey;
    } else {
      headers[providerUrlConfig.authHeader] = providerConfig.apiKey;
    }
  }

  // Forward anthropic-version header if present
  if (req.headers['anthropic-version']) {
    headers['anthropic-version'] = req.headers['anthropic-version'];
  }

  // Forward other relevant headers
  ['content-length', 'accept-encoding'].forEach(header => {
    if (req.headers[header]) {
      headers[header] = req.headers[header];
    }
  });

  // Collect request body
  let body = '';
  req.on('data', chunk => {
    body += chunk.toString();
  });

  req.on('end', () => {
    // Prepare options for the outgoing request
    const options = {
      hostname: targetUrl.hostname,
      port: targetUrl.port || (targetUrl.protocol === 'https:' ? 443 : 80),
      path: targetUrl.pathname + targetUrl.search,
      method: req.method,
      headers: headers
    };

    // Choose http or https module
    const protocol = targetUrl.protocol === 'https:' ? https : http;

    // Log the proxied request for debugging
    console.log(`[${new Date().toISOString()}] Proxying ${req.method} request:`);
    console.log(`  From: ${req.url}`);
    console.log(`  To: ${targetUrl.href} (without API key in logs)`);
    console.log(`  Provider: ${provider}`);
    
    // Make the request to the LLM provider
    const proxyReq = protocol.request(options, (proxyRes) => {
      console.log(`  Response: ${proxyRes.statusCode} ${proxyRes.statusMessage}`);
      
      // Set CORS headers
      const responseHeaders = {
        'Access-Control-Allow-Origin': ALLOWED_ORIGINS,
        'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
        'Access-Control-Allow-Headers': 'Content-Type, Authorization, x-api-key, x-goog-api-key, anthropic-version'
      };

      // Forward relevant response headers
      ['content-type', 'content-length', 'content-encoding'].forEach(header => {
        if (proxyRes.headers[header]) {
          responseHeaders[header] = proxyRes.headers[header];
        }
      });

      res.writeHead(proxyRes.statusCode, responseHeaders);

      // Handle streaming response
      proxyRes.on('data', (chunk) => {
        res.write(chunk);
      });

      proxyRes.on('end', () => {
        res.end();
      });
    });

    proxyReq.on('error', (error) => {
      console.error('Proxy request error:', error);
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

// Start the server
server.listen(PROXY_PORT, () => {
  console.log(`LLM CORS Proxy Server running on http://localhost:${PROXY_PORT}`);
  console.log('\nEndpoints:');
  console.log(`  GET  http://localhost:${PROXY_PORT}/models - List available models`);
  console.log(`  POST http://localhost:${PROXY_PORT}/proxy/<provider>/<api-path> - Proxy LLM requests`);
  console.log('\nConfigured providers:');
  
  Object.entries(config.providers).forEach(([provider, providerConfig]) => {
    if (providerConfig.apiKey && providerConfig.apiKey.length > 0) {
      console.log(`  - ${provider}: ${providerConfig.models.length} models`);
    }
  });
  
  console.log('\nExamples:');
  console.log(`  OpenAI:    POST http://localhost:${PROXY_PORT}/proxy/openai/v1/chat/completions`);
  console.log(`  Anthropic: POST http://localhost:${PROXY_PORT}/proxy/anthropic/v1/messages`);
  console.log(`  Google:    POST http://localhost:${PROXY_PORT}/proxy/google/v1beta/models/gemini-pro/generateContent`);
});