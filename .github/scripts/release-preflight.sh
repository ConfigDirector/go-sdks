#!/usr/bin/env sh
#
# Decides what the release workflow releases for one module, and fails when nothing must be
# released: reads the module's `Version` constant, refuses a version whose tag already exists, and
# takes the release notes from the module's CHANGELOG.md section for that version.
# Writes `version`, `tag` and `prerelease` to $GITHUB_OUTPUT when it is set, and the notes to the
# given file. `make test-release-preflight` runs it against throwaway repositories.
#
# Usage: release-preflight.sh <module> <notes-file>, from the repository root.

set -eu

module=${1:-}
notes_file=${2:-}

if [ -z "$module" ] || [ -z "$notes_file" ]; then
    echo "usage: $0 <module> <notes-file>" >&2
    exit 2
fi

version_file="$module/version.go"
changelog="$module/CHANGELOG.md"

for file in "$version_file" "$changelog"; do
    if [ ! -f "$file" ]; then
        echo "Cannot find $file. Run this from the repository root." >&2
        exit 1
    fi
done

version=$(sed -n 's/^const Version = "\([^"]*\)"$/\1/p' "$version_file")

if [ -z "$version" ]; then
    echo "Could not read the Version constant from $version_file." >&2
    exit 1
fi

if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
    echo "Version '$version' must be a semantic version with no v prefix, such as 1.2.3 or 1.2.3-rc.1." >&2
    exit 1
fi

case "$version" in
    *-*) prerelease=true ;;
    *) prerelease=false ;;
esac

tag="$module/v$version"

if [ -n "$(git ls-remote --tags origin "refs/tags/$tag")" ]; then
    echo "$tag already exists, so $module $version has been released before. The module proxy" >&2
    echo "serves a released version forever: bump Version in $version_file, open a new" >&2
    echo "$changelog section, and run this again." >&2
    exit 1
fi

heading="## [$version]"

if ! awk -v heading="$heading" '
    /^## / {
        if (inside) exit
        inside = ($0 == heading || index($0, heading " ") == 1)
        if (inside) found = 1
        next
    }
    inside { lines[++count] = $0 }
    END {
        if (!found) exit 3
        first = 1
        while (first <= count && lines[first] ~ /^[[:space:]]*$/) first++
        last = count
        while (last >= first && lines[last] ~ /^[[:space:]]*$/) last--
        for (i = first; i <= last; i++) print lines[i]
    }
' "$changelog" > "$notes_file"; then
    rm -f "$notes_file"
    echo "$changelog has no '$heading' section. Rename its '## [Unreleased]' heading to" >&2
    echo "'$heading - YYYY-MM-DD' and open a fresh one above it before releasing." >&2
    exit 1
fi

if [ ! -s "$notes_file" ]; then
    rm -f "$notes_file"
    echo "The '$heading' section in $changelog is empty, so there are no release notes to publish." >&2
    exit 1
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        echo "version=$version"
        echo "tag=$tag"
        echo "prerelease=$prerelease"
    } >> "$GITHUB_OUTPUT"
fi

echo "Releasing $module $version as $tag with these notes:"
echo
cat "$notes_file"
