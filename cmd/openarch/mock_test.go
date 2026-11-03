package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Credentials accepted by the mock OpenArch.
const (
	mockUser     = "admin"
	mockPassword = "Mock-pass-1"
	mockRootID   = "00000000-0000-0000-0000-000000000000"
	mockFolderID = "11111111-1111-1111-1111-111111111111"
)

func mockDoc(id, typ, path, title string) map[string]any {
	name := path[strings.LastIndex(path, "/")+1:]
	return map[string]any{"sys_id": id, "sys_primaryType": typ, "sys_path": path, "sys_name": name,
		"sys_title": title, "sys_creator": map[string]any{"username": mockUser}, "custom_field": "kept"}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter, what string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"entity-type": "exception", "status": 404, "message": what})
}

// newMockOpenArch serves the parts of the OpenArch API that the CLI uses.
func newMockOpenArch() *httptest.Server {
	docs := map[string]map[string]any{
		"/":         mockDoc(mockRootID, "SysRoot", "/", ""),
		"/projects": mockDoc(mockFolderID, "SysFolder", "/projects", "Projects"),
	}
	byID := func(id string) map[string]any {
		for _, d := range docs {
			if d["sys_id"] == id {
				return d
			}
		}
		return nil
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v" {
			writeJSON(w, http.StatusOK, map[string]string{"version": "1.0.90"})
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != mockUser || p != mockPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/user":
			writeJSON(w, http.StatusOK, []map[string]string{{"username": r.URL.Query().Get("name")}})
		case r.URL.Path == "/api/query" && r.Method == http.MethodPost:
			var q struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			_ = json.NewDecoder(r.Body).Decode(&q)
			if !strings.HasPrefix(q.Query, "SELECT") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "message": "missing 'SELECT'"})
				return
			}
			list := []map[string]any{docs["/projects"]}
			writeJSON(w, http.StatusOK, map[string]any{"query": q.Query, "limit": q.Limit, "offset": 0,
				"documents": list, "count": len(list), "totalCount": len(list), "sync": r.URL.Query().Get("sync")})
		case strings.HasPrefix(r.URL.Path, "/api/documents/path/"):
			// The server needs "/" as is: an escaped %2F is a 400 in Tomcat.
			if strings.Contains(r.URL.RawPath, "%2F") {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			path := "/" + strings.TrimPrefix(r.URL.Path, "/api/documents/path/")
			parent, ok := docs[path]
			if !ok {
				notFound(w, path)
				return
			}
			if r.Method == http.MethodPost {
				create(w, r, docs, parent)
				return
			}
			writeJSON(w, http.StatusOK, parent)
		case strings.HasPrefix(r.URL.Path, "/api/documents/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/documents/")
			d := byID(id)
			if d == nil {
				notFound(w, id)
				return
			}
			if r.Method == http.MethodPost {
				create(w, r, docs, d)
				return
			}
			writeJSON(w, http.StatusOK, d)
		default:
			http.NotFound(w, r)
		}
	}))
}

func create(w http.ResponseWriter, r *http.Request, docs map[string]map[string]any, parent map[string]any) {
	data, _ := io.ReadAll(r.Body)
	var body map[string]any
	if json.Unmarshal(data, &body) != nil || body["sys_primaryType"] == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "message": "sys_primaryType is required"})
		return
	}
	name, _ := body["sys_name"].(string)
	path := strings.TrimSuffix(parent["sys_path"].(string), "/") + "/" + name
	title, _ := body["sys_title"].(string)
	d := mockDoc("22222222-2222-2222-2222-222222222222", body["sys_primaryType"].(string), path, title)
	docs[path] = d
	writeJSON(w, http.StatusOK, d)
}
