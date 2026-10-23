package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jenkins-x/go-scm/scm/driver/gitlab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitLabProviderListPullRequestsRequestsOnlyOpen(t *testing.T) {
	state := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state <- r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, "[]")
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client, err := gitlab.New(server.URL)
	require.NoError(t, err)

	p := &GitLabProvider{client: client}
	prs, err := p.ListPullRequests(t.Context(), Repository{Org: "example", Name: "infrastructure"})
	require.NoError(t, err)
	assert.Empty(t, prs)
	assert.Equal(t, "opened", <-state)
}
