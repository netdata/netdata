import { runPhaseOneSuite } from './phase1/runner.js';

const toErrorMessage = (value: unknown): string => (value instanceof Error ? value.message : String(value));

runPhaseOneSuite()
  .then(() => {
    process.exit(0);
  })
  .catch((error: unknown) => {
    const message = toErrorMessage(error);
    console.error(`phase1 scenario failed: ${message}`);
    process.exit(1);
  });
