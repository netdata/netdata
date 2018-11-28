#!/bin/bash

# This is a simple script which should apply labels to unlabelled issues from last 3 days.
# It will soon be deprecated by GitHub Actions so no futher development on it is planned.

if [ "$GITHUB_TOKEN" == "" ]; then
    echo "GITHUB_TOKEN is needed"
    exit 1
fi

# Download hub
HUB_VERSION=${HUB_VERSION:-"2.5.1"}
wget "https://github.com/github/hub/releases/download/v${HUB_VERSION}/hub-linux-amd64-${HUB_VERSION}.tgz" -O "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz"
tar -C /tmp -xvf "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz" &>/dev/null
export PATH=$PATH:"/tmp/hub-linux-amd64-${HUB_VERSION}/bin"

echo "Looking up available labels"
LABELS_FILE=/tmp/exclude_labels
hub issue labels > $LABELS_FILE

for STATE in "open" "closed"; do
  for ISSUE in $(hub issue -f "%I %l%n" -s "$STATE" -d "$(date +%F -d '3 days ago')" | grep -v -f $LABELS_FILE); do
    echo "Processing $STATE issue no. $ISSUE"
    URL="https://api.github.com/repos/netdata/netdata/issues/$ISSUE"
    BODY="$(curl "${URL}" | jq .body 2>/dev/null)"
    case "${BODY}" in
      *"# Question summary"* ) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["question"]}' -X PATCH "${URL}" ;;
      *"# Bug report summary"* ) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["bug"]}' -X PATCH "${URL}" ;;
      * ) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["needs triage"]}' -X PATCH "${URL}" ;;
    esac
  done
done
