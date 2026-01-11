import { execFile } from 'node:child_process';
import path from 'node:path';

import { describe, it } from 'vitest';

const harnessEntry = path.resolve('dist/tests/phase2-harness.js');

const runHarness = (): Promise<void> => new Promise((resolve, reject) => {
  const childEnv = { ...process.env } as Record<string, string | undefined>;
  delete childEnv.CI;
  delete childEnv.ci;
  const child = execFile(
    process.execPath,
    [harnessEntry],
    {
      env: childEnv,
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
  it('executes all registered scenarios via CLI harness', async () => {
    await runHarness();
  });
});
