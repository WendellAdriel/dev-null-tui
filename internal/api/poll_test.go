package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestPollPayloadFollowsServerLimits(t *testing.T) {
	cases := map[string]struct {
		poll NewPoll
		ok   bool
	}{
		"two options":        {NewPoll{Options: []string{"Tabs", "Spaces"}, Duration: 1440}, true},
		"four options":       {NewPoll{Options: []string{"a", "b", "c", "d"}, Duration: 60}, true},
		"one option":         {NewPoll{Options: []string{"Tabs"}, Duration: 60}, false},
		"five options":       {NewPoll{Options: []string{"a", "b", "c", "d", "e"}, Duration: 60}, false},
		"blank option":       {NewPoll{Options: []string{"Tabs", "  "}, Duration: 60}, false},
		"50 characters":      {NewPoll{Options: []string{strings.Repeat("界", 50), "b"}, Duration: 60}, true},
		"51 characters":      {NewPoll{Options: []string{strings.Repeat("界", 51), "b"}, Duration: 60}, false},
		"same ignoring case": {NewPoll{Options: []string{"Tabs", " tabs "}, Duration: 60}, false},
		"invisible only":     {NewPoll{Options: []string{"Tabs", "\u200b"}, Duration: 60}, false},
		"same but invisible": {NewPoll{Options: []string{"Tabs", "Tabs\u200b"}, Duration: 60}, false},
		"unknown duration":   {NewPoll{Options: []string{"a", "b"}, Duration: 120}, false},
	}
	for name, c := range cases {
		if err := ValidatePoll(c.poll); (err == nil) != c.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCreateCommitRefusesPollCombinationsBeforeSending(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	key, _ := auth.GenerateKey()
	client, _ := NewClient(server.URL, key)
	poll := &NewPoll{Options: []string{"a", "b"}, Duration: 60}
	image := ImageFile{Upload: auth.Upload{Name: "a.png", Type: "image/png", Data: []byte("x")}}
	for name, draft := range map[string]NewCommit{
		"reply":  {Body: "Synthetic", ParentID: "parent", Poll: poll},
		"quote":  {Body: "Synthetic", QuotedID: "quoted", Poll: poll},
		"images": {Body: "Synthetic", Poll: poll, Images: []ImageFile{image}},
		"four":   {Body: "Synthetic", Images: []ImageFile{image, image, image, image}},
		"empty":  {Body: " "},
	} {
		if _, err := client.CreateCommit(t.Context(), draft); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if calls != 0 {
		t.Fatalf("sent %d refused drafts", calls)
	}
}

func TestCreateCommitSendsTrimmedPollAsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"body":"Pick one","poll":{"options":["Tabs","Spaces"],"duration":4320}}` {
			t.Errorf("payload %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"c1","author":{"id":"a","handle":"a"},"status":"pending","poll":{"id":"p","closes_at":null,"is_closed":false,"has_voted":false,"total_votes":0,"options":[{"id":"o1","label":"Tabs","votes_count":0,"voted":false,"percentage":0},{"id":"o2","label":"Spaces","votes_count":0,"voted":false,"percentage":0}]}}}`)
	}))
	defer server.Close()
	key, _ := auth.GenerateKey()
	client, _ := NewClient(server.URL, key)
	commit, err := client.CreateCommit(t.Context(), NewCommit{Body: "Pick one", Poll: &NewPoll{Options: []string{" Tabs ", "Spaces"}, Duration: 4320}})
	if err != nil || commit.Poll == nil || commit.Poll.ClosesAt != nil || len(commit.Poll.Options) != 2 {
		t.Fatalf("created poll: %+v %v", commit.Poll, err)
	}
}

func TestCreateCommitWithImagesSendsDescriptionsInMultipartPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		var data []byte
		images := 0
		for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
			switch part.FormName() {
			case "payload":
				data, _ = io.ReadAll(part)
			case "images[]":
				images++
			}
		}
		if string(data) != `{"body":"","image_descriptions":["A chart",null]}` || images != 2 || r.URL.Path != "/api/v1/commits" {
			t.Errorf("payload %s images %d", data, images)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"c1","author":{"id":"a","handle":"a"},"status":"pending"}}`)
	}))
	defer server.Close()
	key, _ := auth.GenerateKey()
	client, _ := NewClient(server.URL, key)
	images := []ImageFile{
		{Upload: auth.Upload{Name: "a.png", Type: "image/png", Data: []byte("a")}, Description: " A chart "},
		{Upload: auth.Upload{Name: "b.png", Type: "image/png", Data: []byte("b")}, Description: "  "},
	}
	if _, err := client.CreateCommit(t.Context(), NewCommit{Images: images}); err != nil {
		t.Fatal(err)
	}
}

func TestVoteReturnsTheUpdatedPollAndKeepsRefusals(t *testing.T) {
	var refusal bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/v1/commits/c1/poll/votes" || string(body) != `{"option":"o2"}` {
			t.Errorf("vote %s %s", r.URL.Path, body)
		}
		if refusal {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"errors":{"option":["This poll has closed."]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"c1","author":{"id":"a","handle":"a"},"status":"approved","poll":{"id":"p","closes_at":"2026-11-03T10:00:00Z","is_closed":false,"has_voted":true,"total_votes":3,"options":[{"id":"o1","label":"Tabs","votes_count":1,"voted":false,"percentage":33},{"id":"o2","label":"Spaces","votes_count":2,"voted":true,"percentage":67}]}}}`)
	}))
	defer server.Close()
	key, _ := auth.GenerateKey()
	client, _ := NewClient(server.URL, key)
	commit, err := client.Vote(t.Context(), "c1", "o2")
	if err != nil || !commit.Poll.HasVoted || *commit.Poll.TotalVotes != 3 || *commit.Poll.Options[1].Percentage != 67 {
		t.Fatalf("vote: %+v %v", commit.Poll, err)
	}
	refusal = true
	_, err = client.Vote(t.Context(), "c1", "o2")
	var failure *auth.APIError
	if !errors.As(err, &failure) || len(failure.FieldErrors) != 1 || failure.FieldErrors[0] != "This poll has closed." {
		t.Fatalf("refusal: %v", err)
	}
}

func TestPollDecodesHiddenResultsAndOrganizationAuthors(t *testing.T) {
	var commit Commit
	data := `{"id":"c","author":{"type":"organization","id":"o","handle":"acme"},"poll":{"id":"p","closes_at":"2026-11-03T10:00:00Z","is_closed":false,"has_voted":false,"total_votes":null,"options":[{"id":"o1","label":"a","votes_count":null,"voted":false,"percentage":null},{"id":"o2","label":"b","votes_count":null,"voted":false,"percentage":null}]}}`
	if err := json.Unmarshal([]byte(data), &commit); err != nil {
		t.Fatal(err)
	}
	if !commit.Author.Organization() || commit.Poll.TotalVotes != nil || commit.Poll.Options[0].Votes != nil || !commit.Poll.Valid() {
		t.Fatalf("decoded %+v %+v", commit.Author, commit.Poll)
	}
	if (&Author{Type: "user"}).Organization() || (&Author{}).Organization() {
		t.Fatal("users became organizations")
	}
}
