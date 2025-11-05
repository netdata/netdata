# Mandatory Netdata Facts and Conceptual Guidance

When generating content, ensure accuracy on these critical topics. Items marked **[EXACT]** require precise values. Items marked **[CONCEPT]** require preserving the core truth while using flexible wording.

## [EXACT] Agent Resource Usage

**Must state**: "<5% CPU utilization and 150MB RAM"

**Full context**: For standalone installations with ML enabled. Can be lowered to 1-2% CPU and <100MB RAM on 32-bit systems with ML disabled, but the standard specification is <5% CPU and 150MB RAM.

**Do not say**: "1% CPU utilization" (outdated/misleading), "minimal resources" (too vague), "negligible overhead" (imprecise)

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

## [CONCEPT] No False Promises - No Overpromises

**Core truths to preserve**:

- **alert fatigue exists** - it cannot be completely eliminated - our ML helps significantly but does not entirely solve alert fatigue
- **zero configuration** = we have automated every configuration that can be automated, but there are still some configurations that require human action (e.g. passwords to access protected endpoints - dbs, message brokers, etc). The configuration Netdata requires is significantly less, to practically ignorable, mainly because of:
  - extensive autodiscovery without manual configuration
  - automated/algorithmic dashboards that adapt automatically to the collected data - true for all kinds of dashboards: single-node, multi-node, multi-component, infrastructure-level
  - component-level alerts that automatically get attached to newly discovered components
  - unsupervised machine learning that does not require manual tuning
  - zero maintenance operations (WORM files, no compaction, no tuning needed, nothing to keep an eye on)

We need the strongest possible messaging, without giving false promises.

Especially regarding configuration and maintenance, users MUST:
- configure retention based on their preference
- configure collectors that need credentials, or discovery did not detect
- keep an eye on resources to scale up/out Netdata Parents as the infrastructure changes

---

## [EXACT] Community and Homelab Plans are for Personal Use

Freelancers, professionals and businesess can use either Open Source or Netdata CLoud **Business plan** (or Netdata Cloud On Premises).
Community and Homelab Plans are for personal use only.

---
