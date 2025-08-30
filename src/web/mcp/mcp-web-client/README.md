# Netdata MCP LLM Client

An all-in-one proxy server and web client for interacting with Netdata's Model Context Protocol (MCP) server using various LLM providers. The proxy server handles API key management, serves the web interface, and provides comprehensive usage accounting.

## Installation

### System-wide Installation (Recommended)

For a production setup, install the LLM proxy server as a system service:

```bash
# Clone or download the repository
cd /path/to/mcp-web-client

# Run the installation script (requires root)
sudo ./install.sh
```

This will:
- Install files to `/opt/llm-proxy`
- Create a systemd service named `llm-proxy`
- Set up logs directory at `/opt/llm-proxy/logs`
- Configure proper security permissions

After installation:
1. Create your configuration:
   ```bash
   sudo nano /opt/llm-proxy/llm-proxy-config.json
   ```
   (The service will create a template on first run)

2. Start the service:
   ```bash
   sudo systemctl start llm-proxy
   sudo systemctl enable llm-proxy  # Enable auto-start on boot
   ```

3. Check status and logs:
   ```bash
   sudo systemctl status llm-proxy
   sudo journalctl -u llm-proxy -f
   ```

4. Access the web interface at: http://localhost:8081

### Development Setup

For development or testing, you can run directly from the source directory:

```bash
cd /path/to/mcp-web-client
node llm-proxy.js
```

Configuration will be created in the current directory as `llm-proxy-config.json`.
Logs will be written to `./logs/`.

## Setup Guide

### Prerequisites

- Node.js (v14 or higher)
- A running Netdata instance with MCP server enabled
- API keys for at least one LLM provider (OpenAI, Anthropic, or Google) OR a local Ollama installation

### 1. Setting up the LLM Proxy Server

The proxy server is the single entry point that:
- Manages API keys securely (never exposed to the browser)
- Serves the web client interface at http://localhost:8081/
- Proxies all LLM API requests
- Tracks usage and costs in accounting logs

#### Configuration

The proxy server needs a configuration file with your API keys. The location depends on how you run it:
- **System service**: `/opt/llm-proxy/llm-proxy-config.json`
- **Development**: `./llm-proxy-config.json` (current directory)

On first run without a config file, the server will create a template and exit.

Edit the configuration file to add your API keys:
   ```json
   {
     "port": 8081,
     "allowedOrigins": "*",
     "providers": {
       "openai": {
         "apiKey": "sk-YOUR-OPENAI-KEY",
         "models": [
           // Models are automatically populated from built-in definitions
         ]
       },
       "anthropic": {
         "apiKey": "sk-ant-YOUR-ANTHROPIC-KEY",
         "models": [
           // Models are automatically populated from built-in definitions
         ]
       },
       "google": {
         "apiKey": "YOUR-GOOGLE-AI-KEY",
         "models": [
           // Models are automatically populated from built-in definitions
         ]
       },
       "ollama": {
         "endpoint": "http://localhost:11434",
         "models": [
           // Models are automatically discovered from your Ollama installation
         ]
       }
     }
   }
   ```

4. Start the service (system installation) or run directly (development):
   ```bash
   # For system service:
   sudo systemctl restart llm-proxy
   
   # For development:
   node llm-proxy.js
   ```

   You should see output like:
   ```
   ============================================================
   LLM Proxy Server & MCP Web Client
   ============================================================
   
   🚀 Server Started Successfully!
   ============================================================
   
   🌐 Available Services:
      • Web UI:          http://localhost:8081/
      • Models API:      http://localhost:8081/models
      • Proxy Endpoint:  http://localhost:8081/proxy/<provider>/<path>
   
   📊 Accounting:
      • Log directory:   /opt/llm-proxy/logs (or ./logs for development)
      • Today's log:     llm-accounting-2024-01-15.jsonl
      • Format:          JSON Lines (JSONL)
   ```

### 2. Accessing the Web Client

