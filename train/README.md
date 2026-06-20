# train/

Python side of distill: dataset assembly, encoder training with README dropout, and per-facet eval against teacher labels (SPEC §4–6).

```sh
cd train
uv sync
uv run pytest

uv run python train.py \
  --labels ../slice50-labels.jsonl \
  --features ../slice50-features.jsonl \
  --readme-dropout 0.3 --min-confidence medium --epochs 5 --out out

uv run python evaluate.py \
  --model-dir out \
  --labels ../slice50-labels.jsonl \
  --features ../slice50-features.jsonl
```

`train.py` reports `val_full_f1` and `val_code_f1` per facet each epoch; `evaluate.py` reports the SPEC §6 headline (code-only vs teacher, per facet) plus full-vs-code-only agreement. Numbers on 46 examples are for plumbing only.

terms.txt is read from `../cmd/distill/terms.txt` so the label vocabulary stays single-sourced.
