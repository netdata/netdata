# Rust Crates Area Instructions

The repo-root `AGENTS.md` applies in full; where the two conflict, this more specific file wins.

## Writing User-Facing Files

- When you write or edit user-facing files, write about 80% of the way to ASD-STE100. User-facing files are:
  - under this directory: `README.md`, `metadata.yaml`, comments in shipped config files;
  - the related documentation pages under `docs/`, such as `docs/opentelemetry/` and `docs/npm/network-flows/`.
- This does not apply to code, code comments, developer specs (for example `FORMAT.md`), or your chat replies.
- Do not edit generated `integrations/*.md` files; edit the crate's `metadata.yaml` instead.

## Writing Code Comments

These rules apply to every comment you write or change in the Rust crates under this directory. Their purpose: few
comments, in plain words, that stay true when other files change.

- **Local**: a comment MUST describe its own file: what the code does, the contract it offers, and what it requires
  from other code.
  - A comment MUST NOT describe what other code does: who calls an item, what another file or process does before or
    after, or the inside of a dependency. A search answers those questions, and the comment is wrong as soon as that
    other code changes.
  - Keep these, even when they mention other code: the reason for a choice made in this file, when to call an item and
    what its arguments mean, and the meaning of a metric, a field, or a wire value that the file defines.
  - A fact about how several files work together has one home: the crate root doc (`//!` at the top of `lib.rs` or
    `main.rs`). Other files MAY point at it by symbol. They MUST NOT repeat it.
  - A root doc MUST NOT repeat the module docs. Its module list gives each module one line.
- **True**: a comment MUST say what the code does, not what it should do. Check it against the code before you write
  it.
  - A number in a comment MUST be computed from the code, not estimated.
  - A claim about code outside these crates, such as the Agent's C code, MUST be checked against the implementation
    that the build compiles. The Agent has several implementations of some subsystems, one per platform.
- **Worth reading**: a comment MUST NOT restate the code or the name of the item.
  - A comment MUST NOT tell the history of a change ("now uses", "previously", "fixed", "as requested"). History
    belongs in the commit message.
  - A comment MUST NOT cite codes of plans, decisions, phases, or review rounds (`D6`, `P7`, "(#4)"). Nothing in the
    repository resolves them.
  - Write what a reader cannot get from the code: an invariant, the reason behind a choice, a contract, a pitfall.
- **Plain**: write for a reader who knows Rust and has only this file open.
  - One idea per sentence. Do not chain clauses with dashes, semicolons, or parentheses.
  - Do not use a term that is not an identifier, not standard in Rust or in the domain, and not defined in the crate
    root doc.
  - No capitals or bold for emphasis, and no banner lines. Use `MUST`, `MUST NOT`, and `MAY` only to state a contract.
- **Tests**: a comment in a test MUST claim only what the assertions check. Say what is not checked when a reader would
  assume that it is.
- **References**: a comment MUST NOT cite a line number. Name the symbol. A source path that a comment cites MUST
  exist.
- **Help text**: the doc comments of an item that derives clap's `Parser`, `Args`, `Subcommand`, or `ValueEnum`, and
  of its fields and variants, are the program's `--help` output. Write them for the user of the command, and treat a
  change to them as a change to the program's output.
- **When you change code**: in the same change, you MUST correct or delete every comment that the change makes false.
  You MUST NOT reword comments that the change does not affect.
