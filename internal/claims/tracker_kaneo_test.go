package claims

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// kTask is one task as the fake Kaneo serves it. Deferred leaves the
// description off the board, as Kaneo does for one over 64 KiB, so only the
// single-task route has it.
type kTask struct {
	ID, Created, Description string
	Labels                   []string
	Deferred                 bool
}

// kPage is one page of a project's board: each column's share of the tasks,
// keyed by the column's slug, and the archived and planned tasks on it.
type kPage struct {
	Columns  map[string][]kTask
	Archived []kTask
	Planned  []kTask
}

// kProject is one project. Related is how many related pages each board page
// splits its tasks' labels across, as Kaneo does; 0 leaves relatedTotalPages
// out of the answer, so every label is on the one page.
type kProject struct {
	ID       string
	Archived bool
	Related  int
	Pages    []kPage
}

type kWorkspace struct {
	ID       string
	Projects []kProject
}

type kEvent struct {
	Type, At, NewStatus string
}

// kColumns are a project's columns as Kaneo creates them: two open, and Done
// and Won't do both final.
var kColumns = []struct {
	slug, name string
	final      bool
}{
	{"to-do", "To do", false},
	{"in-progress", "In progress", false},
	{"done", "Done", true},
	{"wont-do", "Won't do", true},
}

type kaneoFake struct {
	token      string
	workspaces []kWorkspace
	activity   map[string][]kEvent
	mu         sync.Mutex
	hits       map[string]int // request path -> times asked
}

func (f *kaneoFake) hit(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hits == nil {
		f.hits = map[string]int{}
	}
	f.hits[path]++
}

func (f *kaneoFake) activityCalls() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for p, n := range f.hits {
		if id, ok := strings.CutPrefix(p, "/api/activity/"); ok {
			out[id] = n
		}
	}
	return out
}

// kTaskJSON is a task as Kaneo answers it. With of above 1, the labels are
// only related page related's share of of pages: the task's i-th label is on
// related page i%of + 1, so a task with fewer labels than pages has none on
// the last ones.
func kTaskJSON(t kTask, status, projectID string, full bool, related, of int) map[string]any {
	created := t.Created
	if created == "" {
		created = "2026-06-01T00:00:00.000Z"
	}
	labels := []map[string]any{}
	for i, l := range t.Labels {
		if of > 1 && i%of+1 != related {
			continue
		}
		labels = append(labels, map[string]any{"id": "l-" + l, "name": l, "color": "#94a3b8"})
	}
	out := map[string]any{"id": t.ID, "title": "task " + t.ID, "createdAt": created, "labels": labels, "status": status, "projectId": projectID}
	if t.Deferred && !full {
		out["descriptionDeferred"] = true
	} else {
		out["description"] = t.Description
	}
	return out
}

