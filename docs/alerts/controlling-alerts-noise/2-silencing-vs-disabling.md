# 4.2 Silencing Versus Disabling Alerts

This distinction is critical—confusing them is one of the most common sources of alert configuration mistakes.

| Aspect | Disabling | Silencing |
|--------|-----------|-----------|
| **What stops** | Alert **evaluation** (never runs) | **Notifications** only (evaluation still runs) |
| **Alert events generated?** | **No** | **Yes**, but suppressed |
| **Alert visible in UI?** | **No** (not loaded) | **Yes**, with its current status |
| **Use case** | Permanent removal | Temporary quiet periods |

## 4.2.1 How Silencing Works

When you silence an alert:
1. The alert **continues to evaluate** normally
2. Status transitions still happen (CLEAR → WARNING → CRITICAL)
3. **No notifications are sent** to recipients
4. The alert **appears in the UI** with its actual status
5. Events are **still recorded** in the log

Silencing is like putting a "Do Not Disturb" sign on an alert—it keeps working, but nobody gets notified.

## 4.2.2 When to Use Each

| Scenario | Approach |
|----------|----------|
| Alert monitors a service you don't run | **Disable** (never evaluate) |
| Alert is replaced by a custom version | **Disable** (permanent) |
| Nighttime maintenance window (1-4 AM) | **Silence** (temporary) |
| On-call person is on vacation | **Silence** (personal override) |
| Alert fires frequently during peak hours | **Silence** + review thresholds |

## 4.2.3 Related Sections

- **[4.1 Disabling Alerts](1-disabling-alerts.md)** - How to permanently stop alert evaluation
- **[4.3 Silencing in Netdata Cloud](3-silencing-cloud.md)** - Cloud-managed silencing rules
- **[4.4 Reducing Flapping and Noise](4-reducing-flapping.md)** - Using delays to prevent flapping

## What's Next

- **[4.3 Silencing in Netdata Cloud](3-silencing-cloud.md)** - Cloud-based silencing rules
- **[4.4 Reducing Flapping](4-reducing-flapping.md)** - Delay and repeat techniques
- **[8.1 Hysteresis](../advanced-techniques/1-hysteresis.md)** - Status-based conditions