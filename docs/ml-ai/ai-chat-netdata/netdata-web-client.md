# Netdata Web Client

A self-hosted AI chat interface purpose-built for infrastructure observability, featuring advanced cost optimization and multi-provider LLM support.

![Netdata Web Client Interface](https://github.com/user-attachments/assets/f2facc59-66c1-4ea5-9404-d335e8f67ff2)

## Purpose-Built for Observability

The Netdata Web Client is an open-source browser-based AI assistant that connects directly to your Netdata infrastructure via MCP (Model Context Protocol). Unlike generic AI chat interfaces, it's specifically optimized for DevOps and SRE workflows with specialized system prompts and infrastructure-aware features.

## Key Features

### Multi-Provider LLM Support

- **Unified Interface** - Switch between OpenAI (GPT-4), Anthropic (Claude), and Google (Gemini) models within the same conversation
- **Model Discovery** - Automatically detects available models from each provider
- **Provider Arbitrage** - Use cheaper models for simple queries, premium models for complex analysis

### Advanced Cost Optimization

- **Real-time Cost Tracking** - See exact costs per message and cumulative session costs
- **Token Accounting** - Detailed breakdown of input, output, cache read, and cache write tokens
- **Smart Context Management**:
  - Automatic tool memory pruning after N conversation turns
  - Large response summarization using cheaper models
  - Configurable cache control strategies
  - Auto-summarization when approaching context limits
- **Safety Limits** - Prevents runaway costs with iteration and request size limits

### Infrastructure-Specific Features

- **DevOps System Prompts** - Pre-configured for infrastructure monitoring and analysis
- **Time-Aware Analysis** - Understands relative time references ("last night", "this morning")
- **Multi-Chat Architecture** - Run parallel investigations in separate contexts
- **MCP WebSocket Integration** - Real-time connection to multiple Netdata instances

### Professional Observability Workflow

- **Persistent Conversations** - All chats auto-saved with full history
- **Accounting Logs** - JSONL export for cost analysis and auditing
- **Error Recovery** - Maintains state through API failures
- **Rich Formatting** - Markdown, code blocks, tables, and ASCII diagrams

## Cost-Effective Infrastructure Analysis

The web client's cost optimization features make AI-assisted troubleshooting affordable at scale:

| Feature | Cost Impact |
|---------|------------|
| Tool Memory Window | -40% context size |
| Response Summarization | -60% for large outputs |
| Smart Caching | -30% for repetitive queries |
| Model Selection | -80% using appropriate models |

## Get Started

The complete source code, installation instructions, and documentation are available on GitHub:

🔗 **[github.com/netdata/netdata/tree/master/src/web/mcp/mcp-web-client](https://github.com/netdata/netdata/tree/master/src/web/mcp/mcp-web-client)**

### Quick Start

```bash
# Clone and run locally
git clone https://github.com/netdata/netdata.git
cd netdata/src/web/mcp/mcp-web-client
node llm-proxy.js
# Open http://localhost:3456 in your browser
```

### Requirements

- Node.js 18+
- API keys from OpenAI, Anthropic, or Google
- Access to a Netdata instance with MCP enabled
- Modern web browser

## Why Self-Host?

- **Data Privacy** - Your infrastructure data never leaves your control
- **Cost Control** - Use your own API keys with transparent pay-per-use pricing
- **Customization** - Modify prompts and behavior for your specific needs
- **No Vendor Lock-in** - Switch LLM providers anytime

## Ideal For

- **Cost-Conscious Teams** - Pay only for what you use
- **Security-Focused Organizations** - Keep all data within your infrastructure
- **Advanced Users** - Full control over prompts and model selection
- **Multi-Cloud Environments** - Connect to multiple Netdata instances

---

For detailed setup instructions, configuration options, and the complete feature list, visit the [GitHub repository](https://github.com/netdata/netdata/tree/master/src/web/mcp/mcp-web-client).
