# 13.3 Alert Lifecycle and State Transitions

Understanding alert lifecycle helps in troubleshooting and in designing effective alert strategies.

## Initial Evaluation

When an alert definition is first loaded, it enters the UNINITIALIZED state. This state persists until sufficient historical data exists to evaluate conditions.

During UNINITIALIZED, no notifications are sent. The alert exists but has not yet established a baseline for comparison.

When enough data accumulates, the alert transitions to CLEAR. From this point, subsequent evaluations track changes from baseline.

## State Transition Logic

Each evaluation applies transition logic defined in the alert configuration. The fundamental logic compares calculated values against warning and critical thresholds.

From CLEAR, conditions exceeding warning thresholds transition to WARNING. From WARNING, conditions exceeding critical thresholds transition to CRITICAL.

When transitioning toward CLEAR, the alert fires with CLEAR status. This firing indicates resolution and may trigger resolution notifications if configured.

## Repeat Interval and Notification Suppression

The repeat interval defines how frequently notifications are sent for ongoing alert states. A repeat interval of `1h` means at most one notification per hour for an ongoing alert condition.

Repeat intervals prevent notification storms while maintaining awareness of unresolved problems.