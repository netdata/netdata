# Alerts

The class that reaches support most often. Most reports are "not firing"; the diagnosis is usually
that the alert was silenced, never enabled, or already resolved by something the reporter did not
connect to it.

Owners: `src/health/README.md#alert-lifecycle-and-states` for states and
`src/health/README.md#evaluation-timing` for when evaluation happens;
`src/health/REFERENCE.md#troubleshooting-decision-trees` for the decision trees;
`src/health/REFERENCE.md#how-to-disable-or-silence-alerts` for every way an alert goes quiet;
`src/web/api/health/README.md#persistence` for where silencers live;
`src/health/notifications/README.md#troubleshooting-alert-notifications` for delivery;
`src/health/overriding-stock-alerts.md#verify-your-override` when a customization is ignored.

## Order of checks

1. **Silencers.** The persisted silencer file is the first read, because a silenced alert is
   indistinguishable from a broken one. It survives restarts. POSIX only - see the gap below.
2. **Is the alert enabled at all?** An alert is enabled on demand, when the chart it targets starts
   collecting. An alert pointed at a context that never appears on this host is never enabled, and
   looks exactly like "not firing". Cross-check the alert's target against what the node actually
   collects, using the current alert state in the runtime area.
3. **Current state.** The runtime area carries active alerts and all alert instances. Compare the two:
   an alert present in the full list but absent from the active list is evaluating and not raising,
   which is a threshold or lookup question, not a plumbing one.
4. **The alert's own configuration.** User alert files live under the config directory; UI-edited
   alerts live in the dynamic configuration area and **override** the files. Reading only the files
   is how an "impossible" configuration gets reported.
5. **Transitions.** Health transitions exist only as log records, bounded by the collection window.
   Narrow the logs to health records rather than reading everything.
6. **Delivery.** If the alert did raise, the question is notification, not health. That is a separate
   configuration file and a separate failure surface.

## What the bundle settles well

- Whether a silencer was active.
- Whether the alert exists, is enabled, and what state it is in right now.
- Whether the effective health configuration matches what the reporter believes.
- Whether transitions happened inside the collection window.

## What it does not settle

- **Why an alert fired last week.** There is no structured transition history in the bundle; only log
  text inside the window. Outside it, the bundle is silent and that silence means nothing.
- **Whether a notification was delivered.** The bundle sees the agent's side of a send, not the
  recipient's side.
- **Cloud-side notification routing**, which is configured and evaluated outside the agent entirely.

## Recurring causes

Written as patterns, not as a measured ranking.

- A **silencer** left in place, commonly after earlier maintenance. A routing rule is a different
  symptom: it suppresses the notification, not the alert, so it explains "no notification" and never
  "no alert".
- The alert **targets a context this node does not collect**, so it was never enabled.
- An alert **will not clear because the entity it watched disappeared** - a removed disk, a deleted
  container - leaving the instance without fresh data rather than with good data.
- **Restart storms**: many nodes reported unreachable within a few minutes, which is an agent or
  parent restart rather than a real outage. Correlate with restart evidence before believing it.
- **Threshold and lookup shape** - a window or delay that does not match the reporter's expectation.
  Read the expression rather than the alert name.
- On **static installs**, a notification script path that points at the distribution location rather
  than the install prefix.

## Traps

- Routing an alert to a silent destination suppresses the **notification**, not the alert. The alert
  still raises and still appears in the runtime captures - so "no notification" and "no alert" are
  different findings with different evidence.
- A resolved alert without a matching raise in the logs usually means the raise happened before the
  collection window, not that the agent invented a recovery.
- "Alerts are broken" after an upgrade is commonly a stock-alert change, not a failure. Compare the
  effective configuration against the customization rather than against memory.

## Windows evidence gap

The Windows bundle collects **neither the persisted silencers nor the dynamic configuration area**.
On a Windows bundle, steps 1 and the UI-edited half of step 4 have no evidence at all. Do not
conclude "not silenced" from a Windows bundle - state that the evidence is absent and ask for the
silencer state directly. See `./windows.md` and `./evidence-limits.md`.
