[[ page.title ]]

[% for paragraph in page.intro %]
[[ paragraph ]]

[% endfor %]
### Jump To

[[ page.jump_to_line ]]


## Resolver Quick Reference

| Resolver | Syntax | Best for | Notes |
|:---------|:-------|:---------|:------|
[% for item in page.quick_reference %]
| [[ item.resolver ]] | [[ item.syntax ]] | [[ item.best_for ]] | [[ item.notes ]] |
[% endfor %]

## Choosing a Resolver

[% for item in page.choosing_a_resolver %]
- [[ item ]]
[% endfor %]

[% for section in page.sections %]
[[ section.heading ]]

[[ section.body ]]

[[ section.example ]]

[% for note in section.notes %]
- [[ note ]]
[% endfor %]

[% endfor %]
[[ page.store.heading ]]

[[ page.store.body ]]

[[ page.store.reference_intro ]]

```text
[[ page.store.reference_syntax ]]
```

| Part | Description |
|:-----|:------------|
[% for part in page.store.reference_parts %]
| [[ part.name ]] | [[ part.description ]] |
[% endfor %]

Example:

[[ page.store.example ]]

### Configuration Methods

#### Dynamic Configuration UI

[% for step in page.store.ui_steps %]
[[ loop.index ]]. [[ step ]]
[% endfor %]

#### Configuration Files

[[ page.store.file_intro ]]

| File | Backend |
|:-----|:--------|
[% for backend in secretstores %]
| `[[ backend.config_file ]]` | [[ backend.name ]] |
[% endfor %]

Each file contains a `jobs` array. The backend kind is determined by the filename.

:::note

[[ page.store.file_note ]]

:::

[[ page.store.file_directory ]]

### Multiple Secretstores

[[ page.store.multiple_stores ]]

### Mixing Resolver Types

[[ page.store.mixing ]]

[[ page.secretstores.heading ]]

[[ page.secretstores.intro ]]

| Backend | Kind | Operand Format | Example Operand |
|:--------|:-----|:---------------|:----------------|
[% for backend in secretstores %]
| [[ backend.name_link ]] | `[[ backend.kind ]]` | `[[ backend.operand_format ]]` | `[[ backend.example_operand ]]` |
[% endfor %]

## How It Works

[% for item in page.how_it_works %]
- [[ item ]]
[% endfor %]

## Security Notes

[% for item in page.security_notes %]
- [[ item ]]
[% endfor %]

## Troubleshooting

[% for item in page.troubleshooting.intro %]
- [[ item ]]
[% endfor %]

Representative error patterns:

[% for err in page.troubleshooting.errors %]
- [[ err.syntax ]]: [[ err.message ]]
[% endfor %]
