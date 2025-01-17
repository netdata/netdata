# Node Rule-Based Room Assignment

Organize Nodes within Rooms automatically using configurable label-based rules. This feature simplifies infrastructure management by dynamically assigning Nodes to appropriate Rooms based on their host labels, eliminating manual intervention.

**Important**:

- Rules can be used with all Rooms except "All Nodes" (which automatically includes every Node)
- You must have Node management permissions to create or modify Rules
- Rules are evaluated in real-time, they automatically update Node assignments when labels change
- When both inclusion and exclusion rules exist, exclusion rules always take priority

## Understanding Rules

A rule consists of two main parts:
1. An Action (Include or Exclude) that determines whether matching Nodes will be added to or removed from the Room
2. One or more Clauses that define the conditions for matching Nodes (all clauses must be true for a match)

### Clause Components

Each clause checks a specific host label using these components:

| Component | Description |
|:----------|:------------|
| Label | The host label to evaluate |
| Operator | The comparison method (e.g., equals, contains) |
| Value | What to compare the label against |
| Negate | When set to true, reverses the comparison result (e.g., "equals" becomes "does not equal") |

#### Available Operators

The following operators can be used to compare label values:

| Operator | Description |
|:---------|:------------|
| equals | Exact match of the entire value |
| starts_with | Value begins with the specified text |
| ends_with | Value ends with the specified text |
| contains | Value includes the specified text anywhere |

For example, with the label `environment: production-us-east`:
- `equals: production-us-east` ✓ matches
- `starts_with: production` ✓ matches
- `ends_with: east` ✓ matches
- `contains: us` ✓ matches

### Example Rule

Below is a conceptual representation of a rule that includes all production database Nodes:

```yaml
Action: Include
Clauses:
  - Label: environment
    Operator: equals
    Value: production
  - Label: service-type
    Operator: equals
    Value: database
```

## How Rules Work

### How Rules Are Processed

1. Exclusion Rules are checked first, then Inclusion Rules. Processing stops at the first matching rule
2. Within each rule:
  - All clauses must match for the rule to apply (AND logic)
  - If any clause doesn't match, the rule is skipped and the next rule is evaluated
3. Between different rules:
  - Only one rule needs to match (OR logic)
4. If no rules match:
  - The Node is removed from the Room if it was previously added by rules
  - Note: This doesn't affect Nodes that were manually added (STATIC membership)

### Node Membership Types

Nodes can belong to a Room in different ways:

| Type | Description |
|:-----|:------------|
| STATIC | Node was manually added to the Room |
| RULE | Node was added by matching Rule(s) |
| STATIC and RULE | Node matches both criteria |

You can view each Node's membership status in the Room's Nodes table under the "Membership" column.

Note: STATIC and RULE memberships are independent. A rule cannot remove a manual (STATIC) membership, and manually removing a Node doesn't affect its rule-based (RULE) membership.

## Creating Rules

1. Access Settings
    - Click ⚙️ (Room settings)
    - Select "Nodes" tab
2. Create Rule
    - Click "Add new Rule"
    - Select Action (Include/Exclude)
    - Add clause(s)
    - Save changes