func (f *kaneoFake) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hit(r.URL.Path)
		if r.Header.Get("x-api-key") != f.token {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		write := func(v any) {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(v); err != nil {
				t.Errorf("encoding the fake's answer: %v", err)
			}
		}
		path := r.URL.Path
		switch {
		case path == "/api/auth/organization/list":
			out := []map[string]any{}
			for _, ws := range f.workspaces {
				out = append(out, map[string]any{"id": ws.ID, "name": "workspace " + ws.ID})
			}
			write(out)
		case path == "/api/project":
			out := []map[string]any{}
			for _, ws := range f.workspaces {
				if ws.ID != r.URL.Query().Get("workspaceId") {
					continue
				}
				for _, p := range ws.Projects {
					if p.Archived && r.URL.Query().Get("includeArchived") != "true" {
						continue
					}
					out = append(out, map[string]any{"id": p.ID, "name": "project " + p.ID, "createdAt": "2026-01-01T00:00:00.000Z"})
				}
			}
			write(out)
		case strings.HasPrefix(path, "/api/task/tasks/"):
			p := f.project(strings.TrimPrefix(path, "/api/task/tasks/"))
			if p == nil {
				http.NotFound(w, r)
				return
			}
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			// Kaneo answers a request without relatedPage with the first
			// related page's share.
			related, _ := strconv.Atoi(r.URL.Query().Get("relatedPage"))
			if related < 1 {
				related = 1
			}
			var pg kPage
			if page >= 1 && page <= len(p.Pages) {
				pg = p.Pages[page-1]
			}
			columns := []map[string]any{}
			for _, c := range kColumns {
				tasks := []map[string]any{}
				for _, task := range pg.Columns[c.slug] {
					tasks = append(tasks, kTaskJSON(task, c.slug, p.ID, false, related, p.Related))
				}
				columns = append(columns, map[string]any{"id": c.slug, "slug": c.slug, "name": c.name, "isFinal": c.final, "tasks": tasks})
			}
			archived, planned := []map[string]any{}, []map[string]any{}
			for _, task := range pg.Archived {
				archived = append(archived, kTaskJSON(task, "archived", p.ID, false, related, p.Related))
			}
			for _, task := range pg.Planned {
				planned = append(planned, kTaskJSON(task, "planned", p.ID, false, related, p.Related))
			}
			pagination := map[string]any{"total": 0, "page": page, "pageSize": limit, "totalPages": len(p.Pages)}
			if p.Related > 0 {
				pagination["relatedTotalPages"] = p.Related
			}
			write(map[string]any{
				"data":       map[string]any{"columns": columns, "archivedTasks": archived, "plannedTasks": planned},
				"pagination": pagination,
			})
		case strings.HasPrefix(path, "/api/task/"):
			id := strings.TrimPrefix(path, "/api/task/")
			for _, ws := range f.workspaces {
				for _, p := range ws.Projects {
					for _, pg := range p.Pages {
						for slug, tasks := range pg.Columns {
							for _, task := range tasks {
								if task.ID == id {
									write(kTaskJSON(task, slug, p.ID, true, 1, 0))
									return
								}
							}
						}
						for _, task := range append(append([]kTask{}, pg.Archived...), pg.Planned...) {
							if task.ID == id {
								write(kTaskJSON(task, "archived", p.ID, true, 1, 0))
								return
							}
						}
					}
				}
			}
			http.NotFound(w, r)
		case strings.HasPrefix(path, "/api/activity/"):
			out := []map[string]any{}
			for _, e := range f.activity[strings.TrimPrefix(path, "/api/activity/")] {
				data := map[string]any{}
				if e.Type == "status_changed" {
					data = map[string]any{"newStatus": e.NewStatus, "oldStatus": "to-do"}
				}
				out = append(out, map[string]any{"id": "e", "type": e.Type, "createdAt": e.At, "eventData": data})
			}
			write(out)
		default:
			http.NotFound(w, r)
		}
	}))
}

func (f *kaneoFake) project(id string) *kProject {
	for _, ws := range f.workspaces {
		for i := range ws.Projects {
			if ws.Projects[i].ID == id {
				return &ws.Projects[i]
			}
		}
	}
	return nil
}

// oneProject is a fake with a single workspace holding a single project.
func oneProject(activity map[string][]kEvent, pages ...kPage) *kaneoFake {
	return &kaneoFake{token: "key", activity: activity,
		workspaces: []kWorkspace{{ID: "w1", Projects: []kProject{{ID: "p1", Pages: pages}}}}}
}

// footer is the line an import from Vikunja ends a task's description with.
func footer(created, done string) string {
	s := "\n\n---\n*Imported from [Vikunja 105](https://tracker.example/tasks/105)"
	if created != "" {
		s += ", created " + created
	}
	if done != "" {
		s += ", done " + done
	}
	return s + ".*"
}

func doneAt(at string) []kEvent {
	return []kEvent{{Type: "status_changed", At: at, NewStatus: "done"}}
}

var kNow = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

