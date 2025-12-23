# TODO: BigQuery Domain Model Documentation

## TL;DR
Add foundational domain model documentation to `neda/bigquery.ai` so an LLM can understand Netdata's entity model, relationships, and business logic before processing queries.

## Analysis

### Current State
- `neda/bigquery.ai` is ~3,400 lines of SQL templates, query patterns, and metrics
- Assumes reader already understands Netdata's domain model
- No explanation of core entities: user, space, node, subscription, trial, plan

### Problem
An LLM without domain context will:
- Guess at entity relationships
- Misinterpret user queries
- Make incorrect JOIN decisions
- Misunderstand metrics and their business meaning

### Solution
Add a "Domain Model" section near the top of the file (~500-800 words) covering:
1. Entity model and relationships
2. Subscription state machine
3. Revenue model (ARR/MRR, realized vs unrealized)
4. Key table mappings

## Domain Knowledge (from user Q&A)

### Users and Spaces

- **Users are independent of spaces** - a person signs in once and can access multiple spaces
- **Each space can have a different role** for the same user (admin, observer, troubleshooter, etc.)
- **Spaces = complete infrastructure isolation** - shared users, but totally isolated infrastructure
- **Use case**: A freelancer or MSP can manage many customers with the same login/session
- **Spaces can have any number of users** - Netdata does not charge for number of users
- **Spaces are owned by admin users** (role = admin) - same user can be admin in multiple spaces

**Example**: User Costa is:
- Admin at his homelab space (Community plan)
- Observer at another user's space (Business Trial)
- Troubleshooter at company space (Business plan)

All accessed with single sign-in, each space is a different billable account.

### Rooms

- **Spaces have rooms** for organizing infrastructure and people/users
- **Nodes can appear in multiple rooms** within the same space
- **Nodes are isolated to 1 space** (cannot span spaces)
- **Room use cases**: incidents, teams, services, regions, datacenters, cloud providers, different functions, etc.

### Nodes

- **Nodes are Netdata agents** running on monitored infrastructure
- **Virtual nodes**: Created by configuration on existing agents, appear as independent nodes in dashboards/DB
  - Used for: SNMP devices, cloud RDS instances, remotely monitored systems, places agents can't run
- **Billing is based on nodes** - all subscription plans are node-based
- **Nodes can be ephemeral or permanent** (configured at agent level)
  - Non-reachable permanent nodes trigger reachability notifications from Netdata Cloud

### Node States

| State | Meaning |
|-------|---------|
| **Reachable** | Online NOW - directly (agent connected to Cloud) or indirectly (parent in streaming path connected) |
| **Stale** | Historical data reported (usually via parent), but node not online now. Cloud can query historical data but not recent. |
| **Offline** | Known to exist, but not connected and no other node reporting for it. A "ghost". |

### Node Connectivity

- Agents can connect directly to Netdata Cloud
- Or indirectly through **parents** (streaming path)
- Parents can report historical data for children (stale state)
- Netdata Cloud fills the gap for standalone agents (no parents) and parents themselves

### Subscriptions

- **Subscriptions are billable entities** related to spaces
- **Each space has 1 active subscription** at any time
- **Subscription transitions are crucial** for trend analysis, revenue estimation, growth, churn
- Transitions = history of plan changes the space went through

### Plan Types

| Plan | Description | Pricing | Target |
|------|-------------|---------|--------|
| **Business** | Full feature set, unlimited users | $6/node/month ($4.5/node/month annually) | Companies |
| **Homelab** | Same features as Business, limited to 1 user, fair usage policy | $90/year unlimited nodes | Enthusiasts supporting Netdata |
| **Business Trial** | 14-day free trial of Business | Free for 14 days | New signups |
| **Community** | Free forever, non-commercial use, 5-node limit on multi-node dashboards | Free | Non-commercial users |

