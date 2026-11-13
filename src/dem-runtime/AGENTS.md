# DEM Runtime Instructions

## JavaScript Formatting

- Before committing changes to `.js`, `.cjs` or `.mjs` files in this tree, you MUST format them with Prettier 3.6.2
  and run its `--check` on the same files.
- You MUST limit formatting to files changed by the task; do not reformat unrelated files.
- Use Prettier's default options. The formatter is a development tool and does not need a runtime dependency.

Run from the repository root, replacing the example path with the changed JavaScript files:

```bash
npx --yes prettier@3.6.2 --write src/dem-runtime/runner/browser-summary.cjs
npx --yes prettier@3.6.2 --check src/dem-runtime/runner/browser-summary.cjs
```
