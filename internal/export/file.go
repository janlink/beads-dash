package export

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath expands ~ and anchors a relative path at dir. A path rooted
// without a volume (/x on Windows) takes the volume of dir.
func ResolvePath(p, dir, home string) string {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"), strings.HasPrefix(p, "~"+string(filepath.Separator)):
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) && dir != "" {
		if rooted(p) {
			p = filepath.VolumeName(dir) + p
		} else {
			p = filepath.Join(dir, p)
		}
	}
	return filepath.Clean(p)
}

func rooted(p string) bool {
	return p != "" && os.IsPathSeparator(p[0]) && filepath.VolumeName(p) == ""
}

// Exists reports whether path is taken.
func Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// FreeName returns path with -1, -2, ... added before the extension until
// nothing has that name.
func FreeName(path string) (string, error) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for n := 1; ; n++ {
		p := fmt.Sprintf("%s-%d%s", base, n, ext)
		taken, err := Exists(p)
		if err != nil {
			return "", err
		}
		if !taken {
			return p, nil
		}
	}
}

// WriteFile writes data through a temporary file in the target directory, so
// nothing is written partially. Overwrite replaces an existing file and keeps
// its permissions; without it the file is created only if nothing has the
// name, which a concurrent writer cannot race. A new file takes the
// permissions the process umask leaves. A missing directory is an error, and
// so is a directory as the target.
func WriteFile(path string, data []byte, overwrite bool) error {
	dir := filepath.Dir(path)
	if st, err := os.Stat(dir); err != nil {
		return err
	} else if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	mode := fs.FileMode(0o666)
	if st, err := os.Stat(path); err == nil {
		if st.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
		if overwrite {
			mode = st.Mode().Perm()
		}
	}
	tmp, name, err := createTemp(dir, filepath.Base(path), mode)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()
	_, err = tmp.Write(data)
	if err == nil && overwrite {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if overwrite {
		return os.Rename(name, path)
	}
	if err := os.Link(name, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s already exists", path)
		}
		return err
	}
	return nil
}

func createTemp(dir, base string, mode fs.FileMode) (*os.File, string, error) {
	for range 8 {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, "."+base+"."+hex.EncodeToString(b[:])+".tmp")
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, name, err
	}
	return nil, "", errors.New("no free temporary file name")
}
