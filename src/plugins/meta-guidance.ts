import type { ResolvedFinalReportPluginRequirement } from './types.js';

export interface MetaPromptGuidance {
  reminderShort: string;
  reminderShortLocked: string;
  reminderFull: string;
  reminderFullLocked: string;
  detailedBlock: string;
  exampleSnippets: string;
  xmlNextSnippets: string;
}

export const buildMetaWrapper = (nonce: string, pluginName: string): string => (
  `<ai-agent-${nonce}-META plugin="${pluginName}">{...}</ai-agent-${nonce}-META>`
);

const META_NONE_REQUIRED = 'META: none required for this session.';

const formatSchemaBlock = (schema: Record<string, unknown>): string => {
  const serialized = JSON.stringify(schema, null, 2);
  return `\`\`\`json\n${serialized}\n\`\`\``;
};

const formatPluginDetailedBlock = (
  requirement: ResolvedFinalReportPluginRequirement,
  nonce: string,
): string => {
  const wrapper = buildMetaWrapper(nonce, requirement.name);
  const schemaBlock = formatSchemaBlock(requirement.schema);
  return [
    `#### META Plugin: ${requirement.name}`,
    `Required wrapper: ${wrapper}`,
    '',
    'System instructions:',
    requirement.systemPromptInstructions,
    '',
    'Schema (must match exactly):',
    schemaBlock,
  ].join('\n');
};

const buildReminderShort = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): string => {
  if (requirements.length === 0) {
    return META_NONE_REQUIRED;
  }
  const pluginNames = requirements.map((requirement) => requirement.name).join(', ');
  return `META REQUIRED WITH FINAL. Plugins: ${pluginNames}. Use <ai-agent-${nonce}-META plugin="name">{...}</ai-agent-${nonce}-META>.`;
};

const buildReminderShortLocked = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): string => {
  if (requirements.length === 0) {
    return META_NONE_REQUIRED;
  }
  const pluginNames = requirements.map((requirement) => requirement.name).join(', ');
  return `FINAL already accepted. Provide required META wrappers only. Plugins: ${pluginNames}. Use <ai-agent-${nonce}-META plugin="name">{...}</ai-agent-${nonce}-META>.`;
};

const buildReminderFull = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): string => {
  if (requirements.length === 0) {
    return META_NONE_REQUIRED;
  }
  const wrappers = requirements.map((requirement) => buildMetaWrapper(nonce, requirement.name)).join(' | ');
  return `META REQUIRED WITH FINAL. Exact META wrappers: ${wrappers}.`;
};

const buildReminderFullLocked = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): string => {
  if (requirements.length === 0) {
    return META_NONE_REQUIRED;
  }
  const wrappers = requirements.map((requirement) => buildMetaWrapper(nonce, requirement.name)).join(' | ');
  return `FINAL already accepted. Provide required META wrappers only: ${wrappers}.`;
};

const buildDetailedBlock = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): string => {
  if (requirements.length === 0) {
    return [
      '### META Requirements',
      'No META blocks are required for this session.',
    ].join('\n');
  }

  const wrappers = requirements.map((requirement) => buildMetaWrapper(nonce, requirement.name));
  const wrapperList = wrappers.map((wrapper) => `- ${wrapper}`).join('\n');
  const pluginBlocks = requirements
    .map((requirement) => formatPluginDetailedBlock(requirement, nonce))
    .join('\n\n');

  return [
    '### META Requirements (Mandatory With FINAL)',
    'Your task is complete only when BOTH of the following are true:',
    '- You output the FINAL wrapper with the final report/answer.',
    '- You output ALL required META wrappers listed below.',
    '',
    'Required META wrappers (exact tags):',
    wrapperList,
    '',
    pluginBlocks,
  ].join('\n');
};

const buildExampleSnippets = (requirements: ResolvedFinalReportPluginRequirement[]): string => (
  requirements.length === 0
    ? 'META examples: no META blocks are required for this session.'
    : requirements.map((requirement) => requirement.finalReportExampleSnippet).join('\n\n')
);

const buildXmlNextSnippets = (requirements: ResolvedFinalReportPluginRequirement[]): string => (
  requirements.length === 0
    ? 'META reminder: no META blocks are required for this session.'
    : requirements.map((requirement) => requirement.xmlNextSnippet).join('\n')
);

export const buildMetaPromptGuidance = (
  requirements: ResolvedFinalReportPluginRequirement[],
  nonce: string,
): MetaPromptGuidance => ({
  reminderShort: buildReminderShort(requirements, nonce),
  reminderShortLocked: buildReminderShortLocked(requirements, nonce),
  reminderFull: buildReminderFull(requirements, nonce),
  reminderFullLocked: buildReminderFullLocked(requirements, nonce),
  detailedBlock: buildDetailedBlock(requirements, nonce),
  exampleSnippets: buildExampleSnippets(requirements),
  xmlNextSnippets: buildXmlNextSnippets(requirements),
});
