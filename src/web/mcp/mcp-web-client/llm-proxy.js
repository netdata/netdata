#!/usr/bin/env node

const http = require('http');
const https = require('https');
const url = require('url');
const fs = require('fs');
const path = require('path');
const os = require('os');

// Display startup banner
console.log('='.repeat(60));
console.log('LLM Proxy Server & MCP Web Client');
console.log('='.repeat(60));

// Configuration file path in user's home directory
const CONFIG_DIR = path.join(os.homedir(), '.config');
const CONFIG_FILE = path.join(CONFIG_DIR, 'llm-proxy-config.json');

// Default configuration template
// Model context window sizes
const MODEL_CONTEXT_WINDOWS = {
  // OpenAI
  'gpt-4o': 128000,
  'gpt-4o-mini': 128000,
  'gpt-4-turbo': 128000,
  'gpt-4-turbo-preview': 128000,
  'gpt-4': 8192,
  'gpt-3.5-turbo': 16384,
  'gpt-3.5-turbo-16k': 16384,
  
  // Anthropic
  'claude-opus-4-20250514': 200000,
  'claude-sonnet-4-20250514': 200000,
  'claude-3-7-sonnet-20250219': 200000,
  'claude-3-5-haiku-20241022': 200000,
  'claude-3-5-sonnet-20241022': 200000,
  'claude-3-5-sonnet-20240620': 200000,
  'claude-3-opus-20240229': 200000,
  'claude-3-sonnet-20240229': 200000,
  'claude-3-haiku-20240307': 200000,
  
  // Google
  'gemini-2.0-flash-exp': 1000000,
  'gemini-2.0-flash-thinking-exp': 1000000,
  'gemini-1.5-pro': 2000000,
  'gemini-1.5-flash': 1000000,
  'gemini-pro': 32760,
  'gemini-pro-vision': 32760
};

