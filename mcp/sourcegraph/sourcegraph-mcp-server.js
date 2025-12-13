#!/usr/bin/env node
/**
 * Sourcegraph MCP stdio server (no external deps).
 *
 * Usage: node sourcegraph-mcp-server.js
 *
 * Tools:
 * - Search(query, count?, context_window?, timeout?): Search code across public repositories
 *   using Sourcegraph's GraphQL API. Returns formatted markdown with context around matches.
 *
 * Based on crush.git's sourcegraph.go implementation.
 * No authentication required for public repositories.
 */

import https from 'node:https';

// -------------------------- Utility: JSON-RPC over LSP framing --------------------------
const STDIN = process.stdin;
const STDOUT = process.stdout;

function send(message) {
  const json = JSON.stringify(message);
  // MCP SDK uses newline-delimited JSON, not LSP framing
  const payload = json + '\n';
  STDOUT.write(payload);
}

function respond(id, result) { send({ jsonrpc: '2.0', id, result }); }
function respondError(id, code, message, data) {
  const error = data === undefined ? { code, message } : { code, message, data };
  send({ jsonrpc: '2.0', id, error });
}

// -------------------------- Server Instructions --------------------------
const SERVER_INSTRUCTIONS = `Sourcegraph Code Search - Search code across millions of public repositories.

QUERY SYNTAX:
- Basic: "functionName" or "error handling"
- Language: lang:go, lang:python, lang:typescript, lang:rust, lang:java
- File type: file:.go, file:.py, file:.ts, file:Dockerfile, file:.yaml
- Repository: repo:org/name, repo:^github\\.com/kubernetes/, -repo:forks
- Path: file:src/, file:internal/, -file:test, -file:vendor
- Content: content:"exact phrase", -content:deprecated
- Type: type:symbol (definitions), type:file (filenames), type:commit

OPERATORS:
- AND: term1 AND term2 (both required)
- OR: term1 OR term2 (either matches)
- NOT: term1 NOT term2 (exclude)
- Grouping: (term1 OR term2) AND term3
- Negation: -file:test, -repo:archived

COMMON PATTERNS:
- Find function definitions: "func.*Name" lang:go type:symbol
- Find config files: file:config repo:org/project
- Find imports/dependencies: "import.*package" lang:python
- Find error handling: "catch|except|error" lang:typescript
- Find TODOs: "TODO|FIXME|HACK" file:.go

TIPS:
- Use lang: filter to reduce noise
- Add repo: filter for targeted searches
- Use type:symbol for function/class definitions
- Combine filters: lang:go file:.go -file:test repo:kubernetes`;

// -------------------------- GraphQL Query --------------------------
const GRAPHQL_QUERY = `query Search($query: String!) {
  search(query: $query, version: V2, patternType: keyword) {
    results {
      matchCount
      limitHit
      resultCount
      approximateResultCount
      missing { name }
      timedout { name }
      indexUnavailable
      results {
        __typename
        ... on FileMatch {
          repository { name }
          file { path, url, content }
          lineMatches { preview, lineNumber, offsetAndLengths }
        }
      }
    }
  }
}`;

// -------------------------- Sourcegraph API --------------------------
async function searchSourcegraph(query, count, contextWindow, timeoutSeconds) {
  const requestBody = JSON.stringify({
    query: GRAPHQL_QUERY,
    variables: { query }
  });

  return new Promise((resolve, reject) => {
    const timeoutMs = timeoutSeconds * 1000;

    const options = {
      hostname: 'sourcegraph.com',
      path: '/.api/graphql',
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(requestBody),
        'User-Agent': 'ai-agent-mcp/1.0'
      }
    };

    const req = https.request(options, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode !== 200) {
          reject(new Error(`HTTP ${res.statusCode}: ${data.substring(0, 200)}`));
          return;
        }
        try {
          const result = JSON.parse(data);
          resolve(formatSourcegraphResults(result, count, contextWindow));
        } catch (e) {
          reject(new Error(`Failed to parse response: ${e.message}`));
        }
      });
    });

    req.on('error', (e) => reject(new Error(`Request failed: ${e.message}`)));

    // Timeout handling
    req.setTimeout(timeoutMs, () => {
      req.destroy();
      reject(new Error(`Request timed out after ${timeoutSeconds}s`));
    });

    req.write(requestBody);
    req.end();
  });
}

