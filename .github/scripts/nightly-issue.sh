#!/bin/sh
# nightly-issue.sh <name> failure|success: one open issue a nightly,
# labelled nightly:<name>. A failure opens it, or comments on the one
# open; a success closes it. A red nightly is seen the next morning, not
# five days later. Needs gh, GH_TOKEN with issues: write.
set -eu
name=$1
state=$2
label="nightly:$name"
run="$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID"
n=$(gh issue list --label "$label" --state open --json number --jq '.[0].number // empty')
if [ "$state" = failure ]; then
  if [ -n "$n" ]; then
    gh issue comment "$n" --body "Failed again: $run"
  else
    gh label create "$label" --color B60205 --force >/dev/null
    gh issue create --title "The nightly $name fails" --label "$label" \
      --body "The nightly $name failed: $run. This issue closes itself on the next pass."
  fi
elif [ -n "$n" ]; then
  gh issue close "$n" --comment "Passed again: $run"
fi
