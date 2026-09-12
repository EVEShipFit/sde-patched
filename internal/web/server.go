// Package web serves an editor for the patches.
//
// The SDE is read once and kept. Every request re-reads the patches and
// applies them to a copy, which takes a few tens of milliseconds, so what the
// browser shows is always what is on disk.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"sync"

	"github.com/EVEShipFit/sde-patched/internal/patch"
	"github.com/EVEShipFit/sde-patched/internal/sde"
)

//go:embed static
var static embed.FS

// Server holds the pristine SDE and hands out a freshly patched copy.
type Server struct {
	pristine   *sde.Data
	patchesDir string

	mu      sync.Mutex
	current *state

	// writing keeps two saves from reading the same file and dropping one.
	writing sync.Mutex
}

// state is one good run of the patches: what was declared, and what it
// matched. Working every item out is expensive and the answer only changes
// when the patches do, so it is kept here and thrown away with the run.
type state struct {
	spec *patch.Spec
	ctx  *patch.Context
	data *sde.Data

	mu    sync.Mutex
	swept map[string]map[int32][]reading
}

func New(data *sde.Data, patchesDir string) *Server {
	return &Server{pristine: data, patchesDir: patchesDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/enums", s.handleEnums)

	mux.HandleFunc("GET /api/patch/{name}", s.handlePatch)
	mux.HandleFunc("POST /api/patch/{name}", s.handleCreate)
	mux.HandleFunc("DELETE /api/patch/{name}", s.handleDelete)
	mux.HandleFunc("PUT /api/patch/{name}/notes", s.handleNotes)
	mux.HandleFunc("PUT /api/patch/{name}/define/{section}", s.handleDefine)
	mux.HandleFunc("POST /api/patch/{name}/{section}", s.handleInsert)
	mux.HandleFunc("PUT /api/patch/{name}/{section}/{index}", s.handleReplace)
	mux.HandleFunc("DELETE /api/patch/{name}/{section}/{index}", s.handleRemove)

	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/complete", s.handleComplete)
	mux.HandleFunc("GET /api/preview", s.handlePreview)

	mux.HandleFunc("GET /api/type/{id}", s.handleType)
	mux.HandleFunc("GET /api/attribute/{name}", s.handleAttribute)
	mux.HandleFunc("GET /api/effect/{name}", s.handleEffect)
	mux.HandleFunc("GET /api/selector/{name}", s.handleSelector)
	mux.HandleFunc("GET /api/group/{id}", s.handleGroup)
	mux.HandleFunc("GET /api/category/{id}", s.handleCategory)

	mux.HandleFunc("GET /api/sheets", s.handleSheets)
	mux.HandleFunc("GET /api/sheet/{name}", s.handleSheet)

	mux.HandleFunc("GET /api/live/{id}", s.handleLive)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/health/{name}", s.handleHealthAttribute)

	pages, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", fresh(http.FileServer(http.FS(pages))))

	return mux
}

// fresh stops the browser holding on to a page from before a rebuild. It is
// all embedded and small, so caching it gains nothing.
func fresh(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// reload runs the patches again. It is cheap enough to do on every request,
// which saves having to watch the files for changes.
//
// A failed run keeps the last good one, so the rest of the editor still works
// while the file being edited is half-written. Both are returned; the state is
// nil only when nothing has ever read.
func (s *Server) reload() (*state, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	spec, err := patch.Load(s.patchesDir)
	if err == nil {
		err = spec.Validate()
	}
	if err != nil {
		return s.current, err
	}

	data := s.pristine.Clone()
	ctx, err := patch.Apply(spec, data)
	if err != nil {
		return s.current, err
	}

	s.current = &state{spec: spec, ctx: ctx, data: data, swept: map[string]map[int32][]reading{}}
	return s.current, nil
}

// write encodes before sending anything, so a body that cannot be encoded
// still becomes an error rather than half a response with a 200.
func write(w http.ResponseWriter, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(append(raw, '\n'))
}

func fail(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// settings reads the two toggles every worked-out answer depends on. Both are
// on unless turned off.
func settings(r *http.Request) (published, active bool) {
	query := r.URL.Query()
	return query.Get("published") != "0", query.Get("active") != "0"
}

// handleState is what the editor asks for first: which SDE it is working on,
// and whether the patches still read.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	_, err := s.reload()

	body := map[string]any{
		"build": s.pristine.BuildNumber,
		"types": len(s.pristine.Types),
	}
	if err != nil {
		body["error"] = err.Error()
	}
	write(w, body)
}

func (s *Server) handleEnums(w http.ResponseWriter, r *http.Request) {
	write(w, patch.Enums())
}
