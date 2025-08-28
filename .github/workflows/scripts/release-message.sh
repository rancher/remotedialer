#!/bin/sh
#
# Automatically generates a message for a new release of remotedialer with some useful
# links and embedded release notes.
#
# Usage:
#   ./release-message.sh <prev remotedialer release> <new remotedialer release>
#
# Example:
# ./release-message.sh "v0.5.2-rc.3" "v0.5.2-rc.4"

PREV_REMOTEDIALER_VERSION=$1   # e.g. v0.5.2-rc.3
NEW_REMOTEDIALER_VERSION=$2    # e.g. v0.5.2-rc.4
GITHUB_TRIGGERING_ACTOR=${GITHUB_TRIGGERING_ACTOR:-}

usage() {
    cat <<EOF
Usage:
  $0 <prev remotedialer release> <new remotedialer release>
EOF
}

if [ -z "$PREV_REMOTEDIALER_VERSION" ] || [ -z "$NEW_REMOTEDIALER_VERSION" ]; then
    usage
    exit 1
fi

set -ue

# XXX: That's wasteful but doing it by caching the response in a var was giving
# me unicode error.
url=$(gh release view --repo rancher/remotedialer --json url "${NEW_REMOTEDIALER_VERSION}" --jq '.url')
body=$(gh release view --repo rancher/remotedialer --json body "${NEW_REMOTEDIALER_VERSION}" --jq '.body')

generated_by=""
if [ -n "$GITHUB_TRIGGERING_ACTOR" ]; then
    generated_by=$(cat <<EOF
# About this PR

The workflow was triggered by $GITHUB_TRIGGERING_ACTOR.
EOF
)
fi

cat <<EOF
# Release note for [${NEW_REMOTEDIALER_VERSION}]($url)

$body

# Useful links

- Commit comparison: https://github.com/rancher/remotedialer/compare/${PREV_REMOTEDIALER_VERSION}...${NEW_REMOTEDIALER_VERSION}
- Release ${PREV_REMOTEDIALER_VERSION}: https://github.com/rancher/remotedialer/releases/tag/${PREV_REMOTEDIALER_VERSION}

$generated_by
EOF
