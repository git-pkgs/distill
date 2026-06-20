package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/git-pkgs/enrichment"
)

// Features is the deterministic per-repo record the student model trains on
// (SPEC.md §2). It must be reproducible from a brief scan with no LLM in the
// loop, since brief computes the same record at inference time.
type Features struct {
	Input       string    `json:"input"`
	Purl        string    `json:"purl,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	StackTags   []string  `json:"stack_tags"`
	Structure   Structure `json:"structure"`
	Identifiers []string  `json:"identifiers"`
	Readme      string    `json:"readme"`
	Error       string    `json:"error,omitempty"`
}

type Structure struct {
	Languages      map[string]float64 `json:"languages"`
	HasEntrypoint  bool               `json:"has_entrypoint"`
	HasTests       bool               `json:"has_tests"`
	HasDockerfile  bool               `json:"has_dockerfile"`
	HasMigrations  bool               `json:"has_migrations"`
	HasProto       bool               `json:"has_proto"`
	HasInfra       bool               `json:"has_infra"`
	FileCount      int                `json:"file_count"`
	DirectDepCount int                `json:"direct_dep_count"`
}

const (
	defaultMaxIdentifiers = 4000
	minIdentifierLen      = 2
)

func cmdExtract(args []string) {
	fs := flag.NewFlagSet("distill extract", flag.ExitOnError)
	briefBin := fs.String("brief", "brief", "Path to brief binary")
	maxFiles := fs.Int("max-files", 0, "Pass through to brief outline -max-files (0 = outline default)")
	maxIdents := fs.Int("max-identifiers", defaultMaxIdentifiers, "Cap on distinct identifiers kept per repo")
	keep := fs.Bool("keep", false, "Keep cloned source directories")
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one purl or url required")
		os.Exit(exitUsage)
	}

	var outlineArgs []string
	if *maxFiles > 0 {
		outlineArgs = []string{"-max-files", fmt.Sprint(*maxFiles)}
	}

	enrich, err := enrichment.NewClient(enrichment.WithUserAgent("git-pkgs/distill"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: enrichment client: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	exit := 0
	for _, target := range fs.Args() {
		f := extractOne(enrich, target, gatherOpts{briefBin: *briefBin, outlineArgs: outlineArgs, keep: *keep}, *maxIdents)
		if f.Error != "" {
			exit = 1
		}
		_ = enc.Encode(f)
	}
	os.Exit(exit)
}

func extractOne(enrich enrichment.Client, target string, gopts gatherOpts, maxIdents int) Features {
	f := Features{Input: target}
	g, err := gather(enrich, target, gopts)
	defer g.cleanup()
	f.Purl = g.purl
	f.Repo = g.repo
	if err != nil {
		f.Error = err.Error()
		return f
	}

	rep, perr := parseBriefReport(g.briefJSON)
	if perr != nil {
		f.Error = fmt.Sprintf("parse brief json: %v", perr)
		return f
	}

	tree, body := splitOutline(g.outline)
	f.StackTags = stackTags(rep)
	f.Structure = structure(rep, tree)
	f.Identifiers = identifiers(body, maxIdents)
	f.Readme = g.readme
	return f
}

// briefReport is the subset of brief's JSON output that extract reads. It is
// deliberately partial so distill does not import brief as a library (which
// would create a module cycle once brief imports the trained artifact).
type briefReport struct {
	Languages []struct {
		Name string `json:"name"`
	} `json:"languages"`
	Lines struct {
		TotalFiles int            `json:"total_files"`
		ByLanguage map[string]int `json:"by_language"`
	} `json:"lines"`
	Layout struct {
		SourceDirs []string `json:"source_dirs"`
	} `json:"layout"`
	Tools        map[string][]detection `json:"tools"`
	Dependencies []struct {
		Direct bool `json:"direct"`
	} `json:"dependencies"`
}

type detection struct {
	Name     string              `json:"name"`
	Taxonomy map[string][]string `json:"taxonomy"`
}

func parseBriefReport(s string) (*briefReport, error) {
	var r briefReport
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// stackTags collects every facet:term pair from every detected tool's taxonomy
// block. These are the cheap, file-presence-derived tags brief already knows.
func stackTags(r *briefReport) []string {
	seen := map[string]bool{}
	add := func(d detection) {
		for facet, terms := range d.Taxonomy {
			for _, t := range terms {
				seen[facet+":"+t] = true
			}
		}
	}
	for _, cat := range r.Tools {
		for _, d := range cat {
			add(d)
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func structure(r *briefReport, tree string) Structure {
	s := Structure{
		Languages: languageFractions(r),
		FileCount: r.Lines.TotalFiles,
	}
	for _, d := range r.Dependencies {
		if d.Direct {
			s.DirectDepCount++
		}
	}
	for _, dir := range r.Layout.SourceDirs {
		if dir == "cmd" || dir == "bin" || dir == "exe" {
			s.HasEntrypoint = true
		}
	}
	tools := map[string]bool{}
	for _, cat := range r.Tools {
		for _, d := range cat {
			tools[d.Name] = true
		}
	}
	s.HasDockerfile = tools["Docker"] || tools["Podman"]
	s.HasInfra = tools["Terraform"] || tools["Helm"] || tools["Pulumi"] || tools["Kubernetes"] || tools["CloudFormation"] || tools["Ansible"]
	s.HasTests = hasCategory(r, "test")

	for _, line := range strings.Split(tree, "\n") {
		path := treeLinePath(line)
		switch {
		case path == "":
		case strings.HasSuffix(path, ".proto"):
			s.HasProto = true
		case strings.Contains(path, "migrations/") || strings.Contains(path, "migrate/") || strings.HasSuffix(path, ".sql"):
			s.HasMigrations = true
		case path == "Dockerfile" || path == "Containerfile":
			s.HasDockerfile = true
		case isEntrypointPath(path):
			s.HasEntrypoint = true
		}
	}
	return s
}

func languageFractions(r *briefReport) map[string]float64 {
	primary := map[string]bool{}
	for _, l := range r.Languages {
		primary[l.Name] = true
	}
	total := 0
	for name, n := range r.Lines.ByLanguage {
		if primary[name] {
			total += n
		}
	}
	out := map[string]float64{}
	if total == 0 {
		return out
	}
	for name, n := range r.Lines.ByLanguage {
		if primary[name] {
			out[name] = round3(float64(n) / float64(total))
		}
	}
	return out
}

func hasCategory(r *briefReport, cat string) bool {
	return len(r.Tools[cat]) > 0
}

func isEntrypointPath(p string) bool {
	switch {
	case p == "main.go", strings.HasSuffix(p, "/main.go"):
		return true
	case p == "src/main.rs", p == "__main__.py":
		return true
	case strings.HasPrefix(p, "cmd/"), strings.HasPrefix(p, "bin/"), strings.HasPrefix(p, "exe/"):
		return true
	}
	return false
}

// splitOutline separates the `## Structure` file tree from the `## Files` body.
func splitOutline(md string) (tree, body string) {
	const marker = "\n## Files\n"
	i := strings.Index(md, marker)
	if i < 0 {
		return md, ""
	}
	return md[:i], md[i+len(marker):]
}

// treeLinePath extracts the trailing path component from a line of outline's
// box-drawing tree. Returns "" for non-entry lines.
func treeLinePath(line string) string {
	i := strings.LastIndex(line, "── ")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(line[i+len("── "):])
}

var (
	wordRe    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	stopwords = map[string]bool{}
	codeLangs = map[string]bool{}
)

func init() {
	// Language keywords plus common English stopwords from doc comments. These
	// carry no classification signal and would otherwise dominate the
	// frequency-ranked identifier list.
	for _, k := range strings.Fields(
		// language keywords
		"func type struct interface package import const var return range chan go defer select " +
			"class def self lambda from with elif pass yield async await raise except " +
			"function let export default this new typeof instanceof extends super " +
			"pub impl trait enum match where dyn crate mod use unsafe " +
			"public private protected static final void abstract throws implements " +
			"if else for while switch case break continue do then end try catch finally throw " +
			"int string bool float double char byte long short error any " +
			"true false null nil none undefined nan " +
			// English stopwords (doc comments)
			"the is to of in that or and not be are by it no we an as on at " +
			"this all but can has have its will may was so you your our they them " +
			"which when what who how why one two also each only same other than " +
			"into out over under more most some such these those there here their " +
			"about after before between both does did been being would should could " +
			"must used using uses use see get got set sets",
	) {
		stopwords[k] = true
	}
	for _, l := range strings.Fields(
		"go python rust javascript typescript ruby java kotlin scala swift " +
			"c cpp csharp php elixir erlang haskell ocaml clojure dart zig nim " +
			"lua perl r julia",
	) {
		codeLangs[l] = true
	}
}

// identifiers tokenises code-language blocks in the outline body into a
// frequency-ranked, deduplicated bag of lowercase sub-words. camelCase and
// snake_case are split; keywords and very short tokens are dropped. Only
// fenced blocks whose language tag is a programming language are read, so
// prose from LICENSE, README, YAML etc. does not leak in.
func identifiers(body string, maxN int) []string {
	freq := map[string]int{}
	for _, block := range codeBlocks(body) {
		for _, w := range wordRe.FindAllString(block, -1) {
			for _, sub := range splitIdent(w) {
				if len(sub) < minIdentifierLen || stopwords[sub] {
					continue
				}
				freq[sub]++
			}
		}
	}
	out := make([]string, 0, len(freq))
	for k := range freq {
		out = append(out, k)
	}
	// Stable: most frequent first, then alphabetical.
	slices.SortFunc(out, func(a, b string) int {
		if d := freq[b] - freq[a]; d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})
	if maxN > 0 && len(out) > maxN {
		out = out[:maxN]
	}
	return out
}

// codeBlocks walks the outline `## Files` body and yields the contents of each
// file-level fenced block whose info string is a recognised programming
// language. outline picks one fence length for the whole document (long enough
// to enclose any file content), so we detect that length from the first fence
// after a `### path` header and match only fences of exactly that length —
// shorter fences are nested code examples inside markdown files.
func codeBlocks(body string) []string {
	lines := strings.Split(body, "\n")
	fence := outlineFence(lines)
	if fence == "" {
		return nil
	}
	var blocks []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		if !isFence(line, fence) {
			i++
			continue
		}
		lang := strings.TrimSpace(line[len(fence):])
		j := i + 1
		for j < len(lines) && !isFence(lines[j], fence) {
			j++
		}
		if codeLangs[lang] {
			blocks = append(blocks, strings.Join(lines[i+1:j], "\n"))
		}
		i = j + 1
	}
	return blocks
}

