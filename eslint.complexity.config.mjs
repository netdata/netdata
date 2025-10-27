// ESLint configuration for complexity analysis
// Used by lint.sh to check for overly complex functions

import tsParser from '@typescript-eslint/parser';

export default [
  {
    files: ['src/**/*.ts'],
    ignores: ['src/tests/**/*.ts', 'src/**/*.d.ts'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        project: './tsconfig.json',
      },
    },
    rules: {
      // Cyclomatic complexity - measures decision points
      'complexity': ['error', { max: 10 }],

      // Function length - measures maintainability
      'max-lines-per-function': ['error', {
        max: 100,
        skipBlankLines: true,
        skipComments: true,
        IIFEs: true
      }],

      // Nesting depth - measures readability
      'max-depth': ['error', { max: 4 }],

      // Callback nesting - measures async complexity
      'max-nested-callbacks': ['error', { max: 3 }],

      // Disable other rules to focus only on complexity
      '@typescript-eslint/no-unused-vars': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      'no-undef': 'off'
    }
  }
];