1. The web client is served directly by the proxy server. Simply open:
   ```
   http://localhost:8081/
   ```
   Or if accessing remotely:
   ```
   http://YOUR_SERVER_IP:8081/
   ```

2. Click the settings icon (⚙️) in the bottom left

### 3. Configure MCP Server

1. In Settings, go to the "MCP Servers" tab
2. Click "+ Add MCP Server"
3. Enter your Netdata MCP WebSocket URL:
   ```
   ws://localhost:19999/ws/mcp?api_key=YOUR_API_KEY
   ```
4. Give it a name (e.g., "Local Netdata")
5. Click "Add Server"

### 4. Configure LLM Proxy

1. In Settings, go to the "LLM Providers" tab
2. Click "+ Add LLM Provider"
3. Enter the proxy URL (default: `http://localhost:8081`)
4. Give it a name (e.g., "Local LLM Proxy")
5. The client will test the connection and show available providers
6. Click "Add Provider"

### 5. Create a Chat

1. Click "+ New" in the chat sidebar
2. Select your MCP server
3. Select your LLM proxy
4. Choose a model from the dropdown (organized by provider)
5. Click "Create Chat"

## Features

- **Secure API Key Management**: API keys are stored only in the proxy server, never in the browser
- **Multiple LLM Support**: Use OpenAI, Anthropic, Google AI, or Ollama models
- **Ollama Support**: Full support for local Ollama models with automatic discovery
- **Multiple Provider Instances**: Configure multiple instances of the same provider with different settings
- **Model-Specific Endpoints**: Support for different API endpoints per model (e.g., o1 models use /v1/responses)
- **Tool Control Per Model**: Enable/disable MCP tools on a per-model basis
- **Model Selection**: Choose specific models for each chat with automatic pricing info
- **MCP Integration**: Full access to Netdata metrics and functions
- **Chat History**: All conversations are saved locally
- **Temperature Control**: Adjust response creativity per chat
- **Context Window Tracking**: Monitor token usage in real-time
- **Cost Accounting**: Automatic tracking of all LLM usage with detailed cost breakdown
- **Compressed Response Support**: Handles gzip, deflate, and brotli compressed responses
- **Model Discovery**: Automatic model fetching from OpenAI, Google, and Ollama APIs
- **Built-in Model Database**: Comprehensive pricing and context window information
- **Strict Model Validation**: Enforces provider-specific pricing requirements to prevent silent failures

## Proxy Server

### Command Line Options

```bash
node llm-proxy.js [options]

Options:
  --help, -h          Show help message
  --show-models       Display all configured models with pricing and status
  --update-config     Update configuration with latest model definitions
  --sync              Sync configuration with built-in MODEL_DEFINITIONS
```

### Managing Models

#### Understanding Model Configuration

**CRITICAL**: The proxy server enforces strict validation of all model configurations to prevent silent failures and ensure accurate cost tracking.

The proxy server uses model information from two sources:

1. **Configuration file** (`llm-proxy-config.json`):
   - **This is the ONLY source of truth during runtime**
   - Contains your API keys and active models
   - You MUST add models here for them to be available
   - You control all pricing and context window settings
   - If a model is not in the config, it cannot be used
   - **All models MUST pass strict validation or they will be rejected**

2. **Built-in MODEL_DEFINITIONS** (in `llm-proxy.js`):
   - Reference database of known models
   - Used ONLY for configuration management (--show-models, --sync)
   - Provides defaults when setting up new configurations
   - NEVER used during normal proxy operation

#### Strict Model Validation

**NEW**: The proxy server now enforces strict validation of all model configurations. Models that don't meet the requirements are automatically rejected and will not be available for use.

##### Validation Requirements by Provider

Each provider has specific pricing field requirements:

- **Google models**: Must have `input` and `output` pricing. Must NOT have `cacheRead` or `cacheWrite`.
- **OpenAI models**: Must have `input`, `output`, and `cacheRead` pricing. Must NOT have `cacheWrite`.
- **Anthropic models**: Must have all four pricing fields: `input`, `output`, `cacheRead`, and `cacheWrite`.

