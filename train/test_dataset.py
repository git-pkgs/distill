import json

from dataset import (
    IDENT_TOK,
    README_TOK,
    STACK_TOK,
    STRUCT_TOK,
    Example,
    Vocabulary,
    _bucket,
    load_pairs,
)


def _ex(**over):
    base = {
        "input": "pkg:x/y",
        "features": {
            "stack_tags": ["role:library", "function:testing"],
            "structure": {
                "languages": {"Go": 0.9, "C": 0.1},
                "has_entrypoint": False,
                "has_tests": True,
                "has_dockerfile": False,
                "has_migrations": False,
                "has_proto": True,
                "has_infra": False,
                "file_count": 73,
                "direct_dep_count": 4,
            },
            "identifiers": ["command", "flag", "args"],
            "readme": "Cobra is a library",
        },
        "tags": [
            {"facet": "role", "term": "library", "confidence": "high"},
            {"facet": "function", "term": "parsing", "confidence": "medium"},
            {"facet": "domain", "term": "machine-learning", "confidence": "low"},
        ],
        "unclassified": [],
    }
    base.update(over)
    return Example(**base)


def test_vocabulary_loads_default():
    v = Vocabulary.load()
    assert len(v) > 100
    assert "role:library" in v.index
    assert v.by_facet["role"]
    # Every facet present.
    for facet in ("role", "function", "layer", "domain", "audience", "technology"):
        assert facet in v.by_facet


def test_label_vector_threshold():
    v = Vocabulary.load()
    ex = _ex()
    low = ex.label_vector(v, "low")
    med = ex.label_vector(v, "medium")
    hi = ex.label_vector(v, "high")
    assert sum(low) == 3
    assert sum(med) == 2
    assert sum(hi) == 1
    assert low[v.index["role:library"]] == 1.0
    assert med[v.index["domain:machine-learning"]] == 0.0


def test_label_vector_unknown_term_ignored():
    v = Vocabulary.load()
    ex = _ex(tags=[{"facet": "role", "term": "not-real", "confidence": "high"}])
    assert sum(ex.label_vector(v, "low")) == 0


def test_serialise_full_and_code_only():
    ex = _ex()
    full = ex.serialise(include_readme=True)
    code = ex.serialise(include_readme=False)
    for tok in (STACK_TOK, STRUCT_TOK, IDENT_TOK, README_TOK):
        assert tok in full and tok in code
    assert "Cobra is a library" in full
    assert "Cobra is a library" not in code
    # Code-only is a strict prefix of full up to the readme content.
    assert full.startswith(code)
    # Structure flags rendered as expected.
    assert "has_tests" in full and "no_entrypoint" in full and "has_proto" in full
    assert "lang:go=0.90" in full
    assert "files:100" in full and "deps:5" in full
    # Stack tags and identifiers present.
    assert "role:library" in full and "command" in full


def test_serialise_max_identifiers():
    ex = _ex()
    ex.features["identifiers"] = [f"id{i}" for i in range(1000)]
    s = ex.serialise(include_readme=False, max_identifiers=10)
    assert "id9" in s and "id10" not in s


def test_bucket():
    assert _bucket(0) == "0"
    assert _bucket(3) == "5"
    assert _bucket(73) == "100"
    assert _bucket(9000) == "5000+"


def test_load_pairs(tmp_path):
    labels = tmp_path / "l.jsonl"
    feats = tmp_path / "f.jsonl"
    labels.write_text(
        "\n".join(
            json.dumps(r)
            for r in [
                {"input": "a", "tags": [{"facet": "role", "term": "library", "confidence": "high"}]},
                {"input": "b", "error": "boom"},
                {"input": "c", "tags": []},  # no matching feature
            ]
        )
    )
    feats.write_text(
        "\n".join(
            json.dumps(r)
            for r in [
                {"input": "a", "stack_tags": [], "structure": {}, "identifiers": [], "readme": ""},
                {"input": "b", "stack_tags": []},
                {"input": "d", "stack_tags": []},
            ]
        )
    )
    pairs = load_pairs(labels, feats)
    assert [p.input for p in pairs] == ["a"]
    assert pairs[0].tags[0]["term"] == "library"
