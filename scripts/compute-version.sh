#!/usr/bin/env bash
set -euo pipefail

pattern='^v[0-9]+\.[0-9]+\.[0-9]+$'
latest_tag="$(git tag --sort=-v:refname | grep -E "$pattern" | head -n1 || true)"

if [[ -z "$latest_tag" ]]; then
  base_version='0.0.0'
  distance="$(git rev-list HEAD --count)"
else
  base_version="${latest_tag#v}"
  distance="$(git rev-list "${latest_tag}..HEAD" --count)"
fi

if [[ "$distance" == '0' ]]; then
  echo "$base_version"
else
  echo "${base_version}.${distance}"
fi