All models must also have:
- Valid `id` (string)
- Valid `contextWindow` (positive number)
- Valid `pricing` object with the required fields

##### Validation Behavior

- **At startup**: Invalid models are logged with specific error messages. Server continues with only valid models.
- **During requests**: Requests to invalid models are rejected with HTTP 400 error.
- **In API responses**: Only valid models are exposed via the `/models` endpoint.

##### Example Validation Errors

```
❌ Configuration validation errors:
   - openai model "gpt-4": Invalid pricing.cacheRead: must be a number >= 0
   - google model "gemini-pro": Invalid pricing: Google models should not have cacheRead
   - anthropic model "claude-3": Missing required fields (cacheWrite)
```

#### Using --show-models to Discover Changes

The `--show-models` command helps you understand the state of your configuration:

```bash
node llm-proxy.js --show-models
```

Output shows a status column for each model:
- **same**: Configuration matches built-in definitions
- **different**: Configuration has different pricing or context window
- **not in code**: Model exists in your config but not in built-in definitions (custom or deprecated)
- **not in config**: Model exists in built-in definitions but not in your config (new model available)

Example output:
```
🏢 OPENAI
---------------------------------------------------
Model ID                    Status         Context    Input $/MTok  Output $/MTok
gpt-4o                     same           128000     $2.00         $8.00
gpt-4-turbo               different       128000     $10.00        $30.00
custom-model              not in code     8192       $5.00         $10.00
gpt-4o-mini               not in config   128000     $0.40         $1.60
```

#### Managing Models in Configuration

##### Adding or Removing Models

**IMPORTANT**: All models must follow the strict validation requirements listed above.

Edit your configuration file to manage your models:

```json
{
  "providers": {
    "openai": {
      "apiKey": "sk-...",
      "models": [
        {
          "id": "gpt-4o",
          "contextWindow": 128000,
          "pricing": {
            "input": 2.50,    // Update when prices change
            "output": 10.00,
            "cacheRead": 0.625  // Required for OpenAI models
          }
        },
        // Add new models here - must follow validation rules
        {
          "id": "my-custom-openai-model",
          "contextWindow": 100000,
          "pricing": {
            "input": 5.00,
            "output": 20.00,
            "cacheRead": 5.00  // Required for OpenAI models
          }
        }
      ]
    }
  }
}
```

##### Updating Pricing

When providers change their pricing:

1. Edit the configuration file directly
2. Update the `pricing` object for affected models
3. Restart the proxy server
4. All future accounting will use the new prices

##### Using --sync (Optional)

The `--sync` command overwrites your model list with built-in definitions:

```bash
node llm-proxy.js --update-config --sync
```

**Warning**: This will:
- Replace ALL models with those from MODEL_DEFINITIONS
- Reset any custom pricing you've configured
- Remove any custom models you've added
- Preserve only your API keys

Use `--sync` only when you want to reset to defaults or after major updates.

#### Model Discovery from Provider APIs

For some providers, the proxy can fetch available models directly:

- **OpenAI**: Fetches from `/v1/models` endpoint
- **Google**: Fetches from `/v1/models` endpoint  
- **Anthropic**: No models endpoint available
- **Ollama**: Fetches from `/api/tags` endpoint (discovers all locally installed models)

To check which models are actually available with your API key:
```bash
node llm-proxy.js --update-config --sync --check-availability
```

For Ollama, the proxy automatically discovers all models installed locally and makes them available through the web interface. Model names with multiple colons (e.g., "llama3.3:latest", "hermes3:70b") are fully supported.

### Advanced Provider Configuration

#### Multiple Provider Instances

You can configure multiple instances of the same provider type with different settings:

