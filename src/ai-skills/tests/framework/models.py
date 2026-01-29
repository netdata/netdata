#!/usr/bin/env python3
"""
Model API wrappers for AI Skills testing.
Supports both standard and reasoning models via OpenAI-compatible endpoints.
"""

import json
import requests
from dataclasses import dataclass
from typing import Optional
from pathlib import Path
import yaml


@dataclass
class ModelResponse:
    """Response from a model API call."""
    content: str
    reasoning: Optional[str] = None
    model: str = ""
    prompt_tokens: int = 0
    completion_tokens: int = 0
    finish_reason: str = ""  # "stop", "length", etc.
    truncated: bool = False  # True if finish_reason == "length"
    success: bool = True
    error: Optional[str] = None


@dataclass
class ModelConfig:
    """Configuration for a model endpoint."""
    name: str
    endpoint: str
    model_id: str
    max_tokens: int = 4096
    is_reasoning: bool = False
    timeout: int = 120


def load_config(config_path: Optional[Path] = None) -> dict:
    """Load configuration from YAML file."""
    if config_path is None:
        config_path = Path(__file__).parent.parent / "config.yaml"

    with open(config_path) as f:
        return yaml.safe_load(f)


def get_model_config(model_size: str, config: Optional[dict] = None) -> ModelConfig:
    """Get model configuration by size (large/small)."""
    if config is None:
        config = load_config()

    model_cfg = config["models"][model_size]
    timeout = config.get("test_settings", {}).get("timeout", 120)

    return ModelConfig(
        name=model_cfg["name"],
        endpoint=model_cfg["endpoint"],
        model_id=model_cfg["model_id"],
        max_tokens=model_cfg.get("max_tokens", 4096),
        is_reasoning=model_cfg.get("is_reasoning", False),
        timeout=timeout,
    )


def call_model(
    model: ModelConfig,
    system_prompt: str,
    user_prompt: str,
    temperature: float = 0.0,
) -> ModelResponse:
    """
    Call a model with the given prompts.

    Args:
        model: Model configuration
        system_prompt: System/instruction prompt
        user_prompt: User message
        temperature: Sampling temperature (0.0 for deterministic)

    Returns:
        ModelResponse with content and optional reasoning
    """
    url = f"{model.endpoint}/chat/completions"

    payload = {
        "model": model.model_id,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": user_prompt},
        ],
        "max_tokens": model.max_tokens,
        "temperature": temperature,
    }

    try:
        response = requests.post(
            url,
            headers={"Content-Type": "application/json"},
            json=payload,
            timeout=model.timeout,
        )
        response.raise_for_status()
        data = response.json()

        choice = data["choices"][0]
        message = choice["message"]
        usage = data.get("usage", {})
        finish_reason = choice.get("finish_reason", "")

        # Extract content - handle reasoning models
        content = message.get("content") or ""
        reasoning = message.get("reasoning_content") or message.get("reasoning")

        # Check if response was truncated due to max_tokens
        truncated = finish_reason == "length"

        return ModelResponse(
            content=content,
            reasoning=reasoning,
            model=model.name,
            prompt_tokens=usage.get("prompt_tokens", 0),
            completion_tokens=usage.get("completion_tokens", 0),
            finish_reason=finish_reason,
            truncated=truncated,
            success=True,
        )

    except requests.exceptions.Timeout:
        return ModelResponse(
            content="",
            model=model.name,
            success=False,
            error=f"Timeout after {model.timeout}s",
        )
    except requests.exceptions.RequestException as e:
        return ModelResponse(
            content="",
            model=model.name,
            success=False,
            error=str(e),
        )
    except (KeyError, json.JSONDecodeError) as e:
        return ModelResponse(
            content="",
            model=model.name,
            success=False,
            error=f"Invalid response: {e}",
        )


def test_model_connection(model: ModelConfig) -> bool:
    """Test if a model endpoint is reachable and responding."""
    try:
        response = requests.get(
            f"{model.endpoint}/models",
            timeout=10,
        )
        return response.status_code == 200
    except requests.exceptions.RequestException:
        return False


if __name__ == "__main__":
    # Quick test of both models
    config = load_config()

    for size in ["large", "small"]:
        model = get_model_config(size, config)
        print(f"\nTesting {model.name} ({size})...")

        if test_model_connection(model):
            print(f"  Connection: OK")
            response = call_model(
                model,
                system_prompt="You are a helpful assistant.",
                user_prompt="Say 'test successful' and nothing else.",
            )
            if response.success:
                print(f"  Response: {response.content}")
                if response.reasoning:
                    print(f"  Reasoning: {response.reasoning[:100]}...")
            else:
                print(f"  Error: {response.error}")
        else:
            print(f"  Connection: FAILED")
