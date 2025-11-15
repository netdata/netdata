import tseslint from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';
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

const IGNORE_GLOBS = ['**/node_modules/**', '**/dist/**', '**/mcp/**', '**/tmp/**', '**/.venv/**', 'src/config-resolver.ts', 'src/agent-loader.ts', 'eslint.complexity.config.mjs'];

export default [
  // Global ignores - must be first
  { ignores: IGNORE_GLOBS },
  // Overrides for scripts
  {
    files: ['**/scripts/**'],
    rules: { 'no-console': 'off' },
  },
  {
    files: ['**/*.js', '**/*.cjs', '**/*.mjs'],
    ignores: IGNORE_GLOBS,
    languageOptions: { ecmaVersion: 2023, sourceType: 'module' },
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
      'no-restricted-syntax': [
        'error',
        { selector: "ImportAttribute[key.name='defer']", message: 'import defer is disabled until runtime support is complete.' },
        { selector: "ImportAttribute[key.value='defer']", message: 'import defer is disabled until runtime support is complete.' }
      ],
    },
  },
  {
    files: ['src/**/*.ts', 'src/**/*.tsx'],
    ignores: IGNORE_GLOBS,
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: tsProject,
        tsconfigRootDir: process.cwd(),
        sourceType: 'module',
        ecmaVersion: 2023,
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
      // Workaround for https://github.com/typescript-eslint/typescript-eslint/issues/9105 (crash under ESLint 9.39 / TS 5.9)
      '@typescript-eslint/unified-signatures': 'off',
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
      'no-restricted-syntax': [
        'error',
        { selector: "ImportAttribute[key.name='defer']", message: 'import defer is disabled until runtime support is complete.' },
        { selector: "ImportAttribute[key.value='defer']", message: 'import defer is disabled until runtime support is complete.' }
      ],
    },
  },
  {
    files: ['code-review/**/*.ts'],
    ignores: IGNORE_GLOBS,
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        tsconfigRootDir: process.cwd(),
        sourceType: 'module',
        ecmaVersion: 2023,
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
      'eslint-comments': eslintComments,
      n,
      perfectionist,
      'no-secrets': noSecrets,
      regexp,
    },
    rules: {
      ...tseslint.configs.strict?.rules,
      ...tseslint.configs.stylistic?.rules,
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-floating-promises': 'off',
      '@typescript-eslint/require-array-sort-compare': 'off',
      '@typescript-eslint/strict-boolean-expressions': 'off',
      'functional/no-loop-statements': 'off',
      'import/no-default-export': 'off',
      'perfectionist/sort-imports': ['error', { type: 'natural', order: 'asc', groups: ['builtin','external','type','internal','parent','sibling','index','object'] }],
      'promise/prefer-await-to-then': 'off',
      'eslint-comments/no-unused-disable': 'error',
    },
  },
  // Temporary overrides for server headend until strict typing refinements are completed
  {
    files: ['src/server/**/*.ts'],
    rules: {
      'import/order': 'off',
      'perfectionist/sort-imports': 'off',
      '@typescript-eslint/consistent-type-imports': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/strict-boolean-expressions': 'off',
      '@typescript-eslint/await-thenable': 'off',
      '@typescript-eslint/no-redundant-type-constituents': 'off',
      '@typescript-eslint/no-unnecessary-condition': 'off',
      '@typescript-eslint/no-misused-promises': 'off',
      '@typescript-eslint/require-await': 'off',
      '@typescript-eslint/no-unnecessary-type-assertion': 'off',
      '@typescript-eslint/non-nullable-type-assertion-style': 'off',
      '@typescript-eslint/restrict-template-expressions': 'off',
      'functional/no-loop-statements': 'off',
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
];
