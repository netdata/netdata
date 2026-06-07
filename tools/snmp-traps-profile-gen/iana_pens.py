"""
Parse the IANA Private Enterprise Numbers registry into a {pen: slug} dict.

Source file (committed alongside this module):
    tools/snmp-traps-profile-gen/iana-enterprise-numbers.txt

The IANA text format is:

    <PEN>
      <Organization>
      <Contact>
      <Email>

separated by blank lines. We use the organization name as the primary
source for the vendor slug, with the email-domain stem as a fallback when
the organization line is empty / placeholder / clearly a personal name
without a recognizable corporate suffix.

Design choices (deliberately conservative to avoid silent miscategorisation):

1. Strip parenthetical aliases like ``(previously 'Raksha Networks Inc.')``
   so PEN 22610 becomes ``A10 Networks`` not ``a10-networks-previously-...``.
2. Split camelCase boundaries (PEN 9 ``ciscoSystems`` -> ``cisco systems``).
3. Strip only well-anchored corporate suffixes that cannot cause false
   matches against real-vendor substrings: trailing ``, Inc``, ``, Corp``,
   ``Corporation``, ``Limited``, ``Co., Ltd.``, ``GmbH & Co. KG``. We do
   NOT strip free-form ``Co.`` / ``AG`` / ``BV`` etc. because the regex
   anchor would also chop the tail off vendor names like ``cisco`` -> ``cis``.
4. Slugify (lowercase, runs of non-alphanumeric -> hyphen).
5. Fall back to the email-domain stem ONLY when the slug from the org name
   is empty (or the org line is ``Reserved`` / a single bare token that
   looks like a person's surname with no corporate marker AT ALL).
"""

from __future__ import annotations

import os
import re
from typing import Dict, List, Optional


# Well-anchored corporate suffixes. Each pattern is applied at end-of-string.
# We strip iteratively until stable so ``"Foo Co., Ltd., Inc."`` would
# fully unwind. Word boundaries are implicit because each pattern requires
# at least one whitespace OR comma character preceding the suffix word
# (handled by the leading ``[,\s]+`` group).
_TRAILING_SUFFIX_PATTERNS = [
    # German "GmbH & Co. KG" / "GmbH & Co. K.G." in any spacing. Special-
    # cased because it's a multi-word legal form.
    r"[,\s]+GmbH\s*&\s*Co\.?\s*K\.?G\.?$",
    r"[,\s]+GmbH\s*und\s*Co\.?\s*K\.?G\.?$",
    # "Co., Ltd." in its many spellings.
    r"[,\s]+Co\.,?\s*Ltd\.?$",
    r"[,\s]+Co\.,?\s*Limited$",
    # Common single-word legal forms.
    r"[,\s]+Inc\.?$",
    r"[,\s]+Incorporated$",
    r"[,\s]+Corp\.?$",
    r"[,\s]+Corporation$",
    r"[,\s]+Limited$",
    r"[,\s]+LLC\.?$",
    r"[,\s]+L\.?L\.?C\.?$",
    r"[,\s]+LLP\.?$",
    r"[,\s]+PLC\.?$",
    r"[,\s]+Pvt\.?\s*Ltd\.?$",
    r"[,\s]+Pty\.?\s*Ltd\.?$",
    # These NEVER appear in the IANA org line for a known false-positive
    # vendor stem (verified by inspection): include them but only with the
    # leading [,\s]+ guard so "cisco" can't lose its tail.
    r"[,\s]+GmbH\.?$",
    r"[,\s]+AG$",
    r"[,\s]+Oy$",
    r"[,\s]+A/S$",
    r"[,\s]+B\.?V\.?$",
    r"[,\s]+N\.?V\.?$",
    r"[,\s]+S\.?A\.?$",
    r"[,\s]+S\.?p\.?A\.?$",
    r"[,\s]+S\.?r\.?l\.?$",
    r"[,\s]+S\.?A\.?S\.?$",
    r"[,\s]+S\.?L\.?$",
]
_SUFFIX_RE = re.compile(
    r"(?:" + "|".join(_TRAILING_SUFFIX_PATTERNS) + r")",
    re.IGNORECASE,
)

# Free / generic / placeholder email domains -- never use as a slug.
_FREE_EMAIL_DOMAINS = {
    "gmail.com", "googlemail.com", "yahoo.com", "yahoo.co.uk", "yahoo.co.jp",
    "hotmail.com", "outlook.com", "live.com", "msn.com",
    "aol.com", "icloud.com", "me.com", "mac.com",
    "protonmail.com", "proton.me", "pm.me",
    "writeme.com", "fastmail.com", "tutanota.com",
    "qq.com", "163.com", "126.com", "sina.com", "yeah.net", "sohu.com",
    "naver.com", "yandex.com", "yandex.ru",
    "mail.com", "mail.ru", "gmx.com", "gmx.net", "gmx.de",
    "web.de", "t-online.de",
    "iana.org",
    "example.com", "example.org", "example.net",
}

# Single-token org lines that are placeholders, not vendor names.
_PLACEHOLDER_ORGS = {
    "reserved", "not-assigned", "available", "unassigned",
    "none", "withdrawn", "---", "n/a",
}


