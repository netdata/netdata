# Library Embedding

Use AI Agent programmatically in your applications.

---

## Overview

AI Agent is designed library-first. The core performs no I/O and communicates via callbacks.

---

## Basic Usage

```typescript
import { AIAgent, AIAgentEventCallbacks } from 'ai-agent';

const callbacks: AIAgentEventCallbacks = {
  onEvent: (event) => {
    if (event.type === 'output') {
      process.stdout.write(event.text);
    }
    if (event.type === 'log') {
      console.error(`[${event.entry.severity}] ${event.entry.message}`);
    }
    if (event.type === 'final_report') {
      console.log('Final:', event.content);
    }
  }
};

const agent = new AIAgent({
  configPath: '.ai-agent.json',
  callbacks
});

const result = await agent.run({
  agentPath: 'chat.ai',
  userPrompt: 'Hello!'
});
```

---

## Event Types

| Event Type | Description |
|------------|-------------|
| `output` | Text output from agent |
| `log` | Log entry (VRB/WRN/ERR/TRC) |
| `accounting` | Token/cost accounting entry |
| `progress` | Progress updates |
| `status` | Status changes |
| `final_report` | Final agent response |
| `handoff` | Handoff to another agent |

---

## Event Handling

```typescript
const callbacks: AIAgentEventCallbacks = {
  onEvent: (event) => {
    switch (event.type) {
      case 'output':
        // Real-time text output
        process.stdout.write(event.text);
        break;

      case 'log':
        // Logging
        const { severity, message } = event.entry;
        if (severity === 'ERR') {
          console.error(message);
        }
        break;

      case 'accounting':
        // Track costs
        if (event.entry.type === 'llm') {
          totalCost += event.entry.cost || 0;
        }
        break;

      case 'final_report':
        // Handle final response
        if (event.meta.isFinal) {
          saveResult(event.content);
        }
        break;

      case 'handoff':
        // Agent delegated to another
        console.log(`Handing off to: ${event.targetAgent}`);
        break;
    }
  }
};
```

---

## Configuration Options

```typescript
const agent = new AIAgent({
  // Required
  configPath: '.ai-agent.json',

  // Optional
  callbacks: myCallbacks,
  llmTimeout: 120000,
  toolTimeout: 60000,
  verbose: true,

  // Override defaults
  defaults: {
    temperature: 0.5,
    maxTurns: 15
  }
});
```

---

## Running Agents

### From File

```typescript
const result = await agent.run({
  agentPath: 'path/to/agent.ai',
  userPrompt: 'User question'
});
```

### With Input Object

```typescript
const result = await agent.run({
  agentPath: 'analyzer.ai',
  userPrompt: JSON.stringify({
    text: 'Content to analyze',
    detailed: true
  })
});
```

### With Conversation History

```typescript
const result = await agent.run({
  agentPath: 'chat.ai',
  userPrompt: 'Follow-up question',
  conversationHistory: previousHistory
});
```

---

## Result Object

```typescript
interface AIAgentResult {
  status: 'success' | 'error';
  content: string;
  format: 'markdown' | 'json' | 'text';
  tokens: {
    input: number;
    output: number;
  };
  cost: number;
  conversationHistory: Message[];
}
```

---

## Error Handling

```typescript
try {
  const result = await agent.run({
    agentPath: 'agent.ai',
    userPrompt: 'query'
  });

  if (result.status === 'error') {
    console.error('Agent failed:', result.content);
  }
} catch (error) {
  // Configuration or initialization errors
  console.error('Fatal error:', error);
}
```

---

## Silent Core

The core library:
- Never writes to files
- Never writes to stdout/stderr
- All output via `onEvent` callbacks

This allows complete control over I/O in your application.

---

## Finality Notes

- `event.meta.isFinal` is authoritative **only for `final_report` events**
- For other event types, treat it as informational
- Use `handoff` event + `pendingHandoffCount` to detect delegation chains

---

## Example: REST Server

```typescript
import express from 'express';
import { AIAgent } from 'ai-agent';

const app = express();
const agent = new AIAgent({ configPath: '.ai-agent.json' });

app.post('/chat', async (req, res) => {
  let response = '';

  const result = await agent.run({
    agentPath: 'chat.ai',
    userPrompt: req.body.message,
    callbacks: {
      onEvent: (event) => {
        if (event.type === 'output') {
          response += event.text;
        }
      }
    }
  });

  res.json({ response });
});

app.listen(3000);
```

---

## See Also

- [Headends](Headends) - Built-in headends
- [docs/AI-AGENT-INTERNAL-API.md](../docs/AI-AGENT-INTERNAL-API.md) - Full API documentation
- [docs/specs/library-api.md](../docs/specs/library-api.md) - Technical spec