**Notes**:
- There is only **Business Trial** - no separate Homelab Trial (even homelabers go through Business Trial)
- Homelab is a way for enthusiasts to support Netdata while getting full features
- Community 5-node limit matches the limit on Netdata parents

### Trial Mechanics

- **Trial trigger**: New user signup with space creation
  - Existing users creating new spaces do NOT get a trial
- **Duration**: 14 days
- **Trial end**: Auto-converts to Community
- **Opt-out**: New users are not given opt-out option. Old users don't get trials on new spaces.

### Unrealized ARR (45-day newcomer problem)

The `Business_45d_monthly_newcomer` / `Homelab_45d_monthly_newcomer` plan class exists to track **unrealized ARR**:

**Scenario**:
1. User signs up in Business Trial
2. User says "ok, I want monthly subscription"
3. Subscription starts after 14 days of trial
4. 1 month later, first invoice issued with 15 days allowance to pay
5. Customer never pays → downgraded to Community

**Problem**: We recorded ARR when they said "I want it", but they never actually paid. This was "fake" ARR.

**Solution**: Finance created this plan class to isolate and monitor promised-but-not-yet-paid ARR separately from realized ARR.

- **45 days** = ~14 days trial + ~30 days first billing cycle + ~15 days payment grace
- Until payment is received, ARR is "unrealized"
- After payment confirmed, ARR becomes "realized"

### Revenue Model

| Term | Meaning |
|------|---------|
| **Discounted ARR** | Actual ARR received (after any special discounts applied) |
| **Undiscounted ARR** | What ARR would be at list price (without special discounts) |
| **Realized ARR** | ARR from customers who have actually paid |
| **Unrealized ARR** | ARR from customers who committed but haven't paid yet (45-day newcomers) |
| **business_overrides_arr** | Likely manual adjustments (needs confirmation) |

### Plan Transitions