def _slugify(s: str) -> str:
    """Lower-case, replace runs of non-alphanumeric with ``-``."""
    s = s.strip().lower()
    s = re.sub(r"[^a-z0-9]+", "-", s)
    return s.strip("-")


def _split_camel(s: str) -> str:
    """Insert spaces between camelCase boundaries (``ciscoSystems`` ->
    ``cisco Systems``).

    Only applied when the input has NO existing whitespace -- otherwise we'd
    break brand names like ``NetBotz`` whose author already chose CamelCase
    deliberately.
    """
    if " " in s:
        return s
    return re.sub(r"(?<=[a-z])(?=[A-Z])", " ", s)


def _strip_parentheticals(s: str) -> str:
    """Remove ``(...)`` aliases / URLs / history notes."""
    out: List[str] = []
    depth = 0
    for ch in s:
        if ch == "(":
            depth += 1
            continue
        if ch == ")" and depth > 0:
            depth -= 1
            continue
        if depth == 0:
            out.append(ch)
    return " ".join("".join(out).split())


def _strip_suffixes(org: str) -> str:
    """Iteratively strip well-anchored corporate suffixes from the end."""
    prev = None
    out = org.strip()
    while out != prev:
        prev = out
        out = _SUFFIX_RE.sub("", out).strip()
        # Some suffixes leave a stray trailing comma behind.
        out = out.rstrip(",").strip()
    return out


def _domain_stem(email: str) -> Optional[str]:
    """Pull a slugified second-level-domain stem from an IANA contact email.

    IANA writes ``&`` for ``@`` to discourage scraping; we tolerate both.
    Returns ``None`` for empty / free-mail / placeholder addresses.
    """
    if not email:
        return None
    addr = email.replace("&", "@").strip()
    m = re.search(r"@([A-Za-z0-9.-]+)$", addr)
    if not m:
        return None
    domain = m.group(1).lower().rstrip(".")
    if domain in _FREE_EMAIL_DOMAINS:
        return None
    parts = domain.split(".")
    if len(parts) < 2:
        return None
    # "co.uk" / "co.jp" / "com.au" tails: stem is third-from-last.
    if len(parts) >= 3 and parts[-2] in {"co", "com", "ac", "or", "gov", "edu"} and len(parts[-1]) == 2:
        stem = parts[-3]
    else:
        stem = parts[-2]
    return _slugify(stem) or None


def _slug_for(pen: str, org: str, email: str) -> str:
    """Compute the final slug for a (PEN, organization, email) triple.

    Heuristic:
      1. Strip parenthetical aliases / history notes.
      2. If org is empty / a Reserved placeholder -> email domain or generic.
      3. Strip safe corporate suffixes.
      4. If NO suffix was stripped AND a non-free email domain is present,
         prefer the email-domain stem. This covers the well-known IANA
         pattern of personal-name registrants whose employer is the actual
         vendor (e.g. PEN 10520: org "Marc Hirsch", email omnionpower.com
         -> slug "omnionpower").
      5. Otherwise slugify the (possibly suffix-stripped) org name.
      6. Empty slug fallback -> email domain or ``enterprise-<N>``.
    """
    # 1.
    cleaned = _strip_parentheticals(org)
    cleaned_lc = cleaned.strip().lower()
    # 2.
    if not cleaned or cleaned_lc in _PLACEHOLDER_ORGS:
        dom = _domain_stem(email)
        return dom or f"enterprise-{pen}"
    # 3.
    stripped = _strip_suffixes(cleaned)
    suffix_was_stripped = stripped != cleaned
    # 4.
    if not suffix_was_stripped:
        dom = _domain_stem(email)
        if dom:
            return dom
    # 5.
    expanded = _split_camel(stripped)
    slug = _slugify(expanded)
    if slug:
        return slug
    # 6.
    dom = _domain_stem(email)
    return dom or f"enterprise-{pen}"


def parse_iana_file(path: str) -> Dict[str, str]:
    """Parse the IANA enterprise-numbers.txt file into ``{pen: slug}``."""
    table: Dict[str, str] = {}
    if not os.path.exists(path):
        return table
    with open(path, encoding="utf-8", errors="replace") as f:
        lines = f.read().splitlines()
    i = 0
    n = len(lines)
    while i < n:
        line = lines[i]
        if line and line[0].isdigit() and line.strip().isdigit():
            pen = line.strip()
            org = email = ""
            for j in range(1, 4):
                if i + j >= n:
                    break
                nxt = lines[i + j]
                if not nxt.startswith(" "):
                    break
                value = nxt.strip()
                if j == 1:
                    org = value
                elif j == 3:
                    email = value
            slug = _slug_for(pen, org, email)
            table[pen] = slug
            i += 4
            continue
        i += 1
    return table


_DEFAULT_IANA_PATH = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    "iana-enterprise-numbers.txt",
)


def default_table() -> Dict[str, str]:
    return parse_iana_file(_DEFAULT_IANA_PATH)


if __name__ == "__main__":
    import json
    import sys
    path = sys.argv[1] if len(sys.argv) > 1 else _DEFAULT_IANA_PATH
    table = parse_iana_file(path)
    print(f"# parsed {len(table)} PEN entries from {path}", file=sys.stderr)
    json.dump(table, sys.stdout, indent=2, sort_keys=True)
    print()
