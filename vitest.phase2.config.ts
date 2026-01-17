import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    pool: 'forks',
    maxWorkers: 1,
    isolate: false,
    testTimeout: 300000,
    hookTimeout: 300000,
    sequence: {
      concurrent: false,
    },
    coverage: {
      provider: 'v8',
      enabled: false,
    },
    include: ['src/tests/phase2/phase2-suite.spec.ts'],
  },
});
