/**
 * Type definitions for the Phase 2 deterministic test harness.
 * Extracted from phase2-runner.ts for modularity.
 */

import type { AIAgentResult, AIAgentSessionConfig, Configuration, TurnRequest, TurnResult } from '../../../types.js';

/**
 * Test context object passed between configure, execute, and expect phases.
 * Each test gets its own context instance - no shared global state needed.
 */
export type TestContext = Record<string, unknown>;

/**
 * Interface for a single harness test scenario.
 */
export interface HarnessTest {
  /** Unique identifier for the test scenario */
  id: string;
  /** Human-readable description of what the test validates */
  description?: string;
  /** Custom timeout in milliseconds (overrides DEFAULT_SCENARIO_TIMEOUT_MS) */
  timeoutMs?: number;
  /**
   * When true, this test MUST run sequentially (not in parallel with other tests).
   * Set this flag for tests that use shared global state variables (not DI-based).
   */
  sequential?: boolean;
  /**
   * Optional configuration phase - mutate configuration and session config before execution.
   * Called before execute() to set up test-specific configuration.
   */
  configure?: (
    configuration: Configuration,
    sessionConfig: AIAgentSessionConfig,
    defaults: NonNullable<Configuration['defaults']>,
    context: TestContext
  ) => void;
  /**
   * Optional custom execution phase - override default session execution.
   * If not provided, the harness creates a session and runs it with AIAgent.run().
   */
  execute?: (
    configuration: Configuration,
    sessionConfig: AIAgentSessionConfig,
    defaults: NonNullable<Configuration['defaults']>,
    context: TestContext
  ) => Promise<AIAgentResult>;
  /**
   * Assertion phase - validate the result after execution.
   * Throw an error (via invariant) if assertions fail.
   */
  expect: (result: AIAgentResult, context: TestContext) => void;
}

/**
 * Output from running a single scenario.
 */
export interface ScenarioRunOutput {
  /** The result from the agent session */
  result: AIAgentResult;
  /** The test context populated during execution */
  context: TestContext;
}

/**
 * Result from running a single test scenario.
 */
export interface TestRunResult {
  /** The test scenario that was executed */
  scenario: HarnessTest;
  /** The result from the agent session (undefined if execution failed before completion) */
  result?: AIAgentResult;
  /** Error if the test failed */
  error?: Error;
  /** Duration of the test in milliseconds */
  durationMs: number;
}

/**
 * Options for running the Phase 1/2 test suite.
 */
export interface PhaseOneRunOptions {
  /** Optional list of scenario IDs to run (filters the full suite) */
  readonly filterIds?: string[];
}

/**
 * Handler type for executeTurn override via DI.
 * Used by runWithExecuteTurnOverride to intercept LLM calls.
 */
export type ExecuteTurnHandler = (ctx: {
  /** The turn request being executed */
  request: TurnRequest;
  /** Invocation counter (1-indexed) */
  invocation: number;
}) => Promise<TurnResult>;

/**
 * Handler type for pass-through capture.
 * Return undefined to pass through to provider, or return a TurnResult to override.
 */
export type PassThroughHandler = (ctx: {
  /** The turn request being executed */
  request: TurnRequest;
  /** Invocation counter (1-indexed) */
  invocation: number;
  /** Test context for storing captured data */
  context: TestContext;
  /** Function to call the actual provider */
  passThrough: () => Promise<TurnResult>;
}) => Promise<TurnResult | undefined>;
