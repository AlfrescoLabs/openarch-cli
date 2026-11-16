package demo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// fakeReader answers like OpenArch for one user: it returns only the demo
// documents in visible. The leak fields make it return more.
type fakeReader struct {
	visible func(Doc) bool
	// listAll returns every document in lists.
	listAll bool
	// readAll answers 200 to every read.
	readAll bool
	// forbiddenBody is the body of a 403 answer.
	forbiddenBody func(Doc) string
	// vectorAll returns every document in vector search.
	vectorAll bool
	// noDescription leaves the descriptions out of lists.
	noDescription bool
	// skip is a document left out of every list.
	skip string
	// noVectors is the number of vector searches that find nothing, as
	// before the vectors reach the search index.
	noVectors int
	searches  int
}

func fakeID(d Doc) string { return "id-" + d.Name }

func (f *fakeReader) docs(keep func(Doc) bool) []Doc {
	var out []Doc
	for _, d := range Data.Documents {
		if d.Name != f.skip && keep(d) {
			out = append(out, d)
		}
	}
	return out
}

func docJSON(d Doc, description bool) map[string]any {
	m := map[string]any{"sys_id": fakeID(d), "sys_name": d.Name, "sys_title": d.Title, "sys_primaryType": "SysFile",
		"sys_path": d.Path()}
	if description {
		m["sys_description"] = d.Description
	}
	return m
}

func (f *fakeReader) Query(_ context.Context, o api.QueryOptions) (api.QueryResult, error) {
	if !strings.HasPrefix(o.Query, ListQuery) {
		return api.QueryResult{}, errors.New("unexpected query " + o.Query)
	}
	word := ""
	if _, after, ok := strings.Cut(o.Query, "sys_title LIKE '%"); ok {
		word, _, _ = strings.Cut(after, "%'")
	}
	docs := f.docs(func(d Doc) bool {
		return (f.listAll || f.visible(d)) && strings.Contains(d.Title, word)
	})
	var list []map[string]any
	res := api.QueryResult{}
	for _, d := range docs {
		list = append(list, docJSON(d, !f.noDescription))
		res.Documents = append(res.Documents, api.Document{})
		res.Documents[len(res.Documents)-1].ID = fakeID(d)
		res.Documents[len(res.Documents)-1].Name = d.Name
	}
	res.Raw, _ = json.Marshal(map[string]any{"documents": list})
	return res, nil
}

func (f *fakeReader) GetDocumentRaw(_ context.Context, id string) (int, []byte, error) {
	for _, d := range Data.Documents {
		if fakeID(d) != id {
			continue
		}
		if f.readAll || f.visible(d) {
			b, _ := json.Marshal(docJSON(d, true))
			return http.StatusOK, b, nil
		}
		body := `{"status":403,"message":"forbidden"}`
		if f.forbiddenBody != nil {
			body = f.forbiddenBody(d)
		}
		return http.StatusForbidden, []byte(body), nil
	}
	return http.StatusNotFound, nil, nil
}

func (f *fakeReader) SearchEmbeddings(context.Context, []float32, string, string, int64) ([]api.EmbeddingHit, int64, error) {
	f.searches++
	if f.searches <= f.noVectors {
		return nil, 0, nil
	}
	var hits []api.EmbeddingHit
	for _, d := range f.docs(func(d Doc) bool { return f.vectorAll || f.visible(d) }) {
		hits = append(hits, api.EmbeddingHit{DocID: fakeID(d), Score: 0.5, Text: d.EmbeddingText()})
	}
	return hits, int64(len(hits)), nil
}

// fakeServer returns an admin reader and a reader per demo user. change, when
// set, changes the reader of one user.
func fakeServer(user string, change func(*fakeReader)) CheckOptions {
	admin := &fakeReader{visible: func(Doc) bool { return true }}
	o := CheckOptions{Admin: admin, Users: map[string]Reader{}}
	for _, u := range Users {
		r := &fakeReader{visible: func(d Doc) bool { return slices.Contains(d.VisibleTo, u.Name) }}
		if u.Name == user && change != nil {
			change(r)
		}
		o.Users[u.Name] = r
	}
	if user == AdminName && change != nil {
		change(admin)
	}
	o.Embed = func(_ context.Context, texts []string) ([][]float32, error) {
		v := make([][]float32, len(texts))
		for i := range v {
			v[i] = []float32{1}
		}
		return v, nil
	}
	o.EmbeddingType = "t"
	return o
}

