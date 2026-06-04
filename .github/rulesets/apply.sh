#!/usr/bin/env bash
# Apply (create or update) the repository rulesets defined in this directory.
#
# Rulesets are matched by name: if a ruleset with the same "name" already exists
# it is updated in place, otherwise it is created. Requires the `gh` CLI,
# authenticated as a repo admin (`gh auth login`), and `jq`.
#
# The numeric id of the release GitHub App (used as the Integration bypass actor)
# is supplied at runtime via RELEASE_APP_ID so it never has to be committed to the
# repo. A private app's id cannot be read back through the REST API with a normal
# user token, so pass it from your secret store when running this script.
#
# Usage:
#   RELEASE_APP_ID=1234567 .github/rulesets/apply.sh
#   REPO=thiemok/tiny-dash RELEASE_APP_ID=1234567 .github/rulesets/apply.sh
set -euo pipefail

APP_ID="${RELEASE_APP_ID:?Set RELEASE_APP_ID to the release GitHub App numeric App ID, found on its General settings page}"
REPO="${REPO:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Applying rulesets to ${REPO} (bypass app id ${APP_ID})"

# Map of existing ruleset name -> id
existing="$(gh api "repos/${REPO}/rulesets" --jq '.[] | "\(.name)\t\(.id)"' 2>/dev/null || true)"

for file in "${DIR}"/*.json; do
  name="$(jq -r '.name' "$file")"

  # Inject the supplied app id into any Integration bypass actor.
  payload="$(jq --argjson appid "$APP_ID" \
    '.bypass_actors |= map(if .actor_type == "Integration" then .actor_id = $appid else . end)' \
    "$file")"

  id="$(printf '%s\n' "$existing" | awk -F'\t' -v n="$name" '$1==n {print $2}')"
  if [ -n "${id}" ]; then
    echo "  • updating ruleset '${name}' (id ${id})"
    printf '%s' "$payload" | gh api -X PUT "repos/${REPO}/rulesets/${id}" --input - >/dev/null
  else
    echo "  • creating ruleset '${name}'"
    printf '%s' "$payload" | gh api -X POST "repos/${REPO}/rulesets" --input - >/dev/null
  fi
done

echo "Done."