// -------------------------- Result Formatting (ported from crush.git) --------------------------
function formatSourcegraphResults(result, maxCount, contextWindow) {
  const lines = [];

  // Handle API errors
  if (result.errors && Array.isArray(result.errors) && result.errors.length > 0) {
    lines.push('## Sourcegraph API Error\n');
    for (const err of result.errors) {
      if (err.message) {
        lines.push(`- ${err.message}`);
      }
    }
    return lines.join('\n');
  }

  const data = result.data;
  if (!data || !data.search || !data.search.results) {
    return 'Error: Invalid response format from Sourcegraph API';
  }

  const searchResults = data.search.results;
  const matchCount = searchResults.matchCount || 0;
  const resultCount = searchResults.resultCount || 0;
  const limitHit = searchResults.limitHit || false;

  lines.push('# Sourcegraph Search Results\n');
  lines.push(`Found ${matchCount} matches across ${resultCount} results`);

  if (limitHit) {
    lines.push('(Result limit reached, try a more specific query)');
  }

  lines.push('');

  const results = searchResults.results;
  if (!results || results.length === 0) {
    lines.push('No results found. Try a different query.');
    return lines.join('\n');
  }

  // Limit to maxCount results (default 10, max 20)
  const displayResults = results.slice(0, Math.min(maxCount, 10));

  for (let i = 0; i < displayResults.length; i++) {
    const fileMatch = displayResults[i];

    if (fileMatch.__typename !== 'FileMatch') {
      continue;
    }

    const repo = fileMatch.repository;
    const file = fileMatch.file;
    const lineMatches = fileMatch.lineMatches;

    if (!repo || !file) {
      continue;
    }

    const repoName = repo.name || '';
    const filePath = file.path || '';
    const fileURL = file.url || '';
    const fileContent = file.content || '';

    lines.push(`## Result ${i + 1}: ${repoName}/${filePath}\n`);

    if (fileURL) {
      lines.push(`URL: https://sourcegraph.com${fileURL}\n`);
    }

    if (lineMatches && lineMatches.length > 0) {
      for (const lm of lineMatches) {
        const lineNumber = lm.lineNumber || 0; // 0-indexed from API
        const preview = lm.preview || '';

        if (fileContent) {
          const contentLines = fileContent.split('\n');

          lines.push('```');

          // Lines before match
          const startLine = Math.max(1, lineNumber - contextWindow);
          for (let j = startLine - 1; j < lineNumber && j < contentLines.length; j++) {
            if (j >= 0) {
              lines.push(`${j + 1}| ${contentLines[j]}`);
            }
          }

          // The matched line (from preview, with extra indent to highlight)
          lines.push(`${lineNumber + 1}|  ${preview}`);

          // Lines after match
          const endLine = lineNumber + contextWindow;
          for (let j = lineNumber + 1; j <= endLine && j < contentLines.length; j++) {
            lines.push(`${j + 1}| ${contentLines[j]}`);
          }

          lines.push('```\n');
        } else {
          // No file content, just show the preview
          lines.push('```');
          lines.push(`${lineNumber + 1}| ${preview}`);
          lines.push('```\n');
        }
      }
    }
  }

  return lines.join('\n');
}

// -------------------------- Tool Implementation --------------------------
async function toolSearch(args) {
  const { query, count, context_window, timeout } = args;

  if (typeof query !== 'string' || query.trim() === '') {
    throw new Error('query parameter is required and must be a non-empty string');
  }

  // Apply defaults and limits (matching crush.git)
  let resultCount = 10;
  if (typeof count === 'number' && count > 0) {
    resultCount = Math.min(count, 20);
  }

  let contextWindow = 10;
  if (typeof context_window === 'number' && context_window >= 0) {
    contextWindow = context_window;
  }

  let timeoutSeconds = 30;
  if (typeof timeout === 'number' && timeout > 0) {
    timeoutSeconds = Math.min(timeout, 120);
  }

  return await searchSourcegraph(query, resultCount, contextWindow, timeoutSeconds);
}

