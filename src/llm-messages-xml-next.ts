import type { OutputFormatId } from './formats.js';

/**
 * XML-NEXT message builder using a 3-part structure:
 * 1. Header - Turn/attempt/context status (always present)
 * 2. Warnings - Optional warnings for wasted turns, final turn reasons
 * 3. Footer - Instructions for next move (always present)
 */

// ============================================================================
// Single Source of Truth: Message Constants
// ============================================================================

export interface XmlNextSlugConfig {
  message: string;
  priority: number;
  stop?: boolean;
}

/**
 * All message strings used by XML-NEXT notices.
 * This is the SINGLE SOURCE OF TRUTH - builder functions reference these.
 * Tests also use these to verify correct message output.
 */
export const XML_NEXT_SLUGS = {
  // Final turn warnings (stop: true means session should end)
  final_turn_context: {
    message: 'You run out of context window. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.',
    priority: 1,
    stop: true,
  },
  final_turn_max_turns: {
    message: 'You are not allowed to run more turns. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.',
    priority: 1,
    stop: true,
  },
  final_turn_task_status_completed: {
    message: 'You marked the task-status as completed. You MUST now provide your final report/answer.',
    priority: 1,
    stop: true,
  },
  final_turn_task_status_only: {
    message: 'You are repeatedly calling task-status without any other tools or a final report/answer and the system stopped you to prevent infinite loops. You MUST now provide your final report/answer, even if incomplete. If you have not completed the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.',
    priority: 1,
    stop: true,
  },
  final_turn_retry_exhaustion: {
    message: 'All retry attempts have been exhausted. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.',
    priority: 1,
    stop: true,
  },
  final_turn_user_stop: {
    message: 'The user has requested to stop. You MUST now provide your final report/answer, summarizing your progress so far. If you have not finished the task, state clearly what was completed and what remains. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. The user may call you again to complete the task.',
    priority: 1,
    stop: true,
  },

  // Wasted turn warning (base message - occurrence count appended dynamically)
  turn_wasted_task_status_only: {
    message: 'Turn wasted: you called task-status without any other tools and without providing a final report/answer, so a turn has been wasted without any progress on your task. CRITICAL: CALL task-status ONLY TOGETHER WITH OTHER TOOLS. Focus on making progress towards your final report/answer. Do not call task-status again.',
    priority: 2,
  },

  // Footer messages
  tools_available_header: {
    message: 'You now need to decide your next move:',
    priority: 5,
  },
  tools_available_call_tools: {
    message: '- Call tools to advance your task following the main prompt instructions (pay attention to tool formatting and schema requirements).',
    priority: 5,
  },
  tools_available_task_status_line: {
    message: '- Together with these tool calls, also call `agent__task_status` to inform your user about what you are doing and why you are calling these tools.',
    priority: 6,
  },
  tools_available_or: {
    message: 'OR',
    priority: 7,
  },
  tools_unavailable: {
    message: 'You must now provide your final report/answer in the expected format',
    priority: 5,
  },
  last_retry_tool_reminder: {
    message: 'Reminder: do not end with plain text. Use an available tool to make progress. When ready to conclude, provide your final report/answer in the required XML wrapper.',
    priority: 9,
  },
} as const satisfies Record<string, XmlNextSlugConfig>;

const ROUTER_HANDOFF_TOOL = 'router__handoff-to';

// Type for final turn reasons - derived from the slug keys
type FinalTurnReason = 'context' | 'max_turns' | 'task_status_completed' | 'task_status_only' | 'retry_exhaustion' | 'user_stop';

// Map from reason to slug key
const FINAL_TURN_SLUG_MAP: Record<FinalTurnReason, keyof typeof XML_NEXT_SLUGS> = {
  context: 'final_turn_context',
  max_turns: 'final_turn_max_turns',
  task_status_completed: 'final_turn_task_status_completed',
  task_status_only: 'final_turn_task_status_only',
  retry_exhaustion: 'final_turn_retry_exhaustion',
  user_stop: 'final_turn_user_stop',
};

// ============================================================================
// Context Interface
// ============================================================================

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
  forcedFinalTurnReason?: FinalTurnReason;
  finalTurnTools?: string[];
  // Wasted turn tracking
  consecutiveProgressOnlyTurns?: number;
}

// ============================================================================
// Part 1: Header Builder
// ============================================================================

const buildHeader = (context: XmlNextNoticeContext): string => {
  const parts: string[] = [];

  // Turn info
  const turnPart = context.maxTurns !== undefined
    ? `This is turn/step ${String(context.turn)} of ${String(context.maxTurns)}`
    : `This is turn/step ${String(context.turn)}`;
  parts.push(turnPart);

  // Attempt info (only shown on retry, i.e., attempt > 1)
  if (context.attempt > 1) {
    parts.push(`, attempt ${String(context.attempt)} of ${String(context.maxRetries)}`);
  }

  // End turn info with period
  parts.push('.');

  // Context window info
  parts.push(` Your context window is ${String(context.contextPercentUsed)}% full.`);

  return parts.join('');
};

// ============================================================================
// Part 2: Warnings Builder
// ============================================================================

type WarningType = 'wasted_turn' | 'final_turn';

interface Warning {
  type: WarningType;
  message: string;
}

const hasRouterHandoffTool = (context: XmlNextNoticeContext): boolean => (
  Array.isArray(context.finalTurnTools)
  && context.finalTurnTools.some((tool) => tool === ROUTER_HANDOFF_TOOL)
);

const appendRouterHandoffOption = (message: string, allowRouterHandoff: boolean): string => {
  if (!allowRouterHandoff) return message;
  return `${message} OR call \`${ROUTER_HANDOFF_TOOL}\` to hand off the user request to another agent.`;
};

