/**
 * Phase 2 deterministic test harness infrastructure.
 * Re-exports all infrastructure modules for convenience.
 */

// Type definitions
export type {
  ExecuteTurnHandler,
  HarnessTest,
  PassThroughHandler,
  PhaseOneRunOptions,
  ScenarioRunOutput,
  TestContext,
  TestRunResult,
} from './harness-types.js';

// Helper functions
export {
  assertRecord,
  createDeferred,
  decodeBase64,
  delay,
  expectLogDetailNumber,
  expectLogEvent,
  expectRecord,
  expectTurnFailureContains,
  expectTurnFailureSlugs,
  extractNonceFromMessages,
  findLogByEvent,
  findLogByIdentifier,
  findTurnFailedMessage,
  formatDurationMs,
  getLogDetail,
  getLogsByIdentifier,
  getPrivateField,
  getPrivateMethod,
  invariant,
  isRecord,
  LOG_EVENTS,
  LOG_TURN_FAILURE,
  logHasDetail,
  logHasEvent,
  parseDumpList,
  parseScenarioFilter,
  RUN_LOG_DECIMAL_PRECISION,
  runWithTimeout,
  safeJsonByteLengthLocal,
  estimateMessagesBytesLocal,
  stripAnsiCodes,
  toError,
  toErrorMessage,
} from './harness-helpers.js';

// Mock classes and factories
export {
  BASE_DEFAULTS,
  buildInMemoryConfigLayers,
  createParentSessionStub,
  defaultPersistenceCallbacks,
  HarnessSharedHandle,
  HarnessSharedRegistry,
  isLlmAccounting,
  isToolAccounting,
  makeBasicConfiguration,
  makeSuccessResult,
  makeTempDir,
  MODEL_NAME,
  PRIMARY_PROVIDER,
  SECONDARY_PROVIDER,
  TMP_PREFIX,
} from './harness-mocks.js';

// Runner functions
export {
  cleanupActiveHandles,
  DEFAULT_SCENARIO_TIMEOUT_MS,
  formatFailureHint,
  getEffectiveTimeoutMs,
  PHASE2_FORCE_SEQUENTIAL,
  PHASE2_MODE,
  PHASE2_PARALLEL_CONCURRENCY,
  resetStateBetweenBatches,
  resetStateBetweenSequentialTests,
  runParallelBatch,
  runWithExecuteTurnOverride,
  runWithPassThroughCapture,
} from './harness-runner.js';
