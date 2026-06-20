"""
Encoder + flat multi-label head over the oss-taxonomy term set (SPEC §5).
Per-facet heads are the alternative to A/B against this; keep this simple
until the first eval exists.
"""

from __future__ import annotations

import torch
from torch import nn
from transformers import AutoConfig, AutoModel, AutoTokenizer

DEFAULT_ENCODER = "distilroberta-base"


class DistillClassifier(nn.Module):
    def __init__(self, n_labels: int, encoder_name: str = DEFAULT_ENCODER, dropout: float = 0.1):
        super().__init__()
        self.config = AutoConfig.from_pretrained(encoder_name)
        self.encoder = AutoModel.from_pretrained(encoder_name)
        self.dropout = nn.Dropout(dropout)
        self.head = nn.Linear(self.config.hidden_size, n_labels)

    def forward(self, input_ids, attention_mask):
        out = self.encoder(input_ids=input_ids, attention_mask=attention_mask)
        # Mean-pool over the sequence rather than CLS; identifier bags have no
        # natural sentence structure so positional CLS isn't obviously better.
        mask = attention_mask.unsqueeze(-1).float()
        pooled = (out.last_hidden_state * mask).sum(1) / mask.sum(1).clamp(min=1)
        return self.head(self.dropout(pooled))


def load_tokenizer(encoder_name: str = DEFAULT_ENCODER):
    tok = AutoTokenizer.from_pretrained(encoder_name)
    # Section markers as additional special tokens so they survive tokenisation
    # intact and the model can learn segment boundaries.
    from dataset import IDENT_TOK, README_TOK, STACK_TOK, STRUCT_TOK

    tok.add_special_tokens(
        {"additional_special_tokens": [STACK_TOK, STRUCT_TOK, IDENT_TOK, README_TOK]}
    )
    return tok
