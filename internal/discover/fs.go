package discover

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

// MaxTemplateSize is the largest template file that discovery reads.
// GitHub limits issue and pull request bodies to 65,536 characters, so a
// larger template is not a real template.
const MaxTemplateSize = 256 << 10

// maxDirEntries limits how many entries discovery reads from one folder.
const maxDirEntries = 500

// maxResponseSize limits how much of one API response discovery reads.
// A base64 file of MaxTemplateSize plus the JSON envelope fits well inside it.
const maxResponseSize = 2 << 20

// ErrTooLarge reports a file that is larger than MaxTemplateSize.
var ErrTooLarge = errors.New("file is larger than the template size limit")

// Entry is one item in a folder listing.
type Entry struct {
	Name  string
	IsDir bool
}

// FS is a read-only view of the files in one repository.
// Paths use forward slashes and are relative to the repository root.
// The root itself is "".
type FS interface {
	// ReadDir lists a folder. A missing folder returns an error that
	// matches fs.ErrNotExist.
	ReadDir(ctx context.Context, dir string) ([]Entry, error)
	// ReadFile reads a regular file of at most MaxTemplateSize bytes.
	ReadFile(ctx context.Context, name string) ([]byte, error)
	// Source names the repository for messages, for example "local" or
	// "github.com/octo/.github".
	Source() string
}

// LocalFS reads files from a working tree on disk.
// It uses os.Root, so a symbolic link cannot reach a file outside the tree.
type LocalFS struct {
	root *os.Root
}

// OpenLocal opens the working tree at dir.
func OpenLocal(dir string) (*LocalFS, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &LocalFS{root: root}, nil
}

// Close releases the root handle.
func (l *LocalFS) Close() error { return l.root.Close() }

// Source implements FS.
func (l *LocalFS) Source() string { return "local" }

func localName(name string) string {
	if name == "" {
		return "."
	}
	return name
}

// ReadDir implements FS.
func (l *LocalFS) ReadDir(_ context.Context, dir string) ([]Entry, error) {
	f, err := l.root.Open(localName(dir))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	infos, err := f.ReadDir(maxDirEntries)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		isDir := info.IsDir()
		if info.Type()&fs.ModeSymlink != 0 {
			// Resolve the link inside the root. A link that leaves the
			// root fails here and the entry is skipped.
			st, err := l.root.Stat(path.Join(dir, info.Name()))
			if err != nil {
				continue
			}
			isDir = st.IsDir()
		}
		entries = append(entries, Entry{Name: info.Name(), IsDir: isDir})
	}
	return entries, nil
}

// ReadFile implements FS.
func (l *LocalFS) ReadFile(_ context.Context, name string) ([]byte, error) {
	f, err := l.root.Open(localName(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if st.Size() > MaxTemplateSize {
		return nil, fmt.Errorf("%s: %w", name, ErrTooLarge)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxTemplateSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTemplateSize {
		return nil, fmt.Errorf("%s: %w", name, ErrTooLarge)
	}
	return data, nil
}

// Requester sends one REST request. *api.RESTClient satisfies it.
type Requester interface {
	RequestWithContext(ctx context.Context, method, path string, body io.Reader) (*http.Response, error)
}

// RemoteFS reads files from a repository through the REST contents API.
type RemoteFS struct {
	client Requester
	host   string
	owner  string
	repo   string
	dirs   map[string][]Entry
}

// NewRemote returns a RemoteFS for host/owner/repo.
func NewRemote(client Requester, host, owner, repo string) *RemoteFS {
	return &RemoteFS{client: client, host: host, owner: owner, repo: repo, dirs: map[string][]Entry{}}
}

// Source implements FS.
func (r *RemoteFS) Source() string { return r.host + "/" + r.owner + "/" + r.repo }

type contentItem struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

func (r *RemoteFS) contentsPath(name string) string {
	p := "repos/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.repo) + "/contents"
	if name == "" {
		return p
	}
	for _, seg := range strings.Split(name, "/") {
		p += "/" + url.PathEscape(seg)
	}
	return p
}

func (r *RemoteFS) get(ctx context.Context, name string) ([]byte, error) {
	resp, err := r.client.RequestWithContext(ctx, http.MethodGet, r.contentsPath(name), nil)
	if err != nil {
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseSize {
		return nil, fmt.Errorf("%s: API response is too large", name)
	}
	return data, nil
}

// validName rejects names that could change the meaning of a path.
func validName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return false
	}
	for _, c := range name {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// ReadDir implements FS.
func (r *RemoteFS) ReadDir(ctx context.Context, dir string) ([]Entry, error) {
	if entries, ok := r.dirs[dir]; ok {
		return entries, nil
	}
	data, err := r.get(ctx, dir)
	if err != nil {
		return nil, err
	}
	var items []contentItem
	if err := json.Unmarshal(data, &items); err != nil {
		// A file in place of a folder decodes as an object, not a list.
		return nil, fmt.Errorf("%s: %w", dir, fs.ErrNotExist)
	}
	var entries []Entry
	for _, it := range items {
		if len(entries) >= maxDirEntries {
			break
		}
		if !validName(it.Name) {
			continue
		}
		switch it.Type {
		case "file":
			entries = append(entries, Entry{Name: it.Name})
		case "dir":
			entries = append(entries, Entry{Name: it.Name, IsDir: true})
		}
	}
	r.dirs[dir] = entries
	return entries, nil
}

// ReadFile implements FS.
func (r *RemoteFS) ReadFile(ctx context.Context, name string) ([]byte, error) {
	data, err := r.get(ctx, name)
	if err != nil {
		return nil, err
	}
	var it contentItem
	if err := json.Unmarshal(data, &it); err != nil {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if it.Type != "file" {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if it.Size > MaxTemplateSize {
		return nil, fmt.Errorf("%s: %w", name, ErrTooLarge)
	}
	if it.Encoding != "base64" {
		return nil, fmt.Errorf("%s: unexpected encoding %q", name, it.Encoding)
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(it.Content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(content) > MaxTemplateSize {
		return nil, fmt.Errorf("%s: %w", name, ErrTooLarge)
	}
	return content, nil
}
