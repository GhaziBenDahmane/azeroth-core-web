package web

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesRouteIndexes(t *testing.T) {
	root := fstest.MapFS{
		"index.html":        {Data: []byte("home")},
		"armory/index.html": {Data: []byte("armory")},
		"app.js":            {Data: []byte("script")},
	}
	h := spaHandler(fs.FS(root))
	for route, want := range map[string]string{"/": "home", "/armory": "armory", "/armory/": "armory", "/app.js": "script"} {
		r := httptest.NewRequest("GET", route, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != want {
			t.Errorf("%s: got %d %q, want 200 %q", route, w.Code, w.Body.String(), want)
		}
	}
}
