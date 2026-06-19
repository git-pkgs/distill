# distill

Uses an LLM to classify packages and repositories into [oss-taxonomy](https://github.com/ecosyste-ms/oss-taxonomy) terms, then compiles those classifications into deterministic rules under `data/` that [brief](https://github.com/git-pkgs/brief) can apply without a model.

The LLM runs at curation time. brief embeds `data/` and runs the rules at scan time.

## classify

```
ANTHROPIC_API_KEY=... distill classify pkg:pypi/torch pkg:gem/rails > out.jsonl
```

Needs `brief` on PATH. Each line of output is a JSON object with `tags` (facet, term, evidence, evidence_kind, confidence) and `unclassified` (things the model wanted to tag but no term fit, plus any terms it invented that were rejected).

Flags: `-model`, `-brief`, `-outline-cap`, `-readme-cap`, `-keep`.
