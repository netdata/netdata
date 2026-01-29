#!/usr/bin/env python3
"""
Three-call skill testing framework.

Tests that an AI model can:
1. Describe an artifact in natural language (no config statements)
2. Recreate the artifact from the description
3. Self-assess the result and provide skill improvement recommendations

This approach surfaces skill gaps through the model's own reasoning.
"""

import argparse
import json
import sys
from dataclasses import dataclass, asdict, field
from datetime import datetime
from pathlib import Path
from typing import List, Optional

try:
    from .models import load_config, get_model_config, call_model, ModelConfig, ModelResponse
except ImportError:
    from models import load_config, get_model_config, call_model, ModelConfig, ModelResponse


@dataclass
class Assessment:
    """LLM self-assessment of the round-trip test."""
    is_equivalent: bool = False
    step_1_errors: List[str] = field(default_factory=list)
    step_2_errors: List[str] = field(default_factory=list)
    step_1_reasoning_issues: List[str] = field(default_factory=list)
    step_2_reasoning_issues: List[str] = field(default_factory=list)
    skill_improvements: List[str] = field(default_factory=list)
    raw_json: str = ""
    parse_error: Optional[str] = None


@dataclass
class TestResult:
    """Complete result of a three-call test."""
    test_name: str
    model_name: str
    model_size: str

    # Step 1: Describe
    prompt_step1: str = ""
    description: str = ""
    reasoning_step1: Optional[str] = None

    # Step 2: Recreate
    prompt_step2: str = ""
    recreated: str = ""
    reasoning_step2: Optional[str] = None

    # Step 3: Assessment
    prompt_step3: str = ""
    assessment: Optional[Assessment] = None
    reasoning_step3: Optional[str] = None

    # Original artifact
    original: str = ""

    # Status
    passed: bool = False
    error: Optional[str] = None


@dataclass
class TestSuiteResult:
    """Result of testing a skill with multiple test cases."""
    skill_name: str
    timestamp: str
    model_name: str
    model_size: str
    skill_content: str  # The system prompt (skill file contents)
    results: List[TestResult]
    all_skill_improvements: List[str]
    summary: dict


def load_skill(skill_path: Path) -> str:
    """Load skill content from file."""
    with open(skill_path) as f:
        return f.read()


def load_test_case(case_path: Path) -> str:
    """Load a single test case (alert definition)."""
    with open(case_path) as f:
        return f.read().strip()


def extract_json_from_response(text: str) -> Optional[dict]:
    """Extract JSON object from model response, handling markdown code blocks."""
    import re

    # Try to find JSON in code block first
    code_block = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', text, re.DOTALL)
    if code_block:
        try:
            return json.loads(code_block.group(1))
        except json.JSONDecodeError:
            pass

    # Try to find raw JSON object
    json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', text, re.DOTALL)
    if json_match:
        try:
            return json.loads(json_match.group())
        except json.JSONDecodeError:
            pass

    # Last resort: try the whole text
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return None


def run_step1_describe(
    model: ModelConfig,
    skill_content: str,
    original_alert: str,
) -> tuple[ModelResponse, str]:
    """
    Step 1: Ask model to describe the alert in natural language.

    The description must NOT contain any configuration statements - only
    natural language that describes what the alert does.

    Returns: (response, user_prompt)
    """
    user_prompt = f"""Describe in natural language the following Netdata alert definition.

CRITICAL: Do NOT use any configuration keywords in your description. Describe WHAT the alert does, not HOW it's configured.

RULES:
1. State the alert name in backticks (e.g., `disk_space_usage`)
2. Describe the SCOPE: Does it apply to ONE specific instance, or ALL instances of something? (Do NOT say "template" or "alarm" - describe what it monitors)
3. CRITICAL - Copy the summary and info strings VERBATIM in quotes. Example format:
   - Summary: "System CPU utilization"
   - Info: "Average CPU utilization over the last 10 minutes (excluding iowait, nice and steal)"
   If summary or info is missing from the original, say "No summary provided" or "No info provided"
4. For label filters, describe in plain English what is included and excluded
5. Describe the data calculation in ONE sentence: what data is queried, over what time window, and how it's transformed
6. If the calculation references another alert's value, state the EXACT variable name (e.g., "uses the value from `$disk_fill_rate`")
7. Explain warning and critical conditions in plain English, including the hysteresis behavior
8. Include evaluation frequency, delay behavior, and who receives notifications
9. Mention the units of the result
10. Mention classification (class, type, component) ONLY if present in the original - if not present, say nothing about classification

FORBIDDEN WORDS: template, alarm, lookup, calc, on:, warn:, crit:, every:, delay:, to:

Write flowing natural language that a user would say when requesting this alert.

ALERT DEFINITION:
```
{original_alert}
```

Write your natural language description:"""

    return call_model(model, system_prompt=skill_content, user_prompt=user_prompt), user_prompt


