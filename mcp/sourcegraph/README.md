# sourcegraph-mcp-server

A minimal MCP (Model Context Protocol) server for searching code across public repositories using Sourcegraph's GraphQL API.

## Overview

This server provides a single `Search` tool that allows AI assistants to search code across millions of public repositories on Sourcegraph. It returns formatted markdown with context around matches, optimized for token efficiency.

Based on [crush.git's sourcegraph.go](https://github.com/anthropics/crush) implementation.

## Features

- **No authentication required** - Works with public sourcegraph.com
- **No external dependencies** - Pure Node.js, no npm install needed
- **Token-efficient output** - Markdown format with contextual snippets
- **Configurable context** - Control how many lines around each match
- **Timeout handling** - Configurable request timeout up to 120s

## Usage

```bash
# Run directly
node sourcegraph-mcp-server.js

# Or make executable and run
chmod +x sourcegraph-mcp-server.js
./sourcegraph-mcp-server.js
```

## Available Tool

### Search

Search code across public repositories using Sourcegraph's query syntax.

**Parameters:**

| Parameter | Type | Required | Default | Max | Description |
|-----------|------|----------|---------|-----|-------------|
| `query` | string | Yes | - | - | Sourcegraph search query |
| `count` | integer | No | 10 | 20 | Number of results to return |
| `context_window` | integer | No | 10 | - | Lines of context around each match |
| `timeout` | integer | No | 30 | 120 | Request timeout in seconds |

**Query Syntax Examples:**

```
"fmt.Println"                    # exact matches
"file:.go fmt.Println"           # limit to Go files
"repo:^github\.com/org/repo$"    # specific repos (regex)
"lang:go fmt.Println"            # limit by language
"fmt.Println AND log.Fatal"      # combined terms
"fmt\.(Print|Printf|Println)"    # regex patterns
"\"exact phrase\""               # exact phrase matching
"-file:test"                     # exclude matches
```

**Key Filters:**

| Filter Type | Examples |
|-------------|----------|
| Repository | `repo:name`, `repo:^exact$`, `-repo:exclude`, `fork:yes` |
| File | `file:\.js$`, `file:internal/`, `-file:test` |
| Content | `content:"exact"`, `-content:"unwanted"`, `case:yes` |
| Type | `type:symbol`, `type:file`, `type:path`, `type:diff`, `type:commit` |
| Time | `after:"1 month ago"`, `before:"2023-01-01"`, `author:name` |

**Output Format:**

```markdown
# Sourcegraph Search Results

Found 42 matches across 10 results

## Result 1: github.com/org/repo/src/main.go

URL: https://sourcegraph.com/github.com/org/repo/-/blob/src/main.go

` ` `
35| func setup() {
36|     config := loadConfig()
37|     logger := initLogger()
38|  func main() {              <- matched line
39|     app := NewApp()
40|     app.Run()
41|     defer cleanup()
` ` `
```

## Integration with ai-agent

Add to your agent configuration:

```json
{
  "mcp": {
    "sourcegraph": {
      "command": ["node", "/path/to/mcp/sourcegraph/sourcegraph-mcp-server.js"]
    }
  }
}
```

## Limitations

- Only searches public repositories (no private repo access)
- Rate limits may apply from Sourcegraph
- Max 20 results per query
- Complex queries may take longer

## Security

- No authentication tokens stored or transmitted
- No local filesystem access
- Read-only access to public code only

## Testing

```bash
# Test with a simple query
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"Search","arguments":{"query":"lang:go fmt.Println count:1"}}}' | node sourcegraph-mcp-server.js
```

## License

Same as ai-agent project.
