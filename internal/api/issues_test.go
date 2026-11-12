package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestOpenIssueSendsTheCommandBodyAndEnvironment(t *testing.T) {
	var sent map[string]any
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/issues" || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"number":1555,"url":"https://github.com/dev-null-sh/dev-null.sh/issues/1555"}`)
	})
	issue, err := client.OpenIssue(t.Context(), NewIssue{Command: "/bug", Body: "# Crash\n\nIt breaks.", Environment: map[string]string{"os": "linux/amd64", "page": ""}})
	if err != nil || issue.Number != 1555 || !strings.HasSuffix(issue.URL, "/issues/1555") {
		t.Fatalf("issue %+v, %v", issue, err)
	}
	environment, _ := sent["environment"].(map[string]any)
	if sent["command"] != "/bug" || sent["body"] != "# Crash\n\nIt breaks." || environment["os"] != "linux/amd64" || environment["page"] != nil {
		t.Fatalf("sent %v", sent)
	}
}

func TestNewIssueValidatesLikeTheWeb(t *testing.T) {
	for _, draft := range []NewIssue{
		{Command: "/idea", Body: "A good idea"},
		{Command: "/feature", Body: "  ab  "},
		{Command: "/feature", Body: strings.Repeat("x", MaxBodyLength+1)},
		{Command: "/bug", Body: "It breaks", Environment: map[string]string{"os": strings.Repeat("x", 101)}},
	} {
		if draft.Validate() == nil {
			t.Errorf("accepted %+v", draft.Command)
		}
	}
	if err := (NewIssue{Command: "/feature", Body: "abc"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenIssueRefusesAMalformedAnswerAndKeepsServerErrors(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"number":0}`)
	})
	if _, err := client.OpenIssue(t.Context(), NewIssue{Command: "/bug", Body: "It breaks"}); err == nil {
		t.Fatal("accepted an answer without an issue")
	}
	client = fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"code":"github_issue_unanswered","message":"GitHub didn't answer."}`)
	})
	_, err := client.OpenIssue(t.Context(), NewIssue{Command: "/bug", Body: "It breaks"})
	var failure *auth.APIError
	if !errors.As(err, &failure) || failure.Code != "github_issue_unanswered" {
		t.Fatalf("lost the server's error code: %v", err)
	}
}
