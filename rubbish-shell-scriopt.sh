#!/bin/sh

# XXX: Confirming we don't do `shellcheck` on shell scripts (Re: #7770)

if [ -z x"FOO' == ]; then
  echo "..."
fi
