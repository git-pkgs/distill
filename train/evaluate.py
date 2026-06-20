"""
Per-facet evaluation. SPEC §6: the headline number is how closely code-only
classification (README dropped) matches the teacher's full-context labels,
reported per facet so structural facets (role/function/layer) and domain can be
read separately.
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


def per_facet_f1(gold: np.ndarray, pred: np.ndarray, vocab: Vocabulary) -> dict[str, float]:
    out: dict[str, float] = {}
    for facet, idxs in sorted(vocab.by_facet.items()):
        g = gold[:, idxs]
        p = pred[:, idxs]
        # macro-F1 over the facet's terms; zero_division=0 so unseen terms
        # don't warn on a tiny val set.
        out[facet] = round(float(f1_score(g, p, average="macro", zero_division=0)), 4)
    out["_micro_all"] = round(float(f1_score(gold, pred, average="micro", zero_division=0)), 4)
    return out


def per_facet_report(gold: np.ndarray, pred: np.ndarray, vocab: Vocabulary) -> dict:
    rep: dict = {}
    for facet, idxs in sorted(vocab.by_facet.items()):
        g, p = gold[:, idxs], pred[:, idxs]
        rep[facet] = {
            "precision": round(float(precision_score(g, p, average="micro", zero_division=0)), 4),
            "recall": round(float(recall_score(g, p, average="micro", zero_division=0)), 4),
            "f1_macro": round(float(f1_score(g, p, average="macro", zero_division=0)), 4),
            "support": int(g.sum()),
        }
    return rep


def _predict(model, tok, examples, vocab, *, include_readme: bool, max_length: int, device: str, batch: int):
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
    preds, golds = [], []
    model.eval()
    for b in dl:
        with torch.no_grad():
            logits = model(b["input_ids"].to(device), b["attention_mask"].to(device))
        preds.append((torch.sigmoid(logits) > 0.5).cpu().numpy())
        golds.append(b["labels"].cpu().numpy())
    return np.vstack(golds), np.vstack(preds)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-dir", required=True)
    ap.add_argument("--labels", required=True)
    ap.add_argument("--features", required=True)
    ap.add_argument("--encoder", default=None, help="override encoder name (defaults to config.json)")
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
    # Load to CPU first; map_location straight to MPS hits an unaligned-blit assert.
    model.load_state_dict(torch.load(mdir / "model.pt", map_location="cpu"))
    model.to(args.device)

    examples = load_pairs(args.labels, args.features)
    print(f"{len(examples)} examples")

    g_full, p_full = _predict(model, tok, examples, vocab, include_readme=True, max_length=args.max_length, device=args.device, batch=args.batch_size)
    g_code, p_code = _predict(model, tok, examples, vocab, include_readme=False, max_length=args.max_length, device=args.device, batch=args.batch_size)

    out = {
        "n": len(examples),
        "full_context": per_facet_report(g_full, p_full, vocab),
        "code_only": per_facet_report(g_code, p_code, vocab),
        # SPEC §6 headline: code-only prediction vs teacher labels, per facet.
        "code_only_vs_teacher_f1": per_facet_f1(g_code, p_code, vocab),
        # Agreement between the two modes (the divergence signal's complement).
        "full_vs_code_agreement": round(float((p_full == p_code).mean()), 4),
    }
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
