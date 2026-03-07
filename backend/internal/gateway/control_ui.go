package gateway

import (
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

const defaultControlUIIndex = "index.html"

func newControlUIHandler(controlUIDir string, controlUIFS fs.FS, index string) (http.Handler, bool) {
	root, ok := resolveControlUIRoot(controlUIDir, controlUIFS)
	if !ok {
		return nil, false
	}
	indexPath := sanitizeControlUIPath(index)
	if indexPath == "" {
		indexPath = defaultControlUIIndex
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/ui/")
		target, ok := resolveControlUITarget(root, reqPath, indexPath)
		if !ok {
			http.NotFound(w, r)
			return
		}
		serveControlUIFile(w, r, root, target)
	}), true
}

func hasControlUISource(controlUIDir string, controlUIFS fs.FS) bool {
	if controlUIDir != "" {
		return true
	}
	return controlUIFS != nil
}

func resolveControlUIRoot(controlUIDir string, controlUIFS fs.FS) (fs.FS, bool) {
	if controlUIDir != "" {
		return os.DirFS(controlUIDir), true
	}
	if controlUIFS != nil {
		return controlUIFS, true
	}
	return nil, false
}

func resolveControlUITarget(root fs.FS, reqPath, indexPath string) (string, bool) {
	clean := sanitizeControlUIPath(reqPath)
	if clean == "" {
		return indexPath, true
	}

	if info, err := fs.Stat(root, clean); err == nil {
		if info.IsDir() {
			candidate := path.Join(clean, defaultControlUIIndex)
			if controlUIFileExists(root, candidate) {
				return candidate, true
			}
		} else {
			return clean, true
		}
	}

	if path.Ext(clean) == "" && controlUIFileExists(root, indexPath) {
		return indexPath, true
	}
	return "", false
}

func sanitizeControlUIPath(name string) string {
	clean := path.Clean("/" + strings.TrimSpace(name))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." {
		return ""
	}
	return clean
}

func controlUIFileExists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && !info.IsDir()
}

func serveControlUIFile(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	data, err := fs.ReadFile(root, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var modTime time.Time
	if info, err := fs.Stat(root, name); err == nil {
		modTime = info.ModTime()
	}
	http.ServeContent(w, r, path.Base(name), modTime, bytes.NewReader(data))
}
