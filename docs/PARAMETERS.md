# Parameter Change Checklist

Whenever a new runtime parameter is introduced (or an existing one is
refactored), verify the following surfaces are updated in lockstep:

1. **Options registry** (`src/options-registry.ts`)
   - Add the option metadata
   - Ensure CLI aliases, descriptions, and frontmatter allowances are correct
2. **Resolution pipeline**
   - Extend `resolveEffectiveOptions` with precedence rules
   - Update `options-schema.ts` so validation covers the new key
   - Propagate through `LoadAgentOptions`, `GlobalOverrides`, and
     `AIAgentSessionConfig`
3. **CLI & overrides**
   - Map command-line flags in `cli.ts`
   - Add override handling in `buildGlobalOverrides`
   - Pass the option when loading agents or headends
4. **LLM providers**
   - Extend `TurnRequest` if the provider needs the parameter
   - Update provider adapters to transmit the correct payload
5. **Configuration & documentation**
   - Update `.ai-agent.json` schema (`src/config.ts`) when configuration knobs
     are required
   - Document behaviour in the relevant `docs/*.md` files and mention the flag
     in `--help`
6. **Tests & linting**
   - Add or update deterministic harness coverage when reasonable
   - Run `npm run lint` and `npm run build`

Use this list as a gate before concluding a parameter change is complete.
