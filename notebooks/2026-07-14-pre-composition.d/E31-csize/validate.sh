#!/bin/bash
# Validate a csize solver against the vendored go/pointer oracle on all 126 corpora.
# Agreement = the SET of dynamically-resolved callees is identical.
#
# This is the check E29 bug #2 demands: an under-approximating solver CONVERGES FAST
# and is WRONG. Convergence without oracle agreement is worthless.
SOLVER="$1"
S="$(cd "$(dirname "$0")" && pwd)"
agree=0; differ=0; failed=0
for c in "$S"/corpora/*/; do
  name=$(basename "$c")
  orc=$("$S"/genprobe "$c" 2>/dev/null | grep 'INVOKE' | sed 's/.*-> //' | sort -u)
  sol=$(CALLEES=1 BUDGET=short "$SOLVER" "$c" 2>/dev/null | grep '^CALLEE ' | sed 's/^CALLEE //' | sort -u)
  if [ -z "$orc" ] && [ -z "$sol" ]; then
    failed=$((failed+1)); echo "EMPTY  $name"; continue
  fi
  if [ "$orc" == "$sol" ]; then
    agree=$((agree+1))
  else
    differ=$((differ+1))
    echo "DIFFER $name"
    diff <(echo "$orc") <(echo "$sol") | sed 's/^/         /' | head -6
  fi
done
echo "-----------------------------------------"
echo "solver:  $SOLVER"
echo "agree:   $agree"
echo "differ:  $differ"
echo "empty:   $failed"
