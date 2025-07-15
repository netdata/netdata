# Getting Started with Netdata

## Transform Your Infrastructure Monitoring in Minutes, Not Weeks

Netdata Cloud gives you complete visibility into every system and application performance metric across your entire infrastructure, whether it's on-premises or in the cloud. Bring teams together to find answers faster and squash the threat of anomalies or outages with composite charts, metric correlations, pre-built and custom dashboards, intelligent alarms, and collaboration tools to help you drive down your time to resolution.

:::tip

This guide will show you how simple it is to get started with Netdata and experience the instant value of real-time observability.

:::

### Getting Started

[**Getting Started Guide Netdata**](https://www.youtube.com/watch?v=he-ysUlrZIw)
## [1. Sign in & Access Your Space](https://github.com/netdata/netdata/blob/master/src/claim/README.md)

Getting started is as simple as visiting [app.netdata.cloud](https://app.netdata.cloud/) and creating your account. 

**What happens next:**
- Your personalized Space is automatically created
- You get your central hub for all monitoring needs
- The dashboard is ready to receive your first node

:::info

**Why Cloud Connection is Essential:** Starting with Netdata v2.0, all dashboards use Netdata Cloud for authentication, providing enhanced security and unified experience across all platforms. This unlocks enterprise-grade monitoring capabilities for deployments of any size.

:::

## [2. Connect a Node & See Instant Results](https://github.com/netdata/netdata/blob/master/src/claim/README.md)

**Connect Your First Agent**
Once logged into Netdata Cloud, you'll see connection instructions. There are three easy ways to connect:

<details>
<summary><strong>Method 1: Through the Cloud Interface</strong></summary><br/>

1. Navigate to **Space Settings** (cogwheel icon)
2. Select **Nodes** tab
3. Click the **"+"** button to add a new node
4. Copy and run the generated connection command

</details>

<details>
<summary><strong>Method 2: From the Nodes Tab</strong></summary><br/>

1. Go to the **Nodes** tab in your Room
2. Click **Add nodes** button
3. Follow the step-by-step instructions

</details>

<details>
<summary><strong>Method 3: Via Integrations Page</strong></summary><br/>

1. Visit the **Integrations** page
2. Select your OS or container environment
3. Execute the provided connection command

</details>

:::tip

**The One-Command Solution:**
All methods will show you a command like this:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --claim-token YOUR_TOKEN --claim-rooms YOUR_ROOMS --claim-url https://app.netdata.cloud
```
:::

:::info

<details>
<summary><strong>What this single command does:</strong></summary><br/>

- Automatically detects your operating system
- Installs the latest Netdata Agent
- Connects to your Cloud Space
- Starts monitoring immediately

</details>

:::

**What You'll See When Connected:**

Within seconds of connection, you'll experience the power of real-time observability:
- **Your node appears live in your Space**
- **Charts immediately start streaming real-time data**
- **System Overview dashboard populates automatically**
- **All metrics update with 1-second granularity**
- **Zero additional configuration required**

## [3. Analyze Your Data & See What You Get Out-of-the-Box](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/README.md)

**Automatic Dashboards:**
- **System Overview** - Fully automated dashboard showing all your nodes
- **Nodes Tab** - Unified view of all infrastructure with key metrics
- **Composite Charts** - Data from multiple nodes combined intelligently
- **Real-Time Updates** - Every metric updates with 1-second granularity

**Auto-Discovery in Action:**
Netdata automatically discovers and monitors:
- **System Resources** - CPU, memory, disk, network
- **Containers** - Docker, Kubernetes, LXC
- **Databases** - MySQL, PostgreSQL, MongoDB, Redis
- **Web Servers** - Apache, Nginx, IIS
- **Applications** - Over 1000+ integrations available

**[Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/metric-correlations.md):**
Click the correlation button on any chart to instantly find related metrics that help diagnose issues - turning complex troubleshooting into point-and-click simplicity.

## [4. Customize & Set New Alerts](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md)

**Out-of-the-Box Alerts:**
Netdata comes with intelligent alerts pre-configured for common issues. But you can easily customize them.

**Simple Alert Customization:**
Navigate to your node and click "Edit" on any alert to modify thresholds:

```bash
# Example: Adjust CPU warning from 85% to 75%
warn: $this > 75
crit: $this > 90
```

**Create New Alerts:**
```bash
# Example: Custom RAM alert
alarm: custom_ram_usage
   on: system.ram
 calc: $used * 100 / ($used + $free)
 warn: $this > 80
 crit: $this > 95
```

**Notification Methods:**
Configure alerts to reach you through:
- **Email** - Direct email notifications
- **Slack** - Team channel integration
- **PagerDuty** - Incident management
- **Microsoft Teams** - Workplace collaboration
- **Plus dozens more** - Webhooks, SMS, and custom integrations

:::tip

**Silencing Alerts:** Need to temporarily quiet alerts during maintenance?

**Quick Silence Options:**
- **Individual alerts** - Change `to: silent` in alert configuration
- **Specific alerts** - Edit `netdata.conf` with `enabled alarms = !alert_name *`
- **All alerts** - Set `enabled = no` in `[health]` section

**Temporary Control:** Use the Health Management API for dynamic control without config changes - perfect for maintenance windows.

**Permanent Solutions:**
- **Disable specific alerts permanently** - Comment out alert definitions in health configuration files and reload with `sudo netdatacli reload-health`
- **Remove noisy alerts completely** - Delete unwanted alert configurations from `health.d/*.conf` files

:::

## [5. Organize Your Infrastructure & Invite Your Team](https://github.com/netdata/netdata/blob/master/docs/netdata-cloud/organize-your-infrastructure-invite-your-team.md)

**Spaces and Rooms:**
- **Space** - Your main collaboration environment for the entire team
- **Rooms** - Flexible groupings within Spaces (by service, location, or purpose)
- **Example**: Create rooms for "Production", "Development", "Database Servers"

**Team Collaboration:**
Click "Invite Users" in your Space sidebar to add team members. Set appropriate access levels:
- **Admins** - Full control over Spaces, Rooms, and billing
- **Managers** - Room and user management
- **Troubleshooters** - Monitoring and analysis access
- **Observers** - View-only access to specific rooms

:::tip

**Role-Based Access Control (RBAC):** Business plan subscribers get fine-grained control over who can access what data, execute functions, and modify configurations - perfect for teams with different responsibilities.

:::

**Organize by Your Needs:**

| **Category** | **Examples** |
|---|---|
| **By Service** | Web servers, databases, applications |
| **By Location** | Data centers, cloud regions |
| **By Team** | DevOps, SRE, development teams |
| **By Environment** | Production, staging, development |

## What's the Value for You

### Experience the Difference with Business Plan

**[Start Your Free Business Trial](https://netdata.cloud/pricing):** Experience the full power of Netdata Business with our free trial:
- **No credit card required** - Start immediately
- **Full access to all features** - Nothing held back
- **Cancel anytime** - No commitments
- **[Expert support](https://www.netdata.cloud/support/)** - Get help when you need it

### Traditional Monitoring vs Netdata Business

| **Traditional Monitoring** | | **Netdata Business** |
|---|:---:|---|
| **Navigate complex interfaces** during incidents | | **Get instant analysis** with natural language |
| **Build dashboards** during incidents | **VS** | **Automatic dashboards** with zero configuration |
| **Manually correlate data** across systems | | **AI-powered correlation** and root cause analysis |
| **Wait 15 seconds to 1 minute** for updates | | **Real-time 1-second granularity** |
| **Pay per metric with surprise bills** | | **70% less expensive than most competitors** |

### [AI Capabilities That Transform Operations](https://github.com/netdata/netdata/blob/master/docs/category-overview-pages/machine-learning-and-assisted-troubleshooting.md)

Experience the future of infrastructure monitoring with AI that actually works. Chat with your infrastructure in natural language, get professional reports in minutes, and let machine learning find problems before they impact your users. From automated troubleshooting to predictive insights, Netdata's AI capabilities turn complex monitoring into simple conversations.

**AI Features Overview:**

| **Capability** | **What It Does** | **Access** |
|---|---|---|
| **AI Chat with Netdata** | Ask questions in natural language | Available now for all deployments |
| **AI DevOps Copilot** | CLI-based AI automation | Available now with MCP tools |
| **AI Insights** | Professional reports in 2-3 minutes | Business plans get unlimited reports |
| **Anomaly Advisor** | Find root causes in minutes | Available to all users |
| **ML Anomaly Detection** | Continuous anomaly detection | Free for everyone |

#### Ask Questions & Get Answers

<details>
<summary><strong>AI Chat with Netdata</strong></summary><br/>

Transform troubleshooting from complex queries to natural conversation. Ask questions like "Which servers have high CPU usage?" or "Show database errors from last hour" or "What is wrong with my infrastructure now?"

**Why this matters:** No more complex queries or dashboard hunting - get instant answers about performance, find specific logs, identify top resource consumers, or investigate issues through simple conversation.

**How it works:** Multi-node visibility through Netdata Parents, flexible AI options including Claude, GPT-4, and Gemini, with real-time access to metrics, logs, processes, network connections, and system state.

</details>

<details>
<summary><strong>Model Context Protocol (MCP) Integration</strong></summary><br/>

Every Netdata Agent and Parent is an MCP server, enabling seamless integration with AI assistants for natural language queries and automated analysis.

**Why this matters:** Use your existing AI tools or our standalone web chat with choice of AI providers. Query live metrics, logs, processes, network connections, and system state securely.

**Technical details:** MCP integration via WebSocket, choice of Claude, GPT-4, Gemini and others, two deployment options available, real-time data access, secure connection where LLM has access to your data via the LLM client.

</details>

#### Automate & Optimize

<details>
<summary><strong>AI DevOps Copilot</strong></summary><br/>

Transform observability into action with CLI AI assistants. Combine the power of AI with system automation for intelligent infrastructure optimization.

**Why this matters:** CLI-based AI assistants like Claude Code and Gemini CLI can access your Netdata metrics and execute commands, enabling observability-driven automation, automated troubleshooting, and configuration management driven by real observability data.

**Key capabilities:** Observability-driven automation where AI analyzes metrics and executes fixes, infrastructure optimization with automatic tuning based on performance data, intelligent troubleshooting from problem detection to resolution, and AI-generated configs based on actual usage.

</details>

#### Analyze & Report

<details>
<summary><strong>AI Insights - Professional Reports</strong></summary><br/>

Generate comprehensive reports in 2-3 minutes that explain what happened, why it happened, and what to do about it. Transform past data into actionable insights with AI-generated reports.

**Why this matters:** Perfect for capacity planning, performance reviews, and executive briefings. Get comprehensive analysis of your infrastructure trends, optimization opportunities, and future requirements in professionally formatted PDFs.

**Four report types:**
- **Infrastructure Summary** - Complete system health and incident analysis
- **Capacity Planning** - Growth projections and resource recommendations  
- **Performance Optimization** - Bottleneck identification and tuning suggestions
- **Anomaly Analysis** - Deep dive into unusual patterns and their impacts

**Access:** Business subscriptions get unlimited reports, free trial users get full access during trial, Community users get 10 free reports.

</details>

#### Detect Issues Automatically

<details>
<summary><strong>Anomaly Advisor</strong></summary><br/>

Revolutionary troubleshooting that finds root causes in minutes. Stop guessing what went wrong - the Anomaly Advisor instantly shows you how problems cascade across your infrastructure.

**Why this matters:** Root causes typically appear in the top 20-30 results, turning hours of investigation into minutes of discovery. See cascading effects as anomalies propagate across systems with automatic ranking of every metric by anomaly severity.

**Revolutionary approach:** Data-driven analysis with no hypotheses needed, influence tracking showing what influenced and what was influenced, works identically from 10 to 10,000 nodes, visual propagation of anomaly clusters and cascades.

</details>

<details>
<summary><strong>Machine Learning Anomaly Detection</strong></summary><br/>

The foundation of Netdata's AI capabilities. Machine learning models run locally on every Agent, continuously learning normal patterns and detecting anomalies in real-time with zero configuration required.

**Why this matters:** Automatic protection across every metric with ML analyzing all metrics continuously. Visual anomaly indicators show purple ribbons on every chart displaying anomaly rates, with ML scores saved with metrics for past analysis.

**How it works:** Local ML engine runs on every Netdata Agent with no cloud dependency, multiple models use consensus approach reducing noise and false positives by 99%, integrated storage saves anomaly scores in database with metrics, designed for production environments with minimal overhead.

</details>

### Join the Premium Experience

:::tip

**What Happens Next:**
1. **Sign up for your free trial** - No credit card required
2. **Connect your Agents** - Use the simple one-command installation
3. **Experience real-time observability** - 1-second granularity across all metrics
4. **Try AI-powered features** - Chat with your infrastructure and generate insights
5. **Build unlimited dashboards** - Create the monitoring views you need
6. **Set up team access** - Invite colleagues and configure permissions

**Transform your infrastructure monitoring today. Your future self and your team will thank you.**

**[Start Free Business Trial](https://netdata.cloud/pricing)**

:::
