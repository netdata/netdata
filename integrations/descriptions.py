#!/usr/bin/env python3
"""Shared description extraction and validation for integration documentation."""

from __future__ import annotations

import html
import re
import unicodedata
from collections import Counter
from typing import Any, Dict, Iterable, Optional

from markdown_it import MarkdownIt


MIN_DESCRIPTION_LENGTH = 50
MAX_DESCRIPTION_LENGTH = 160
_MISSING_ID = "<missing-id>"
_OVERVIEW_HEADING = "## Overview"

DOCUMENTATION_TYPES = {
    "agent_notification",
    "authentication",
    "cloud_notification",
    "collector",
    "device",
    "exporter",
    "flows",
    "logs",
    "secretstore",
    "service_discovery",
}

_ASCII_URL_PREFIX_PATTERN = (
    r"(?:[A-Za-z][A-Za-z0-9+.-]*://|"
    r"[Mm][Aa][Ii][Ll][Tt][Oo]:|"
    r"[Ww][Ww][Ww]\.)"
)
_BARE_URL_RE = re.compile(_ASCII_URL_PREFIX_PATTERN + r"\S*")
_URL_SYNTAX_RE = re.compile(_ASCII_URL_PREFIX_PATTERN)
_MARKDOWN_LINK_RE = re.compile(r"!?\[([^]]*)\]\([^)]*\)")
_MARKDOWN_SPECIAL_CHARACTER_RE = re.compile(r"[*_\[\]<>#`~]")
_COMMONMARK_LIST_START_RE = re.compile(r"^(?:[-+*] |\d{1,9}[.)] )", re.ASCII)
_COMMONMARK_THEMATIC_BREAK_RE = re.compile(r"^-(?: *-){2,}$")
_RELATED_RESOURCE_RE = re.compile(
    r'\{% relatedResource id="[^"]*" %\}(.*?)\{% /relatedResource %\}',
    re.DOTALL,
)
_SENTENCE_END_RE = re.compile(r"(?<=[.!?])(?:\s+|$)")
_CONTROL_CHARACTER_RE = re.compile(r"[\x00-\x1f\x7f-\x9f\u2028\u2029\ud800-\udfff]")
_COMMONMARK = MarkdownIt("commonmark")


def parentheses_are_balanced(text: str) -> bool:
    """Return whether round parentheses are balanced, including nested pairs."""
    depth = 0
    for character in text:
        if character == "(":
            depth += 1
        elif character == ")":
            depth -= 1
            if depth < 0:
                return False
    return depth == 0


def _remove_fenced_blocks(markdown: str) -> str:
    return re.sub(r"```.*?```|~~~.*?~~~", " ", markdown, flags=re.DOTALL)


def _remove_admonition_blocks(markdown: str) -> str:
    lines = []
    in_admonition = False

    for line in markdown.splitlines():
        stripped = line.strip()
        if stripped.startswith(":::"):
            in_admonition = stripped != ":::"
            continue
        if not in_admonition:
            lines.append(line)

    return "\n".join(lines)


def markdown_to_plain_text(markdown: str) -> str:
    """Reduce inline Markdown and HTML to one line of readable text."""
    text = _remove_fenced_blocks(markdown or "")
    text = re.sub(r"<!--.*?-->", " ", text, flags=re.DOTALL)
    text = _RELATED_RESOURCE_RE.sub(r"\1", text)
    text = _MARKDOWN_LINK_RE.sub(r"\1", text)
    text = re.sub(r"<https?://[^>]+>", " ", text)
    text = re.sub(r"<[^>]+>", " ", text)
    text = _BARE_URL_RE.sub(" ", text)
    # Preserve underscores so identifiers cannot silently change meaning. The
    # final plain-text contract rejects them and requires an explicit override.
    text = re.sub(r"[`*~]+", "", text)
    text = re.sub(r"\s+", " ", html.unescape(text)).strip()
    if len(text) >= 2 and text.startswith('"') and text.endswith('"'):
        text = text[1:-1].strip()
    return text


