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
	"os/exec"
	"path/filepath"
	"regexp"
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
			case "repos/octo/api/contents":
				return []map[string]string{{"name": "README.md", "type": "file"}}, true
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

func TestRepoNotFoundFails(t *testing.T) {
	h := newHarness(t, "")
	h.env.NewClient = func(string) (discover.Requester, error) {
		// Only the owner's shared repository exists, with a template.
		return fakeContents(func(p string) (any, bool) {
			switch p {
			case "repos/octo/.github/contents":
				return []map[string]string{{"name": "pull_request_template.md", "type": "file"}}, true
			case "repos/octo/.github/contents/pull_request_template.md":
				return file("## Shared\n"), true
			}
			return nil, false
		}), nil
	}
	body := h.write(t, "b.md", "junk")
	for _, args := range [][]string{
		{"check", "--repo", "github.com/octo/missing", "--body-file", body},
		{"check", "--repo", "github.com/octo/missing", "--body-file", body, "--json"},
		{"show", "--repo", "github.com/octo/missing"},
		{"pr", "create", "--repo", "github.com/octo/missing", "--body-file", body},
	} {
		code := h.run(args...)
		if code != ExitUsage {
			t.Errorf("%v: exit %d, want %d\nstdout: %s", args, code, ExitUsage, h.stdout)
		}
		if !strings.Contains(h.stderr.String(), "github.com/octo/missing: repository not found") {
			t.Errorf("%v: stderr = %s", args, h.stderr)
		}
		if strings.Contains(h.stdout.String(), `"ok": true`) || strings.Contains(h.stdout.String(), "PASS") {
			t.Errorf("%v: reported a pass: %s", args, h.stdout)
		}
	}
	if len(h.calls) != 0 {
		t.Errorf("ran gh for a missing repository: %v", h.calls)
	}
}

func TestCheckJSONListsTemplatesWhenAmbiguous(t *testing.T) {
	h := newHarness(t, "issue-mixed")
	body := h.write(t, "b.md", "x")
	for _, args := range [][]string{
		{"check", "issue", "--body-file", body, "--json"},
		{"check", "issue", "--body-file", body, "--json", "--name", "nope"},
	} {
		if code := h.run(args...); code != ExitUsage {
			t.Fatalf("%v: exit %d", args, code)
		}
		var out struct {
			OK        bool   `json:"ok"`
			Error     string `json:"error"`
			Templates []struct {
				ID string `json:"id"`
			} `json:"templates"`
		}
		if err := json.Unmarshal(h.stdout.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.OK || out.Error == "" || len(out.Templates) != 2 || out.Templates[0].ID != "bug_report" {
			t.Errorf("%v: json = %s", args, h.stdout)
		}
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
		body     string
		bodyFile string
		repo     string
	}{
		{name: "long flags", kind: discover.PR, args: []string{"--title", "--body", "--body-file=b.md"}, bodyFile: "b.md"},
		{name: "short cluster", kind: discover.PR, args: []string{"-dwF", "b.md"}, bodyFile: "b.md"},
		{name: "attached value", kind: discover.PR, args: []string{"-Fb.md", "-R=o/r"}, bodyFile: "b.md", repo: "o/r"},
		{name: "body text", kind: discover.Issue, args: []string{"-b", "-F looks like a flag"}, body: "-F looks like a flag"},
		{name: "a value that looks like a flag", kind: discover.Issue, args: []string{"--attach", "--body-file", "--title", "t"}},
		{name: "issue relation flags", kind: discover.Issue, args: []string{"--parent", "-F", "--blocked-by", "-F", "--blocking", "-b"}},
		{name: "unknown flags", kind: discover.PR, args: []string{"--future-flag", "-z"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := scanCreate(tt.kind, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			body := ""
			if c.body != nil {
				body = *c.body
			}
			if body != tt.body || c.bodyFile != tt.bodyFile || c.repo != tt.repo {
				t.Errorf("got body %q, file %q, repo %q", body, c.bodyFile, c.repo)
			}
		})
	}
	for _, bad := range [][]string{{"--title"}, {"-F"}} {
		if _, err := scanCreate(discover.PR, bad); err == nil {
			t.Errorf("scanCreate(%q) returned no error", bad)
		}
	}
}

// TestCreatePassesArgumentsInOrder runs the create commands through the
// generated parser. It checks that the --tmpl-* flags are removed and that
// everything else reaches gh unchanged and in order.
func TestCreatePassesArgumentsInOrder(t *testing.T) {
	h := newHarness(t, "pr-multi")
	body := h.write(t, "b.md", "## Feature\n\nDark mode.\n")
	code := h.run("pr", "create", "--title", "help", "--tmpl-name", "feature", "-dF", body, "--tmpl-offline", "--base=main", "-l", "-x")
	if code != ExitOK || len(h.calls) != 1 {
		t.Fatalf("exit %d, calls %v\n%s", code, h.calls, h.stderr)
	}
	want := []string{"pr", "create", "--title", "help", "-dF", body, "--base=main", "-l", "-x"}
	if got := h.calls[0].args; strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("args = %q, want %q", got, want)
	}

	h.calls = nil
	if code := h.run("pr", "create", "--tmpl-name=bugfix", "--body-file", body, "--tmpl-offline"); code != ExitFailed || len(h.calls) != 0 {
		t.Errorf("--tmpl-name=bugfix: exit %d, calls %v", code, h.calls)
	}
}

// ghValueFlags reads the flags that take a value from "gh <sub> create
// --help". A flag that takes a value has a placeholder after its name.
func ghValueFlags(t *testing.T, sub string) map[string]bool {
	t.Helper()
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh is not installed")
	}
	cmd := exec.Command(ghPath, sub, "create", "--help")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("gh %s create --help failed: %v", sub, err)
	}
	flags := map[string]bool{}
	re := regexp.MustCompile(`^\s+(?:-[A-Za-z], )?--([a-z][a-z-]*) [^\s-]`)
	section := ""
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" && line[0] != ' ' {
			section = line
			continue
		}
		if section != "FLAGS" && section != "INHERITED FLAGS" {
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			flags[m[1]] = true
		}
	}
	if len(flags) == 0 {
		t.Skipf("could not read the flags of gh %s create", sub)
	}
	return flags
}

