#!/usr/bin/env bash

set -euo pipefail

root=$(git rev-parse --show-toplevel)
website="$root/website"
docs="$website/src/content/docs"
temp=$(mktemp -d "${TMPDIR:-/tmp}/lnr-versioned-site.XXXXXX")
dev_docs="$temp/dev-docs"
stable_tag=""
stable_routes="/|"

while IFS= read -r tag; do
  if [[ "$tag" != *-* ]]; then
    stable_tag="$tag"
    break
  fi
done < <(git tag --list 'v[0-9]*' --sort=-v:refname)

if [[ -z "$stable_tag" ]]; then
  echo "no stable documentation tag found" >&2
  exit 1
fi
stable_version=${stable_tag#v}

stable_has_docs=false
root_docs_version=dev
if git cat-file -e "$stable_tag:website/src/content/docs" 2>/dev/null; then
  stable_has_docs=true
  root_docs_version=$stable_version
fi

if $stable_has_docs; then
  route_source=$(git ls-tree -r --name-only "$stable_tag" website/src/content/docs)
else
  route_source=$(find "$docs" -type f -print | sort)
fi

while IFS= read -r file; do
  file=${file#"$root/"}
  route=${file#website/src/content/docs/}
  route=${route%.*}
  route=${route%/index}
  [[ "$route" == "index" ]] && route=""
  stable_routes+="/docs/${route:+$route/}|"
done <<< "$route_source"

restore_docs() {
  [[ -d "$dev_docs" ]] || return 0
  [[ ! -d "$docs" ]] || mv "$docs" "$temp/interrupted-docs"
  mv "$dev_docs" "$docs"
}

finish() {
  restore_docs
  if command -v trash >/dev/null; then
    trash "$temp"
  fi
}
trap finish EXIT

aube --dir "$website" run check

if $stable_has_docs; then
  mv "$docs" "$dev_docs"
  mkdir -p "$temp/stable-source"
  git archive "$stable_tag" website/src/content/docs | tar -x -C "$temp/stable-source"
  mv "$temp/stable-source/website/src/content/docs" "$docs"
fi

LNR_DOCS_VERSION="$root_docs_version" \
  LNR_HAS_STABLE_DOCS="$stable_has_docs" LNR_STABLE_VERSION="$stable_version" \
  LNR_STABLE_ROUTES="$stable_routes" LNR_SITE_BASE=/ \
  aube --dir "$website" run build-only
mv "$website/dist" "$temp/stable-dist"

if $stable_has_docs; then
  mv "$docs" "$temp/stable-docs"
  mv "$dev_docs" "$docs"
fi

LNR_DOCS_VERSION=dev LNR_STABLE_VERSION="$stable_version" \
  LNR_HAS_STABLE_DOCS="$stable_has_docs" \
  LNR_STABLE_ROUTES="$stable_routes" LNR_SITE_BASE=/dev \
  aube --dir "$website" run build-only
mv "$website/dist" "$temp/dev-dist"

mv "$temp/stable-dist" "$website/dist"
mkdir -p "$website/dist/dev"
cp -R "$temp/dev-dist/." "$website/dist/dev/"
