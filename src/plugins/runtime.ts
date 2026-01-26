import Ajv from 'ajv';

import type { FinalReportManager } from '../final-report-manager.js';
import type { TurnFailedSlug } from '../llm-messages-turn-failed.js';
import type {
  AIAgentSessionConfig,
  ConversationMessage,
  FinalReportPayload,
  LogEntry,
} from '../types.js';
import type {
  FinalReportPlugin,
  FinalReportPluginContext,
  PreparedFinalReportPluginDescriptor,
  ResolvedFinalReportPluginRequirement,
} from './types.js';
import type { Ajv as AjvClass, ErrorObject, Options as AjvOptions } from 'ajv';

import { formatJsonParseHint, formatSchemaMismatchSummary } from '../llm-messages.js';
import { isPlainObject, parseJsonRecordDetailed } from '../utils.js';

import { loadFinalReportPluginFactories } from './loader.js';

const NONCE_TOKEN_PATTERN = /\bNONCE\b/g;
const META_REMOTE_IDENTIFIER = 'agent:final-report-plugins';
const META_TURN_FAILED_INVALID: TurnFailedSlug = 'final_meta_invalid';

type AjvInstance = AjvClass;
type AjvConstructor = new (options?: AjvOptions) => AjvInstance;
const AjvCtor: AjvConstructor = Ajv as unknown as AjvConstructor;

interface MetaBlock {
  plugin?: string;
  content: string;
}

interface RuntimePluginEntry {
  descriptor: PreparedFinalReportPluginDescriptor;
  plugin: FinalReportPlugin;
  requirements: ResolvedFinalReportPluginRequirement;
  validator: ReturnType<AjvInstance['compile']>;
  metaWrapper: string;
}

interface MetaProcessingIssue {
  slug: TurnFailedSlug;
  reason: string;
}

interface MetaProcessingResult {
  issues: MetaProcessingIssue[];
  invalidPluginNames: string[];
}

interface SessionInfo {
  sessionId: string;
  originId: string;
  agentId: string;
  agentPath: string;
}

interface CompletionContext {
  finalReport: FinalReportPayload;
  messages: readonly ConversationMessage[];
  userRequest: string;
  fromCache: boolean;
}

const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

const replaceNonce = (value: string, nonce: string): string => value.replace(NONCE_TOKEN_PATTERN, nonce);

const buildMetaWrapper = (nonce: string, pluginName: string): string => (
  `<ai-agent-${nonce}-META plugin="${pluginName}">{...}</ai-agent-${nonce}-META>`
);

const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

const requireNonEmptyString = (
  value: unknown,
  field: string,
  pluginName: string,
  rawPath: string,
): string => {
  if (typeof value !== 'string') {
    throw new Error(`Final report plugin '${pluginName}' (${rawPath}) must provide string field '${field}'.`);
  }
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    throw new Error(`Final report plugin '${pluginName}' (${rawPath}) field '${field}' cannot be empty.`);
  }
  return trimmed;
};

const formatAjvErrors = (errors: ErrorObject[] | null | undefined): string => (
  Array.isArray(errors)
    ? errors
      .map((error) => {
        const pathValue = typeof error.instancePath === 'string' && error.instancePath.length > 0
          ? error.instancePath
          : (typeof error.schemaPath === 'string' ? error.schemaPath : '');
        const message = typeof error.message === 'string' ? error.message : '';
        return `${pathValue} ${message}`.trim();
      })
      .join('; ')
    : ''
);

export class FinalReportPluginRuntimeManager {
  private readonly ajv: AjvInstance = new AjvCtor({ allErrors: true, strict: false });
  private readonly entries: RuntimePluginEntry[] = [];
  private readonly entriesByName = new Map<string, RuntimePluginEntry>();
  private initialized = false;
  private completionExecuted = false;

  constructor(
    private readonly descriptors: PreparedFinalReportPluginDescriptor[],
    private readonly sessionNonce: string,
    private readonly finalReportManager: FinalReportManager,
    private readonly log: (entry: LogEntry) => void,
    private readonly getCurrentTurn: () => number,
    private readonly sessionInfo: SessionInfo,
    private readonly createSession: (config: AIAgentSessionConfig) => { run: () => Promise<unknown> },
  ) {}

  private ensureInitialized(): void {
    if (!this.initialized) {
      throw new Error('Final report plugins must be initialized before use.');
    }
  }

