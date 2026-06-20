# distill

Trains a small multi-label classifier that assigns [oss-taxonomy](https://github.com/ecosyste-ms/oss-taxonomy) terms to a repository from code-derived signals (brief detection output, outline identifiers, structural facts), with the README as an optional and explicitly-distrusted input. An LLM teacher labels a corpus at curation time; the trained student runs offline inside [brief](https://github.com/git-pkgs/brief) at scan time and generalises to repos the teacher never saw.

Needs `brief` and `claude` on PATH.

## classify (teacher)

```
distill classify pkg:pypi/torch pkg:gem/rails > labels.jsonl
```

Shells to `claude -p` so it uses your existing Claude Code login. Each line is `{input, purl, repo, model, tags: [{facet, term, evidence, evidence_kind, confidence}], unclassified, cost_usd}`. Flags: `-model`, `-brief`, `-claude`, `-outline-cap`, `-readme-cap`, `-keep`.

## extract (student input)

```
distill extract pkg:pypi/torch pkg:gem/rails > features.jsonl
```

Deterministic feature record per repo: `{stack_tags, structure, identifiers, readme}`. No model call. Flags: `-brief`, `-max-files`, `-max-identifiers`, `-keep`.

## corpus

```
distill corpus -from corpus/seed.txt -labels labels.jsonl -features features.jsonl
```

Clones each target once and writes both the label and feature records. Appends to the output files and skips inputs that already have a successful label, so an interrupted run restarts where it left off. Flags are the union of `classify` and `extract` flags plus `-from`.