**Valid transitions**:
- Trial → Business
- Trial → Community (trial ended, didn't upgrade)
- Business → Business (change of node commitment)
- Business → Community (churn)
- Homelab → Community (churn)
- Community → Business (re-subscription)
- Community → Homelab (re-subscription)
- Community → Deleted

**Re-subscription**: Yes, spaces can go from Community back to paid (intentionally or after accidental non-payment)

### Churn Definition

Churn has multiple faces:
1. **Paid → Non-paid**: Business/Homelab → Community
2. **Paid with lower commitment**: Remain paid but reduce node count
3. **Monthly disconnect**: Be on monthly plan and disconnect nodes (reducing billable nodes)

### What Counts as "Won"?

Won is churn reversed:
1. **Non-paid → Paid**: Community → Business/Homelab
2. **Increase of commitment**: Paid customer increases committed node count
3. **Monthly increasing nodes**: Monthly plan customer adds more reachable nodes

### Billing Model Details

#### Committed Nodes
- Business subscriptions have a **committed node count** = minimum they pay for
- Excess nodes (overage) charged at $6/node/month
- Commitment is a floor, not a cap

#### Pricing Tiers

| Plan | Pricing | Notes |
|------|---------|-------|
| **Business Monthly** | $6/node/month flat | No discount |
| **Business Annual** | $4.50/node/month | 25% discount + volume discount |
| **Homelab** | $90/year | Unlimited nodes, 1 user, fair usage |
| **Community** | Free | 5-node multi-node dashboard limit |

#### Overage
- `annual_overage_arr` = revenue from nodes exceeding commitment
- Overage rate: $6/node/month (same as monthly rate)

#### Node Billing Calculation (Double P90)
Netdata uses **double P90** for billing reachable nodes:
1. **Daily P90**: Exclude top 2.5 hours of each day
2. **Monthly P90**: Exclude top 3 days of the month

This is quite fair - spikes don't disproportionately affect billing.

### On-Prem

- **What it is**: Netdata Cloud installed at customer premises, totally off the grid (self-hosted)
- **Tracking**: Manual entries in `manual360_asat_*` tables
- **Source**: `manual_360_bq` is a Google Sheet, but agent cannot access it due to BigQuery MCP server bug
- **Workaround**: Use `manual360_asat_YYYYMMDD` snapshots instead
- **2025-10-01 cutoff**: Finance department made changes around this date; it's used as a transition point for baseline calculations

### Stripe Integration

- **Stripe customer = Space** (1:1 relationship)
- Each space is linked to a single Stripe customer
- `customer_id` format: `cus_XXXXXXXXXXXXXX`

### AI Credits

- **What they are**: Credits for using LLMs in Netdata Cloud for troubleshooting, root cause analysis, etc.
- **Free tier**: All customers get 10 credits free per month
- **Bundles**: Additional credits purchased as bundles (add-on to Business/Homelab)
- **Bundle values**: Likely credit amounts (e.g., Bundle625 = 625 credits)
- **1 credit = 1 report/troubleshoot** (one run of the entire playbook)
- **Launch**: Feature is new (started ~Oct-Nov 2024)

### Node Grades

- **Purpose**: Classify spaces by size (node count) for analysis
- **Scale**: A = largest, E = smallest
- **Thresholds** (from bigquery.ai):
  - Grade A: 501+ nodes
  - Grade B-E: Smaller tiers (exact thresholds in file)

Note: Grades are counter-intuitive - "A" is alphabetically first but represents LARGEST spaces.

### AWS Marketplace

- Netdata is also sold through AWS Marketplace
- Alternative billing channel (same product)
- Pricing likely the same as direct
- Tracked separately: `aws_business_arr`, `aws_business_subscriptions`

### Entity Cardinalities (Verified)

| Relationship | Cardinality | Notes |
|--------------|-------------|-------|
| User ↔ Space | N:N | User can be in many spaces with different roles; space has many users |
| Space ↔ Subscription | 1:1 | One active subscription per space at any time |
| Space ↔ Stripe Customer | 1:1 | Each space linked to single Stripe customer |
| Space ↔ Node | 1:N | Space has many nodes; node belongs to exactly one space |
| Node ↔ Room | N:N | Node can appear in multiple rooms within same space |
| Space ↔ Room | 1:N | Space contains many rooms |

## Decisions

### Pending User Input
All questions answered ✓

### Answered
1. ~~What is a user? What is an account? Relationship?~~ ✓
2. ~~What is a space? What does it represent?~~ ✓
3. ~~What is a node? Types of nodes?~~ ✓
4. ~~Subscription model and lifecycle~~ ✓
5. ~~Trial mechanics~~ ✓
6. ~~Plan types and differences~~ ✓
7. ~~Plan transition rules~~ ✓
8. ~~Revenue model (discounted, realized, etc.)~~ ✓
9. ~~What counts as "won"?~~ ✓
10. ~~Billing model (committed nodes, overage, pricing)~~ ✓
11. ~~On-prem model~~ ✓
12. ~~Stripe integration~~ ✓
13. ~~AI credits~~ ✓
14. ~~Node grades~~ ✓
15. ~~AWS Marketplace~~ ✓
16. ~~Entity cardinalities~~ ✓

### Made
(none yet - awaiting user approval to proceed with documentation)

## Plan

### PHASE 1: Domain Model Foundations ✓ COMPLETE
1. [x] Gather domain knowledge from user via Q&A ✓ COMPLETE

### PHASE 2: Terminology & Technical Details ✓ COMPLETE
2. [x] Document terminology issues throughout file ✓
3. [x] Research answers from analytics-bi repo docs ✓
4. [x] Research answers from cloud-spaceroom-service source ✓
5. [x] Research answers from BigQuery queries ✓
6. [x] Document remaining low-priority questions for devs ✓

### PHASE 3: Documentation Integration ✓ COMPLETE
7. [x] Draft domain model documentation section ✓
8. [x] Integrate into `neda/bigquery.ai` ✓ (inserted after line 36, before "Understanding user requests")
9. [x] Add Entity-to-Table Mapping section ✓ (links entities to tables, clarifies table family purposes)
10. [ ] Review with user
11. [ ] Run tests to verify no regressions
12. [ ] Delete this TODO file when verified

## PHASE 2: Terminology Questions & Research Findings

### 2.1 Field/Column Terminology

| Question | Status | Answer |
|----------|--------|--------|
| What is `ce_plan_class` vs `aw_sub_plan`? | ✓ ANSWERED | See below |
| What does `ax_trial_ends_at` NULL vs non-NULL mean? | ✓ ANSWERED | See below |
| What is `cx_arr_realized_at`? | ✓ ANSWERED | See below |
| What is `business_overrides_arr`? | ⚠️ PARTIAL | Likely manual adjustments - ASK DEVS |
| Field prefix convention (aa_, ab_, etc.)? | ❌ NOT FOUND | ASK DEVS |

**Answers:**

**`aw_sub_plan` vs `ce_plan_class`:**
- `aw_sub_plan`: Raw subscription plan name from payment system (e.g., "Business", "Homelab", "Business2024.03")
- `ce_plan_class`: Normalized plan classification derived from `aw_sub_plan`
  - Adds `_45d_monthly_newcomer` suffix for monthly Stripe customers within 45 days of plan start
  - **Source**: DAG SQL line 141: `IF((bc_period = "month" and ax_trial_ends_at is null and (ce_plan_class = "Business" or ce_plan_class = "Homelab") and DATE_DIFF(...) <= 45 and cd_payment_provider = "Stripe"), ce_plan_class || "_45d_monthly_newcomer", ce_plan_class)`
- **When to use**: Use `ce_plan_class` for plan classification logic (it handles newcomer state); use `aw_sub_plan` only when you need raw plan name

**`ax_trial_ends_at` meaning:**
| Value | Meaning |
|-------|---------|
| NULL | Paid customer (never was trial, or trial converted) |
| > CURRENT_TIMESTAMP() | Active trial (hasn't ended yet) |
| <= CURRENT_TIMESTAMP() AND ce_plan_class = 'Community' | Trial expired, didn't convert |

**`cx_arr_realized_at`:**
- ARR realization date for 45-day newcomers = `ca_cur_plan_start_date + 46 days`
- NULL = ARR not yet realized (newcomer in first 45 days)
- > CURRENT_DATE() = ARR will realize in future
- <= CURRENT_DATE() = ARR is realized (count it in total ARR)

### 2.2 Node State Terminology

| Question | Status | Answer |
|----------|--------|--------|
| Exact definition of "reachable" node? | ✓ ANSWERED | Currently online |
| Exact definition of "stale" node? | ✓ ANSWERED | Historical data but not online |
| Exact definition of "unreachable" node? | ✓ ANSWERED | Was connected, not responding |
| Exact definition of "unseen" node? | ✓ ANSWERED | = "created" state |
| What's the difference between states? | ✓ ANSWERED | See below |

**Node States (verified from cloud-spaceroom-service source code):**

Source: `/home/costa/src/netdata/cloud-spaceroom-service/model/node.go` lines 407-413

```go
const (
	NodeInstanceStateReachable   NodeInstanceState = "reachable"
	NodeInstanceStateStale       NodeInstanceState = "stale"
	NodeInstanceStateUnreachable NodeInstanceState = "unreachable"
	NodeInstanceStatePruned      NodeInstanceState = "pruned"
	NodeInstanceStateCreated     NodeInstanceState = "created"
)
```

| State | Meaning | Field in watch_towers |
|-------|---------|----------------------|
| `reachable` | Currently online (directly or via parent) | `ae_reachable_nodes` |
| `stale` | Historical data available (via parent) but node not online | `cs_stale_nodes` |
| `unreachable` | Was connected but not responding now | `ag_unreachable_nodes` |
| `pruned` | Node deleted/removed | (not in spaces_asat) |
| `created` | Node exists but never connected | `ct_created_nodes` |

**Priority Order** (for determining node status when multiple instances exist):
`reachable > stale > unreachable > pruned > created`

**"Unseen" Nodes (RESOLVED):**
- "unseen" = `ct_created_nodes` = nodes in "created" state
- Definition: Nodes that exist in the system but have **never sent any data** (registered but never connected)
- Source: `metrics.metrics_daily.sql` line 1739: `SUM(ct_created_nodes) AS metric_value` for `business_unseen_node_sum`
- **Note**: "unseen" is a metric name, not an official node state. The underlying state is "created".

### 2.3 Revenue/ARR Terminology

| Question | Status | Answer |
|----------|--------|--------|
| What is `direct_arr` vs `indirect_arr`? | ✓ ANSWERED | See below |
| How do ARR components relate to total? | ✓ ANSWERED | See below |
| What are "overrides"? | ✓ ANSWERED | Placeholder metric (never used) |
| What triggers ARR realization? | ✓ ANSWERED | Day 46 for monthly newcomers |

**Direct vs Indirect ARR:**
```sql
-- Direct: Customer pays Netdata via Stripe
cu_reseller_id IS NULL AND cd_payment_provider = 'Stripe'

-- Indirect: Customer pays via reseller OR marketplace
cu_reseller_id IS NOT NULL OR cd_payment_provider != 'Stripe'
```

**Note:** AWS Marketplace (`cd_payment_provider = 'AWS'`) is considered indirect even without a reseller.

**ARR Component Breakdown:**
- `stripe_business_arr` + `stripe_homelab_arr` = Direct cloud ARR
- `aws_business_arr` = AWS Marketplace ARR (indirect)
- `indirect_arr` = Reseller + marketplace ARR
- `direct_arr` = Stripe ARR
- `onprem_arr` = On-prem contracts (from manual360)
- **Total Realized ARR** = `arr_business_discount + arr_homelab_discount + ai_credits_space_revenue + onprem_arr`

**`business_overrides_arr` (RESOLVED):**
- This metric exists in `metrics_daily` but has **never had any non-zero values**
- It's a placeholder metric defined in dashboard SQL but not populated by any DAG
- Used in "Total ARR + Unrealized ARR" dashboard panel as a COALESCE(..., 0)
- **Safe to ignore** in documentation - it's dead code

### 2.4 Grade System

| Question | Status | Answer |
|----------|--------|--------|
| Exact node thresholds for grades? | ✓ ANSWERED | See below |
| Why is A=largest? | ✓ ANSWERED | Legacy naming |

**Node Grade Thresholds (verified from DAG SQL lines 1105-1115):**
| Grade | Node Count | Description |
|-------|------------|-------------|
| E | 1-5 | Smallest paid tier |
| D | 6-20 | Small |
| C | 21-100 | Medium |
| B | 101-500 | Large |
| A | 501+ | Enterprise |
| EMPTY | 0 | No billable nodes |

**Why A=largest**: Historical naming convention. "A" is alphabetically first but represents LARGEST spaces. For string comparisons in hierarchy tracking: "A" < "B" < "C" < "D" < "E" alphabetically.

### 2.5 Dataset/Table Naming

| Question | Status | Answer |
|----------|--------|--------|
| Why "watch_towers"? | ❌ NOT FOUND | ASK DEVS (likely historical) |
| What does "360" mean? | ⚠️ PARTIAL | Likely "360-degree view" |
| What is a "feed event"? | ✓ ANSWERED | Plan change audit event |

**feed_events_plan_change_finance:**
- Audit log of plan changes
- Columns include: `telemetry_space_id`, `current_telemetry_plan`, `previous_telemetry_plan`, `event_timestamp`, `Business_counter`, `Trial_counter` (+1/-1 for net change)

### 2.6 Metric Granularity

| Question | Status | Answer |
|----------|--------|--------|
| What does `asat` mean? | ✓ ANSWERED | Point-in-time snapshot |
| What does `daily` mean? | ✓ ANSWERED | Incremental delta |
| How does `_TABLE_SUFFIX` work? | ✓ ANSWERED | Date sharding pattern |

**Granularity Rules:**
| Granularity | Meaning | Aggregation | Example |
|-------------|---------|-------------|---------|
| `asat` / `as_at` | Point-in-time snapshot | Use `MAX()` | `total_business_subscriptions` |
| `daily` | Incremental change/delta | Use `SUM()` | `new_business_subs` |
| `hourly` | Hourly increment | Use `SUM()` | (rare) |

**_TABLE_SUFFIX Pattern:**
- Format: `YYYYMMDD` (no dashes), e.g., `20251220`
- Example: `WHERE _TABLE_SUFFIX = FORMAT_DATE('%Y%m%d', CURRENT_DATE() - 1)`

### 2.7 Plan/Subscription Fields

| Question | Status | Answer |
|----------|--------|--------|
| Full list of `ce_plan_class` values? | ✓ ANSWERED | See below |
| Full list of `aw_sub_plan` values? | ✓ ANSWERED | See below |
| What makes member "active"? | ❌ NOT FOUND | ASK DEVS |

**`ce_plan_class` Values (COMPLETE from BigQuery):**
| Value | Description |
|-------|-------------|
| `Business` | Paid business tier (after 45-day threshold if monthly) |
| `Business_45d_monthly_newcomer` | Business, monthly, first 45 days |
| `Homelab` | Paid homelab tier |
| `Homelab_45d_monthly_newcomer` | Homelab, monthly, first 45 days |
| `Community` | Free tier |
| `Pro` | Legacy Pro plan |

**Note**: `Trial` is NOT a separate `ce_plan_class` value. Trial spaces have `ax_trial_ends_at IS NOT NULL` with another plan class.

**`aw_sub_plan` Values (COMPLETE from BigQuery):**
| Value | Description |
|-------|-------------|
| `Business` | Standard business plan |
| `Business2024.03` | Business plan with March 2024 pricing |
| `Homelab` | Homelab plan |
| `Community2023.11` | Community plan (Nov 2023 version) |
| `Pro` | Legacy Pro plan |

**`cd_payment_provider` Values (from BigQuery):**
- `Stripe` - Direct payment via Stripe
- `AWS` - AWS Marketplace
- `GCP` - GCP Marketplace
- `NULL` - Community/Trial (no payment)

### 2.8 Churn/Won Detailed Definitions

| Question | Status | Answer |
|----------|--------|--------|
| Status values in delta templates? | ⚠️ PARTIAL | won/lost/increase/decrease/no_change |
| How is each status determined? | ❌ NOT FOUND | Need to check bigquery.ai templates |

## Questions for Devs/Finance

### Resolved via Research
1. ~~**business_overrides_arr**~~: Placeholder metric, never populated, safe to ignore
2. ~~**Full aw_sub_plan list**~~: Business, Business2024.03, Homelab, Community2023.11, Pro
3. ~~**"unseen" node state**~~: = "created" state (nodes registered but never connected)

### Remaining Questions (Low Priority)
1. **Field prefix convention**: What do the prefixes (aa_, ab_, ae_, aw_, ax_, bq_, ce_, cg_, cx_) mean? Is there a pattern?
2. **Active members**: What makes a member "active" in `spaces_active_members_sum`?
3. **watch_towers naming**: Why is the dataset called this? (likely historical)
4. **360 naming**: Is "360" meant to mean "360-degree view"? (likely yes)
5. **Status values in delta SQL**: What are all possible values (won/lost/increase/decrease/no_change)?

**Note**: These are low priority - we have enough information to proceed with documentation.

## Testing Requirements
- N/A (documentation only)

## Documentation Updates
- `neda/bigquery.ai` - add domain model section (~500-800 words) near the top
- `neda/bigquery.ai` - add glossary section with all terminology
- `neda/bigquery.ai` - add context to metrics list (group by category)
