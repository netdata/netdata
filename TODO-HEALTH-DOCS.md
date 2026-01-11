# Health Documentation Review - PR #21333

**PR**: https://github.com/netdata/netdata/pull/21333
**Branch**: alerts-doc-rewrite
**Author**: Kanela
**Reviewer**: Costa + Claude

---

## TL;DR

We are reviewing PR #21333, a comprehensive rewrite of Netdata's alerting documentation (74 files). Our goal is to ensure every page is technically accurate, complete, and useful for users.

---


## What We Are Doing

1. **Reviewing a user documentation PR** - This is a major rewrite of the alerting documentation
2. **Listing all files** with their purpose (what users will learn from each page)
3. **Scoring each page** against 5 metrics (max 50 points, 90% threshold = 45 points)
4. **Identifying issues** in claims, configuration syntax, API endpoints, and overall effectiveness

---


## Review Goals

| Goal | Description |
|------|-------------|
| **G1** | Identify any claims about Netdata that are incomplete or wrong |
| **G2** | Identify any configuration instructions that are incomplete or wrong |
| **G3** | Identify any API endpoints or related instructions that are incomplete or wrong |
| **G4** | Confirm each page achieves its stated purpose |
| **G5** | Score each page and flag those below 90% threshold |

---


## Scoring Metrics

Each page is scored on 5 metrics (0-10 each, max 50 points):

| Metric | Score Range | What It Measures |
|--------|-------------|------------------|
| **Technical Accuracy** | 0-10 | All claims, syntax, API endpoints, and behaviors match actual implementation |
| **Completeness** | 0-10 | Covers all relevant aspects, no critical gaps, includes prerequisites |
| **Clarity** | 0-10 | Easy to understand, proper structure, logical flow, good examples |
| **Practical Value** | 0-10 | Working examples, addresses real use cases, helps users achieve goals |
| **Maintainability** | 0-10 | Uses stable patterns, won't break with minor updates, avoids brittle details |

**Threshold**: Pages scoring below **45/50 (90%)** require improvement.

**Priority Levels**:
- **HIGH**: Core functionality users need daily
- **MEDIUM**: Important but less frequently used
- **LOW**: Reference/advanced topics

---


## Files to Review (74 total)

### Chapter 1: Understanding Alerts (4 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `understanding-alerts/understanding-alerts.md` | Introduces what alerts are and where they run in Netdata | HIGH | 45/50 | DONE |
| `understanding-alerts/what-is-a-netdata-alert.md` | Explains alert status values and how alerts relate to charts/contexts | HIGH | 40/50 | DONE |
| `understanding-alerts/alert-types-alarm-vs-template.md` | Explains the difference between `alarm` (single chart) and `template` (multiple charts) | HIGH | 46/50 | DONE |
| `understanding-alerts/where-alerts-live.md` | Shows where alert config files are stored on disk | HIGH | 45/50 | DONE |

### Chapter 2: Creating and Managing Alerts (7 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `creating-alerts-pages/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `creating-alerts-pages/creating-alerts.md` | General introduction to creating alerts | MEDIUM | --/50 | PENDING |
| `creating-alerts-pages/quick-start-create-your-first-alert.md` | Step-by-step tutorial for a simple alert | HIGH | --/50 | PENDING |
| `creating-alerts-pages/creating-and-editing-alerts-via-config-files.md` | How to edit health.d files and use edit-config | HIGH | --/50 | PENDING |
| **`creating-alerts-pages/creating-and-editing-alerts-via-cloud.md`** | How to use Cloud's Alerts Configuration Manager | HIGH | **35/50** | **DONE** |
| `creating-alerts-pages/managing-stock-vs-custom-alerts.md` | How to safely override stock alerts without losing them | HIGH | --/50 | PENDING |
| `creating-alerts-pages/reloading-and-validating-alert-configuration.md` | How to reload configs and verify they loaded correctly | HIGH | --/50 | PENDING |

