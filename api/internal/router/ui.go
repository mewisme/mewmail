package router

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mewmail/api/internal/httputil"
	"mewmail/api/internal/models"
)

// ponytail: pinned CDN libs; bump versions here only
const (
	cdnPicoCSS     = "https://cdn.jsdelivr.net/npm/@picocss/pico@2.0.6/css/pico.min.css"
	cdnDOMPurifyJS = "https://cdn.jsdelivr.net/npm/dompurify@3.2.4/dist/purify.min.js"
	cdnDayjsJS     = "https://cdn.jsdelivr.net/npm/dayjs@1.11.13/dayjs.min.js"
)

func cdnHead() string {
	return `<link rel="preconnect" href="https://cdn.jsdelivr.net" crossorigin>
<link rel="stylesheet" href="` + cdnPicoCSS + `" crossorigin>
<link rel="preload" href="/ui/static/ui.css?v=__V__" as="style">
<link rel="stylesheet" href="/ui/static/ui.css?v=__V__">`
}

func cdnScripts() string {
	return `<script defer src="` + cdnDOMPurifyJS + `" crossorigin></script>
<script defer src="` + cdnDayjsJS + `" crossorigin></script>`
}

func injectUIHTML(data []byte) []byte {
	v := strconv.FormatInt(time.Now().Unix(), 10)
	s := string(data)
	s = strings.Replace(s, "__CDN_HEAD__", cdnHead(), 1)
	s = strings.Replace(s, "__CDN_SCRIPTS__", cdnScripts(), 1)
	return []byte(strings.ReplaceAll(s, "__V__", v))
}

func mountUI(r interface {
	Get(pattern string, handlerFn http.HandlerFunc)
	Handle(pattern string, handler http.Handler)
}) {
	r.Get("/", uiIndexHandler)
	sub, err := fs.Sub(uiFS, "static/ui")
	if err != nil {
		panic(err)
	}
	r.Handle("/ui/static/*", uiStaticHandler(http.StripPrefix("/ui/static/", http.FileServer(http.FS(sub)))))
}

func uiStaticHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func uiIndexHandler(w http.ResponseWriter, _ *http.Request) {
	data, err := uiFS.ReadFile("static/ui/index.html")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "ui not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", uiCSP)
	_, _ = w.Write(injectUIHTML(data))
}

func servePreviewHTML(w http.ResponseWriter, email *models.Email) {
	data, err := uiFS.ReadFile("static/ui/preview.html")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "preview ui not found")
		return
	}
	emailJSON, err := json.Marshal(email)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to encode email")
		return
	}
	// ponytail: keep </script> out of embedded JSON
	emailJSON = bytes.ReplaceAll(emailJSON, []byte("<"), []byte(`\u003c`))
	body := injectUIHTML(data)
	body = bytes.Replace(body, []byte("__EMAIL__"), emailJSON, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", previewCSP)
	_, _ = w.Write(body)
}

const uiCSP = "default-src 'none'; script-src 'self' https:; style-src 'self' 'unsafe-inline' https:; font-src 'self' https:; connect-src 'self'; img-src data: https:; frame-src 'self'"

const previewCSP = "default-src 'none'; script-src 'self' https:; style-src 'self' 'unsafe-inline' https:; font-src 'self' https:; img-src data: https: http:; frame-src 'self'"