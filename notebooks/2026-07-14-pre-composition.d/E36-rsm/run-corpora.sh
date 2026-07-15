#!/bin/bash
# 126-corpus EQUIV+DISPATCH+PROBE gate. Usage: ./run-corpora.sh ./rsm
# PROBE is the isolated single-seed correctness gate: a corpus passes only if
# every receiver queried ALONE reproduces batch pts exactly ("PROBE: 0 /").
set -u
SOLVER="${1:-./rsm}"
CORPORA="$HOME/ClaudeProjects/LLMAccuracy/code-restructuring/packages_iface_decoy"
ep=0; ef=0; dp=0; df=0; pp=0; pf=0
for c in "$CORPORA"/ifdecoy-*/; do
  out=$(GOWORK=off "$SOLVER" "$c" 2>/dev/null)
  if echo "$out" | grep -q "EQUIV: PASS"; then ep=$((ep+1)); else ef=$((ef+1)); echo "EQUIV FAIL  $(basename "$c")"; fi
  if echo "$out" | grep -q "DISPATCH: PASS"; then dp=$((dp+1)); else df=$((df+1)); echo "DISPATCH FAIL  $(basename "$c")"; fi
  if echo "$out" | grep -q "PROBE: 0 /"; then pp=$((pp+1)); else pf=$((pf+1)); echo "PROBE FAIL  $(basename "$c")"; fi
done
echo "-----------------------------------------"
echo "EQUIV    $ep pass / $ef fail"
echo "DISPATCH $dp pass / $df fail"
echo "PROBE    $pp pass / $pf fail"
