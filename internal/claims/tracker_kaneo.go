package claims

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// Kaneo counts tasks by label and date. Kaneo 2.50.0 has no route that lists
// tasks across projects, so the adapter walks every workspace the key can
// see, every project in each, archived ones included because their tasks were
// still done, and every page of each project's board.
type Kaneo struct {
	URL, Token string
	Client     *http.Client
	perPage    int // tests lower it to exercise paging
	maxPages   int // tests lower it to exercise the cap
}

func (k *Kaneo) Name() string { return "tracker" }

type kaneoTask struct {
	ID                  string `json:"id"`
	Description         string `json:"description"`
	DescriptionDeferred bool   `json:"descriptionDeferred"`
	CreatedAt           string `json:"createdAt"`
	Labels              []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type kaneoBoard struct {
	Data *struct {
		Columns []struct {
			ID      string      `json:"id"`
			Slug    string      `json:"slug"`
			IsFinal bool        `json:"isFinal"`
			Tasks   []kaneoTask `json:"tasks"`
		} `json:"columns"`
		ArchivedTasks []kaneoTask `json:"archivedTasks"`
		PlannedTasks  []kaneoTask `json:"plannedTasks"`
	} `json:"data"`
	Pagination struct {
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
}

type kaneoEvent struct {
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
	EventData struct {
		NewStatus string `json:"newStatus"`
	} `json:"eventData"`
}

// boardTask is a task as the board placed it: done when it sits in a final
// column or in the archive.
type boardTask struct {
	kaneoTask
	done bool
}

// Check counts done tasks by when they finished, or open ones by when they
// were created, carrying the label (case-insensitively) since the claim's
// date. A label on no task at all is no-evidence (§8.1: "the query matched
// nothing it could count"); a label on some task with none in the window is
// fail by count.
func (k *Kaneo) Check(ctx context.Context, args map[string]string, now time.Time) (Outcome, error) {
	since, err := resolveDate(args["since"], now)
	if err != nil {
		return Outcome{}, err
	}
	min, err := strconv.Atoi(args["min"])
	if err != nil {
		return Outcome{}, fmt.Errorf("min: %w", err)
	}
	wantDone := args["done"] != "false"
	label := strings.ToLower(args["label"])
	headers := map[string]string{"x-api-key": k.Token}
	limit := pageLimit(k.maxPages)

	var workspaces []struct {
		ID string `json:"id"`
	}
	if o, err := get(ctx, k.Client, k.URL+"/api/auth/organization/list", headers, &workspaces); err != nil || o != nil {
		return deref(o), err
	}
	count, seenLabel, capped := 0, false, false
	for _, ws := range workspaces {
		var projects []struct {
			ID string `json:"id"`
		}
		endpoint := fmt.Sprintf("%s/api/project?workspaceId=%s&includeArchived=true", k.URL, url.QueryEscape(ws.ID))
		if o, err := get(ctx, k.Client, endpoint, headers, &projects); err != nil || o != nil {
			return deref(o), err
		}
		for _, p := range projects {
			tasks, final, stopped, o, err := k.board(ctx, headers, p.ID, limit)
			if err != nil || o != nil {
				return deref(o), err
			}
			capped = capped || stopped
			for _, task := range tasks {
				labelled := false
				for _, l := range task.Labels {
					labelled = labelled || strings.ToLower(l.Name) == label
				}
				if !labelled {
					continue
				}
				seenLabel = true
				if task.done != wantDone {
					continue
				}
				when, o, err := k.dated(ctx, headers, task, final)
				if err != nil || o != nil {
					return deref(o), err
				}
				if !when.Before(since) {
					count++
				}
			}
		}
	}
	if capped {
		return exhausted(count, min, limit), nil
	}
	if !seenLabel {
		return Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("label %q unknown to the tracker", args["label"])}, nil
	}
	return counted(count, min), nil
}

func deref(o *Outcome) Outcome {
	if o == nil {
		return Outcome{}
	}
	return *o
}

// board reads every page of one project's board and returns its tasks once
// each, the slugs of its final columns, and whether paging stopped at the cap
// before the last page. Every page repeats every column with that page's share
// of the tasks, so a task can appear on more than one, and a column's
// finality is read from whichever page names it.
func (k *Kaneo) board(ctx context.Context, headers map[string]string, projectID string, limit int) ([]boardTask, map[string]bool, bool, *Outcome, error) {
	perPage := k.perPage
	if perPage == 0 {
		perPage = 100
	}
	var tasks []boardTask
	seen := map[string]int{}
	final := map[string]bool{}
	add := func(t kaneoTask, done bool) {
		if i, ok := seen[t.ID]; ok {
			tasks[i].done = tasks[i].done || done
			return
		}
		seen[t.ID] = len(tasks)
		tasks = append(tasks, boardTask{t, done})
	}
	for page := 1; ; page++ {
		var b kaneoBoard
		endpoint := fmt.Sprintf("%s/api/task/tasks/%s?limit=%d&page=%d", k.URL, url.PathEscape(projectID), perPage, page)
		if o, err := getObject(ctx, k.Client, endpoint, headers, &b); err != nil || o != nil {
			return nil, nil, false, o, err
		}
		// A board without data would read as an empty project and
		// undercount into a fail, so it is no-evidence like any other
		// answer the adapter cannot read.
		if b.Data == nil {
			return nil, nil, false, &Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("unreadable board for project %s: no data", projectID)}, nil
		}
		for _, c := range b.Data.Columns {
			slug := c.Slug
			if slug == "" {
				slug = c.ID
			}
			if c.IsFinal {
				final[slug] = true
			}
			for _, t := range c.Tasks {
				add(t, c.IsFinal)
			}
		}
		for _, t := range b.Data.ArchivedTasks {
			add(t, true)
		}
		for _, t := range b.Data.PlannedTasks {
			add(t, false)
		}
		if page >= b.Pagination.TotalPages {
			return tasks, final, false, nil, nil
		}
		if page >= limit {
			return tasks, final, true, nil, nil
		}
	}
}