func kCheck(t *testing.T, f *kaneoFake, k *Kaneo, args map[string]string) Outcome {
	t.Helper()
	srv := f.server(t)
	t.Cleanup(srv.Close)
	if k == nil {
		k = &Kaneo{}
	}
	k.URL, k.Client = srv.URL, srv.Client()
	if k.Token == "" {
		k.Token = "key"
	}
	out, err := k.Check(context.Background(), args, kNow)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return out
}

func since(label, date, min, done string) map[string]string {
	return map[string]string{"label": label, "since": date, "min": min, "done": done}
}

func TestKaneoMatchesTheLabelCaseInsensitively(t *testing.T) {
	f := oneProject(map[string][]kEvent{"a": doneAt("2026-11-10T12:00:00.000Z"), "b": doneAt("2026-11-10T12:00:00.000Z"), "c": doneAt("2026-11-10T12:00:00.000Z")},
		kPage{Columns: map[string][]kTask{"done": {{ID: "a", Labels: []string{"Article"}}, {ID: "b", Labels: []string{"article"}}, {ID: "c", Labels: []string{"chore"}}}}})
	if out := kCheck(t, f, nil, since("ARTICLE", "2026-11-01", "1", "true")); out.State != store.Pass || out.Detail != "2 found" {
		t.Errorf("%+v, want 2 found: a and b carry the label in other cases, c carries another", out)
	}
}

// Done is a final column, either of them, or the archive; a planned task, or
// one in an open column, is open.
func TestKaneoReadsDoneFromAFinalColumnOrTheArchive(t *testing.T) {
	task := kTask{ID: "t", Created: "2026-11-05T00:00:00.000Z", Labels: []string{"article"}}
	cases := []struct {
		name     string
		page     kPage
		moved    string
		wantDone bool
	}{
		{"in Done", kPage{Columns: map[string][]kTask{"done": {task}}}, "done", true},
		{"in Won't do", kPage{Columns: map[string][]kTask{"wont-do": {task}}}, "wont-do", true},
		{"archived", kPage{Archived: []kTask{task}}, "done", true},
		{"planned", kPage{Planned: []kTask{task}}, "", false},
		{"in progress", kPage{Columns: map[string][]kTask{"in-progress": {task}}}, "in-progress", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := map[string][]kEvent{}
			if c.moved != "" {
				events["t"] = []kEvent{{Type: "status_changed", At: "2026-11-10T12:00:00.000Z", NewStatus: c.moved}}
			}
			doneOut := kCheck(t, oneProject(events, c.page), nil, since("article", "2026-11-01", "1", "true"))
			openOut := kCheck(t, oneProject(events, c.page), nil, since("article", "2026-11-01", "1", "false"))
			if c.wantDone && (doneOut.Detail != "1 found" || openOut.Detail != "0 found") {
				t.Errorf("done: %+v, open: %+v; want it counted as done only", doneOut, openOut)
			}
			if !c.wantDone && (doneOut.Detail != "0 found" || openOut.Detail != "1 found") {
				t.Errorf("done: %+v, open: %+v; want it counted as open only", doneOut, openOut)
			}
		})
	}
}

