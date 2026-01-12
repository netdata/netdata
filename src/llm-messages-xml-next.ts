import type { OutputFormatId } from './formats.js';

export type XmlNextSlug = keyof typeof XML_NEXT_SLUGS;

export interface XmlNextSlugConfig {
  message: string;
  priority: number;
  stop?: boolean;
}

export interface XmlNextNoticeEvent {
  slug: XmlNextSlug;
  reason?: string;
  order: number;
}

export interface XmlNextNoticeContext {
  nonce: string;
  turn: number;
  maxTurns?: number;
  attempt: number;
  maxRetries: number;
  contextPercentUsed: number;
  expectedFinalFormat: OutputFormatId | 'text';
  hasExternalTools: boolean;
  taskStatusToolEnabled: boolean;
  forcedFinalTurnReason?: 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion';
  finalTurnTools?: string[];
}

export const XML_NEXT_SLUGS: Record<string, XmlNextSlugConfig> = {
  turn_status: {
    message: 'Turn status.',
    priority: 0,
  },
  final_turn_context: {
    message: 'Final turn forced by context window limit. You MUST NOW provide your final report/answer. If any tools are allowed for this final turn, only call those. Use the required XML wrapper and format instructions to deliver your final report/answer. If information is insufficient or missing, state the limitation in your final report.',
    priority: 1,
    stop: true,
  },
  final_turn_max_turns: {
    message: 'Final turn reached (max turns). You MUST NOW provide your final report/answer. If any tools are allowed for this final turn, only call those. Use the required XML wrapper and format instructions to deliver your final report/answer. If information is insufficient or missing, state the limitation in your final report.',
    priority: 1,
    stop: true,
  },
  final_turn_task_status_completed: {
    message: 'Task marked completed. You MUST NOW provide your final report/answer. If any tools are allowed for this final turn, only call those. Use the required XML wrapper and format instructions to deliver your final report/answer.',
    priority: 1,
    stop: true,
  },
  final_turn_task_status_only: {
    message: 'Repeated standalone task status calls detected without calling any other tools, so final turn has been enforced. You MUST NOW provide your final report/answer. If any tools are allowed for this final turn, only call those. Use the required XML wrapper and format instructions to deliver your final report/answer. If information is insufficient or missing, state the limitation explicitly in your final report/answer.',
    priority: 1,
    stop: true,
  },
  final_turn_retry_exhaustion: {
    message: 'All retry attempts exhausted, so final turn has been enforced. You MUST NOW provide your final report/answer. If any tools are allowed for this final turn, only call those. Use the required XML wrapper and format instructions to deliver your final report/answer. If information is insufficient or missing, state the limitation explicitly in your final report.',
    priority: 1,
    stop: true,
  },
  tools_available_header: {
    message: 'You now need to decide your next move:\nEITHER\n- Call tools to advance your task following the main prompt instructions (pay attention to tool formatting and schema requirements).',
    priority: 5,
  },
  tools_available_task_status_line: {
    message: '- Together with these tool calls, also call `agent__task_status` to inform your user about what you are doing and why you are calling these tools.',
    priority: 6,
  },
  tools_available_or: {
    message: 'OR\n- Provide your final report/answer in the expected format using the XML wrapper.',
    priority: 7,
  },
  tools_available_task_status_footer: {
    message: 'Call `agent__task_status` to inform the user about your progress and mark tasks as complete.',
    priority: 8,
  },
  tools_unavailable: {
    message: 'You MUST now provide your final report/answer in the expected format using the XML wrapper.',
    priority: 5,
  },
  last_retry_tool_reminder: {
    message: 'Reminder: do not end with plain text. Use an available tool to make progress. When ready to conclude, provide your final report/answer in the required XML wrapper.',
    priority: 9,
  },
} as const;

const renderXmlNextReason = (reason?: string): string => {
  if (reason === undefined) return '';
  const trimmed = reason.trim();
  if (trimmed.length === 0) return '';
  return ` Reason: ${trimmed}`;
};

const renderXmlNextEntry = (entry: XmlNextNoticeEvent): string => {
  const config = XML_NEXT_SLUGS[entry.slug];
  const base = config.message.trimEnd();
  return `${base}${renderXmlNextReason(entry.reason)}`;
};

