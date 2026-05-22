# Node Rule-Based Room Assignment

You can organize Nodes within Rooms automatically using configurable label-based rules. This feature simplifies infrastructure management by dynamically assigning Nodes to appropriate Rooms based on their host labels, eliminating manual intervention.

## How It Works

Rules automatically assign Nodes to Rooms based on their host labels. When you create a rule, it continuously evaluates all Nodes and assigns them to the appropriate Room when they match your criteria.

**Rule Evaluation Order:**

- Exclusion rules are evaluated first
- Inclusion rules are evaluated second

In cases where both an inclusion and exclusion rule match, the exclusion rule takes precedence.

**Child Node Behavior**

- Child nodes that are only indirectly connected to Cloud (through a Parent Agent) are no longer automatically added to the same Room as the Parent. 
- If your workflow depended on this automatic grouping, we recommend setting up Room assignment rules. 

:::tip

This allows for fine-grained automatic assignment of nodes to Rooms based on hostnames or other host labels.

:::

:::important

- You can use rules with all Rooms except the "All Nodes" Room, as it includes all Nodes by default
- You need Node management permissions to create and edit Rules
- Rules are evaluated in real-time as labels change
- Exclusion rules always override inclusion rules

:::

## Create Your First Rule

1. **Access Settings**
    - Click ⚙️ (Room settings)
    - Select "Nodes" tab

2. **Create Rule**
    - Click "Add new Rule"
    - Select Action (Include/Exclude)
    - Add clause(s)
    - Save changes

## Rule Structure

You can build rules with the following elements:

| Element     | Description                                                                                       |
|:------------|:--------------------------------------------------------------------------------------------------|
| **Action**  | Determines whether matching Nodes will be included or excluded from the Room                      |
| **Clauses** | Set of conditions that determine which Nodes match the Rule (all must be satisfied - logical AND) |

Each clause consists of:

| Element      | Description                  |
|:-------------|:-----------------------------|
| **Label**    | The host label to check      |
| **Value**    | The comparison method        |
| **Operator** | The value to compare against |

**Example Rule Structure:**

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

## Example: Kubernetes Environments

If you run multiple environments (such as dev, acc, and prod) in a single Kubernetes cluster, you can use host labels and room assignment rules to automatically route each environment's Nodes into separate Rooms.

### Step 1 — Set host labels on child Nodes

Use the Helm chart's `child.configs.netdata.data` override to add a `[host labels]` section to every child Node. Create (or edit) an `override.yml` file:

```yaml
child:
  configs:
    netdata:
      data: |
        [host labels]
          environment = ${CLUSTER_ENV}
```

Deploy (or upgrade) with the override:

```bash
helm upgrade -f override.yml netdata netdata/netdata
```

Set `CLUSTER_ENV` to `dev`, `acc`, or `prod` per cluster (or node pool) via your deployment tooling. For more Helm chart options, see the [Kubernetes installation guide](/docs/netdata-agent/installation/kubernetes.md) and the [Helm chart reference](https://github.com/netdata/helmchart#configuration).

### Step 2 — Create one Room per environment

In Netdata Cloud, create three Rooms — for example **Dev**, **Acc**, and **Prod**.

### Step 3 — Create inclusion rules

In each Room's **Nodes** tab, add an inclusion rule that matches the `environment` host label:

| Room | Label         | Operator | Value  |
|:-----|:--------------|:---------|:-------|
| Dev  | `environment` | equals   | `dev`  |
| Acc  | `environment` | equals   | `acc`  |
| Prod | `environment` | equals   | `prod` |

Rules are evaluated in real time — as soon as a child Node connects with the matching label, it is assigned to the corresponding Room.

:::note

Netdata deploys one child Agent per Kubernetes **node**, not per namespace. This pattern works best when each environment runs on dedicated node pools. If environments share nodes, consider using additional host labels (for example `node-role`) to differentiate them.

:::

## Comparison Operators

You can use the following operators to compare label values:

| Operator    | Description                                         |
|:------------|:----------------------------------------------------|
| equals      | Matches the exact value                             |
| starts_with | Matches if the value begins with the specified text |
| ends_with   | Matches if the value ends with the specified text   |
| contains    | Matches if the text appears anywhere in the value   |

## Membership Status

Nodes can have multiple membership types in a Room:

| Status          | Description                |
|:----------------|:---------------------------|
| STATIC          | Manually added to the Room |
| RULE            | Added by matching Rule(s)  |
| STATIC and RULE | Both manual and Rule-based |

You can view each Node's membership status in the Room's Nodes table under the "Membership" column.

:::note

Group membership can be either STATIC or RULE—these work independently. A node can belong to groups through STATIC assignments (added manually) or through RULE assignments (matched automatically). RULEs cannot override STATIC memberships, and removing a node's STATIC membership does not affect its RULE memberships.

:::