func TestCheckPasses(t *testing.T) {
	waited := false
	o := fakeServer("", nil)
	o.Wait = func(context.Context) error { waited = true; return nil }
	rep, err := Check(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Passed || rep.Leaks != 0 || rep.Problems != 0 || len(rep.Failures) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if !waited {
		t.Error("Check did not wait for the search index")
	}
	if len(rep.Users) != 1+len(Users) || rep.Users[0].User != AdminName || rep.Users[0].Reads != len(Data.Documents) {
		t.Errorf("users = %+v", rep.Users)
	}
	perUser := 2 + len(titleWords()) + len(Data.Documents) + len(Questions)
	if rep.Requests != perUser*len(rep.Users) {
		t.Errorf("requests = %d, want %d", rep.Requests, perUser*len(rep.Users))
	}
	var b strings.Builder
	rep.CLIVersion, rep.OpenArchVersion = "1.2.3", "1.0.90"
	if err := rep.WriteText(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "PASS: 4 users, 10 documents") ||
		!strings.Contains(b.String(), "vector search), 0 leaks, 0 problems. CLI for OpenArch 1.2.3, Hyland OpenArch 1.0.90.") {
		t.Errorf("text:\n%s", b.String())
	}
}

func TestCheckFindsLeaks(t *testing.T) {
	merger := "merger-proposal"
	tests := []struct {
		name   string
		user   string
		change func(*fakeReader)
		want   string
	}{
		{"list", "carol", func(r *fakeReader) { r.listAll = true }, "leak: carol, list: got " + merger},
		{"read", "bob", func(r *fakeReader) { r.readAll = true }, "leak: bob, read " + merger + ": HTTP 200, want 403"},
		{"refused read with data", "alice", func(r *fakeReader) {
			r.forbiddenBody = func(d Doc) string { return `{"message":"no access to ` + d.Description + `"}` }
		}, "leak: alice, read " + merger + ": the response has the canary of " + merger},
		{"vector search", "carol", func(r *fakeReader) { r.vectorAll = true }, `leak: carol, vector search "Is there a confidential merger?": got ` + merger},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := Check(context.Background(), fakeServer(tt.user, tt.change))
			if err != nil {
				t.Fatal(err)
			}
			if rep.Passed || rep.Leaks == 0 {
				t.Fatalf("report = %+v", rep)
			}
			if !slices.Contains(rep.Failures, tt.want) {
				t.Errorf("failures do not have %q:\n%s", tt.want, strings.Join(rep.Failures, "\n"))
			}
			var b strings.Builder
			_ = rep.WriteText(&b)
			if !strings.Contains(b.String(), "\nFAIL: 4 users") {
				t.Errorf("text:\n%s", b.String())
			}
		})
	}
}

func TestCheckFindsProblems(t *testing.T) {
	rep, err := Check(context.Background(), fakeServer("bob", func(r *fakeReader) { r.noDescription = true }))
	if err != nil {
		t.Fatal(err)
	}
	// Without descriptions, a canary check proves nothing: it is a problem.
	if rep.Passed || rep.Leaks != 0 || !slices.Contains(rep.Failures, "problem: bob, list: the description of budget-forecast-2027 is missing") {
		t.Errorf("report = %+v", rep)
	}
	rep, err = Check(context.Background(), fakeServer("alice", func(r *fakeReader) { r.skip = "salary-bands-2026" }))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed || !slices.Contains(rep.Failures, "problem: alice, list: salary-bands-2026 is missing") {
		t.Errorf("report = %+v", rep)
	}
}

func TestCheckNeedsTheDemoData(t *testing.T) {
	o := fakeServer(AdminName, func(r *fakeReader) { r.skip = "merger-proposal" })
	_, err := Check(context.Background(), o)
	if output.CodeOf(err) != output.ExitPrerequisite || !strings.Contains(err.Error(), "1 of 10 demo documents are missing") {
		t.Errorf("err = %v", err)
	}
}

func TestTitleWords(t *testing.T) {
	words := titleWords()
	for _, w := range []string{"2026", "plan", "Plan", "office", "Office"} {
		if !slices.Contains(words, w) {
			t.Errorf("%q not in %v", w, words)
		}
	}
	if slices.Contains(words, "") || len(words) != 10 {
		t.Errorf("words = %v", words)
	}
}

func TestCheckWaitsForVectors(t *testing.T) {
	var admin *fakeReader
	o := fakeServer(AdminName, func(r *fakeReader) { r.noVectors = 3; admin = r })
	o.VectorTimeout, o.PollInterval = time.Minute, time.Millisecond
	rep, err := Check(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Passed || admin.searches != 4+len(Questions) {
		t.Errorf("passed = %v, admin searches = %d:\n%s", rep.Passed, admin.searches, strings.Join(rep.Failures, "\n"))
	}
	// Vectors that never come are problems once the wait is over.
	o = fakeServer(AdminName, func(r *fakeReader) { r.noVectors = 1000 })
	o.VectorTimeout, o.PollInterval = 5*time.Millisecond, time.Millisecond
	rep, err = Check(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed || !strings.Contains(strings.Join(rep.Failures, "\n"), "problem: admin, vector search") {
		t.Errorf("report = %+v", rep)
	}
}
