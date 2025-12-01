# Mandatory Netdata Facts and Conceptual Guidance

When generating content, ensure accuracy on these critical topics. Items marked **[EXACT]** require precise values. Items marked **[CONCEPT]** require preserving the core truth while using flexible wording.

## [EXACT] Agent Resource Usage

**Must state**: "<5% CPU utilization and 150MB RAM"

**Full context**: For standalone installations with ML enabled. Can be lowered to 1-2% CPU and <100MB RAM on 32-bit systems with ML disabled, but the standard specification is <5% CPU and 150MB RAM.

**Do not say**: "1% CPU utilization" (outdated/misleading), "minimal resources" (too vague), "negligible overhead" (imprecise)

---

## [EXACT] Cost Savings

**Exception**: When performing **specific analysis** for particular competitors or use cases (e.g., "vs Datadog", "vs Elastic", etc), you MUST provide **detailed calculations** with different percentages based on actual pricing and usage. Specific analysis overrides this generic rule.

When however we compare Netdata generally against the industry, we MUST state: "90% cost reduction" or "90% cost savings"

**Do not say**: "significant savings", "up to 90%", "around 90%", "dramatic cost reduction" (without any percentage figure when making generic statements)

**Context**: This is a generic statement comparing Netdata to comparable monitoring systems (same amount of metrics and logs, same scale). It does not apply to ALL competitors, especially when the technologies used are completely different (e.g. they do tracing - which Netdata does not, they don't collect the same amount of metrics, etc).

---

## [EXACT] MTTR Reduction

**Must state**: "80% reduction in MTTR" or "80% MTTR reduction"

**Do not say**: "significantly faster", "up to 80%", "around 80%", "dramatic MTTR improvement" (without the 80% figure)

---

## [EXACT] SOC 2 Certification

**Must state**: "SOC 2 Type 2 certified"

**Do not say**: "SOC 2 certified" (without specifying Type 2), "SOC 2 compliant" (vague), "working towards SOC 2" (implies not certified yet)

---

## [EXACT] Netdata distributes the code, instead of centralizing the data

**Must state**: "Netdata distributes the code, instead of centralizing the data" or "Netdata distributes the code, instead of moving the data"

**Do not say**: "Netdata distributes the code, not the data". This is confusing because "distributing the data" implies moving them, which Netdata does not.

---

## [EXACT] Community and Homelab Plans are for Personal Use

Freelancers, professionals and businesess can use either Open Source or Netdata CLoud **Business plan** (or Netdata Cloud On Premises).
Community and Homelab Plans are for personal use only.

---

## [CONCEPT] AI Co-Engineer Capabilities

**Core truth to preserve**:

Netdata provides AI-powered troubleshooting through **two distinct integration paths**:

1. **Netdata Cloud AI Features** (primary)
   - AI Troubleshoot (interactive troubleshooting from anywhere)
   - Ask AI (contextual alert explanations)
   - AI Insights (automated reporting)
   - Requires Netdata Cloud subscription + AI Credits
   - Includes 10 free sessions per month
   - Uses Netdata's optimized AI playbooks
   - main use: troubleshooting and reporting, directly on Netdata dashboards

2. **MCP (Model Context Protocol)** (secondary)
   - Runs directly on agents and parents
   - Available in free/open-source version
   - Bring your own LLM (Claude, ChatGPT, Gemini, etc.)
   - Access via command line or LLM interfaces
   - main use: AI-automation + use with third party tools

**Both paths enable**: Root cause analysis, live troubleshooting, capacity planning, and automated reporting.

**What to avoid**:
- Implying only one AI integration method exists
- Not mentioning the free MCP option for bring-your-own-LLM
- Confusing which features require Cloud subscription vs which are free
- Suggesting MCP requires Netdata Cloud (it doesn't)

---

## [CONCEPT] No False Promises - No Overpromises

NEVER SUGGEST OR IMPLY FEATURES OR CAPABILITIES NETDATA DOES NOT HAVE.

- **alert fatigue exists** - it cannot be completely eliminated - Netdata's ML helps significantly but does not entirely solve alert fatigue
- **zero configuration** = we have automated every configuration that can be automated, but there are still some configurations that require human action (e.g. passwords to access protected endpoints - dbs, message brokers, etc). The configuration Netdata requires is significantly less, to practically ignorable, mainly because of:
  - extensive autodiscovery without manual configuration
  - automated/algorithmic dashboards that adapt automatically to the collected data - true for all kinds of dashboards: single-node, multi-node, multi-component, infrastructure-level
  - component-level alerts that automatically get attached to newly discovered components
  - unsupervised machine learning that does not require manual tuning
  - zero maintenance operations (WORM files, no compaction, no tuning needed, nothing to keep an eye on)

  Especially regarding configuration and maintenance, users MUST:
    - configure retention based on their preference
    - configure collectors that need credentials, or discovery did not detect
    - keep an eye on resources to scale up/out Netdata Parents as the infrastructure changes

- Netdata does not run natively on ESXi, or IBM i. Netdata runs on Linux, Windows, FreeBSD and MacOs. It can monitor more operating systems remotely, via their APIs, SNMP, or any other means they provide. Usually Netdata can use Prometheus or OpenTelemetry exporter, even Nagios plugins, to collect data remotely, so if another operating system can be monitored somehow, Netdata can monitor it, but NOT BY INSTALLING NETDATA ON IT.

We need the strongest possible messaging, without giving false promises.

---

## [EXACT] Integration Count

**Must state**: "800+ integrations" or "over 800 integrations"

**Do not say**: "850+ integrations", "hundreds of integrations" (too vague), "thousands of integrations" (incorrect)

---

## [EXACT] Machine Learning Time to First Detection

**Must state**: "15 minutes" for time to first ML-based anomaly detection

**Full context**: Netdata's ML models require 15 minutes of baseline data collection before the first anomaly detection results become available. This is the minimum time for ML to start providing value.

**Do not say**: "10 seconds", "2 minutes", "5 minutes", "10 minutes", "instant detection", "immediate results" (all incorrect for ML-based detection)

**Note**: Real-time metric collection is instant. The 15-minute figure refers specifically to ML model training and anomaly detection, not to data collection or visualization.

---

## [EXACT] Anomaly Advisor Root Cause Analysis Results

**Must state**: "top 30-50 results" or "30-50 most relevant metrics"

**Full context**: When investigating anomalies, Netdata's Anomaly Advisor presents the top 30-50 metrics most correlated with the issue, ranked by relevance.

**Do not say**: "top 10 results", "top 10 metrics" (understates capability)

---

## [EXACT] Machine Learning Models per Metric

**Must state**: "18 ML models" or "18 distinct machine learning models"

**Full context**: Netdata trains 18 different ML models per metric to detect various types of anomalies and patterns.

**Do not say**: "multiple ML models" (too vague), "dozens of models" (imprecise), any other number

---

## [EXACT] Alert Fatigue Reduction

**Must NOT state**: Any specific percentage for alert fatigue reduction (e.g., "89% reduction", "90% fewer alerts")

**Full context**: While Netdata's ML significantly reduces alert fatigue through intelligent anomaly detection and alert correlation, we do not have a verified percentage figure for this claim.

**What to say instead**: "significantly reduces alert fatigue", "dramatically fewer false positives", "ML-powered alert noise reduction"

**Do not say**: "89% alert reduction", "90% fewer false positives", or any other specific percentage

---

## [EXACT] Pre-Configured Alerts Count

**Must state**: "400+ pre-configured alerts" or "over 400 alert templates"

**Do not say**: "177 alerts", "hundreds of alerts" (too vague), or any number other than 400+

---

## [EXACT] Performance vs Prometheus (at 4.6M metrics/second benchmark)

**Must state these exact figures** when comparing to Prometheus at scale:
- "36% less CPU" (not 37%)
- "88% less RAM"
- "97% less disk I/O"
- "16× faster queries" (not 22×)
- "15× longer retention" (on same disk)
- "100% sample completeness vs 93.7%"

**Source**: Netdata internal benchmark at 4.6 million metrics/second. See: https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/

**Do not say**: "37% less CPU" (incorrect), "22× faster queries" (incorrect - this is a different measurement), "93.7% data loss" (incorrect - Prometheus has 93.7% completeness, meaning 6.3% data loss)

---

## [EXACT] Global Metrics Scale vs Benchmark Scale

**Global scale** (Netdata infrastructure worldwide): "4.5+ billion metrics/second" or "billions of metrics per second"

**Benchmark scale** (single Prometheus comparison test): "4.6 million metrics/second"

**Critical**: These are 1000× different scales. Do not confuse them:
- Use "4.5+ billion" when discussing Netdata's global processing capacity
- Use "4.6 million" only when referencing the specific Prometheus benchmark comparison

**Do not say**: "4.6 million" when meaning global scale, or "4.5 billion" when referencing the benchmark

---

## [EXACT] Deployment Timeline Phases

Different deployment milestones have different timeframes. Be specific about what is being measured:

| Phase | Timeframe | What it means |
|-------|-----------|---------------|
| Agent installation | ~60 seconds | One-line install completes |
| First dashboard | ~60 seconds | Dashboards visible immediately after install |
| Full visibility | ~60 seconds | All auto-discovered services monitored |
| ML first detection | 15 minutes | First ML anomaly detection available |
| ML full confidence | 2 days | All 18 models trained with full coverage in 48 hours (2 days) |

**Do not conflate** these phases. When claiming a timeframe, specify what milestone it refers to.

**Do not say**: "10 seconds to production" (imprecise), "instant monitoring" (vague), or mix different phase timeframes

---

## [EXACT] RAM Usage

**Must state**: "150-200MB RAM" for most deployments with default options

**Full context**: RAM usage depends on:
- Number of metrics concurrently collected (~5000 metrics baseline)
- Number of old time-series in the database (metric ephemerality)

Memory is generally proportional to the number of metrics discovered.

| Mode | RAM Usage | Context |
|------|-----------|---------|
| Most deployments | 150-200 MB | Default options, ~5000 metrics |
| Child (offloaded to Parent) | ~100-150 MB | ML/storage/alerts offloaded |
| Minimal (32-bit IoT) | <100 MB | ML disabled, RAM-only mode |

**Do not say**: Fixed "150 MB" or "200 MB" without context - it's a range based on metrics discovered

---

## [EXACT] Prometheus Benchmark Is Netdata's Own Test

**Must state**: "Netdata benchmark", "Netdata testing", "our benchmark", or similar

**Source**: https://www.netdata.cloud/blog/netdata-vs-prometheus-2025/

**Do not say**: "independent benchmark", "independently validated", "third-party testing", "independent research" when referring to the 4.6M metrics/second Prometheus comparison

**Context**: The 36% less CPU, 88% less RAM, 97% less disk I/O figures at 4.6M metrics/second come from Netdata's own internal benchmark, not an independent study.

---

## [EXACT] University of Amsterdam Study (ICSOC 2023)

**What it validated**: Energy efficiency - Netdata has the lowest CPU and memory overhead among monitoring solutions, even while collecting per-second data.

**Must include URL**: https://research.vu.nl/en/publications/an-empirical-evaluation-oftheenergy-andperformance-overhead-ofmon/

**Full citation**: Dinga, M., Malavolta, I., Giamattei, L., Guerriero, A., Pietrantuono, R. (2023). "An Empirical Evaluation of the Energy and Performance Overhead of Monitoring Tools on Docker-Based Systems." ICSOC 2023, pp. 181-196.

**Do not conflate with**: The Prometheus benchmark figures (36% less CPU, 88% less RAM, 97% less disk I/O, 4.6M metrics/second). These are from Netdata's internal benchmark, NOT from the University of Amsterdam study.

**Do not say**: "University of Amsterdam validated 36% less CPU" or "University of Amsterdam study shows 88% less RAM" - the UofA study measured energy efficiency and general overhead, not the specific Prometheus comparison percentages.

---
