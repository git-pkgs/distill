package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAllowedTerms(t *testing.T) {
	terms := allowedTerms()
	if len(terms) < 100 {
		t.Fatalf("expected >100 terms, got %d", len(terms))
	}
	for _, want := range []string{"role:library", "domain:machine-learning", "technology:go", "function:parsing"} {
		if !terms[want] {
			t.Errorf("missing expected term %q", want)
		}
	}
	if terms["role:made-up"] {
		t.Error("unexpected term role:made-up")
	}
}

func TestValidateTermsDropsUnknown(t *testing.T) {
	mk := func(term string) Tag {
		return Tag{Facet: "role", Term: term, Evidence: "x", EvidenceKind: "file", Confidence: "high"}
	}
	c := &Classification{Tags: []Tag{mk("library"), mk("not-a-real-term")}}
	validateTerms(c)
	if len(c.Tags) != 1 || c.Tags[0].Term != "library" {
		t.Fatalf("expected only library to survive, got %+v", c.Tags)
	}
	if len(c.Unclassified) != 1 || !strings.Contains(c.Unclassified[0], "not-a-real-term") {
		t.Fatalf("expected rejection recorded, got %+v", c.Unclassified)
	}
}

func TestClassifySchemaIsValidJSON(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(classifySchema), &v); err != nil {
		t.Fatalf("schema.json is not valid JSON: %v", err)
	}
	if _, ok := v["properties"].(map[string]any)["tags"]; !ok {
		t.Fatal("schema missing tags property")
	}
}

func TestParseClaudeOutput(t *testing.T) {
	inner := `{"tags":[{"facet":"role","term":"library","evidence":"x","evidence_kind":"file","confidence":"high"}],"unclassified":[]}`
	raw := []byte(`{"type":"result","subtype":"success","is_error":false,"result":"prose ignored","structured_output":` + inner + `,"total_cost_usd":0.0123}`)
	cls, cost, err := parseClaudeOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cls.Tags) != 1 || cls.Tags[0].Term != "library" {
		t.Fatalf("unexpected tags: %+v", cls.Tags)
	}
	if cost != 0.0123 {
		t.Fatalf("cost not captured: %v", cost)
	}
	if _, _, err := parseClaudeOutput([]byte(`{"type":"result","subtype":"error","is_error":true,"result":"boom"}`)); err == nil {
		t.Fatal("expected error on is_error envelope")
	}
	if _, _, err := parseClaudeOutput([]byte(`{"type":"result","subtype":"success","is_error":false,"result":"only prose"}`)); err == nil {
		t.Fatal("expected error when structured_output missing")
	}
}

func TestBuildPromptIncludesInputs(t *testing.T) {
	p := buildPrompt("pkg:npm/x", "pkg:npm/x", `{"languages":[]}`, "## Structure", "# X readme")
	for _, want := range []string{"pkg:npm/x", "languages", "## Structure", "X readme", "role:library", "aka: orm"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestVocabAndTermsAligned(t *testing.T) {
	// Every line in vocab.txt must start with a term that appears in terms.txt,
	// otherwise validateTerms would reject things the prompt offered.
	allowed := allowedTerms()
	for _, line := range strings.Split(strings.TrimSpace(vocabTxt), "\n") {
		key := line
		if i := strings.IndexAny(line, " ("); i > 0 {
			key = line[:i]
		}
		if !allowed[key] {
			t.Errorf("vocab.txt line %q has no matching terms.txt entry", key)
		}
	}
	if len(allowed) != strings.Count(strings.TrimSpace(vocabTxt), "\n")+1 {
		t.Errorf("terms.txt (%d) and vocab.txt line counts differ", len(allowed))
	}
}

func TestCapBytes(t *testing.T) {
	if got := capBytes("hello", 0); got != "hello" {
		t.Errorf("zero cap should be no-op, got %q", got)
	}
	if got := capBytes("hello", 10); got != "hello" {
		t.Errorf("under cap should be no-op, got %q", got)
	}
	got := capBytes("0123456789", 4)
	if !strings.HasPrefix(got, "0123") || !strings.Contains(got, "truncated") {
		t.Errorf("over cap should truncate+mark, got %q", got)
	}
}