```json
{
  "providers": {
    "openai-gpt4": {
      "apiKey": "sk-YOUR-API-KEY",
      "type": "openai",
      "baseUrl": "https://api.openai.com",
      "models": [
        {
          "id": "gpt-4o",
          "contextWindow": 128000,
          "pricing": { "input": 2.50, "output": 10.00, "cacheRead": 0.625 }
        }
      ]
    },
    "openai-o1": {
      "apiKey": "sk-YOUR-API-KEY",
      "type": "openai",
      "baseUrl": "https://api.openai.com",
      "models": [
        {
          "id": "o1-mini",
          "contextWindow": 128000,
          "endpoint": "responses",
          "supportsTools": false,
          "pricing": { "input": 3.00, "output": 12.00, "cacheRead": 0.75 }
        }
      ]
    },
    "ollama-local": {
      "type": "ollama",
      "endpoint": "http://localhost:11434",
      "models": []
    },
    "ollama-remote": {
      "type": "ollama",
      "endpoint": "http://remote-server:11434",
      "models": []
    }
  }
}
```

This allows you to:
- Separate models by use case (e.g., GPT-4 for complex tasks, o1 for reasoning)
- Use different API keys or endpoints for the same provider
- Connect to multiple Ollama instances (local and remote)

#### Model-Specific Configuration

Each model can have custom settings:

```json
{
  "id": "o1-mini",
  "contextWindow": 128000,
  "endpoint": "responses",      // Use /v1/responses instead of /v1/chat/completions
  "supportsTools": false,       // Disable tool/function calling for this model
  "pricing": {
    "input": 3.00,
    "output": 12.00,
    "cacheRead": 0.75
  }
}
```

**Model Configuration Options:**
- `endpoint`: Specify which API endpoint to use ("responses" for o1/o3 models)
- `supportsTools`: Enable/disable native tool calling (MCP tools)
  - `true`: Model supports function/tool calling (default for most models)
  - `false`: Disable tools for models that don't support them (e.g., o1 series)

**Note**: The web client automatically handles these configurations:
- Routes requests to the correct endpoint based on model configuration
- Filters out tools when sending requests to models with `supportsTools: false`
- Preserves full tool functionality for models that support it

### Pricing Information

#### How Pricing Works

Model pricing is stored in the built-in MODEL_DEFINITIONS and includes:

- **input**: Cost per million input tokens
- **output**: Cost per million output tokens
- **cacheRead**: Discounted rate for cached content (OpenAI/Anthropic)
- **cacheWrite**: Additional cost for creating cache (Anthropic only, 25% surcharge)

#### Pricing Sources

All pricing in MODEL_DEFINITIONS is manually maintained based on official provider pricing:

- **OpenAI**: https://openai.com/pricing
- **Anthropic**: https://www.anthropic.com/pricing
- **Google**: https://ai.google.dev/pricing

**Note**: The proxy does NOT fetch pricing from provider APIs as this information is not available programmatically. Prices must be updated manually in the code when providers change their rates.

#### Keeping Pricing Updated

To ensure accurate cost tracking:

1. Periodically check provider pricing pages for updates
2. Edit your configuration file to update prices for affected models
3. Restart the proxy server to load the new configuration

The accounting logs ONLY use pricing from your configuration file. **With strict validation enabled, models without proper pricing information are automatically rejected and cannot be used.** This ensures you have full control and awareness of all pricing used in your system, and prevents requests to improperly configured models.

### MCP Server Configuration

You can configure default MCP servers that will be automatically available in the web client:

```json
{
  "mcpServers": [
    {
      "id": "local_netdata",
      "name": "Local Netdata",
      "url": "ws://localhost:19999/mcp?api_key=YOUR_API_KEY"
    },
    {
      "id": "remote_server",
      "name": "Remote Server",
      "url": "ws://remote.example.com:19999/mcp?api_key=YOUR_API_KEY"
    }
  ]
}
```

- **id**: Unique identifier for the server (required)
- **name**: Display name shown in the UI (required)
- **url**: WebSocket URL for the MCP server (required)

These servers will be automatically loaded when users access the web client. Users can still add additional servers through the UI, which are stored in their browser's localStorage.

**Note**: If no MCP servers are configured, the proxy will automatically provide a default server pointing to `ws://localhost:19999/mcp` to ensure the web client can start properly. You'll need to configure the API key through the UI or add your own servers to the configuration.