// TestGhFlagsMatchGhHelp compares the ghFlags table with the gh that is
// installed. A new gh flag that takes a value must be added to ghFlags, or
// a value after it that starts with "-" is read as a flag.
func TestGhFlagsMatchGhHelp(t *testing.T) {
	for kind, sub := range map[discover.Kind]string{discover.PR: "pr", discover.Issue: "issue"} {
		t.Run(sub, func(t *testing.T) {
			want := ghValueFlags(t, sub)
			for name := range want {
				if _, ok := ghFlags[kind][name]; !ok {
					t.Errorf("gh %s create --%s takes a value, but ghFlags does not list it", sub, name)
				}
			}
			for name := range ghFlags[kind] {
				if !want[name] {
					t.Errorf("ghFlags lists --%s, but gh %s create --help does not show it taking a value", name, sub)
				}
			}
		})
	}
}

func TestCompletion(t *testing.T) {
	h := newHarness(t, "")
	for _, shell := range []string{"bash", "zsh", "fish", "nu", "powershell"} {
		if code := h.run("completion", shell); code != ExitOK || !strings.Contains(h.stdout.String(), "__complete_word__") {
			t.Errorf("%s: exit %d, script %q", shell, code, h.stdout)
		}
	}
	if code := h.run("completion", "tcsh"); code != ExitUsage {
		t.Errorf("tcsh: exit %d", code)
	}

	// The script calls the binary back with the line typed so far.
	if code := h.run("__complete_word__", "--shell", "bash", "--line", "gh-tmpl sh"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != "show" {
		t.Errorf("complete subcommand: exit %d, out %q", code, h.stdout)
	}
	if code := h.run("__complete_word__", "--shell", "bash", "--line", "gh-tmpl check --t"); code != ExitOK || !strings.Contains(h.stdout.String(), "--template-file") {
		t.Errorf("complete flag: exit %d, out %q", code, h.stdout)
	}
	if code := h.run("__complete_word__", "--shell", "bash", "--line", "gh-tmpl show i"); code != ExitOK || strings.TrimSpace(h.stdout.String()) != "issue" {
		t.Errorf("complete kind: exit %d, out %q", code, h.stdout)
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
