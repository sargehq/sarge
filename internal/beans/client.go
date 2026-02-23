package beans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/logging"
)

// Dependency represents a blocking relationship (this bean is blocked by another).
type Dependency struct {
	BeanID      string // the bean that is blocked
	BlockedByID string // the bean doing the blocking
	Status      string // status of the blocking bean
	Title       string // title of the blocking bean
}

// Dependent represents a bean that this bean is blocking.
type Dependent struct {
	BeanID     string // the bean being blocked
	BlockerID  string // this bean (the one doing the blocking)
	Type       string // "blocking" or "parent-child"
	Status     string // status of the blocked bean
	Title      string // title of the blocked bean
}

// BeanWithDeps bundles a bean with its relationships.
type BeanWithDeps struct {
	*Bean
	Dependencies []Dependency // beans that block this one
	Dependents   []Dependent  // beans that this one blocks, or children
}

// BeansWithDepsResult holds the result of GetBeansWithDeps.
type BeansWithDepsResult struct {
	Beans        map[string]Bean
	Dependencies map[string][]Dependency // beanID -> beans blocking it
	Dependents   map[string][]Dependent  // beanID -> beans it blocks / children
}

// GetBean returns a single BeanWithDeps from the result, or nil if not found.
func (r *BeansWithDepsResult) GetBean(id string) *BeanWithDeps {
	bean, ok := r.Beans[id]
	if !ok {
		return nil
	}
	return &BeanWithDeps{
		Bean:         &bean,
		Dependencies: r.Dependencies[id],
		Dependents:   r.Dependents[id],
	}
}

// Client reads beans data via the beans CLI and GraphQL.
// No sqlite dependency — all reads go through the beans CLI.
type Client struct {
	beansDir string
}

// ClientConfig holds configuration for the Client.
type ClientConfig struct {
	BeansDir string
}

// NewClient creates a new beans reader client.
func NewClient(_ context.Context, cfg ClientConfig) (*Client, error) {
	return &Client{
		beansDir: cfg.BeansDir,
	}, nil
}

// Close is a no-op for the CLI-based client (no database connection to close).
func (c *Client) Close() error {
	return nil
}

// FlushCache is a no-op — the CLI-based client has no cache.
func (c *Client) FlushCache(_ context.Context) error {
	return nil
}

// graphqlBeanResult is the JSON structure from beans graphql queries.
type graphqlBeanResult struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Path         string   `json:"path"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Type         string   `json:"type"`
	Priority     string   `json:"priority"`
	Body         string   `json:"body"`
	Tags         []string `json:"tags"`
	ParentID     *string  `json:"parentId"`
	BlockedByIDs []string `json:"blockedByIds"`
	BlockingIDs  []string `json:"blockingIds"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
	Etag         string   `json:"etag"`

	// Nested relationships (when queried)
	BlockedBy []graphqlBeanResult `json:"blockedBy,omitempty"`
	Blocking  []graphqlBeanResult `json:"blocking,omitempty"`
	Children  []graphqlBeanResult `json:"children,omitempty"`
}

func (g *graphqlBeanResult) toBean() Bean {
	b := Bean{
		ID:           g.ID,
		Slug:         g.Slug,
		Path:         g.Path,
		Title:        g.Title,
		Body:         g.Body,
		Status:       g.Status,
		Priority:     g.Priority,
		Type:         g.Type,
		Tags:         g.Tags,
		Etag:         g.Etag,
		BlockedByIDs: g.BlockedByIDs,
		BlockingIDs:  g.BlockingIDs,
		IsEpic:       g.Type == "epic",
	}
	if g.ParentID != nil {
		b.ParentID = *g.ParentID
	}
	if t, err := time.Parse(time.RFC3339, g.CreatedAt); err == nil {
		b.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, g.UpdatedAt); err == nil {
		b.UpdatedAt = t
	}
	return b
}

// graphqlQuery executes a GraphQL query and returns the raw JSON response.
func (c *Client) graphqlQuery(ctx context.Context, query string) (json.RawMessage, error) {
	cmd := beansCommand(ctx, c.beansDir, "graphql", "--json", query)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("graphql query failed: %w\n%s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("graphql query failed: %w", err)
	}
	return output, nil
}

