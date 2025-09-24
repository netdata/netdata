# Netdata AI

Netdata AI brings synthesis, investigation, and automation to your observability data—turning per‑second telemetry into explanations, recommendations, and faster incident resolution.

## What's Available Today

### 1. AI Chat with Netdata

**Available Now** - Chat with your infrastructure using natural language

Ask questions about your infrastructure like you're talking to a colleague. Get instant answers about performance, find specific logs, identify top resource consumers, or investigate issues - all through simple conversation. No more complex queries or dashboard hunting.

**Key capabilities**:

- **Natural language queries** - "Which servers have high CPU usage?" or "Show database errors from last hour" or "What is wrong with my infrastructure now", or "Do a post-mortem analysis of the outage we had yesteday", or "Show me all network dependencies of process X"
- **Multi-node visibility** - Analyzes your entire infrastructure through Netdata Parents
- **Flexible AI options** - Use your existing AI tools or our standalone web chat

<details>
<summary><strong>How it works</strong></summary>

- **MCP integration** - You chat with an LLM, that has access to your observability data, via Model Context Protocol (MCP)
- **Choice of AI providers** - Claude, GPT-4, Gemini, and others
- **Two deployment options** - Use an existing AI client that supports MCP, or use a web page chat we created for it (LLM is pay-per-use with API keys)
- **Real-time data access** - Query live metrics, logs, processes, network connections, and system state
- **Secure connection** - LLM has access to your data via the LLM client

</details>

**Access**: Available now for all Netdata Agent deployments (Standalone and Parents)

[Explore AI Chat →](/docs/ml-ai/ai-chat-netdata/ai-chat-netdata.md)

### 2. MCP Clients

**Available Now** - Transform observability into action with CLI AI assistants

Combine the power of AI with system automation. CLI-based AI assistants like Claude Code and Gemini CLI can access your Netdata metrics and execute commands, enabling intelligent infrastructure optimization, automated troubleshooting, and configuration management - all driven by real observability data.

**Key capabilities**:

- **Observability-driven automation** - AI analyzes metrics and executes fixes
- **Infrastructure optimization** - Automatic tuning based on performance data
- **Intelligent troubleshooting** - From problem detection to resolution
- **Configuration management** - AI-generated configs based on actual usage

<details>
<summary><strong>How it works</strong></summary>

- **MCP-enabled CLI tools** - Claude Code, Gemini CLI, and others
- **Bidirectional integration** - Read metrics, execute commands
- **Context-aware decisions** - AI understands your infrastructure state
- **Safe execution** - Review AI suggestions before implementation
- **Team collaboration** - Share configurations via version control

</details>

**Access**: Available now with MCP‑supported AI clients

[Explore MCP Clients →](/docs/ml-ai/ai-devops-copilot/ai-devops-copilot.md)

### 3. AI Insights

**Preview (Netdata Cloud Feature)** - Strategic infrastructure analysis in minutes

Transform past data into actionable insights with AI-generated reports. Perfect for capacity planning, performance reviews, and executive briefings. Get comprehensive analysis of your infrastructure trends, optimization opportunities, and future requirements - all in professionally formatted PDFs.

**Four report types**:

- **Infrastructure Summary** - Complete system health and incident analysis
- **Capacity Planning** - Growth projections and resource recommendations
- **Performance Optimization** - Bottleneck identification and tuning suggestions
- **Anomaly Analysis** - Deep dive into unusual patterns and their impacts

<details>
<summary><strong>How it works</strong></summary>

- **2-3 minute generation** - Analyzes historical data comprehensively
- **PDF downloads** - Professional reports ready for sharing
- **Embedded visualizations** - Charts and graphs from your actual data
- **Executive-ready** - Clear summaries with technical details included
- **Secure processing** - Data analyzed then immediately discarded

</details>

**Access**:

- Eligible Spaces receive 10 free AI runs/month; additional usage via AI Credits
- Available for Business and Free Trial plans

[Explore AI Reports →](/docs/ml-ai/ai-insights.md)


### 4. Anomaly Advisor

**Available to All** - Revolutionary troubleshooting that finds root causes in minutes

Stop guessing what went wrong. The Anomaly Advisor instantly shows you how problems cascade across your infrastructure and ranks every metric by anomaly severity. Root causes typically appear in the top 20-30 results, turning hours of investigation into minutes of discovery.

