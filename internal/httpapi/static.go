package httpapi

import (
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

func spa(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(pathpkg.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		file := filepath.Join(root, filepath.FromSlash(name))
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			http.ServeFile(w, r, file)
			return
		}

		index := filepath.Join(root, "index.html")
		if info, err := os.Stat(index); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}