  private logEntry(severity: LogEntry['severity'], turn: number, message: string): void {
    this.log({
      timestamp: Date.now(),
      severity,
      turn,
      subturn: 0,
      direction: 'response',
      type: 'llm',
      remoteIdentifier: META_REMOTE_IDENTIFIER,
      fatal: false,
      message,
    });
  }

  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    if (this.descriptors.length === 0) {
      this.finalReportManager.setRequiredPlugins([]);
      this.initialized = true;
      return;
    }

    const factories = await loadFinalReportPluginFactories(this.descriptors);

    const seenNames = new Map<string, string>();
    const entries = factories.map((factory, index) => {
      const descriptor = this.descriptors[index];
      const plugin = (() => {
        try {
          return factory();
        } catch (error) {
          throw new Error(
            `Final report plugin '${descriptor.rawPath}' factory threw: ${toErrorMessage(error)}`,
          );
        }
      })();

      if (!isPlainObject(plugin)) {
        throw new Error(
          `Final report plugin '${descriptor.rawPath}' must return an object with name/getRequirements/onComplete.`,
        );
      }

      const pluginName = requireNonEmptyString(plugin.name, 'name', descriptor.rawPath, descriptor.rawPath);
      const previousPath = seenNames.get(pluginName);
      if (previousPath !== undefined) {
        throw new Error(
          `Duplicate final report plugin name '${pluginName}' found in '${previousPath}' and '${descriptor.rawPath}'.`,
        );
      }
      seenNames.set(pluginName, descriptor.rawPath);

      if (typeof plugin.getRequirements !== 'function') {
        throw new Error(`Final report plugin '${pluginName}' (${descriptor.rawPath}) must implement getRequirements().`);
      }
      if (typeof plugin.onComplete !== 'function') {
        throw new Error(`Final report plugin '${pluginName}' (${descriptor.rawPath}) must implement onComplete().`);
      }

      const requirementsRaw = (() => {
        try {
          return plugin.getRequirements();
        } catch (error) {
          throw new Error(
            `Final report plugin '${pluginName}' (${descriptor.rawPath}) getRequirements() threw: ${toErrorMessage(error)}`,
          );
        }
      })();

      if (!isPlainObject(requirementsRaw)) {
        throw new Error(`Final report plugin '${pluginName}' (${descriptor.rawPath}) must return an object from getRequirements().`);
      }

      const schema = requirementsRaw.schema;
      if (!isPlainObject(schema)) {
        throw new Error(`Final report plugin '${pluginName}' (${descriptor.rawPath}) must provide a JSON schema object.`);
      }

      try {
        JSON.stringify(schema);
      } catch (error) {
        throw new Error(
          `Final report plugin '${pluginName}' (${descriptor.rawPath}) schema is not serializable: ${toErrorMessage(error)}`,
        );
      }

      const systemPromptInstructions = replaceNonce(
        requireNonEmptyString(requirementsRaw.systemPromptInstructions, 'systemPromptInstructions', pluginName, descriptor.rawPath),
        this.sessionNonce,
      );
      const xmlNextSnippet = replaceNonce(
        requireNonEmptyString(requirementsRaw.xmlNextSnippet, 'xmlNextSnippet', pluginName, descriptor.rawPath),
        this.sessionNonce,
      );
      const finalReportExampleSnippet = replaceNonce(
        requireNonEmptyString(requirementsRaw.finalReportExampleSnippet, 'finalReportExampleSnippet', pluginName, descriptor.rawPath),
        this.sessionNonce,
      );

      const requirements: ResolvedFinalReportPluginRequirement = {
        name: pluginName,
        schema,
        systemPromptInstructions,
        xmlNextSnippet,
        finalReportExampleSnippet,
      };

      const validator = (() => {
        try {
          return this.ajv.compile(schema);
        } catch (error) {
          throw new Error(
            `Final report plugin '${pluginName}' (${descriptor.rawPath}) schema compilation failed: ${toErrorMessage(error)}`,
          );
        }
      })();

      return {
        descriptor,
        plugin,
        requirements,
        validator,
        metaWrapper: buildMetaWrapper(this.sessionNonce, pluginName),
      } satisfies RuntimePluginEntry;
    });

