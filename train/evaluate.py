"""
Per-facet evaluation. SPEC §6: how closely code-only classification (README
dropped) matches the teacher's full-context labels.

Defaults to the held-out val split (the same split train.py used, reproduced
from config.json) so numbers aren't inflated by training data. Reports per-facet
micro precision/recall/F1 at both the fixed 0.5 threshold and per-facet tuned
thresholds, plus precision@k, plus full-vs-code agreement.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import numpy as np
import torch
from sklearn.metrics import f1_score, precision_score, recall_score
from torch.utils.data import DataLoader

from dataset import Vocabulary, load_pairs
from model import DistillClassifier, load_tokenizer

# DistillDataset and split are imported lazily inside functions: train.py imports
# per_facet_f1 from here, so a module-level import of train would be circular.


def per_facet_f1(gold: np.ndarray, pred: np.ndarray, vocab: Vocabulary) -> dict[str, float]:
    """Per-facet micro-F1 + overall micro, for compact per-epoch logging."""
    out: dict[str, float] = {}
    for facet, idxs in sorted(vocab.by_facet.items()):
        out[facet] = round(float(f1_score(gold[:, idxs], pred[:, idxs], average="micro", zero_division=0)), 4)
    out["_micro_all"] = round(float(f1_score(gold, pred, average="micro", zero_division=0)), 4)
    return out


def per_facet_report(gold: np.ndarray, pred: np.ndarray, vocab: Vocabulary) -> dict:
    rep: dict = {}
    for facet, idxs in sorted(vocab.by_facet.items()):
        g, p = gold[:, idxs], pred[:, idxs]
        rep[facet] = {
            "precision": round(float(precision_score(g, p, average="micro", zero_division=0)), 4),
            "recall": round(float(recall_score(g, p, average="micro", zero_division=0)), 4),
            "f1": round(float(f1_score(g, p, average="micro", zero_division=0)), 4),
            "support": int(g.sum()),
        }
    rep["_overall"] = {
        "precision": round(float(precision_score(gold, pred, average="micro", zero_division=0)), 4),
        "recall": round(float(recall_score(gold, pred, average="micro", zero_division=0)), 4),
        "f1": round(float(f1_score(gold, pred, average="micro", zero_division=0)), 4),
        "support": int(gold.sum()),
    }
    return rep


def tune_thresholds(gold: np.ndarray, probs: np.ndarray, vocab: Vocabulary) -> dict[str, float]:
    """Per-facet threshold (one per facet, not per term) that maximises micro-F1.

    Low DOF (6 thresholds) so fitting and reporting on the same val split is a
    defensible ceiling estimate rather than heavy overfitting.
    """
    grid = np.arange(0.05, 0.96, 0.05)
    thresholds: dict[str, float] = {}
    for facet, idxs in vocab.by_facet.items():
        g = gold[:, idxs]
        pr = probs[:, idxs]
        best_t, best_f1 = 0.5, -1.0
        for t in grid:
            f1 = f1_score(g, (pr >= t).astype(int), average="micro", zero_division=0)
            if f1 > best_f1:
                best_f1, best_t = f1, float(t)
        thresholds[facet] = round(best_t, 2)
    return thresholds


def apply_thresholds(probs: np.ndarray, vocab: Vocabulary, thresholds: dict[str, float]) -> np.ndarray:
    pred = np.zeros_like(probs, dtype=int)
    for facet, idxs in vocab.by_facet.items():
        t = thresholds.get(facet, 0.5)
        for j in idxs:
            pred[:, j] = (probs[:, j] >= t).astype(int)
    return pred


def precision_at_k(gold: np.ndarray, probs: np.ndarray, k: int) -> float:
    """Mean over examples of (relevant in top-k) / min(k, n_gold)."""
    hits, denom = 0.0, 0
    order = np.argsort(-probs, axis=1)[:, :k]
    for i in range(gold.shape[0]):
        n = int(gold[i].sum())
        if n == 0:
            continue
        hits += float(gold[i, order[i]].sum()) / min(k, n)
        denom += 1
    return round(hits / denom, 4) if denom else 0.0


def _predict(model, tok, examples, vocab, *, include_readme, max_length, device, batch):
    from train import DistillDataset

    ds = DistillDataset(
        examples,
        vocab=vocab,
        tokenizer=tok,
        max_length=max_length,
        readme_dropout=0.0 if include_readme else 1.0,
        min_confidence="low",
        max_identifiers=512,
        seed=0,
    )
    dl = DataLoader(ds, batch_size=batch)
    probs, golds = [], []
    model.eval()
    for b in dl:
        with torch.no_grad():
            logits = model(b["input_ids"].to(device), b["attention_mask"].to(device))
        probs.append(torch.sigmoid(logits).cpu().numpy())
        golds.append(b["labels"].cpu().numpy())
    return np.vstack(golds), np.vstack(probs)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-dir", required=True)
    ap.add_argument("--labels", required=True)
    ap.add_argument("--features", required=True)
    ap.add_argument("--encoder", default=None, help="override encoder (default from config.json)")
    ap.add_argument("--on", choices=("val", "train", "all"), default="val", help="which split to evaluate")
    ap.add_argument("--max-length", type=int, default=512)
    ap.add_argument("--batch-size", type=int, default=8)
    ap.add_argument("--device", default="cuda" if torch.cuda.is_available() else "cpu")
    args = ap.parse_args()

    mdir = Path(args.model_dir)
    cfg = json.loads((mdir / "config.json").read_text())
    encoder = args.encoder or cfg.get("encoder")
    vocab = Vocabulary.load(mdir / "terms.txt")
    tok = load_tokenizer(encoder)
    model = DistillClassifier(n_labels=len(vocab), encoder_name=encoder)
    model.encoder.resize_token_embeddings(len(tok))
    model.load_state_dict(torch.load(mdir / "model.pt", map_location="cpu"))
    model.to(args.device)

    from train import split

    examples = load_pairs(args.labels, args.features)
    # Reproduce train.py's split so we report on held-out data by default.
    train_ex, val_ex = split(examples, cfg.get("val_frac", 0.15), cfg.get("seed", 13))
    chosen = {"val": val_ex, "train": train_ex, "all": examples}[args.on]

    pred_kw = dict(max_length=args.max_length, device=args.device, batch=args.batch_size)
    g_code, prob_code = _predict(model, tok, chosen, vocab, include_readme=False, **pred_kw)
    g_full, prob_full = _predict(model, tok, chosen, vocab, include_readme=True, **pred_kw)

    p05 = (prob_code >= 0.5).astype(int)
    thresholds = tune_thresholds(g_code, prob_code, vocab)
    ptuned = apply_thresholds(prob_code, vocab, thresholds)
    full_tuned = apply_thresholds(prob_full, vocab, thresholds)

    out = {
        "split": args.on,
        "n": len(chosen),
        # SPEC §6 headline: code-only vs teacher labels.
        "code_only_at_0.5": per_facet_report(g_code, p05, vocab),
        "code_only_tuned": per_facet_report(g_code, ptuned, vocab),
        "tuned_thresholds": thresholds,
        "precision_at_k": {
            "p@3": precision_at_k(g_code, prob_code, 3),
            "p@5": precision_at_k(g_code, prob_code, 5),
            "p@10": precision_at_k(g_code, prob_code, 10),
        },
        # Premise check: code-only predictions should match full-context.
        "full_vs_code_agreement_0.5": round(float((p05 == (prob_full >= 0.5)).mean()), 4),
        "full_vs_code_agreement_tuned": round(float((ptuned == full_tuned).mean()), 4),
    }
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
