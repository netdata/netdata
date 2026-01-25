# Internal Guide: Alerts & Notifications Structure (DO NOT PUBLISH)

**For:** Fotis
**Purpose:** Understand the restructured alerts documentation layout

---

## Why This Restructure?

Docs previously lived under nested `/Alerts & Notifications/The Book/` path which didn't surface well on learn.netdata.cloud. Flattened to `/Alerts & Notifications/` for better discoverability.

---

## Structure Overview

```
docs/alerts/
├── understanding-alerts/           # Chapter 1 - Fundamentals
│   ├── README.md                  # Landing page (uses README.md not index.md!)
│   ├── 1-what-is-a-netdata-alert.md
│   ├── 2-alert-types-alarm-vs-template.md
│   └── 3-where-alerts-live.md
├── creating-alerts-pages/          # Chapter 2 - Getting Started
│   ├── README.md
│   ├── 1-quick-start-create-your-first-alert.md
│   ├── 2-creating-and-editing-alerts-via-config-files.md
│   ├── 3-creating-and-editing-alerts-via-cloud.md
│   ├── 4-managing-stock-vs-custom-alerts.md
│   └── 5-reloading-and-validating-alert-configuration.md
├── alert-configuration-syntax/    # Chapter 3 - Deep Dive
│   ├── README.md
│   ├── 1-alert-definition-lines.md
│   ├── 2-lookup-and-time-windows.md
│   ├── 3-calculations-and-transformations.md
│   ├── 4-expressions-operators-functions.md
│   ├── 5-variables-and-special-symbols.md
│   └── 6-optional-metadata.md
├── controlling-alerts-noise/      # Chapter 4
│   ├── README.md
│   ├── 1-disabling-alerts.md
│   ├── 2-silencing-vs-disabling.md
│   ├── 3-silencing-cloud.md
│   └── 4-reducing-flapping.md
├── receiving-notifications/       # Chapter 5
│   ├── README.md
│   ├── 1-notification-concepts.md
│   ├── 2-agent-parent-notifications.md
│   ├── 3-cloud-notifications.md
│   ├── 4-controlling-recipients.md
│   └── 5-testing-troubleshooting.md
├── alert-examples/                # Chapter 6
│   ├── README.md
│   └── *.md files
├── troubleshooting-alerts/        # Chapter 7
│   ├── README.md
│   └── *.md files
├── essential-patterns/           # Chapter 8
│   ├── README.md
│   └── *.md files
├── apis-alerts-events/           # Chapter 9
│   ├── README.md
│   └── *.md files
├── cloud-alert-features/         # Chapter 10
│   ├── README.md
│   └── *.md files
├── stock-alerts/                # Chapter 11
│   ├── README.md
│   └── *.md files
├── best-practices/              # Chapter 12
│   ├── README.md
│   ├── 1-designing-useful-alerts.md
│   ├── 2-notification-strategy.md
│   ├── 3-maintaining-configurations.md
│   ├── 4-scaling-large-environments.md
│   └── 5-sli-slo-alerts.md
└── architecture/                # Chapter 13
    ├── README.md
    ├── 1-evaluation-architecture.md
    ├── 2-alert-lifecycle.md
    ├── 3-notification-dispatch.md
    ├── 4-configuration-layers.md
    └── 5-scaling-topologies.md
```

---

## File Naming Conventions

| Category | Pattern | Example |
|-----------|---------|---------|
| Landing page | `README.md` | `understanding-alerts/README.md` |
| Topic | `N-topic-name.md` | `1-what-is-a-netdata-alert.md` |
| Numbering | Sequential starting at 1 | 1, 2, 3, 4, 5... |

**IMPORTANT:** This branch uses `README.md` for landing pages, NOT `index.md`.

---

## map.csv Entry Format

```
https://github.com/netdata/netdata/edit/master/docs/alerts/{folder}/{filename}.md,Sidebar Label,Published,Alerts & Notifications/{Folder Name},
```

**Critical:** Never use `Alerts & Notifications/The Book/...` - that's been flattened.

Example:
```
https://github.com/netdata/netdata/edit/master/docs/alerts/understanding-alerts/1-what-is-a-netdata-alert.md,What is a Netdata Alert,Published,Alerts & Notifications/Understanding Alerts,
```

---

## Landing Page Template (README.md)

```markdown
# Chapter Title

One-paragraph description of what this chapter covers.

## Sections

- [**N. Section Title**](section-link.md) - Description
- [**N. Section Title**](section-link.md) - Description

## Related

- [Chapter X](../chapter-folder/README.md)
```

---

## Cross-Reference Pattern

Link to other chapters like this:

```markdown
- **[Chapter X: Title]((../chapter-folder/README.md))** for topic
- **[Section X.N](/docs/alerts/folder/n-section-title.md)** for specifics
```

---

## Key Differences From Previous Structure

| Old Pattern | New Pattern |
|-------------|-------------|
| `/The Book/Chapter/` | `/Chapter/` (flattened) |
| `index.md` landing | `README.md` landing |
| Descriptive names | Numerical prefix `N-` |
| Mixed ordering | Strict sequential numbering |

---

## Chapters With Sub-sections Numbered

| Chapter | Subsection Numbers |
|---------|-------------------|
| 3. Alert Configuration Syntax | 1-6 |
| 4. Controlling Alerts and Noise | 1-4 |
| 5. Receiving Notifications | 1-5 |
| 12. Best Practices | 1-5 |
| 13. Architecture | 1-5 |

---

## Testing the Structure

After making changes:
1. Run docs builder locally to catch broken links
2. Check `git diff docs/.map/map.csv` - should only show "The Book/" → "" change
3. Verify numerically-prefixed files sort correctly in file explorer

---

## Questions?

Check `src/health/*.c` for source of truth on alert terminology.