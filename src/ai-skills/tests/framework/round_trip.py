#!/usr/bin/env python3
"""
Round-trip test framework for AI Skills.

Tests that an AI model can:
1. Describe an artifact (alert, config, etc.) in natural language
2. Recreate the exact artifact from that description

If original == regenerated, the skill contains sufficient information.
"""

import argparse
import json
import sys
from dataclasses import dataclass, asdict
from datetime import datetime
from pathlib import Path
from typing import List, Optional, Callable

try:
    from .models import load_config, get_model_config, call_model, ModelConfig
    from .normalize import alerts_equal
except ImportError:
    from models import load_config, get_model_config, call_model, ModelConfig
    from normalize import alerts_equal


@dataclass
class TestCase:
    """A single test case."""
    name: str
    original: str
    skill_path: Path


@dataclass
class TestResult:
    """Result of a single test."""
    test_name: str
    model_name: str
    model_size: str  # "large" or "small"
    passed: bool
    original: str
    description: str
    regenerated: str
    differences: List[str]
    reasoning_describe: Optional[str] = None
    reasoning_regenerate: Optional[str] = None
    error: Optional[str] = None


@dataclass
class TestSuiteResult:
    """Result of a complete test suite run."""
    skill_name: str
    timestamp: str
    results: List[TestResult]
    summary: dict


def load_skill(skill_path: Path) -> str:
    """Load a skill file content."""
    with open(skill_path) as f:
        return f.read()


def run_single_test(
    test_case: TestCase,
    model: ModelConfig,
    model_size: str,
    skill_content: str,
    compare_fn: Callable[[str, str], tuple[bool, List[str]]],
) -> TestResult:
    """
    Run a single round-trip test.

    1. Ask model to describe the original artifact
    2. Ask model to regenerate artifact from description
    3. Compare original with regenerated
    """
    # Step 1: Describe the original
    describe_prompt = f"""You are given the following Netdata configuration artifact.
Describe it completely and precisely. Include every detail needed to recreate it exactly.

ARTIFACT:
```
{test_case.original}
```

Provide a complete description that captures all fields, values, and their meanings."""

    describe_response = call_model(
        model,
        system_prompt=skill_content,
        user_prompt=describe_prompt,
    )

    if not describe_response.success:
        return TestResult(
            test_name=test_case.name,
            model_name=model.name,
            model_size=model_size,
            passed=False,
            original=test_case.original,
            description="",
            regenerated="",
            differences=[],
            error=f"Describe step failed: {describe_response.error}",
        )

    description = describe_response.content

    # Step 2: Regenerate from description
    regenerate_prompt = f"""Based on the following description, recreate the exact Netdata configuration artifact.
Output ONLY the configuration, no explanations or markdown formatting.

DESCRIPTION:
{description}

Output the configuration exactly as it should appear in the configuration file."""

    regenerate_response = call_model(
        model,
        system_prompt=skill_content,
        user_prompt=regenerate_prompt,
    )

    if not regenerate_response.success:
        return TestResult(
            test_name=test_case.name,
            model_name=model.name,
            model_size=model_size,
            passed=False,
            original=test_case.original,
            description=description,
            regenerated="",
            differences=[],
            reasoning_describe=describe_response.reasoning,
            error=f"Regenerate step failed: {regenerate_response.error}",
        )

    regenerated = regenerate_response.content

    # Step 3: Compare
    passed, differences = compare_fn(test_case.original, regenerated)

    return TestResult(
        test_name=test_case.name,
        model_name=model.name,
        model_size=model_size,
        passed=passed,
        original=test_case.original,
        description=description,
        regenerated=regenerated,
        differences=differences,
        reasoning_describe=describe_response.reasoning,
        reasoning_regenerate=regenerate_response.reasoning,
    )