### Chapter 3: Alert Configuration Syntax (8 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `alert-configuration-syntax/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `alert-configuration-syntax/alert-configuration-syntax.md` | Overview of alert syntax structure | MEDIUM | --/50 | PENDING |
| `alert-configuration-syntax/alert-definition-lines.md` | Reference for all config lines (alarm, on, lookup, warn, crit, etc.) | HIGH | --/50 | PENDING |
| `alert-configuration-syntax/lookup-and-time-windows.md` | How to use lookup with aggregation functions and time windows | HIGH | --/50 | PENDING |
| `alert-configuration-syntax/calculations-and-transformations.md` | How to use calc expressions to transform lookup results | HIGH | --/50 | PENDING |
| `alert-configuration-syntax/expressions-operators-functions.md` | Reference for operators (+, -, *, /, AND, OR) and functions | HIGH | --/50 | PENDING |
| `alert-configuration-syntax/variables-and-special-symbols.md` | Reference for $this, $status, $now, dimension variables | HIGH | --/50 | PENDING |
| `alert-configuration-syntax/optional-metadata.md` | How to use class, type, component, summary, info lines | MEDIUM | --/50 | PENDING |

### Chapter 4: Controlling Alert Noise (5 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `controlling-alerts-noise/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `controlling-alerts-noise/silencing-vs-disabling.md` | Explains the difference between silencing and disabling alerts | HIGH | --/50 | PENDING |
| `controlling-alerts-noise/silencing-cloud.md` | How to silence alerts via Netdata Cloud | MEDIUM | --/50 | PENDING |
| `controlling-alerts-noise/disabling-alerts.md` | How to disable alerts via config or API | HIGH | --/50 | PENDING |
| `controlling-alerts-noise/reducing-flapping.md` | How to use delay and hysteresis to reduce flapping | HIGH | --/50 | PENDING |

### Chapter 5: Receiving Notifications (6 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `receiving-notifications/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `receiving-notifications/notification-concepts.md` | Explains notification flow and concepts | MEDIUM | --/50 | PENDING |
| `receiving-notifications/agent-parent-notifications.md` | How to configure Agent/Parent notifications (exec, to, alarm-notify.sh) | HIGH | --/50 | PENDING |
| `receiving-notifications/cloud-notifications.md` | How Cloud notifications work and how to configure them | HIGH | --/50 | PENDING |
| `receiving-notifications/controlling-recipients.md` | How to configure notification recipients and roles | MEDIUM | --/50 | PENDING |
| `receiving-notifications/testing-troubleshooting.md` | How to test notifications and troubleshoot issues | HIGH | --/50 | PENDING |

### Chapter 6: Troubleshooting Alerts (6 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `troubleshooting-alerts/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `troubleshooting-alerts/alert-never-triggers.md` | How to debug alerts that never fire | HIGH | --/50 | PENDING |
| `troubleshooting-alerts/always-critical.md` | How to debug alerts stuck in CRITICAL | HIGH | --/50 | PENDING |
| `troubleshooting-alerts/flapping.md` | How to fix alerts that flip between states rapidly | HIGH | --/50 | PENDING |
| `troubleshooting-alerts/notifications-not-sent.md` | How to debug missing notifications | HIGH | --/50 | PENDING |
| `troubleshooting-alerts/variables-not-found.md` | How to debug variable resolution issues | MEDIUM | --/50 | PENDING |

### Chapter 7: Advanced Techniques (6 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `advanced-techniques/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `advanced-techniques/hysteresis.md` | How to implement hysteresis in warn/crit expressions | MEDIUM | --/50 | PENDING |
| `advanced-techniques/label-targeting.md` | How to target alerts to specific hosts/charts using labels | MEDIUM | --/50 | PENDING |
| `advanced-techniques/multi-dimensional.md` | How to create alerts across multiple dimensions | MEDIUM | --/50 | PENDING |
| `advanced-techniques/custom-actions.md` | How to create custom exec scripts for alerts | MEDIUM | --/50 | PENDING |
| `advanced-techniques/performance.md` | How to optimize alert performance at scale | LOW | --/50 | PENDING |

### Chapter 8: Alert Examples (6 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `alert-examples/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `alert-examples/core-system-alerts.md` | Example alerts for CPU, memory, disk | MEDIUM | --/50 | PENDING |
| `alert-examples/application-alerts.md` | Example alerts for databases, web servers | MEDIUM | --/50 | PENDING |
| `alert-examples/service-availability.md` | Example alerts for service health checks | MEDIUM | --/50 | PENDING |
| `alert-examples/anomaly-alerts.md` | How to create alerts using ML anomaly detection | MEDIUM | --/50 | PENDING |
| `alert-examples/trend-capacity.md` | How to create alerts for trend/capacity forecasting | LOW | --/50 | PENDING |