    entries.forEach((entry) => {
      this.entries.push(entry);
      this.entriesByName.set(entry.requirements.name, entry);
    });

    this.finalReportManager.setRequiredPlugins(this.entries.map((entry) => entry.requirements.name));
    this.initialized = true;
  }

  getResolvedRequirements(): ResolvedFinalReportPluginRequirement[] {
    this.ensureInitialized();
    return this.entries.map((entry) => entry.requirements);
  }

  hasPlugins(): boolean {
    this.ensureInitialized();
    return this.entries.length > 0;
  }

  getMissingRequiredPluginNames(): string[] {
    this.ensureInitialized();
    return this.finalReportManager.getMissingRequiredPluginMetas();
  }

  buildMissingMetaReason(missingPluginNames?: string[]): string {
    this.ensureInitialized();
    const missing = Array.isArray(missingPluginNames) && missingPluginNames.length > 0
      ? missingPluginNames
      : this.getMissingRequiredPluginNames();
    const summaries = this.entries
      .filter((entry) => missing.includes(entry.requirements.name))
      .map((entry) => `plugin="${entry.requirements.name}" wrapper=${entry.metaWrapper}`);

    if (summaries.length === 0) {
      const missingNames = missing.join(', ');
      if (missingNames.length > 0) {
        this.logEntry('ERR', this.getCurrentTurn(), `Missing META plugins not found in registry: ${missingNames}`);
      }
      return 'required META blocks are missing';
    }

    return summaries.join(' | ');
  }

  validateCachePayload(payload: { finalReport: FinalReportPayload; pluginMetas: unknown }, turn: number): boolean {
    this.ensureInitialized();
    if (this.entries.length === 0) {
      return true;
    }

    const metasRecord = isPlainObject(payload.pluginMetas) ? payload.pluginMetas : undefined;
    if (metasRecord === undefined) {
      this.logEntry('WRN', turn, 'Agent cache entry has no META record but final report plugins are required; treating as cache miss.');
      return false;
    }

    const validations = this.entries.map((entry) => {
      const metaValue = metasRecord[entry.requirements.name];
      if (!isPlainObject(metaValue)) {
        return {
          entry,
          valid: false,
          reason: `missing_or_invalid_meta wrapper=${entry.metaWrapper}`,
        } as const;
      }
      const valid = entry.validator(metaValue) === true;
      if (!valid) {
        const errors = formatAjvErrors(entry.validator.errors);
        const summary = formatSchemaMismatchSummary(errors.length > 0 ? errors : 'schema validation failed');
        return {
          entry,
          valid: false,
          reason: `schema_mismatch=${summary} wrapper=${entry.metaWrapper}`,
        } as const;
      }
      return {
        entry,
        valid: true,
        meta: metaValue,
      } as const;
    });

    const invalidReasons = validations
      .filter((validation) => !validation.valid)
      .map((validation) => `plugin="${validation.entry.requirements.name}" ${validation.reason}`);

    if (invalidReasons.length > 0) {
      this.logEntry('WRN', turn, `Agent cache miss due to META validation failure: ${invalidReasons.join(' | ')}`);
      return false;
    }

    validations.forEach((validation) => {
      if (validation.valid) {
        this.finalReportManager.commitPluginMeta(validation.entry.requirements.name, validation.meta);
      }
    });

    const cachedReport = payload.finalReport;
    this.finalReportManager.commit(
      {
        format: cachedReport.format,
        content: cachedReport.content,
        content_json: cachedReport.content_json,
        metadata: cachedReport.metadata,
      },
      'tool-call',
    );

    return this.finalReportManager.isFinalizationReady();
  }

  processMetaBlocks(metaBlocks: MetaBlock[], turn: number): MetaProcessingResult {
    this.ensureInitialized();

    if (this.entries.length === 0) {
      if (metaBlocks.length > 0) {
        this.logEntry('WRN', turn, 'META blocks were provided but no final report plugins are configured; ignoring META.');
      }
      return { issues: [], invalidPluginNames: [] };
    }

    const lastInvalidReasonByPlugin = new Map<string, string>();

    metaBlocks.forEach((metaBlock) => {
      const pluginNameRaw = metaBlock.plugin;
      const pluginName = typeof pluginNameRaw === 'string' ? pluginNameRaw.trim() : '';
      if (pluginName.length === 0) {
        this.logEntry('WRN', turn, 'META block missing plugin attribute; ignoring META block.');
        return;
      }

      const entry = this.entriesByName.get(pluginName);
      if (entry === undefined) {
        this.logEntry('WRN', turn, `META block references unknown plugin '${pluginName}'; ignoring META block.`);
        return;
      }

      this.logEntry('VRB', turn, `Received META block for plugin '${pluginName}'.`);

      const parsed = parseJsonRecordDetailed(metaBlock.content);
      if (parsed.value === undefined) {
        const parseError = parsed.error ?? 'expected_json_object';
        const parseHint = formatJsonParseHint(parseError);
        const reason = `plugin="${pluginName}" invalid_json=${parseHint} wrapper=${entry.metaWrapper}`;
        lastInvalidReasonByPlugin.set(pluginName, reason);
        this.logEntry('WRN', turn, `Plugin META invalid JSON for '${pluginName}': ${parseHint}`);
        return;
      }

      if (parsed.repairs.length > 0) {
        this.logEntry('WRN', turn, `Plugin META repaired for '${pluginName}' via [${parsed.repairs.join('>')}].`);
      }

      const valid = entry.validator(parsed.value) === true;
      if (!valid) {
        const errors = formatAjvErrors(entry.validator.errors);
        const summary = formatSchemaMismatchSummary(errors.length > 0 ? errors : 'schema validation failed');
        const reason = `plugin="${pluginName}" schema_mismatch=${summary} wrapper=${entry.metaWrapper}`;
        lastInvalidReasonByPlugin.set(pluginName, reason);
        this.logEntry('WRN', turn, `Plugin META schema validation failed for '${pluginName}': ${summary}`);
        return;
      }

      this.finalReportManager.commitPluginMeta(pluginName, parsed.value);
    });

    const invalidEntries = this.entries.filter((entry) => (
      this.finalReportManager.getPluginMeta(entry.requirements.name) === undefined
      && lastInvalidReasonByPlugin.has(entry.requirements.name)
    ));

    if (invalidEntries.length === 0) {
      return { issues: [], invalidPluginNames: [] };
    }

    const invalidReason = invalidEntries
      .map((entry) => lastInvalidReasonByPlugin.get(entry.requirements.name) ?? '')
      .filter((reason) => reason.length > 0)
      .join(' | ');

    if (invalidReason.length === 0) {
      return { issues: [], invalidPluginNames: [] };
    }

    return {
      issues: [{ slug: META_TURN_FAILED_INVALID, reason: invalidReason }],
      invalidPluginNames: invalidEntries.map((entry) => entry.requirements.name),
    };
  }

  runOnComplete(context: CompletionContext): void {
    this.ensureInitialized();
    if (this.entries.length === 0) {
      return;
    }
    if (!this.finalReportManager.isFinalizationReady()) {
      const missing = this.finalReportManager.getMissingRequiredPluginMetas().join(', ');
      this.logEntry('ERR', this.getCurrentTurn(), `Final report plugins not ready at completion time; missing META for: ${missing}`);
      return;
    }
    if (this.completionExecuted) {
      return;
    }
    this.completionExecuted = true;

    const pluginMetas = this.finalReportManager.getPluginMetasRecord();
    const baseContext: Omit<FinalReportPluginContext, 'pluginData' | 'finalReport'> = {
      sessionId: this.sessionInfo.sessionId,
      originId: this.sessionInfo.originId,
      agentId: this.sessionInfo.agentId,
      agentPath: this.sessionInfo.agentPath,
      userRequest: context.userRequest,
      messages: context.messages,
      fromCache: context.fromCache,
      createSession: this.createSession,
    };

    const runPluginCompletion = async (entry: RuntimePluginEntry): Promise<void> => {
      const pluginData = pluginMetas[entry.requirements.name] ?? {};
      const safeFinalReport = cloneJson(context.finalReport);
      const safePluginData = cloneJson(pluginData);
      try {
        await entry.plugin.onComplete({ ...baseContext, finalReport: safeFinalReport, pluginData: safePluginData });
      } catch (error: unknown) {
        this.logEntry('WRN', this.getCurrentTurn(), `Final report plugin '${entry.requirements.name}' onComplete failed: ${toErrorMessage(error)}`);
      }
    };

    this.entries.forEach((entry) => {
      void runPluginCompletion(entry);
    });
  }
}
