package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestSplitIdent(t *testing.T) {
	cases := map[string][]string{
		"snake_case":     {"snake", "case"},
		"camelCase":      {"camel", "case"},
		"HTTPServer":     {"http", "server"},
		"parseJSONFile":  {"parse", "json", "file"},
		"already":        {"already"},
		"UPPER":          {"upper"},
		"foo2bar":        {"foo", "bar"},
		"__dunder__":     {"dunder"},
		"X":              {"x"},
		"TokenBucketV2":  {"token", "bucket", "v"},
		"read_JSON_file": {"read", "json", "file"},
	}
	for in, want := range cases {
		if got := splitIdent(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitIdent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIdentifiers(t *testing.T) {
	body := "\n### a.go\n\n````go\nfunc ParseConfig() {}\ntype HTTPServer struct {}\nfunc (s *HTTPServer) handle_request() {}\n````\n" +
		"\n### LICENSE\n\n````\nabsolutely about acceptable\n````\n"
	got := identifiers(body, 0)
	// http/server appear twice (type + receiver), so rank first.
	want := []string{"http", "server", "config", "handle", "parse", "request"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("identifiers = %v, want %v", got, want)
	}
	for _, leaked := range []string{"func", "type", "struct", "absolutely", "about", "acceptable"} {
		if slices.Contains(got, leaked) {
			t.Errorf("token %q leaked into identifiers", leaked)
		}
	}
}

func TestIdentifiersCap(t *testing.T) {
	body := "\n### a.go\n\n````go\nalpha alpha beta gamma delta epsilon\n````"
	got := identifiers(body, 3)
	if len(got) != 3 || got[0] != "alpha" {
		t.Fatalf("expected cap of 3 with alpha first, got %v", got)
	}
}

func TestCodeBlocks(t *testing.T) {
	body := "\n### a.go\n\n````go\nA\n````\n\n### b.yml\n\n````yaml\nB\n````\n\n### c.py\n\n````python\nC\n````\n"
	got := codeBlocks(body)
	if len(got) != 2 || got[0] != "A" || got[1] != "C" {
		t.Fatalf("codeBlocks = %q", got)
	}
}

func TestCodeBlocksVariableFence(t *testing.T) {
	// outline bumps fence length when content contains backticks; nested
	// 3-backtick fences inside a markdown file must not be picked up.
	body := "\n### README.md\n\n`````markdown\n```python\nignored = 1\n```\n`````\n" +
		"\n### main.py\n\n`````python\ndef kept(): pass\n`````\n"
	got := codeBlocks(body)
	if len(got) != 1 || !contains(got[0], "kept") || contains(got[0], "ignored") {
		t.Fatalf("codeBlocks = %q", got)
	}
}

func TestOutlineFence(t *testing.T) {
	cases := map[string]string{
		"\n### a\n\n````go\nx\n````":     "````",
		"\n### a\n\n`````go\nx\n`````":   "`````",
		"\n### a\n\n```rust\nx\n```":     "```",
		"no headers":                     "",
		"\n### a\n\nnot a fence\n````go": "",
	}
	for in, want := range cases {
		if got := outlineFence(slicesOfLines(in)); got != want {
			t.Errorf("outlineFence(%q) = %q, want %q", in, got, want)
		}
	}
}

func slicesOfLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func TestStackTags(t *testing.T) {
	r := &briefReport{
		Tools: map[string][]detection{
			"build": {{Name: "GoReleaser", Taxonomy: map[string][]string{"role": {"build-tool"}, "function": {"release-management", "deployment"}}}},
			"test":  {{Name: "go test", Taxonomy: map[string][]string{"role": {"testing-framework"}, "function": {"testing"}}}},
		},
	}
	got := stackTags(r)
	want := []string{"function:deployment", "function:release-management", "function:testing", "role:build-tool", "role:testing-framework"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stackTags = %v, want %v", got, want)
	}
}

func TestStructure(t *testing.T) {
	r := &briefReport{
		Tools: map[string][]detection{
			"test":      {{Name: "go test"}},
			"container": {{Name: "Docker"}},
		},
	}
	r.Languages = []struct {
		Name string `json:"name"`
	}{{Name: "Go"}, {Name: "Ruby"}}
	r.Lines.ByLanguage = map[string]int{"Go": 750, "Ruby": 250, "Markdown": 999}
	r.Lines.TotalFiles = 42
	r.Dependencies = []struct {
		Direct bool `json:"direct"`
	}{{Direct: true}, {Direct: false}, {Direct: true}}
	r.Layout.SourceDirs = []string{"cmd"}
	tree := "├── cmd/\n│   └── main.go\n├── proto/\n│   └── api.proto\n├── db/\n│   └── migrate/\n│       └── 001_init.sql\n└── README.md"
	s := structure(r, tree)
	if !s.HasEntrypoint || !s.HasTests || !s.HasDockerfile || !s.HasProto || !s.HasMigrations {
		t.Fatalf("missing structural booleans: %+v", s)
	}
	if s.HasInfra {
		t.Fatalf("unexpected has_infra: %+v", s)
	}
	if s.DirectDepCount != 2 || s.FileCount != 42 {
		t.Fatalf("counts wrong: %+v", s)
	}
	if s.Languages["Go"] != 0.75 || s.Languages["Ruby"] != 0.25 {
		t.Fatalf("language fractions wrong: %+v", s.Languages)
	}
	if _, ok := s.Languages["Markdown"]; ok {
		t.Fatalf("non-primary language leaked: %+v", s.Languages)
	}
}

func TestTreeLinePath(t *testing.T) {
	cases := map[string]string{
		"├── cmd/":              "cmd/",
		"│   └── main.go":       "main.go",
		"│   │   └── api.proto": "api.proto",
		"```":                   "",
		"## Structure":          "",
	}
	for in, want := range cases {
		if got := treeLinePath(in); got != want {
			t.Errorf("treeLinePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitOutline(t *testing.T) {
	md := "## Structure\n\n```\ntree\n```\n\n## Files\n\n### a.go\n```go\ncode\n```"
	tree, body := splitOutline(md)
	if !contains(tree, "tree") || contains(tree, "code") {
		t.Fatalf("tree wrong: %q", tree)
	}
	if !contains(body, "code") || contains(body, "tree") {
		t.Fatalf("body wrong: %q", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