def run_test_suite(
    test_cases: List[TestCase],
    skill_path: Path,
    compare_fn: Callable[[str, str], tuple[bool, List[str]]],
    config: Optional[dict] = None,
) -> TestSuiteResult:
    """
    Run a complete test suite against both models.

    Returns results with summary:
    - PASS: Both models passed
    - WARNING: Large passed, small failed
    - FAIL: Large model failed
    """
    if config is None:
        config = load_config()

    skill_content = load_skill(skill_path)
    results = []

    large_model = get_model_config("large", config)
    small_model = get_model_config("small", config)

    for test_case in test_cases:
        print(f"\n  Testing: {test_case.name}")

        # Test with large model first
        print(f"    {large_model.name}...", end=" ", flush=True)
        large_result = run_single_test(
            test_case, large_model, "large", skill_content, compare_fn
        )
        results.append(large_result)
        print("PASS" if large_result.passed else "FAIL")

        # Test with small model
        print(f"    {small_model.name}...", end=" ", flush=True)
        small_result = run_single_test(
            test_case, small_model, "small", skill_content, compare_fn
        )
        results.append(small_result)
        print("PASS" if small_result.passed else "FAIL")

    # Calculate summary
    large_results = [r for r in results if r.model_size == "large"]
    small_results = [r for r in results if r.model_size == "small"]

    large_passed = sum(1 for r in large_results if r.passed)
    small_passed = sum(1 for r in small_results if r.passed)

    total_tests = len(test_cases)

    # Determine overall status
    if large_passed == total_tests and small_passed == total_tests:
        status = "PASS"
    elif large_passed == total_tests:
        status = "WARNING"  # Large passed but small failed some
    else:
        status = "FAIL"

    summary = {
        "status": status,
        "total_tests": total_tests,
        "large_model": {
            "name": large_model.name,
            "passed": large_passed,
            "failed": total_tests - large_passed,
        },
        "small_model": {
            "name": small_model.name,
            "passed": small_passed,
            "failed": total_tests - small_passed,
        },
    }

    return TestSuiteResult(
        skill_name=skill_path.stem,
        timestamp=datetime.now().isoformat(),
        results=results,
        summary=summary,
    )


def save_results(result: TestSuiteResult, output_dir: Path):
    """Save test results to JSON file."""
    output_dir.mkdir(parents=True, exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    output_file = output_dir / f"{result.skill_name}_{timestamp}.json"

    # Convert to serializable format
    output = {
        "skill_name": result.skill_name,
        "timestamp": result.timestamp,
        "summary": result.summary,
        "results": [asdict(r) for r in result.results],
    }

    with open(output_file, "w") as f:
        json.dump(output, f, indent=2)

    print(f"\nResults saved to: {output_file}")
    return output_file


def print_summary(result: TestSuiteResult):
    """Print test summary to console."""
    s = result.summary

    print("\n" + "=" * 60)
    print(f"SKILL: {result.skill_name}")
    print("=" * 60)

    status_colors = {
        "PASS": "\033[92m",    # Green
        "WARNING": "\033[93m", # Yellow
        "FAIL": "\033[91m",    # Red
    }
    reset = "\033[0m"

    color = status_colors.get(s["status"], "")
    print(f"\nStatus: {color}{s['status']}{reset}")

    print(f"\n{s['large_model']['name']} (large):")
    print(f"  Passed: {s['large_model']['passed']}/{s['total_tests']}")

    print(f"\n{s['small_model']['name']} (small):")
    print(f"  Passed: {s['small_model']['passed']}/{s['total_tests']}")

    # Show failures
    failures = [r for r in result.results if not r.passed]
    if failures:
        print("\nFailures:")
        for f in failures:
            print(f"\n  {f.test_name} ({f.model_name}):")
            if f.error:
                print(f"    Error: {f.error}")
            else:
                for diff in f.differences[:5]:  # Show first 5 differences
                    print(f"    - {diff}")


def main():
    parser = argparse.ArgumentParser(
        description="Run round-trip tests for AI skills"
    )
    parser.add_argument(
        "skill",
        type=Path,
        help="Path to skill file",
    )
    parser.add_argument(
        "test_cases",
        type=Path,
        help="Path to test cases directory or file",
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

    # Load test cases
    # TODO: Implement test case loading based on format
    print("Loading test cases...")
    test_cases = []  # Placeholder

    if not test_cases:
        print("No test cases found. Please add test cases to run.")
        sys.exit(0)

    print(f"Running {len(test_cases)} tests...")

    config = None
    if args.config:
        import yaml
        with open(args.config) as f:
            config = yaml.safe_load(f)

    result = run_test_suite(
        test_cases,
        args.skill,
        alerts_equal,  # Default comparator for alerts
        config,
    )

    print_summary(result)
    save_results(result, args.output)

    # Exit with appropriate code
    if result.summary["status"] == "FAIL":
        sys.exit(1)
    elif result.summary["status"] == "WARNING":
        sys.exit(2)
    else:
        sys.exit(0)


if __name__ == "__main__":
    main()
