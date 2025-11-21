import fs from 'node:fs';
import path from 'node:path';

import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, ErrorCode, type CallToolResult, McpError } from '@modelcontextprotocol/sdk/types.js';

const server = new McpServer({
  name: 'test-mcp-server',
  version: '0.2.0',
});

const noopResult: CallToolResult = {
  content: [
    {
      type: 'text',
      text: 'noop',
    },
  ],
};

server.registerTool(
  'test',
  {
    description: 'Echo helper used for deterministic tests.',
    inputSchema: {},
  },
  async () => noopResult,
);

server.registerTool(
  'test-summary',
  {
    description: 'Summarises values for deterministic tests.',
    inputSchema: {},
  },
  async () => noopResult,
);

const longPayload = '#'.repeat(3200);
const guardOverflowPayload = '@'.repeat(1200);

const fixtureMode = process.env.MCP_FIXTURE_MODE ?? '';
const fixtureStateFile = process.env.MCP_FIXTURE_STATE_FILE;

interface FixtureState {
  phase: string;
  failOnStart?: boolean;
  remainingInitFails?: number;
  remainingRestartFails?: number;
}

const parsePositiveInt = (value: string | undefined): number | undefined => {
  if (value === undefined) return undefined;
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined;
  return Math.trunc(parsed);
};

const ensureFixtureFile = (): void => {
  if (fixtureStateFile === undefined) return;
  const dir = path.dirname(fixtureStateFile);
  fs.mkdirSync(dir, { recursive: true });
  if (!fs.existsSync(fixtureStateFile)) {
    const initial: FixtureState = { phase: 'initial' };
    const initFails = parsePositiveInt(process.env.MCP_FIXTURE_FAIL_INIT_ATTEMPTS);
    const restartFails = parsePositiveInt(process.env.MCP_FIXTURE_FAIL_RESTART_ATTEMPTS);
    if (initFails !== undefined) {
      initial.remainingInitFails = initFails;
    }
    if (restartFails !== undefined) {
      initial.remainingRestartFails = restartFails;
    }
    fs.writeFileSync(fixtureStateFile, JSON.stringify(initial), 'utf8');
  }
};

const readFixtureState = (): FixtureState | undefined => {
  if (fixtureStateFile === undefined) return undefined;
  ensureFixtureFile();
  try {
    const raw = fs.readFileSync(fixtureStateFile, 'utf8');
    return JSON.parse(raw) as FixtureState;
  } catch {
    return { phase: 'initial' };
  }
};

const writeFixtureState = (state: FixtureState): void => {
  if (fixtureStateFile === undefined) return;
  ensureFixtureFile();
  fs.writeFileSync(fixtureStateFile, JSON.stringify(state), 'utf8');
};

const updateFixtureState = (updater: (prev: FixtureState) => FixtureState): void => {
  const prev = readFixtureState() ?? { phase: 'initial' };
  const next = updater(prev);
  writeFixtureState(next);
};

const decrementStateCounter = (key: 'remainingInitFails' | 'remainingRestartFails'): void => {
  updateFixtureState((prev) => {
    const nextValue = Math.max(0, (prev[key] ?? 0) - 1);
    return { ...prev, [key]: nextValue };
  });
};

const maybeAbortOnStart = (): void => {
  if (fixtureStateFile === undefined) return;
  const state = readFixtureState();
  if ((state?.remainingInitFails ?? 0) > 0) {
    decrementStateCounter('remainingInitFails');
    process.exit(5);
  }
  if (state?.phase === 'exited' && (state.remainingRestartFails ?? 0) > 0) {
    decrementStateCounter('remainingRestartFails');
    process.exit(6);
  }
  if (state?.failOnStart === true) {
    process.exit(5);
  }
};

maybeAbortOnStart();

const shouldBlockEventLoop = process.env.MCP_FIXTURE_BLOCK_EVENT_LOOP === '1';

const sleep = async (ms: number): Promise<void> => await new Promise((resolve) => { setTimeout(resolve, ms); });
const blockCpu = (ms: number): void => {
  const end = Date.now() + ms;
  // eslint-disable-next-line no-empty
  while (Date.now() < end) { /* intentional busy wait to simulate hung server */ }
};

server.server.setRequestHandler(CallToolRequestSchema, async (request, extra) => {
  const { name, arguments: rawArgs } = request.params;
  const args = (rawArgs ?? {}) as Record<string, unknown>;

  if (name === 'test') {
    const textUnknown = args.text;
    const payload = typeof textUnknown === 'string' ? textUnknown : JSON.stringify(textUnknown);

    if (payload === 'trigger-mcp-failure') {
      throw new McpError(ErrorCode.InternalError, 'Simulated MCP tool failure');
    }

    if (payload === 'trigger-timeout') {
      await sleep(1500);
    }

    if (fixtureMode === 'restart') {
      const state = readFixtureState() ?? { phase: 'initial' };
      const hangMs = Number(process.env.MCP_FIXTURE_HANG_MS ?? '4000');
      const exitDelayMs = Number(process.env.MCP_FIXTURE_EXIT_DELAY_MS ?? '2000');
      if (state.phase === 'initial' || state.phase === 'hang-started') {
        const skipExit = process.env.MCP_FIXTURE_SKIP_EXIT === '1';
        updateFixtureState((prev) => ({ ...prev, phase: 'hang-started' }));
        if (!skipExit) {
          setTimeout(() => {
            updateFixtureState((prev) => ({ ...prev, phase: 'exited' }));
            process.exit(0);
          }, exitDelayMs);
        }
        if (shouldBlockEventLoop) {
          blockCpu(hangMs);
        } else {
          await sleep(hangMs);
        }
      } else if (state.phase === 'exited') {
        updateFixtureState((prev) => ({ ...prev, phase: 'recovered' }));
        return {
          content: [
            { type: 'text', text: typeof payload === 'string' ? `recovered:${payload}` : 'recovered' },
          ],
        };
      }
    }

    if (payload === 'long-output') {
      return {
        content: [
          { type: 'text', text: longPayload },
        ],
      };
    }

    if (payload === 'context-guard-600') {
      return {
        content: [
          { type: 'text', text: guardOverflowPayload },
        ],
      };
    }

    return {
      content: [
        { type: 'text', text: payload },
      ],
    };
  }

  if (name === 'test-summary') {
    const valueUnknown = args.value;
    const normalized = typeof valueUnknown === 'string' ? valueUnknown : JSON.stringify(valueUnknown);
    return {
      content: [
        {
          type: 'text',
          text: `# Summary\n\nReceived: ${normalized}`,
        },
      ],
    };
  }

  throw new McpError(ErrorCode.InvalidParams, `Unknown tool ${name}`);
});

async function main(): Promise<void> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((error: unknown) => {
  const message = error instanceof Error ? error.message : String(error);
  // eslint-disable-next-line no-console
  console.error(`test-mcp-server failed: ${message}`);
  process.exitCode = 1;
});
