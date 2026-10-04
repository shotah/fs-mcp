package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CleanRoot resolves --root to a real directory. The returned path is the jail.
func CleanRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("--root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("root is not a directory")
	}
	return resolved, nil
}

// Resolve maps p into root. Missing path components are allowed when
// allowMissing is set, so create can jail a file that does not exist yet.
// A leading slash that is not already inside root is the workspace root:
// /c.txt is <root>/c.txt. Symlinks are followed and must stay inside root.
func Resolve(root, p string, allowMissing bool) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", errors.New("invalid path")
	}
	p = strings.TrimSpace(p)
	if p == "" {
		p = "."
	}
	requested := p
	p = slashFromRoot(root, p)
	var lexical string
	if filepath.IsAbs(p) {
		lexical = filepath.Clean(p)
	} else {
		lexical = filepath.Clean(filepath.Join(root, p))
	}
	if err := inside(root, lexical); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, lexical)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return root, nil
	}

	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", errors.New("path escapes workspace")
		}
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) && allowMissing {
				dest := filepath.Join(cur, filepath.Join(parts[i:]...))
				if err := inside(root, dest); err != nil {
					return "", err
				}
				return dest, nil
			}
			if os.IsNotExist(err) {
				return "", missingPath(cur, part, requested)
			}
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(next)
			if err != nil {
				return "", err
			}
			if err := inside(root, resolved); err != nil {
				return "", err
			}
			cur = resolved
			continue
		}
		cur = next
	}
	if err := inside(root, cur); err != nil {
		return "", err
	}
	return cur, nil
}

// slashFromRoot leaves an absolute path that is already inside root alone.
// Any other absolute path drops its leading slash and is resolved from root.
// ".." in that relative form still has to pass the jail.
func slashFromRoot(root, p string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	if inside(root, filepath.Clean(p)) == nil {
		return filepath.Clean(p)
	}
	rel := strings.TrimLeft(p, `/\`)
	if rel == "" {
		return "."
	}
	return rel
}

func missingPath(dir, name, requested string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no such file %s", requested)
	}
	match := ""
	n := 0
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			n++
			match = e.Name()
		}
	}
	if n == 1 {
		return fmt.Errorf("no such file %s; did you mean %s?", requested, match)
	}
	return fmt.Errorf("no such file %s", requested)
}

func inside(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return errors.New("path escapes workspace")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("path escapes workspace")
	}
	return nil
}

// Rel returns p relative to root, using forward slashes.
func Rel(root, abs string) (string, error) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if err := inside(root, abs); err != nil {
		return "", err
	}
	if strings.ContainsAny(rel, "\r\n") {
		return "", errors.New("invalid path")
	}
	return filepath.ToSlash(rel), nil
}
