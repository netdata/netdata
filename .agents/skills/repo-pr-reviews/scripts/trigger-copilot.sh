#!/usr/bin/env bash
# Re-request the GitHub Copilot reviewer on a PR.
#
# Copilot does not react to comments. It re-runs only when re-added as a
# requested reviewer. Calling this after pushing is the right way to get a
# follow-up Copilot review.
#
# Usage:
#   trigger-copilot.sh <pr-number>

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number>}"
pr_require_numeric "${PR}"
SLUG="$(pr_require_slug)"

echo -e "${PR_GRAY}[trigger-copilot] PR ${SLUG}#${PR}: re-add @copilot as reviewer${PR_NC}" >&2

# Force a fresh review run by removing then re-adding the reviewer.
# `--remove-reviewer @copilot` returns non-zero when copilot is NOT
# currently a requested reviewer (nothing to remove); we ignore that exit
# so the subsequent `--add-reviewer` always runs and triggers the review.
gh pr edit "${PR}" --repo "${SLUG}" --remove-reviewer "@copilot" >/dev/null 2>&1 || true
gh pr edit "${PR}" --repo "${SLUG}" --add-reviewer "@copilot"
echo -e "${PR_GREEN}[trigger-copilot] @copilot re-requested.${PR_NC}" >&2