// -------------------------- MCP Tool Definition --------------------------
const tools = [
  {
    name: 'Search',
    description: `Search code across public repositories using Sourcegraph's GraphQL API.

<usage>
- Provide search query using Sourcegraph syntax
- Optional result count (default: 10, max: 20)
- Optional context window (default: 10 lines around each match)
- Optional timeout for request (default: 30s, max: 120s)
</usage>

<basic_syntax>
- "fmt.Println" - exact matches
- "file:.go fmt.Println" - limit to Go files
- "repo:^github\\.com/golang/go$ fmt.Println" - specific repos
- "lang:go fmt.Println" - limit to Go code
- "fmt.Println AND log.Fatal" - combined terms
- "fmt\\.(Print|Printf|Println)" - regex patterns
- "\\"exact phrase\\"" - exact phrase matching
- "-file:test" or "-repo:forks" - exclude matches
</basic_syntax>

<key_filters>
Repository: repo:name, repo:^exact$, repo:org/repo@branch, -repo:exclude, fork:yes, archived:yes, visibility:public
File: file:\\.js$, file:internal/, -file:test, file:has.content(text)
Content: content:"exact", -content:"unwanted", case:yes
Type: type:symbol, type:file, type:path, type:diff, type:commit
Time: after:"1 month ago", before:"2023-01-01", author:name, message:"fix"
Result: select:repo, select:file, select:content, count:100, timeout:30s
</key_filters>

<examples>
- "file:.go context.WithTimeout" - Go code using context.WithTimeout
- "lang:typescript useState type:symbol" - TypeScript React useState hooks
- "repo:^github\\.com/kubernetes/kubernetes$ pod list type:file" - Kubernetes pod files
- "file:Dockerfile (alpine OR ubuntu) -content:alpine:latest" - Dockerfiles with base images
</examples>

<limitations>
- Only searches public repositories
- Rate limits may apply
- Complex queries take longer
- Max 20 results per query
</limitations>`,
    inputSchema: {
      type: 'object',
      properties: {
        query: { type: 'string', description: 'Sourcegraph search query using their query syntax' },
        count: { type: 'integer', minimum: 1, maximum: 20, description: 'Number of results to return (default: 10, max: 20)' },
        context_window: { type: 'integer', minimum: 0, description: 'Lines of context around each match (default: 10)' },
        timeout: { type: 'integer', minimum: 1, maximum: 120, description: 'Request timeout in seconds (default: 30, max: 120)' }
      },
      required: ['query']
    },
    handler: toolSearch
  }
];

function listToolsResponse() {
  return {
    tools: tools.map((t) => ({
      name: t.name,
      description: t.description,
      inputSchema: t.inputSchema
    }))
  };
}

// -------------------------- Message loop (copied from fs-mcp-server.js) --------------------------
let buffer = Buffer.alloc(0);

STDIN.on('data', (chunk) => {
  buffer = Buffer.concat([buffer, chunk]);
  processBuffer();
});

STDIN.on('end', () => {
  processBuffer();
  process.exit(0);
});

function processNewlineJSON() {
  while (true) {
    const nl = buffer.indexOf(0x0A); // '\n'
    if (nl === -1) break;

    const lineBuf = buffer.subarray(0, nl);
    buffer = buffer.subarray(nl + 1);

    let lineStr = lineBuf.toString('utf8');
    if (lineStr.endsWith('\r')) {
      lineStr = lineStr.slice(0, -1);
    }

    if (lineStr.length === 0) continue;

    try {
      const msg = JSON.parse(lineStr);
      handleMessage(msg);
    } catch (e) {
      console.error('MCP: Failed to parse NDJSON line:', e.message, 'Line:', lineStr.slice(0, 200));
    }
  }
}

