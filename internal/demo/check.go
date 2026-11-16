package demo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AlfrescoLabs/openarch-cli/internal/api"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// Questions are the texts of the vector search part of the check. Their
// nearest documents are in different permission zones.
var Questions = []string{
	"How much is the pay for each job level?",
	"When is the office closed?",
	"Who moves to the new floor first?",
	"What does a contractor cost per day?",
	"Is there a confidential merger?",
}

// ListQuery selects every demo document.
const ListQuery = "SELECT * FROM SysFile WHERE sys_path STARTSWITH '" + Root + "/'"

// checkLimit is the page size of the check queries: more than the number of
// demo documents, so that one page holds every document a user may read.
const checkLimit = 50

// Reader is the part of the API client that Check uses, logged in as one user.
type Reader interface {
	Query(ctx context.Context, o api.QueryOptions) (api.QueryResult, error)
	GetDocumentRaw(ctx context.Context, id string) (int, []byte, error)
	SearchEmbeddings(ctx context.Context, vector []float32, typ, query string, limit int64) ([]api.EmbeddingHit, int64, error)
}

// AdminName is the name of the administrator in the check report.
const AdminName = "admin"

// CheckOptions are the inputs of Check.
type CheckOptions struct {
	// Admin reads every document.
	Admin Reader
	// Wait, when set, waits for the search index before the first query.
	Wait func(ctx context.Context) error
	// Users has a reader for each demo user, by name.
	Users map[string]Reader
	// Embed, when set, also checks vector search with Questions, against the
	// vectors of type EmbeddingType.
	Embed         Embedder
	EmbeddingType string
	// VectorTimeout is how long to wait until the admin finds every vector:
	// they reach the search index a few seconds after they are stored.
	VectorTimeout time.Duration
	// PollInterval is the wait between two vector searches of that wait.
	PollInterval time.Duration
}

// UserResult is the check of one user.
type UserResult struct {
	User string `json:"user"`
	// Reads is the number of demo documents the user may read.
	Reads    int `json:"reads"`
	Requests int `json:"requests"`
	Leaks    int `json:"leaks"`
	Problems int `json:"problems"`
}

// CheckReport is the result of Check.
//
// A leak is data of a document the user may not read: its name, its
// description, or a read that works. A problem is a document the user may read
// but does not get.
type CheckReport struct {
	Passed       bool         `json:"passed"`
	Users        []UserResult `json:"users"`
	Documents    int          `json:"documents"`
	Requests     int          `json:"requests"`
	Leaks        int          `json:"leaks"`
	Problems     int          `json:"problems"`
	VectorSearch bool         `json:"vectorSearch"`
	// Failures describe each leak and problem.
	Failures        []string `json:"failures"`
	CLIVersion      string   `json:"cliVersion,omitempty"`
	OpenArchVersion string   `json:"openarchVersion,omitempty"`
}

// Check logs in as the administrator and as each demo user, and compares what
// each one gets with visibleTo in the fixture: document lists from the search
// index and from the database, title queries, reads by id and, with Embed,
// vector search. Every response is searched for the canaries of the documents
// the user may not read.
func Check(ctx context.Context, o CheckOptions) (CheckReport, error) {
	rep := CheckReport{Documents: len(Data.Documents), VectorSearch: o.Embed != nil, Users: []UserResult{}, Failures: []string{}}
	if o.Wait != nil {
		if err := o.Wait(ctx); err != nil {
			return rep, err
		}
	}
	res, err := o.Admin.Query(ctx, api.QueryOptions{Query: ListQuery, Limit: checkLimit, Sync: true})
	if err != nil {
		return rep, err
	}
	ids := map[string]string{}
	for _, d := range res.Documents {
		ids[d.Name] = d.ID
	}
	var missing []string
	for _, d := range Data.Documents {
		if ids[d.Name] == "" {
			missing = append(missing, d.Name)
		}
	}
	if len(missing) > 0 {
		return rep, output.NewError(output.ExitPrerequisite,
			fmt.Sprintf("%d of %d demo documents are missing under %s", len(missing), len(Data.Documents), Root),
			"Run 'openarch demo load --force'.")
	}
	var vectors [][]float32
	if o.Embed != nil {
		if vectors, err = o.Embed(ctx, Questions); err != nil {
			return rep, err
		}
		if len(vectors) != len(Questions) {
			return rep, fmt.Errorf("%d vectors for %d questions", len(vectors), len(Questions))
		}
		if err := waitForVectors(ctx, o, vectors[0]); err != nil {
			return rep, err
		}
	}
	all := map[string]bool{}
	for _, d := range Data.Documents {
		all[d.Name] = true
	}
	type who struct {
		name    string
		r       Reader
		visible map[string]bool
	}
	list := []who{{AdminName, o.Admin, all}}
	for _, u := range Users {
		r, ok := o.Users[u.Name]
		if !ok {
			return rep, fmt.Errorf("no client for demo user %s", u.Name)
		}
		visible := map[string]bool{}
		for _, d := range Data.Documents {
			if slices.Contains(d.VisibleTo, u.Name) {
				visible[d.Name] = true
			}
		}
		list = append(list, who{u.Name, r, visible})
	}
	for _, w := range list {
		c := &userCheck{ctx: ctx, user: w.name, r: w.r, visible: w.visible, ids: ids, rep: &rep,
			res: UserResult{User: w.name, Reads: len(w.visible)}}
		if err := c.run(vectors, o.EmbeddingType); err != nil {
			return rep, fmt.Errorf("%s: %w", w.name, err)
		}
		rep.Users = append(rep.Users, c.res)
		rep.Requests += c.res.Requests
		rep.Leaks += c.res.Leaks
		rep.Problems += c.res.Problems
	}
	rep.Passed = rep.Leaks == 0 && rep.Problems == 0
	return rep, nil
}

