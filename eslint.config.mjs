// Aggressive ESLint configuration (flat config) for strict TypeScript projects
// Uses type-aware rules and a wide set of community plugins.
// Note: requires the listed devDependencies to be installed in the workspace.

import tseslint from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';
import importPlugin from 'eslint-plugin-import';
import unicorn from 'eslint-plugin-unicorn';
import sonarjs from 'eslint-plugin-sonarjs';
import security from 'eslint-plugin-security';
import jsdoc from 'eslint-plugin-jsdoc';
import promise from 'eslint-plugin-promise';
import functional from 'eslint-plugin-functional';
import totalFunctions from 'eslint-plugin-total-functions';
import deprecation from 'eslint-plugin-deprecation';
import eslintComments from 'eslint-plugin-eslint-comments';
import n from 'eslint-plugin-n';
import perfectionist from 'eslint-plugin-perfectionist';
import noSecrets from 'eslint-plugin-no-secrets';
import regexp from 'eslint-plugin-regexp';

const tsProject = ['./codex/tsconfig.json'];

export default [
  // Base for JS files if any
  {
    files: ['**/*.js', '**/*.cjs', '**/*.mjs'],
    ignores: ['**/node_modules/**', '**/dist/**', '**/libs/**'],
    languageOptions: { ecmaVersion: 'latest', sourceType: 'module' },
    plugins: { import: importPlugin, unicorn, promise, sonarjs, security, jsdoc, 'eslint-comments': eslintComments, n, perfectionist, regexp },
    rules: {
      'no-var': 'error',
      'prefer-const': 'error',
      'no-console': ['warn', { allow: ['error'] }],
      'import/no-default-export': 'error',
      'import/order': ['error', { 'newlines-between': 'always', alphabetize: { order: 'asc' } }],
      'unicorn/prefer-node-protocol': 'error',
      'unicorn/filename-case': ['error', { case: 'kebabCase' }],
      'promise/prefer-await-to-then': 'error',
      'sonarjs/no-duplicate-string': 'warn',
      'security/detect-object-injection': 'off',
      'jsdoc/require-jsdoc': 'off',
      'eslint-comments/no-unused-disable': 'error',
      'n/no-missing-import': 'off',
      'perfectionist/sort-imports': ['error', { type: 'natural', order: 'asc' }],
      'regexp/no-dupe-characters-character-class': 'error',
    },
  },
  // Strict TypeScript rules (type-aware)
  {
    files: ['**/*.ts', '**/*.tsx'],
    ignores: ['**/node_modules/**', '**/dist/**', '**/libs/**'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: tsProject,
        tsconfigRootDir: new URL('.', import.meta.url),
        sourceType: 'module',
        ecmaVersion: 'latest',
      },
    },
    plugins: {
      '@typescript-eslint': tseslint,
      import: importPlugin,
      unicorn,
      sonarjs,
      security,
      jsdoc,
      promise,
      functional,
      'total-functions': totalFunctions,
      deprecation,
      'eslint-comments': eslintComments,
      n,
      perfectionist,
      'no-secrets': noSecrets,
      regexp,
    },
    rules: {
      // typescript-eslint strict + stylistic + recommended type-checked selections
      ...tseslint.configs['strict-type-checked']?.rules,
      ...tseslint.configs['stylistic-type-checked']?.rules,

      // Additional strictness
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unsafe-assignment': 'error',
      '@typescript-eslint/no-unsafe-call': 'error',
      '@typescript-eslint/no-unsafe-member-access': 'error',
      '@typescript-eslint/no-unsafe-return': 'error',
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/require-array-sort-compare': ['error', { ignoreStringArrays: false }],
      '@typescript-eslint/strict-boolean-expressions': ['error', { allowNullableObject: false }],
      '@typescript-eslint/no-confusing-void-expression': 'error',
      '@typescript-eslint/method-signature-style': ['error', 'property'],
      '@typescript-eslint/prefer-nullish-coalescing': 'error',
      '@typescript-eslint/prefer-optional-chain': 'error',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],

      // Functional and totality
      'functional/no-try-statements': 'off',
      'functional/no-loop-statements': 'warn',
      'functional/no-throw-statements': 'off',
      'total-functions/no-mutable-params': 'error',

      // Import hygiene
      'import/no-default-export': 'error',
      'import/no-cycle': 'error',
      'import/order': ['error', { 'newlines-between': 'always', alphabetize: { order: 'asc' } }],

      // Unicorn and others
      'unicorn/prefer-node-protocol': 'error',
      'unicorn/no-abusive-eslint-disable': 'error',
      'unicorn/filename-case': ['error', { case: 'kebabCase' }],

      // Security and quality
      'security/detect-object-injection': 'off',
      'sonarjs/no-duplicate-string': 'warn',
      'no-secrets/no-secrets': ['warn', { tolerance: 4.5 }],

      // Promises, comments, etc.
      'promise/prefer-await-to-then': 'error',
      'eslint-comments/no-unused-disable': 'error',

      // Sorting
      'perfectionist/sort-imports': ['error', { type: 'natural', order: 'asc' }],
    },
  },
  // Root ignores
  {
    ignores: ['**/node_modules/**', '**/dist/**', '**/.git/**', '**/libs/**'],
  },
];