### Chapter 9: Built-in Alerts Reference (5 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `built-in-alerts/system-resource-alerts.md` | Reference for stock CPU, RAM, disk, network alerts | HIGH | --/50 | PENDING |
| `built-in-alerts/application-alerts.md` | Reference for stock MySQL, PostgreSQL, Redis, etc. alerts | MEDIUM | --/50 | PENDING |
| `built-in-alerts/container-alerts.md` | Reference for stock Docker, Kubernetes alerts | MEDIUM | --/50 | PENDING |
| `built-in-alerts/hardware-alerts.md` | Reference for stock RAID, UPS, IPMI alerts | MEDIUM | --/50 | PENDING |
| `built-in-alerts/network-alerts.md` | Reference for stock DNS, HTTP, SSL alerts | MEDIUM | --/50 | PENDING |

### Chapter 10: APIs for Alerts and Events (6 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `apis-alerts-events/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `apis-alerts-events/query-alerts.md` | How to query current alert status via API | HIGH | --/50 | PENDING |
| `apis-alerts-events/inspect-variables.md` | How to inspect alert variables via API | MEDIUM | --/50 | PENDING |
| `apis-alerts-events/health-management.md` | How to use the Health Management API (DISABLE, SILENCE, RESET) | HIGH | --/50 | PENDING |
| `apis-alerts-events/alert-history.md` | How to query alert history via API | MEDIUM | --/50 | PENDING |
| `apis-alerts-events/cloud-events.md` | How to access alert events via Cloud API | MEDIUM | --/50 | PENDING |

### Chapter 11: Cloud Alert Features (5 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `cloud-alert-features/index.md` | Chapter overview and navigation guide | LOW | --/50 | PENDING |
| `cloud-alert-features/events-feed.md` | How to use the Cloud Events Feed | MEDIUM | --/50 | PENDING |
| `cloud-alert-features/silencing-rules.md` | How to create silencing rules in Cloud | MEDIUM | --/50 | PENDING |
| `cloud-alert-features/deduplication.md` | How Cloud deduplicates alerts from multiple agents | MEDIUM | --/50 | PENDING |
| `cloud-alert-features/room-based.md` | How room-based alert views work in Cloud | LOW | --/50 | PENDING |

### Chapter 12: Best Practices (5 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `best-practices/designing-useful-alerts.md` | Guidelines for creating actionable alerts | MEDIUM | --/50 | PENDING |
| `best-practices/notification-strategy.md` | How to design a notification strategy | MEDIUM | --/50 | PENDING |
| `best-practices/maintaining-configurations.md` | How to maintain alert configs over time | LOW | --/50 | PENDING |
| `best-practices/scaling-large-environments.md` | Best practices for alerts at scale | LOW | --/50 | PENDING |
| `best-practices/sli-slo-alerts.md` | How to create SLI/SLO-based alerts | LOW | --/50 | PENDING |

### Chapter 13: Architecture (5 files)

| File | Purpose | Priority | Score | Status |
|------|---------|----------|-------|--------|
| `architecture/alert-lifecycle.md` | Explains alert state transitions (UNINITIALIZED -> CLEAR -> WARNING -> CRITICAL) | MEDIUM | --/50 | PENDING |
| `architecture/evaluation-architecture.md` | Explains how and when alerts are evaluated | MEDIUM | --/50 | PENDING |
| `architecture/configuration-layers.md` | Explains stock vs custom vs Cloud config precedence | MEDIUM | --/50 | PENDING |
| `architecture/notification-dispatch.md` | Explains how notifications are dispatched | LOW | --/50 | PENDING |
| `architecture/scaling-topologies.md` | Explains alert behavior in Parent/Child setups | LOW | --/50 | PENDING |

---


## Review Progress

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | List all files with purposes | DONE |
| 2 | Define scoring metrics | DONE |
| 3 | Review Chapter 1 (Understanding) | DONE |
| 4 | Review Chapter 2 (Creating) - **creating-and-editing-alerts-via-cloud.md** | DONE |
| 5 | Review Chapter 3 (Syntax Reference) | PENDING |
| 6 | Review Chapter 4 (Controlling Noise) | PENDING |
| 7 | Review Chapter 5 (Notifications) | PENDING |
| 8 | Review Chapter 6 (Troubleshooting) | PENDING |
| 9 | Review Chapter 7 (Advanced) | PENDING |
| 10 | Review Chapter 8 (Examples) | PENDING |
| 11 | Review Chapter 9 (Built-in Alerts) | PENDING |
| 12 | Review Chapter 10 (APIs) | PENDING |
| 13 | Review Chapter 11 (Cloud Features) | PENDING |
| 14 | Review Chapter 12 (Best Practices) | PENDING |
| 15 | Review Chapter 13 (Architecture) | PENDING |
| 16 | Final summary and recommendations | PENDING |

