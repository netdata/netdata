# Fix JavaScript Formatting Findings

Codacy's `ESLint8_prettier-vue_prettier` finding (`prettier-vue/prettier`) reports Prettier formatting differences.
The rule name does not mean the affected code uses Vue.

For DEM runtime files, follow the pinned formatter commands in
[the runtime instructions](../../../../src/dem-runtime/AGENTS.md#javascript-formatting).
Format only the task's changed files, inspect the diff and check those same files before committing.
For other JavaScript trees, consult their own formatter configuration and instructions.

Prettier addresses layout; evaluate semantic lint findings separately. A passing formatter check does not establish
that the current-head Codacy gate passes.
