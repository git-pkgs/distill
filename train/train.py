"""
Train the multi-label classifier on (features, labels) pairs.

SPEC §5: README dropout — on a fraction of training examples the README segment
is blanked so the model learns to classify from stack_tags + structure +
identifiers alone. This is what makes code-only inference (and the
full-vs-code-only divergence signal) possible from one model.
"""

from __future__ import annotations

import argparse
import json
import random
from pathlib import Path

import numpy as np
import torch
from torch import nn
from torch.utils.data import DataLoader, Dataset

from dataset import Example, Vocabulary, load_pairs
from evaluate import per_facet_f1
from model import DEFAULT_ENCODER, DistillClassifier, load_tokenizer


class DistillDataset(Dataset):
    def __init__(
        self,
        examples: list[Example],
        vocab: Vocabulary,
        tokenizer,
        *,
        max_length: int,
        readme_dropout: float,
        min_confidence: str,
        max_identifiers: int,
        seed: int,
    ):
        self.examples = examples
        self.vocab = vocab
        self.tok = tokenizer
        self.max_length = max_length
        self.readme_dropout = readme_dropout
        self.min_confidence = min_confidence
        self.max_identifiers = max_identifiers
        self.rng = random.Random(seed)

    def __len__(self):
        return len(self.examples)

    def __getitem__(self, i):
        ex = self.examples[i]
        include_readme = self.rng.random() >= self.readme_dropout
        text = ex.serialise(include_readme=include_readme, max_identifiers=self.max_identifiers)
        enc = self.tok(
            text,
            truncation=True,
            max_length=self.max_length,
            padding="max_length",
            return_tensors="pt",
        )
        return {
            "input_ids": enc["input_ids"].squeeze(0),
            "attention_mask": enc["attention_mask"].squeeze(0),
            "labels": torch.tensor(ex.label_vector(self.vocab, self.min_confidence)),
        }


def split(examples: list[Example], val_frac: float, seed: int):
    idx = list(range(len(examples)))
    random.Random(seed).shuffle(idx)
    n_val = max(1, int(len(idx) * val_frac))
    val_idx = set(idx[:n_val])
    train = [examples[i] for i in idx if i not in val_idx]
    val = [examples[i] for i in idx if i in val_idx]
    return train, val


def run_epoch(model, loader, loss_fn, optim, device, train: bool):
    model.train(train)
    total, n = 0.0, 0
    preds, golds = [], []
    for batch in loader:
        ids = batch["input_ids"].to(device)
        mask = batch["attention_mask"].to(device)
        y = batch["labels"].to(device)
        with torch.set_grad_enabled(train):
            logits = model(ids, mask)
            loss = loss_fn(logits, y)
            if train:
                optim.zero_grad()
                loss.backward()
                optim.step()
        total += loss.item() * y.size(0)
        n += y.size(0)
        preds.append((torch.sigmoid(logits) > 0.5).cpu().numpy())
        golds.append(y.cpu().numpy())
    return total / max(n, 1), np.vstack(preds), np.vstack(golds)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--labels", required=True)
    ap.add_argument("--features", required=True)
    ap.add_argument("--terms", default=None)
    ap.add_argument("--encoder", default=DEFAULT_ENCODER)
    ap.add_argument("--out", default="train/out")
    ap.add_argument("--epochs", type=int, default=5)
    ap.add_argument("--batch-size", type=int, default=8)
    ap.add_argument("--lr", type=float, default=2e-5)
    ap.add_argument("--max-length", type=int, default=512)
    ap.add_argument("--max-identifiers", type=int, default=512)
    ap.add_argument("--readme-dropout", type=float, default=0.3)
    ap.add_argument("--min-confidence", choices=("low", "medium", "high"), default="medium")
    ap.add_argument("--val-frac", type=float, default=0.15)
    ap.add_argument("--seed", type=int, default=13)
    ap.add_argument("--device", default="cuda" if torch.cuda.is_available() else "cpu")
    args = ap.parse_args()

    torch.manual_seed(args.seed)
    vocab = Vocabulary.load(args.terms) if args.terms else Vocabulary.load()
    examples = load_pairs(args.labels, args.features)
    if not examples:
        raise SystemExit("no joined examples (check labels/features inputs)")
    train_ex, val_ex = split(examples, args.val_frac, args.seed)
    print(f"{len(examples)} examples -> train {len(train_ex)} / val {len(val_ex)}; {len(vocab)} labels")

    tok = load_tokenizer(args.encoder)
    model = DistillClassifier(n_labels=len(vocab), encoder_name=args.encoder)
    model.encoder.resize_token_embeddings(len(tok))
    model.to(args.device)

    ds_kwargs = dict(
        vocab=vocab,
        tokenizer=tok,
        max_length=args.max_length,
        min_confidence=args.min_confidence,
        max_identifiers=args.max_identifiers,
    )
    train_ds = DistillDataset(train_ex, readme_dropout=args.readme_dropout, seed=args.seed, **ds_kwargs)
    # Validation runs twice: full-context and code-only (readme_dropout=1.0).
    val_full = DistillDataset(val_ex, readme_dropout=0.0, seed=args.seed, **ds_kwargs)
    val_code = DistillDataset(val_ex, readme_dropout=1.0, seed=args.seed, **ds_kwargs)

    train_dl = DataLoader(train_ds, batch_size=args.batch_size, shuffle=True)
    val_full_dl = DataLoader(val_full, batch_size=args.batch_size)
    val_code_dl = DataLoader(val_code, batch_size=args.batch_size)

    loss_fn = nn.BCEWithLogitsLoss()
    optim = torch.optim.AdamW(model.parameters(), lr=args.lr)

    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    history = []
    for epoch in range(1, args.epochs + 1):
        tr_loss, *_ = run_epoch(model, train_dl, loss_fn, optim, args.device, train=True)
        vf_loss, vf_pred, vf_gold = run_epoch(model, val_full_dl, loss_fn, optim, args.device, train=False)
        vc_loss, vc_pred, vc_gold = run_epoch(model, val_code_dl, loss_fn, optim, args.device, train=False)
        row = {
            "epoch": epoch,
            "train_loss": round(tr_loss, 4),
            "val_full_loss": round(vf_loss, 4),
            "val_code_loss": round(vc_loss, 4),
            "val_full_f1": per_facet_f1(vf_gold, vf_pred, vocab),
            "val_code_f1": per_facet_f1(vc_gold, vc_pred, vocab),
        }
        history.append(row)
        print(json.dumps(row))

    # Save CPU tensors so the checkpoint is portable and avoids the MPS
    # unaligned-blit assert on load (torch.load(map_location="mps") is fragile).
    torch.save({k: v.cpu() for k, v in model.state_dict().items()}, out / "model.pt")
    tok.save_pretrained(out)
    (out / "terms.txt").write_text("\n".join(vocab.terms) + "\n")
    (out / "history.json").write_text(json.dumps(history, indent=2))
    (out / "config.json").write_text(json.dumps(vars(args), indent=2))
    print(f"saved to {out}")


if __name__ == "__main__":
    main()