function processBuffer() {
  const bufStr = buffer.toString('utf8');

  if (bufStr.match(/^Content-Length:/i) || bufStr.includes('\r\n\r\n') || bufStr.includes('\n\n')) {
    // Process as LSP-framed messages (handled below)
  } else if (buffer.indexOf(0x0A) !== -1) {
    processNewlineJSON();
    return;
  } else {
    return;
  }

  while (true) {
    const headerEnd = buffer.indexOf('\r\n\r\n');
    if (headerEnd === -1) {
      const headerEndLF = buffer.indexOf('\n\n');
      if (headerEndLF === -1) {
        if (buffer.length > 0 && buffer.length < 8192) {
          const preview = buffer.subarray(0, Math.min(100, buffer.length)).toString('utf8').replace(/[\r\n]/g, '\\n');
          console.error('MCP: Waiting for header, buffer has:', buffer.length, 'bytes, preview:', preview);
        }
        return;
      }
      const header = buffer.subarray(0, headerEndLF).toString('utf8');
      const match = /Content-Length:\s*(\d+)/i.exec(header);
      if (!match) {
        console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
        buffer = buffer.subarray(headerEndLF + 2);
        continue;
      }
      const length = parseInt(match[1], 10);
      const total = headerEndLF + 2 + length;
      if (buffer.length < total) return;
      const body = buffer.subarray(headerEndLF + 2, total).toString('utf8');
      buffer = buffer.subarray(total);
      let msg;
      try {
        msg = JSON.parse(body);
      } catch (e) {
        console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
        continue;
      }
      handleMessage(msg);
      continue;
    }
    const sepLen = 4;

    const header = buffer.subarray(0, headerEnd).toString('utf8');
    const match = /Content-Length:\s*(\d+)/i.exec(header);
    if (!match) {
      console.error('MCP: Invalid header (no Content-Length):', header.replace(/[\r\n]/g, '\\n'));
      buffer = buffer.subarray(headerEnd + sepLen);
      continue;
    }
    const length = parseInt(match[1], 10);
    const total = headerEnd + sepLen + length;
    if (buffer.length < total) return;
    const body = buffer.subarray(headerEnd + sepLen, total).toString('utf8');
    buffer = buffer.subarray(total);
    let msg;
    try {
      msg = JSON.parse(body);
    } catch (e) {
      console.error('MCP: Failed to parse JSON:', e.message, 'Body:', body.substring(0, 200));
      continue;
    }
    handleMessage(msg);
  }
}

function handleMessage(msg) {
  const { id, method, params } = msg;

  if (method === 'initialize') {
    const result = {
      protocolVersion: '2024-11-05',
      serverInfo: { name: 'sourcegraph-mcp', version: '1.0.0' },
      capabilities: { tools: {} },
      instructions: SERVER_INSTRUCTIONS
    };
    respond(id, result);
    return;
  }

  if (method === 'tools/list') {
    respond(id, listToolsResponse());
    return;
  }

  if (method === 'tools/call') {
    const { name, arguments: args } = params || {};
    const tool = tools.find((t) => t.name === name);
    if (!tool) {
      respondError(id, -32601, `Unknown tool: ${name}`);
      return;
    }
    (async () => {
      try {
        const result = await tool.handler(args || {});
        const text = typeof result === 'string' ? result : JSON.stringify(result, null, 2);
        respond(id, { content: [{ type: 'text', text }] });
      } catch (e) {
        const message = e && typeof e.message === 'string' ? e.message : 'Tool error';
        respondError(id, -32000, message);
      }
    })();
    return;
  }

  if (method === 'notifications/initialized') return;
  if (method === 'notifications/cancelled') return;
  if (method === 'prompts/list') { respond(id, { prompts: [] }); return; }
  if (method === 'prompts/get') { respond(id, { prompt: null }); return; }
  if (method === 'resources/list') { respond(id, { resources: [] }); return; }
  if (method === 'resources/read') { respond(id, { contents: [] }); return; }
  if (method === 'logging/setLevel') { respond(id, {}); return; }
  if (method === 'ping') { respond(id, {}); return; }

  console.error('MCP: Unknown method:', String(method), 'id:', id, 'params:', JSON.stringify(params));
  if (id !== undefined) respondError(id, -32601, `Unknown method: ${String(method)}`);
}

STDIN.resume();
