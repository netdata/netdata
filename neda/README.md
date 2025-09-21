# Neda - AI Assistant for Business Operations

## Overview

Neda is an AI-powered assistant integrated with Slack that provides access to internal business support systems. It uses a hierarchical multi-agent architecture where specialized sub-agents handle different domains of knowledge and services. The system is designed for Netdata's Sales and Management teams, providing comprehensive intelligence gathering and analysis capabilities.

## Architecture

- **Host Agent**: Main orchestrator (neda.ai) that manages conversations and coordinates sub-agents
- **Sub-Agents**: 20 specialized agents with focused capabilities and tool access
- **MCP Tools**: Model Context Protocol servers that provide specific functionalities
- **REST Tools**: Direct API integrations for specific services

## Security & Privacy

### Security Measures
- Read-only operations enforced through system prompts
- Risk mitigation for prompt injection attacks
- Limited access scope based on Slack channel invitations
- API keys stored securely in environment variables

### Privacy Protections
- Rules to prevent exposure of personal information and API keys
- Data exposure controls for AI providers
- Planned: Man-in-the-middle LLM for data sanitization

## Slack Integration

Neda is deployed as a Slack App using ai-agent's built-in Slack integration.

### Configuration

**Enablement**: Use `--slack` option when running ai-agent

**Required Environment Variables**:
```bash
SLACK_BOT_TOKEN=<xoxb-token>  # Bot User OAuth Token
SLACK_APP_TOKEN=<xapp-token>  # App-Level Token for Socket Mode
```

**Setup**:
1. Create Slack App at https://api.slack.com/apps
2. Add Bot User with required OAuth scopes:
   - `app_mentions:read` - Read messages that mention the app
   - `channels:history` - View messages in public channels
   - `channels:read` - View basic channel info
   - `chat:write` - Send messages
   - `im:history` - View direct messages
   - `im:read` - View basic DM info
   - `im:write` - Send direct messages
   - `users:read` - View people in the workspace
   - `users:read.email` - View email addresses of people in the workspace
3. Enable Socket Mode for real-time events
4. Install app to workspace
5. Get tokens from Slack App settings
6. Run ai-agent with: `ai-agent --agent ./neda.ai --slack --api 8800 --verbose`

### Operation

- **Access**: Only channels where Neda is explicitly invited
- **Mode**: Reactive - responds when @mentioned or in direct messages
- **Context**: Uses up to 50 past messages (max 25KB) for conversation context
- **Invitation**: Use `/invite @Neda` in any channel

### Limitations

- No image/screenshot processing
- No file attachment access
- Cannot access channels/threads where not invited
- Cannot proactively message users (reactive only)

### Testing

```bash
# Start Neda with Slack integration and API server
ai-agent --agent ./neda.ai --slack --api 8800 --verbose

# Or as the neda user:
su neda -c 'cd /opt/neda && ai-agent --agent ./neda.ai --slack --api 8800 --verbose'

# In Slack, invite Neda to a channel:
/invite @Neda

# Test interaction:
@Neda hello
```

## Sub-Agents Configuration

### Sales Data Enrichment Agents

#### company
**Purpose**: Enrich sales data by researching companies online - business profile, leadership, funding, company intelligence
**Integrations Used**: 
- `jina` - Web content extraction and parsing
- `fetcher` - URL content fetching
- `brave` - Web search engine
- `cloudflare-browser` - Web page rendering and screenshots

**Example Usage**:
```
Input: find everything we know about google.com
Output: General information about google.com including business profile, size, revenue, key people
```

---

#### company-tech
**Purpose**: Enrich sales data with company technology stack, infrastructure, engineering teams, and technical partnerships
**Integrations Used**: 
- `jina` - Web content extraction
- `fetcher` - URL content fetching  
- `brave` - Web search engine

**Features**: Analyzes job postings for tech stack insights, researches engineering management structure

**Example Usage**:
```
Input: find the technology stack and infrastructure of google.com
Output: Technology stack, infrastructure details, engineering structure and partners
```

---

