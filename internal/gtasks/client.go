package gtasks

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/pashagolub/pagotask/internal/queue"
)

// Client creates tasks and resolves list titles to ids.
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
	t := &tasks.Task{Title: it.Title, Notes: it.Notes}
	if !it.Due.IsZero() {
		t.Due = it.Due.UTC().Format("2006-01-02T00:00:00.000Z")
	}
	_, err = svc.Tasks.Insert(listID, t).Context(ctx).Do()
	var gerr *googleapi.Error
	if errors.As(err, &gerr) && gerr.Code >= 400 && gerr.Code < 500 && gerr.Code != 401 && gerr.Code != 429 {
		return &queue.Permanent{Err: err}
	}
	return err
}
