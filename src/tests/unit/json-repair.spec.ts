import { describe, expect, it } from 'vitest';

import { parseJsonRecordDetailed, parseJsonValueDetailed } from '../../utils.js';

describe('json repair pipeline', () => {
  it('repairs trailing comma via jsonrepair', () => {
    const input = '{"a":1,}';
    const result = parseJsonRecordDetailed(input);
    expect(result.value).toEqual({ a: 1 });
    expect(result.repairs).toContain('jsonrepair');
  });

  it('returns valid JSON untouched', () => {
    const result = parseJsonRecordDetailed('{"data":{"value":"ok"}}');
    expect(result.value).toEqual({ data: { value: 'ok' } });
    expect(result.repairs).toHaveLength(0);
  });

  it('repairs common JSON lint issues via jsonrepair', () => {
    const input = '{"data":{"value":"ok",},"extra":true}';
    const result = parseJsonRecordDetailed(input);
    expect(result.value).toEqual({ data: { value: 'ok' }, extra: true });
    expect(result.repairs).toContain('jsonrepair');
  });
});

describe('parseJsonValueDetailed', () => {
  it('returns original object without repairs', () => {
    const obj = { x: 1 };
    const result = parseJsonValueDetailed(obj);
    expect(result.value).toEqual(obj);
    expect(result.repairs).toEqual([]);
  });

  it('repairs backslash-newline (line continuation) in JSON strings', () => {
    // Model sometimes outputs backslash followed by literal newline (invalid JSON)
    // This should be converted to escaped newline \n
    const input = '{"text": "hello\\\nthere"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ text: 'hello\nthere' });
    expect(result.repairs).toContain('backslashNewlineFix');
  });

  it('repairs multiple backslash-newline occurrences', () => {
    const input = '{"a": "line1\\\nline2\\\nline3"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ a: 'line1\nline2\nline3' });
    expect(result.repairs).toContain('backslashNewlineFix');
  });

  it('does not modify JSON without backslash-newline', () => {
    const input = '{"text": "hello\\nthere"}';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ text: 'hello\nthere' });
    expect(result.repairs).not.toContain('backslashNewlineFix');
  });
});