#### contact
**Purpose**: Enrich sales data by researching professional profiles of individuals, build comprehensive professional background
**Integrations Used**:
- `fetcher` - URL content fetching
- `brave` - Web search engine
- `jina` - Web content extraction
- `cloudflare-browser` - Web page rendering and screenshots

**Example Usage**:
```
Input: find all information about Costa Tsaousis, working at Netdata
Output: Public professional profile of Costa Tsaousis, founder and CEO of Netdata
```

---

### Research Agents

#### web-research
**Purpose**: Open-ended web research on any topic, can fetch and summarize URLs
**Integrations Used**:
- `jina` - Web content extraction
- `brave` - Web search engine
- `fetcher` - URL content fetching
- `cloudflare-browser` - Web page rendering and screenshots

**Example Usage**:
```
Input: Research the latest trends in observability platforms
Output: Comprehensive research on current observability trends
```

---

### CRM Agents

#### hubspot
**Purpose**: Access Netdata's HubSpot CRM data - companies, contacts, deals, emails, meetings, notes, forms, web activity
**Integrations Used**:
- `hubspot` - HubSpot MCP server

**Live Data**: Real-time updates

**Example Usage**:
```
Input: find all information about company Ellusium
Output: CRM data including contacts, deals, web activity, engagement history
```

---

#### fireflies
**Purpose**: Analyze sales meeting transcripts for pain points, infrastructure scale, decision-makers, SEE sales stage mapping
**Integrations Used**:
- `fireflies` - Fireflies MCP server

**Example Usage**:
```
Input: find the last meeting with Ellusium and analyze pain points
Output: Meeting analysis with identified pain points and customer needs
```

---

### Payments & Invoicing Agents

#### stripe
**Purpose**: Access payment processing, invoicing, and subscription billing data
**Integrations Used**:
- `stripe` - Stripe MCP server

**Note**: Payment emails often use accounts@, payments@, finance@ addresses

**Example Usage**:
```
Input: find all billing information about company Ellusium
Output: Payment history, subscription details, invoices, billing contacts
```

---

### Product & Usage Analytics Agents

#### posthog
**Purpose**: Query user activities in-app and on public websites, analyze engagement patterns
**Integrations Used**:
- `posthog-query` - PostHog HogQL query API (REST tool)

**Live Data**: Real-time updates

**Example Usage**:
```
Input: check how frequently john.smith@google.com accessed Netdata Cloud
Output: Access frequency analysis and last access timestamp
```

---

### Netdata Cloud Production & Billing Agents

#### bigquery
**Purpose**: Query Netdata Cloud production systems data - infrastructure scale, nodes, subscriptions, billing, MRR/ARR
**Integrations Used**:
- `bigquery` - BigQuery MCP server

**Data Lag**: Few hours behind real-time

**Example Usage**:
```
Input: how many nodes and what hardware does Ellusium have?
Output: Node count, hardware specifications, cloud providers used
```

---

#### executive
**Purpose**: Analyze business metrics - ARR, MRR, churn, subscriptions, migrations
**Integrations Used**:
- `bigquery` - BigQuery MCP server (uses same data source)

**Note**: Based on BigQuery data, few hours lag. Use for trends over customer segments, use bigquery agent for individual customer data.

**Example Usage**:
```
Input: analyze overall churn over the last 90 days
Output: Churn analysis with ARR impact and trends
```

---

### Ticketing & Support Agents

#### freshdesk
**Purpose**: Retrieve and analyze support tickets from paying customers
**Integrations Used**:
- `freshdesk` - Freshdesk MCP server

**Features**: Ticket status, priority, SLAs, owners, conversation history

**Example Usage**:
```
Input: show open tickets for Ellusium this quarter
Output: Open tickets table with priorities, owners, SLA status
```

---

#### github
**Purpose**: Explore Netdata's GitHub repositories - code, PRs, issues, commit history
**Integrations Used**:
- `github` - GitHub MCP server

**Example Usage**:
```
Input: find where ml_anomaly_score is computed in netdata/netdata
Output: File paths with code snippets and GitHub links
```

---


### Marketing Agents