// GetBeansWithDeps retrieves beans and their relationships via GraphQL.
func (c *Client) GetBeansWithDeps(ctx context.Context, beanIDs []string) (*BeansWithDepsResult, error) {
	if len(beanIDs) == 0 {
		return &BeansWithDepsResult{
			Beans:        make(map[string]Bean),
			Dependencies: make(map[string][]Dependency),
			Dependents:   make(map[string][]Dependent),
		}, nil
	}

	// Build a GraphQL query using aliased bean(id:) queries.
	// The BeanFilter type does not support filtering by ID, so we use
	// individual bean(id:) lookups with aliases to batch them.
	const beanFields = `id slug path title status type priority body tags
			parentId blockedByIds blockingIds
			createdAt updatedAt etag
			blockedBy { id title status }
			blocking { id title status }
			children { id title status }`

	var queryParts []string
	aliasMap := make(map[string]string, len(beanIDs)) // alias -> beanID
	for i, id := range beanIDs {
		alias := fmt.Sprintf("b%d", i)
		aliasMap[alias] = id
		queryParts = append(queryParts, fmt.Sprintf(`%s: bean(id: %q) { %s }`, alias, id, beanFields))
	}
	query := "{ " + strings.Join(queryParts, "\n") + " }"

	raw, err := c.graphqlQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("fetching beans: %w", err)
	}

	// Parse the aliased response: { "b0": {...}, "b1": {...}, ... }
	var aliasedResp map[string]*graphqlBeanResult
	if err := json.Unmarshal(raw, &aliasedResp); err != nil {
		return nil, fmt.Errorf("parsing beans response: %w", err)
	}

	var resp []graphqlBeanResult
	for _, result := range aliasedResp {
		if result != nil {
			resp = append(resp, *result)
		}
	}

	beansMap := make(map[string]Bean, len(resp))
	depsMap := make(map[string][]Dependency)
	dependentsMap := make(map[string][]Dependent)

	for _, gb := range resp {
		bean := gb.toBean()
		beansMap[bean.ID] = bean

		// Build dependencies (beans that block this one)
		for _, blocker := range gb.BlockedBy {
			depsMap[bean.ID] = append(depsMap[bean.ID], Dependency{
				BeanID:      bean.ID,
				BlockedByID: blocker.ID,
				Status:      blocker.Status,
				Title:       blocker.Title,
			})
		}

		// Build dependents (beans this one is blocking)
		for _, blocked := range gb.Blocking {
			dependentsMap[bean.ID] = append(dependentsMap[bean.ID], Dependent{
				BeanID:    blocked.ID,
				BlockerID: bean.ID,
				Type:      "blocking",
				Status:    blocked.Status,
				Title:     blocked.Title,
			})
		}

		// Children are also dependents (parent-child relationship)
		for _, child := range gb.Children {
			dependentsMap[bean.ID] = append(dependentsMap[bean.ID], Dependent{
				BeanID:    child.ID,
				BlockerID: bean.ID,
				Type:      "parent-child",
				Status:    child.Status,
				Title:     child.Title,
			})
		}
	}

	return &BeansWithDepsResult{
		Beans:        beansMap,
		Dependencies: depsMap,
		Dependents:   dependentsMap,
	}, nil
}