// dated is when a done task finished or an open one was created. A done task
// finished at its newest move into a final column; a task imported from
// Vikunja has no such move, because the importer's events are stamped at the
// import, so its footer's done date stands in. An open task's createdAt is
// the import time when it was imported, so the footer's created date comes
// first. A done task with neither is no-evidence naming it: skipping it would
// undercount into a fail, which is how parseStamp treats a stamp it cannot
// read.
func (k *Kaneo) dated(ctx context.Context, headers map[string]string, task boardTask, final map[string]bool) (time.Time, *Outcome, error) {
	if task.done {
		var events []kaneoEvent
		if o, err := get(ctx, k.Client, fmt.Sprintf("%s/api/activity/%s", k.URL, url.PathEscape(task.ID)), headers, &events); err != nil || o != nil {
			return time.Time{}, o, err
		}
		var newest time.Time
		for _, e := range events {
			if e.Type != "status_changed" || !final[e.EventData.NewStatus] {
				continue
			}
			when, o := parseStamp("createdAt", e.CreatedAt)
			if o != nil {
				return time.Time{}, o, nil
			}
			if when.After(newest) {
				newest = when
			}
		}
		if !newest.IsZero() {
			return newest, nil, nil
		}
	}
	desc, o, err := k.description(ctx, headers, task.kaneoTask)
	if err != nil || o != nil {
		return time.Time{}, o, err
	}
	created, done := importFooter(desc)
	if task.done {
		if done.IsZero() {
			return time.Time{}, &Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("task %s is done with no completion date: no move into a final column and no done date in an import footer", task.ID)}, nil
		}
		return done, nil, nil
	}
	if !created.IsZero() {
		return created, nil, nil
	}
	when, o := parseStamp("createdAt", task.CreatedAt)
	return when, o, nil
}

// description is the task's description, read from the task itself when the
// board left it out: Kaneo defers one over 64 KiB.
func (k *Kaneo) description(ctx context.Context, headers map[string]string, task kaneoTask) (string, *Outcome, error) {
	if !task.DescriptionDeferred {
		return task.Description, nil, nil
	}
	var full struct {
		Description string `json:"description"`
	}
	if o, err := getObject(ctx, k.Client, fmt.Sprintf("%s/api/task/%s", k.URL, url.PathEscape(task.ID)), headers, &full); err != nil || o != nil {
		return "", o, err
	}
	return full.Description, nil, nil
}

var (
	footerCreated = regexp.MustCompile(`, created ([0-9]{4}-[0-9]{2}-[0-9]{2})\b`)
	footerDone    = regexp.MustCompile(`, done ([0-9]{4}-[0-9]{2}-[0-9]{2})\b`)
)

// importFooter reads the created and done dates from the footer an import
// from Vikunja ends a description with: a rule, then a line starting
// "*Imported from [Vikunja", each date present only when the import knew it.
// Only the first line after the description's last rule is read, so the same
// words quoted in the body are not taken for the footer. A date that is
// absent or not a real date is returned as the zero time.
func importFooter(desc string) (created, done time.Time) {
	desc = "\n" + strings.ReplaceAll(desc, "\r\n", "\n")
	i := strings.LastIndex(desc, "\n---\n")
	if i < 0 {
		return
	}
	line, _, _ := strings.Cut(desc[i+len("\n---\n"):], "\n")
	if !strings.HasPrefix(line, "*Imported from [Vikunja ") {
		return
	}
	parse := func(re *regexp.Regexp) time.Time {
		if m := re.FindStringSubmatch(line); m != nil {
			if t, err := time.Parse("2006-01-02", m[1]); err == nil {
				return t
			}
		}
		return time.Time{}
	}
	return parse(footerCreated), parse(footerDone)
}