def extract_first_sentence(text: str) -> str:
    """Extract the first period-terminated sentence using the catalog's legacy rules."""
    if not text:
        return text

    match = re.search(r"\.\s", text)
    if match:
        return text[: match.start() + 1].strip()
    if text.endswith("."):
        return text.strip()
    return text.strip()


def _first_prose_paragraph(markdown: str) -> Optional[str]:
    if not markdown or not markdown.strip():
        return None

    text = _remove_admonition_blocks(markdown)
    tokens = _COMMONMARK.parse(text)
    content_start = 0
    for index, token in enumerate(tokens[:-1]):
        if token.type != "heading_open" or token.tag != "h2" or token.level != 0:
            continue
        inline = tokens[index + 1]
        if inline.type == "inline" and inline.content.strip() == "Overview":
            content_start = index + 3
            break

    for index in range(content_start, len(tokens) - 1):
        token = tokens[index]
        if token.type != "paragraph_open" or token.level != 0:
            continue

        inline = tokens[index + 1]
        if inline.type != "inline":
            continue
        lines = [line.strip() for line in inline.content.splitlines() if line.strip()]
        if not lines:
            continue
        first = lines[0]
        if first.startswith(("#", "Plugin:", "Module:", "Kind:", "<img", "|", "- ", "* ")):
            continue

        plain = markdown_to_plain_text(" ".join(lines))
        if plain:
            return plain

    return None


def _summarize(text: str) -> str:
    boundaries = list(_SENTENCE_END_RE.finditer(text))
    summary = ""

    for boundary in boundaries:
        summary = text[: boundary.start()].strip()
        if len(summary) >= MIN_DESCRIPTION_LENGTH:
            break

    return summary or text.strip()


def _truncate(text: str) -> str:
    if len(text) <= MAX_DESCRIPTION_LENGTH:
        return text

    prefix = text[: MAX_DESCRIPTION_LENGTH - 1]
    if " " in prefix:
        prefix = prefix.rsplit(" ", 1)[0]
    return prefix.rstrip(" ,;:-") + "…"


def normalize_description(text: str, *, summarize: bool) -> str:
    """Normalize and optionally summarize mechanically derived overview prose."""
    plain = markdown_to_plain_text(text)
    if summarize:
        plain = _summarize(plain)
    return _truncate(plain)


def _legacy_overview_paragraph(overview: str) -> Optional[str]:
    _, separator, body = overview.partition(_OVERVIEW_HEADING)
    if not separator:
        return None

    paragraph = []
    for raw_line in body.strip().splitlines():
        line = raw_line.strip()
        if not line:
            if paragraph:
                break
            continue
        if line.startswith(("#", "Plugin:", "Module:")):
            if paragraph:
                break
            continue
        paragraph.append(line)

    return " ".join(paragraph) or None


def _legacy_overview_description(overview: str) -> Optional[str]:
    paragraph = _legacy_overview_paragraph(overview)
    if not paragraph:
        return None
    first_sentence = extract_first_sentence(paragraph)
    return re.sub(r"\s+", " ", first_sentence) if first_sentence else None


def extract_description_from_overview(overview: str, *, for_meta: bool = False) -> Optional[str]:
    """Extract overview prose, preserving the existing catalog output by default."""
    if not for_meta:
        return _legacy_overview_description(overview)

    paragraph = _first_prose_paragraph(overview)
    if not paragraph:
        return None
    return normalize_description(paragraph, summarize=True)


def get_description_override(integration: Dict[str, Any]) -> Optional[str]:
    """Return the exact explicit metadata description, if present and valid."""
    integration_id = integration.get("id", _MISSING_ID)
    meta = integration.get("meta", {})
    if not isinstance(meta, dict):
        raise ValueError(
            f"Invalid description for {integration_id}: meta must be a mapping: {meta!r}"
        )
    monitored_instance = meta.get("monitored_instance")
    owner = monitored_instance if isinstance(monitored_instance, dict) else meta
    if not isinstance(owner, dict) or "description" not in owner:
        return None

    value = owner["description"]
    if not isinstance(value, str):
        raise ValueError(f"Invalid description for {integration_id}: must be a string: {value!r}")

    validate_description(value, integration_id)
    return value