const selectXmlNextEntries = (events: XmlNextNoticeEvent[]): XmlNextNoticeEvent[] => {
  const deduped = new Map<XmlNextSlug, XmlNextNoticeEvent>();
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

  const ordered = [...deduped.values()].sort((a, b) => {
    const priorityDiff = XML_NEXT_SLUGS[a.slug].priority - XML_NEXT_SLUGS[b.slug].priority;
    if (priorityDiff !== 0) return priorityDiff;
    return a.order - b.order;
  });

  const selected: XmlNextNoticeEvent[] = [];
  // eslint-disable-next-line functional/no-loop-statements -- ordered selection with stop logic
  for (const entry of ordered) {
    selected.push(entry);
    if (XML_NEXT_SLUGS[entry.slug].stop === true) {
      break;
    }
  }

  return selected;
};

export const buildXmlNextNotice = (events: XmlNextNoticeEvent[]): string => {
  const selected = selectXmlNextEntries(events);
  const lines: string[] = ['# System Notice', ''];
  selected.forEach((entry) => {
    lines.push(renderXmlNextEntry(entry));
  });
  lines.push('');
  return lines.join('\n');
};

const buildTurnStatusReason = (context: XmlNextNoticeContext): string => {
  const turnInfo = `This is turn/step ${String(context.turn)}${context.maxTurns !== undefined ? ` of ${String(context.maxTurns)}` : ''}.`;
  const retryInfo = context.attempt > 1 ? ` Retry ${String(context.attempt)} of ${String(context.maxRetries)}.` : '';
  const ctxInfo = ` Your context window is ${String(context.contextPercentUsed)}% full.`;
  return `${turnInfo}${retryInfo}${ctxInfo}`.trim();
};

const buildFinalWrapperReason = (context: XmlNextNoticeContext): string => {
  const finalSlotId = `${context.nonce}-FINAL`;
  const tools = Array.isArray(context.finalTurnTools) && context.finalTurnTools.length > 0
    ? `; allowed tools: ${context.finalTurnTools.join(', ')}`
    : '';
  return `expected format: ${context.expectedFinalFormat}; wrapper: <ai-agent-${finalSlotId} format="${context.expectedFinalFormat}">${tools}`;
};

const buildReminderReason = (context: XmlNextNoticeContext): string => {
  if (!context.taskStatusToolEnabled) return '';
  return 'exclude agent__task_status';
};

export const buildXmlNextEvents = (context: XmlNextNoticeContext, orderStart: number): { events: XmlNextNoticeEvent[]; nextOrder: number } => {
  let order = orderStart;
  const events: XmlNextNoticeEvent[] = [];
  const push = (slug: XmlNextSlug, reason?: string): void => {
    events.push({ slug, reason, order });
    order += 1;
  };

  push('turn_status', buildTurnStatusReason(context));

  if (context.forcedFinalTurnReason !== undefined) {
    const finalReason = buildFinalWrapperReason(context);
    switch (context.forcedFinalTurnReason) {
      case 'context':
        push('final_turn_context', finalReason);
        break;
      case 'max_turns':
        push('final_turn_max_turns', finalReason);
        break;
      case 'task_status_completed':
        push('final_turn_task_status_completed', finalReason);
        break;
      case 'task_status_only':
        push('final_turn_task_status_only', finalReason);
        break;
      case 'retry_exhaustion':
        push('final_turn_retry_exhaustion', finalReason);
        break;
      default:
        push('final_turn_max_turns', finalReason);
    }
    return { events, nextOrder: order };
  }

  if (context.hasExternalTools) {
    push('tools_available_header');
    if (context.taskStatusToolEnabled) {
      push('tools_available_task_status_line');
    }
    push('tools_available_or', buildFinalWrapperReason(context));
    if (context.taskStatusToolEnabled) {
      push('tools_available_task_status_footer');
    }
  } else {
    push('tools_unavailable', buildFinalWrapperReason(context));
  }

  const lastRetryNonFinal = context.maxTurns !== undefined
    ? context.turn < (context.maxTurns - 1)
    : true;
  if (context.attempt === context.maxRetries && context.maxRetries > 0 && lastRetryNonFinal) {
    const reminderReason = buildReminderReason(context);
    push('last_retry_tool_reminder', reminderReason.length > 0 ? reminderReason : undefined);
  }

  return { events, nextOrder: order };
};
