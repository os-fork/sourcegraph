#!/usr/bin/env bash

# Run `sg wolfi lock` to update all package lockfiles for Wolfi base images.
# Push a new branch to GitHub, and open a PR.
# Can be run from any base branch, and will create an appropriate PR.

# set -exu -o pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../../../.."

echo "~~~ :aspect: :stethoscope: Agent Health check"
/etc/aspect/workflows/bin/agent_health_check

aspectRC="/tmp/aspect-generated.bazelrc"
rosetta bazelrc >"$aspectRC"
export BAZELRC="$aspectRC"

echo "~~~ Running sg wolfi lock"

echo "Author Name: $GIT_AUTHOR_NAME"
echo "Author Email: $GIT_AUTHOR_EMAIL"
echo "Committer Name: $GIT_COMMITTER_NAME"
echo "Committer Email: $GIT_COMMITTER_EMAIL"

echo "git config Author Name: $(git config user.name)"
echo "git config Author Email: $(git config user.email)"

buildkite-agent artifact download sg . --step bazel-prechecks
chmod +x ./sg

# Update hashes for all base images
./sg wolfi lock

echo "~~~ Committing changes and opening PR"

# Print git status
echo "[$(date)] Running git status"
git status

# Git and GitHub config
BRANCH_NAME="wolfi-auto-update/${BUILDKITE_BRANCH}"
TIMESTAMP=$(TZ=UTC date "+%Y-%m-%d %H:%M:%S UTC")
PR_TITLE="Auto-update package lockfiles for Wolfi base images"
# PR_REVIEWER="sourcegraph/security"
PR_LABELS="SSDLC,security-auto-update,security-auto-update/images"
PR_BODY="Automatically generated PR to update package lockfiles for Wolfi base images.

Built from Buildkite run [#${BUILDKITE_BUILD_NUMBER}](https://buildkite.com/sourcegraph/sourcegraph/builds/${BUILDKITE_BUILD_NUMBER}).
## Test Plan
- CI build verifies image functionality
- [ ] Confirm PR should be backported to release branch"

# Ensure git commit details are correct
git config --global user.email \"buildkite@sourcegraph.com\"
git config --global user.name \"Buildkite\"

# Commit changes to dev/oci-deps.bzl
# Delete branch if it exists; catch status code if not
echo "[$(date)] Deleting branch ${BRANCH_NAME} if it exists"
git branch -D "${BRANCH_NAME}" || true
echo "[$(date)] Switching to new branch ${BRANCH_NAME}"
git switch -c "${BRANCH_NAME}"
echo "[$(date)] Git add lockfiles"
git add wolfi-images/*.lock.json
echo "[$(date)] Git commit"
git commit -m "Auto-update package lockfiles for Wolfi base images at ${TIMESTAMP}"
echo "[$(date)] Git push"
git push --force -u origin "${BRANCH_NAME}"
echo ":git: Successfully commited changes and pushed to branch ${BRANCH_NAME}"

GHR="bazel --bazelrc=${aspectRC} run //dev/tools:gh"

# Check if an update PR already exists
if $GHR -- pr list --head "${BRANCH_NAME}" --state open | grep -q "${PR_TITLE}"; then
  echo ":github: A pull request already exists - editing it"
  $GHR -- pr edit "${BRANCH_NAME}" --body "${PR_BODY}"
else
  # If not, create a new PR from the branch
  $GHR -- pr create --title "${PR_TITLE}" --head "${BRANCH_NAME}" --base "${BUILDKITE_BRANCH}" --body "${PR_BODY}" --label "${PR_LABELS}"
  echo ":github: Created a new pull request from branch '${BRANCH_NAME}' with title '${PR_TITLE}'"
fi