// outlineFence finds the file-level fence string by looking for the first
// backtick run that opens immediately after a `### path` header.
func outlineFence(lines []string) string {
	for i, line := range lines {
		if !strings.HasPrefix(line, "### ") {
			continue
		}
		// Next non-blank line should be the opening fence.
		const lookahead, minFence = 3, 3
		for j := i + 1; j < len(lines) && j <= i+lookahead; j++ {
			if lines[j] == "" {
				continue
			}
			if n := backtickRun(lines[j]); n >= minFence {
				return strings.Repeat("`", n)
			}
			break
		}
	}
	return ""
}

func backtickRun(s string) int {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	return n
}

// isFence reports whether line is a fence of exactly len(fence) backticks
// (optionally followed by a language tag). A longer backtick run is a
// different fence; a shorter one is nested content.
func isFence(line, fence string) bool {
	if !strings.HasPrefix(line, fence) {
		return false
	}
	rest := line[len(fence):]
	return len(rest) == 0 || rest[0] != '`'
}

// splitIdent breaks an identifier on underscores and camelCase boundaries and
// lowercases the parts. HTTPServer -> [http server], parseJSONFile -> [parse json file].
func splitIdent(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == '_' || (r >= '0' && r <= '9'):
			flush()
		case r >= 'A' && r <= 'Z':
			// Boundary before an uppercase that follows a lowercase, or before
			// the last uppercase in a run that precedes a lowercase (HTTPServer).
			prevLower := i > 0 && runes[i-1] >= 'a' && runes[i-1] <= 'z'
			nextLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			prevUpper := i > 0 && runes[i-1] >= 'A' && runes[i-1] <= 'Z'
			if prevLower || (prevUpper && nextLower) {
				flush()
			}
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

func round3(f float64) float64 {
	const k, half = 1000, 0.5
	return float64(int(f*k+half)) / k
}
