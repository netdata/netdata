import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

import type { LLMInterceptor, LLMInterceptorContext, TurnResult, TurnRequest, ConversationMessage } from '../types.js';

interface FixtureManagerOptions {
  rootDir: string;
  modelLabel: string;
  provider: string;
  modelId: string;
  scenarioId: string;
  scenarioLabel: string;
  stream: boolean;
  refresh: boolean;
}

interface FixtureTurnEntry {
  version: number;
  meta: {
    provider: string;
    model: string;
    scenarioId: string;
    scenarioLabel: string;
    modelLabel: string;
    stream: boolean;
    turn: number;
    timestamp: string;
  };
  request: NormalizedRequest;
  requestHash: string;
  result: NormalizedResult;
}

interface NormalizedRequest {
  provider: string;
  model: string;
  stream: boolean;
  temperature: number | null;
  topP: number | null;
  maxOutputTokens: number | null;
  reasoningLevel: string | null;
  reasoningValue: unknown;
  parallelToolCalls: boolean;
  isFinalTurn: boolean;
  turn: number | null;
  messages: ConversationMessage[];
  toolNames: string[];
}

type NormalizedResult = TurnResult;

const FIXTURE_VERSION = 1;

const sanitizeSegment = (value: string): string => {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .replace(/-{2,}/g, '-');
};

const isRecord = (value: unknown): value is Record<string, unknown> => {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
};

const isFixtureTurnEntry = (value: unknown): value is FixtureTurnEntry => {
  if (!isRecord(value)) return false;
  if (typeof value.version !== 'number') return false;
  if (!isRecord(value.meta)) return false;
  if (typeof value.requestHash !== 'string') return false;
  if (!isRecord(value.request)) return false;
  if (!isRecord(value.result)) return false;
  return true;
};

const stableSortValue = (value: unknown): unknown => {
  if (Array.isArray(value)) {
    return value.map((entry) => stableSortValue(entry));
  }
  if (isRecord(value)) {
    const sorted: Record<string, unknown> = {};
    Object.keys(value)
      .sort((a, b) => a.localeCompare(b))
      .forEach((key) => {
        sorted[key] = stableSortValue(value[key]);
      });
    return sorted;
  }
  return value;
};

const stableStringify = (value: unknown): string => JSON.stringify(stableSortValue(value));

const structuredCloneSafe = <T>(value: T): T => {
  try {
    return structuredClone(value);
  } catch {
    return JSON.parse(JSON.stringify(value)) as T;
  }
};

export class Phase2FixtureManager implements LLMInterceptor {
  private readonly dir: string;
  private readonly options: FixtureManagerOptions;
  private readonly entries: FixtureTurnEntry[] = [];
  private pointer = 0;
  private recorded = 0;
  private replayed = 0;
  private readonly recordEverything: boolean;

  constructor(options: FixtureManagerOptions) {
    this.options = options;
    const providerSegment = sanitizeSegment(options.provider);
    const modelSegment = sanitizeSegment(options.modelId);
    const scenarioSegment = sanitizeSegment(options.scenarioId);
    const streamSegment = options.stream ? 'stream-on' : 'stream-off';
    this.dir = path.join(options.rootDir, providerSegment, modelSegment, scenarioSegment, streamSegment);

    let dirExists = fs.existsSync(this.dir);
    if (options.refresh && dirExists) {
      fs.rmSync(this.dir, { recursive: true, force: true });
      dirExists = false;
    }

    if (dirExists) {
      const files = fs.readdirSync(this.dir)
        .filter((name) => name.startsWith('turn-') && name.endsWith('.json'))
        .sort((a, b) => a.localeCompare(b));
      files.forEach((file) => {
        const fullPath = path.join(this.dir, file);
        const raw = fs.readFileSync(fullPath, 'utf8');
        const parsed = JSON.parse(raw) as unknown;
        if (isFixtureTurnEntry(parsed) && parsed.version === FIXTURE_VERSION) {
          this.entries.push(parsed);
        }
      });
    }
    this.recordEverything = !dirExists || options.refresh || this.entries.length === 0;
  }

  replay(context: LLMInterceptorContext): TurnResult | undefined {
    if (this.recordEverything) {
      return undefined;
    }
    if (this.pointer >= this.entries.length) {
      return undefined;
    }
    const entry = this.entries[this.pointer];
    const candidate = this.normalizeRequest(context.request);
    const hash = this.hashRequest(candidate);
    if (hash !== entry.requestHash) {
      throw new Error(`Phase2 fixture mismatch for ${this.describe()}. Run with --refresh-llm-fixtures to regenerate.`);
    }
    this.pointer += 1;
    this.replayed += 1;
    return structuredCloneSafe(entry.result);
  }

  record(context: LLMInterceptorContext, result: TurnResult): void {
    const needsRecording = this.recordEverything || this.pointer >= this.entries.length;
    if (!needsRecording) {
      return;
    }
    const normalizedRequest = this.normalizeRequest(context.request);
    const normalizedResult = this.normalizeResult(result);
    const entry: FixtureTurnEntry = {
      version: FIXTURE_VERSION,
      meta: {
        provider: this.options.provider,
        model: this.options.modelId,
        scenarioId: this.options.scenarioId,
        scenarioLabel: this.options.scenarioLabel,
        modelLabel: this.options.modelLabel,
        stream: this.options.stream,
        turn: this.pointer + 1,
        timestamp: new Date().toISOString(),
      },
      request: normalizedRequest,
      requestHash: this.hashRequest(normalizedRequest),
      result: normalizedResult,
    };

    if (!fs.existsSync(this.dir)) {
      fs.mkdirSync(this.dir, { recursive: true });
    }

    this.entries.push(entry);
    this.pointer += 1;
    this.recorded += 1;
    const filename = `turn-${String(this.entries.length).padStart(4, '0')}.json`;
    const target = path.join(this.dir, filename);
    fs.writeFileSync(target, JSON.stringify(entry, null, 2));
  }

  finalize(): void {
    if (!this.recordEverything && this.pointer < this.entries.length) {
      throw new Error(`Phase2 fixture replay did not consume all turns for ${this.describe()}: used ${String(this.pointer)} of ${String(this.entries.length)}`);
    }
  }

  stats(): { recorded: number; replayed: number; dir: string; turns: number } {
    return { recorded: this.recorded, replayed: this.replayed, dir: this.dir, turns: this.pointer };
  }

  private describe(): string {
    return `${this.options.provider}/${this.options.modelId} scenario=${this.options.scenarioId} stream=${this.options.stream ? 'on' : 'off'}`;
  }

  private normalizeRequest(request: TurnRequest): NormalizedRequest {
    return {
      provider: request.provider,
      model: request.model,
      stream: request.stream ?? false,
      temperature: request.temperature ?? null,
      topP: request.topP ?? null,
      maxOutputTokens: request.maxOutputTokens ?? null,
      reasoningLevel: request.reasoningLevel ?? null,
      reasoningValue: request.reasoningValue ?? null,
      parallelToolCalls: request.parallelToolCalls ?? false,
      isFinalTurn: request.isFinalTurn ?? false,
      turn: request.turnMetadata?.turn ?? null,
      messages: structuredCloneSafe<ConversationMessage[]>(request.messages),
      toolNames: request.tools.map((tool) => tool.name),
    };
  }

  private normalizeResult(result: TurnResult): NormalizedResult {
    return structuredCloneSafe(result);
  }

  private hashRequest(request: NormalizedRequest): string {
    const hash = crypto.createHash('sha256');
    hash.update(stableStringify(request));
    return hash.digest('hex');
  }
}
