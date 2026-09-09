<!-- markdownlint-disable MD013 -->

# Python GC operator model

## Operator goal

The profile answers three runtime-level questions for each garbage-collector generation:

- the rate of successfully collected objects;
- the rate of uncollectable objects; and
- the rate of garbage-collection cycles.

These are distinct event populations, so each remains a separate view.

## Identity and dimensions

`generation` is the required identity of each garbage-collector chart instance and remains available as a chart label.
Each chart has one fixed semantic dimension. This lets operators filter or compare generations directly and aggregate the
bounded generation instances in Netdata when they want a runtime-wide view.

## Composition

The profile deliberately omits `app`. An application profile supplies application identity when composed, while the job
name supplies fallback identity when Python GC metrics are selected alone. The retained `process_runtime` namespace and
`Process Runtime/Python GC` family place these views under the common runtime hierarchy.
