export type TurnFailurePriority = 'critical' | 'high' | 'normal';

export interface TurnFailedSlugConfig {
  message: string;
  priority: TurnFailurePriority;
  stop?: boolean;
}

export const TURN_FAILED_SLUGS: Record<string, TurnFailedSlugConfig> = {
  final_report_format_mismatch: {
    message: 'Final report format mismatch. Use the required format and resend your final report/answer.',
    priority: 'high',
  },
  final_report_content_missing: {
    message: 'Final report content is missing or empty. Provide non-empty content in the required format and resend your final report/answer.',
    priority: 'high',
  },
  final_report_json_required: {
    message: 'Final report must be a JSON object that matches the final-report/answer schema. Provide a JSON object (not a string) and retry.',
    priority: 'high',
  },
  final_report_slack_messages_missing: {
    message: 'Slack Block Kit final report is missing the messages array. Provide valid Block Kit messages and retry.',
    priority: 'high',
  },
  final_report_schema_validation_failed: {
    message: 'Final report failed schema validation. Fix the payload to match the final-report/answer schema and resend your final report/answer.',
    priority: 'high',
  },
  tool_message_fallback_schema_failed: {
    message: 'Fallback final report from tool output failed schema validation. Provide your valid final report/answer in the required format.',
    priority: 'critical',
  },
  content_without_tools_or_report: {
    message: 'Text output detected without any tool calls or final report/answer. Call tool(s) or provide your final report/answer in the required wrapper exactly as instructed.',
    priority: 'normal',
  },
  empty_response: {
    message: 'Empty response detected: no tool calls and no final report/answer were received. Call a tool or output your final report/answer in the required XML wrapper.',
    priority: 'normal',
  },
  reasoning_only: {
    message: 'Reasoning-only output detected with no visible answer, tool calls, or final report. Call tools or provide your final report/answer in the required XML wrapper.',
    priority: 'normal',
  },
  reasoning_only_final: {
    message: 'Reasoning-only output detected with no visible final report/answer. Relax. This is easy. You must now summarize and provide your final report/answer. Output now the XML wrapper exactly as instructed, and then summarize your work in the requested format.',
    priority: 'normal',
  },
  output_truncated: {
    message: 'Output was truncated (stopReason=length). Retry with a shorter response that still includes the required tool calls or a complete final report.',
    priority: 'critical',
  },
  final_turn_no_report: {
    message: 'Final turn ended without a valid final report. Provide the final report/answer now using the required XML wrapper and do not call any tools.',
    priority: 'critical',
  },
  xml_wrapper_as_tool: {
    message: 'You called the XML wrapper tag as a tool, but it must be plain text in your response. Do NOT use tool-calling syntax; output the XML wrapper directly.',
    priority: 'high',
  },
  xml_final_report_not_json: {
    message: 'Final report payload is not valid JSON. Provide a JSON object that matches the final-report/answer schema and retry.',
    priority: 'high',
  },
  xml_tool_payload_not_json: {
    message: 'Tool payload is not valid JSON. Provide a JSON object for the tool parameters and retry.',
    priority: 'high',
  },
  xml_slot_mismatch: {
    message: 'XML wrapper ignored: the XML wrapper you used does not match the expected wrapper. Use the correct XML wrapper and retry.',
    priority: 'normal',
  },
  xml_missing_closing_tag: {
    message: 'Malformed XML: missing closing tag. Close the tag exactly as shown and retry.',
    priority: 'normal',
  },
  xml_malformed_mismatch: {
    message: 'Malformed XML: nonce/slot/tool mismatch or empty content. Use the exact nonce and slot, include non-empty content, and retry.',
    priority: 'normal',
  },
  xml_structured_output_truncated: {
    message: 'Your response was truncated (stopReason=length) because it exceeded the output token limit. Retry with a shorter output that still delivers your complete final report/answer in the required XML wrapper.',
    priority: 'critical',
  },
} as const;

export type TurnFailedSlug = keyof typeof TURN_FAILED_SLUGS;

export interface TurnFailedNoticeEvent {
  slug: TurnFailedSlug;
  reason?: string;
  order: number;
}

export const TURN_FAILURE_PRIORITY_WEIGHT: Record<TurnFailurePriority, number> = {
  critical: 3,
  high: 2,
  normal: 1,
};

export const MAX_TURN_FAILED_REASONS = 2;

const lookupTurnFailedConfig = (slug: string): TurnFailedSlugConfig | undefined => {
  if (!Object.prototype.hasOwnProperty.call(TURN_FAILED_SLUGS, slug)) {
    return undefined;
  }
  return TURN_FAILED_SLUGS[slug];
};

export const renderTurnFailedSlug = (slug: TurnFailedSlug, reason?: string): string => {
  const config = lookupTurnFailedConfig(slug);
  const base = config?.message.trimEnd() ?? `Turn failed due to an unknown error (slug=${slug}). Follow the latest instructions and retry.`;
  if (reason === undefined || reason.trim().length === 0) {
    return base;
  }
  return `${base} Reason: ${reason.trim()}`;
};

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

export const buildTurnFailedNotice = (events: TurnFailedNoticeEvent[]): string => {
  const selected = selectTurnFailedEntries(events);
  const reasons = selected.map((entry) => renderTurnFailedSlug(entry.slug, entry.reason));
  return `TURN-FAILED: ${reasons.join(' | ')}`;
};