---


## Detailed Review: creating-and-editing-alerts-via-cloud.md

### File Summary
| Metric | Score |
|--------|-------|
| **Technical Accuracy** | 5/10 |
| **Completeness** | 7/10 |
| **Clarity** | 8/10 |
| **Practical Value** | 8/10 |
| **Maintainability** | 7/10 |
| **TOTAL** | **35/50** (70%) |

### Verification Results

#### 1. Cloud Push Mechanism via dyncfg ✓ VERIFIED

**Documentation Claim (Line 3-4, 99-101, 204-206)**:
> "This section shows you how to create, edit, and manage alert definitions using the Netdata Cloud UI"
> "Store the alert definition in Cloud" > "Push it to the selected node"
> "Netdata Cloud immediately pushes the updated definition to all connected nodes"

**Evidence from source code**:
- `src/health/health_dyncfg.c:554-780` - `dyncfg_health_cb()` function handles Cloud pushes
- `DYNCFG_SOURCE_TYPE_DYNCFG` is set when Cloud pushes alerts (line 565, 692)
- Alerts registered via `dyncfg_add()` with callback `dyncfg_health_cb` (lines 805-813, 838-845)
- `DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX "health:alert:prototype"` is the ID format (line 5)

**Code snippet from health_dyncfg.c:565**:
```c
nap->config.source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
bool added = health_prototype_add(nap, &msg);
```

**Conclusion**: ✅ VERIFIED - The Cloud push mechanism via dyncfg exists in the implementation.

---

#### 2. /var/lib/netdata/config Path ✓ VERIFIED

**Documentation Claim (Line 42)**:
> "They are **persisted locally** in `/var/lib/netdata/config/` for reliability"

**Evidence from source code**:
- `src/daemon/dyncfg/dyncfg.c:182-190` - Path construction:
```c
char path[PATH_MAX];
snprintfz(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, "config");

if(mkdir(path, 0755) == -1) {
    if(errno != EEXIST)
        nd_log(NDLS_DAEMON, NDLP_CRIT, "DYNCFG: failed to create dynamic configuration directory '%s'", path);
}

dyncfg_globals.dir = strdupz(path);
```

- `src/daemon/dyncfg/dyncfg-files.c:9,16` - Files saved as `{id}.dyncfg`:
```c
snprintfz(filename, sizeof(filename), "%s/%s.dyncfg", dyncfg_globals.dir, escaped_id);
```

**Conclusion**: ✅ VERIFIED - The path `/var/lib/netdata/config` is used for persistence.

---

#### 3. Detection Modes ⚠️ PARTIALLY VERIFIED - MISMATCHED TERMINOLOGY

**Documentation Claims (Lines 70, 79, 108-181)**:
| Detection Mode | Description |
|----------------|-------------|
| **Static Threshold** | Compares aggregated metric values against fixed thresholds |
| **Dynamic Baseline (ML-Based)** | Uses ML models to detect anomalies based on learned patterns |
| **Rate of Change** | Alerts when a metric changes too rapidly |

**Actual Implementation in source code**:
- `src/health/health_prototypes.c:47-57` - Only 3 data sources:
```c
static struct {
    ALERT_LOOKUP_DATA_SOURCE source;
    const char *name;
} data_sources[] = {
    { .source = ALERT_LOOKUP_DATA_SOURCE_SAMPLES, .name = "samples" },
    { .source = ALERT_LOOKUP_DATA_SOURCE_PERCENTAGES, .name = "percentages" },
    { .source = ALERT_LOOKUP_DATA_SOURCE_ANOMALIES, .name = "anomalies" },
    { .source = 0, .name = NULL },
};
```

**Terminology Mapping**:
| Documentation Term | Implementation Term | Status |
|-------------------|---------------------|--------|
| Static Threshold | `samples` | ❌ **MISMATCH** - Terms don't match |
| Dynamic Baseline (ML-Based) | `anomalies` | ⚠️ Partial match |
| Rate of Change | (none) | ❌ **NOT FOUND** in implementation |

**Evidence**: The term "Rate of Change" appears ONLY in documentation files, NOT in any source code:
```bash
$ grep -r "rate of change" --include="*.c" /home/costa/src/netdata-ktsaou.git/src/
# No matches found for "rate of change" or "Rate of Change"
```