### API Endpoints

The proxy server provides:
- `GET /` - Serves the web client interface (index.html)
- `GET /*.js`, `GET /*.css` - Serves web client static files
- `GET /models` - Returns available providers and models with pricing (JSON API)
- `GET /mcp-servers` - Returns configured MCP servers (JSON API)
- `POST /proxy/<provider>/<api-path>` - Proxies requests to LLM providers

### Accounting Logs

All LLM requests are logged to:
- **System service**: `/opt/llm-proxy/logs/llm-accounting-YYYY-MM-DD.jsonl`
- **Development**: `./logs/llm-accounting-YYYY-MM-DD.jsonl`

Log format:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "clientIp": "127.0.0.1",
  "provider": "openai",
  "model": "gpt-4",
  "endpoint": "/v1/chat/completions",
  "statusCode": 200,
  "duration": 1523,
  "requestBytes": 1024,
  "responseBytes": 2048,
  "decompressedBytes": 8192,
  "tokens": {
    "prompt": 150,
    "completion": 50,
    "cachedRead": 0,
    "cacheCreation": 0
  },
  "unitPricing": {
    "input": 30,
    "output": 60,
    "cacheRead": 30,
    "cacheWrite": 30
  },
  "costs": {
    "input": 0.0045,
    "output": 0.003,
    "cacheRead": 0,
    "cacheWrite": 0
  },
  "totalCost": 0.0075
}
```

Status codes:
- `200-299`: Successful responses
- `400-599`: HTTP errors from provider
- `0`: Network/connection failure

## Security Notes

- API keys are only stored in the server's configuration file
- The web client never sees or stores API keys
- Configure `allowedOrigins` in production for better security
- Keep your configuration file secure and never commit it to version control
- System service runs as dedicated `llm-proxy` user with restricted permissions

## Troubleshooting

### Proxy won't start
- Check if port 8081 is already in use
- Verify your configuration file is valid JSON
- Ensure at least one API key is configured
- For system service: check `sudo journalctl -u llm-proxy -e`
- For development: ensure `./logs` directory is writable

### Can't connect to proxy
- Verify the proxy is running (`node llm-proxy.js`)
- Check the proxy URL in settings (default: `http://localhost:8081`)
- Check browser console for CORS errors

### No models available
- Ensure API keys are correctly configured in your configuration file
- **Check for validation errors**: Look at server startup logs for model validation failures
- **Fix invalid models**: Ensure all models have required pricing fields for their provider
- Run `node llm-proxy.js --sync --update-config` to sync models (development)
- Restart the proxy after configuration changes
- Test the connection in the LLM provider settings

### Ollama model names display incorrectly
- **Fixed in latest version**: Models with multiple colons (e.g., "llama3.3:latest") now display correctly
- The web client automatically migrates existing chat data with incomplete model names
- Model names are now consistently stored with provider prefix (e.g., "ollama:llama3.3:latest")
- Token usage breakdown and accounting nodes properly display full model names

### Zero costs in accounting logs
- **This should no longer occur with strict validation** - invalid models are now rejected at startup
- Check if the model name is being extracted correctly
- Verify the model passes validation (see startup logs)
- For Google, ensure the model name matches (e.g., "gemini-1.5-pro" not "gemini-pro")
- Check console logs for "No pricing found for model" warnings (rare with validation)

### MCP connection fails
- Verify Netdata is running and MCP is enabled
- Check the WebSocket URL format
- Ensure the API key has appropriate permissions

### Model validation errors
- **NEW**: Check server startup output for specific validation error messages
- Fix pricing field requirements for each provider (see validation section above)
- Remove or fix models that don't meet provider-specific requirements
- Use `node llm-proxy.js --show-models` to compare config vs code definitions

### Accounting logs not writing
- For system service: Check `/opt/llm-proxy/logs` permissions
- For development: Ensure `./logs` directory exists and is writable
- Check console for "ACCOUNTING_FALLBACK" messages
- Look for backup logs in `/tmp/llm-accounting-backup.jsonl`