describe('array extraction', () => {
  it('extracts array from markdown code fence', () => {
    const input = '```json\n[{"blocks": [{"type": "section"}]}]\n```';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual([{ blocks: [{ type: 'section' }] }]);
    // jsonrepair handles code fence extraction internally
    expect(result.repairs.length).toBeGreaterThan(0);
  });

  it('extracts array from text with leading non-JSON characters (preferArrayExtraction)', () => {
    // Use binary characters that jsonrepair can't interpret as JSON
    // preferArrayExtraction: true for slack-block-kit scenarios
    const input = '\x00\x01\x02[{"blocks": [{"type": "section"}]}]';
    const result = parseJsonValueDetailed(input, { preferArrayExtraction: true });
    // Should extract the array, not the inner object
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts array from XML wrapper (preferArrayExtraction)', () => {
    // preferArrayExtraction: true for slack-block-kit scenarios
    const input = '<ai-agent-final format="slack-block-kit">\n[{"blocks": [{"type": "section"}]}]\n</ai-agent-final>';
    const result = parseJsonValueDetailed(input, { preferArrayExtraction: true });
    // Should extract the array, not the inner object
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts nested array from malformed wrapper (preferArrayExtraction)', () => {
    // preferArrayExtraction: true for slack-block-kit scenarios
    const input = 'Response: [{"items": [1, 2, 3]}, {"items": [4, 5]}] end';
    const result = parseJsonValueDetailed(input, { preferArrayExtraction: true });
    // Should extract the full array with both elements
    expect(Array.isArray(result.value)).toBe(true);
    const arr = result.value as unknown[];
    expect(arr.length).toBe(2);
    expect(result.repairs).toContain('extractFirstArray');
  });

  it('extracts slack-block-kit array with nested blocks', () => {
    const input = '```json\n[\n  {\n    "blocks": [\n      {\n        "type": "header",\n        "text": {"type": "plain_text", "text": "Report"}\n      }\n    ]\n  }\n]\n```';
    const result = parseJsonValueDetailed(input);
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
  });

  it('returns valid array untouched', () => {
    const input = '[{"a": 1}, {"b": 2}]';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual([{ a: 1 }, { b: 2 }]);
    expect(result.repairs).toHaveLength(0);
  });

  it('extracts object when both array and object are present (default)', () => {
    // By default, object extraction runs first for backwards compatibility
    const input = 'Prefix [1, 2, 3] then {"key": "value"} suffix';
    const result = parseJsonValueDetailed(input);
    expect(result.value).toEqual({ key: 'value' });
  });

  it('extracts array when preferArrayExtraction is true', () => {
    // With preferArrayExtraction, array extraction runs first (for slack-block-kit)
    const input = 'Prefix [1, 2, 3] then {"key": "value"} suffix';
    const result = parseJsonValueDetailed(input, { preferArrayExtraction: true });
    expect(Array.isArray(result.value)).toBe(true);
    expect(result.value).toEqual([1, 2, 3]);
  });

  it('extracts array from anonymized snapshot payload (wrapper + array)', () => {
    const input = "<ai-agent-49e50b35-FINAL format=\"slack-block-kit\">\n[\n  {\n    \"blocks\": [\n      {\n        \"type\": \"header\",\n        \"text\": {\n          \"type\": \"plain_text\",\n          \"text\": \"\\ud83c\\udf93 Example Institute Contact Form Analysis - EXISTING CUSTOMER\"\n        }\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*Good afternoon!* I've completed a comprehensive analysis of the contact form submission from Alex Doe at Example Institute.\"\n        }\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*\\ud83d\\udea8 CRITICAL FINDING: Example Institute is already an active paying customer since March 2024*\\n\\n\\u2022 *Active Business Annual Plan*: $3,780 ARR (150 nodes, 30% EDU discount)\\n\\u2022 *Primary Contact*: Jamie Roe (user@example.edu) - IT Director\\n\\u2022 *Published Case Study*: 50% cost reduction achieved\\n\\u2022 *Today's Form*: Alex Doe (Associate Professor) submitted sales inquiry at 12:15 UTC\"\n        }\n      },\n      {\n        \"type\": \"divider\"\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*\\ud83d\\udcca ICP Assessment*\"\n        }\n      },\n      {\n        \"type\": \"section\",\n        \"fields\": [\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*ICP Score*\\n45/100 (MODERATE FIT)\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Confidence*\\n95%\"\n          }\n        ]\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*ICP Breakdown:*\\n\\u2022 *Firmographic (15/40)*: Small academic institution (~100 personnel), $15M budget, education sector\\n\\u2022 *Technographic (20/25)*: Sophisticated HPC infrastructure (47 nodes, 198 GPUs, Beehive cluster), Docker/Python stack, active monitoring deployment\\n\\u2022 *Buying Signals (5/20)*: No hiring signals, no recent funding, but active product usage\\n\\u2022 *Persona Fit (5/15)*: Associate Professor (not primary buyer), existing IT Director relationship\"\n        }\n      },\n      {\n        \"type\": \"divider\"\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*\\ud83c\\udfe2 Organization Profile: Example Research Institute*\"\n        }\n      },\n      {\n        \"type\": \"section\",\n        \"fields\": [\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Type*\\nPhilanthropically endowed graduate CS research institute\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Size*\\n~100 personnel (23 faculty, 39 PhD students, 12 admin)\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Endowment*\\n$205-255M\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Annual Budget*\\n$15.3M operating\"\n          }\n        ]\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*Infrastructure:*\\n\\u2022 *Beehive HPC Cluster*: 47 nodes, 1,176 CPU cores, 198 NVIDIA GPUs (6 generations)\\n\\u2022 *Scheduler*: Slurm 22.05.08\\n\\u2022 *Deployment*: 100% on-premises (Example University data center)\\n\\u2022 *Monitoring*: Netdata (deployed via Ansible)\\n\\u2022 *Stack*: Python/PyTorch, Docker, CUDA 13.0\"\n          }\n        }\n      },\n      {\n        \"type\": \"divider\"\n      },\n      {\n        \"type\": \"section\",\n        \"text\": {\n          \"type\": \"mrkdwn\",\n          \"text\": \"*\\ud83d\\udc64 Contact Profile: Alex R. Doe*\"\n        }\n      },\n      {\n        \"type\": \"section\",\n        \"fields\": [\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Title*\\nAssociate Professor\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Role*\\nDirector, Robot Intelligence through Perception Lab (RIPL)\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Research*\\nRobotics, AI, Computer Vision\"\n          },\n          {\n            \"type\": \"mrkdwn\",\n            \"text\": \"*Citations*\\n10,096 (Google Scholar)\"\n          }\n        ]\n      }\n    ]\n  }\n]\n</ai-agent-49e50b35-FINAL>";
    const result = parseJsonValueDetailed(input, { preferArrayExtraction: true });
    expect(Array.isArray(result.value)).toBe(true);
    expect((result.value as unknown[])[0]).toHaveProperty('blocks');
    expect(result.repairs).toContain('extractFirstArray');
  });
});
