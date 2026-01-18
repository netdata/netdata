# 5. Receiving Notifications

Now that you know **how to create alerts** (Chapter 2) and **control their behavior** (Chapter 4), this chapter shows you how to **deliver alert events** to people and systems.

Alert events are only useful if the right people see them. Netdata supports three notification dispatch models:

| Model | Where Notifications Run | Best For |
|-------|------------------------|----------|
| **Agent-Dispatched** | Each Agent evaluates and sends notifications | Air-gapped environments, local teams |
| **Parent-Dispatched** | Parents aggregate and send for children | Hierarchical deployments, reduced Cloud traffic |
| **Cloud-Dispatched** | Cloud receives events and routes notifications | Centralized management, Cloud integrations |

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **[5.1 Notification Concepts](1-notification-concepts.md)** | The three dispatch models and when to use each |
| **[5.2 Agent and Parent Notifications](2-agent-parent-notifications.md)** | Configuring local notification methods (email, Slack, PagerDuty, etc.) |
| **[5.3 Cloud Notifications](3-cloud-notifications.md)** | Setting up Cloud-integrated notifications with roles and routing |
| **[5.4 Controlling Recipients](4-controlling-recipients.md)** | Mapping severities to people, using Cloud roles |
| **[5.5 Testing and Troubleshooting](5-testing-troubleshooting.md)** | Verifying notifications work, common issues |