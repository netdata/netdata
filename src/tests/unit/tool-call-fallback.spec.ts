import { describe, expect, it } from 'vitest';

import { tryExtractLeakedToolCalls } from '../../tool-call-fallback.js';

// Common test inputs to avoid duplication
const SIMPLE_TOOLS_INPUT = '<tools>{"name": "test", "arguments": {}}</tools>';
const TASK_STATUS_ALLOWED = new Set(['agent__task_status']);

describe('tryExtractLeakedToolCalls', () => {
  describe('no patterns found', () => {
    it('returns input unchanged when no XML patterns present', () => {
      const input = 'Just some regular text content';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe(input);
      expect(result.toolCalls).toEqual([]);
    });

    it('returns input unchanged for empty string', () => {
      const input = '';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe(input);
      expect(result.toolCalls).toEqual([]);
    });

    it('handles non-matching XML-like content', () => {
      const input = '<div>Hello</div><span>World</span>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe(input);
      expect(result.toolCalls).toEqual([]);
    });
  });

  describe('<tool_call> pattern', () => {
    it('extracts single tool call', () => {
      const input = '<tool_call>\n{"name": "my_tool", "arguments": {"foo": "bar"}}\n</tool_call>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('my_tool');
      expect(result.toolCalls[0].parameters).toEqual({ foo: 'bar' });
      expect(result.toolCalls[0].id).toBeDefined();
    });

    it('extracts tool call with surrounding text', () => {
      const input = 'Before text\n<tool_call>{"name": "test", "arguments": {}}</tool_call>\nAfter text';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe('Before text\n\nAfter text');
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('test');
    });
  });

  describe('<tool_calls> pattern (Hermes format)', () => {
    it('extracts single tool call', () => {
      const input = '<tool_calls>{"name": "hermes_tool", "arguments": {"x": 1}}</tool_calls>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('hermes_tool');
      expect(result.toolCalls[0].parameters).toEqual({ x: 1 });
    });

    it('extracts array of tool calls', () => {
      const input = '<tool_calls>[{"name": "tool1", "arguments": {}}, {"name": "tool2", "arguments": {}}]</tool_calls>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(2);
      expect(result.toolCalls[0].name).toBe('tool1');
      expect(result.toolCalls[1].name).toBe('tool2');
    });
  });

  describe('<tools> pattern', () => {
    it('extracts single tool', () => {
      const input = '<tools>{"name": "bigquery__execute_sql", "arguments": {"sql": "SELECT 1"}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('bigquery__execute_sql');
      expect(result.toolCalls[0].parameters).toEqual({ sql: 'SELECT 1' });
    });

    it('extracts multiple <tools> tags', () => {
      const input = `
<tools>{"name": "tool1", "arguments": {"a": 1}}</tools>

<tools>{"name": "tool2", "arguments": {"b": 2}}</tools>
`;
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(2);
      expect(result.toolCalls[0].name).toBe('tool1');
      expect(result.toolCalls[1].name).toBe('tool2');
    });
  });

  describe('<function_call> pattern', () => {
    it('extracts function call', () => {
      const input = '<function_call>{"name": "my_function", "arguments": {"arg": "value"}}</function_call>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('my_function');
    });
  });

  describe('<function> pattern', () => {
    it('extracts function', () => {
      const input = '<function>{"name": "old_style_func", "arguments": {}}</function>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('old_style_func');
    });
  });

  describe('field normalization', () => {
    it('normalizes "function" field to "name"', () => {
      const input = '<tools>{"function": "my_func", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('my_func');
    });

    it('normalizes "tool" field to "name"', () => {
      const input = '<tools>{"tool": "my_tool", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('my_tool');
    });

    it('normalizes "parameters" field (already correct for ToolCall)', () => {
      const input = '<tools>{"name": "test", "parameters": {"key": "value"}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].parameters).toEqual({ key: 'value' });
    });

    it('normalizes "arguments" field to "parameters"', () => {
      const input = '<tools>{"name": "test", "arguments": {"key": "value"}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].parameters).toEqual({ key: 'value' });
    });

    it('prefers "name" over "function" when both present', () => {
      const input = '<tools>{"name": "correct", "function": "wrong", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls[0].name).toBe('correct');
    });

    it('prefers "parameters" over "arguments" when both present', () => {
      const input = '<tools>{"name": "test", "parameters": {"p": 1}, "arguments": {"a": 2}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls[0].parameters).toEqual({ p: 1 });
    });
  });

  describe('batch format (agent__batch)', () => {
    it('extracts batch tool call as single tool', () => {
      const input = `<tool_call>
{"name": "agent__batch", "arguments": {"calls": [{"id": "1", "tool": "bigquery__execute_sql", "parameters": {"sql": "SELECT 1"}}]}}
</tool_call>`;
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('agent__batch');
      expect(result.toolCalls[0].parameters).toHaveProperty('calls');
    });
  });

  describe('invalid JSON handling', () => {
    it('strips invalid XML tags but returns empty toolCalls', () => {
      const input = 'Before\n<tools>not valid json at all</tools>\nAfter';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe('Before\n\nAfter');
      expect(result.toolCalls).toEqual([]);
    });

    it('handles partial valid JSON - extracts what it can', () => {
      const input = '<tools>{"name": "valid", "arguments": {}}</tools>\n<tools>invalid</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('valid');
    });

    it('handles missing name field - skips invalid', () => {
      const input = '<tools>{"arguments": {"foo": "bar"}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toEqual([]);
    });

    it('handles empty name field - skips invalid', () => {
      const input = '<tools>{"name": "", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toEqual([]);
    });
  });

  describe('mixed content', () => {
    it('handles text before and after', () => {
      const input = 'Thinking about what to do...\n\n<tools>{"name": "search", "arguments": {"q": "test"}}</tools>\n\nNow waiting for results.';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe('Thinking about what to do...\n\n\n\nNow waiting for results.');
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('search');
    });

    it('handles multiple different tag types in same content', () => {
      const input = '<tools>{"name": "tool1", "arguments": {}}</tools>\n<function>{"name": "func1", "arguments": {}}</function>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(2);
      expect(result.toolCalls[0].name).toBe('tool1');
      expect(result.toolCalls[1].name).toBe('func1');
    });
  });

  describe('real-world examples from production trace', () => {
    it('extracts agent__batch call from <tool_call>', () => {
      const input = `

<tool_call>
{"name": "agent__batch", "arguments": {"calls": [{"id": "1", "tool": "bigquery__execute_sql", "parameters": {"sql": "SELECT MAX(bd_data_ingested_at) AS last_ingested_at FROM \`netdata-analytics-bi.watch_towers.spaces_latest\`"}}, {"id": "2", "tool": "bigquery__execute_sql", "parameters": {"sql": "SELECT COUNT(*) AS new_users FROM \`netdata-analytics-bi.app_db_replication.account_accounts_latest\` WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)"}}]}}
</tool_call>`;
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('agent__batch');
      const calls = result.toolCalls[0].parameters.calls as unknown[];
      expect(calls).toHaveLength(2);
    });

    it('extracts multiple <tools> tags from production trace', () => {
      const input = `

<tools>
{"name": "bigquery__execute_sql", "arguments": {"sql": "SELECT MAX(bd_data_ingested_at) AS last_ingested_at FROM \`netdata-analytics-bi.watch_towers.spaces_latest\`"}}
</tools>

<tools>
{"name": "bigquery__execute_sql", "arguments": {"sql": "SELECT COUNT(*) AS new_users FROM \`netdata-analytics-bi.app_db_replication.account_accounts_latest\`"}}
</tools>`;
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(2);
      expect(result.toolCalls[0].name).toBe('bigquery__execute_sql');
      expect(result.toolCalls[1].name).toBe('bigquery__execute_sql');
    });
  });

  describe('tool-name tag format (minimax)', () => {
    it('extracts agent__task_status with parameters and bare tags', () => {
      const input = `
Intro text

<agent__task_status>
<parameter name="status">in-progress</parameter>
<parameter name="done">Researching MS SQL Server monitoring configuration</parameter>
<pending>Verifying windows authentication configuration option</pending>
<pending>Checking collect lock metrics configuration flags</pending>
<now>Searching documentation and source code for MS SQL Server configuration</now>
<ready_for_final_report">false</ready_for_final_report>
<parameter name="need_to_run_more_tools">true</need_to_run_more_tools>
</agent__task_status>
`;
      const result = tryExtractLeakedToolCalls(input, { allowedToolNames: TASK_STATUS_ALLOWED });
      expect(result.content).toBe('Intro text');
      expect(result.toolCalls).toHaveLength(1);
      const params = result.toolCalls[0].parameters;
      expect(params.status).toBe('in-progress');
      expect(params.done).toBe('Researching MS SQL Server monitoring configuration');
      expect(params.pending).toBe('Verifying windows authentication configuration option\nChecking collect lock metrics configuration flags');
      expect(params.now).toBe('Searching documentation and source code for MS SQL Server configuration');
      expect(params.ready_for_final_report).toBe(false);
      expect(params.need_to_run_more_tools).toBe(true);
    });

    it('ignores tool-name tags that are not allowed', () => {
      const input = '<agent__task_status><parameter name="status">in-progress</parameter></agent__task_status>';
      const result = tryExtractLeakedToolCalls(input, { allowedToolNames: new Set(['other_tool']) });
      expect(result.content).toBe(input);
      expect(result.toolCalls).toEqual([]);
    });
  });

  describe('case insensitivity', () => {
    it('handles uppercase tags', () => {
      const input = '<TOOLS>{"name": "test", "arguments": {}}</TOOLS>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('test');
    });

    it('handles mixed case tags', () => {
      const input = '<Tool_Call>{"name": "test", "arguments": {}}</tool_call>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].name).toBe('test');
    });
  });

  describe('ID generation', () => {
    it('generates unique IDs for each tool call', () => {
      const input = '<tools>{"name": "t1", "arguments": {}}</tools><tools>{"name": "t2", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(2);
      expect(result.toolCalls[0].id).toBeDefined();
      expect(result.toolCalls[1].id).toBeDefined();
      expect(result.toolCalls[0].id).not.toBe(result.toolCalls[1].id);
    });

    it('generates valid UUID format', () => {
      const result = tryExtractLeakedToolCalls(SIMPLE_TOOLS_INPUT);
      const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
      expect(result.toolCalls[0].id).toMatch(uuidRegex);
    });
  });

  describe('edge cases', () => {
    it('handles unclosed tags - ignores them', () => {
      const input = '<tools>{"name": "test", "arguments": {}}';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBe(input);
      expect(result.toolCalls).toEqual([]);
    });

    it('handles nested-looking content in JSON', () => {
      const input = '<tools>{"name": "test", "arguments": {"html": "<div>content</div>"}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      expect(result.toolCalls[0].parameters).toEqual({ html: '<div>content</div>' });
    });

    it('handles whitespace-only content after extraction', () => {
      const input = '   \n\n<tools>{"name": "test", "arguments": {}}</tools>\n\n   ';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.content).toBeNull();
      expect(result.toolCalls).toHaveLength(1);
    });

    it('handles tool name sanitization', () => {
      const input = '<tools>{"name": "some-weird:tool.name", "arguments": {}}</tools>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.toolCalls).toHaveLength(1);
      // sanitizeToolName should clean the name
      expect(result.toolCalls[0].name).toBeDefined();
    });
  });

  describe('patternsMatched tracking', () => {
    it('returns empty array when no patterns found', () => {
      const input = 'Just some regular text';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.patternsMatched).toEqual([]);
    });

    it('returns single pattern name when one pattern matched', () => {
      const result = tryExtractLeakedToolCalls(SIMPLE_TOOLS_INPUT);
      expect(result.patternsMatched).toEqual(['tools']);
    });

    it('returns multiple pattern names when different patterns matched', () => {
      const input = '<tools>{"name": "tool1", "arguments": {}}</tools>\n<function>{"name": "func1", "arguments": {}}</function>';
      const result = tryExtractLeakedToolCalls(input);
      expect(result.patternsMatched).toContain('tools');
      expect(result.patternsMatched).toContain('function');
      expect(result.patternsMatched).toHaveLength(2);
    });

    it('returns pattern name even when JSON is invalid', () => {
      const input = '<tools>not valid json</tools>';
      const result = tryExtractLeakedToolCalls(input);
      // Pattern was matched (tag stripped) but no tools extracted
      expect(result.patternsMatched).toEqual(['tools']);
      expect(result.toolCalls).toEqual([]);
    });
  });
});
