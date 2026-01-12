import { execFile } from 'node:child_process';
import path from 'node:path';

import { describe, it } from 'vitest';

const phaseThreeRunner = path.resolve('dist/tests/phase3-runner.js');
const tierArg = process.env.PHASE3_TIERS ?? '--tier=1';

const runPhaseThree = (): Promise<void> => new Promise((resolve, reject) => {
  const child = execFile(
    process.execPath,
    [phaseThreeRunner, tierArg],
    {
      env: { ...process.env },
      windowsHide: true,
    },
    (error, _stdout, stderr) => {
      if (error !== null) {
        const message = stderr.length > 0 ? `\n${stderr}` : '';
        reject(new Error(`phase3 harness failed: ${error.message}${message}`));
        return;
      }
      resolve();
    },
  );
  child.stdout?.pipe(process.stdout);
  child.stderr?.pipe(process.stderr);
});

describe('phase3 real LLM harness', () => {
  it('executes the configured tiers via CLI harness', async () => {
    await runPhaseThree();
  });
});
