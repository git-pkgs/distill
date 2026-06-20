package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrorClass(t *testing.T) {
	cases := map[string]string{
		"model: claude -p: exit status 1":  "model (claude -p)",
		"brief outline: ... exit status 2": "brief outline",
		"brief json: ...":                  "brief json",
		"could not resolve target":         "unresolved repo",
		"something weird":                  "other",
	}
	for in, want := range cases {
		if got := errorClass(in); got != want {
			t.Errorf("errorClass(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGapKeywords(t *testing.T) {
	got := gapKeywords("no term for ORM / object mapping; also dependency injection")
	if !contains2(got, "orm") || !contains2(got, "dependency-injection") {
		t.Fatalf("expected orm + dependency-injection, got %v", got)
	}
	if len(gapKeywords("a generic note with no technical nouns")) != 0 {
		t.Fatal("expected no keywords")
	}
	// space form of a hyphenated term should still match
	if !contains2(gapKeywords("uses webassembly heavily"), "webassembly") {
		t.Fatal("expected webassembly")
	}
}

func TestSortedByCount(t *testing.T) {
	s := sortedByCount(map[string]int{"a": 1, "b": 3, "c": 3})
	// count desc, then key asc for ties
	if s[0].key != "b" || s[1].key != "c" || s[2].key != "a" {
		t.Fatalf("bad sort: %+v", s)
	}
}

func TestReadLabelsAndAnalyse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "labels.jsonl")
	lines := []string{
		`{"input":"a","tags":[{"facet":"role","term":"library","evidence_kind":"file","confidence":"high"},{"facet":"audience","term":"developer","evidence_kind":"readme","confidence":"medium"}],"unclassified":["no ORM term"]}`,
		`{"input":"b","error":"model: claude -p: exit status 1"}`,
		``,
		`{"input":"c","tags":[{"facet":"role","term":"cli-tool","evidence_kind":"manifest","confidence":"high"}]}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recs, err := readLabels(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
	// analyse should not panic on this mix; capture nothing, just exercise it.
	w, _ := os.Open(os.DevNull)
	defer func() { _ = w.Close() }()
	analyse(os.Stdout, path, recs, 5, 5)
}

func contains2(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}
