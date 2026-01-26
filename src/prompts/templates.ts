import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import type { LoadedTemplate, TemplateEngine } from './template-engine.js';

import { collectTemplateSources, createTemplateEngine, loadTemplate, renderTemplate } from './template-engine.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const PROMPTS_DIR = join(__dirname, '..', 'prompts');

const TEMPLATE_FILES = {
  batchWithProgress: 'batch-with-progress.md',
  batchWithoutProgress: 'batch-without-progress.md',
  finalReport: 'final-report.md',
  internalTools: 'internal-tools.md',
  mandatoryRules: 'mandatory-rules.md',
  routerInstructions: 'router-instructions.md',
  taskStatus: 'task-status.md',
  xmlNext: 'xml-next.md',
  xmlPast: 'xml-past.md',
  turnFailed: 'turn-failed.md',
  toolOutputInstructions: 'tool-output/instructions.md',
  toolOutputHandle: 'tool-output/handle-message.md',
  toolOutputSuccess: 'tool-output/success-message.md',
  toolOutputFailure: 'tool-output/failure-message.md',
  toolOutputNoOutput: 'tool-output/no-output.md',
  toolOutputTruncated: 'tool-output/truncated-message.md',
  toolOutputErrorInvalidParams: 'tool-output/error-invalid-params.md',
  toolOutputErrorHandleMissing: 'tool-output/error-handle-missing.md',
  toolOutputErrorReadGrepFailed: 'tool-output/error-read-grep-failed.md',
  toolOutputErrorReadGrepSynthetic: 'tool-output/error-read-grep-synthetic.md',
  toolOutputWarningTruncate: 'tool-output/warning-truncate.md',
  toolOutputMapSystem: 'tool-output/map-system.md',
  toolOutputReduceSystem: 'tool-output/reduce-system.md',
  toolOutputReduceUser: 'tool-output/reduce-user.md',
  toolOutputReadGrepSystem: 'tool-output/read-grep-system.md',
  toolOutputReadGrepUser: 'tool-output/read-grep-user.md',
  toolSchemaFinalReport: 'tool-schemas/final-report.json',
  toolSchemaTaskStatus: 'tool-schemas/task-status.json',
  toolSchemaBatch: 'tool-schemas/batch.json',
  toolSchemaRouter: 'tool-schemas/router-handoff.json',
  toolSchemaToolOutput: 'tool-schemas/tool-output.json',
  toolResultUnknownTool: 'tool-results/unknown-tool.md',
  toolResultXmlWrapperCalledAsTool: 'tool-results/xml-wrapper-called-as-tool.md',
  toolResultFinalReportJsonRequired: 'tool-results/final-report-json-required.md',
  toolResultFinalReportSlackMissing: 'tool-results/final-report-slack-messages-missing.md',
} as const;

const TEMPLATE_SOURCES = collectTemplateSources(PROMPTS_DIR, Object.values(TEMPLATE_FILES));
const PROMPT_ENGINE: TemplateEngine = createTemplateEngine(TEMPLATE_SOURCES);

type TemplateKey = keyof typeof TEMPLATE_FILES;

const loadByKey = (key: TemplateKey): LoadedTemplate => loadTemplate(PROMPT_ENGINE, TEMPLATE_FILES[key]);

const TEMPLATE_REGISTRY: Record<TemplateKey, LoadedTemplate> = Object.fromEntries(
  (Object.keys(TEMPLATE_FILES) as TemplateKey[]).map((key) => [key, loadByKey(key)])
) as Record<TemplateKey, LoadedTemplate>;

export const renderPromptTemplate = (key: TemplateKey, context: Record<string, unknown>): string => (
  renderTemplate(PROMPT_ENGINE, TEMPLATE_REGISTRY[key], context)
);

export const getPromptTemplateSource = (key: TemplateKey): string => TEMPLATE_REGISTRY[key].source;
