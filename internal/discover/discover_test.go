package discover

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func openFixture(t *testing.T, name string) *LocalFS {
	t.Helper()
	l, err := OpenLocal(filepath.Join("testdata", "repos", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func ids(ts []Template) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Path)
	}
	return out
}

func TestFindIn(t *testing.T) {
	tests := []struct {
		repo     string
		kind     Kind
		paths    []string
		shadowed []string
		problems int
	}{
		{repo: "pr-single", kind: PR, paths: []string{".github/pull_request_template.md"}},
		{
			repo:     "pr-precedence",
			kind:     PR,
			paths:    []string{".github/pull_request_template.md"},
			shadowed: []string{"pull_request_template.md", "docs/PULL_REQUEST_TEMPLATE.md"},
		},
		{
			repo: "pr-multi",
			kind: PR,
			// Folder templates are sorted by name. YAML files are not pull
			// request templates.
			paths: []string{
				".github/PULL_REQUEST_TEMPLATE/bugfix.md",
				".github/PULL_REQUEST_TEMPLATE/feature.md",
				"docs/pull_request_template.md",
			},
		},
		{
			repo: "issue-mixed",
			kind: Issue,
			paths: []string{
				".github/ISSUE_TEMPLATE/bug_report.yml",
				".github/ISSUE_TEMPLATE/feature_request.md",
			},
			problems: 1, // broken.yml
		},
		{repo: "issue-mixed", kind: PR},
		{repo: "empty", kind: PR},
		{repo: "root-upper", kind: PR, paths: []string{"PULL_REQUEST_TEMPLATE.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.repo+"/"+string(tt.kind), func(t *testing.T) {
			res, err := FindIn(context.Background(), openFixture(t, tt.repo), tt.kind)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(res.Templates); strings.Join(got, ",") != strings.Join(tt.paths, ",") {
				t.Errorf("templates = %v, want %v", got, tt.paths)
			}
			if strings.Join(res.Shadowed, ",") != strings.Join(tt.shadowed, ",") {
				t.Errorf("shadowed = %v, want %v", res.Shadowed, tt.shadowed)
			}
			if len(res.Problems) != tt.problems {
				t.Errorf("problems = %v, want %d", res.Problems, tt.problems)
			}
		})
	}
}

func TestLoadParsesFrontMatterAndForms(t *testing.T) {
	res, err := FindIn(context.Background(), openFixture(t, "issue-mixed"), Issue)
	if err != nil {
		t.Fatal(err)
	}
	form, md := res.Templates[0], res.Templates[1]
	if form.Format != Form || form.Name != "Bug report" || form.Form == nil {
		t.Errorf("form template = %+v", form)
	}
	if !strings.HasPrefix(form.Body, "### What happened?\n") {
		t.Errorf("form body starts with %q", form.Body[:30])
	}
	if md.Format != Markdown || md.Name != "Feature request" || md.About != "Suggest an idea." {
		t.Errorf("markdown template = %+v", md)
	}
	if strings.Contains(md.Body, "---") || !strings.HasPrefix(md.Body, "## Problem") {
		t.Errorf("front matter not removed: %q", md.Body)
	}
	if !bytes.HasPrefix(md.Raw, []byte("---\n")) {
		t.Errorf("raw content lost the front matter")
	}
}

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		in, name, body string
	}{
		{"---\nname: A\n---\nbody\n", "A", "body\n"},
		{"---\nname: A\n---\n\n\nbody", "A", "body"},
		{"---\nname: A\n---", "A", ""},
		{"no front matter\n---\n", "", "no front matter\n---\n"},
		{"---\nname: [unclosed\n---\nbody", "", "---\nname: [unclosed\n---\nbody"},
		{"---\nname: A\n---x\nbody", "", "---\nname: A\n---x\nbody"},
		{"---\nonly an opening line\n", "", "---\nonly an opening line\n"},
	}
	for _, tt := range tests {
		fm, body := splitFrontMatter(tt.in)
		if fm.Name != tt.name || body != tt.body {
			t.Errorf("splitFrontMatter(%q) = %q, %q; want %q, %q", tt.in, fm.Name, body, tt.name, tt.body)
		}
	}
}

func TestSelect(t *testing.T) {
	multi, err := FindIn(context.Background(), openFixture(t, "pr-multi"), PR)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := FindIn(context.Background(), openFixture(t, "issue-mixed"), Issue)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		res     Result
		pick    string
		want    string
		wantErr error
	}{
		{"pr default", multi, "", "docs/pull_request_template.md", nil},
		{"pr by id", multi, "feature", ".github/PULL_REQUEST_TEMPLATE/feature.md", nil},
		{"pr by file name, any case", multi, "BUGFIX.md", ".github/PULL_REQUEST_TEMPLATE/bugfix.md", nil},
		{"issue ambiguous", issues, "", "", ErrAmbiguous},
		{"issue by form name", issues, "bug report", ".github/ISSUE_TEMPLATE/bug_report.yml", nil},
		{"issue by front matter name", issues, "Feature request", ".github/ISSUE_TEMPLATE/feature_request.md", nil},
		{"no match", issues, "nope", "", ErrNoMatch},
		{"none", Result{Kind: PR}, "", "", ErrNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Select(tt.res, tt.pick)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.Path != tt.want {
				t.Errorf("path = %q, want %q", got.Path, tt.want)
			}
		})
	}
}

