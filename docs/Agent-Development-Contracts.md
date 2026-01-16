# Input/Output Contracts

Define structured interfaces for agent inputs and outputs using JSON Schema.

---

## Output Contracts

### Format Selection

```yaml
output:
  format: json | markdown | text
```

- `markdown` (default): Free-form markdown output
- `text`: Plain text output
- `json`: Structured JSON output (requires schema)

### JSON Schema Output

```yaml
---
models:
  - openai/gpt-4o
output:
  format: json
  schema:
    type: object
    properties:
      summary:
        type: string
        description: Brief summary of findings
      confidence:
        type: number
        minimum: 0
        maximum: 1
      sources:
        type: array
        items:
          type: string
    required:
      - summary
      - confidence
---
You are a research assistant. Return structured findings.
```

### Validation

- Output is validated against schema before returning
- Invalid output triggers retry with guidance
- Schema is included in LLM context

---

## Input Contracts

### Define Expected Input

```yaml
---
models:
  - openai/gpt-4o
input:
  type: object
  properties:
    query:
      type: string
      description: The research question
    maxResults:
      type: number
      default: 5
    language:
      type: string
      enum:
        - en
        - es
        - fr
  required:
    - query
---
```

### Input Validation

When called as a sub-agent tool:

```json
{
  "query": "Latest AI developments",
  "maxResults": 10,
  "language": "en"
}
```

Invalid input is rejected before execution.

---

## Complete Example

### Sentiment Analyzer

```yaml
#!/usr/bin/env ai-agent
---
description: Analyze sentiment of text
models:
  - openai/gpt-4o-mini
input:
  type: object
  properties:
    text:
      type: string
      description: Text to analyze
    detailed:
      type: boolean
      default: false
  required:
    - text
output:
  format: json
  schema:
    type: object
    properties:
      sentiment:
        type: string
        enum:
          - positive
          - negative
          - neutral
          - mixed
      confidence:
        type: number
        minimum: 0
        maximum: 1
      explanation:
        type: string
    required:
      - sentiment
      - confidence
---
Analyze the sentiment of the provided text.

If "detailed" is true, provide a thorough explanation.
Otherwise, keep the explanation brief.
```

### Usage

```bash
# CLI
ai-agent --agent sentiment.ai '{"text": "I love this product!"}'

# As sub-agent tool
{
  "name": "sentiment",
  "arguments": {
    "text": "I love this product!",
    "detailed": true
  }
}
```

### Response

```json
{
  "sentiment": "positive",
  "confidence": 0.95,
  "explanation": "Strong positive language with 'love' indicating enthusiasm."
}
```

---

## Schema Tips

### Use Descriptions

```yaml
schema:
  type: object
  properties:
    summary:
      type: string
      description: A 2-3 sentence summary of the main findings
```

Descriptions help the LLM understand expected content.

### Use Enums for Categories

```yaml
schema:
  properties:
    category:
      type: string
      enum:
        - bug
        - feature
        - question
        - documentation
```

### Use Arrays for Lists

```yaml
schema:
  properties:
    items:
      type: array
      items:
        type: object
        properties:
          name:
            type: string
          score:
            type: number
```

---

## Headend Behavior

### REST API

```bash
# Request format
curl "http://localhost:8080/v1/agent?format=json" \
  -d '{"text": "Hello"}'

# Response
{
  "sentiment": "neutral",
  "confidence": 0.8
}
```

### MCP Tool

When agents are exposed as MCP tools, the schema becomes the tool's parameter schema.

---

## Error Handling

### Invalid Output

If the LLM produces invalid JSON:
1. Error is logged
2. Turn is retried with TURN-FAILED guidance
3. After max retries, session fails with error

### Invalid Input

If caller provides invalid input:
1. Request is rejected immediately
2. Error message describes schema violation
3. No LLM calls are made

---

## See Also

- [Frontmatter Schema](Agent-Development-Frontmatter)
- [Multi-Agent Orchestration](Agent-Development-Multi-Agent)