def run_step2_recreate(
    model: ModelConfig,
    skill_content: str,
    description: str,
) -> tuple[ModelResponse, str]:
    """
    Step 2: Ask model to recreate the alert from the description.

    Returns: (response, user_prompt)
    """
    user_prompt = f"""Create a Netdata alert configuration based on this description.

RULES:
1. Include ONLY what is described - do NOT add fields that weren't mentioned
2. Use EXACT quoted strings for summary and info (they appear in quotes in the description)
3. If the description doesn't mention classification fields, don't add them

DESCRIPTION:
{description}

Output ONLY the raw alert configuration. No explanations, no markdown."""

    return call_model(model, system_prompt=skill_content, user_prompt=user_prompt), user_prompt


def run_step3_assess(
    model: ModelConfig,
    skill_content: str,
    original_alert: str,
    step1_reasoning: Optional[str],
    step1_output: str,
    step2_reasoning: Optional[str],
    step2_output: str,
) -> tuple[ModelResponse, Assessment, str]:
    """
    Step 3: Ask model to self-assess and identify skill gaps.

    Returns: (response, assessment, user_prompt)
    """
    # Format reasoning sections (handle None)
    r1 = step1_reasoning if step1_reasoning else "(no reasoning captured)"
    r2 = step2_reasoning if step2_reasoning else "(no reasoning captured)"

    user_prompt = f"""We need you to identify issues and score your system prompt. To do so, we did the following:

1. We first called you to create a natural language description of this alert:

<original_alert>
{original_alert}
</original_alert>

Your reasoning was:

<reasoning_of_step_1>
{r1}
</reasoning_of_step_1>

And your output was:

<output_of_step_1>
{step1_output}
</output_of_step_1>

2. Then we provided back the `output_of_step_1` and we asked you to recreate the original alert.

Your reasoning was:

<reasoning_of_step_2>
{r2}
</reasoning_of_step_2>

And your output was:

<output_of_step_2>
{step2_output}
</output_of_step_2>

3. Your system prompt is complete and correct ONLY if `output_of_step_2` is 100% equivalent to the `original_alert`.

You now need to provide your assessment in the following JSON schema (output ONLY valid JSON, no other text):

{{
  "is_equivalent": true or false,
  "step_1_errors": ["list of things step 1 description got wrong or missed"],
  "step_2_errors": ["list of things step 2 recreation got wrong"],
  "step_1_reasoning_issues": ["stress points or confusions in step 1 reasoning"],
  "step_2_reasoning_issues": ["stress points or confusions in step 2 reasoning"],
  "skill_improvements": ["concrete recommendations to improve your system prompt"]
}}"""

    response = call_model(model, system_prompt=skill_content, user_prompt=user_prompt)

    assessment = Assessment()
    if response.success:
        assessment.raw_json = response.content
        parsed = extract_json_from_response(response.content)
        if parsed:
            assessment.is_equivalent = parsed.get("is_equivalent", False)
            assessment.step_1_errors = parsed.get("step_1_errors", [])
            assessment.step_2_errors = parsed.get("step_2_errors", [])
            assessment.step_1_reasoning_issues = parsed.get("step_1_reasoning_issues", [])
            assessment.step_2_reasoning_issues = parsed.get("step_2_reasoning_issues", [])
            assessment.skill_improvements = parsed.get("skill_improvements", [])
        else:
            assessment.parse_error = "Failed to parse JSON from response"

    return response, assessment, user_prompt


