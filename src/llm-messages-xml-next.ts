import type { OutputFormatId } from './formats.js';
import type { ResolvedFinalReportPluginRequirement } from './plugins/types.js';

import { formatPromptValue } from './formats.js';
import { buildPromptVars } from './prompt-builder.js';
import { renderPromptTemplate } from './prompts/templates.js';

const ROUTER_HANDOFF_TOOL = 'router__handoff-to';

// Type for final turn reasons - derived from the slug keys
type FinalTurnReason = 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion' | 'user_stop';

// Context Interface
export interface XmlNextNoticeContext {
  nonce: string;
  turn: number;
  maxTurns?: number;
  maxTools?: number;
  attempt: number;
  maxRetries: number;
  contextPercentUsed: number;
  expectedFinalFormat: OutputFormatId | 'text';
  responseMode?: 'agentic' | 'chat';
  hasExternalTools: boolean;
  taskStatusToolEnabled: boolean;
  forcedFinalTurnReason?: FinalTurnReason;
  finalTurnTools?: string[];
  // Wasted turn tracking
  consecutiveProgressOnlyTurns?: number;
  finalReportPluginRequirements: ResolvedFinalReportPluginRequirement[];
  finalReportLocked: boolean;
  missingMetaPluginNames: string[];
}

const resolveFormatPromptValue = (format: OutputFormatId | 'text'): string => {
  if (format === 'text') return formatPromptValue('pipe');
  return formatPromptValue(format);
};

const hasRouterHandoffTool = (context: XmlNextNoticeContext): boolean => (
  !context.finalReportLocked
  && Array.isArray(context.finalTurnTools)
  && context.finalTurnTools.some((tool) => tool === ROUTER_HANDOFF_TOOL)
);

const selectMissingMetaPluginNames = (context: XmlNextNoticeContext): string[] => {
  const requiredNames = context.finalReportPluginRequirements.map((requirement) => requirement.name);
  const requiredSet = new Set(requiredNames);
  const missing = context.missingMetaPluginNames.filter((name) => requiredSet.has(name));
  return missing.length > 0 ? missing : requiredNames;
};

const shouldShowLastRetryReminder = (context: XmlNextNoticeContext): boolean => {
  if (context.forcedFinalTurnReason !== undefined || context.finalReportLocked) return false;
  const lastRetryNonFinal = context.maxTurns !== undefined
    ? context.turn < (context.maxTurns - 1)
    : true;
  return context.attempt === context.maxRetries && context.maxRetries > 0 && lastRetryNonFinal;
};

export const buildXmlNextNotice = (context: XmlNextNoticeContext): string => {
  const runtimeVars = buildPromptVars();
  return renderPromptTemplate('xmlNext', {
    nonce: context.nonce,
    turn: context.turn,
    max_turns: context.maxTurns ?? null,
    max_tools: context.maxTools ?? null,
    attempt: context.attempt,
    max_retries: context.maxRetries,
    context_percent_used: context.contextPercentUsed,
    expected_final_format: context.expectedFinalFormat,
    response_mode: context.responseMode ?? 'agentic',
    format_prompt_value: resolveFormatPromptValue(context.expectedFinalFormat),
    datetime: runtimeVars.DATETIME,
    timestamp: runtimeVars.TIMESTAMP,
    day: runtimeVars.DAY,
    timezone: runtimeVars.TIMEZONE,
    has_external_tools: context.hasExternalTools,
    task_status_tool_enabled: context.taskStatusToolEnabled,
    forced_final_turn_reason: context.forcedFinalTurnReason ?? '',
    final_turn_tools: Array.isArray(context.finalTurnTools) ? context.finalTurnTools : [],
    final_report_locked: context.finalReportLocked,
    consecutive_progress_only_turns: context.consecutiveProgressOnlyTurns ?? 0,
    show_last_retry_reminder: shouldShowLastRetryReminder(context),
    allow_router_handoff: hasRouterHandoffTool(context),
    plugin_requirements: context.finalReportPluginRequirements,
    missing_meta_plugin_names: selectMissingMetaPluginNames(context),
  });
};
