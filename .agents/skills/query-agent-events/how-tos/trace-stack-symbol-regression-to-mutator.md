# Trace a stack-symbol crash regression to a concurrent mutator

Use this workflow when many agent-events rows mention the same stack symbol, but the symbol may be either the faulting function or only a lower frame, and the suspected regression must be tied to a source change.

## 1. Preserve the reported interval exactly

Cloud log links carry millisecond epoch bounds. Convert them to seconds for the Function payload and record the UTC interval in the local SOW:

```bash
date -u -d @"$((AFTER_MS / 1000))" '+%Y-%m-%d %H:%M:%S UTC'
date -u -d @"$((BEFORE_MS / 1000))" '+%Y-%m-%d %H:%M:%S UTC'
```

Do not put the Cloud link, node ID, or any event identifier in a durable artifact.

## 2. Query crash classes first, then apply the symbol as FTS

A symbol inside `AE_FATAL_STACK_TRACE` is not necessarily the value of `AE_FATAL_FUNCTION`. Use the structured crash selection first and the symbol only as a residual full-text narrower:

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
  --since "$AFTER_SECONDS" \
  --before "$BEFORE_SECONDS" \
  --health crash \
  --version all \
  --query "$SYMBOL" \
  --facets AE_FATAL_SIGNAL_CODE,AE_FATAL_FUNCTION,AE_AGENT_VERSION,AE_AGENT_HEALTH,AE_EXIT_CAUSE \
  --last 10000 \
  --output .local/audits/query-agent-events/symbol-window.json
```

Why:

- `AE_AGENT_HEALTH` uses the facet index to reduce the search to crash records.
- `query` then finds the symbol anywhere in the already narrowed record, including the stack trace.
- `--version all` is intentional for a reported exact interval; auto-version filtering could hide the first affected build.

## 3. Prove that the response is complete

Do not interpret row counts before checking the envelope:

```bash
jq '{status, partial, pagination, items, sampling:._sampling, rows:(.data|length)}' \
  .local/audits/query-agent-events/symbol-window.json
```

Required checks:

- `status == 200`
- `partial == false`
- `items.returned == items.matched`
- `items.returned < items.max_to_return`
- `_sampling.sampled == 0`

If any check fails, narrow the query further or paginate before drawing conclusions.

## 4. Separate direct-function matches from stack-only matches

Function responses store rows as arrays and provide the field indexes in `columns`. Always project by the column map; column order is not a stable contract.

Calculate separately:

- rows where `AE_FATAL_FUNCTION == $SYMBOL`;
- rows where `AE_FATAL_STACK_TRACE` contains `$SYMBOL` but `AE_FATAL_FUNCTION` differs;
- the symbol's frame number and the first symbolized application frame;
- normalized application stack families after replacing raw addresses.

The distinction matters:

- A direct `AE_FATAL_FUNCTION` match supports the symbol as the captured fault location.
- A stack-only match proves only that the symbol was on the call chain.
- If every stack places the symbol at the first application frame, stack-only metadata differences may be status-file timing artifacts, but this remains an inference until the call-site mapping is checked.

Raw stack addresses remain in the gitignored audit dump. Durable notes use only normalized frames such as `0xADDR`.

## 5. Measure prevalence against the correct denominator

The matching slice cannot provide a crash rate. Fetch all crash-class events for the affected versions over the same interval, using explicit structured version values and no FTS:

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
  --since "$AFTER_SECONDS" \
  --before "$BEFORE_SECONDS" \
  --health crash \
  --versions "$AFFECTED_VERSIONS_CSV" \
  --facets AE_FATAL_SIGNAL_CODE,AE_FATAL_FUNCTION,AE_AGENT_VERSION \
  --last 10000 \
  --output .local/audits/query-agent-events/affected-version-crashes.json
```

Report at least:

- matching symbol rows / all crash-class rows;
- matching symbol rows / all signal-crash rows;
- distinct affected agents;
- per-version matching rows and distinct agents.

Do not call per-version row shares "rates" without an active-install population denominator. Nightly rollout time strongly biases the version mix.

## 6. Find the first affected build

For a recent regression, widen to 7 days but keep both structured crash and explicit recent-version selections. Include several versions before and after the apparent boundary.

Interpretation rules:

- A single historical row with a different signal or stack family is not the same regression.
- The first build with a sharp, repeated cluster is the candidate boundary.
- Account for the after-the-fact posting model and 23-hour client dedup described in `../update-cadence.md`.

Map version strings to repository commits through `packaging/version`:

