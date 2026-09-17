package pack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Install copies a pack folder or zip into gamesDir. Refuses to overwrite.
func Install(src, gamesDir string) (Info, error) {
	st, err := os.Stat(src)
	if err != nil {
		return Info{}, fmt.Errorf("install: %w", err)
	}
	if err := os.MkdirAll(gamesDir, 0o755); err != nil {
		return Info{}, err
	}
	var dir string
	if st.IsDir() {
		dir, err = installDir(src, gamesDir)
	} else {
		dir, err = installZip(src, gamesDir)
	}
	if err != nil {
		return Info{}, err
	}
	p, err := Load(dir)
	if err != nil {
		return Info{}, fmt.Errorf("installed to %s but failed to load: %w", dir, err)
	}
	if err := p.Validate(); err != nil {
		return Info{ID: p.Meta.ID, Title: p.Meta.Title, Dir: dir}, fmt.Errorf("installed to %s but pack is invalid:\n%w", dir, err)
	}
	return Info{ID: p.Meta.ID, Title: p.Meta.Title, Dir: dir}, nil
}

func installDir(src, gamesDir string) (string, error) {
	p, err := Load(src)
	if err != nil {
		return "", fmt.Errorf("not a pack (missing pack.yaml?): %w", err)
	}
	id := p.Meta.ID
	if id == "" {
		id = filepath.Base(src)
	}
	dest := filepath.Join(gamesDir, id)
	if destAbs, err := filepath.Abs(dest); err == nil {
		if srcAbs, err := filepath.Abs(src); err == nil && destAbs == srcAbs {
			return dest, nil
		}
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("games/%s already exists", id)
	}
	if err := copyDir(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func installZip(src, gamesDir string) (string, error) {
	tmp, err := os.MkdirTemp("", "sudengine-pack-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := unzip(src, tmp); err != nil {
		return "", err
	}
	root, err := findPackRoot(tmp)
	if err != nil {
		return "", err
	}
	return installDir(root, gamesDir)
}

func findPackRoot(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "pack.yaml")); err == nil {
		return dir, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cand := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(cand, "pack.yaml")); err == nil {
			return cand, nil
		}
	}
	return "", fmt.Errorf("zip has no pack.yaml at the root or one folder down")
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("zip path escapes: %s", f.Name)
		}
		path := filepath.Join(dest, name)
		if !strings.HasPrefix(path, dest+string(os.PathSeparator)) && path != dest {
			return fmt.Errorf("zip path escapes: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		out := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, info.Mode())
	})
}