def validate_description(description: str, integration_id: str) -> None:
    if not isinstance(description, str):
        raise ValueError(f"Invalid description for {integration_id}: must be a string: {description!r}")

    errors = []
    length = len(description)

    if length < MIN_DESCRIPTION_LENGTH or length > MAX_DESCRIPTION_LENGTH:
        errors.append(
            f"length {length} is outside {MIN_DESCRIPTION_LENGTH}-{MAX_DESCRIPTION_LENGTH} characters"
        )
    if description != description.strip():
        errors.append("contains leading or trailing whitespace")
    if _CONTROL_CHARACTER_RE.search(description):
        errors.append("contains a control, surrogate, or Unicode line/paragraph separator")
    if _URL_SYNTAX_RE.search(description):
        errors.append("contains a URL")
    if _MARKDOWN_SPECIAL_CHARACTER_RE.search(description):
        errors.append("contains a Markdown-special character")
    if description.startswith("-"):
        errors.append("starts with a character that Learn's frontmatter parser strips")
    if (
        _COMMONMARK_LIST_START_RE.search(description)
        or _COMMONMARK_THEMATIC_BREAK_RE.search(description)
    ):
        errors.append("starts a CommonMark block")
    if description.endswith(":"):
        errors.append("ends with a colon")
    if description.endswith(("…", "...")):
        errors.append("ends with an ellipsis")
    if not parentheses_are_balanced(description):
        errors.append("contains unbalanced parentheses")
    if '"' in description or "\\" in description:
        errors.append("contains characters that Learn's frontmatter parser cannot preserve")

    if errors:
        raise ValueError(f"Invalid description for {integration_id}: {'; '.join(errors)}: {description!r}")


def get_integration_meta_description(integration: Dict[str, Any]) -> str:
    """Resolve the override-first description used in generated page frontmatter."""
    integration_id = integration.get("id", _MISSING_ID)
    description = get_description_override(integration)
    if description is None:
        description = extract_description_from_overview(integration.get("overview", ""), for_meta=True)
    if description is None:
        raise ValueError(f"Missing description source for {integration_id}")

    validate_description(description, integration_id)
    return description


def build_description_index(integrations: Iterable[Dict[str, Any]]) -> Dict[str, str]:
    """Resolve and validate every generated documentation description, including uniqueness."""
    descriptions = {}
    normalized_to_ids = {}

    for integration in integrations:
        if integration.get("integration_type") not in DOCUMENTATION_TYPES:
            continue

        integration_id = integration.get("id", _MISSING_ID)
        description = get_integration_meta_description(integration)
        descriptions[integration_id] = description
        identity = unicodedata.normalize("NFC", description.casefold())
        normalized_to_ids.setdefault(identity, []).append(integration_id)

    duplicates = [ids for ids in normalized_to_ids.values() if len(ids) > 1]
    if duplicates:
        details = "; ".join(", ".join(ids) for ids in duplicates)
        raise ValueError(f"Duplicate generated descriptions: {details}")

    return descriptions


def description_report(integrations: Iterable[Dict[str, Any]]) -> Dict[str, Any]:
    """Return deterministic counts used by the generator's check-only mode."""
    integrations = [
        integration
        for integration in integrations
        if integration.get("integration_type") in DOCUMENTATION_TYPES
    ]
    descriptions = build_description_index(integrations)
    modes = Counter(integration["integration_type"] for integration in integrations)
    overrides = sum(get_description_override(integration) is not None for integration in integrations)

    return {
        "pages": len(descriptions),
        "modes": dict(sorted(modes.items())),
        "explicit_overrides": overrides,
        "mechanical_descriptions": len(descriptions) - overrides,
    }
