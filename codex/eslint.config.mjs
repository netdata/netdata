import tseslint from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';
import deprecation from 'eslint-plugin-deprecation';
import eslintComments from 'eslint-plugin-eslint-comments';
import functional from 'eslint-plugin-functional';
import importPlugin from 'eslint-plugin-import';
import jsdoc from 'eslint-plugin-jsdoc';
import n from 'eslint-plugin-n';
import noSecrets from 'eslint-plugin-no-secrets';
import perfectionist from 'eslint-plugin-perfectionist';
import promise from 'eslint-plugin-promise';
import regexp from 'eslint-plugin-regexp';
import security from 'eslint-plugin-security';
import sonarjs from 'eslint-plugin-sonarjs';
import unicorn from 'eslint-plugin-unicorn';

const tsProject = ['./tsconfig.json'];

const IGNORE_GLOBS = ['**/node_modules/**', '**/dist/**'];

export default [
  // Overrides for scripts
  {
    files: ['**/scripts/**'],
    rules: { 'no-console': 'off' },
  },
  {
    files: ['**/*.js', '**/*.cjs', '**/*.mjs'],
    ignores: IGNORE_GLOBS,
    languageOptions: { ecmaVersion: 'latest', sourceType: 'module' },
    plugins: { import: importPlugin, unicorn, promise, sonarjs, security, jsdoc, 'eslint-comments': eslintComments, n, perfectionist, regexp },
    rules: {
      'no-var': 'error',
      'prefer-const': 'error',
      'no-console': ['warn', { allow: ['error'] }],
      'import/no-default-export': 'error',
      'import/order': ['error', { groups: ['builtin','external','type','internal','parent','sibling','index','object'], 'newlines-between': 'always', alphabetize: { order: 'asc', caseInsensitive: true } }],
      'unicorn/prefer-node-protocol': 'error',
      'unicorn/filename-case': ['error', { case: 'kebabCase' }],
      'promise/prefer-await-to-then': 'error',
      'sonarjs/no-duplicate-string': 'warn',
      'security/detect-object-injection': 'off',
      'jsdoc/require-jsdoc': 'off',
      'eslint-comments/no-unused-disable': 'error',
      'n/no-missing-import': 'off',
      'perfectionist/sort-imports': ['error', { type: 'natural', order: 'asc', groups: ['builtin','external','type','internal','parent','sibling','index','object'] }],
      'regexp/no-dupe-characters-character-class': 'error',
    },
  },
  {
    files: ['**/*.ts', '**/*.tsx'],
    ignores: IGNORE_GLOBS,
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: tsProject,
        tsconfigRootDir: process.cwd(),
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
      deprecation,
      'eslint-comments': eslintComments,
      n,
      perfectionist,
      'no-secrets': noSecrets,
      regexp,
    },
    rules: {
      ...tseslint.configs['strict-type-checked']?.rules,
      ...tseslint.configs['stylistic-type-checked']?.rules,
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
      'functional/no-try-statements': 'off',
      'functional/no-loop-statements': 'warn',
      'functional/no-throw-statements': 'off',
      'import/no-default-export': 'error',
      'import/no-cycle': 'error',
      'import/order': ['error', { groups: ['builtin','external','type','internal','parent','sibling','index','object'], 'newlines-between': 'always', alphabetize: { order: 'asc', caseInsensitive: true } }],
      'unicorn/prefer-node-protocol': 'error',
      'unicorn/no-abusive-eslint-disable': 'error',
      'unicorn/filename-case': ['error', { case: 'kebabCase' }],
      'security/detect-object-injection': 'off',
      'sonarjs/no-duplicate-string': 'warn',
      'no-secrets/no-secrets': ['warn', { tolerance: 4.5 }],
      'promise/prefer-await-to-then': 'error',
      'eslint-comments/no-unused-disable': 'error',
      'perfectionist/sort-imports': ['error', { type: 'natural', order: 'asc', groups: ['builtin','external','type','internal','parent','sibling','index','object'] }],
    },
  },
  // Place config override last so it wins over JS override rules
  {
    files: ['eslint.config.mjs'],
    rules: {
      'import/no-default-export': 'off',
      'import/order': 'off',
      'perfectionist/sort-imports': 'off',
      'sonarjs/no-duplicate-string': 'off',
    },
  },
  { ignores: ['**/node_modules/**', '**/dist/**'] },
];
