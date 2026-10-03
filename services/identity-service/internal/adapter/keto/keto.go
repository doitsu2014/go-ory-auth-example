// Package keto implements the Authorizer and RoleStore ports over the Keto
// v26 read (:4466) and write (:4467) HTTP APIs. Subjects are subject sets
// User:<identity-uuid> with an empty relation (verified against Keto v26.2.0).
package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// Namespaces and the single console object.
const (
	NamespaceConsole = "Console"
	NamespaceUser    = "User"
	ConsoleObject    = "main"
	defaultTimeout   = 2 * time.Second
	listPageSize     = 500
)

// Client implements app.Authorizer and app.RoleStore.
type Client struct {
	readURL  string
	writeURL string
	http     *http.Client
}

var (
	_ app.Authorizer = (*Client)(nil)
	_ app.RoleStore  = (*Client)(nil)
)

// New creates a Keto client.
func New(readURL, writeURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{readURL: strings.TrimRight(readURL, "/"), writeURL: strings.TrimRight(writeURL, "/"), http: hc}
}

type subjectSet struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
}

type tuple struct {
	Namespace  string      `json:"namespace"`
	Object     string      `json:"object"`
	Relation   string      `json:"relation"`
	SubjectID  *string     `json:"subject_id,omitempty"`
	SubjectSet *subjectSet `json:"subject_set,omitempty"`
}

func userSubject(id uuid.UUID) *subjectSet {
	return &subjectSet{Namespace: NamespaceUser, Object: id.String(), Relation: ""}
}

func consoleTuple(relation string, subject uuid.UUID) tuple {
	return tuple{Namespace: NamespaceConsole, Object: ConsoleObject, Relation: relation, SubjectSet: userSubject(subject)}
}

func (c *Client) do(ctx context.Context, method, u string, body any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	attempts := 1
	if method == http.MethodGet || (method == http.MethodPost && strings.Contains(u, "/check")) {
		attempts = 2 // idempotent reads only
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if seeker, ok := rdr.(io.Seeker); ok {
			_, _ = seeker.Seek(0, io.SeekStart)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: keto %s: %v", app.ErrDependencyUnavailable, method, err)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("%w: keto read: %v", app.ErrDependencyUnavailable, err)
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%w: keto %s status %d", app.ErrDependencyUnavailable, method, resp.StatusCode)
			continue
		}
		return resp.StatusCode, b, nil
	}
	return 0, nil, lastErr
}

// Check implements app.Authorizer: Console:main#<perm>@User:<subject>.
func (c *Client) Check(ctx context.Context, subject uuid.UUID, perm identity.Permission) (bool, error) {
	body := tuple{Namespace: NamespaceConsole, Object: ConsoleObject, Relation: string(perm), SubjectSet: userSubject(subject)}
	status, b, err := c.do(ctx, http.MethodPost, c.readURL+"/relation-tuples/check/openapi", body)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("keto check: status %d", status)
	}
	var r struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return false, fmt.Errorf("keto check decode: %w", err)
	}
	return r.Allowed, nil
}

type listResp struct {
	RelationTuples []tuple `json:"relation_tuples"`
	NextPageToken  string  `json:"next_page_token"`
}

func (c *Client) list(ctx context.Context, q url.Values) ([]tuple, error) {
	q.Set("namespace", NamespaceConsole)
	q.Set("object", ConsoleObject)
	q.Set("page_size", fmt.Sprint(listPageSize))
	var out []tuple
	for {
		status, b, err := c.do(ctx, http.MethodGet, c.readURL+"/relation-tuples?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("keto list: status %d", status)
		}
		var r listResp
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("keto list decode: %w", err)
		}
		out = append(out, r.RelationTuples...)
		if r.NextPageToken == "" {
			return out, nil
		}
		q.Set("page_token", r.NextPageToken)
	}
}

func subjectQuery(subject uuid.UUID) url.Values {
	q := url.Values{}
	q.Set("subject_set.namespace", NamespaceUser)
	q.Set("subject_set.object", subject.String())
	q.Set("subject_set.relation", "")
	return q
}

// RolesOf implements app.RoleStore.
func (c *Client) RolesOf(ctx context.Context, subject uuid.UUID) ([]identity.Role, error) {
	ts, err := c.list(ctx, subjectQuery(subject))
	if err != nil {
		return nil, err
	}
	roles := []identity.Role{}
	for _, t := range ts {
		if r, ok := identity.RoleFromRelation(t.Relation); ok {
			roles = append(roles, r)
		}
	}
	return roles, nil
}

// AllAssignments implements app.RoleStore.
func (c *Client) AllAssignments(ctx context.Context) (map[uuid.UUID][]identity.Role, error) {
	ts, err := c.list(ctx, url.Values{})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]identity.Role{}
	for _, t := range ts {
		if t.SubjectSet == nil || t.SubjectSet.Namespace != NamespaceUser || t.SubjectSet.Relation != "" {
			continue
		}
		id, err := uuid.Parse(t.SubjectSet.Object)
		if err != nil {
			continue
		}
		if r, ok := identity.RoleFromRelation(t.Relation); ok {
			out[id] = append(out[id], r)
		}
	}
	return out, nil
}

// CountRole implements app.RoleStore.
func (c *Client) CountRole(ctx context.Context, role identity.Role) (int, error) {
	rel, err := role.Relation()
	if err != nil {
		return 0, err
	}
	q := url.Values{}
	q.Set("relation", rel)
	ts, err := c.list(ctx, q)
	if err != nil {
		return 0, err
	}
	return len(ts), nil
}

type patchDelta struct {
	Action        string `json:"action"`
	RelationTuple tuple  `json:"relation_tuple"`
}

// SetRole implements app.RoleStore with one atomic PATCH (insert the new
// relation, delete the others).
func (c *Client) SetRole(ctx context.Context, subject uuid.UUID, role identity.Role) error {
	rel, err := role.Relation()
	if err != nil {
		return err
	}
	deltas := []patchDelta{{Action: "insert", RelationTuple: consoleTuple(rel, subject)}}
	for _, other := range identity.AllRoles {
		if other == role {
			continue
		}
		orel, _ := other.Relation()
		deltas = append(deltas, patchDelta{Action: "delete", RelationTuple: consoleTuple(orel, subject)})
	}
	status, _, err := c.do(ctx, http.MethodPatch, c.writeURL+"/admin/relation-tuples", deltas)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("keto patch: status %d", status)
	}
	return nil
}

// RemoveAll implements app.RoleStore.
func (c *Client) RemoveAll(ctx context.Context, subject uuid.UUID) error {
	q := subjectQuery(subject)
	q.Set("namespace", NamespaceConsole)
	q.Set("object", ConsoleObject)
	status, _, err := c.do(ctx, http.MethodDelete, c.writeURL+"/admin/relation-tuples?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("keto delete: status %d", status)
	}
	return nil
}

// Ready checks Keto read and write readiness.
func (c *Client) Ready(ctx context.Context) error {
	for _, base := range []string{c.readURL, c.writeURL} {
		status, _, err := c.do(ctx, http.MethodGet, base+"/health/ready", nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("keto not ready: %d", status)
		}
	}
	return nil
}