def run_single_test(
    test_name: str,
    original_alert: str,
    model: ModelConfig,
    model_size: str,
    skill_content: str,
) -> TestResult:
    """Run a complete three-call test on a single alert."""

    result = TestResult(
        test_name=test_name,
        model_name=model.name,
        model_size=model_size,
        original=original_alert,
    )

    # Step 1: Describe
    print(f"      Step 1 (describe)...", end=" ", flush=True)
    resp1, prompt1 = run_step1_describe(model, skill_content, original_alert)
    result.prompt_step1 = prompt1
    if not resp1.success:
        result.error = f"Step 1 failed: {resp1.error}"
        print("FAILED")
        return result
    if resp1.truncated:
        result.error = f"Step 1 truncated (finish_reason=length, used {resp1.completion_tokens} tokens). Increase max_tokens."
        print("TRUNCATED")
        return result
    result.description = resp1.content
    result.reasoning_step1 = resp1.reasoning
    print("OK")

    # Step 2: Recreate
    print(f"      Step 2 (recreate)...", end=" ", flush=True)
    resp2, prompt2 = run_step2_recreate(model, skill_content, result.description)
    result.prompt_step2 = prompt2
    if not resp2.success:
        result.error = f"Step 2 failed: {resp2.error}"
        print("FAILED")
        return result
    if resp2.truncated:
        result.error = f"Step 2 truncated (finish_reason=length, used {resp2.completion_tokens} tokens). Increase max_tokens."
        print("TRUNCATED")
        return result
    result.recreated = resp2.content
    result.reasoning_step2 = resp2.reasoning
    print("OK")

    # Step 3: Assess
    print(f"      Step 3 (assess)...", end=" ", flush=True)
    resp3, assessment, prompt3 = run_step3_assess(
        model, skill_content,
        original_alert,
        result.reasoning_step1, result.description,
        result.reasoning_step2, result.recreated,
    )
    result.prompt_step3 = prompt3
    if not resp3.success:
        result.error = f"Step 3 failed: {resp3.error}"
        print("FAILED")
        return result
    if resp3.truncated:
        result.error = f"Step 3 truncated (finish_reason=length, used {resp3.completion_tokens} tokens). Increase max_tokens."
        print("TRUNCATED")
        return result
    result.assessment = assessment
    result.reasoning_step3 = resp3.reasoning

    if assessment.parse_error:
        print(f"PARSE ERROR")
        result.error = assessment.parse_error
    else:
        result.passed = assessment.is_equivalent
        print("PASS" if result.passed else "FAIL")

    return result


def run_test_suite(
    test_cases: List[tuple[str, str]],  # List of (name, content)
    skill_path: Path,
    model_size: str = "large",
    config: Optional[dict] = None,
) -> TestSuiteResult:
    """
    Run the full test suite for a skill.

    Args:
        test_cases: List of (test_name, alert_content) tuples
        skill_path: Path to the skill file
        model_size: "large" or "small"
        config: Optional config dict
    """
    if config is None:
        config = load_config()

    skill_content = load_skill(skill_path)
    model = get_model_config(model_size, config)

    results = []
    all_improvements = []

    for test_name, original_alert in test_cases:
        print(f"\n  Testing: {test_name}")
        print(f"    Model: {model.name}")

        result = run_single_test(
            test_name, original_alert, model, model_size, skill_content
        )
        results.append(result)

        # Collect improvements
        if result.assessment and result.assessment.skill_improvements:
            for imp in result.assessment.skill_improvements:
                if imp not in all_improvements:
                    all_improvements.append(imp)

    # Summary
    passed = sum(1 for r in results if r.passed)
    failed = len(results) - passed

    summary = {
        "total": len(results),
        "passed": passed,
        "failed": failed,
        "pass_rate": f"{100 * passed / len(results):.1f}%" if results else "N/A",
    }

    return TestSuiteResult(
        skill_name=skill_path.stem,
        timestamp=datetime.now().isoformat(),
        model_name=model.name,
        model_size=model_size,
        skill_content=skill_content,
        results=results,
        all_skill_improvements=all_improvements,
        summary=summary,
    )


