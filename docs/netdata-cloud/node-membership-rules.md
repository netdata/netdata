# Node Membership Rules

Any Node in your Space can be dynamically included or excluded from any of your Rooms. This is done via creating Rules matching on host labels. It allows for dynamic and automated organization of Nodes based on their characteristics.

> **Note**
>
> Node Membership Rules are available for all Rooms except the "All Nodes" Room, as it includes all Nodes by default.
>
> Only users who have the permission to add Nodes can create or edit Node Membership Rules.

## Structure

Rules are composed of the following elements:

| element | description                                                             |
|:--------|:------------------------------------------------------------------------|
| Action  | Can be either "Included" or "Excluded"                                  |
| Clauses | A collection of conditions that must **all be satisfied** (logical AND) |

## Clauses

Each clause consists of:

| element  | description                                  |
|:---------|:---------------------------------------------|
| Label    | The name of the host label to evaluate       |
| Value    | The value to compare against                 |
| Operator | The operator between the label and the value |

## Creation

To create a new Rule on your Room:

1. Go to Room settings (Cog next to your Room's name)
2. Click on the "Nodes" tab
3. Click the "Add new Rule" prompt
4. Fill in your Rule's details and save it

## Evaluation

Rules are evaluated in a specific order to determine Node Membership in a Room. If a Node is both included and excluded by two Rules, the exclusion is honored.

## Membership

Each Node in the Room has a Membership status of `STATIC`, `RULE`, or both.

| status          | description                                                                            |
|:----------------|:---------------------------------------------------------------------------------------|
| STATIC          | The Node has been statically assigned to this Room                                     |
| RULE            | The Node has been automatically assigned to this Room as a result of a Membership Rule |
| STATIC and RULE | The Node is statically assigned and also matches one or more Membership Rules          |

You can view each Node's Membership status from the same-named column in the Room's Nodes table.
