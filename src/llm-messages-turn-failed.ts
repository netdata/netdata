import type { ResolvedFinalReportPluginRequirement } from './plugins/types.js';

import { renderPromptTemplate } from './prompts/templates.js';

export type TurnFailurePriority = 'critical' | 'high' | 'normal';

export interface TurnFailedSlugConfig {
  priority: TurnFailurePriority;
}

export const TURN_FAILED_SLUGS: Record<string, TurnFailedSlugConfig> = {
  final_report_format_mismatch: { priority: 'high' },
  final_report_content_missing: { priority: 'high' },
  final_report_json_required: { priority: 'high' },
  final_report_slack_messages_missing: { priority: 'high' },
  final_report_schema_validation_failed: { priority: 'high' },
  final_meta_missing: { priority: 'critical' },
  final_meta_invalid: { priority: 'critical' },
  tool_message_fallback_schema_failed: { priority: 'critical' },
  content_without_tools_or_report: { priority: 'normal' },
  empty_response: { priority: 'normal' },
  reasoning_only: { priority: 'normal' },
  reasoning_only_final: { priority: 'normal' },
  output_truncated: { priority: 'critical' },
  final_turn_no_report: { priority: 'critical' },
  xml_wrapper_as_tool: { priority: 'high' },
  xml_final_report_not_json: { priority: 'high' },
  xml_tool_payload_not_json: { priority: 'high' },
  xml_slot_mismatch: { priority: 'normal' },
  xml_missing_closing_tag: { priority: 'normal' },
  xml_malformed_mismatch: { priority: 'normal' },
  xml_structured_output_truncated: { priority: 'critical' },
} as const;

export type TurnFailedSlug = keyof typeof TURN_FAILED_SLUGS;

export interface TurnFailedNoticeEvent {
  slug: TurnFailedSlug;
  reason?: string;
  order: number;
}

export interface TurnFailedNoticeRenderContext {
  sessionNonce?: string;
  pluginRequirements: ResolvedFinalReportPluginRequirement[];
  finalReportLocked: boolean;
}

export const TURN_FAILURE_PRIORITY_WEIGHT: Record<TurnFailurePriority, number> = {
  critical: 3,
  high: 2,
  normal: 1,
};

export const MAX_TURN_FAILED_REASONS = 2;

const normalizeReason = (reason?: string): string => (typeof reason === 'string' ? reason.trim() : '');

export const renderTurnFailedSlug = (slug: TurnFailedSlug, reason?: string, sessionNonce?: string): string => (
  renderPromptTemplate('turnFailed', {
    events: [{ slug, reason: normalizeReason(reason) }],
    session_nonce: sessionNonce ?? '',
    plugin_requirements: [],
    final_report_locked: false,
    include_prefix: false,
  })
);

const selectTurnFailedEntries = (events: TurnFailedNoticeEvent[]): TurnFailedNoticeEvent[] => {
  const deduped = new Map<TurnFailedSlug, TurnFailedNoticeEvent>();
  events.forEach((entry) => {
    const existing = deduped.get(entry.slug);
    if (existing === undefined) {
      deduped.set(entry.slug, entry);
      return;
    }
    if ((existing.reason === undefined || existing.reason.trim().length === 0) && typeof entry.reason === 'string' && entry.reason.trim().length > 0) {
      existing.reason = entry.reason;
    }
  });

  const sorted = [...deduped.values()].sort((a, b) => {
    const priorityDiff = TURN_FAILURE_PRIORITY_WEIGHT[TURN_FAILED_SLUGS[b.slug].priority] - TURN_FAILURE_PRIORITY_WEIGHT[TURN_FAILED_SLUGS[a.slug].priority];
    if (priorityDiff !== 0) return priorityDiff;
    return a.order - b.order;
  });

  return sorted.slice(0, MAX_TURN_FAILED_REASONS);
};

export const buildTurnFailedNotice = (
  events: TurnFailedNoticeEvent[],
  context?: TurnFailedNoticeRenderContext,
): string => {
  const selected = selectTurnFailedEntries(events);
  const requirements = context?.pluginRequirements ?? [];
  const finalReportLocked = context?.finalReportLocked === true;
  return renderPromptTemplate('turnFailed', {
    events: selected.map((entry) => ({ slug: entry.slug, reason: normalizeReason(entry.reason) })),
    session_nonce: context?.sessionNonce ?? '',
    plugin_requirements: requirements,
    final_report_locked: finalReportLocked,
    include_prefix: true,
  });
};