const buildWastedTurnWarning = (consecutiveCount: number): Warning | undefined => {
  if (consecutiveCount <= 0) return undefined;

  // Use base message from slugs, append occurrence count
  const baseMessage = XML_NEXT_SLUGS.turn_wasted_task_status_only.message;
  return {
    type: 'wasted_turn',
    message: `${baseMessage} (This is occurrence ${String(consecutiveCount)} of 5 before forced finalization)`,
  };
};

const buildFinalTurnWarning = (reason: FinalTurnReason, allowRouterHandoff: boolean): Warning => {
  const slugKey = FINAL_TURN_SLUG_MAP[reason];
  const message = appendRouterHandoffOption(XML_NEXT_SLUGS[slugKey].message, allowRouterHandoff);
  return {
    type: 'final_turn',
    message,
  };
};

const buildWarnings = (context: XmlNextNoticeContext): Warning[] => {
  const warnings: Warning[] = [];
  const allowRouterHandoff = hasRouterHandoffTool(context);

  // Wasted turn warning (only shown when not on forced final turn)
  if (context.forcedFinalTurnReason === undefined) {
    const wastedWarning = buildWastedTurnWarning(context.consecutiveProgressOnlyTurns ?? 0);
    if (wastedWarning !== undefined) {
      warnings.push(wastedWarning);
    }
  }

  // Final turn warning
  if (context.forcedFinalTurnReason !== undefined) {
    warnings.push(buildFinalTurnWarning(context.forcedFinalTurnReason, allowRouterHandoff));
  }

  return warnings;
};

// ============================================================================
// Part 3: Footer Builder
// ============================================================================

const buildFinalWrapperExample = (context: XmlNextNoticeContext): string => {
  const finalSlotId = `${context.nonce}-FINAL`;
  return `<ai-agent-${finalSlotId} format="${context.expectedFinalFormat}">[final report/answer]</ai-agent-${finalSlotId}>`;
};

const buildNonFinalFooter = (context: XmlNextNoticeContext): string => {
  const wrapperExample = buildFinalWrapperExample(context);
  const lines: string[] = [];

  if (context.hasExternalTools) {
    lines.push(XML_NEXT_SLUGS.tools_available_header.message);
    lines.push('EITHER');
    lines.push(XML_NEXT_SLUGS.tools_available_call_tools.message);

    if (context.taskStatusToolEnabled) {
      lines.push(XML_NEXT_SLUGS.tools_available_task_status_line.message);
    }

    lines.push(XML_NEXT_SLUGS.tools_available_or.message);
    lines.push(`- Provide your final report/answer in the expected format (${context.expectedFinalFormat}) using the XML wrapper: ${wrapperExample}`);
  } else {
    // No external tools - must provide final report
    lines.push(`${XML_NEXT_SLUGS.tools_unavailable.message} (${context.expectedFinalFormat}) using the XML wrapper: ${wrapperExample}`);
  }

  return lines.join('\n');
};

const buildFinalTurnFooter = (context: XmlNextNoticeContext): string => {
  const wrapperExample = buildFinalWrapperExample(context);
  const lines: string[] = [];
  const allowRouterHandoff = hasRouterHandoffTool(context);
  const finalLine = appendRouterHandoffOption(
    `${XML_NEXT_SLUGS.tools_unavailable.message} (${context.expectedFinalFormat}) using the XML wrapper: ${wrapperExample}`,
    allowRouterHandoff
  );

  lines.push(finalLine);

  // Allowed tools for final turn
  if (Array.isArray(context.finalTurnTools) && context.finalTurnTools.length > 0) {
    lines.push('');
    lines.push(`Allowed tools for this final turn: ${context.finalTurnTools.join(', ')}`);
  }

  return lines.join('\n');
};

const buildLastRetryReminder = (context: XmlNextNoticeContext): string | undefined => {
  // Only show on last retry when not on final turn
  if (context.forcedFinalTurnReason !== undefined) return undefined;

  const lastRetryNonFinal = context.maxTurns !== undefined
    ? context.turn < (context.maxTurns - 1)
    : true;

  if (context.attempt === context.maxRetries && context.maxRetries > 0 && lastRetryNonFinal) {
    return XML_NEXT_SLUGS.last_retry_tool_reminder.message;
  }

  return undefined;
};

const buildFooter = (context: XmlNextNoticeContext): string => {
  const lines: string[] = [];

  // Main footer content based on final vs non-final turn
  if (context.forcedFinalTurnReason !== undefined) {
    lines.push(buildFinalTurnFooter(context));
  } else {
    lines.push(buildNonFinalFooter(context));
  }

  // Last retry reminder
  const reminder = buildLastRetryReminder(context);
  if (reminder !== undefined) {
    lines.push('');
    lines.push(reminder);
  }

  return lines.join('\n');
};

// ============================================================================
// Main Builder
// ============================================================================

/**
 * Builds the XML-NEXT system notice with 3-part structure:
 * 1. Header - Turn/attempt/context status
 * 2. Warnings - Optional warnings (wasted turns, final turn reasons)
 * 3. Footer - Instructions for next move
 */
export const buildXmlNextNotice = (context: XmlNextNoticeContext): string => {
  const parts: string[] = [];

  // Title
  parts.push('# System Notice');
  parts.push('');

  // Part 1: Header (always present)
  parts.push(buildHeader(context));

  // Part 2: Warnings (optional)
  const warnings = buildWarnings(context);
  if (warnings.length > 0) {
    parts.push('');
    warnings.forEach((w) => {
      parts.push(w.message);
    });
  }

  // Part 3: Footer (always present)
  parts.push('');
  parts.push(buildFooter(context));

  // Trailing newline for consistency
  parts.push('');

  return parts.join('\n');
};
