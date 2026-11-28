#!/usr/bin/env bash
# Check the Go forks used by the node (see docs/forks.md):
#   1. root, evmd and the test modules that build the node (tests/speedtest,
#      tests/systemtests) replace cosmos-sdk and go-ethereum with the
#      xitcoin-xtc forks, at the same version everywhere;
#   2. each fork commit is reachable from a tag or a protected branch of the
#      fork repository, so it cannot silently disappear or be rewritten;
#   3. root and evmd have no local or unversioned replace directive, except
#      evmd's replace of the root module by ../;
#   4. the packages the forks remove, and x/crisis, are not linked.
# Needs git, jq and the Go toolchain; uses gh when available to list the
# protected branches of the forks.
set -euo pipefail

# The current fork commits are on unprotected branches. Set to true once they
# are tagged (or their branches are protected) to make that a failure.
require_tag_or_protected_branch=false

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOWORK=off
status=0

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  status=1
}

# upstream module => fork module
forks='github.com/cosmos/cosmos-sdk github.com/xitcoin-xtc/cosmos-sdk
github.com/ethereum/go-ethereum github.com/xitcoin-xtc/go-ethereum'
fork_versions=""  # lines: upstream fork version

for module in . evmd tests/speedtest tests/systemtests; do
  replaces="$(cd "$repo_root/$module" && go mod edit -json | jq -r '.Replace // [] | .[] | [.Old.Path, .New.Path, (.New.Version // "")] | @tsv')"

  # Test modules replace the root module and evmd by local paths.
  while IFS=$'\t' read -r old new version; do
    [[ -n "$old" && "$module" != tests/* ]] || continue
    if [[ -z "$version" ]]; then
      if [[ "$module" == evmd && "$old" == github.com/xitcoin-xtc/pos-chain && "$new" == ../ ]]; then
        continue
      fi
      fail "$module: local or unversioned replace $old => $new"
    fi
  done <<< "$replaces"

  while read -r old fork; do
    line="$(awk -F'\t' -v old="$old" '$1 == old' <<< "$replaces")"
    new="$(cut -f2 <<< "$line")"
    version="$(cut -f3 <<< "$line")"
    if [[ "$new" != "$fork" || -z "$version" ]]; then
      fail "$module: $old must be replaced by $fork at a version (found '${line:-none}')"
      continue
    fi
    seen="$(awk -v old="$old" '$1 == old {print $3}' <<< "$fork_versions")"
    if [[ -z "$seen" ]]; then
      fork_versions+="$old $fork $version"$'\n'
    elif [[ "$seen" != "$version" ]]; then
      fail "$old: root uses $seen, $module uses $version"
    fi
  done <<< "$forks"
done

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

while read -r old fork version; do
  [[ -n "$old" ]] || continue
  repo="${fork#github.com/}"
  git_dir="$workdir/${repo//\//_}"

  git init -q --bare "$git_dir"
  # Commits only (no trees or blobs): enough to test ancestry, a few MB.
  git -C "$git_dir" fetch -q --filter=tree:0 --no-tags "https://$fork.git" \
    '+refs/heads/*:refs/heads/*' '+refs/tags/*:refs/tags/*'

  if [[ "$version" =~ [-.][0-9]{14}-([0-9a-f]{12})$ ]]; then
    rev="${BASH_REMATCH[1]}"
  else
    rev="refs/tags/$version"
  fi
  if ! commit="$(git -C "$git_dir" rev-parse -q --verify "$rev^{commit}")"; then
    fail "$fork $version: commit $rev is not reachable from any branch or tag of the fork"
    continue
  fi

  tags="$(git -C "$git_dir" for-each-ref --contains "$commit" --format='%(refname:short)' refs/tags)"
  branches="$(git -C "$git_dir" for-each-ref --contains "$commit" --format='%(refname:short)' refs/heads)"
  protected=""
  if command -v gh > /dev/null && gh auth status > /dev/null 2>&1; then
    protected="$(gh api --paginate "repos/$repo/branches?protected=true" --jq '.[].name')"
  fi
  kept_on="$(printf '%s\n' "$tags")"
  while IFS= read -r branch; do
    [[ -n "$branch" ]] || continue
    if grep -qxF "$branch" <<< "$protected"; then
      kept_on+=$'\n'"$branch (protected)"
    fi
  done <<< "$branches"
  kept_on="$(sed '/^$/d' <<< "$kept_on" | paste -sd, - | sed 's/,/, /g')"

  if [[ -n "$kept_on" ]]; then
    printf 'ok: %s => %s %s (%s)\n' "$old" "$fork" "$version" "$kept_on"
  elif [[ -n "$branches" ]]; then
    message="$fork $version: commit ${commit:0:12} is only on unprotected branches ($(paste -sd, - <<< "$branches")); tag it"
    if [[ "$require_tag_or_protected_branch" == true ]]; then
      fail "$message"
    else
      printf 'warning: %s\n' "$message" >&2
      [[ -z "${GITHUB_ACTIONS:-}" ]] || printf '::warning title=check-forks::%s\n' "$message"
    fi
  else
    fail "$fork $version: commit ${commit:0:12} is not on any branch or tag of the fork"
  fi
done <<< "$fork_versions"

banned='^(golang\.org/x/crypto/openpgp|github\.com/pion/(dtls|stun)/v2|github\.com/cosmos/cosmos-sdk/(contrib/)?x/crisis|cosmossdk\.io/x/crisis)(/|$)'
for module in . evmd; do
  if found="$(cd "$repo_root/$module" && go list -deps ./... | grep -E "$banned")"; then
    fail "$module links banned packages: $(paste -sd' ' - <<< "$found")"
  fi
done

if [[ "$status" -eq 0 ]]; then
  echo 'check-forks: ok'
fi
exit "$status"
