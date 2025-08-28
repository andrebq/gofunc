package uploader

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

// LoadIgnoreFile reads patterns from a file, returning empty slice on error or missing file.
func LoadIgnoreFile(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Upload srcdir, which should be a go module, to a gofaas server located at gofaasBaseURL
func Upload(ctx context.Context, gofaasBaseURL string, name string, srcdir string) error {
	// Build ignore patterns
	ga := LoadIgnoreFile(filepath.Join(srcdir, ".gofaasignore"))
	gi := LoadIgnoreFile(filepath.Join(srcdir, ".gitignore"))
	patterns := append(ga, gi...)

	// Create zip
	tmp, err := os.CreateTemp("", "gofaas-upload-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp zip: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := CreateZip(tmpPath, srcdir, patterns); err != nil {
		return fmt.Errorf("failed to create zip: %w", err)
	}

	// Upload
	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer f.Close()

	serverURL, err := url.Parse(gofaasBaseURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}
	serverURL.Path = path.Join(serverURL.Path, "_admin", name, "recompile")

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, serverURL.String(), f)
	if err != nil {
		return cli.Exit("failed to create request: "+err.Error(), 1)
	}
	req.Header.Set("Content-Type", "application/zip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return cli.Exit("upload failed: "+err.Error(), 1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return cli.Exit(fmt.Sprintf("upload failed: status=%d body=%s", resp.StatusCode, string(body)), 1)
	}
	return nil
}

// CreateZip walks root and writes files to dest zipPath, skipping patterns.
func CreateZip(zipPath, root string, patterns []string) error {
	zf, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	defer zw.Close()

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	return filepath.WalkDir(rootAbs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		relUnix := toUnixPath(rel)

		// Check ignore
		if shouldIgnore(relUnix, d.IsDir(), patterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			// add directory entry
			_, err := zw.Create(relUnix + "/")
			return err
		}

		fi, err := d.Info()
		if err != nil {
			return err
		}

		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()

		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		hdr.Name = relUnix
		hdr.Method = zip.Deflate

		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
}

func shouldIgnore(rel string, isDir bool, patterns []string) bool {
	// Patterns processed in order; negation (!) un-ignores
	included := true
	for _, p := range patterns {
		neg := false
		pat := p
		if strings.HasPrefix(pat, "!") {
			neg = true
			pat = strings.TrimPrefix(pat, "!")
		}
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}

		// Convert to unix-style for matching
		pat = toUnixPath(pat)

		matched := false
		if strings.HasSuffix(pat, "/") {
			// directory prefix
			prefix := strings.TrimSuffix(pat, "/")
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
				matched = true
			}
		} else if strings.Contains(pat, "/") {
			// match against the path
			ok, _ := path.Match(pat, rel)
			matched = ok
		} else {
			// match against basename
			base := path.Base(rel)
			ok, _ := path.Match(pat, base)
			matched = ok
		}

		if matched {
			if neg {
				included = true
			} else {
				included = false
			}
		}
	}
	return !included
}

func toUnixPath(p string) string {
	return strings.ReplaceAll(p, string(os.PathSeparator), "/")
}