#### gsc
**Purpose**: Google Search Console SEO performance insights for Netdata websites
**Integrations Used**:
- `gsc1` - Google Search Console MCP server

**Websites**: www.netdata.cloud, learn.netdata.cloud, community.netdata.cloud

**Example Usage**:
```
Input: What are the top performing pages on learn.netdata.cloud?
Output: SEO performance metrics and top pages analysis
```

---

### Netdata Knowledge & Documentation Agents

#### source-code
**Purpose**: Direct filesystem access to all Netdata repositories for code analysis
**Integrations Used**:
- `filesystem` - Filesystem MCP server

**Example Usage**:
```
Input: Explain how the TROUBLESHOOT button works in dashboards
Output: Detailed code analysis of frontend and backend implementation
```

---

#### ask-netdata
**Purpose**: Query Netdata documentation using RAG-powered assistant
**Integrations Used**:
- `ask-netdata` - Ask Netdata API (REST tool)

**Note**: Optimized for fast documentation queries. Shareable link format:
`https://learn.netdata.cloud/docs/ask-netdata?q=<URL-encoded-question>`

**Example Usage**:
```
Input: How does Netdata help optimize SRE teams?
Output: Documentation-based answer about SRE optimization
```

---

### Monitoring Agents

#### netdata
**Purpose**: Health monitoring of Netdata Cloud production deployment
**Integrations Used**:
- `netdata_production` - Netdata production MCP server

**Example Usage**:
```
Input: how are netdata production systems today?
Output: Current operational status report
```

---


### Web & Traffic Analytics Agents

#### ga
**Purpose**: Google Analytics (GA4) traffic, engagement, acquisition, conversions, funnels, and retention
**Status**: Implemented
**Tools**: `analytics-mcp` (stdio)
**Testing**:
```bash
su neda -c '/opt/neda/ga.ai "traffic trends for www.netdata.cloud last 30 days" --verbose'
```

---

#### cloudflare
**Purpose**: Cloudflare Analytics - website traffic, performance, security events, AI bot analysis, and usage patterns with detailed hostname breakdowns
**Status**: Implemented
**Tools**: `cloudflare-graphql` (SSE), `cloudflare-radar` (SSE)
**Testing**:
```bash
su neda -c '/opt/neda/cloudflare.ai "analyze AI bot traffic on learn.netdata.cloud" --verbose'
```

---

#### zoominfo
**Purpose**: Enriched data about prospects
**Status**: Planned
**Planned Tools**: ZoomInfo API integration

---

#### encharge
**Purpose**: Access to mailing lists
**Status**: Partially implemented
**Current Tools**: `encharge` OpenAPI spec available in config

---

## MCP Servers & Tools Configuration

### Web Search & Content Tools

#### brave
**Type**: MCP Server (stdio)
**Purpose**: Web search engine
**Required Environment Variables**:
```bash
BRAVE_API_KEY=<your-brave-api-key>
```
**Setup**: 
- Subscription: "Pro AI" plan
- Get API key from https://brave.com/search/api/
- API key name on Brave website: "neda"
- Rate limit: 50 requests/second, unlimited total requests
- Payment: Brex card ending in 3782
**Testing**: 
```bash
# Test brave search using web-research agent
su neda -c '/opt/neda/web-research.ai "find latest news about observability" --verbose --tools brave'
```

---

#### jina
**Type**: MCP Server (SSE)
**Purpose**: Web content extraction and parsing
**Required Environment Variables**:
```bash
JINA_API_KEY=<your-jina-api-key>
```
**Setup**: 
- Get API key from https://jina.ai/api-dashboard/key-manager
- Payment: Card ending in 5763
**Testing**: 
```bash
# Test jina using web-research agent
su neda -c '/opt/neda/web-research.ai "extract content from https://example.com" --verbose --tools jina'
```

---

#### fetcher
**Type**: MCP Server (stdio)
**Purpose**: URL content fetching and web page parsing
**Source**: https://github.com/jae-jae/fetcher-mcp
**Required Environment Variables**: None
**Setup**: 
- Automatically installed via npx fetcher-mcp
- Requires Playwright browsers to be installed:
  ```bash
  # Install as neda user (included in neda-setup.sh)
  export PLAYWRIGHT_BROWSERS_PATH="/opt/neda/.cache/ms-playwright"
  sudo -u neda -E npx playwright install
  ```
