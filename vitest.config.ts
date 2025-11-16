import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',
    pool: 'forks',
    poolOptions: {
      forks: {
        singleFork: true,
      },
    },
    testTimeout: 300000,
    hookTimeout: 300000,
    sequence: {
      concurrent: false,
    },
    coverage: {
      provider: 'v8',
      enabled: false,
    },
    include: ['src/tests/**/*.spec.ts'],
    exclude: ['src/tests/phase2/**'],
  },
});
