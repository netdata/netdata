#/bin/sh

if [ ! -t 1 ] && ! printf "\n"; then
  echo "foo"
else
  echo "bar"
fi
