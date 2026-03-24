#!/usr/bin/env bash
# Enforce per-package statement coverage threshold.
# Usage: covercheck.sh <threshold> <coverage.out>
# Exit 0 if all included packages meet threshold, 1 otherwise.

set -euo pipefail

if [ $# -ne 2 ]; then
	echo "Usage: $0 <threshold> <coverage.out>" >&2
	exit 2
fi

THRESHOLD="$1"
COVERAGE_FILE="$2"

if [ ! -f "$COVERAGE_FILE" ]; then
	echo "Error: coverage file not found: $COVERAGE_FILE" >&2
	exit 2
fi

MODULE="github.com/aalpar/wile-goast"

# Packages excluded from coverage enforcement.
EXCLUDED_PKGS=(
	"cmd/wile-goast"
	"testutil"
)

is_excluded() {
	local pkg="$1"
	for excl in "${EXCLUDED_PKGS[@]}"; do
		if [ "$pkg" = "$excl" ]; then
			return 0
		fi
	done
	return 1
}

declare -A PKG_TOTAL
declare -A PKG_COVERED

while IFS= read -r line; do
	if [[ "$line" == mode:* ]]; then
		continue
	fi

	file="${line%%:*}"
	pkg_path="${file%/*}"
	short_pkg="${pkg_path#"$MODULE"/}"
	if [ "$short_pkg" = "$MODULE" ]; then
		short_pkg="wile-goast"
	fi

	rest="${line##* }"
	middle="${line% *}"
	stmts="${middle##* }"
	count="$rest"

	PKG_TOTAL["$short_pkg"]=$(( ${PKG_TOTAL["$short_pkg"]:-0} + stmts ))
	if [ "$count" -gt 0 ]; then
		PKG_COVERED["$short_pkg"]=$(( ${PKG_COVERED["$short_pkg"]:-0} + stmts ))
	fi
done < "$COVERAGE_FILE"

declare -a FAILED=()
declare -a PASSED=()
declare -a SKIPPED=()

for pkg in $(echo "${!PKG_TOTAL[@]}" | tr ' ' '\n' | sort); do
	total="${PKG_TOTAL[$pkg]}"
	covered="${PKG_COVERED[$pkg]:-0}"

	if [ "$total" -eq 0 ]; then
		pct="0.0"
	else
		pct=$(awk "BEGIN { printf \"%.1f\", ($covered / $total) * 100 }")
	fi

	if is_excluded "$pkg"; then
		SKIPPED+=("  skip  $pkg: ${pct}%")
		continue
	fi

	if awk "BEGIN { exit ($pct >= $THRESHOLD) ? 1 : 0 }"; then
		FAILED+=("  FAIL  $pkg: ${pct}% < ${THRESHOLD}%")
	else
		PASSED+=("  pass  $pkg: ${pct}%")
	fi
done

echo "Coverage threshold: ${THRESHOLD}%"
echo ""

if [ ${#PASSED[@]} -gt 0 ]; then
	echo "Passing (${#PASSED[@]}):"
	printf '%s\n' "${PASSED[@]}"
	echo ""
fi

if [ ${#FAILED[@]} -gt 0 ]; then
	echo "Failing (${#FAILED[@]}):"
	printf '%s\n' "${FAILED[@]}"
	echo ""
fi

if [ ${#SKIPPED[@]} -gt 0 ]; then
	echo "Excluded (${#SKIPPED[@]}):"
	printf '%s\n' "${SKIPPED[@]}"
	echo ""
fi

if [ ${#FAILED[@]} -gt 0 ]; then
	echo "FAIL: ${#FAILED[@]} package(s) below ${THRESHOLD}% coverage"
	exit 1
fi

echo "OK: all ${#PASSED[@]} package(s) meet ${THRESHOLD}% coverage"
exit 0