**Testing**: 
```bash
# Test fetcher using web-research agent
su neda -c '/opt/neda/web-research.ai "today's news on observability? Use DuckDuckGo" --verbose --tools fetcher'
```

---

#### cloudflare-browser
**Type**: MCP Server (SSE)
**Purpose**: Fetch web pages, convert to markdown, take screenshots
**Required Environment Variables**:
```bash
CLOUDFLARE_API_KEY=<your-cloudflare-api-key>
```
**Setup**: 
- Create API token at https://dash.cloudflare.com/profile/api-tokens
- Required permissions:
  - Account → Account Settings:Read
  - Browser Rendering → Browser Rendering:Edit (Note: Write/Edit permission is required, not just Read)
**Testing**: 
```bash
# Test cloudflare-browser using web-research agent
su neda -c '/opt/neda/web-research.ai "get screenshot of https://example.com" --verbose --tools cloudflare-browser'
```

---

#### cloudflare-radar
**Type**: MCP Server (SSE)
**Purpose**: Global Internet traffic insights, trends, URL scans
**Required Environment Variables**:
```bash
CLOUDFLARE_API_KEY=<your-cloudflare-api-key>
```
**Setup**: 
- Uses same API token as cloudflare-browser
- Provides internet traffic analytics and security insights
**Testing**: 
```bash
# Test cloudflare-radar via ai-agent
ai-agent prompt "get internet traffic trends for the last week" --tools cloudflare-radar
```

---

#### cloudflare-graphql
**Type**: MCP Server (SSE)
**Purpose**: Analytics data using Cloudflare's GraphQL API
**Required Environment Variables**:
```bash
CLOUDFLARE_API_KEY=<your-cloudflare-api-key>
```
**Setup**: 
- Uses same API token as cloudflare-browser
- Access to Cloudflare analytics and performance data
**Testing**: 
```bash
# Test cloudflare-graphql via ai-agent
ai-agent prompt "get analytics data for my zones" --tools cloudflare-graphql
```

---

### Business Intelligence Tools

#### hubspot
**Type**: MCP Server (stdio)
**Purpose**: HubSpot CRM access
**Source**: Official HubSpot MCP server (@hubspot/mcp-server)
**Required Environment Variables**:
```bash
PRIVATE_APP_ACCESS_TOKEN=<your-hubspot-private-app-token>
```
**Setup**: 
- Use legacy app key named 'neda'
- Access at https://app.hubspot.com/legacy-apps/4567453
- Ensure app has appropriate CRM scopes
**Testing**: 
```bash
# Test hubspot using the hubspot agent
su neda -c '/opt/neda/hubspot.ai "how many users self-signed-up today?" --verbose'
```

---

#### stripe
**Type**: MCP Server (HTTP)
**Purpose**: Stripe payment data
**Required Environment Variables**:
```bash
STRIPE_SECRET_KEY=<your-stripe-secret-key>
```
**Setup**: 
- Get API key from https://dashboard.stripe.com/apikeys
- Use restricted key named 'neda'
- Ensure key has appropriate read permissions for payment data
**Testing**: 
```bash
# Test stripe using the stripe agent
su neda -c '/opt/neda/stripe.ai "any failed/errored/blocks payments today?" --verbose'
```

---

#### fireflies
**Type**: MCP Server (HTTP)
**Purpose**: Meeting transcripts and analysis
**Source**: Official Fireflies MCP server (HTTP remote)
**Required Environment Variables**:
```bash
FIREFLIES_API_KEY=<your-fireflies-api-key>
```
**Setup**: 
- Get API key from https://app.fireflies.ai/settings
- Single API key for all uses
**Testing**: 
```bash
# Test fireflies using the fireflies agent
su neda -c '/opt/neda/fireflies.ai "any meetings with customers in the last 2 days?" --verbose'
```

---

