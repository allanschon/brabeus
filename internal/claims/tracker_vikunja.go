package claims

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allanschon/brabeus/internal/store"
)

// pageCap bounds every backend's paging, so a runaway source cannot hold a
// run for ever. A count short of min when the cap stopped paging is only a
// lower bound, and reads no-evidence rather than fail.
var pageCap = 200

// Vikunja counts tasks by label and date. It pages /tasks/all and filters
// itself: measured 2026-09-28, Vikunja 2.5.0 answered 400 to every filter=
// form tried, and a personal tracker is a few hundred tasks.
type Vikunja struct {
	URL, Token string
	Client     *http.Client
	perPage    int // tests lower it to exercise paging
}

func (v *Vikunja) Name() string { return "tracker" }

type vikunjaTask struct {
	Done    bool   `json:"done"`
	DoneAt  string `json:"done_at"`
	Created string `json:"created"`
	Labels  []struct {
		Title string `json:"title"`
	} `json:"labels"`
}

// Check counts done tasks by done_at, or open ones by created, carrying the
// label (case-insensitively) since the claim's date. A label on no task at
// all is no-evidence (§8.1: "the query matched nothing it could count"); a
// label on some task with none in the window is fail by count.
func (v *Vikunja) Check(ctx context.Context, args map[string]string, now time.Time) (Outcome, error) {
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
	perPage := v.perPage
	if perPage == 0 {
		perPage = 50
	}
	count, seenLabel := 0, false
	for page := 1; ; page++ {
		if page > pageCap {
			return exhausted(count, min, pageCap), nil
		}
		var tasks []vikunjaTask
		endpoint := fmt.Sprintf("%s/api/v1/tasks/all?per_page=%d&page=%d", v.URL, perPage, page)
		if o, err := get(ctx, v.Client, endpoint, map[string]string{"Authorization": "Bearer " + v.Token}, &tasks); err != nil {
			return Outcome{}, err
		} else if o != nil {
			return *o, nil
		}
		for _, task := range tasks {
			labelled := false
			for _, l := range task.Labels {
				labelled = labelled || strings.ToLower(l.Title) == label
			}
			if !labelled {
				continue
			}
			seenLabel = true
			if task.Done != wantDone {
				continue
			}
			stamp := task.Created
			if wantDone {
				stamp = task.DoneAt
			}
			if when, err := time.Parse(time.RFC3339, stamp); err == nil && !when.Before(since) {
				count++
			}
		}
		// Only an empty page is the end: a server whose page size is below
		// ours answers short pages, and stopping there would undercount.
		if len(tasks) == 0 {
			break
		}
	}
	if !seenLabel {
		return Outcome{State: store.NoEvidence, Detail: fmt.Sprintf("label %q unknown to the tracker", args["label"])}, nil
	}
	return counted(count, min), nil
}
