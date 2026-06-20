package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// cmdAnalyse summarises a labels.jsonl from classify/corpus: success rate,
// evidence grounding, per-facet coverage, term frequency, and the recurring
// unclassified themes that feed oss-taxonomy term proposals.
const (
	defaultTopTerms = 25
	defaultTopGaps  = 30
)

func cmdAnalyse(args []string) {
	fs := flag.NewFlagSet("distill analyse", flag.ExitOnError)
	topTerms := fs.Int("top-terms", defaultTopTerms, "How many of the most-used terms to list")
	topGaps := fs.Int("top-gaps", defaultTopGaps, "How many unclassified gap themes to list")
	_ = fs.Parse(args)

	path := "labels.jsonl"
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	recs, err := readLabels(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	analyse(os.Stdout, path, recs, *topTerms, *topGaps)
}

func readLabels(path string) ([]Result, error) {
	f, err := os.Open(path) //nolint:gosec // user-supplied path is the point
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []Result
	sc := bufio.NewScanner(f)
	const maxLine = 1 << 20
	sc.Buffer(make([]byte, 0), maxLine)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Result
		if jerr := json.Unmarshal([]byte(line), &r); jerr != nil {
			return nil, fmt.Errorf("parse line: %w", jerr)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

var facetOrder = []string{"role", "function", "technology", "layer", "domain", "audience"}

func analyse(w io.Writer, path string, recs []Result, topTerms, topGaps int) {
	var ok []Result
	errs := map[string]int{}
	for _, r := range recs {
		if r.Error != "" {
			errs[errorClass(r.Error)]++
			continue
		}
		ok = append(ok, r)
	}
	var tags []Tag
	for _, r := range ok {
		tags = append(tags, r.Tags...)
	}
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

	reportSummary(p, path, recs, ok, errs)
	reportTags(p, ok, tags)
	reportFacets(p, ok)
	reportTerms(p, tags, topTerms)
	reportGaps(p, ok, topGaps)
}

type printf func(format string, a ...any)

func reportSummary(p printf, path string, recs, ok []Result, errs map[string]int) {
	total := len(recs)
	p("= corpus: %s =\n", path)
	p("  %d records  %d ok (%s)  %d errors\n", total, len(ok), pct(len(ok), total), total-len(ok))
	for _, e := range sortedByCount(errs) {
		p("    %d  %s\n", e.n, e.key)
	}
}

func reportTags(p printf, ok []Result, tags []Tag) {
	minTags, maxTags := -1, 0
	for _, r := range ok {
		n := len(r.Tags)
		if minTags == -1 || n < minTags {
			minTags = n
		}
		if n > maxTags {
			maxTags = n
		}
	}
	if minTags == -1 {
		minTags = 0
	}
	mean := 0.0
	if len(ok) > 0 {
		mean = float64(len(tags)) / float64(len(ok))
	}
	p("\n= tags =\n")
	p("  %d total  mean %.1f  min %d  max %d\n", len(tags), mean, minTags, maxTags)

	kinds := map[string]int{}
	conf := map[string]int{}
	grounded := 0
	for _, t := range tags {
		kinds[t.EvidenceKind]++
		conf[t.Confidence]++
		if t.EvidenceKind != "readme" {
			grounded++
		}
	}
	p("  evidence: %s\n", joinCounts(sortedByCount(kinds)))
	p("  code-grounded %s  readme %s\n", pct(grounded, len(tags)), pct(len(tags)-grounded, len(tags)))
	p("  confidence: high %d  medium %d  low %d\n", conf["high"], conf["medium"], conf["low"])
}

func reportFacets(p printf, ok []Result) {
	p("\n= per-facet (tags / distinct terms / pkgs covered of %d) =\n", len(ok))
	for _, f := range facetOrder {
		var ft int
		distinct := map[string]bool{}
		covered := 0
		for _, r := range ok {
			has := false
			for _, t := range r.Tags {
				if t.Facet == f {
					ft++
					distinct[t.Term] = true
					has = true
				}
			}
			if has {
				covered++
			}
		}
		p("  %-11s %4d  %3d terms  %3d pkgs (%s)\n", f, ft, len(distinct), covered, pct(covered, len(ok)))
	}
}

func reportTerms(p printf, tags []Tag, topTerms int) {
	p("\n= top %d terms =\n", topTerms)
	termCounts := map[string]int{}
	for _, t := range tags {
		termCounts[t.Facet+":"+t.Term]++
	}
	for _, e := range topN(sortedByCount(termCounts), topTerms) {
		p("  %4d  %s\n", e.n, e.key)
	}
}

func reportGaps(p printf, ok []Result, topGaps int) {
	p("\n= unclassified gap themes (top %d) =\n", topGaps)
	gaps := map[string]int{}
	for _, r := range ok {
		for _, u := range r.Unclassified {
			if strings.HasPrefix(u, "rejected") {
				continue
			}
			for _, kw := range gapKeywords(u) {
				gaps[kw]++
			}
		}
	}
	for _, e := range topN(sortedByCount(gaps), topGaps) {
		p("  %3d  %s\n", e.n, e.key)
	}
}

func errorClass(msg string) string {
	switch {
	case strings.Contains(msg, "claude -p"):
		return "model (claude -p)"
	case strings.Contains(msg, "outline"):
		return "brief outline"
	case strings.Contains(msg, "brief json"):
		return "brief json"
	case strings.Contains(msg, "resolve"):
		return "unresolved repo"
	default:
		return "other"
	}
}

// gapTerms are recurring technical nouns pulled out of free-text unclassified
// notes so the same concept clusters regardless of phrasing.
var gapTerms = []string{
	"middleware", "migration", "seeding", "config", "env", "wasm", "webassembly",
	"opentelemetry", "tracing", "observability", "ffi", "graphql", "grpc", "orm",
	"regex", "profiling", "feature-flag", "dependency-injection", "mock", "fuzzing",
	"llm", "embedding", "vector-search", "crdt", "raft", "consensus", "key-value",
	"columnar", "state-machine", "connection-pool", "circuit-breaker", "rate-limiting",
	"retry", "backoff", "date-time", "timezone", "uuid", "wasi", "simd",
}

func gapKeywords(text string) []string {
	t := strings.ToLower(text)
	var out []string
	for _, kw := range gapTerms {
		if strings.Contains(t, kw) || strings.Contains(t, strings.ReplaceAll(kw, "-", " ")) {
			out = append(out, kw)
		}
	}
	return out
}

type kv struct {
	key string
	n   int
}

func sortedByCount(m map[string]int) []kv {
	out := make([]kv, 0, len(m))
	for k, n := range m {
		out = append(out, kv{k, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].key < out[j].key
	})
	return out
}

func topN(s []kv, n int) []kv {
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

func joinCounts(s []kv) string {
	parts := make([]string, len(s))
	for i, e := range s {
		parts[i] = fmt.Sprintf("%s %d", e.key, e.n)
	}
	return strings.Join(parts, "  ")
}

func pct(n, d int) string {
	if d == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(d))
}
