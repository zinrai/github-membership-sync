package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	httpTimeout      = 30 * time.Second
	maxResponseBytes = 10 << 20

	// defaultAPIBase is the REST API root of github.com. Enterprise Server
	// serves the API somewhere else, and the operator passes that root as
	// written rather than a hostname this program would have to map.
	defaultAPIBase = "https://api.github.com"
)

// Reads go through GraphQL, which can leave out the members a team inherits
// from its child teams; REST lists them without marking them as inherited.
// Writes go through REST, as GraphQL has no membership mutations.
type client struct {
	restBase   string
	graphqlURL string
	tokenFile  string
	http       *http.Client
}

func newClient(apiBase, tokenFile string) *client {
	return &client{
		restBase:   apiBase,
		graphqlURL: graphqlURL(apiBase),
		tokenFile:  tokenFile,
		http:       &http.Client{Timeout: httpTimeout},
	}
}

// graphqlURL derives the endpoint from the REST root rather than taking a
// second flag that could point at another server.
func graphqlURL(apiBase string) string {
	if root, ok := strings.CutSuffix(apiBase, "/api/v3"); ok {
		return root + "/api/graphql"
	}
	return apiBase + "/graphql"
}

func (c *client) fetch(ctx context.Context, org string, slugs []string) (*membership, error) {
	members, err := c.orgMembers(ctx, org)
	if err != nil {
		return nil, err
	}
	m := &membership{Members: members, Teams: make(map[string]map[string]string, len(slugs))}
	for _, slug := range slugs {
		team, err := c.teamMembers(ctx, org, slug)
		if err != nil {
			return nil, err
		}
		m.Teams[slug] = team
	}
	return m, nil
}

func (c *client) apply(ctx context.Context, org string, ch change) error {
	path := "/orgs/" + url.PathEscape(org)
	if ch.team != "" {
		path += "/teams/" + url.PathEscape(ch.team)
	}
	path += "/memberships/" + url.PathEscape(ch.login)

	var err error
	if ch.op == opRemove {
		_, err = c.do(ctx, http.MethodDelete, c.restBase+path, nil)
	} else {
		_, err = c.do(ctx, http.MethodPut, c.restBase+path, map[string]string{"role": ch.to})
	}
	return err
}

const orgMembersQuery = `query($org: String!, $after: String) {
  organization(login: $org) {
    membersWithRole(first: 100, after: $after) {
      pageInfo { hasNextPage endCursor }
      edges { role node { login } }
    }
  }
}`

func (c *client) orgMembers(ctx context.Context, org string) (map[string]string, error) {
	members := make(map[string]string)
	err := paginate(func(after *string) (pageInfo, error) {
		var data struct {
			Organization struct{ MembersWithRole memberConnection }
		}
		if err := c.graphql(ctx, orgMembersQuery, map[string]any{"org": org, "after": after}, &data); err != nil {
			return pageInfo{}, err
		}
		return data.Organization.MembersWithRole.addTo(members), nil
	})
	return members, err
}

const teamMembersQuery = `query($org: String!, $slug: String!, $after: String) {
  organization(login: $org) {
    team(slug: $slug) {
      members(first: 100, after: $after, membership: IMMEDIATE) {
        pageInfo { hasNextPage endCursor }
        edges { role node { login } }
      }
    }
  }
}`

func (c *client) teamMembers(ctx context.Context, org, slug string) (map[string]string, error) {
	members := make(map[string]string)
	err := paginate(func(after *string) (pageInfo, error) {
		var data struct {
			Organization struct {
				Team *struct{ Members memberConnection }
			}
		}
		vars := map[string]any{"org": org, "slug": slug, "after": after}
		if err := c.graphql(ctx, teamMembersQuery, vars, &data); err != nil {
			return pageInfo{}, err
		}
		// Read as empty instead, a misspelt slug would be planned around
		// rather than reported, as this program does not create teams.
		if data.Organization.Team == nil {
			return pageInfo{}, fmt.Errorf("team %s not found in %s", slug, org)
		}
		return data.Organization.Team.Members.addTo(members), nil
	})
	return members, err
}

const teamSlugsQuery = `query($org: String!, $after: String) {
  organization(login: $org) {
    teams(first: 100, after: $after) {
      pageInfo { hasNextPage endCursor }
      nodes { slug }
    }
  }
}`

func (c *client) teamSlugs(ctx context.Context, org string) ([]string, error) {
	var slugs []string
	err := paginate(func(after *string) (pageInfo, error) {
		var data struct {
			Organization struct {
				Teams struct {
					PageInfo pageInfo
					Nodes    []struct{ Slug string }
				}
			}
		}
		if err := c.graphql(ctx, teamSlugsQuery, map[string]any{"org": org, "after": after}, &data); err != nil {
			return pageInfo{}, err
		}
		for _, n := range data.Organization.Teams.Nodes {
			slugs = append(slugs, n.Slug)
		}
		return data.Organization.Teams.PageInfo, nil
	})
	return slugs, err
}

type pageInfo struct {
	HasNextPage bool
	EndCursor   string
}

func paginate(page func(after *string) (pageInfo, error)) error {
	var after *string
	for {
		info, err := page(after)
		if err != nil {
			return err
		}
		if !info.HasNextPage {
			return nil
		}
		after = &info.EndCursor
	}
}

type memberConnection struct {
	PageInfo pageInfo
	Edges    []struct {
		Role string
		Node struct{ Login string }
	}
}

func (conn *memberConnection) addTo(members map[string]string) pageInfo {
	for _, e := range conn.Edges {
		members[e.Node.Login] = strings.ToLower(e.Role)
	}
	return conn.PageInfo
}

func (c *client) graphql(ctx context.Context, query string, vars map[string]any, data any) error {
	raw, err := c.do(ctx, http.MethodPost, c.graphqlURL, map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	var resp struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return err
	}
	// GitHub answers 200 with partial data next to the errors. Taken as it is,
	// a team missing from that data would read as a team with nobody in it.
	if len(resp.Errors) > 0 {
		return fmt.Errorf("POST %s: %s", c.graphqlURL, resp.Errors[0].Message)
	}
	return json.Unmarshal(resp.Data, data)
}

func (c *client) do(ctx context.Context, method, u string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, err
	}
	// Read for every request rather than once, as the file may be replaced
	// with a fresh token during a run.
	tok, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "github-membership-sync/"+version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, apiError(raw))
	}
	return raw, nil
}

// What GitHub says here is the whole of what an operator has to work from, so
// it is worth digging out of the payload rather than reporting the status code
// alone.
func apiError(raw []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &payload)
	if payload.Message != "" {
		return payload.Message
	}
	return strings.TrimSpace(string(raw))
}
