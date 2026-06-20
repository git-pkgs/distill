"""
Data loading, label vocabulary, and feature serialisation for the distill
classifier. Pure Python so tests run without torch/transformers installed;
train.py wraps these in a torch Dataset.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path

CONFIDENCE_RANK = {"low": 0, "medium": 1, "high": 2}
DEFAULT_TERMS = Path(__file__).resolve().parents[1] / "cmd" / "distill" / "terms.txt"

STACK_TOK = "[STACK]"
STRUCT_TOK = "[STRUCT]"
IDENT_TOK = "[IDENT]"
README_TOK = "[README]"


@dataclass
class Vocabulary:
    """oss-taxonomy facet:term <-> index, with per-facet slices for eval."""

    terms: list[str]
    index: dict[str, int] = field(init=False)
    by_facet: dict[str, list[int]] = field(init=False)

    def __post_init__(self) -> None:
        self.index = {t: i for i, t in enumerate(self.terms)}
        self.by_facet = {}
        for i, t in enumerate(self.terms):
            facet = t.split(":", 1)[0]
            self.by_facet.setdefault(facet, []).append(i)

    @classmethod
    def load(cls, path: str | Path = DEFAULT_TERMS) -> "Vocabulary":
        terms = [
            ln.strip()
            for ln in Path(path).read_text().splitlines()
            if ln.strip() and not ln.startswith("#")
        ]
        return cls(terms)

    def __len__(self) -> int:
        return len(self.terms)


@dataclass
class Example:
    input: str
    features: dict
    tags: list[dict]
    unclassified: list[str]

    def label_vector(self, vocab: Vocabulary, min_confidence: str = "low") -> list[float]:
        """SPEC §4: drop labels below the confidence threshold."""
        thresh = CONFIDENCE_RANK[min_confidence]
        vec = [0.0] * len(vocab)
        for t in self.tags:
            if CONFIDENCE_RANK.get(t.get("confidence", "low"), 0) < thresh:
                continue
            key = f"{t['facet']}:{t['term']}"
            idx = vocab.index.get(key)
            if idx is not None:
                vec[idx] = 1.0
        return vec

    def serialise(self, *, include_readme: bool, max_identifiers: int = 512) -> str:
        """Render the feature record as a single string for the encoder.

        README goes last so dropout (include_readme=False) is a clean truncation
        and the two-mode (full vs code-only) inference is the same model with a
        different input, per SPEC §5.
        """
        f = self.features
        parts: list[str] = [STACK_TOK, *f.get("stack_tags", [])]

        s = f.get("structure", {}) or {}
        struct: list[str] = []
        for lang, frac in sorted((s.get("languages") or {}).items(), key=lambda kv: -kv[1]):
            struct.append(f"lang:{lang.lower()}={frac:.2f}")
        for flag in (
            "has_entrypoint",
            "has_tests",
            "has_dockerfile",
            "has_migrations",
            "has_proto",
            "has_infra",
        ):
            struct.append(flag if s.get(flag) else f"no_{flag[4:]}")
        struct.append(f"files:{_bucket(s.get('file_count', 0))}")
        struct.append(f"deps:{_bucket(s.get('direct_dep_count', 0))}")
        parts += [STRUCT_TOK, *struct]

        idents = (f.get("identifiers") or [])[:max_identifiers]
        parts += [IDENT_TOK, *idents]

        parts.append(README_TOK)
        if include_readme:
            parts.append(f.get("readme", "") or "")
        return " ".join(p for p in parts if p)


def _bucket(n: int) -> str:
    """Coarse buckets so file/dep counts become a small token vocabulary."""
    for upper in (0, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000):
        if n <= upper:
            return str(upper)
    return "5000+"


def load_pairs(labels_path: str | Path, features_path: str | Path) -> list[Example]:
    """Join labels.jsonl and features.jsonl on .input, dropping errored rows."""
    feats: dict[str, dict] = {}
    for rec in _read_jsonl(features_path):
        if rec.get("error"):
            continue
        feats[rec["input"]] = rec

    out: list[Example] = []
    for rec in _read_jsonl(labels_path):
        if rec.get("error"):
            continue
        inp = rec["input"]
        f = feats.get(inp)
        if f is None:
            continue
        out.append(
            Example(
                input=inp,
                features=f,
                tags=rec.get("tags") or [],
                unclassified=rec.get("unclassified") or [],
            )
        )
    return out


def _read_jsonl(path: str | Path):
    with open(path) as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            yield json.loads(line)
