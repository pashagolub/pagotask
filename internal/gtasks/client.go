package gtasks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/pashagolub/pagotask/internal/dates"
	"github.com/pashagolub/pagotask/internal/opentasks"
	"github.com/pashagolub/pagotask/internal/queue"
	"github.com/pashagolub/pagotask/internal/recent"
)

// Client creates, fetches and checks off tasks, and resolves list titles to ids.
type Client struct {
	auth  *Auth
	lists func() map[string]string // list key -> title, read live from config

	mu  sync.Mutex
	ids map[string]string // list title -> Google list id
}

// NewClient wires auth to the list-title lookup.
func NewClient(a *Auth, lists func() map[string]string) *Client {
	return &Client{auth: a, lists: lists, ids: map[string]string{}}
}

func (c *Client) service(ctx context.Context) (*tasks.Service, error) {
	hc, err := c.auth.Client(ctx)
	if err != nil {
		return nil, err
	}
	return tasks.NewService(ctx, option.WithHTTPClient(hc))
}

// RefreshLists fetches all task lists and caches title -> id.
func (c *Client) RefreshLists(ctx context.Context) error {
	svc, err := c.service(ctx)
	if err != nil {
		return err
	}
	ids := map[string]string{}
	err = svc.Tasklists.List().MaxResults(100).Pages(ctx, func(p *tasks.TaskLists) error {
		for _, l := range p.Items {
			ids[l.Title] = l.Id
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.ids = ids
	c.mu.Unlock()
	return nil
}

// ListID resolves a config list key to a Google list id, refreshing once on a miss.
func (c *Client) ListID(ctx context.Context, key string) (string, error) {
	title, ok := c.lists()[key]
	if !ok {
		return "", &queue.Permanent{Err: fmt.Errorf("list key %q is not in config", key)}
	}
	c.mu.Lock()
	id, ok := c.ids[title]
	c.mu.Unlock()
	if ok {
		return id, nil
	}
	if err := c.RefreshLists(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	id, ok = c.ids[title]
	c.mu.Unlock()
	if !ok {
		return "", &queue.Permanent{Err: fmt.Errorf("no Google Tasks list named %q", title)}
	}
	return id, nil
}

// Send implements queue.Sender.
func (c *Client) Send(ctx context.Context, it queue.Item) error {
	listID, err := c.ListID(ctx, it.ListKey)
	if err != nil {
		return err
	}
	svc, err := c.service(ctx)
	if err != nil {
		return err
	}
	switch it.Op {
	case queue.OpCreate:
		t := &tasks.Task{Title: it.Title, Notes: it.Notes}
		if !it.Due.IsZero() {
			// The due date is the local calendar day. Converting local midnight
			// to UTC first would give yesterday east of Greenwich.
			t.Due = dates.RFC3339(it.Due)
		}
		_, err = svc.Tasks.Insert(listID, t).Context(ctx).Do()
	case queue.OpComplete:
		_, err = svc.Tasks.Patch(listID, it.TaskID, &tasks.Task{Status: "completed"}).Context(ctx).Do()
	case queue.OpReopen:
		t := &tasks.Task{Status: "needsAction", NullFields: []string{"Completed"}}
		_, err = svc.Tasks.Patch(listID, it.TaskID, t).Context(ctx).Do()
	default:
		return &queue.Permanent{Err: fmt.Errorf("unknown queue op %q", it.Op)}
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code >= 400 && gerr.Code < 500 && gerr.Code != 401 && gerr.Code != 429 {
		return &queue.Permanent{Err: err}
	}
	return err
}

// OpenTasks fetches the open (unchecked) tasks of the given lists (key -> title).
func (c *Client) OpenTasks(ctx context.Context, lists map[string]string) ([]opentasks.Task, error) {
	svc, err := c.service(ctx)
	if err != nil {
		return nil, err
	}
	var out []opentasks.Task
	for key := range lists {
		listID, err := c.ListID(ctx, key)
		if err != nil {
			var perm *queue.Permanent
			if errors.As(err, &perm) {
				continue // a list in config that Google does not have: nothing to show
			}
			return nil, err
		}
		err = svc.Tasks.List(listID).ShowCompleted(false).ShowHidden(false).MaxResults(100).
			Pages(ctx, func(p *tasks.Tasks) error {
				for _, t := range p.Items {
					out = append(out, openTask(key, t))
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// RecentTasks fetches the tasks of the given lists (key -> title) that
// were changed since since, open or completed, for the recent list in the
// add popup. Updated is the best Google offers: it moves on edits and
// checks as well as on creation.
func (c *Client) RecentTasks(ctx context.Context, lists map[string]string, since time.Time) ([]recent.Entry, error) {
	svc, err := c.service(ctx)
	if err != nil {
		return nil, err
	}
	var out []recent.Entry
	for key := range lists {
		listID, err := c.ListID(ctx, key)
		if err != nil {
			var perm *queue.Permanent
			if errors.As(err, &perm) {
				continue
			}
			return nil, err
		}
		err = svc.Tasks.List(listID).UpdatedMin(since.UTC().Format(time.RFC3339)).
			ShowCompleted(true).ShowHidden(true).MaxResults(100).
			Pages(ctx, func(p *tasks.Tasks) error {
				for _, t := range p.Items {
					if t.Title == "" || t.Deleted {
						continue
					}
					used, _ := time.Parse(time.RFC3339, t.Updated)
					out = append(out, recent.Entry{Title: t.Title, List: key, Used: used})
				}
				return nil
			})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func openTask(listKey string, t *tasks.Task) opentasks.Task {
	o := opentasks.Task{ID: t.Id, ListKey: listKey, Title: t.Title, Notes: t.Notes, ParentID: t.Parent, Position: t.Position}
	if len(t.Due) >= 10 {
		o.Due = t.Due[:10]
	}
	o.Updated, _ = time.Parse(time.RFC3339, t.Updated)
	links := make([]string, 0, len(t.Links))
	for _, l := range t.Links {
		links = append(links, l.Link)
	}
	o.Link = opentasks.FirstLink(t.Notes, links)
	return o
}
