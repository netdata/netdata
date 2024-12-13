# Node Membership Rules

## Overview

The Node Membership feature provides automatic node assignment to rooms based on user-defined rules. Users can create multiple rules per room to control node membership. When a node in the space matches these rules, it will be automatically included in or excluded from the room where the rules are defined. This allows for dynamic and automated organization of nodes based on their characteristics.

## Rule structure

A node membership rule defines whether a node should be included or excluded from a room based on its host labels. Rules are composed of the following elements:

- `Action`: (Required) It determined the rule's behavior and has the following possible values:
  - `Included`: (Default) Any node that matches the current rull will be included in the room.
  - `Excluded`: Any node that matches the current rull will not be included in the room.
- `Clauses`: (Required) A collection of conditions that must **all be satisfied**. At least one condition is required.
- `Description`: (Optional) A description of the rule.

### Clauses

Each clause represents a condition that must be met and is consisted of three parts:

- `Label`: The name of the host label to evaluate.
- `Operator`: The comparison operator between the label and the value.
- `Value`: The value to compare against.

All three parts of clause must be specified to consider this clause valid.

**Example of a valid clause**

```
[_architecture] [equals] [x86_64]
```

## Rules evaluation

Rules are evaluated in a specific order to determine node membership. `Exclude` rules are evaluated first and then `Include` rules do. This means that an `Include` rule takes priority over an `Exclude` rule with the same clauses.

If a room has multiple rules, a node will be automatically asigned to this room, if it matches **at least one** of them.

**Evaluation logic**

```
Rule 1: [action] [clause 1] AND [clause 2]
Rule 2: [action] [clause 3] AND [clause 4]

Evaluation: [Rule 1] OR [Rule 2]
```

## Create rules

In order to create a rule, you have to go to `Space settings` > `Rooms`, select the desired room by clicking the arrow icon from the `Actions` column on the right and then click on the `Nodes` tab. If you have the right [permissions](#notes), you will see an `Add new rule` button at the top of the screen, click it and a new blank rule will be created.

Select the desired action (by default is `Included`) and create one or more clauses by clicking on the `Select label...` placeholder. Optionally, add a description by clicking on the rule field label (`Rule X`).

Note that at that stage nothing has been saved, you will have to click `Save` button on the right of the rule. After clicking `Save`, the rules list get refreshed and the membership status of the nodes on the table below will be updated accordingly. You can add more rules in the same way.

## Nodes membership

Each node in the room has a membership status of `STATIC`, `RULE`, or both of them.

- `STATIC`: The node has been statically asigned to this room, either as part of the claiming process or through user actions (manual addidtion from the UI).
- `RULE`: The node has been automatically asigned to this room as a result of the room's node membership rules evaluation.
- `STATIC` and `RULE`: The node is statically assigned and also matches one or more room node membership rules.

You can view each node's membership status from the `Membership` column in the room's nodes table.

## Notes

- You cannot create node membership rules for `All nodes` roon, since this room contains all space nodes by default.
- Only users who have the permission to add nodes can actually create or edit node membership rules.