#### posthog-query
**Type**: REST Tool (Direct API)
**Purpose**: Product analytics with dynamic HogQL queries
**Note**: Uses direct API instead of MCP server (MCP server only supports predefined insights, or requires write access to create insights, and unfortunately does not support deletion, so as the application would run, posthog insights would just grow)
**API Endpoint**: `https://app.posthog.com/api/projects/${PROJECT}/query`
**Required Environment Variables**:
```bash
POSTHOG_QUERY_API_KEY=<your-posthog-query-api-key>
POSTHOG_QUERY_API_PROJECT=<your-project-id>  # Project ID: 7139
```
**Setup**: 
- Get API key from https://us.posthog.com/project/7139/settings/user-api-keys
- Uses Costa's personal API key named 'neda-query' (Posthog does not support any other type of api keys)
- Ensure key has query permissions
**Testing**: 
```bash
# Test posthog using the posthog agent
su neda -c '/opt/neda/posthog.ai "how many unique agent and cloud dashboard visitors in the last 24h?" --verbose'
```

---

#### bigquery
**Type**: MCP Server (stdio)
**Purpose**: Google BigQuery access for production data
**Required Environment Variables**:
```bash
BIGQUERY_PROJECT=netdata-analytics-bi
# GCP credentials file: .neda--netdata-analytics-bi--service-account.json
```
**Setup**: 
- GCP Project: `netdata-analytics-bi`
- Service Account: `neda`
- Credentials saved to: `.neda--netdata-analytics-bi--service-account.json`
- Ensure service account has BigQuery Data Viewer and Job User roles
**Testing**: 
```bash
# Test bigquery using the bigquery agent
su neda -c '/opt/neda/bigquery.ai "find spaces created yesterday" --verbose'
```

---

#### freshdesk
**Type**: MCP Server (stdio)
**Purpose**: Support tickets
**Source**: freshdesk-mcp (installed via uvx)
**Required Environment Variables**:
```bash
FRESHDESK_API_KEY=<your-freshdesk-api-key>
FRESHDESK_DOMAIN=netdatacloud  # without .freshdesk.com
```
**Setup**: 
- Get API key from Freshdesk settings (Profile > View Profile > API Key)
- Domain: `netdatacloud` (appears as netdatacloud.freshdesk.com)
- Ensure API key has appropriate read permissions
**Testing**: 
```bash
# Test freshdesk using the freshdesk agent
su neda -c '/opt/neda/freshdesk.ai "show open tickets for this week" --verbose'
```

---

### Ticketing & Issue Management

#### github
**Type**: MCP Server (stdio)
**Purpose**: GitHub issues, PRs, and ticketing system access
**Source**: @modelcontextprotocol/server-github
**Required Environment Variables**:
```bash
GITHUB_PERSONAL_ACCESS_TOKEN=<your-github-pat>
```
**Setup**: 
- Create PAT with repo, issues, and pull request access
- Token needs read access to netdata organization repositories
- **Note**: GitHub App authentication is not supported by the MCP server
  - The sync-netdata-repos.sh script uses GitHub App for cloning
  - MCP server requires Personal Access Token (PAT)
  - PATs don't expire but have less granular permissions than Apps
**Testing**: 
```bash
# Test github using the github agent
su neda -c '/opt/neda/github.ai "find open issues in netdata/netdata" --verbose'
```

---

#### filesystem
**Type**: MCP Server (stdio)
**Purpose**: Local filesystem access to Netdata repositories
**Source**: Custom fs-mcp-server.js (included in ai-agent)
**Required Environment Variables**:
```bash
NETDATA_REPOS_DIR=/opt/neda/repos  # Path to synced Netdata repositories
```
**Setup**: 
- Repositories are automatically synced by sync-netdata-repos.sh cron job
- Default path: `/opt/neda/repos` (contains all Netdata GitHub repositories)
- Server provides read-only access with path traversal protection
**Testing**: 
```bash
# Test filesystem using the source-code agent
su neda -c '/opt/neda/source-code.ai "how big is the file netdata-installer.sh of the netdata agent" --verbose'
```

