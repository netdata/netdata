/**
 * Prompt loader - loads all prompts at module initialization (PR-001 compliant).
 * All prompts are read synchronously at import time, not lazily at runtime.
 */
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import type { ResolvedFinalReportPluginRequirement } from '../plugins/types.js';

import { buildMetaPromptGuidance } from '../plugins/meta-guidance.js';
import { SLACK_BLOCK_KIT_MRKDWN_RULES } from '../slack-block-kit.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const PROMPTS_DIR = join(__dirname, '..', 'prompts');

// -----------------------------------------------------------------------------
// Load all prompts at module initialization (PR-001: load-time, not runtime)
// -----------------------------------------------------------------------------

function loadPromptSync(fileName: string): string {
  const filePath = join(PROMPTS_DIR, fileName);
  try {
    return readFileSync(filePath, 'utf-8');
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : String(error);
    if (error instanceof Error && 'code' in error && error.code === 'ENOENT') {
      throw new Error(`LLM prompt file not found: ${filePath}`);
    }
    throw new Error(`Failed to read LLM prompt file: ${filePath} - ${errorMessage}`);
  }
}

// All prompts loaded at module initialization
const TASK_STATUS_TEMPLATE = loadPromptSync('task-status.md');
const FINAL_REPORT_TEMPLATE = loadPromptSync('final-report.md');
const MANDATORY_RULES_TEMPLATE = loadPromptSync('mandatory-rules.md');
const BATCH_WITH_PROGRESS_TEMPLATE = loadPromptSync('batch-with-progress.md');
const BATCH_WITHOUT_PROGRESS_TEMPLATE = loadPromptSync('batch-without-progress.md');

// -----------------------------------------------------------------------------
// Exported functions - return pre-loaded templates (no file I/O at runtime)
// -----------------------------------------------------------------------------

export function loadTaskStatusInstructions(metaReminderShort: string): string {
  return TASK_STATUS_TEMPLATE.replace(/\{\{\{metaReminderShort\}\}\}/g, metaReminderShort);
}

export function loadFinalReportInstructions(
  formatId: string,
  formatDescription: string,
  schemaBlock: string,
  sessionNonce: string | undefined,
  pluginRequirements: ResolvedFinalReportPluginRequirement[],
): string {
  // Caller MUST provide a nonce - without it, LLM will output literal 'NONCE-FINAL'
  if (sessionNonce === undefined) {
    throw new Error('sessionNonce is required for XML final report instructions');
  }

  const metaGuidance = buildMetaPromptGuidance(pluginRequirements, sessionNonce);

  const slotId = `${sessionNonce}-FINAL`;
  const exampleContent = formatId === 'slack-block-kit'
    ? '[ { "blocks": [ ... ] } ]'
    : formatId === 'json'
      ? '{ ... your JSON here ... }'
      : '[Your final report/answer here]';

  // Include actual Slack mrkdwn rules, not a placeholder
  const slackMrkdwnGuidance = formatId === 'slack-block-kit'
    ? `\n${SLACK_BLOCK_KIT_MRKDWN_RULES}\n`
    : '';

  return FINAL_REPORT_TEMPLATE
    .replace(/\{\{\{slotId\}\}\}/g, slotId)
    .replace(/\{\{\{formatId\}\}\}/g, formatId)
    .replace(/\{\{\{exampleContent\}\}\}/g, exampleContent)
    .replace(/\{\{\{formatDescription\}\}\}/g, formatDescription)
    .replace(/\{\{\{schemaBlock\}\}\}/g, schemaBlock)
    .replace(/\{\{\{slackMrkdwnGuidance\}\}\}/g, slackMrkdwnGuidance)
    .replace(/\{\{\{metaReminderShort\}\}\}/g, metaGuidance.reminderShort)
    .replace(/\{\{\{metaReminderFull\}\}\}/g, metaGuidance.reminderFull)
    .replace(/\{\{\{metaDetailedBlock\}\}\}/g, metaGuidance.detailedBlock)
    .replace(/\{\{\{metaExampleSnippets\}\}\}/g, metaGuidance.exampleSnippets);
}

export function loadMandatoryRules(metaReminderShort: string): string {
  return MANDATORY_RULES_TEMPLATE.replace(/\{\{\{metaReminderShort\}\}\}/g, metaReminderShort);
}

export function loadBatchInstructions(hasProgressTool: boolean, metaReminderShort: string): string {
  const template = hasProgressTool ? BATCH_WITH_PROGRESS_TEMPLATE : BATCH_WITHOUT_PROGRESS_TEMPLATE;
  return template.replace(/\{\{\{metaReminderShort\}\}\}/g, metaReminderShort);
}