```bash
git log --format='%H %ad %s' --date=iso-strict -- packaging/version
git log -S"$VERSION" --format='%H %ad %s' --date=iso-strict -- packaging/version
```

Then list commits between the last unaffected and first affected nightly.

## 7. Prove which same-symbol call failed

One static function can have multiple call sites with different inputs. Read every reference, then inspect the built binary's DWARF line mapping:

```bash
rg -n "\\b${SYMBOL}\\s*\\(" src
nm -an /path/to/netdata | rg "$SYMBOL|CALLER_FUNCTION"
objdump -dSl --disassemble=CALLER_FUNCTION /path/to/netdata
```

Match the caller line reported by the event stack to the disassembly. This can distinguish, for example, an environment-vector encoding call from a command-argument encoding call even though both enter the same function.

Use a binary built from the same source blob as the affected nightly. Verify equivalence with `git rev-parse REV:path/to/source.c` before relying on the mapping.

## 8. Use fault-address shape as supporting memory-lifetime evidence

Group fault addresses without publishing them:

- null / low-page;
- normal-width mapped-looking userspace address;
- shortened heap-derived value;
- other.

On glibc releases using safe-linking, a freed tcache chunk's first machine word can resemble its heap address shifted right by 12 bits. The [glibc safe-linking patch](https://sourceware.org/pipermail/libc-alpha/2020-March/112022.html) defines the pointer mangling used by tcache. If the crash dereferences that shortened value from the first slot of a stale vector, this is strong use-after-free evidence.

Do not diagnose a tcache use-after-free from address shape alone. Require all of:

- the exact faulting read from source/line mapping;
- a vector or object that a concurrent mutator can reallocate and free;
- a version boundary containing that mutator;
- consistent event stacks and address classes.

## 9. Search the boundary for concurrent mutators

Search all C and C++ sources, not only `*.c` and `*.h`:

```bash
rg -n --glob '*.{c,h,cc,cpp,cxx}' \
  '\b(setenv|unsetenv|putenv|clearenv|nd_setenv)\s*\(' src
git log --format='%H %ad %s' --date=iso-strict LAST_UNAFFECTED..FIRST_AFFECTED
```

For each candidate:

1. Prove it is absent from the last unaffected build and present in the first affected build:

   ```bash
   git merge-base --is-ancestor CANDIDATE LAST_UNAFFECTED
   git merge-base --is-ancestor CANDIDATE FIRST_AFFECTED
   ```

2. Prove its execution overlaps the crashing readers by tracing thread creation and startup order.
3. Check whether the mutation is common to the affected population, rather than enabled only by an uncommon optional feature.
4. Verify the platform contract. For example, the [GNU libc environment documentation](https://sourceware.org/glibc/manual/2.29/html_node/Environment-Access.html) marks environment mutation as `MT-Unsafe const:env`; a reader walking `environ` concurrently is not protected by libc's writer lock. Check version-specific changes too: [glibc 2.41 retained old environment arrays](https://sourceware.org/pipermail/glibc-bugs/2025-July/059702.html), reducing one failure mode without making direct concurrent `environ` use a portable contract.

## 10. State confidence precisely

Use this evidence ladder:

- **Symptom cluster:** same symbol appears in many records.
- **Fault localization:** same symbol is the first application frame and the call site is identified.
- **Memory-failure class:** fault-address shape and source read identify stale/freed input.
- **Regression boundary:** the cluster starts in the first build containing a specific mutator.
- **Root cause established:** the mutator's execution overlaps the reader and explains the exact invalid value.
- **Reproduced:** a focused concurrency test or core confirms the stale object directly.

Do not skip from symptom cluster to root cause. If the final two levels are missing, label the result a working theory and state the evidence still required.

## How I figured this out

Files and guides used:

- `../AE_FIELDS.md`
- `../query-discipline.md`
- `../finding-crashes.md`
- `../update-cadence.md`
- `../recipes/find-by-function.md`
- `scripts/get-events.sh`
- `scripts/analyze-events.sh`
- the implicated function, every call site, startup thread tables, and version-changing commits

Queries used:

- exact-window crash selection plus residual stack-symbol FTS;
- exact-window all-crashes query for explicit affected versions;
- seven-day crash selection over explicit recent versions for the regression boundary;
- local `jq` projection through the response `columns` map.

Validation used:

- response completeness and sampling checks;
- normalized stack-family clustering;
- distinct-agent and per-version counts;
- built-binary DWARF/disassembly call-site mapping;
- commit ancestry checks around the first affected build;
- primary libc documentation for environment-mutation thread safety.