**Conclusion**: ❌ NOT VERIFIED AS WRITTEN - Detection mode terminology in documentation does NOT match implementation:
1. "Static Threshold", "Dynamic Baseline", "Rate of Change" are NOT in source code
2. Implementation uses: `samples`, `percentages`, `anomalies`
3. "Rate of Change" mode does not exist in implementation

---

### Issues Found

| # | Issue | Severity | Location |
|---|-------|----------|----------|
| 1 | Detection mode terminology mismatch | HIGH | Lines 70, 79, 108-181 |
| 2 | "Rate of Change" mode doesn't exist in implementation | HIGH | Lines 160-181 |
| 3 | Documentation uses user-friendly terms not in code | MEDIUM | Throughout section 2.3.3 |

### Recommendations

1. **Update detection mode terminology** to match implementation:
   - Replace "Static Threshold" with "Samples" (or keep as descriptive term but note the actual value is `samples`)
   - Replace "Dynamic Baseline (ML-Based)" with "Anomalies"
   - Remove or rename "Rate of Change" - it doesn't exist in the implementation

2. **Add a glossary mapping** between user-friendly documentation terms and actual implementation values:
   ```
   UI Term              | Implementation Value
   ---------------------|---------------------
   Static Threshold     | samples (default)
   Percentage           | percentages
   Dynamic Baseline     | anomalies
   ```

3. **If Rate of Change is a desired feature**, it needs to be implemented first.

---

### Additional Notes on Cloud Push Flow

The documentation describes the Cloud push flow but doesn't mention:
- Alerts are pushed via ACLK (Agent Cloud Link)
- Configuration is stored in SQLite with `source_type = DYNCFG_SOURCE_TYPE_DYNCFG`
- The `/api/v3/config` endpoint handles dynamic configuration

See `src/database/sqlite/sqlite_aclk.c:524-533` for alert push worker:
```c
static void start_alert_push(uv_work_t *req __maybe_unused)
{
    // ...
    aclk_push_alert_events_for_all_hosts();
}
```

---

## Issues Found

### Technical Accuracy Issues

| # | File | Line | Issue | Severity | Status |
|---|------|------|-------|----------|--------|
| 1 | `creating-alerts-pages/creating-and-editing-alerts-via-cloud.md` | 70, 79, 108-181 | Detection modes use terminology not found in source code | HIGH | OPEN |
| 2 | `creating-alerts-pages/creating-and-editing-alerts-via-cloud.md` | 160-181 | "Rate of Change" mode does not exist in implementation | HIGH | OPEN |

### Completeness Issues

| # | File | Section | What's Missing | Severity | Status |
|---|------|---------|----------------|----------|--------|
| 1 | `creating-alerts-pages/creating-and-editing-alerts-via-cloud.md` | Cloud Push Flow | No mention of ACLK or `/api/v3/config` mechanism | MEDIUM | OPEN |
| 2 | `creating-alerts-pages/creating-and-editing-alerts-via-cloud.md` | Detection Modes | No mapping between UI terms and implementation values | MEDIUM | OPEN |

---

## Verification Sources

When reviewing, verify against:

| Source | Path | What to verify |
|--------|------|----------------|
| Health config parser | `src/health/health_config.c` | Keywords, syntax, defaults |
| Expression parser | `src/libnetdata/eval/eval-*.c` | Operators, functions, variables |
| Query engine | `src/web/api/queries/` | Lookup options, aggregation methods |
| Stock alerts | `src/health/health.d/*.conf` | Real alert names, thresholds, contexts |
| API definitions | `src/web/api/netdata-swagger.yaml` | Endpoints, parameters, responses |
| Notification script | `src/health/notifications/alarm-notify.sh.in` | Environment variables, parameters |
| Health REFERENCE | `src/health/REFERENCE.md` | Canonical syntax documentation |
| **DynCfg implementation** | `src/daemon/dyncfg/dyncfg.c` | Cloud push mechanism |
| **Health DynCfg** | `src/health/health_dyncfg.c` | Cloud alert callbacks |
| **Data sources enum** | `src/health/health_prototypes.c:47-57` | Actual detection modes |

---

## Notes

- Previous review found 97 unique discrepancies, mostly fixed
- Chapter 9 (Built-in Alerts) was completely rewritten with real stock alerts
- Focus on HIGH priority files first
- Each page should be independently useful (not require reading other pages for basic understanding)
- **CRITICAL**: Detection modes in creating-and-editing-alerts-via-cloud.md DO NOT match implementation