// The task moved to Done on 2026-10-20, back out on 2026-10-25 and to Done
// again on 2026-11-10, and was commented on later: it finished on 2026-11-10.
func TestKaneoDatesCompletionByTheNewestMoveIntoAFinalColumn(t *testing.T) {
	events := map[string][]kEvent{"a": {
		{Type: "comment", At: "2026-11-20T09:00:00.000Z"},
		{Type: "status_changed", At: "2026-11-10T12:00:00.000Z", NewStatus: "done"},
		{Type: "status_changed", At: "2026-10-25T12:00:00.000Z", NewStatus: "in-progress"},
		{Type: "status_changed", At: "2026-10-20T12:00:00.000Z", NewStatus: "done"},
	}}
	page := kPage{Columns: map[string][]kTask{"done": {{ID: "a", Labels: []string{"article"}}}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.Detail != "1 found" {
		t.Errorf("since 2026-11-01: %+v, want the 2026-11-10 move counted", out)
	}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-15", "1", "true")); out.Detail != "0 found" {
		t.Errorf("since 2026-11-15: %+v, want nothing: the comment is not a completion", out)
	}
}

// An archived task last moved to an open column is dated by the move into a
// final column before it, not by the later move.
func TestKaneoIgnoresANewerMoveIntoAnOpenColumn(t *testing.T) {
	events := map[string][]kEvent{"a": {
		{Type: "status_changed", At: "2026-11-20T12:00:00.000Z", NewStatus: "to-do"},
		{Type: "status_changed", At: "2026-10-20T12:00:00.000Z", NewStatus: "done"},
	}}
	page := kPage{Archived: []kTask{{ID: "a", Labels: []string{"article"}}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.State != store.Fail || out.Detail != "0 found" {
		t.Errorf("%+v, want 0 found: it finished on 2026-10-20", out)
	}
}

// An imported task has no move into a final column, so its import footer
// dates it.
func TestKaneoFallsBackToTheImportFootersDoneDate(t *testing.T) {
	events := map[string][]kEvent{"a": {{Type: "comment", At: "2026-11-27T16:00:00.000Z"}}}
	page := kPage{Columns: map[string][]kTask{"done": {
		{ID: "a", Created: "2026-11-27T16:00:00.000Z", Labels: []string{"article"}, Description: "Write it." + footer("2026-09-01", "2026-11-26")},
		{ID: "b", Created: "2026-11-27T16:00:00.000Z", Labels: []string{"article"}, Description: "Write it." + footer("2026-08-01", "2026-10-01")},
	}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.Detail != "1 found" {
		t.Errorf("%+v, want a (done 2026-11-26) counted and b (done 2026-10-01) not", out)
	}
}

func TestKaneoPrefersTheMoveIntoAFinalColumnToTheFooter(t *testing.T) {
	events := map[string][]kEvent{"a": doneAt("2026-11-10T12:00:00.000Z")}
	page := kPage{Columns: map[string][]kTask{"done": {{ID: "a", Labels: []string{"article"}, Description: "x" + footer("", "2026-10-01")}}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.Detail != "1 found" {
		t.Errorf("%+v, want the 2026-11-10 move to date it", out)
	}
}

// An imported task's createdAt is the import, so an open one is dated by the
// footer's created date; one with no footer by its createdAt.
func TestKaneoDatesAnOpenImportedTaskByItsFootersCreatedDate(t *testing.T) {
	page := kPage{Columns: map[string][]kTask{"to-do": {
		{ID: "imported", Created: "2026-11-27T16:00:00.000Z", Labels: []string{"article"},
			Description: "x" + footer("2026-09-01", "") + "\n*Also assigned in Vikunja to someone.*"},
		{ID: "native", Created: "2026-11-15T10:00:00.000Z", Labels: []string{"article"}},
	}}}
	if out := kCheck(t, oneProject(nil, page), nil, since("article", "2026-11-01", "1", "false")); out.Detail != "1 found" {
		t.Errorf("%+v, want only native counted: imported was created 2026-09-01", out)
	}
}

// The footer is the block after the description's last rule; the same words
// quoted in the body are not one.
func TestKaneoIgnoresFooterTextOutsideTheFooter(t *testing.T) {
	quoted := "Notes.\n\n*Imported from [Vikunja 9](https://tracker.example/tasks/9), created 2026-09-01.*\n\nMore notes."
	page := kPage{Columns: map[string][]kTask{"to-do": {{ID: "a", Created: "2026-11-15T10:00:00.000Z", Labels: []string{"article"}, Description: quoted}}}}
	if out := kCheck(t, oneProject(nil, page), nil, since("article", "2026-11-01", "1", "false")); out.Detail != "1 found" {
		t.Errorf("%+v, want it dated by createdAt 2026-11-15", out)
	}
}

// A done task with neither a move into a final column nor a footer date has
// no completion date; skipping it could undercount into a fail.
func TestKaneoAnswersNoEvidenceForADoneTaskWithNoDate(t *testing.T) {
	page := kPage{Columns: map[string][]kTask{"done": {{ID: "undated", Labels: []string{"article"}, Description: "x" + footer("2026-09-01", "")}}}}
	out := kCheck(t, oneProject(nil, page), nil, since("article", "2026-11-01", "1", "true"))
	if out.State != store.NoEvidence || !strings.Contains(out.Detail, "undated") {
		t.Errorf("%+v, want no-evidence naming the task", out)
	}
}

// Every page repeats every column with its own share of the tasks; a task
// seen on two pages is one task.
func TestKaneoReadsEveryPageAndCountsARepeatedTaskOnce(t *testing.T) {
	a := kTask{ID: "a", Labels: []string{"article"}}
	b := kTask{ID: "b", Labels: []string{"article"}}
	c := kTask{ID: "c", Labels: []string{"article"}}
	ev := doneAt("2026-11-10T12:00:00.000Z")
	f := oneProject(map[string][]kEvent{"a": ev, "b": ev, "c": ev},
		kPage{Columns: map[string][]kTask{"done": {a}}},
		kPage{Columns: map[string][]kTask{"done": {a, b}}},
		kPage{Columns: map[string][]kTask{"done": {c}}})
	if out := kCheck(t, f, &Kaneo{perPage: 2}, since("article", "2026-11-01", "1", "true")); out.Detail != "3 found" {
		t.Errorf("%+v, want 3 found", out)
	}
}

func TestKaneoStoppedAtItsPageCapCannotFail(t *testing.T) {
	ev := doneAt("2026-11-10T12:00:00.000Z")
	f := oneProject(map[string][]kEvent{"a": ev, "b": ev},
		kPage{Columns: map[string][]kTask{"done": {{ID: "a", Labels: []string{"article"}}}}},
		kPage{Columns: map[string][]kTask{"done": {{ID: "b", Labels: []string{"article"}}}}})
	out := kCheck(t, f, &Kaneo{maxPages: 1}, since("article", "2026-11-01", "2", "true"))
	if out.State != store.NoEvidence || !strings.Contains(out.Detail, "1 found") {
		t.Errorf("%+v, want no-evidence: 1 found is a lower bound", out)
	}
}

func TestKaneoCountsAcrossWorkspacesAndProjects(t *testing.T) {
	ev := doneAt("2026-11-10T12:00:00.000Z")
	one := func(id string) []kPage {
		return []kPage{{Columns: map[string][]kTask{"done": {{ID: id, Labels: []string{"article"}}}}}}
	}
	f := &kaneoFake{token: "key", activity: map[string][]kEvent{"a": ev, "b": ev, "c": ev},
		workspaces: []kWorkspace{
			{ID: "w1", Projects: []kProject{{ID: "p1", Pages: one("a")}, {ID: "p2", Archived: true, Pages: one("b")}}},
			{ID: "w2", Projects: []kProject{{ID: "p3", Pages: one("c")}}},
		}}
	if out := kCheck(t, f, nil, since("article", "2026-11-01", "1", "true")); out.Detail != "3 found" {
		t.Errorf("%+v, want 3 found, the archived project included", out)
	}
}

func TestKaneoAnswersNoEvidenceForAnUnknownLabel(t *testing.T) {
	page := kPage{Columns: map[string][]kTask{"to-do": {{ID: "a", Labels: []string{"article"}}}}}
	out := kCheck(t, oneProject(nil, page), nil, since("novel", "2026-11-01", "1", "true"))
	if out.State != store.NoEvidence || !strings.Contains(out.Detail, `"novel" unknown to the tracker`) {
		t.Errorf("%+v, want no-evidence naming the label", out)
	}
}

func TestKaneoAnswersNoEvidenceForARefusedKey(t *testing.T) {
	page := kPage{Columns: map[string][]kTask{"to-do": {{ID: "a", Labels: []string{"article"}}}}}
	out := kCheck(t, oneProject(nil, page), &Kaneo{Token: "revoked"}, since("article", "2026-11-01", "1", "true"))
	if out.State != store.NoEvidence || !strings.Contains(out.Detail, "401") {
		t.Errorf("%+v, want no-evidence naming HTTP 401", out)
	}
}

// Activity costs one call per task, so it is read only for a done task
// carrying the label, and only when the claim counts done tasks.
func TestKaneoReadsActivityOnlyForDoneTasksCarryingTheLabel(t *testing.T) {
	ev := doneAt("2026-11-10T12:00:00.000Z")
	f := oneProject(map[string][]kEvent{"match": ev, "other": ev},
		kPage{Columns: map[string][]kTask{
			"done":  {{ID: "match", Labels: []string{"article"}}, {ID: "other", Labels: []string{"chore"}}},
			"to-do": {{ID: "open", Labels: []string{"article"}}},
		}})
	kCheck(t, f, nil, since("article", "2026-11-01", "1", "true"))
	if got := f.activityCalls(); len(got) != 1 || got["match"] != 1 {
		t.Errorf("activity read for %v, want only match, once", got)
	}
	f.hits = nil
	kCheck(t, f, nil, since("article", "2026-11-01", "1", "false"))
	if got := f.activityCalls(); len(got) != 0 {
		t.Errorf("activity read for %v counting open tasks, want none", got)
	}
}

// Kaneo leaves a description over 64 KiB off the board, so its footer is
// read from the task itself.
func TestKaneoReadsTheFooterOfADeferredDescription(t *testing.T) {
	page := kPage{Columns: map[string][]kTask{"to-do": {{ID: "big", Created: "2026-11-27T16:00:00.000Z", Labels: []string{"article"},
		Description: "x" + footer("2026-09-01", ""), Deferred: true}}}}
	if out := kCheck(t, oneProject(nil, page), nil, since("article", "2026-11-01", "1", "false")); out.State != store.Fail || out.Detail != "0 found" {
		t.Errorf("%+v, want 0 found: the footer says created 2026-09-01", out)
	}
}

// A board splits each task's labels across related pages, so a label on any
// one of them is the task's.
func TestKaneoMatchesALabelOnAnyRelatedPage(t *testing.T) {
	cases := []struct {
		name    string
		related int
		labels  []string
	}{
		{"second of two", 2, []string{"dom:security", "article"}},
		{"third of three", 3, []string{"kind:defect", "cons:3", "article"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &kaneoFake{token: "key", activity: map[string][]kEvent{"a": doneAt("2026-11-10T12:00:00.000Z")},
				workspaces: []kWorkspace{{ID: "w1", Projects: []kProject{{ID: "p1", Related: c.related,
					Pages: []kPage{{Columns: map[string][]kTask{"done": {{ID: "a", Labels: c.labels}}}}}}}}}}
			if out := kCheck(t, f, nil, since("article", "2026-11-01", "1", "true")); out.State != store.Pass || out.Detail != "1 found" {
				t.Errorf("%+v, want 1 found: article is on the last related page", out)
			}
		})
	}
}

// Every related page repeats the page's tasks, so a task met on several pages
// and several related pages is still one task, and its activity is read once.
func TestKaneoCountsATaskOnceAcrossPagesAndRelatedPages(t *testing.T) {
	labels := []string{"chore", "article"}
	a := kTask{ID: "a", Labels: labels}
	b := kTask{ID: "b", Labels: labels}
	c := kTask{ID: "c", Labels: labels}
	ev := doneAt("2026-11-10T12:00:00.000Z")
	f := &kaneoFake{token: "key", activity: map[string][]kEvent{"a": ev, "b": ev, "c": ev},
		workspaces: []kWorkspace{{ID: "w1", Projects: []kProject{{ID: "p1", Related: 2, Pages: []kPage{
			{Columns: map[string][]kTask{"done": {a}}},
			{Columns: map[string][]kTask{"done": {a, b}}},
			{Columns: map[string][]kTask{"done": {c}}},
		}}}}}}
	if out := kCheck(t, f, &Kaneo{perPage: 2}, since("article", "2026-11-01", "1", "true")); out.Detail != "3 found" {
		t.Errorf("%+v, want 3 found", out)
	}
	if got := f.activityCalls(); len(got) != 3 || got["a"] != 1 || got["b"] != 1 || got["c"] != 1 {
		t.Errorf("activity read for %v, want a, b and c once each", got)
	}
}

// A label on a related page past the cap was never seen, so the count is a
// lower bound and cannot fail.
func TestKaneoStoppedAtItsPageCapOnRelatedPagesCannotFail(t *testing.T) {
	f := &kaneoFake{token: "key", activity: map[string][]kEvent{"a": doneAt("2026-11-10T12:00:00.000Z")},
		workspaces: []kWorkspace{{ID: "w1", Projects: []kProject{{ID: "p1", Related: 3,
			Pages: []kPage{{Columns: map[string][]kTask{"done": {{ID: "a", Labels: []string{"x", "y", "article"}}}}}}}}}}}
	out := kCheck(t, f, &Kaneo{maxPages: 2}, since("article", "2026-11-01", "1", "true"))
	if out.State != store.NoEvidence || !strings.Contains(out.Detail, "stopped before the end") {
		t.Errorf("%+v, want no-evidence: the third related page was not read", out)
	}
}

// Archiving is a status change to "archived". A task archived without ever
// being moved into a final column finished when it was archived.
func TestKaneoDatesAnArchivedTaskByItsArchivingEvent(t *testing.T) {
	events := map[string][]kEvent{"a": {{Type: "status_changed", At: "2026-11-10T12:00:00.000Z", NewStatus: "archived"}}}
	page := kPage{Archived: []kTask{{ID: "a", Labels: []string{"article"}, Description: "x"}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.State != store.Pass || out.Detail != "1 found" {
		t.Errorf("%+v, want 1 found: it was archived on 2026-11-10", out)
	}
}

// Archiving tidies a finished task away; it does not finish it again. A task
// moved to Done and archived later finished at the move.
func TestKaneoDatesADoneThenArchivedTaskByItsMoveToDone(t *testing.T) {
	events := map[string][]kEvent{"a": {
		{Type: "status_changed", At: "2026-11-20T12:00:00.000Z", NewStatus: "archived"},
		{Type: "status_changed", At: "2026-10-20T12:00:00.000Z", NewStatus: "done"},
	}}
	page := kPage{Columns: map[string][]kTask{"done": nil}, Archived: []kTask{{ID: "a", Labels: []string{"article"}}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.State != store.Fail || out.Detail != "0 found" {
		t.Errorf("%+v, want 0 found: it finished on 2026-10-20 and was only archived later", out)
	}
}

// An imported task archived after the import finished at its footer's done
// date, not at the archiving.
func TestKaneoDatesAnArchivedImportedTaskByItsFooter(t *testing.T) {
	events := map[string][]kEvent{"a": {{Type: "status_changed", At: "2026-11-20T12:00:00.000Z", NewStatus: "archived"}}}
	page := kPage{Archived: []kTask{{ID: "a", Labels: []string{"article"}, Description: "x" + footer("2026-08-01", "2026-10-01")}}}
	if out := kCheck(t, oneProject(events, page), nil, since("article", "2026-11-01", "1", "true")); out.State != store.Fail || out.Detail != "0 found" {
		t.Errorf("%+v, want 0 found: it was done on 2026-10-01 and only archived later", out)
	}
}