def save_results(result: TestSuiteResult, output_dir: Path) -> Path:
    """Save test results to JSON file."""
    output_dir.mkdir(parents=True, exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    filename = f"{result.skill_name}_{result.model_size}_{timestamp}.json"
    output_file = output_dir / filename

    # Convert to serializable dict
    def serialize_assessment(a: Optional[Assessment]) -> Optional[dict]:
        if a is None:
            return None
        return asdict(a)

    output = {
        "skill_name": result.skill_name,
        "timestamp": result.timestamp,
        "model_name": result.model_name,
        "model_size": result.model_size,
        "summary": result.summary,
        "all_skill_improvements": result.all_skill_improvements,
        "system_prompt": result.skill_content,
        "results": [
            {
                "test_name": r.test_name,
                "model_name": r.model_name,
                "model_size": r.model_size,
                "passed": r.passed,
                "error": r.error,
                "original": r.original,
                "prompt_step1": r.prompt_step1,
                "description": r.description,
                "reasoning_step1": r.reasoning_step1,
                "prompt_step2": r.prompt_step2,
                "recreated": r.recreated,
                "reasoning_step2": r.reasoning_step2,
                "prompt_step3": r.prompt_step3,
                "reasoning_step3": r.reasoning_step3,
                "assessment": serialize_assessment(r.assessment),
            }
            for r in result.results
        ],
    }

    with open(output_file, "w") as f:
        json.dump(output, f, indent=2)

    return output_file


def print_summary(result: TestSuiteResult):
    """Print test summary to console."""
    GREEN = "\033[92m"
    YELLOW = "\033[93m"
    RED = "\033[91m"
    CYAN = "\033[96m"
    RESET = "\033[0m"

    print("\n" + "=" * 70)
    print(f"SKILL: {result.skill_name}")
    print(f"MODEL: {result.model_name} ({result.model_size})")
    print("=" * 70)

    s = result.summary
    color = GREEN if s["failed"] == 0 else (YELLOW if s["passed"] > 0 else RED)
    print(f"\n{color}Results: {s['passed']}/{s['total']} passed ({s['pass_rate']}){RESET}")

    # Show failures
    failures = [r for r in result.results if not r.passed]
    if failures:
        print(f"\n{RED}Failures:{RESET}")
        for r in failures:
            print(f"\n  {r.test_name}:")
            if r.error:
                print(f"    Error: {r.error}")
            elif r.assessment:
                if r.assessment.step_1_errors:
                    print("    Step 1 errors:")
                    for e in r.assessment.step_1_errors[:3]:
                        print(f"      - {e}")
                if r.assessment.step_2_errors:
                    print("    Step 2 errors:")
                    for e in r.assessment.step_2_errors[:3]:
                        print(f"      - {e}")

    # Show skill improvements
    if result.all_skill_improvements:
        print(f"\n{CYAN}Skill Improvements Recommended:{RESET}")
        for i, imp in enumerate(result.all_skill_improvements, 1):
            print(f"  {i}. {imp}")

    print()


def load_test_cases_from_dir(cases_dir: Path) -> List[tuple[str, str]]:
    """Load all test cases from a directory."""
    cases = []
    for case_file in sorted(cases_dir.glob("*.conf")):
        name = case_file.stem
        content = load_test_case(case_file)
        cases.append((name, content))
    return cases


def main():
    parser = argparse.ArgumentParser(
        description="Run three-call skill tests"
    )
    parser.add_argument(
        "skill",
        type=Path,
        help="Path to skill file",
    )
    parser.add_argument(
        "test_cases",
        type=Path,
        help="Path to test cases directory",
    )
    parser.add_argument(
        "--model",
        choices=["large", "small", "both"],
        default="large",
        help="Which model to test with (default: large)",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(__file__).parent.parent.parent / "results",
        help="Output directory for results",
    )
    parser.add_argument(
        "--config",
        type=Path,
        help="Path to config file",
    )

    args = parser.parse_args()

    if not args.skill.exists():
        print(f"Error: Skill file not found: {args.skill}")
        sys.exit(1)

    if not args.test_cases.exists():
        print(f"Error: Test cases directory not found: {args.test_cases}")
        sys.exit(1)

    # Load test cases
    test_cases = load_test_cases_from_dir(args.test_cases)
    if not test_cases:
        print(f"No .conf files found in {args.test_cases}")
        sys.exit(0)

    print(f"Found {len(test_cases)} test cases")

    # Load config
    config = None
    if args.config:
        import yaml
        with open(args.config) as f:
            config = yaml.safe_load(f)

    # Run tests
    models_to_test = ["large", "small"] if args.model == "both" else [args.model]

    all_results = []
    for model_size in models_to_test:
        print(f"\n{'='*70}")
        print(f"Testing with {model_size} model...")
        print("=" * 70)

        result = run_test_suite(test_cases, args.skill, model_size, config)
        all_results.append(result)

        print_summary(result)

        output_file = save_results(result, args.output)
        print(f"Results saved to: {output_file}")

    # Exit code
    any_failed = any(r.summary["failed"] > 0 for r in all_results)
    sys.exit(1 if any_failed else 0)


if __name__ == "__main__":
    main()