---

#### netdata_production
**Type**: MCP Server (WebSocket)
**Purpose**: Infrastructure monitoring of Netdata Cloud production systems via Netdata MCP protocol
**Configuration**: WebSocket URL with API key (ws://10.20.1.126:19999/mcp)
**Features**: 
- Real-time metrics and performance data
- Anomaly detection and alerts
- Infrastructure health monitoring
- Service throughput analysis
**Note**: The netdata.ai agent currently only uses netdata_production (demos and costa instances are available but commented out)
**Setup**: 
- Contact DevOps for production access
- API key is pre-configured in deployment
**Testing**: 
```bash
# Test netdata monitoring using the netdata agent
su neda -c '/opt/neda/netdata.ai "check production systems health" --verbose'
```

---

#### gsc1/gsc2
**Type**: MCP Server (stdio)
**Purpose**: Google Search Console SEO data
**Required Environment Variables**:
```bash
GSC_CREDENTIALS=.neda--netdata-gemini--service-account.json
```
**Setup**: 
- GCP Project: `netdata-gemini`
- Service Account: `neda`
- Credentials saved to: `.neda--netdata-gemini--service-account.json`
- Ensure service account has Search Console API access
- Add service account email to Search Console properties as owner/viewer
**Testing**: 
```bash
# Test GSC using the gsc agent
su neda -c '/opt/neda/gsc.ai "top performing pages on learn.netdata.cloud" --verbose'
```

---

#### analytics-mcp
**Type**: MCP Server (stdio)
**Purpose**: Google Analytics data access
**Source**: analytics-mcp (installed via pipx)
**Required Environment Variables**:
```bash
GA_CREDENTIALS=.neda--netdata-gemini--service-account.json
GA_PROJECT_ID=netdata-gemini
```
**Setup**: 
- GCP Project: `netdata-gemini`
- Service Account: `neda` (same as GSC)
- Credentials saved to: `.neda--netdata-gemini--service-account.json`
- Ensure service account has Analytics API access
**Testing**: 
```bash
# Test analytics using the GA agent
su neda -c '/opt/neda/ga.ai "traffic trends for www.netdata.cloud last 30 days" --verbose'
```

---

### REST API Tools

#### ask-netdata
**Type**: REST Tool
**Purpose**: Documentation queries
**Configuration**: No API key required (public endpoint)
**Testing**: TBD

---

---

## Environment Variables Summary

Create a `.ai-agent.env` file with all required variables:

```bash
# Web Search & Content
BRAVE_API_KEY=
JINA_API_KEY=

# Business Intelligence
PRIVATE_APP_ACCESS_TOKEN=  # HubSpot
STRIPE_SECRET_KEY=
FIREFLIES_API_KEY=
POSTHOG_API_KEY=
POSTHOG_QUERY_API_KEY=
POSTHOG_QUERY_API_PROJECT=
BIGQUERY_PROJECT=
BIGQUERY_CREDENTIALS=
FRESHDESK_API_KEY=
FRESHDESK_DOMAIN=

# Slack Integration (built-in, not MCP)
SLACK_BOT_TOKEN=  # Bot User OAuth Token (xoxb-)
SLACK_APP_TOKEN=  # App-Level Token for Socket Mode (xapp-)

# Ticketing & Code Access
GITHUB_PERSONAL_ACCESS_TOKEN=

# Development & Monitoring
NETDATA_REPOS_DIR=

# Google Services
GSC_CREDENTIALS=
GA_CREDENTIALS=
GA_PROJECT_ID=

# LLM Providers
ANTHROPIC_API_KEY=
OPENAI_API_KEY=
GOOGLE_API_KEY=
OPENROUTER_API_KEY=

# API Access
API_BEARER_TOKEN=

# Additional Services
CONTEXT7_API_KEY=
ENCHARGE_API_KEY=
```

## Testing

### Individual Agent Testing
```bash
# Test specific agent with a query
ai-agent --agent neda/<agent-name>.ai "<test query>"

# Example
ai-agent --agent neda/hubspot.ai "find information about example.com"
```

### Tool Testing
```bash
# Test MCP server connection
ai-agent --test-mcp <server-name>

# Test REST tool
ai-agent --test-rest <tool-name>
```

### Integration Testing
```bash
# Test full Neda with sub-agents
ai-agent --agent neda/neda.ai "test all connections"
```

### Using the test script
Note: Example script not included in this repository. Use the individual commands above to validate each integration.

## Deployment

### Prerequisites
- Node.js 20+
- Python 3.8+ with pip and venv
- curl for downloading binaries
- Git for repository cloning
- Access to required API keys
- Slack workspace with app installation permissions
- GCP credentials for BigQuery access

### Installation

The `neda-setup.sh` script handles most dependencies automatically:

```bash
# Run the setup script (requires sudo)
cd /path/to/ai-agent/neda
sudo ./neda-setup.sh

# The setup script automatically installs:
# ✓ Google Cloud SDK (for BigQuery and other GCP services)
# ✓ Google AI Toolbox binary (for BigQuery MCP server)
# ✓ Python tools: pipx and uvx (for MCP servers)
# ✓ mcp-gsc Python package and dependencies
# ✓ Playwright browsers (for fetcher MCP server)
# ✓ Repository sync script and GitHub App configuration
# ✓ Creates systemd service files (not auto-installed)
# ✓ Initializes git repository for configuration tracking

# After setup, configure your environment:
cp .env.example /opt/neda/.env
# Edit /opt/neda/.env with your API keys

# Optional: Install systemd services
sudo cp /opt/neda/neda.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable neda
sudo systemctl start neda
```

### Dependencies Managed by Setup Script

| Component | Purpose | Installed To |
|-----------|---------|--------------|
| Google Cloud SDK | BigQuery and GCP access | `/opt/neda/google-cloud-sdk/` |
| Google AI Toolbox | BigQuery MCP server | `/opt/neda/bin/toolbox` |
| pipx | Python package runner | System package |
| uvx (part of uv) | Fast Python tool runner | System binary |
| mcp-gsc | Google Search Console MCP | `/opt/neda/mcp/mcp-gsc/` |
| Playwright browsers | Web scraping for fetcher | `/opt/neda/.cache/ms-playwright/` |
| Python venv | Isolated Python environments | Various locations |
| Git repository | Configuration version control | `/opt/neda/.git/` |

### Configuration Files
- `.ai-agent.json` - Main configuration with MCP servers, tools, and defaults
- `neda/*.ai` - Individual agent configurations
- `.env` - Environment variables and API keys

## Troubleshooting

### Common Issues

#### Agent Not Responding
- Check API key configuration in .env
- Verify MCP server is enabled in .ai-agent.json
- Review logs: `journalctl -u neda -f`
- Test individual MCP server connection

#### Data Access Issues
- Verify permissions for the specific service
- Check rate limits (especially for API services)
- Ensure API key has required scopes
- For BigQuery: verify GCP credentials are properly configured
- For HubSpot: ensure private app has necessary scopes

#### Cross-System Data Linking
- HubSpot ↔ BigQuery: linked via email addresses and space names
- Stripe ↔ BigQuery: linked via Stripe ID and email domains
- HubSpot ↔ Stripe: linked via space names and email domains
- Note: Stripe often uses finance@, payments@, accounts@ emails

#### Performance Issues
- Adjust `maxConcurrentTools` in .ai-agent.json
- Increase `llmTimeout` and `toolTimeout` for slow operations
- Monitor token usage with accounting logs

## Data Synthesis & Cross-Referencing

Neda can cross-reference data between systems using these common identifiers:

### Primary Linking Fields
- **Email addresses**: Primary key across most systems
- **Company domains**: Links company records
- **Space names/IDs**: Netdata Cloud workspace identifiers
- **Stripe IDs**: Payment system references

### System Integration Notes
- **HubSpot**: Live data, real-time updates
- **PostHog**: Live data, real-time events
- **BigQuery**: Few hours lag, production data snapshot
- **Executive**: Based on BigQuery, same lag
- **Stripe**: Payment contacts often differ from primary contacts
