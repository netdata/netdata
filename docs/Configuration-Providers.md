# LLM Providers

Configure LLM providers for AI Agent.

---

## Table of Contents

- [Supported Providers](#supported-providers) - Available provider types
- [OpenAI](#openai) - OpenAI API configuration
- [Anthropic](#anthropic) - Anthropic Claude configuration
- [Google](#google) - Google Gemini configuration
- [OpenRouter](#openrouter) - Multi-provider gateway
- [Ollama](#ollama) - Local models
- [OpenAI-Compatible](#openai-compatible) - Custom endpoints
- [Provider Configuration Reference](#provider-configuration-reference) - All provider options
- [Per-Model Configuration](#per-model-configuration) - Model-specific settings
- [Model Overrides](#model-overrides) - Force or disable parameters
- [Interleaved Reasoning](#interleaved-reasoning) - Reasoning replay
- [Fallback Chains](#fallback-chains) - Model failover
- [Custom Headers](#custom-headers) - Additional HTTP headers
- [Troubleshooting](#troubleshooting) - Common issues
- [See Also](#see-also) - Related documentation

---

## Supported Providers

| Type                | Provider   | Description                           |
| ------------------- | ---------- | ------------------------------------- |
| `openai`            | OpenAI     | GPT-4o, GPT-4o-mini, o1, o3 models    |
| `anthropic`         | Anthropic  | Claude 4, Claude 3.5, Claude 3 models |
| `google`            | Google     | Gemini Pro, Gemini Flash models       |
| `openrouter`        | OpenRouter | Multi-provider gateway                |
| `ollama`            | Ollama     | Local self-hosted models              |
| `openai-compatible` | Custom     | Any OpenAI-compatible API             |
| `test-llm`          | Testing    | Deterministic test provider           |

---

## OpenAI

### Configuration

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "baseUrl": "https://api.openai.com/v1"
    }
  }
}
```

### Configuration Reference

| Property        | Type     | Default                     | Description                      |
| --------------- | -------- | --------------------------- | -------------------------------- |
| `type`          | `string` | Required                    | Must be `"openai"`               |
| `apiKey`        | `string` | Required                    | OpenAI API key (`sk-...`)        |
| `baseUrl`       | `string` | `https://api.openai.com/v1` | API endpoint URL                 |
| `contextWindow` | `number` | Model-specific              | Default context window size      |
| `tokenizer`     | `string` | `tiktoken:gpt-4o`           | Tokenizer for context estimation |

### Usage in Agent

```yaml
---
models:
  - openai/gpt-4o
  - openai/gpt-4o-mini
  - openai/o1
---
```

### Common Models

| Model         | Context Window | Notes                   |
| ------------- | -------------- | ----------------------- |
| `gpt-4o`      | 128,000        | Latest GPT-4 Omni       |
| `gpt-4o-mini` | 128,000        | Cost-effective GPT-4    |
| `o1`          | 200,000        | Reasoning model         |
| `o3-mini`     | 200,000        | Compact reasoning model |

---

## Anthropic

### Configuration

```json
{
  "providers": {
    "anthropic": {
      "type": "anthropic",
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  }
}
```

### Configuration Reference

| Property        | Type     | Default        | Description                      |
| --------------- | -------- | -------------- | -------------------------------- |
| `type`          | `string` | Required       | Must be `"anthropic"`            |
| `apiKey`        | `string` | Required       | Anthropic API key (`sk-ant-...`) |
| `contextWindow` | `number` | Model-specific | Default context window size      |

### Caching Mode

Anthropic's caching mode is controlled via the `caching` runtime option (see [Agent Configuration](Configuration) for usage).

| Value            | Behavior                                                 |
| ---------------- | -------------------------------------------------------- |
| `full` (default) | Apply ephemeral cache control to messages (cost savings) |
| `none`           | Disable cache control                                    |

### Usage in Agent

```yaml
---
models:
  - anthropic/claude-sonnet-4-20250514
  - anthropic/claude-3-5-haiku-20241022
---
```

### Common Models

| Model                        | Context Window | Notes                   |
| ---------------------------- | -------------- | ----------------------- |
| `claude-sonnet-4-20250514`   | 200,000        | Latest Sonnet           |
| `claude-3-5-sonnet-20241022` | 200,000        | Claude 3.5 Sonnet       |
| `claude-3-5-haiku-20241022`  | 200,000        | Fast and cost-effective |
| `claude-3-opus-20240229`     | 200,000        | Most capable Claude 3   |

### Cache Accounting

Cache tokens are tracked in accounting:

- `cacheReadInputTokens`: Tokens read from cache
- `cacheWriteInputTokens`: Tokens written to cache

---

## Google

### Configuration

```json
{
  "providers": {
    "google": {
      "type": "google",
      "apiKey": "${GOOGLE_API_KEY}"
    }
  }
}
```

### Configuration Reference

| Property        | Type     | Default        | Description                 |
| --------------- | -------- | -------------- | --------------------------- |
| `type`          | `string` | Required       | Must be `"google"`          |
| `apiKey`        | `string` | Required       | Google AI API key           |
| `contextWindow` | `number` | Model-specific | Default context window size |

### Usage in Agent

```yaml
---
models:
  - google/gemini-1.5-pro
  - google/gemini-1.5-flash
---
```

### Common Models

| Model              | Context Window | Notes                |
| ------------------ | -------------- | -------------------- |
| `gemini-1.5-pro`   | 1,000,000      | Long context support |
| `gemini-1.5-flash` | 1,000,000      | Fast and efficient   |
| `gemini-2.0-flash` | 1,000,000      | Latest Gemini 2      |

---

## OpenRouter

Multi-provider gateway that routes to various LLM providers.

### Configuration

```json
{
  "providers": {
    "openrouter": {
      "type": "openrouter",
      "apiKey": "${OPENROUTER_API_KEY}"
    }
  }
}
```

### Configuration Reference

| Property | Type     | Default  | Description            |
| -------- | -------- | -------- | ---------------------- |
| `type`   | `string` | Required | Must be `"openrouter"` |
| `apiKey` | `string` | Required | OpenRouter API key     |

### Environment Variables

| Variable             | Purpose                   | Default                  |
| -------------------- | ------------------------- | ------------------------ |
| `OPENROUTER_REFERER` | HTTP-Referer header       | `https://ai-agent.local` |
| `OPENROUTER_TITLE`   | X-OpenRouter-Title header | `ai-agent`               |

### Usage in Agent

```yaml
---
models:
  - openrouter/openai/gpt-4o
  - openrouter/anthropic/claude-3-sonnet
  - openrouter/meta-llama/llama-3.1-70b-instruct
---
```

### Model Naming

Format: `openrouter/<provider>/<model>`

Examples:

- `openrouter/openai/gpt-4o`
- `openrouter/anthropic/claude-3.5-sonnet`
- `openrouter/meta-llama/llama-3.1-405b-instruct`

### Cost Reporting

OpenRouter provides actual cost in API responses:

- `cost`: Total cost charged
- `upstream_inference_cost`: Actual provider cost

These override any pricing table configuration.

---

## Ollama

Local self-hosted models using Ollama.

### Configuration

```json
{
  "providers": {
    "ollama": {
      "type": "ollama",
      "baseUrl": "http://localhost:11434"
    }
  }
}
```

### Configuration Reference

| Property        | Type     | Default                  | Description            |
| --------------- | -------- | ------------------------ | ---------------------- |
| `type`          | `string` | Required                 | Must be `"ollama"`     |
| `baseUrl`       | `string` | `http://localhost:11434` | Ollama server URL      |
| `contextWindow` | `number` | `131072`                 | Default context window |

### Usage in Agent

```yaml
---
models:
  - ollama/llama3.1
  - ollama/mixtral
  - ollama/codestral
---
```

### Remote Ollama

For Ollama on a different machine:

```json
{
  "providers": {
    "ollama": {
      "type": "ollama",
      "baseUrl": "http://192.168.1.100:11434"
    }
  }
}
```

---

## OpenAI-Compatible

For self-hosted or custom APIs that follow the OpenAI API format.

### Configuration

```json
{
  "providers": {
    "nova": {
      "type": "openai-compatible",
      "apiKey": "${NOVA_API_KEY}",
      "baseUrl": "http://10.20.4.21:8090/v1"
    }
  }
}
```

### Configuration Reference

| Property        | Type     | Default  | Description                   |
| --------------- | -------- | -------- | ----------------------------- |
| `type`          | `string` | Required | Must be `"openai-compatible"` |
| `apiKey`        | `string` | Optional | API key if required           |
| `baseUrl`       | `string` | Required | API endpoint URL              |
| `headers`       | `object` | `{}`     | Custom HTTP headers           |
| `contextWindow` | `number` | `131072` | Default context window        |

### Usage in Agent

```yaml
---
models:
  - nova/gpt-oss-120b
---
```

### Common Use Cases

**vLLM Server:**

```json
{
  "providers": {
    "vllm": {
      "type": "openai-compatible",
      "baseUrl": "http://localhost:8000/v1"
    }
  }
}
```

**LM Studio:**

```json
{
  "providers": {
    "lmstudio": {
      "type": "openai-compatible",
      "baseUrl": "http://localhost:1234/v1"
    }
  }
}
```

**Azure OpenAI:**

```json
{
  "providers": {
    "azure": {
      "type": "openai-compatible",
      "apiKey": "${AZURE_OPENAI_KEY}",
      "baseUrl": "https://YOUR-RESOURCE.openai.azure.com/openai/deployments/YOUR-DEPLOYMENT/",
      "headers": {
        "api-version": "2024-02-15-preview"
      }
    }
  }
}
```

---

## Provider Configuration Reference

Complete provider configuration schema:

```json
{
  "providers": {
    "<name>": {
      "type": "openai | anthropic | google | openrouter | ollama | openai-compatible",
      "apiKey": "string",
      "baseUrl": "string",
      "headers": { "string": "string" },
      "contextWindow": "number",
      "tokenizer": "string",
      "models": {
        "<model>": {
          "contextWindow": "number",
          "tokenizer": "string",
          "interleaved": "boolean | string",
          "overrides": {
            "temperature": "number | null",
            "top_p": "number | null",
            "top_k": "number | null",
            "repeat_penalty": "number | null"
          }
        }
      },
      "toolsAllowed": ["string"],
      "toolsDenied": ["string"],
      "stringSchemaFormatsAllowed": ["string"],
      "stringSchemaFormatsDenied": ["string"]
    }
  }
}
```

---

## Per-Model Configuration

Override settings for specific models within a provider:

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}",
      "models": {
        "gpt-4o": {
          "contextWindow": 128000,
          "tokenizer": "tiktoken:gpt-4o"
        },
        "gpt-4o-mini": {
          "contextWindow": 128000,
          "overrides": {
            "temperature": 0.5
          }
        }
      }
    }
  }
}
```

### Model Settings Reference

| Property        | Type                | Description                         |
| --------------- | ------------------- | ----------------------------------- |
| `contextWindow` | `number`            | Maximum tokens for this model       |
| `tokenizer`     | `string`            | Tokenizer ID for context estimation |
| `interleaved`   | `boolean \| string` | Enable reasoning replay             |
| `overrides`     | `object`            | Force or disable LLM parameters     |

### Tokenizer Options

| Tokenizer         | Models                     |
| ----------------- | -------------------------- |
| `tiktoken:gpt-4o` | OpenAI GPT-4o, GPT-4o-mini |
| `tiktoken:gpt-4`  | OpenAI GPT-4               |
| `anthropic`       | Anthropic Claude models    |
| `google:gemini`   | Google Gemini models       |

---

## Model Overrides

Force specific parameter values or disable parameters entirely:

```json
{
  "overrides": {
    "temperature": 0.2,
    "top_p": null,
    "repeat_penalty": null
  }
}
```

### Override Behavior

| Value      | Behavior                        |
| ---------- | ------------------------------- |
| `<number>` | Force this exact value          |
| `null`     | Omit parameter from API request |

### Use Cases

**Disable top_p for models that don't support it:**

```json
{
  "models": {
    "o1": {
      "overrides": {
        "top_p": null,
        "temperature": null
      }
    }
  }
}
```

**Force low temperature for consistent output:**

```json
{
  "models": {
    "gpt-4o": {
      "overrides": {
        "temperature": 0.1
      }
    }
  }
}
```

---

## Interleaved Reasoning

For models that support extended thinking with reasoning replay:

```json
{
  "providers": {
    "nova": {
      "type": "openai-compatible",
      "baseUrl": "http://nova:8090/v1",
      "models": {
        "thinking-model": {
          "interleaved": true
        }
      }
    }
  }
}
```

### interleaved Options

| Value             | Behavior                               |
| ----------------- | -------------------------------------- |
| `true`            | Use standard `reasoning_content` field |
| `"custom_field"`  | Use specified field name for reasoning |
| `false` (default) | No reasoning replay                    |

---

## Fallback Chains

Models are tried in order. On failure, the next model is attempted:

```yaml
---
models:
  - openai/gpt-4o # Try first
  - anthropic/claude-3-haiku # Fallback 1
  - ollama/llama3 # Fallback 2
---
```

### Fallback Behavior

1. **Same model, next provider**: If multiple providers offer the same model
2. **Next model in list**: On failure, try the next model
3. **Final fallback**: Last model in list

### Multiple Providers for Same Model

```json
{
  "providers": {
    "openai": {
      "type": "openai",
      "apiKey": "${OPENAI_API_KEY}"
    },
    "openrouter": {
      "type": "openrouter",
      "apiKey": "${OPENROUTER_API_KEY}"
    }
  }
}
```

```yaml
---
models:
  - openai/gpt-4o # Primary
  - openrouter/openai/gpt-4o # Same model via OpenRouter
---
```

---

## Custom Headers

Add custom headers to provider requests:

```json
{
  "providers": {
    "custom": {
      "type": "openai-compatible",
      "apiKey": "${API_KEY}",
      "baseUrl": "https://api.example.com/v1",
      "headers": {
        "X-Custom-Header": "value",
        "X-Organization-ID": "${ORG_ID}"
      }
    }
  }
}
```

Headers support environment variable expansion.

---

## Troubleshooting

### Invalid API key

```
Error: 401 Unauthorized
```

**Causes**:

- API key not set or invalid
- Wrong API key for provider

**Solutions**:

1. Verify API key is set: `echo $OPENAI_API_KEY`
2. Check key format (OpenAI: `sk-...`, Anthropic: `sk-ant-...`)
3. Regenerate API key if expired

### Model not found

```
Error: Model 'gpt-5' not found
```

**Causes**:

- Model name typo
- Model not available on provider
- API access not enabled for model

**Solutions**:

1. Check exact model name in provider docs
2. Verify API access for the model
3. Use fallback models

### Rate limit exceeded

```
Error: 429 Too Many Requests
```

**Causes**:

- Too many requests to provider
- Account quota exceeded

**Solutions**:

1. Add fallback providers
2. Implement retry with backoff (built-in)
3. Check account usage limits

### Context window exceeded

```
Error: Context length exceeded
```

**Causes**:

- Input + output exceeds model limit
- Incorrect context window configuration

**Solutions**:

1. Configure correct `contextWindow` for model
2. Reduce input size
3. Use model with larger context

### Provider timeout

```
Error: Request timeout
```

**Causes**:

- Provider slow to respond
- Network issues
- Large response generation

**Solutions**:

1. Increase `llmTimeout` in config
2. Check provider status page
3. Use `--verbose` to see timing

---

## See Also

- [Configuration](Configuration) - Configuration overview
- [Context Window](Configuration-Context-Window) - Token budget management
- [Pricing](Configuration-Pricing) - Cost tracking
- [Fallback Chains](Agent-Files-Models) - Agent model configuration
