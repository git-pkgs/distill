package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/git-pkgs/enrichment"
	"github.com/git-pkgs/purl"
)

//go:embed terms.txt
var termsTxt string

const (
	defaultModel       = anthropic.ModelClaudeOpus4_8
	defaultOutlineCap  = 50_000
	defaultReadmeCap   = 30_000
	defaultMaxTokens   = 8192
	enrichmentTimeout  = 30 * time.Second
	briefTimeout       = 5 * time.Minute
	classifyToolName   = "emit_classification"
	tmpDirPattern      = "distill-*"
	readmeGlob         = "README*"
	truncationMarker   = "\n\n[... truncated ...]\n"
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
	Error        string   `json:"error,omitempty"`
}

type classifyOpts struct {
	model      string
	briefBin   string
	outlineCap int
	readmeCap  int
	keep       bool
}

func cmdClassify(args []string) {
	fs := flag.NewFlagSet("distill classify", flag.ExitOnError)
	model := fs.String("model", string(defaultModel), "Claude model ID")
	briefBin := fs.String("brief", "brief", "Path to brief binary")
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
		outlineCap: *outlineCap,
		readmeCap:  *readmeCap,
		keep:       *keep,
	}

	client := anthropic.NewClient()
	enrich, err := enrichment.NewClient(enrichment.WithUserAgent("git-pkgs/distill"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enrichment client: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	exit := 0
	for _, target := range fs.Args() {
		res := classifyOne(client, enrich, target, opts)
		if res.Error != "" {
			exit = 1
		}
		_ = enc.Encode(res)
	}
	os.Exit(exit)
}

func classifyOne(client anthropic.Client, enrich enrichment.Client, target string, opts classifyOpts) Result {
	res := Result{Input: target, Model: opts.model}

	source, purlStr, repo := resolveTarget(enrich, target)
	res.Purl = purlStr
	res.Repo = repo
	if source == "" {
		res.Error = "could not resolve target to a repository"
		return res
	}

	dir, err := os.MkdirTemp("", tmpDirPattern)
	if err != nil {
		res.Error = fmt.Sprintf("mkdtemp: %v", err)
		return res
	}
	if !opts.keep {
		defer func() { _ = os.RemoveAll(dir) }()
	} else {
		fmt.Fprintf(os.Stderr, "kept: %s -> %s\n", target, dir)
	}

	briefJSON, err := runBrief(opts.briefBin, []string{"-json", "-keep", "-dir", dir, source})
	if err != nil {
		res.Error = fmt.Sprintf("brief json: %v", err)
		return res
	}

	outline, err := runBrief(opts.briefBin, []string{"outline", dir})
	if err != nil {
		res.Error = fmt.Sprintf("brief outline: %v", err)
		return res
	}

	readme := readReadme(dir)

	prompt := buildPrompt(target, purlStr, briefJSON, capBytes(outline, opts.outlineCap), capBytes(readme, opts.readmeCap))

	cls, err := callModel(client, opts.model, prompt)
	if err != nil {
		res.Error = fmt.Sprintf("model: %v", err)
		return res
	}
	res.Tags = cls.Tags
	res.Unclassified = cls.Unclassified
	return res
}

// resolveTarget returns (sourceForBrief, purl, repoURL). sourceForBrief is what
// to pass to `brief` (a repo URL or local path); it's empty if unresolvable.
func resolveTarget(enrich enrichment.Client, target string) (source, purlStr, repo string) {
	if strings.HasPrefix(target, "pkg:") {
		purlStr = target
		ctx, cancel := context.WithTimeout(context.Background(), enrichmentTimeout)
		defer cancel()
		info, err := enrich.BulkLookup(ctx, []string{target})
		if err == nil {
			if pi, ok := info[target]; ok && pi.Repository != "" {
				return pi.Repository, purlStr, pi.Repository
			}
		}
		// Fall back to registry URL if enrichment had nothing.
		if p, perr := purl.Parse(target); perr == nil {
			if u, uerr := p.RegistryURLWithVersion(); uerr == nil {
				repo = u
			}
		}
		return "", purlStr, repo
	}
	// Assume it's already a URL or local path brief can handle.
	return target, "", target
}

func runBrief(bin string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), briefTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func readReadme(dir string) string {
	matches, _ := filepath.Glob(filepath.Join(dir, readmeGlob))
	for _, m := range matches {
		if b, err := os.ReadFile(m); err == nil { //nolint:gosec // dir is our own mkdtemp
			return string(b)
		}
	}
	return ""
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
	b.WriteString("# Allowed terms (facet:term)\n\nOnly emit terms that appear exactly in this list:\n\n")
	b.WriteString(termsTxt)
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
	b.WriteString("Call emit_classification once. For each tag, the evidence field must cite a specific ")
	b.WriteString("dependency, import, exported symbol, file, or manifest entry visible in the data above. ")
	b.WriteString("Prefer evidence_kind dependency/import/symbol/file/manifest over readme; use readme only ")
	b.WriteString("when no structural evidence exists. List anything you wanted to tag but couldn't fit ")
	b.WriteString("into the allowed terms under unclassified.\n")
	return b.String()
}

func callModel(client anthropic.Client, model, prompt string) (*Classification, error) {
	tool := anthropic.ToolParam{
		Name:        classifyToolName,
		Description: anthropic.String("Emit the final classification for this package."),
		InputSchema: classifySchema(),
	}
	adaptive := anthropic.ThinkingConfigAdaptiveParam{}
	resp, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: defaultMaxTokens,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		System: []anthropic.TextBlockParam{{
			Text: "You are classifying open-source software into a fixed taxonomy. " +
				"Only use terms from the provided list. Every tag must cite specific, " +
				"mechanically-checkable evidence from the supplied data. Do not invent terms.",
		}},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{Name: classifyToolName}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return nil, err
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == classifyToolName {
			var cls Classification
			if jerr := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &cls); jerr != nil {
				return nil, fmt.Errorf("parse tool input: %w", jerr)
			}
			validateTerms(&cls)
			return &cls, nil
		}
	}
	return nil, fmt.Errorf("no %s tool call in response (stop_reason=%s)", classifyToolName, resp.StopReason)
}

//nolint:goconst // JSON Schema keywords ("type", "enum", etc.) read better inline than as named constants.
func classifySchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"tags": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"facet": map[string]any{
							"type": "string",
							"enum": []string{"domain", "role", "function", "layer", "technology", "audience"},
						},
						"term": map[string]any{
							"type":        "string",
							"description": "Must be one of the allowed terms for the chosen facet.",
						},
						"evidence": map[string]any{
							"type":        "string",
							"description": "Specific dependency, import, symbol, file path or manifest entry that justifies this tag.",
						},
						"evidence_kind": map[string]any{
							"type": "string",
							"enum": []string{"dependency", "import", "symbol", "file", "manifest", "readme"},
						},
						"confidence": map[string]any{
							"type": "string",
							"enum": []string{"high", "medium", "low"},
						},
					},
					"required": []string{"facet", "term", "evidence", "evidence_kind", "confidence"},
				},
			},
			"unclassified": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Aspects of the package that should be tagged but no allowed term fits.",
			},
		},
		Required: []string{"tags", "unclassified"},
	}
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
