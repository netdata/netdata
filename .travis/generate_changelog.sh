#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

ORGANIZATION=$(echo "$TRAVIS_REPO_SLUG" | awk -F '/' '{print $1}')
PROJECT=$(echo "$TRAVIS_REPO_SLUG" | awk -F '/' '{print $2}')
GIT_MAIL=${GIT_MAIL:-"pawel+bot@netdata.cloud"}
GIT_USER=${GIT_USER:-"netdatabot"}

echo "--- Initialize git configuration ---"
git config user.email "${GIT_MAIL}"
git config user.name "${GIT_USER}"

echo "--- Creating changelog ---"
git checkout master
git pull
#docker run -it --rm -v "$(pwd)":/usr/local/src/your-app ferrarimarco/github-changelog-generator:1.14.3 \
docker run -it -v "$(pwd)":/project markmandel/github-changelog-generator:latest \
                                                        --user "${ORGANIZATION}" \
                                                        --project "${PROJECT}" \
                                                        --token "${GITHUB_TOKEN}" \
                                                        --since-tag "v1.10.0" \
                                                        --unreleased-label "**Next release**" \
                                                        --no-compare-link \
                                                        --exclude-labels duplicate,question,invalid,wontfix,discussion,documentation

echo "--- Uploading changelog ---"
git add CHANGELOG.md
git commit -m '[ci skip] Automatic changelog update' || exit 0
git push "https://${GITHUB_TOKEN}:@$(git config --get remote.origin.url | sed -e 's/^https:\/\///')"
