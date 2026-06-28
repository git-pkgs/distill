package main

import (
	"strings"
	"testing"
)

func TestNonInteractiveGitEnv(t *testing.T) {
	env := nonInteractiveGitEnv()
	want := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
	}
	joined := strings.Join(env, "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("nonInteractiveGitEnv() missing %q", w)
		}
	}
}

func TestCloneableRepo(t *testing.T) {
	cases := map[string]string{
		"https://cs.opensource.google/go/x/crypto": "https://github.com/golang/crypto",
		"https://go.googlesource.com/net":          "https://github.com/golang/net",
		"https://golang.org/x/tools":               "https://github.com/golang/tools",
		"https://go.googlesource.com/sys.git":       "https://github.com/golang/sys",
		"https://cs.opensource.google/go/x/text/+/refs": "https://github.com/golang/text",
		// Apache gitbox gitweb (browse) URLs -> github.com/apache mirror.
		"https://gitbox.apache.org/repos/asf?p=commons-fileupload.git":         "https://github.com/apache/commons-fileupload",
		"https://gitbox.apache.org/repos/asf?p=maven.git;a=summary":            "https://github.com/apache/maven",
		"https://gitbox.apache.org/repos/asf/commons-lang.git":                 "https://github.com/apache/commons-lang",
		// Already cloneable URLs pass through untouched.
		"https://github.com/golang/go":     "https://github.com/golang/go",
		"https://github.com/spf13/cobra":   "https://github.com/spf13/cobra",
		"https://gitlab.com/group/project": "https://gitlab.com/group/project",
	}
	for in, want := range cases {
		if got := cloneableRepo(in); got != want {
			t.Errorf("cloneableRepo(%q) = %q, want %q", in, got, want)
		}
	}
}
