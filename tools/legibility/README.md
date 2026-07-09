# find_duplicates legibility probe

A manual smoke that checks whether the `find_duplicates` MCP tool's JSON output
is self-describing enough for an agent to act on. It runs the real tool, hands
the exact output to `claude -p` WITHOUT telling it what the equivalence tiers
mean, and checks the model buckets each pair as verified / review / distinct
consistently with the tool's `equiv_tier` (`proven`/`structural`/`divergent`).

It is NOT part of `make ci`: the file is `//go:build ignore`, and the model call
is non-deterministic and network-bound. Run it by hand.

## Run

    go run tools/legibility/probe.go --fixture dupcluster
    go run tools/legibility/probe.go --fixture nodups --model claude-haiku-4-5-20251001

## Flags

- `--fixture`  dupcluster (default) | nodups
- `--model`    model for `claude -p` (default: CLI default). Judge legibility on
               a mid-tier model: if only the strongest reads it right, it is not
               legible enough.
- `--runs N`   run the model call N times, report a majority verdict (default 1)
- `--answer F` score a saved model answer from file F instead of calling claude
               (deterministic, offline; used by the probe's own verification)
- `--dump-json`, `--dump-expected`, `--print-prompt` — inspection aids

## What PASS means

Every `proven` pair bucketed `verified` (headline), AND >= 80% of all reported
pairs match the tier-implied bucket. The valuable output is the per-pair diff on
a FAIL: it names exactly which pair an agent mis-read, pointing at the field or
name in the output contract to sharpen. Requires the `claude` CLI installed and
authed (or use `--answer`).

A PASS is weak evidence, not strong: the tool description forwarded into the
prompt (kept in, by design, for fidelity to what an agent actually receives)
already names the tiers `proven`/`structural`/`divergent`, and those words'
plain-English connotations do much of the bucketing on their own. So a PASS
partly reflects legible tier *names*, not proof that the structured measures
behind them are legible. The FAIL diff remains the discriminating evidence,
since it names exactly which pair the model mis-read.
