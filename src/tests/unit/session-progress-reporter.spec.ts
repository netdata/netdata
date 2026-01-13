import { describe, expect, it } from 'vitest';

import type { ProgressEvent, TaskStatusData } from '../../types.js';

import { SessionProgressReporter } from '../../session-progress-reporter.js';

describe('SessionProgressReporter', () => {
  it('emits taskStatus on agent_update', () => {
    const events: ProgressEvent[] = [];
    const reporter = new SessionProgressReporter((event) => { events.push(event); });
    const taskStatus: TaskStatusData = {
      status: 'in-progress',
      done: 'step one',
      pending: 'step two',
      now: 'step one',
      ready_for_final_report: false,
      need_to_run_more_tools: true,
    };
    reporter.agentUpdate({
      callPath: 'root',
      agentId: 'agent-1',
      agentPath: 'agent-1',
      message: 'in-progress | step one | step two | step one',
      taskStatus,
    });
    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('agent_update');
    if (events[0].type === 'agent_update') {
      expect(events[0].taskStatus).toEqual(taskStatus);
    }
  });
});
