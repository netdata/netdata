# Input/Output Contracts

Configure structured input and output for agents using JSON schemas. Essential for sub-agents and API integrations.

---

## Table of Contents

- [Overview](#overview) - What contracts enable
- [Quick Example](#quick-example) - Basic contract configuration
- [Output Configuration](#output-configuration) - Structured output with schemas
- [Input Configuration](#input-configuration) - Validated input for sub-agents
- [JSON Schema Basics](#json-schema-basics) - Schema syntax reference
- [Schema Files](#schema-files) - External schema references
- [Common Patterns](#common-patterns) - Typical contract configurations
- [Troubleshooting](#troubleshooting) - Common mistakes and fixes
- [See Also](#see-also) - Related pages

---

## Overview

Contracts define:

- **Output format**: What format the agent returns (`json`, `markdown`, `text`)
- **Output schema**: JSON Schema for structured JSON output
- **Input schema**: JSON Schema for sub-agent input validation

**User questions answered**: "How do I get structured output?" / "How do I validate input?"

**Why use contracts?**

- Predictable, parseable output
- Type-safe integration with other systems
- Clear API for sub-agents
- Validation catches errors early

---

## Quick Example

Structured JSON output:

```yaml
---
description: Company analyzer
models:
  - openai/gpt-4o
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      industry:
        type: string
      summary:
        type: string
    required:
      - name
      - summary
---
Analyze the company and return structured data.
```

Sub-agent with input validation:

```yaml
---
description: Data processor
toolName: data_processor
models:
  - openai/gpt-4o
input:
  format: json
  schema:
    type: object
    properties:
      data:
        type: array
      operation:
        type: string
        enum: ["sum", "average", "max", "min"]
    required:
      - data
      - operation
output:
  format: json
  schema:
    type: object
    properties:
      result:
        type: number
    required:
      - result
---
```

---

## Output Configuration

### output

| Property | Value     |
| -------- | --------- |
| Type     | `object`  |
| Default  | Undefined |

**Description**: Output specification. Defines the expected output format and schema.

**Sub-keys**:

| Sub-key     | Type                                | Required    | Description                  |
| ----------- | ----------------------------------- | ----------- | ---------------------------- |
| `format`    | `'json'`, `'markdown'`, or `'text'` | Yes         | Output format                |
| `schema`    | `object`                            | Optional    | Inline JSON Schema           |
| `schemaRef` | `string`                            | Alternative | Path to external schema file |

---

### output.format

**Valid values**: `json`, `markdown`, `text`

| Format     | Description            | Use Case               |
| ---------- | ---------------------- | ---------------------- |
| `json`     | Structured JSON output | APIs, data processing  |
| `markdown` | Formatted markdown     | Human-readable reports |
| `text`     | Plain text             | Simple responses       |

**Example**:

```yaml
---
output:
  format: json # Structured data
---
---
output:
  format: markdown # Formatted text
---
---
output:
  format: text # Plain text
---
```

**Notes**:

- `format: json` can optionally include a schema (inline or via `schemaRef`) for validation and tool documentation
- Schema is required when format is `json` when calling via MCP headend
- The format affects the `${FORMAT}` variable in prompts
- Headends use format to determine response type

---

### output.schema

**Type**: JSON Schema object

**Description**: Inline JSON Schema defining the structure of JSON output.

**Example**:

```yaml
---
output:
  format: json
  schema:
    type: object
    properties:
      title:
        type: string
        description: Report title
      findings:
        type: array
        items:
          type: object
          properties:
            point:
              type: string
            source:
              type: string
      confidence:
        type: number
        minimum: 0
        maximum: 1
    required:
      - title
      - findings
---
```

**Notes**:

- Schema is used for validation
- Schema is shown in tool documentation for sub-agents
- Agent is instructed to match the schema

---

### output.schemaRef

**Type**: `string` (file path)

**Description**: Path to an external JSON or YAML schema file.

**Example**:

```yaml
---
output:
  format: json
  schemaRef: ./schemas/report.json
---
```

**Schema file** (`./schemas/report.json`):

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "title": { "type": "string" },
    "findings": {
      "type": "array",
      "items": { "type": "string" }
    }
  },
  "required": ["title", "findings"]
}
```

**Path resolution**:

- Relative paths resolve from the agent file's directory
- Absolute paths are used as-is
- Supports `.json` and `.yaml`/`.yml` files

---

## Input Configuration

### input

| Property | Value                                                   |
| -------- | ------------------------------------------------------- |
| Type     | `object`                                                |
| Default  | `{ format: 'json', schema: DEFAULT_TOOL_INPUT_SCHEMA }` |

**Description**: Input specification for sub-agent tools. Defines how parent agents should provide input.

**Sub-keys**:

| Sub-key     | Type                 | Required    | Description                  |
| ----------- | -------------------- | ----------- | ---------------------------- |
| `format`    | `'text'` or `'json'` | No          | Input format                 |
| `schema`    | `object`             | Optional    | Inline JSON Schema           |
| `schemaRef` | `string`             | Alternative | Path to external schema file |

---

### input.format

**Valid values**: `text`, `json`

| Format | Description           | Parent Call           |
| ------ | --------------------- | --------------------- |
| `text` | Free-form text prompt | `{ "prompt": "..." }` |
| `json` | Structured JSON input | Schema-defined fields |

**Example**:

```yaml
---
# Text input
input:
  format: text
---
---
# JSON input with schema
input:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
    required:
      - query
---
```

---

### input.schema

**Type**: JSON Schema object

**Description**: Inline JSON Schema defining expected input structure.

**Example**:

```yaml
---
description: Search tool
toolName: searcher
input:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
        description: Search query
      maxResults:
        type: number
        default: 10
        minimum: 1
        maximum: 100
      filters:
        type: object
        properties:
          dateRange:
            type: string
            enum: ["day", "week", "month", "year"]
    required:
      - query
---
```

**Parent agent calls with**:

```json
{
  "tool": "agent__searcher",
  "input": {
    "query": "AI trends",
    "maxResults": 20,
    "filters": {
      "dateRange": "month"
    }
  }
}
```

**Notes**:

- Invalid inputs return as tool errors to parent
- Schema generates tool input schema for parent
- Descriptions help parent understand usage

---

### input.schemaRef

**Type**: `string` (file path)

**Description**: Path to an external JSON or YAML schema file for input.

**Example**:

```yaml
---
input:
  format: json
  schemaRef: ./schemas/search-input.yaml
---
```

---

## JSON Schema Basics

### Common Types

```yaml
# String
type: string

# Number
type: number

# Integer
type: integer

# Boolean
type: boolean

# Array
type: array
items:
  type: string

# Object
type: object
properties:
  key:
    type: string
```

### Object with Properties

```yaml
type: object
properties:
  name:
    type: string
    description: User's name
  age:
    type: integer
    minimum: 0
  email:
    type: string
    format: email
required:
  - name
  - email
```

### Arrays

```yaml
# Array of strings
type: array
items:
  type: string

# Array of objects
type: array
items:
  type: object
  properties:
    title:
      type: string
    url:
      type: string
  required:
    - title
```

### Enums

```yaml
type: string
enum:
  - low
  - medium
  - high
```

### Nested Objects

```yaml
type: object
properties:
  user:
    type: object
    properties:
      name:
        type: string
      address:
        type: object
        properties:
          street:
            type: string
          city:
            type: string
```

### Number Constraints

```yaml
type: number
minimum: 0
maximum: 100

type: integer
minimum: 1
exclusiveMaximum: 100
```

### String Constraints

```yaml
type: string
minLength: 1
maxLength: 100

type: string
pattern: "^[a-z]+$"

type: string
format: email  # Or: uri, date, date-time
```

### Optional vs Required

```yaml
type: object
properties:
  required_field:
    type: string
  optional_field:
    type: string
required:
  - required_field # Only this is required
```

---

## Schema Files

### JSON Schema File

`schemas/report.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "title": {
      "type": "string",
      "description": "Report title"
    },
    "sections": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "heading": { "type": "string" },
          "content": { "type": "string" }
        },
        "required": ["heading", "content"]
      }
    },
    "metadata": {
      "type": "object",
      "properties": {
        "author": { "type": "string" },
        "date": { "type": "string", "format": "date" }
      }
    }
  },
  "required": ["title", "sections"]
}
```

**Usage**:

```yaml
---
output:
  format: json
  schemaRef: ./schemas/report.json
---
```

### YAML Schema File

`schemas/report.yaml`:

```yaml
$schema: https://json-schema.org/draft/2020-12/schema
type: object
properties:
  title:
    type: string
    description: Report title
  sections:
    type: array
    items:
      type: object
      properties:
        heading:
          type: string
        content:
          type: string
      required:
        - heading
        - content
required:
  - title
  - sections
```

**Usage**:

```yaml
---
output:
  format: json
  schemaRef: ./schemas/report.yaml
---
```

### Shared Schemas

Multiple agents can use the same schema:

**Agent A**:

```yaml
---
output:
  format: json
  schemaRef: /shared/schemas/analysis.json
---
```

**Agent B**:

```yaml
---
output:
  format: json
  schemaRef: /shared/schemas/analysis.json
---
```

---

## Common Patterns

### Simple JSON Output

```yaml
---
description: Company lookup
models:
  - openai/gpt-4o
output:
  format: json
  schema:
    type: object
    properties:
      name:
        type: string
      website:
        type: string
      description:
        type: string
    required:
      - name
---
Look up the company and return basic information.
```

### Sub-Agent with Full Contract

```yaml
---
description: Data analyzer
toolName: data_analyzer
models:
  - openai/gpt-4o
input:
  format: json
  schema:
    type: object
    properties:
      data:
        type: array
        items:
          type: number
      analysis_type:
        type: string
        enum: ["summary", "trends", "anomalies"]
    required:
      - data
      - analysis_type
output:
  format: json
  schema:
    type: object
    properties:
      analysis_type:
        type: string
      results:
        type: object
      insights:
        type: array
        items:
          type: string
    required:
      - analysis_type
      - results
---
Analyze the provided data and return insights.
```

### API-Style Agent

```yaml
---
description: Product search API
toolName: product_search
models:
  - openai/gpt-4o
tools:
  - catalog
input:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
        description: Search terms
      category:
        type: string
        description: Product category filter
      priceRange:
        type: object
        properties:
          min:
            type: number
          max:
            type: number
      limit:
        type: integer
        default: 10
        maximum: 100
    required:
      - query
output:
  format: json
  schema:
    type: object
    properties:
      query:
        type: string
      totalResults:
        type: integer
      products:
        type: array
        items:
          type: object
          properties:
            id:
              type: string
            name:
              type: string
            price:
              type: number
            category:
              type: string
          required:
            - id
            - name
            - price
    required:
      - query
      - products
---
```

### Report Generator

```yaml
---
description: Research report generator
models:
  - anthropic/claude-sonnet-4-20250514
tools:
  - brave
output:
  format: json
  schemaRef: ./schemas/research-report.json
---
Research the topic and generate a comprehensive report.
```

### Markdown Output (No Schema)

```yaml
---
description: Blog post writer
models:
  - anthropic/claude-sonnet-4-20250514
output:
  format: markdown
---
Write a blog post on the given topic.
```

---

## Troubleshooting

### "Output doesn't match schema"

**Problem**: Agent output fails schema validation.

**Causes**:

- Agent doesn't follow schema
- Schema too strict
- Missing required fields

**Solutions**:

1. Improve prompt to emphasize schema:
   ```
   You MUST return JSON matching this exact schema:
   - name (required): string
   - items (required): array of strings
   ```
2. Relax schema (fewer required fields)
3. Use simpler schema structure

### "Input validation failed"

**Problem**: Parent's input rejected by sub-agent.

**Cause**: Input doesn't match sub-agent's `input.schema`.

**Solution**: Check schema requirements:

```yaml
# Sub-agent expects:
input:
  schema:
    type: object
    properties:
      query:
        type: string
    required:
      - query

# Parent must provide:
# { "query": "search term" }  -- NOT { "prompt": "search term" }
```

### "Schema file not found"

**Problem**: Error loading external schema.

**Cause**: Path incorrect or file missing.

**Solution**: Check path resolution:

```yaml
# Relative to agent file directory
schemaRef: ./schemas/my-schema.json

# Absolute path
schemaRef: /path/to/schemas/my-schema.json
```

### "Invalid JSON schema"

**Problem**: Schema itself is malformed.

**Causes**:

- YAML/JSON syntax error
- Invalid schema keyword
- Missing required schema fields

**Solution**: Validate schema:

1. Check YAML/JSON syntax
2. Verify `type` is specified
3. Check property definitions

### Output Is String Instead of Object

**Problem**: JSON output is a string, not parsed object.

**Cause**: Agent returned JSON as escaped string.

**Solution**: Ensure prompt is clear:

```
Return a JSON object (not a string containing JSON):
{
  "key": "value"
}
```

### Parent Agent Doesn't Know Input Format

**Problem**: Parent calls sub-agent incorrectly.

**Cause**: Parent doesn't see input schema.

**Solution**: Document in parent prompt:

```yaml
---
description: Coordinator
agents:
  - ./helpers/searcher.ai
---

Available tools:
- `agent__searcher`: Search for information
  Input: { "query": "search terms", "maxResults": 10 }
  Output: { "results": [...] }
```

---

## See Also

- [Agent-Files](Agent-Files) - Overview of .ai file structure
- [Agent-Files-Sub-Agents](Agent-Files-Sub-Agents) - Sub-agent configuration
- [Agent-Files-Identity](Agent-Files-Identity) - description and usage for sub-agents
- [Technical-Specs-Tool-System](Technical-Specs-Tool-System) - How tools receive schemas
