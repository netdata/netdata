# Comprehensive Linting Guide

## Running All Checks

```bash
./lint.sh
```

This script runs 7 different types of checks:

1. **TypeScript Build Check** - Ensures code compiles
2. **ESLint** - Standard linting rules
3. **Dead Code Detection** - Finds unused files and exports
4. **Unused Exports** - Identifies exports that are never imported
5. **Complexity Analysis** - Detects overly complex functions
6. **Duplicate Code** - Finds copy-pasted code
7. **Unused Dependencies** - Identifies unnecessary packages

## Installing Tools

### Required (already installed)
- ESLint
- TypeScript

### Optional (install as needed)

Install all optional tools at once:
```bash
npm install --save-dev knip ts-prune jscpd depcheck
```

Or individually:

```bash
# Dead code detection
npm install --save-dev knip

# Unused exports
npm install --save-dev ts-prune

# Duplicate code detection
npm install --save-dev jscpd

# Unused dependencies
npm install --save-dev depcheck
```

## Complexity Thresholds

Uses ESLint (config: `eslint.complexity.config.mjs`) to check:
- **Cyclomatic complexity**: max 10 per function
- **Function length**: max 100 lines (excluding comments/blanks)
- **Nesting depth**: max 4 levels
- **Callback nesting**: max 3 levels

## Tool Details

### knip
Finds unused files, exports, dependencies, and types. Most comprehensive.
```bash
npx knip
```

### ts-prune
Laser-focused on unused exports in TypeScript.
```bash
npx ts-prune
```

### jscpd
Detects copy-pasted code blocks (minimum 10 lines, 50 tokens).
```bash
npx jscpd src
```

### depcheck
Finds unused dependencies in package.json.
```bash
npx depcheck
```

## Exit Codes

- **0**: All critical checks passed (build + eslint)
- **1**: Critical checks failed (build or eslint)

Warnings from other tools won't fail the script but will be displayed.