// waitForVectors waits until the admin finds a vector for every demo
// document, or until o.VectorTimeout. After the timeout the check goes on, and
// reports the missing vectors as problems.
func waitForVectors(ctx context.Context, o CheckOptions, v []float32) error {
	deadline := time.Now().Add(o.VectorTimeout)
	for {
		hits, _, err := o.Admin.SearchEmbeddings(ctx, v, o.EmbeddingType, ListQuery, checkLimit)
		if err != nil {
			return err
		}
		if len(hits) >= len(Data.Documents) || !time.Now().Before(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(o.PollInterval):
		}
	}
}

// userCheck runs the requests of one user.
type userCheck struct {
	ctx     context.Context
	user    string
	r       Reader
	visible map[string]bool
	ids     map[string]string
	rep     *CheckReport
	res     UserResult
}

func (c *userCheck) leak(where, format string, args ...any) {
	c.res.Leaks++
	c.rep.Failures = append(c.rep.Failures, fmt.Sprintf("leak: %s, %s: %s", c.user, where, fmt.Sprintf(format, args...)))
}

func (c *userCheck) problem(where, format string, args ...any) {
	c.res.Problems++
	c.rep.Failures = append(c.rep.Failures, fmt.Sprintf("problem: %s, %s: %s", c.user, where, fmt.Sprintf(format, args...)))
}

// scan reports a leak for each document the user may not read whose name is
// in got or whose canary is in raw.
func (c *userCheck) scan(where string, got []string, raw []byte) {
	for _, d := range Data.Documents {
		if c.visible[d.Name] {
			continue
		}
		switch {
		case slices.Contains(got, d.Name):
			c.leak(where, "got %s", d.Name)
		case bytes.Contains(raw, []byte(d.Canary)):
			c.leak(where, "the response has the canary of %s", d.Name)
		}
	}
}

// compare checks that got has exactly the documents of want that the user
// may read, and that the response has the description of each one.
func (c *userCheck) compare(where string, got []string, raw []byte, want []string) {
	c.scan(where, got, raw)
	for _, n := range want {
		if !slices.Contains(got, n) {
			c.problem(where, "%s is missing", n)
		} else if d := doc(n); !bytes.Contains(raw, []byte(d.Canary)) {
			c.problem(where, "the description of %s is missing", n)
		}
	}
	for _, n := range got {
		if c.visible[n] && !slices.Contains(want, n) {
			c.problem(where, "got %s, which does not match the query", n)
		}
	}
}

func (c *userCheck) query(q string, sync bool) ([]string, []byte, error) {
	c.res.Requests++
	res, err := c.r.Query(c.ctx, api.QueryOptions{Query: q, Limit: checkLimit, Sync: sync})
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, len(res.Documents))
	for i, d := range res.Documents {
		names[i] = d.Name
	}
	return names, res.Raw, nil
}

