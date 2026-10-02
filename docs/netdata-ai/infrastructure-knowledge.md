# Infrastructure Knowledge

Netdata AI sees everything your infrastructure *does* — every metric, every anomaly, every alert. What it cannot see is what your infrastructure *is*: which services matter, which host is the one that is supposed to run hot, who owns what, and what your team considers normal. Without that, "CPU at 91%" is a finding. With it, it may be a machine doing exactly its job.

**Infrastructure Knowledge** is where that context lives. Think of it as onboarding a new SRE to your team: the more you tell Netdata AI about your infrastructure, the more accurate its conversations, investigations, and reports become — and the more its answers sound like they come from someone who already knows your environment.

There are two ways to teach it, and they work together:

- **Write it down.** Your Space has a shared document that describes your infrastructure. Your team edits it directly.
- **Tell it in a conversation.** As you work with Netdata AI, you can ask it to remember things, and it can pick up what you tell it along the way. It gets to know your infrastructure progressively, the way a new colleague would.

Everything you teach Netdata AI is visible under **Manage Space → Infrastructure Knowledge**, in two tabs: **Your Context** and **AI Memory**.

## Your Context

**Your Context** is a single Markdown document your team maintains. It is read at the start of every conversation, investigation, and report about your infrastructure.

Describe the things that only your team knows. The editor offers section templates to get you started:

- **Infrastructure Overview** — what you run and where, in a few paragraphs.
- **Service Tiers** — which services are critical, which are best-effort, what is worth waking someone up for.
- **Known Behaviours** — patterns that look like problems and are not: the nightly batch job, the ML node that always runs hot, the idle half of a blue-green deployment.
- **SLOs & Business Context** — your targets and what a breach means to the business.
- **Team Preferences** — how you want findings presented, who to mention for what.
- **Upcoming Events** — maintenance windows, migrations, expected load changes.
- **Architecture Notes** — naming conventions, dependencies, anything that would trip up a newcomer.

You do not need to fill in everything. A few honest lines about what matters and what is expected already change how Netdata AI reads your data. Skip anything Netdata can already see for itself — node lists, current values, running processes — and never put credentials or personal data in it.

Every save keeps a version, so you can review what changed and go back to an earlier version at any time.

You can also ask Netdata AI to update the document for you in a conversation — for example, *"Add the new Kafka cluster to my infrastructure context"* — and it tells you what it changed.

## AI Memory

**AI Memory** is what Netdata AI has learned from talking to your team. Each memory is a single fact about your infrastructure, shared with everyone in the Space and used in every future conversation, investigation, and report.

You build it up simply by talking:

- *"Remember that the 02:00 disk spike on backup-01 is expected."*
- *"That alert is a false positive — the temp files get cleaned hourly."*
- *"db-3 is the replica, not the primary."*
- *"Keep investigation summaries short."*

Netdata AI tells you when it records something, so you can correct it while you still have the context. Say *"don't remember that"* and it forgets. It may also offer to remember a correction you make during troubleshooting.

The **AI Memory** tab lists everything it has learned. You can read each memory, delete the ones that no longer apply, or clear them all.

## How Netdata AI uses it

Netdata AI reads your context and memories before it interprets anything, not after. Your context tells it what things *mean*; telemetry tells it what is *happening*. It uses your knowledge to interpret the data, never to override it — if your document says a node was decommissioned and the node is still reporting, it tells you the two disagree rather than picking a side.

When your context changes a conclusion — a node it did not flag because you marked it as a staging tier, a spike it called expected because you said so — it says where that came from. When it needed context you have not given it, it says that too, so you know what to add next.

## Availability

Infrastructure Knowledge is available on paid Netdata Cloud plans. Space admins, managers, and troubleshooters can edit it; observers can view it.

## See also

- [Conversations](/docs/netdata-ai/conversations.md) — where you teach Netdata AI as you work.
- [Investigations](/docs/netdata-ai/investigations/index.md) and [AI Insights](/docs/ml-ai/ai-insights.md) — reports that use your context.
- [MCP Connections](/docs/netdata-ai/mcp/mcp-connections.md) — bring in context from the tools your team already runs.
