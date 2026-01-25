# Netdata AI Skills

Dense, AI-consumable instructions for operating and developing Netdata.

## Purpose

These skills are designed to be consumed by AI assistants (Claude, GPT, Gemini, etc.) to help users:
- Configure Netdata features
- Troubleshoot issues
- Perform operational tasks
- Develop custom plugins

## Directory Structure

```
src/ai-skills/
├── skills/                    # AI skill files
│   ├── netdata-alerts-config.md
│   ├── netdata-alerts-ops.md
│   └── ...
├── tests/                     # Testing framework
│   ├── framework/             # Test infrastructure
│   │   ├── models.py          # Model API wrappers
│   │   ├── normalize.py       # Output normalization
│   │   └── round_trip.py      # Round-trip test runner
│   ├── alerts-config/         # Tests for alerts-config skill
│   │   └── cases/             # Test case files
│   └── config.yaml            # Model endpoints configuration
├── results/                   # Test output (gitignored)
└── README.md
```

## Skill Naming Convention

Pattern: `netdata-{topic}-{action}.md`

| Action | Meaning | Audience |
|--------|---------|----------|
| `-config` | Configure/customize | Users/Admins |
| `-use` | Use/consume/interact | Users/Admins |
| `-ops` | Operate/maintain/troubleshoot | Users/Admins |
| `-dev` | Develop/extend/create | Developers |

## How to Use These Skills

### With Claude Code

Skills can be used as Claude Code slash commands. See `.claude/skills/` for wrappers.

### With Other AI Assistants

Copy the skill content into your prompt or system message:

```
[Paste content of netdata-alerts-config.md]

User: How do I configure a disk space alert?
```

### Direct Embedding

Include in your AI application's context:

```python
with open("skills/netdata-alerts-config.md") as f:
    skill = f.read()

response = client.chat(
    messages=[
        {"role": "system", "content": skill},
        {"role": "user", "content": user_question}
    ]
)
```

## Testing Framework

Skills are validated using round-trip tests:

1. Give an artifact (e.g., alert config) to the AI
2. AI describes it in natural language
3. AI recreates the artifact from the description
4. Compare: original must equal regenerated

### Test Results

| Large Model | Small Model | Result |
|-------------|-------------|--------|
| Pass | Pass | **PASS** - Skill is good |
| Pass | Fail | **WARNING** - Works but could be clearer |
| Fail | * | **FAIL** - Skill needs fixing |

### Running Tests

```bash
cd tests
python framework/round_trip.py ../skills/netdata-alerts-config.md alerts-config/cases/
```

## Contributing

1. Create skill file following naming convention
2. Add test cases
3. Run tests against both models
4. Iterate until both models pass