// GetBean retrieves a single bean by ID with its relationships.
func (c *Client) GetBean(ctx context.Context, id string) (*BeanWithDeps, error) {
	result, err := c.GetBeansWithDeps(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	return result.GetBean(id), nil
}

// listBeanResult is the JSON structure from beans list --json.
type listBeanResult struct {
	ID        string   `json:"id"`
	Slug      string   `json:"slug"`
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Type      string   `json:"type"`
	Priority  string   `json:"priority"`
	Tags      []string `json:"tags"`
	Parent    string   `json:"parent"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Etag      string   `json:"etag"`
}

func (l *listBeanResult) toBean() Bean {
	b := Bean{
		ID:       l.ID,
		Slug:     l.Slug,
		Path:     l.Path,
		Title:    l.Title,
		Status:   l.Status,
		Priority: l.Priority,
		Type:     l.Type,
		Tags:     l.Tags,
		ParentID: l.Parent,
		Etag:     l.Etag,
		IsEpic:   l.Type == "epic",
	}
	if t, err := time.Parse(time.RFC3339, l.CreatedAt); err == nil {
		b.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, l.UpdatedAt); err == nil {
		b.UpdatedAt = t
	}
	return b
}

// ListBeans lists all beans with optional status filter.
func (c *Client) ListBeans(ctx context.Context, status string) ([]Bean, error) {
	args := []string{"list", "--json"}
	if status != "" {
		args = append(args, "--status", status)
	}

	cmd := beansCommand(ctx, c.beansDir, args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("beans list failed: %w\n%s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("beans list failed: %w", err)
	}

	var results []listBeanResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("parsing beans list response: %w", err)
	}

	beans := make([]Bean, len(results))
	for i, r := range results {
		beans[i] = r.toBean()
	}
	return beans, nil
}

// GetReadyBeans returns all beans available to start (not blocked, todo status).
func (c *Client) GetReadyBeans(ctx context.Context) ([]Bean, error) {
	args := []string{"list", "--json", "--ready"}

	cmd := beansCommand(ctx, c.beansDir, args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("beans list --ready failed: %w\n%s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("beans list --ready failed: %w", err)
	}

	var results []listBeanResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("parsing ready beans response: %w", err)
	}

	beans := make([]Bean, len(results))
	for i, r := range results {
		beans[i] = r.toBean()
	}
	return beans, nil
}

// GetTransitiveDependencies collects all transitive blockers for a bean.
func (c *Client) GetTransitiveDependencies(ctx context.Context, id string) ([]Bean, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(beanID string) error
	collect = func(beanID string) error {
		if visited[beanID] {
			return nil
		}
		visited[beanID] = true

		result, err := c.GetBeansWithDeps(ctx, []string{beanID})
		if err != nil {
			return err
		}

		// Recursively collect all blockers first
		for _, dep := range result.Dependencies[beanID] {
			if !visited[dep.BlockedByID] {
				if err := collect(dep.BlockedByID); err != nil {
					return err
				}
			}
		}

		orderedIDs = append(orderedIDs, beanID)
		return nil
	}

	if err := collect(id); err != nil {
		return nil, err
	}

	// Fetch all collected beans in one call
	result, err := c.GetBeansWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	beans := make([]Bean, 0, len(orderedIDs))
	for _, beanID := range orderedIDs {
		if bean, ok := result.Beans[beanID]; ok {
			beans = append(beans, bean)
		}
	}
	return beans, nil
}

// GetBeanWithChildren retrieves a bean and all its child beans recursively.
func (c *Client) GetBeanWithChildren(ctx context.Context, id string) ([]Bean, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(beanID string) error
	collect = func(beanID string) error {
		if visited[beanID] {
			return nil
		}
		visited[beanID] = true
		orderedIDs = append(orderedIDs, beanID)

		result, err := c.GetBeansWithDeps(ctx, []string{beanID})
		if err != nil {
			return err
		}

		for _, dep := range result.Dependents[beanID] {
			if dep.Type == "parent-child" && !visited[dep.BeanID] {
				if err := collect(dep.BeanID); err != nil {
					return err
				}
			}
		}

		return nil
	}

	if err := collect(id); err != nil {
		return nil, err
	}

	result, err := c.GetBeansWithDeps(ctx, orderedIDs)
	if err != nil {
		return nil, err
	}

	beans := make([]Bean, 0, len(orderedIDs))
	for _, beanID := range orderedIDs {
		if bean, ok := result.Beans[beanID]; ok {
			beans = append(beans, bean)
		}
	}
	return beans, nil
}

// CloseEligibleParents closes any parent beans where all children are complete.
func (c *Client) CloseEligibleParents(ctx context.Context, beansDir string) error {
	// Get all beans
	allBeans, err := c.ListBeans(ctx, "")
	if err != nil {
		return fmt.Errorf("fetching all beans: %w", err)
	}

	if len(allBeans) == 0 {
		return nil
	}

	allIDs := make([]string, len(allBeans))
	for i, b := range allBeans {
		allIDs[i] = b.ID
	}

	result, err := c.GetBeansWithDeps(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("fetching beans with deps: %w", err)
	}

	closed := make(map[string]bool)
	cli := NewCLI(beansDir)

	for {
		var toClose []string

		for id, bean := range result.Beans {
			if IsTerminalStatus(bean.Status) || closed[id] {
				continue
			}

			dependents := result.Dependents[id]
			var children []string
			for _, dep := range dependents {
				if dep.Type == "parent-child" {
					children = append(children, dep.BeanID)
				}
			}

			if len(children) == 0 {
				continue
			}

			allChildrenDone := true
			for _, childID := range children {
				child, ok := result.Beans[childID]
				if !ok || (!IsTerminalStatus(child.Status) && !closed[childID]) {
					allChildrenDone = false
					break
				}
			}

			if allChildrenDone {
				toClose = append(toClose, id)
			}
		}

		if len(toClose) == 0 {
			break
		}

		for _, id := range toClose {
			if err := cli.Close(ctx, id); err != nil {
				logging.Warn("failed to close parent bean", "id", id, "error", err)
				continue
			}
			closed[id] = true
			logging.Debug("closed parent bean", "id", id)
		}
	}

	return nil
}

// Compile-time check that Client implements Reader.
var _ Reader = (*Client)(nil)
