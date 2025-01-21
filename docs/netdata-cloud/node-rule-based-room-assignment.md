# Node Rule-Based Room Assignment

Organize Nodes within Rooms automatically using configurable label-based rules. This feature simplifies infrastructure management by dynamically assigning Nodes to appropriate Rooms based on their host labels, eliminating manual intervention.

**Important**:

- Rules work with all Rooms except the "All Nodes" Room, as it includes all Nodes by default.
- Creating and editing Rules requires Node management permissions.
- Rules are evaluated in real-time as labels change.
- Exclusion rules always override inclusion rules.

## Rule Structure

The rules consist of the following elements:

| Element | Description                                                                                       |
|:--------|:--------------------------------------------------------------------------------------------------|
| Action  | Determines whether matching Nodes will be included or excluded from the Room                      |
| Clauses | Set of conditions that determine which Nodes match the Rule (all must be satisfied - logical AND) |

Each clause consists of:

| Element  | Description                  |
|:---------|:-----------------------------|
| Label    | The host label to check      |
| Value    | The comparison method        |
| Operator | The value to compare against |

Below is a conceptual representation of a rule that includes all production database Nodes. The structure is shown in YAML format for clarity:

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

### Available Operators

The following operators can be used to compare label values:

| Operator    | Description                          |
|:------------|:-------------------------------------|
| equals      | Exact match of  value                |
| starts_with | Value begins with the specified text |
| ends_with   | Value ends with the specified text   |
| contains    | Value includes the specified text    |

## Rule Evaluation Order

- Exclusion rules are checked first
- Inclusion rules are checked second

If both match, exclusion wins

## Creating Rules

1. Access Settings
    - Click ⚙️ (Room settings)
    - Select "Nodes" tab
2. Create Rule
    - Click "Add new Rule"
    - Select Action (Include/Exclude)
    - Add clause(s)
    - Save changes

## Membership Status

Nodes can have multiple membership types in a Room:

| Status          | Description                |
|:----------------|:---------------------------|
| STATIC          | Manually added to the Room |
| RULE            | Added by matching Rule(s)  |
| STATIC and RULE | Both manual and Rule-based |

You can view each Node's membership status in the Room's Nodes table under the "Membership" column.

> **Note**
>
> STATIC and RULE memberships are independent. A rule cannot remove a manual (STATIC) membership, and manually removing a Node doesn't affect its rule-based (RULE) membership.
