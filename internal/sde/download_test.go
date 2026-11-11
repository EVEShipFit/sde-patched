package sde

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func fakeServer(t *testing.T) *atomic.Int32 {
	t.Helper()

	rows := map[string]string{
		"_sde":  `{"key": "sde", "buildNumber": 100, "releaseDate": "2026-11-10T08:00:00Z"}`,
		"types": `{"typeID": 587, "groupID": 25, "name": {"en": "Rifter"}}`,
	}

	var requests atomic.Int32
	mux := http.NewServeMux()
	var server *httptest.Server

	release := func(name string, build int32) map[string]any {
		index := map[string]any{}
		for _, table := range tables {
			index[table] = map[string]any{"jsonl": map[string]string{
				"url": fmt.Sprintf("%s/tranquility/%s/%s.jsonl", server.URL, name, table),
			}}
		}
		return map[string]any{"release": name, "buildNumber": build, "tables": index}
	}
	writeJSON := func(w http.ResponseWriter, v any) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Error(err)
		}
	}

	mux.HandleFunc("/tranquility/{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"releases": []map[string]string{
			{"releaseUrl": server.URL + "/tranquility/new/"},
			{"releaseUrl": server.URL + "/tranquility/old/"},
			{"releaseUrl": server.URL + "/tranquility/broken/"},
		}})
	})
	mux.HandleFunc("/tranquility/latest/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, release("new", 200))
	})
	mux.HandleFunc("/tranquility/new/{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, release("new", 200))
	})
	mux.HandleFunc("/tranquility/old/{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, release("old", 100))
	})
	mux.HandleFunc("/tranquility/broken/{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, release("broken", 50))
	})
	mux.HandleFunc("/tranquility/old/{table}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/blobs/"+r.PathValue("table"), http.StatusFound)
	})
	mux.HandleFunc("/blobs/{table}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "gzip" {
			http.Error(w, "gzip only", http.StatusNotAcceptable)
			return
		}

		var body bytes.Buffer
		writer := gzip.NewWriter(&body)
		writer.Write([]byte(rows[strings.TrimSuffix(r.PathValue("table"), ".jsonl")] + "\n"))
		writer.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Write(body.Bytes())
	})

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	previous := baseURL
	baseURL = server.URL
	t.Cleanup(func() { baseURL = previous })
	return &requests
}

func TestLatestBuild(t *testing.T) {
	fakeServer(t)

	build, err := LatestBuild("tranquility")
	if err != nil {
		t.Fatal(err)
	}
	if build != 200 {
		t.Errorf("build = %d", build)
	}
}

func TestDownloadAndLoad(t *testing.T) {
	requests := fakeServer(t)
	dir := t.TempDir()

	path, err := Download(dir, "tranquility", 100)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "100") {
		t.Errorf("path = %s", path)
	}
	if build := LocalBuild(dir); build != 100 {
		t.Errorf("LocalBuild = %d", build)
	}

	before := requests.Load()
	if _, err := Download(dir, "tranquility", 100); err != nil {
		t.Fatal(err)
	}
	if after := requests.Load(); after != before {
		t.Errorf("downloaded again: %d requests", after-before)
	}

	data, err := Load(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if data.ReleaseDate.IsZero() {
		t.Error("release date not read")
	}
	if item := data.Types[587]; item == nil || item.Name.En != "Rifter" {
		t.Errorf("type 587 = %+v", item)
	}
}

func TestDownloadUnknownBuild(t *testing.T) {
	fakeServer(t)

	if _, err := Download(t.TempDir(), "tranquility", 300); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDownloadInterrupted(t *testing.T) {
	fakeServer(t)
	dir := t.TempDir()

	if _, err := Download(dir, "tranquility", 50); err == nil {
		t.Fatal("expected an error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("left behind %d entries", len(entries))
	}
}
