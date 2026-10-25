package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/kennyg/gh-tmpl/internal/discover"
)

// fixtureRepo copies a fixture repository into a temporary folder and
// marks it as a git working tree.
func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "discover", "testdata", "repos", name)
	dst := t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dst, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

type ghCall struct {
	args  []string
	stdin string
}

type harness struct {
	env    *Env
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	calls  []ghCall
	dir    string
}

func newHarness(t *testing.T, repo string) *harness {
	t.Helper()
	h := &harness{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if repo != "" {
		h.dir = fixtureRepo(t, repo)
	} else {
		h.dir = t.TempDir()
	}
	h.env = &Env{
		Stdin:  strings.NewReader(""),
		Stdout: h.stdout,
		Stderr: h.stderr,
		Getwd:  func() (string, error) { return h.dir, nil },
		CurrentRepo: func() (repository.Repository, error) {
			return repository.Repository{}, errors.New("no git remotes")
		},
		NewClient: func(string) (discover.Requester, error) {
			return nil, errors.New("authentication token not found")
		},
		RunGh: func(_ context.Context, args []string, stdin io.Reader) (int, error) {
			data, _ := io.ReadAll(stdin)
			h.calls = append(h.calls, ghCall{args: args, stdin: string(data)})
			return 0, nil
		},
		Version: "test",
	}
	return h
}

func (h *harness) run(args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	return Run(context.Background(), h.env, args)
}

func (h *harness) write(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(h.dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const goodPR = "## Summary\n\nFix it.\n\n## Testing\n\nRan go test.\n\n## Checklist\n\n- [x] Tests added or updated\n- [x] Docs updated\n"

func TestShow(t *testing.T) {
	h := newHarness(t, "pr-single")
	if code := h.run("show"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.HasPrefix(h.stdout.String(), "## Summary\n") {
		t.Errorf("stdout = %q", h.stdout)
	}

	if code := h.run("show", "pr", "--json"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var out map[string]any
	if err := json.Unmarshal(h.stdout.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["path"] != ".github/pull_request_template.md" || out["source"] != "local" || !strings.Contains(out["body"].(string), "<!--") {
		t.Errorf("json = %v", out)
	}
}

func TestShowIssueAmbiguousAndForm(t *testing.T) {
	h := newHarness(t, "issue-mixed")
	if code := h.run("show", "issue"); code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
	for _, want := range []string{"bug_report", "feature_request", "--name"} {
		if !strings.Contains(h.stderr.String(), want) {
			t.Errorf("stderr does not mention %q:\n%s", want, h.stderr)
		}
	}

	if code := h.run("show", "issue", "--form", "bug_report"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.HasPrefix(h.stdout.String(), "### What happened?") {
		t.Errorf("stdout = %q", h.stdout)
	}

	if code := h.run("show", "issue", "--form", "feature_request"); code != ExitUsage {
		t.Errorf("--form on a Markdown template: exit %d", code)
	}
	if code := h.run("show", "pr", "--form", "bug_report"); code != ExitUsage {
		t.Errorf("--form with pr: exit %d", code)
	}

	if code := h.run("show", "issue", "--name", "bug_report", "--raw"); code != ExitOK || !strings.HasPrefix(h.stdout.String(), "name: Bug report") {
		t.Errorf("--raw: exit %d, stdout %q", code, h.stdout)
	}

	if code := h.run("show", "issue", "--list", "--json"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var list struct {
		Templates []map[string]any `json:"templates"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &list); err != nil || len(list.Templates) != 2 {
		t.Errorf("list = %s (%v)", h.stdout, err)
	}
	if !strings.Contains(h.stderr.String(), "broken.yml") {
		t.Errorf("no warning about the broken form: %s", h.stderr)
	}
}

func TestShowOutsideRepo(t *testing.T) {
	h := newHarness(t, "")
	if code := h.run("show"); code != ExitUsage || !strings.Contains(h.stderr.String(), "--repo") {
		t.Errorf("exit %d, stderr %s", code, h.stderr)
	}
}

func TestShowStripsControlCharacters(t *testing.T) {
	h := newHarness(t, "empty")
	h.write(t, "pull_request_template.md", "## Title\x1b]0;pwned\x07\n\u202eevil\n")
	if code := h.run("show"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := h.stdout.String(); got != "## Title]0;pwned\nevil\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestCheck(t *testing.T) {
	h := newHarness(t, "pr-single")
	good := h.write(t, "good.md", goodPR)
	bad := h.write(t, "bad.md", "## Summary\n\nFix it.\n")

	if code := h.run("check", "--body-file", good); code != ExitOK {
		t.Fatalf("good body: exit %d\n%s", code, h.stdout)
	}
	if !strings.HasPrefix(h.stdout.String(), "PASS") {
		t.Errorf("stdout = %s", h.stdout)
	}

	if code := h.run("check", "--body-file", bad, "pr", "--json"); code != ExitFailed {
		t.Fatalf("bad body: exit %d", code)
	}
	var rep struct {
		OK       bool `json:"ok"`
		Errors   int  `json:"errors"`
		Findings []struct {
			Code string `json:"code"`
			Fix  string `json:"fix"`
		} `json:"findings"`
		Template struct {
			Path string `json:"path"`
		} `json:"template"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.Errors != 2 || rep.Template.Path != ".github/pull_request_template.md" {
		t.Errorf("report = %+v", rep)
	}

	h.env.Stdin = strings.NewReader(goodPR)
	if code := h.run("check", "-F", "-"); code != ExitOK {
		t.Errorf("stdin body: exit %d\n%s%s", code, h.stdout, h.stderr)
	}

	if code := h.run("check", "--body", "## Summary\n\nx"); code != ExitFailed {
		t.Errorf("--body: exit %d", code)
	}
}

func TestCheckUsageErrors(t *testing.T) {
	h := newHarness(t, "pr-single")
	body := h.write(t, "b.md", goodPR)
	tests := [][]string{
		{"check"},
		{"check", "--body", "x", "--body-file", body},
		{"check", "--body-file", filepath.Join(h.dir, "missing.md")},
		{"check", "--body-file", body, "discussion"},
		{"check", "--body-file", body, "--name", "nope"},
		{"check", "--body-file", h.dir},
		{"check", "--no-such-flag"},
	}
	for _, args := range tests {
		if code := h.run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
	if code := h.run("check", "--body-file", h.dir, "--json"); code != ExitUsage || !strings.Contains(h.stdout.String(), `"error"`) {
		t.Errorf("--json usage error: exit %d, stdout %s", code, h.stdout)
	}
}

func TestCheckBodyTooLarge(t *testing.T) {
	h := newHarness(t, "pr-single")
	h.env.Stdin = strings.NewReader(strings.Repeat("a", 1<<20+1))
	if code := h.run("check", "--body-file", "-"); code != ExitUsage || !strings.Contains(h.stderr.String(), "larger") {
		t.Errorf("exit %d, stderr %s", code, h.stderr)
	}
}

func TestCheckWithoutTemplatePasses(t *testing.T) {
	h := newHarness(t, "empty")
	if code := h.run("check", "--body", "anything", "--json"); code != ExitOK || !strings.Contains(h.stdout.String(), `"template": null`) {
		t.Errorf("exit %d, stdout %s", code, h.stdout)
	}
}

func TestCheckTemplateFile(t *testing.T) {
	h := newHarness(t, "")
	tmpl := h.write(t, "t.md", "## Why\n\n<!-- Explain. -->\n")
	form := h.write(t, "f.yml", "name: F\nbody:\n  - type: input\n    attributes: {label: Name}\n    validations: {required: true}\n")
	if code := h.run("check", "--template-file", tmpl, "--body", "## Why\n\nBecause."); code != ExitOK {
		t.Errorf("markdown: exit %d\n%s", code, h.stdout)
	}
	if code := h.run("check", "issue", "--template-file", form, "--body", "### Name\n\n_No response_\n"); code != ExitFailed {
		t.Errorf("form: exit %d\n%s", code, h.stdout)
	}
}

func TestCheckIssueForm(t *testing.T) {
	h := newHarness(t, "issue-mixed")
	body := h.write(t, "b.md", "### What happened?\n\nA crash.\n\n### Version\n\n_No response_\n\n### Operating system\n\nLinux\n\n### Logs\n\n_No response_\n\n### Code of conduct\n\n- [x] I agree to follow the code of conduct.\n- [ ] I searched for existing issues.\n")
	if code := h.run("check", "issue", "--name", "Bug report", "--body-file", body); code != ExitOK {
		t.Errorf("exit %d\n%s", code, h.stdout)
	}
}

func TestOrgFallback(t *testing.T) {
	h := newHarness(t, "empty")
	var requested []string
	h.env.CurrentRepo = func() (repository.Repository, error) {
		return repository.Repository{Host: "ghe.example.com", Owner: "octo", Name: "app"}, nil
	}
	h.env.NewClient = func(host string) (discover.Requester, error) {
		if host != "ghe.example.com" {
			t.Errorf("client for host %q", host)
		}
		return fakeContents(func(p string) (any, bool) {
			requested = append(requested, p)
			switch p {
			case "repos/octo/.github/contents":
				return []map[string]string{{"name": "pull_request_template.md", "type": "file"}}, true
			case "repos/octo/.github/contents/pull_request_template.md":
				return file("## Shared\n\n<!-- Org-wide template. -->\n"), true
			}
			return nil, false
		}), nil
	}
	if code := h.run("show", "--json"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if !strings.Contains(h.stdout.String(), `"source": "ghe.example.com/octo/.github"`) {
		t.Errorf("stdout = %s", h.stdout)
	}

	requested = nil
	if code := h.run("show", "--offline"); code != ExitUsage || len(requested) != 0 {
		t.Errorf("--offline: exit %d, requests %v", code, requested)
	}

	// --repo reads the named repository, then its owner's .github.
	requested = nil
	if code := h.run("show", "-R", "ghe.example.com/octo/api"); code != ExitOK {
		t.Fatalf("--repo: exit %d: %s", code, h.stderr)
	}
	if len(requested) == 0 || requested[0] != "repos/octo/api/contents" {
		t.Errorf("requests = %v", requested)
	}
}

func TestOrgFallbackWithoutLogin(t *testing.T) {
	h := newHarness(t, "empty")
	h.env.CurrentRepo = func() (repository.Repository, error) {
		return repository.Repository{Host: "github.com", Owner: "octo", Name: "app"}, nil
	}
	if code := h.run("check", "--body", "x"); code != ExitOK {
		t.Errorf("exit %d", code)
	}
	if !strings.Contains(h.stderr.String(), "skipped the shared templates") {
		t.Errorf("stderr = %s", h.stderr)
	}
}

func file(content string) map[string]any {
	return map[string]any{"type": "file", "size": len(content), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(content))}
}

type fakeContents func(p string) (any, bool)

func (f fakeContents) RequestWithContext(_ context.Context, _ string, p string, _ io.Reader) (*http.Response, error) {
	v, ok := f(p)
	if !ok {
		return nil, &api.HTTPError{StatusCode: 404, RequestURL: &url.URL{Path: p}}
	}
	data, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func TestPRCreate(t *testing.T) {
	h := newHarness(t, "pr-single")
	good := h.write(t, "good.md", goodPR)

	code := h.run("pr", "create", "--title", "Fix", "--body-file", good, "--tmpl-strict", "--draft")
	if code != ExitOK || len(h.calls) != 1 {
		t.Fatalf("exit %d, calls %v\n%s", code, h.calls, h.stderr)
	}
	want := []string{"pr", "create", "--title", "Fix", "--body-file", good, "--draft"}
	if strings.Join(h.calls[0].args, " ") != strings.Join(want, " ") {
		t.Errorf("args = %q, want %q", h.calls[0].args, want)
	}

	// A failing body stops before gh runs, and the report goes to stderr.
	h.calls = nil
	code = h.run("pr", "create", "-t", "Fix", "-b", "## Summary\n\nx")
	if code != ExitFailed || len(h.calls) != 0 {
		t.Fatalf("exit %d, calls %v", code, h.calls)
	}
	if h.stdout.Len() != 0 || !strings.Contains(h.stderr.String(), "Nothing was created") {
		t.Errorf("stdout %q, stderr %q", h.stdout, h.stderr)
	}

	// A body from standard input is passed on to gh.
	h.calls = nil
	h.env.Stdin = strings.NewReader(goodPR)
	if code := h.run("pr", "create", "-F", "-"); code != ExitOK || h.calls[0].stdin != goodPR {
		t.Errorf("stdin: exit %d, calls %+v", code, h.calls)
	}

	// Without a body there is nothing to check.
	h.calls = nil
	if code := h.run("pr", "create", "--fill"); code != ExitOK || len(h.calls) != 1 {
		t.Errorf("--fill: exit %d, calls %v", code, h.calls)
	}

	// --tmpl-skip runs gh with a failing body.
	h.calls = nil
	if code := h.run("pr", "create", "--body", "x", "--tmpl-skip"); code != ExitOK || len(h.calls) != 1 {
		t.Errorf("--tmpl-skip: exit %d, calls %v", code, h.calls)
	}
}

func TestIssueCreateNeedsName(t *testing.T) {
	h := newHarness(t, "issue-mixed")
	body := h.write(t, "b.md", "## Problem\n\nSlow.\n\n## Proposal\n\nCache it.\n")
	if code := h.run("issue", "create", "--body-file", body); code != ExitUsage || !strings.Contains(h.stderr.String(), "--tmpl-name") {
		t.Fatalf("exit %d, stderr %s", code, h.stderr)
	}
	if code := h.run("issue", "create", "--body-file", body, "--tmpl-name", "feature_request", "-l", "enhancement"); code != ExitOK {
		t.Fatalf("exit %d, stderr %s", code, h.stderr)
	}
	if got := strings.Join(h.calls[0].args, " "); got != "issue create --body-file "+body+" -l enhancement" {
		t.Errorf("args = %s", got)
	}
}

func TestGhExitCodeIsKept(t *testing.T) {
	h := newHarness(t, "empty")
	h.env.RunGh = func(context.Context, []string, io.Reader) (int, error) { return 4, nil }
	if code := h.run("pr", "create", "--body", "x", "--tmpl-offline"); code != 4 {
		t.Errorf("exit %d, want 4", code)
	}
}

func TestScanCreate(t *testing.T) {
	tests := []struct {
		name     string
		kind     discover.Kind
		args     []string
		pass     string
		body     string
		bodyFile string
		repo     string
		tmpl     string
	}{
		{name: "long flags", kind: discover.PR, args: []string{"--title", "--body", "--body-file=b.md"}, pass: "--title --body --body-file=b.md", bodyFile: "b.md"},
		{name: "short cluster", kind: discover.PR, args: []string{"-dwF", "b.md"}, pass: "-dwF b.md", bodyFile: "b.md"},
		{name: "attached value", kind: discover.PR, args: []string{"-Fb.md", "-R=o/r"}, pass: "-Fb.md -R=o/r", bodyFile: "b.md", repo: "o/r"},
		{name: "body text", kind: discover.Issue, args: []string{"-b", "-F looks like a flag"}, pass: "-b -F looks like a flag", body: "-F looks like a flag"},
		{name: "tmpl flags removed", kind: discover.Issue, args: []string{"--tmpl-name", "bug", "-t", "x", "--tmpl-offline"}, pass: "-t x", tmpl: "bug"},
		{name: "tmpl name with equals", kind: discover.PR, args: []string{"--tmpl-name=feature"}, tmpl: "feature"},
		{name: "after --", kind: discover.PR, args: []string{"--", "--tmpl-name", "x"}, pass: "-- --tmpl-name x"},
		{name: "unknown flags pass through", kind: discover.PR, args: []string{"--future-flag", "-z"}, pass: "--future-flag -z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := scanCreate(tt.kind, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(c.pass, " "); got != tt.pass {
				t.Errorf("pass = %q, want %q", got, tt.pass)
			}
			body := ""
			if c.body != nil {
				body = *c.body
			}
			if body != tt.body || c.bodyFile != tt.bodyFile || c.repo != tt.repo || c.opts.name != tt.tmpl {
				t.Errorf("got body %q, file %q, repo %q, name %q", body, c.bodyFile, c.repo, c.opts.name)
			}
		})
	}
	for _, bad := range [][]string{{"--tmpl-name"}, {"--tmpl-bogus"}, {"--title"}, {"-F"}} {
		if _, err := scanCreate(discover.PR, bad); err == nil {
			t.Errorf("scanCreate(%q) returned no error", bad)
		}
	}
}

func TestTopLevel(t *testing.T) {
	h := newHarness(t, "")
	for args, want := range map[string]int{"": ExitUsage, "--help": ExitOK, "--version": ExitOK, "bogus": ExitUsage, "pr": ExitUsage, "pr list": ExitUsage} {
		if code := h.run(strings.Fields(args)...); code != want {
			t.Errorf("%q: exit %d, want %d", args, code, want)
		}
	}
	h.run("--version")
	if h.stdout.String() != "gh-tmpl test\n" {
		t.Errorf("version = %q", h.stdout)
	}
}
