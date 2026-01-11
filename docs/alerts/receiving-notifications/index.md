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
| **5.1 Notification Concepts** | The three dispatch models and when to use each |
| **5.2 Agent and Parent Notifications** | Configuring local notification methods (email, Slack, PagerDuty, etc.) |
| **5.3 Cloud Notifications** | Setting up Cloud-integrated notifications with roles and routing |
| **5.4 Controlling Recipients** | Mapping severities to people, using Cloud roles |
| **5.5 Testing and Troubleshooting** | Verifying notifications work, common issues |

## How to Navigate This Chapter

- Start at **5.1** to understand the three dispatch models
- Jump to **5.2** if you configure notifications on Agents/Parents
- Use **5.3** for Cloud-dispatched notifications
- Go to **5.5** if notifications aren't arriving

## What's Next

- **5.1 Notification Concepts** explains where notifications originate
- **5.2 Agent and Parent Notifications** covers local method configuration
- **5.3 Cloud Notifications** for Cloud-integrated routing
- **6.1 Core System Alerts** for example alert definitions