func (c *userCheck) run(vectors [][]float32, typ string) error {
	visible := c.sorted(func(Doc) bool { return true })
	for _, sync := range []bool{false, true} {
		where := "list"
		if sync {
			where = "list from the database"
		}
		got, raw, err := c.query(ListQuery, sync)
		if err != nil {
			return err
		}
		c.compare(where, got, raw, visible)
	}
	for _, word := range titleWords() {
		got, raw, err := c.query(ListQuery+" AND sys_title LIKE '%"+word+"%'", false)
		if err != nil {
			return err
		}
		c.compare("title query '"+word+"'", got, raw, c.sorted(func(d Doc) bool { return strings.Contains(d.Title, word) }))
	}
	for _, d := range Data.Documents {
		where := "read " + d.Name
		c.res.Requests++
		status, body, err := c.r.GetDocumentRaw(c.ctx, c.ids[d.Name])
		if err != nil {
			return err
		}
		switch {
		case c.visible[d.Name] && status != http.StatusOK:
			c.problem(where, "HTTP %d, want 200", status)
		case c.visible[d.Name] && !bytes.Contains(body, []byte(d.Canary)):
			c.problem(where, "the description is missing")
		case !c.visible[d.Name] && status == http.StatusOK:
			c.leak(where, "HTTP 200, want 403")
		case !c.visible[d.Name] && status != http.StatusForbidden:
			c.problem(where, "HTTP %d, want 403", status)
		}
		if status != http.StatusOK {
			c.scan(where, nil, body)
		}
	}
	names := map[string]string{}
	for n, id := range c.ids {
		names[id] = n
	}
	for i, v := range vectors {
		where := fmt.Sprintf("vector search %q", Questions[i])
		c.res.Requests++
		hits, _, err := c.r.SearchEmbeddings(c.ctx, v, typ, ListQuery, checkLimit)
		if err != nil {
			return err
		}
		var got []string
		var texts bytes.Buffer
		for _, h := range hits {
			n, ok := names[h.DocID]
			if !ok {
				c.problem(where, "got %s, which is not a demo document", h.DocID)
				continue
			}
			got = append(got, n)
			texts.WriteString(h.Text + "\n")
		}
		c.scan(where, got, texts.Bytes())
		if len(hits) == 0 && len(c.visible) > 0 {
			c.problem(where, "no documents found; store the vectors with 'openarch demo load --force --embeddings'")
		}
	}
	return nil
}

// sorted returns the names of the documents the user may read that match keep.
func (c *userCheck) sorted(keep func(Doc) bool) []string {
	var names []string
	for _, d := range Data.Documents {
		if c.visible[d.Name] && keep(d) {
			names = append(names, d.Name)
		}
	}
	sort.Strings(names)
	return names
}

// titleWords are the shared words, as written and capitalized: LIKE is case-sensitive.
func titleWords() []string {
	var words []string
	for _, w := range Data.SharedWords {
		for _, x := range []string{w, strings.ToUpper(w[:1]) + w[1:]} {
			if !slices.Contains(words, x) {
				words = append(words, x)
			}
		}
	}
	sort.Strings(words)
	return words
}

func doc(name string) Doc {
	for _, d := range Data.Documents {
		if d.Name == name {
			return d
		}
	}
	return Doc{}
}

// WriteText writes one line per user, the failures, and a result line that can
// be shared as is.
func (r CheckReport) WriteText(w io.Writer) error {
	var b strings.Builder
	for _, u := range r.Users {
		fmt.Fprintf(&b, "%-6s may read %2d of %d documents: %d requests, %d leaks, %d problems\n",
			u.User, u.Reads, r.Documents, u.Requests, u.Leaks, u.Problems)
	}
	if len(r.Failures) > 0 {
		b.WriteString("\n")
		for _, f := range r.Failures {
			b.WriteString("  " + f + "\n")
		}
	}
	kinds := "lists, title queries, reads"
	if r.VectorSearch {
		kinds += ", vector search"
	}
	result := "PASS"
	if !r.Passed {
		result = "FAIL"
	}
	fmt.Fprintf(&b, "\n%s: %d users, %d documents, %d requests (%s), %d leaks, %d problems.", result, len(r.Users), r.Documents, r.Requests, kinds, r.Leaks, r.Problems)
	if r.CLIVersion != "" {
		fmt.Fprintf(&b, " CLI for OpenArch %s", r.CLIVersion)
		if r.OpenArchVersion != "" {
			fmt.Fprintf(&b, ", Hyland OpenArch %s", r.OpenArchVersion)
		}
		b.WriteString(".")
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}