**Revolutionary approach**:

- **See cascading effects** - Watch anomalies propagate across systems
- **Automatic ranking** - Every metric scored and sorted by anomaly severity
- **No expertise required** - Works even on unfamiliar systems

<details>
<summary><strong>How it works</strong></summary>

- **Data-driven analysis** - No hypotheses needed, the data reveals the story
- **Influence tracking** - Shows what influenced and what was influenced
- **Time window analysis** - Highlight any incident period for investigation
- **Scale-agnostic** - Works identically from 10 to 10,000 nodes
- **Visual propagation** - See anomaly clusters and cascades instantly

</details>

**Find it**: Anomalies tab in any Netdata dashboard

[Learn more about Anomaly Advisor →](/docs/ml-ai/anomaly-advisor.md)

### 5. Machine Learning Anomaly Detection

**Available to All** - Continuous anomaly detection on every metric

The foundation of Netdata's AI capabilities. Machine learning models run locally on every agent, continuously learning normal patterns and detecting anomalies in real-time. Zero configuration required - it just works, protecting your infrastructure 24/7.

**Automatic protection**:

- **Every metric monitored** - ML analyzes all metrics continuously
- **Visual anomaly indicators** - Purple ribbons on every chart show anomaly rates
- **Historical anomaly data** - ML scores saved with metrics for past analysis
- **Zero configuration** - Starts working immediately after installation

<details>
<summary><strong>How it works</strong></summary>

- **Local ML engine** - Runs on every Netdata Agent, no cloud dependency
- **Multiple models** - Consensus approach reduces noise and false positives by 99%
- **Integrated storage** - Anomaly scores saved in the database with metrics
- **Historical queries** - Query past anomaly rates just like any other metric
- **Visual integration** - Purple anomaly ribbons appear on all charts automatically
- **Minimal overhead** - Designed for production environments
- **Privacy by design** - Your data never leaves your infrastructure

</details>

**Access**: Free for everyone - enabled by default

[Explore Machine Learning →](/docs/ml-ai/ml-anomaly-detection/ml-anomaly-detection.md)

### 6. AI-Powered Alert Troubleshooting

When an alert fires, you can now use AI to get a detailed troubleshooting report that determines whether the alert requires immediate action or is just noise. The AI examines your alert's history, correlates it with thousands of other metrics across your infrastructure, and provides actionable insights—all within minutes.

**Key capabilities**:
- **Automated Analysis:** Click "Ask AI" on any alert to generate a comprehensive troubleshooting report
- **Correlation Discovery:** AI scans thousands of metrics to find what else was behaving abnormally
- **Root Cause Hypothesis:** Get likely root causes with specific metrics and dimensions that matter most
- **Noise Reduction:** Quickly identify false positives versus legitimate issues

**How to access**:
- From the Alerts tab: Click the "Ask AI" button on any alert
- From the Insights tab: Select "Alert Troubleshooting" and choose an alert
- From email notifications: Click "Troubleshoot with AI" link

Reports are generated in 1-2 minutes and saved in your Insights tab. All Business plan users get 10 AI troubleshooting sessions per month during trial.

**Access**: Netdata Cloud Business Feature

## Coming Soon

### AI Chat with Netdata (Netdata Cloud version)

**In Development** - Chat with your entire infrastructure through Netdata Cloud

Soon, Netdata Cloud will become an MCP server itself. This means you'll be able to chat with your entire infrastructure without setting up local MCP bridges. Get the same natural language capabilities with the added benefits of Cloud's global view, team collaboration, and seamless access from anywhere.

**What to expect**:

- Direct MCP integration with Netdata Cloud
- Chat with all your infrastructure from one place
- No local bridge setup required
- Team collaboration on AI conversations
- Access from any device, anywhere

### AI Weekly Digest

**In Development (Netdata Cloud)** - Your infrastructure insights delivered weekly

Stay informed without information overload. The AI Weekly Digest will analyze your infrastructure's performance over the past week and deliver a concise summary of what matters most - trends, issues resolved, optimization opportunities, and what to watch next week.

**What to expect**:

- Weekly email summaries customized for your role
- Key metrics and trend analysis
- Proactive recommendations for the week ahead
- Highlights of resolved and ongoing issues
