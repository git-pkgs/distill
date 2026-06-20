package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.txt")
	body := "# comment\n\npkg:npm/a\thttps://example.com/a\npkg:npm/a\nhttps://example.com/b\n  pkg:gem/c  \n# trailing\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadTargets(path, []string{"pkg:npm/a", "pkg:cargo/d"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pkg:npm/a", "https://example.com/b", "pkg:gem/c", "pkg:cargo/d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadTargets = %v, want %v", got, want)
	}
}

func TestLoadTargetsNoFile(t *testing.T) {
	got, err := loadTargets("", []string{"pkg:npm/x", "pkg:npm/x", "pkg:npm/y"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pkg:npm/x", "pkg:npm/y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadTargets = %v, want %v", got, want)
	}
}

func TestAlreadyLabelled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "labels.jsonl")
	body := `{"input":"pkg:npm/a","tags":[]}
{"input":"pkg:npm/b","error":"boom"}
not json
{"input":"pkg:npm/c","tags":[]}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := alreadyLabelled(path)
	if !got["pkg:npm/a"] || !got["pkg:npm/c"] {
		t.Fatalf("missing successes: %v", got)
	}
	if got["pkg:npm/b"] {
		t.Fatalf("errored entry should not count as done: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 done, got %v", got)
	}
}

func TestAlreadyLabelledMissingFile(t *testing.T) {
	got := alreadyLabelled(filepath.Join(t.TempDir(), "nope.jsonl"))
	if len(got) != 0 {
		t.Fatalf("expected empty for missing file, got %v", got)
	}
}
