package router

import (
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mewmail/api/internal/httputil"
)

func mountSwagger(r interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Handle(pattern string, handler http.Handler)
}) {
	r.Get("/swagger", swaggerIndexHandler)
	r.Get("/swagger/", swaggerIndexHandler)
	sub, err := fs.Sub(swaggerFS, "static/swagger")
	if err != nil {
		panic(err)
	}
	r.Handle("/swagger/static/*", http.StripPrefix("/swagger/static/", http.FileServer(http.FS(sub))))
	r.Get("/swagger/openapi.yaml", openAPIHandler)
}

func swaggerIndexHandler(w http.ResponseWriter, _ *http.Request) {
	data, err := swaggerFS.ReadFile("static/swagger/index.html")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "swagger ui not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", swaggerCSP)
	body := strings.Replace(string(data), "__V__", strconv.FormatInt(time.Now().Unix(), 10), 1)
	_, _ = w.Write([]byte(body))
}

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(openAPIFS, "openapi.yaml")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(patchOpenAPIServers(data, requestOrigin(r)))
}

const swaggerCSP = "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; img-src 'self' data: https://cdn.jsdelivr.net; font-src 'self' https://cdn.jsdelivr.net; connect-src 'self'"

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func patchOpenAPIServers(data []byte, origin string) []byte {
	const marker = "servers:\n"
	s := string(data)
	i := strings.Index(s, marker)
	if i < 0 {
		return data
	}
	insert := fmt.Sprintf("  - url: %s/api\n    description: Current browser\n", origin)
	at := i + len(marker)
	return []byte(s[:at] + insert + s[at:])
}
