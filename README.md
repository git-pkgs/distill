# distill

Trains a small multi-label classifier that assigns [oss-taxonomy](https://github.com/ecosyste-ms/oss-taxonomy) terms to a repository from code-derived signals (brief detection output, outline identifiers, structural facts), with the README as an optional and explicitly-distrusted input. An LLM teacher labels a corpus at curation time; the trained student runs offline inside [brief](https://github.com/git-pkgs/brief) at scan time and generalises to repos the teacher never saw.

## classify (teacher)

```
ANTHROPIC_API_KEY=... distill classify pkg:pypi/torch pkg:gem/rails > labels.jsonl
```

Needs `brief` on PATH. Each line is `{input, purl, repo, model, tags: [{facet, term, evidence, evidence_kind, confidence}], unclassified}`. Flags: `-model`, `-brief`, `-outline-cap`, `-readme-cap`, `-keep`.
