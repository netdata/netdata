/**
 * Test suite index - aggregates all category-based test suites.
 */

import type { HarnessTest } from '../infrastructure/index.js';

import { CONTEXT_GUARD_TESTS } from './context-guard.test.js';
import { CORE_ORCHESTRATION_TESTS } from './core-orchestration.test.js';
import { COVERAGE_TESTS } from './coverage.test.js';
import { ERROR_HANDLING_TESTS } from './error-handling.test.js';
import { FINAL_REPORT_TESTS } from './final-report.test.js';
import { FORMAT_SPECIFIC_TESTS } from './format-specific.test.js';
import { PLUGINS_TESTS } from './plugins.test.js';
import { REASONING_TESTS } from './reasoning.test.js';
import { ROUTER_HANDOFF_TESTS } from './router-handoff.test.js';
import { TASK_STATUS_TESTS } from './task-status.test.js';
import { TOOL_EXECUTION_TESTS } from './tool-execution.test.js';

/**
 * All test scenarios from the split test suites.
 * These are combined with BASE_TEST_SCENARIOS in phase2-runner.ts.
 */
export const SUITE_TEST_SCENARIOS: HarnessTest[] = [
  ...CONTEXT_GUARD_TESTS,
  ...CORE_ORCHESTRATION_TESTS,
  ...COVERAGE_TESTS,
  ...ERROR_HANDLING_TESTS,
  ...FINAL_REPORT_TESTS,
  ...FORMAT_SPECIFIC_TESTS,
  ...PLUGINS_TESTS,
  ...REASONING_TESTS,
  ...ROUTER_HANDOFF_TESTS,
  ...TASK_STATUS_TESTS,
  ...TOOL_EXECUTION_TESTS,
];

// Re-export individual suites for selective imports
export { CONTEXT_GUARD_TESTS } from './context-guard.test.js';
export { CORE_ORCHESTRATION_TESTS } from './core-orchestration.test.js';
export { COVERAGE_TESTS } from './coverage.test.js';
export { ERROR_HANDLING_TESTS } from './error-handling.test.js';
export { FINAL_REPORT_TESTS } from './final-report.test.js';
export { FORMAT_SPECIFIC_TESTS } from './format-specific.test.js';
export { PLUGINS_TESTS } from './plugins.test.js';
export { REASONING_TESTS } from './reasoning.test.js';
export { ROUTER_HANDOFF_TESTS } from './router-handoff.test.js';
export { TASK_STATUS_TESTS } from './task-status.test.js';
export { TOOL_EXECUTION_TESTS } from './tool-execution.test.js';
