#!/usr/bin/env sh
#
# Runs release-preflight.sh against throwaway repositories, one per scenario: the releases it must
# allow and every refusal it must make. `make test-release-preflight` runs this.

set -eu

scripts_directory=$(cd "$(dirname "$0")" && pwd)
preflight="$scripts_directory/release-preflight.sh"
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
failures=0

# Nothing from the developer's git configuration (hooks, signing, identity) reaches the throwaway
# repositories.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

version_go() {
    printf 'package %s\n\n// Version is the version of this module.\nconst Version = "%s"\n' "$1" "$2"
}

released_changelog() {
    printf '# Changelog\n\n## [Unreleased]\n\n## [%s] - 2026-10-09\n\n### Added\n\n- A client.\n\n## [0.0.1] - 2026-10-01\n\n### Added\n\n- The repository.\n' "$1"
}

# A clone of a bare origin holding one module with the given version.go and CHANGELOG.md, pushed,
# as the workflow's checkout holds the repository. Sets $clone.
repository_with() {
    directory="$sandbox/$scenario"
    clone="$directory/clone"
    git init -q --bare "$directory/origin"
    git clone -q "$directory/origin" "$clone" 2>/dev/null
    mkdir -p "$clone/$1"
    printf '%s\n' "$2" > "$clone/$1/version.go"
    printf '%s\n' "$3" > "$clone/$1/CHANGELOG.md"
    git -C "$clone" add .
    git -C "$clone" commit -q -m "Prepare the release"
    git -C "$clone" push -q origin HEAD
}

push_tag_to_origin() {
    git -C "$clone" tag "$1"
    git -C "$clone" push -q origin "$1"
}

# Runs the preflight from the clone, as the workflow does from its checkout. Sets $status,
# $outputs_file, $notes_file, $stderr_file.
run_preflight() {
    outputs_file="$clone/github-output"
    notes_file="$clone/release-notes.md"
    stderr_file="$clone/stderr"
    : > "$outputs_file"
    status=0
    (cd "$clone" && GITHUB_OUTPUT="$outputs_file" "$preflight" "$1" "$notes_file") \
        > "$clone/stdout" 2> "$stderr_file" || status=$?
}

fail() {
    printf 'FAIL %s: %s\n' "$scenario" "$1"
    failures=$((failures + 1))
}

assert_status() {
    [ "$status" -eq "$1" ] || fail "exit status was $status, expected $1 (stderr: $(cat "$stderr_file"))"
}

assert_outputs() {
    actual=$(cat "$outputs_file")
    [ "$actual" = "$1" ] || fail "outputs were '$actual', expected '$1'"
}

assert_notes() {
    actual=$(cat "$notes_file" 2>/dev/null || true)
    [ "$actual" = "$1" ] || fail "notes were '$actual', expected '$1'"
}

assert_refused_with() {
    assert_status 1
    grep -qF -- "$1" "$stderr_file" || fail "stderr does not mention '$1': $(cat "$stderr_file")"
    assert_outputs ""
    [ ! -e "$notes_file" ] || fail "notes were written for a refused release"
}

pass() {
    printf 'ok   %s\n' "$scenario"
}

scenario=releases_the_version_the_module_declares_with_its_changelog_section_as_notes
repository_with configdirector "$(version_go configdirector 0.1.0)" "$(released_changelog 0.1.0)"
run_preflight configdirector
assert_status 0
assert_outputs "version=0.1.0
tag=configdirector/v0.1.0
prerelease=false"
assert_notes "### Added

- A client."
pass

scenario=names_the_tag_after_the_module_directory
repository_with configdirector-openfeature "$(version_go configdirectoropenfeature 0.1.0)" "$(released_changelog 0.1.0)"
run_preflight configdirector-openfeature
assert_status 0
assert_outputs "version=0.1.0
tag=configdirector-openfeature/v0.1.0
prerelease=false"
pass

scenario=marks_a_version_with_a_prerelease_suffix_as_a_prerelease
repository_with configdirector "$(version_go configdirector 0.2.0-rc.1)" "$(released_changelog 0.2.0-rc.1)"
run_preflight configdirector
assert_status 0
assert_outputs "version=0.2.0-rc.1
tag=configdirector/v0.2.0-rc.1
prerelease=true"
pass

scenario=refuses_a_version_whose_tag_already_exists
repository_with configdirector "$(version_go configdirector 0.1.0)" "$(released_changelog 0.1.0)"
push_tag_to_origin configdirector/v0.1.0
run_preflight configdirector
assert_refused_with "configdirector/v0.1.0 already exists"
pass

scenario=refuses_a_version_without_a_changelog_section
repository_with configdirector "$(version_go configdirector 0.1.0)" "$(released_changelog 0.0.2)"
run_preflight configdirector
assert_refused_with "no '## [0.1.0]' section"
pass

scenario=refuses_a_version_whose_only_section_is_one_of_its_prereleases
repository_with configdirector "$(version_go configdirector 0.1.0)" "$(released_changelog 0.1.0-rc.1)"
run_preflight configdirector
assert_refused_with "no '## [0.1.0]' section"
pass

scenario=refuses_a_version_whose_changelog_section_is_empty
repository_with configdirector "$(version_go configdirector 0.1.0)" "$(printf '# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2026-10-09\n\n## [0.0.1] - 2026-10-01\n\n- The repository.\n')"
run_preflight configdirector
assert_refused_with "'## [0.1.0]' section in configdirector/CHANGELOG.md is empty"
pass

scenario=refuses_a_version_that_is_not_a_semantic_version
repository_with configdirector "$(version_go configdirector v0.1.0)" "$(released_changelog 0.1.0)"
run_preflight configdirector
assert_refused_with "Version 'v0.1.0' must be a semantic version with no v prefix"
pass

scenario=refuses_a_module_without_a_version_constant
repository_with configdirector "$(printf 'package configdirector\n')" "$(released_changelog 0.1.0)"
run_preflight configdirector
assert_refused_with "Could not read the Version constant from configdirector/version.go"
pass

if [ "$failures" -ne 0 ]; then
    printf '\n%s scenario(s) failed\n' "$failures"
    exit 1
fi