func TestParseKind(t *testing.T) {
	for in, want := range map[string]Kind{"pr": PR, "PR": PR, "pull-request": PR, "issue": Issue, "issues": Issue} {
		if got, err := ParseKind(in); err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseKind("discussion"); err == nil {
		t.Error("ParseKind(discussion) returned no error")
	}
}

func TestLocalFSRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need extra rights on Windows")
	}
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(repo, ".github", "pull_request_template.md")); err != nil {
		t.Fatal(err)
	}
	l, err := OpenLocal(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	res, err := FindIn(context.Background(), l, PR)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Templates) != 0 {
		t.Fatalf("followed a link out of the repository: %+v", res.Templates)
	}
	if _, err := l.ReadFile(context.Background(), ".github/pull_request_template.md"); err == nil {
		t.Fatal("ReadFile followed a link out of the repository")
	}
}

func TestLocalFSSizeLimit(t *testing.T) {
	repo := t.TempDir()
	big := bytes.Repeat([]byte("a"), MaxTemplateSize+1)
	if err := os.WriteFile(filepath.Join(repo, "pull_request_template.md"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := OpenLocal(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.ReadFile(context.Background(), "pull_request_template.md"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	res, err := FindIn(context.Background(), l, PR)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Templates) != 0 || len(res.Problems) != 1 {
		t.Fatalf("templates = %v, problems = %v", res.Templates, res.Problems)
	}
}

// fakeAPI serves a fixture folder the way the REST contents API does.
type fakeAPI struct {
	t        *testing.T
	root     string // fixture folder
	prefix   string // "repos/OWNER/REPO/contents"
	requests []string
}

func (f *fakeAPI) RequestWithContext(_ context.Context, method, p string, _ io.Reader) (*http.Response, error) {
	f.requests = append(f.requests, method+" "+p)
	rel, ok := strings.CutPrefix(p, f.prefix)
	if !ok {
		return nil, notFound(p)
	}
	rel = strings.TrimPrefix(rel, "/")
	unescaped, err := url.PathUnescape(rel)
	if err != nil {
		return nil, err
	}
	full := filepath.Join(f.root, filepath.FromSlash(unescaped))
	st, err := os.Stat(full)
	if err != nil {
		return nil, notFound(p)
	}
	var body any
	if st.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			return nil, err
		}
		var items []map[string]any
		for _, e := range entries {
			typ := "file"
			if e.IsDir() {
				typ = "dir"
			}
			items = append(items, map[string]any{"name": e.Name(), "type": typ, "path": path.Join(unescaped, e.Name())})
		}
		body = items
	} else {
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		body = map[string]any{
			"type": "file", "name": st.Name(), "size": len(data), "encoding": "base64",
			"content": base64.StdEncoding.EncodeToString(data),
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func notFound(p string) error {
	return &api.HTTPError{StatusCode: http.StatusNotFound, Message: "Not Found", RequestURL: &url.URL{Path: p}}
}

func TestRemoteFS(t *testing.T) {
	api := &fakeAPI{t: t, root: filepath.Join("testdata", "repos", "issue-mixed"), prefix: "repos/octo/.github/contents"}
	r := NewRemote(api, "github.com", "octo", ".github")
	res, err := FindIn(context.Background(), r, Issue)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(res.Templates); len(got) != 2 {
		t.Fatalf("templates = %v", got)
	}
	if res.Templates[0].Source != "github.com/octo/.github" {
		t.Errorf("source = %q", res.Templates[0].Source)
	}
	// The root listing is cached, so a second search does not list it again.
	before := len(api.requests)
	if _, err := r.ReadDir(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if len(api.requests) != before {
		t.Errorf("root listing was not cached")
	}
}

func TestFindFallsBackToSecondSource(t *testing.T) {
	local := openFixture(t, "empty")
	remote := NewRemote(&fakeAPI{t: t, root: filepath.Join("testdata", "repos", "pr-single"), prefix: "repos/octo/.github/contents"}, "ghe.example.com", "octo", ".github")
	opened := 0
	res, err := Find(context.Background(), PR,
		func() (FS, error) { return local, nil },
		func() (FS, error) { opened++; return remote, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if opened != 1 || len(res.Templates) != 1 || res.Source != "ghe.example.com/octo/.github" {
		t.Fatalf("opened = %d, result = %+v", opened, res)
	}

	// A source with templates stops the search before the next is opened.
	opened = 0
	if _, err := Find(context.Background(), PR,
		func() (FS, error) { return openFixture(t, "pr-single"), nil },
		func() (FS, error) { opened++; return remote, nil },
	); err != nil {
		t.Fatal(err)
	}
	if opened != 0 {
		t.Errorf("opened the fallback source although the first had templates")
	}

	// A missing first source is an error. A missing fallback is not.
	missing := NewRemote(staticAPIErr{}, "github.com", "octo", "missing")
	if _, err := Find(context.Background(), PR, func() (FS, error) { return missing, nil }); !errors.Is(err, ErrRepoNotFound) {
		t.Errorf("missing first source: err = %v, want ErrRepoNotFound", err)
	}
	res, err = Find(context.Background(), PR,
		func() (FS, error) { return local, nil },
		func() (FS, error) { return missing, nil },
	)
	if err != nil || len(res.Templates) != 0 {
		t.Errorf("missing fallback: %+v, %v", res, err)
	}

	// A nil source is skipped.
	res, err = Find(context.Background(), PR, func() (FS, error) { return nil, nil })
	if err != nil || len(res.Templates) != 0 {
		t.Errorf("nil source: %+v, %v", res, err)
	}
}

func TestRemoteFSRejectsBadResponses(t *testing.T) {
	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), 10))
	tests := map[string]string{
		"too large":     `{"type":"file","size":999999999,"encoding":"base64","content":""}`,
		"symlink":       `{"type":"symlink","size":10,"target":"/etc/passwd"}`,
		"bad encoding":  `{"type":"file","size":10,"encoding":"none","content":"aaaaaaaaaa"}`,
		"bad base64":    `{"type":"file","size":10,"encoding":"base64","content":"!!!"}`,
		"a folder list": `[{"type":"file","name":"x"}]`,
		"fine":          `{"type":"file","size":10,"encoding":"base64","content":"` + big + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			r := NewRemote(staticAPI(body), "github.com", "o", "r")
			_, err := r.ReadFile(context.Background(), "x")
			if (err == nil) != (name == "fine") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRemoteFSSkipsUnsafeNames(t *testing.T) {
	r := NewRemote(staticAPI(`[
		{"type":"file","name":"ok.md"},
		{"type":"file","name":"../escape.md"},
		{"type":"file","name":".."},
		{"type":"dir","name":"a/b"},
		{"type":"file","name":"bell\u0007.md"},
		{"type":"symlink","name":"link.md"},
		{"type":"submodule","name":"sub"}
	]`), "github.com", "o", "r")
	entries, err := r.ReadDir(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "ok.md" {
		t.Fatalf("entries = %+v", entries)
	}
}

// staticAPIErr answers every request with 404 Not Found.
type staticAPIErr struct{}

func (staticAPIErr) RequestWithContext(_ context.Context, _, p string, _ io.Reader) (*http.Response, error) {
	return nil, notFound(p)
}

type staticAPI string

func (s staticAPI) RequestWithContext(context.Context, string, string, io.Reader) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(s)))}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestRemoteFSWithGoGh runs RemoteFS on a real go-gh client to check the
// URLs it builds, including the /api/v3 prefix of GitHub Enterprise Server.
func TestRemoteFSWithGoGh(t *testing.T) {
	tests := map[string]string{
		"github.com":      "https://api.github.com/repos/my%20org/.github/contents/.github",
		"ghe.example.com": "https://ghe.example.com/api/v3/repos/my%20org/.github/contents/.github",
	}
	for host, want := range tests {
		t.Run(host, func(t *testing.T) {
			var got string
			client, err := api.NewRESTClient(api.ClientOptions{
				Host:         host,
				AuthToken:    "test-token",
				LogIgnoreEnv: true,
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					got = r.URL.String()
					return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewRemote(client, host, "my org", ".github").ReadDir(context.Background(), ".github")
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("err = %v, want fs.ErrNotExist", err)
			}
			if got != want {
				t.Errorf("URL = %s, want %s", got, want)
			}
		})
	}
}

func TestFindKeepsProblemsWhenFallingBack(t *testing.T) {
	shared := NewRemote(&fakeAPI{t: t, root: filepath.Join("testdata", "repos", "shared-github"), prefix: "repos/octo/.github/contents"}, "github.com", "octo", ".github")

	tooLarge := t.TempDir()
	if err := os.WriteFile(filepath.Join(tooLarge, "pull_request_template.md"), bytes.Repeat([]byte("a"), MaxTemplateSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	big, err := OpenLocal(tooLarge)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = big.Close() })

	tests := []struct {
		name    string
		local   *LocalFS
		kind    Kind
		path    string
		problem string
	}{
		{"invalid form", openFixture(t, "issue-broken"), Issue, "ISSUE_TEMPLATE/bug.md", ".github/ISSUE_TEMPLATE/bug_report.yml: "},
		{"too large", big, PR, "pull_request_template.md", "pull_request_template.md: " + ErrTooLarge.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Find(context.Background(), tt.kind,
				func() (FS, error) { return tt.local, nil },
				func() (FS, error) { return shared, nil },
			)
			if err != nil {
				t.Fatal(err)
			}
			if res.Source != "github.com/octo/.github" || len(res.Templates) != 1 || res.Templates[0].Path != tt.path {
				t.Fatalf("result = %+v", res)
			}
			if len(res.Problems) != 1 || res.Problems[0].Source != "local" || !strings.HasPrefix(res.Problems[0].Message, tt.problem) {
				t.Fatalf("problems = %+v, want one in local starting with %q", res.Problems, tt.problem)
			}
		})
	}
}
