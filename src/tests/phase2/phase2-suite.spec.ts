import { execFile } from 'node:child_process';
import path from 'node:path';

import { describe, it } from 'vitest';

const phaseTwoRunner = path.resolve('dist/tests/phase2-runner.js');
const tierArg = process.env.PHASE2_TIERS ?? '--tier=1';

const runPhaseTwo = (): Promise<void> => new Promise((resolve, reject) => {
  const child = execFile(
    process.execPath,
    [phaseTwoRunner, tierArg],
    {
      env: { ...process.env },
      windowsHide: true,
    },
    (error, _stdout, stderr) => {
      if (error !== null) {
        const message = stderr.length > 0 ? `\n${stderr}` : '';
        reject(new Error(`phase2 harness failed: ${error.message}${message}`));
        return;
      }
      resolve();
    },
  );
  child.stdout?.pipe(process.stdout);
  child.stderr?.pipe(process.stderr);
});

describe('phase2 deterministic harness', () => {
  it('executes the configured tiers via CLI harness', async () => {
    await runPhaseTwo();
  });
});
