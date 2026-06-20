package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/git-pkgs/enrichment"
)

//go:embed terms.txt
var termsTxt string

//go:embed vocab.txt
var vocabTxt string

//go:embed schema.json
var classifySchema string

const (
	defaultModel      = "claude-opus-4-8"
	defaultOutlineCap = 50_000
	defaultReadmeCap  = 30_000
	claudeTimeout     = 10 * time.Minute
	truncationMarker  = "\n\n[... truncated ...]\n"
	systemPrompt      = "You are classifying open-source software into a fixed taxonomy. " +
		"Only use terms from the provided list. Every tag must cite specific, " +
		"mechanically-checkable evidence from the supplied data. Do not invent terms."
)

type Tag struct {
	Facet        string `json:"facet"`
	Term         string `json:"term"`
	Evidence     string `json:"evidence"`
	EvidenceKind string `json:"evidence_kind"`
	Confidence   string `json:"confidence"`
}

type Classification struct {
	Tags         []Tag    `json:"tags"`
	Unclassified []string `json:"unclassified"`
}

type Result struct {
	Input        string   `json:"input"`
	Purl         string   `json:"purl,omitempty"`
	Repo         string   `json:"repo,omitempty"`
	Model        string   `json:"model"`
	Tags         []Tag    `json:"tags"`
	Unclassified []string `json:"unclassified"`
	CostUSD      float64  `json:"cost_usd,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type classifyOpts struct {
	model      string
	briefBin   string
	claudeBin  string
	outlineCap int
	readmeCap  int
	keep       bool
}

func cmdClassify(args []string) {
	fs := flag.NewFlagSet("distill classify", flag.ExitOnError)
	model := fs.String("model", defaultModel, "Claude model ID")
	briefBin := fs.String("brief", "brief", "Path to brief binary")
	claudeBin := fs.String("claude", "claude", "Path to claude CLI")
	outlineCap := fs.Int("outline-cap", defaultOutlineCap, "Max bytes of outline to include")
	readmeCap := fs.Int("readme-cap", defaultReadmeCap, "Max bytes of README to include")
	keep := fs.Bool("keep", false, "Keep cloned source directories")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one purl or url required")
		os.Exit(exitUsage)
	}

	opts := classifyOpts{
		model:      *model,
		briefBin:   *briefBin,
		claudeBin:  *claudeBin,
		outlineCap: *outlineCap,
		readmeCap:  *readmeCap,
		keep:       *keep,
	}

	enrich, err := enrichment.NewClient(enrichment.WithUserAgent("git-pkgs/distill"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enrichment client: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	exit := 0
	for _, target := range fs.Args() {
		res := classifyOne(enrich, target, opts)
		if res.Error != "" {
			exit = 1
		}
		_ = enc.Encode(res)
	}
	os.Exit(exit)
}

func classifyOne(enrich enrichment.Client, target string, opts classifyOpts) Result {
	g, err := gather(enrich, target, gatherOpts{briefBin: opts.briefBin, keep: opts.keep})
	defer g.cleanup()
	return classifyFrom(g, err, opts)
}

func classifyFrom(g *gathered, gatherErr error, opts classifyOpts) Result {
	res := Result{Input: g.input, Purl: g.purl, Repo: g.repo, Model: opts.model}
	if gatherErr != nil {
		res.Error = gatherErr.Error()
		return res
	}
	prompt := buildPrompt(g.input, g.purl, g.briefJSON, capBytes(g.outline, opts.outlineCap), capBytes(g.readme, opts.readmeCap))
	cls, cost, err := callModel(opts.claudeBin, opts.model, prompt)
	res.CostUSD = cost
	if err != nil {
		res.Error = fmt.Sprintf("model: %v", err)
		return res
	}
	res.Tags = cls.Tags
	res.Unclassified = cls.Unclassified
	return res
}

func capBytes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + truncationMarker
}

func buildPrompt(target, purlStr, briefJSON, outline, readme string) string {
	var b strings.Builder
	b.WriteString("Classify the following open-source package into the oss-taxonomy vocabulary.\n\n")
	b.WriteString("# Allowed terms\n\n")
	b.WriteString("Only emit facet:term pairs from this list. Each line is `facet:term (aka: aliases) — description`. ")
	b.WriteString("The aliases are alternative names for that term: if a concept matches an alias, emit the term it ")
	b.WriteString("belongs to and treat the concept as fully covered. For example an ORM or query builder is ")
	b.WriteString("function:data-mapping (its aliases include orm and query-builder) — emit function:data-mapping and ")
	b.WriteString("do NOT also report ORM under unclassified. Only use unclassified for concepts that match no term ")
	b.WriteString("AND no alias.\n\n")
	b.WriteString(vocabTxt)
	b.WriteString("\n\n# Package\n\n")
	b.WriteString("target: ")
	b.WriteString(target)
	b.WriteString("\n")
	if purlStr != "" {
		b.WriteString("purl: ")
		b.WriteString(purlStr)
		b.WriteString("\n")
	}
	b.WriteString("\n# brief detection (JSON)\n\n")
	b.WriteString(briefJSON)
	b.WriteString("\n\n# outline (public API surface, truncated)\n\n")
	b.WriteString(outline)
	if readme != "" {
		b.WriteString("\n\n# README (truncated)\n\n")
		b.WriteString(readme)
	}
	b.WriteString("\n\n# Task\n\n")
	b.WriteString("Emit the classification as JSON. For each tag, the evidence field must cite a specific ")
	b.WriteString("dependency, import, exported symbol, file, or manifest entry visible in the data above. ")
	b.WriteString("Prefer evidence_kind dependency/import/symbol/file/manifest over readme; use readme only ")
	b.WriteString("when no structural evidence exists. Before adding to unclassified, check the alias lists above; ")
	b.WriteString("only list a concept as unclassified if neither a term nor any alias covers it.\n")
	return b.String()
}

// callModel shells out to `claude -p` so distill uses whatever auth Claude Code
// already has, instead of managing API keys itself.
func callModel(claudeBin, model, prompt string) (*Classification, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), claudeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, claudeBin,
		"-p",
		"--model", model,
		"--system-prompt", systemPrompt,
		"--json-schema", classifySchema,
		"--output-format", "json",
	)
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("claude -p: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	cls, cost, err := parseClaudeOutput(out)
	if err != nil {
		return nil, cost, err
	}
	validateTerms(cls)
	return cls, cost, nil
}

// parseClaudeOutput extracts the Classification from `claude -p --output-format json`.
// With --json-schema set, the schema-conforming object lands in `structured_output`;
// `result` carries any prose summary the harness produced and is ignored here.
func parseClaudeOutput(out []byte) (*Classification, float64, error) {
	var env struct {
		Type             string          `json:"type"`
		Subtype          string          `json:"subtype"`
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		TotalCostUSD     float64         `json:"total_cost_usd"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, 0, fmt.Errorf("parse claude envelope: %w (raw: %.200s)", err, out)
	}
	if env.IsError || env.Subtype != "success" {
		return nil, env.TotalCostUSD, fmt.Errorf("claude returned %s/%s: %s", env.Type, env.Subtype, env.Result)
	}
	if len(env.StructuredOutput) == 0 || string(env.StructuredOutput) == "null" {
		return nil, env.TotalCostUSD, fmt.Errorf("no structured_output in claude response (result: %.200s)", env.Result)
	}
	var cls Classification
	if err := json.Unmarshal(env.StructuredOutput, &cls); err != nil {
		return nil, env.TotalCostUSD, fmt.Errorf("parse structured_output: %w (raw: %.200s)", err, env.StructuredOutput)
	}
	return &cls, env.TotalCostUSD, nil
}

// validateTerms drops tags whose facet:term is not in terms.txt and records the
// rejection in Unclassified so it's visible in the output rather than silent.
func validateTerms(c *Classification) {
	allowed := allowedTerms()
	kept := c.Tags[:0]
	for _, t := range c.Tags {
		if allowed[t.Facet+":"+t.Term] {
			kept = append(kept, t)
			continue
		}
		c.Unclassified = append(c.Unclassified, fmt.Sprintf("rejected invalid term %s:%s (evidence: %s)", t.Facet, t.Term, t.Evidence))
	}
	c.Tags = kept
}

func allowedTerms() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(termsTxt), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			m[line] = true
		}
	}
	return m
}
