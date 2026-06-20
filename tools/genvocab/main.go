// Command genvocab generates the embedded term vocabulary from oss-taxonomy's
// combined-taxonomy.json. It writes two files next to classify.go:
//
//	terms.txt  one facet:term per line, used to validate model output
//	vocab.txt  facet:term (aka: aliases) — description, used in the classify prompt
//
// Run via `go generate ./...` (see the directive in cmd/distill/classify.go) or
// directly with -taxonomy pointing at a combined-taxonomy.json checkout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type term struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
}

func main() {
	home, _ := os.UserHomeDir()
	defaultTax := filepath.Join(home, "code", "ecosystems", "oss-taxonomy", "combined-taxonomy.json")
	taxPath := flag.String("taxonomy", defaultTax, "Path to oss-taxonomy combined-taxonomy.json")
	outDir := flag.String("out", "cmd/distill", "Directory to write terms.txt and vocab.txt")
	flag.Parse()

	if err := run(*taxPath, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "genvocab: %v\n", err)
		os.Exit(1)
	}
}

func run(taxPath, outDir string) error {
	data, err := os.ReadFile(taxPath) //nolint:gosec // path is operator-supplied
	if err != nil {
		return err
	}
	// The file is a flat object of facet -> []term plus scalar metadata
	// (version, generated_at); decode facets as the arrays that parse.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	facets := make([]string, 0, len(raw))
	termsByFacet := map[string][]term{}
	for facet, msg := range raw {
		var ts []term
		if json.Unmarshal(msg, &ts) != nil || len(ts) == 0 {
			continue // scalar metadata key, skip
		}
		facets = append(facets, facet)
		sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
		termsByFacet[facet] = ts
	}
	sort.Strings(facets)

	var terms, vocab []string
	for _, facet := range facets {
		for _, t := range termsByFacet[facet] {
			key := facet + ":" + t.Name
			terms = append(terms, key)
			line := key
			if len(t.Aliases) > 0 {
				line += " (aka: " + strings.Join(t.Aliases, ", ") + ")"
			}
			if desc := firstSentence(t.Description); desc != "" {
				line += " — " + desc
			}
			vocab = append(vocab, line)
		}
	}

	if err := writeLines(filepath.Join(outDir, "terms.txt"), terms); err != nil {
		return err
	}
	if err := writeLines(filepath.Join(outDir, "vocab.txt"), vocab); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "genvocab: wrote %d terms to %s/{terms,vocab}.txt\n", len(terms), outDir)
	return nil
}

func firstSentence(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func writeLines(path string, lines []string) error {
	const perm = 0o644
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), perm)
}
