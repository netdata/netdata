import type { ResolvedFinalReportPluginRequirement } from '../plugins/types.js';

import { SLACK_BLOCK_KIT_MRKDWN_RULES } from '../slack-block-kit.js';

import { renderPromptTemplate } from './templates.js';

export interface MetaTemplateContext {
  pluginRequirements: ResolvedFinalReportPluginRequirement[];
  nonce: string;
  responseMode?: 'agentic' | 'chat';
}

export interface FinalReportTemplateContext extends MetaTemplateContext {
  formatId: string;
  formatDescription: string;
  expectedJsonSchema?: Record<string, unknown>;
  slackSchema?: Record<string, unknown>;
}

export const renderTaskStatusInstructions = (context: MetaTemplateContext): string => (
  renderPromptTemplate('taskStatus', {
    plugin_requirements: context.pluginRequirements,
    nonce: context.nonce,
    final_report_locked: false,
    response_mode: context.responseMode ?? 'agentic',
  })
);

export const renderMandatoryRules = (context: MetaTemplateContext): string => (
  renderPromptTemplate('mandatoryRules', {
    plugin_requirements: context.pluginRequirements,
    nonce: context.nonce,
    final_report_locked: false,
    response_mode: context.responseMode ?? 'agentic',
  })
);

export const renderBatchInstructions = (args: { hasProgressTool: boolean } & MetaTemplateContext): string => (
  renderPromptTemplate(args.hasProgressTool ? 'batchWithProgress' : 'batchWithoutProgress', {
    plugin_requirements: args.pluginRequirements,
    nonce: args.nonce,
    final_report_locked: false,
    response_mode: args.responseMode ?? 'agentic',
  })
);

export const renderFinalReportInstructions = (context: FinalReportTemplateContext): string => (
  renderPromptTemplate('finalReport', {
    plugin_requirements: context.pluginRequirements,
    nonce: context.nonce,
    format_id: context.formatId,
    format_description: context.formatDescription,
    expected_json_schema: context.expectedJsonSchema ?? null,
    slack_schema: context.slackSchema ?? null,
    slack_mrkdwn_rules: SLACK_BLOCK_KIT_MRKDWN_RULES,
    response_mode: context.responseMode ?? 'agentic',
  })
);
