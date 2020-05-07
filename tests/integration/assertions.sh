#!/bin/sh

assert_equals() {
  [ x"$1" = x"$2" ] && return 0
  printf >&2 "Assertion Failed: %s = %s\n" "$1" "$2"
  exit 1
}

assert_not_equals() {
  [ "$1" != "$2" ] && return 0
  printf >&2 "Assertion Failed; %s != %s\n" "$1" "$2"
  exit 1
}
