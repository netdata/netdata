/**
 * Shared constants for the Phase 2 deterministic test harness.
 * Extracted from phase2-runner.ts for use across test suite files.
 *
 * Note: MODEL_NAME, PRIMARY_PROVIDER, SECONDARY_PROVIDER are in harness-mocks.ts
 */

import { MODEL_NAME, PRIMARY_PROVIDER } from './harness-mocks.js';

// Derived identifiers
export const PRIMARY_REMOTE = `${PRIMARY_PROVIDER}:${MODEL_NAME}`;

// Remote log identifiers
export const RETRY_REMOTE = 'agent:retry';
export const FINAL_TURN_REMOTE = 'agent:final-turn';
export const CONTEXT_REMOTE = 'agent:context';

// Tool identifiers
export const SUBAGENT_TOOL = 'agent__pricing-subagent';
export const COVERAGE_CHILD_TOOL = 'coverage.child';
export const ROUTER_HANDOFF_TOOL = 'router__handoff-to';

// Content fragments for log matching
export const COVERAGE_PARENT_BODY = 'Parent body.';
export const COVERAGE_CHILD_BODY = 'Child body.';
export const CONTEXT_OVERFLOW_FRAGMENT = 'context window budget exceeded';
export const CONTEXT_FORCING_FRAGMENT = 'Forcing final turn';
export const BACKING_OFF_FRAGMENT = 'backing off';
export const FINAL_TURN_FRAGMENT = 'final turn';
export const RETRY_EXHAUSTED_MESSAGE = 'retry exhausted';
export const CONTEXT_LIMIT_TURN_WARN = 'Context limit exceeded during turn execution; proceeding with final turn.';

// Tokenizer identifiers
export const TOKENIZER_GPT4O = 'tiktoken:gpt-4o-mini';

// System prompts
export const MINIMAL_SYSTEM_PROMPT = 'Phase 1 deterministic harness: minimal instructions.';
export const HISTORY_SYSTEM_SEED = 'Historical system directive for counter seeding.';
export const HISTORY_ASSISTANT_SEED = 'Historical assistant summary preserved for context metrics.';
export const THRESHOLD_SYSTEM_PROMPT = 'Phase 1 deterministic harness: threshold probe instructions.';

// User prompts
export const THRESHOLD_USER_PROMPT = 'context_guard__threshold_probe';
export const THRESHOLD_ABOVE_USER_PROMPT = 'context_guard__threshold_above_probe';

// Test scenario ID constants
export const RUN_TEST_11 = 'run-test-11';
export const RUN_TEST_21 = 'run-test-21';
export const RUN_TEST_31 = 'run-test-31';
export const RUN_TEST_33 = 'run-test-33';
export const RUN_TEST_37 = 'run-test-37';
export const RUN_TEST_MAX_TURN_LIMIT = 'run-test-max-turn-limit';

// Expected result strings
export const TEXT_EXTRACTION_RETRY_RESULT = 'Valid report after retry.';
export const TEXT_EXTRACTION_INVALID_TEXT_RESULT = 'Valid report after invalid text.';
export const PURE_TEXT_RETRY_RESULT = 'Valid report after pure text retry.';
export const COLLAPSE_RECOVERY_RESULT = 'Proper result after collapse.';
export const COLLAPSE_FIXED_RESULT = 'Fixed after collapse.';
export const MAX_RETRY_SUCCESS_RESULT = 'Success after retries.';

// Task status constants
export const TASK_STATUS_IN_PROGRESS = 'in-progress';
export const TASK_STATUS_COMPLETED = 'completed';
export const TASK_CONTINUE_PROCESSING = 'Continue processing';
export const TASK_COMPLETE_TASK = 'Complete task';
export const TASK_COMPLETED_RESPONSE = 'task completed';
export const TASK_ANALYSIS_COMPLETE = 'Task analysis complete';
export const STATUS_UPDATE_RESPONSE = 'reporting status update';
export const TASK_COMPLETE_REPORT = 'Task complete';

// Router/handoff constants
export const ROUTER_ROUTE_MESSAGE = 'Route to child';
export const ROUTER_CHILD_REPORT_CONTENT = 'Child final report.';
export const ROUTER_PARENT_REPORT_CONTENT = 'Parent final report.';

// Think tag fragments
export const THINK_TAG_INNER = 'Example reasoning with <ai-agent-EXAMPLE-FINAL format="markdown">not real';
export const THINK_TAG_STREAM_FRAGMENT = 'Think tag stream coverage.';
export const THINK_TAG_NONSTREAM_FRAGMENT = 'Think tag non-stream coverage.';

// Tool output constants
export const TOOL_OUTPUT_HANDLE_PREFIX = 'Tool output is too large (';
export const TOOL_OUTPUT_HANDLE_PATTERN = /tool_output\(handle = "[^"]+"/;
export const TOOL_OUTPUT_STORE_LOG_FRAGMENT = 'output stored for tool_output';

// Log identifiers
export const LOG_FAILURE_REPORT = 'agent:failure-report';
export const LOG_ORCHESTRATOR = 'agent:orchestrator';

// Snapshot markers
export const SNAPSHOT_FULL_MARKER = 'SNAPSHOT-FULL-PAYLOAD-END';

// Shared registry constants
export const SHARED_REGISTRY_RESULT = 'shared-response';
export const SECOND_TURN_FINAL_ANSWER = 'Second turn final answer.';

// Restart fixture constants
export const RESTART_TRIGGER_PAYLOAD = 'restart-cycle';
export const RESTART_POST_PAYLOAD = 'post-restart';
export const FIXTURE_PREEMPT_REASON = 'fixture-preempt';

// Default prompt scenario
export const DEFAULT_PROMPT_SCENARIO = 'run-test-1' as const;

// Final report scenario identifiers
export const FINAL_REPORT_RETRY_TEXT_SCENARIO = 'run-test-final-report-retry-text';
export const FINAL_REPORT_TOOL_MESSAGE_SCENARIO = 'run-test-final-report-tool-message-fallback';
export const FINAL_REPORT_MAX_RETRIES_SCENARIO = 'run-test-final-report-max-retries-synthetic';
