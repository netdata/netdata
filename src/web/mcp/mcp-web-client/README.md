# Netdata MCP LLM Client

A web-based client for interacting with Netdata's Model Context Protocol (MCP) server using various LLM providers.

## Setup Guide

### Prerequisites

- Node.js (v14 or higher)
- A running Netdata instance with MCP server enabled
- API keys for at least one LLM provider (OpenAI, Anthropic, or Google)

### 1. Setting up the LLM Proxy Server

The proxy server manages API keys securely and handles CORS for browser-based access to LLM APIs.

#### First Run

1. Start the proxy server:
   ```bash
   node llm-proxy.js
   ```

2. On first run, it will create `~/.config/llm-proxy-config.json` and exit with instructions.

3. Edit `~/.config/llm-proxy-config.json` to add your API keys:
   ```json
   {
     "port": 8081,
     "allowedOrigins": "*",
     "providers": {
       "openai": {
         "apiKey": "sk-YOUR-OPENAI-KEY",
         "models": ["gpt-4-turbo-preview", "gpt-4", "gpt-3.5-turbo"]
       },
       "anthropic": {
         "apiKey": "sk-ant-YOUR-ANTHROPIC-KEY",
         "models": ["claude-3-opus-20240229", "claude-3-sonnet-20240229"]
       },
       "google": {
         "apiKey": "YOUR-GOOGLE-AI-KEY",
         "models": ["gemini-pro", "gemini-pro-vision"]
       }
     }
   }
   ```

4. Start the proxy server again:
   ```bash
   node llm-proxy.js
   ```

   You should see output like:
   ```
   LLM CORS Proxy Server running on http://localhost:8081
   
   Configured providers:
     - openai: 3 models
     - anthropic: 2 models
   ```

### 2. Accessing the Web Client

1. Open `index.html` in your web browser:
   - You can open it directly as a file (`file:///path/to/index.html`)
   - Or serve it via a web server if preferred

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
- **Multiple LLM Support**: Use OpenAI, Anthropic, or Google AI models
- **Model Selection**: Choose specific models for each chat
- **MCP Integration**: Full access to Netdata metrics and functions
- **Chat History**: All conversations are saved locally
- **Temperature Control**: Adjust response creativity per chat
- **Context Window Tracking**: Monitor token usage in real-time

## Proxy Endpoints

The proxy server provides:
- `GET /models` - List available providers and models
- `POST /proxy/<provider>/<api-path>` - Proxy requests to LLM providers

## Security Notes

- API keys are only stored in `~/.config/llm-proxy-config.json` on the server
- The web client never sees or stores API keys
- Configure `allowedOrigins` in production for better security
- Keep `~/.config/llm-proxy-config.json` secure and never commit it to version control

## Troubleshooting

### Proxy won't start
- Check if port 8081 is already in use
- Verify `~/.config/llm-proxy-config.json` is valid JSON
- Ensure at least one API key is configured

### Can't connect to proxy
- Verify the proxy is running (`node llm-proxy.js`)
- Check the proxy URL in settings (default: `http://localhost:8081`)
- Check browser console for CORS errors

### No models available
- Ensure API keys are correctly configured in `~/.config/llm-proxy-config.json`
- Restart the proxy after configuration changes
- Test the connection in the LLM provider settings

### MCP connection fails
- Verify Netdata is running and MCP is enabled
- Check the WebSocket URL format
- Ensure the API key has appropriate permissions