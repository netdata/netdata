import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    pool: 'forks',
    maxWorkers: 1,
    isolate: false,
    testTimeout: 1800000, // 30 minutes - tier1 alone needs 10+ minutes with 4 models
    hookTimeout: 1800000,
    sequence: {
      concurrent: false,
    },
    coverage: {
      provider: 'v8',
      enabled: false,
    },
    include: ['src/tests/phase3/phase3-suite.spec.ts'],
  },
});