const DEFAULT_CONFIG = {
  port: 8081,
  allowedOrigins: '*',
  providers: {
    openai: {
      apiKey: '',
      models: [
        { id: 'gpt-4o', contextWindow: 128000 },
        { id: 'gpt-4o-mini', contextWindow: 128000 },
        { id: 'gpt-4-turbo', contextWindow: 128000 },
        { id: 'gpt-4-turbo-preview', contextWindow: 128000 },
        { id: 'gpt-4', contextWindow: 8192 },
        { id: 'gpt-3.5-turbo', contextWindow: 16384 },
        { id: 'gpt-3.5-turbo-16k', contextWindow: 16384 }
      ]
    },
    anthropic: {
      apiKey: '',
      models: [
        { id: 'claude-opus-4-20250514', contextWindow: 200000 },
        { id: 'claude-sonnet-4-20250514', contextWindow: 200000 },
        { id: 'claude-3-7-sonnet-20250219', contextWindow: 200000 },
        { id: 'claude-3-5-haiku-20241022', contextWindow: 200000 },
        { id: 'claude-3-5-sonnet-20241022', contextWindow: 200000 },
        { id: 'claude-3-5-sonnet-20240620', contextWindow: 200000 },
        { id: 'claude-3-opus-20240229', contextWindow: 200000 },
        { id: 'claude-3-sonnet-20240229', contextWindow: 200000 },
        { id: 'claude-3-haiku-20240307', contextWindow: 200000 }
      ]
    },
    google: {
      apiKey: '',
      models: [
        { id: 'gemini-2.0-flash-exp', contextWindow: 1000000 },
        { id: 'gemini-2.0-flash-thinking-exp', contextWindow: 1000000 },
        { id: 'gemini-1.5-pro', contextWindow: 2000000 },
        { id: 'gemini-1.5-flash', contextWindow: 1000000 },
        { id: 'gemini-pro', contextWindow: 32760 },
        { id: 'gemini-pro-vision', contextWindow: 32760 }
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
  console.log('\n📁 Configuration:');
  console.log(`   Config directory: ${CONFIG_DIR}`);
  console.log(`   Config file: ${CONFIG_FILE}`);
  
  // Ensure .config directory exists
  if (!fs.existsSync(CONFIG_DIR)) {
    console.log(`   Creating config directory...`);
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }

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
    console.log('   3. Save the file and restart this server');
    console.log('\n💡 Tip: You can use any text editor to edit the configuration file');
    console.log('   Example: nano ' + CONFIG_FILE);
    console.log('\n');
    process.exit(0);
  }

  try {
    console.log('   Loading configuration...');
    const config = JSON.parse(fs.readFileSync(CONFIG_FILE, 'utf8'));
    
    // Check if any API keys are configured
    const configuredProviders = [];
    let totalModels = 0;
    
    Object.entries(config.providers).forEach(([provider, settings]) => {
      if (settings.apiKey && settings.apiKey.length > 0) {
        const modelCount = settings.models ? settings.models.length : 0;
        configuredProviders.push(`${provider} (${modelCount} models)`);
        totalModels += modelCount;
      }
    });
    
    if (configuredProviders.length === 0) {
      console.error('\n❌ Error: No API keys configured!');
      console.error('\n📝 Please edit the configuration file and add at least one API key:');
      console.error(`   ${CONFIG_FILE}`);
      console.error('\nExample configuration:');
      console.error('```json');
      console.error('{');
      console.error('  "providers": {');
      console.error('    "openai": {');
      console.error('      "apiKey": "sk-YOUR-API-KEY-HERE",');
      console.error('      "models": [...]');
      console.error('    }');
      console.error('  }');
      console.error('}');
      console.error('```\n');
      process.exit(1);
    }
    
    console.log('   ✅ Configuration loaded successfully!');
    console.log(`   Configured providers: ${configuredProviders.join(', ')}`);
    console.log(`   Total models available: ${totalModels}`);

    return config;
  } catch (error) {
    console.error('\n❌ Error reading configuration file:', error.message);
    console.error('   Please check that the file is valid JSON format');
    process.exit(1);
  }
}

// Load configuration
const config = loadConfig();
const PROXY_PORT = config.port || 8081;
const ALLOWED_ORIGINS = config.allowedOrigins || '*';

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
  let filePath = req.url === '/' ? '/index.html' : req.url;
  filePath = path.join(__dirname, filePath);
  
  // Security: prevent directory traversal
  if (!filePath.startsWith(__dirname)) {
    res.writeHead(403, { 'Content-Type': 'text/plain' });
    res.end('Forbidden');
    return;
  }
  
  fs.readFile(filePath, (err, content) => {
    if (err) {
      if (err.code === 'ENOENT') {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        res.end('Not Found');
      } else {
        res.writeHead(500, { 'Content-Type': 'text/plain' });
        res.end('Internal Server Error');
      }
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
}

// Check for command line arguments
if (process.argv.includes('--help') || process.argv.includes('-h')) {
  console.log('\n📚 Usage:');
  console.log('   node llm-proxy.js [options]');
  console.log('\n🎯 Options:');
  console.log('   --help, -h        Show this help message');
  console.log('   --update-config   Update the configuration file with latest model definitions');
  console.log('                     while preserving your API keys and custom settings');
  console.log('\n📖 Description:');
  console.log('   This server provides:');
  console.log('   • A proxy for LLM API calls (OpenAI, Anthropic, Google)');
  console.log('   • A web interface for the MCP (Model Context Protocol) client');
  console.log('   • Automatic model discovery and context window information');
  console.log('\n🌐 Endpoints:');
  console.log('   • Web UI:      http://localhost:' + (config.port || 8081) + '/');
  console.log('   • Models API:  http://localhost:' + (config.port || 8081) + '/models');
  console.log('   • Proxy API:   http://localhost:' + (config.port || 8081) + '/proxy/<provider>/<path>');
  console.log('\n');
  process.exit(0);
}

if (process.argv.includes('--update-config')) {
  console.log('\n🔄 Configuration Update Mode');
  console.log('   This will update your configuration file with the latest model definitions');
  console.log('   while preserving your API keys and custom settings.\n');
  
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
    
    // Merge providers, preserving API keys and custom models
    Object.entries(DEFAULT_CONFIG.providers).forEach(([provider, defaultProviderConfig]) => {
      const existingProvider = existingConfig.providers?.[provider];
      
      // Create a map of default models for easy lookup
      const defaultModelsMap = new Map();
      defaultProviderConfig.models.forEach(model => {
        const id = typeof model === 'string' ? model : model.id;
        defaultModelsMap.set(id, model);
      });
      
      // Create merged models list
      const mergedModels = [...defaultProviderConfig.models];
      
      // Add any custom models from existing config that aren't in defaults
      if (existingProvider?.models) {
        existingProvider.models.forEach(existingModel => {
          const id = typeof existingModel === 'string' ? existingModel : existingModel.id;
          if (!defaultModelsMap.has(id)) {
            // This is a custom model, preserve it
            mergedModels.push(existingModel);
          }
        });
      }
      
      updatedConfig.providers[provider] = {
        ...defaultProviderConfig,
        // Preserve existing API key if available
        apiKey: existingProvider?.apiKey || '',
        // Use merged models list
        models: mergedModels
      };
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
    
    // Write updated config
    fs.writeFileSync(CONFIG_FILE, JSON.stringify(updatedConfig, null, 2));
    
    console.log('   ✅ Configuration updated successfully!');
    console.log('\n📊 Update Summary:');
    console.log('   • API keys: Preserved');
    console.log('   • Custom models: Preserved');
    console.log('   • Latest model definitions: Added');
    console.log('   • Context window sizes: Updated');
    
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
  if (!parsedUrl.pathname.startsWith('/proxy/') && !parsedUrl.pathname.startsWith('/models')) {
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
    
    Object.entries(config.providers).forEach(([provider, providerConfig]) => {
      if (providerConfig.apiKey && providerConfig.apiKey.length > 0) {
        availableProviders[provider] = {
          models: (providerConfig.models || []).map(model => {
            // Handle both string format (backward compatibility) and object format
            if (typeof model === 'string') {
              return {
                id: model,
                contextWindow: MODEL_CONTEXT_WINDOWS[model] || 4096
              };
            } else if (model.id) {
              // Already in object format with contextWindow
              return {
                id: model.id,
                contextWindow: model.contextWindow || MODEL_CONTEXT_WINDOWS[model.id] || 4096
              };
            }
            return null;
          }).filter(Boolean)
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

    // Extract model from request body for better logging
    let modelInfo = '';
    try {
      const bodyData = JSON.parse(body);
      if (bodyData.model) {
        modelInfo = ` (model: ${bodyData.model})`;
      }
    } catch (e) {
      // Ignore JSON parse errors
    }
    
    console.log(`[${new Date().toISOString()}] 🔄 Proxy ${req.method} to ${provider}: ${targetUrl.pathname}${modelInfo}`);
    
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

      // Handle streaming response
      proxyRes.on('data', (chunk) => {
        res.write(chunk);
      });

      proxyRes.on('end', () => {
        res.end();
      });
    });

    proxyReq.on('error', (error) => {
      console.error(`[${new Date().toISOString()}] ❌ Proxy error for ${provider}: ${error.message}`);
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
  console.log('\n🚀 Server Started Successfully!');
  console.log('='.repeat(60));
  console.log('\n🌐 Available Services:');
  console.log(`   • Web UI:          http://localhost:${PROXY_PORT}/`);
  console.log(`   • Models API:      http://localhost:${PROXY_PORT}/models`);
  console.log(`   • Proxy Endpoint:  http://localhost:${PROXY_PORT}/proxy/<provider>/<path>`);
  
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
  
  console.log('\n' + '='.repeat(60));
  console.log('Server is ready and waiting for connections...\n');
});