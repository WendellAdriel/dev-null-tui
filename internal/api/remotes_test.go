package api

import (
	"io"
	"net/http"
	"testing"
)

const orgUUID = "9f1c2d3e-4b5a-6789-abcd-ef0123456789"

func TestInboxSendsTabSearchAndCursorAndReadsBothKinds(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/remotes" || query.Get("tab") != "stashed" || query.Get("q") != "ada" || query.Get("before") != orgUUID || r.Header.Get("X-SSH-Signature") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"data":[
			{"id":"c1","kind":"direct","name":null,"partner":{"id":"u1","handle":"ada","name":"Ada"},"members":null,"last_message":{"id":7,"user_id":"u1","body":"ship it","created_at":"2026-11-03T10:00:00Z"},"last_message_at":"2026-11-03T10:00:00Z","ahead":2,"last_incoming_message_id":7,"last_incoming_sender":"Ada","partner_read_through_id":0,"partner_read_at":null,"is_archived":true,"pinned_at":"2026-11-02T09:00:00Z","muted_until_at":null},
			{"id":"`+orgUUID+`","kind":"org","name":"core","partner":null,"members":[{"user":{"id":"me","handle":"me","name":"Me"},"role":"maintainer"},{"user":{"id":"u2","handle":"bob","name":"Bob"},"role":"contributor"}],"last_message":null,"last_message_at":null,"ahead":0,"last_incoming_message_id":null,"last_incoming_sender":null,"partner_read_through_id":0,"partner_read_at":null,"is_archived":false,"pinned_at":null,"muted_until_at":"2026-11-04T00:00:00Z"}
		],"has_more":true,"unread_count":3,"requests_count":1}`)
	})
	inbox, err := client.Inbox(t.Context(), Stashed, "ada", orgUUID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Data) != 2 || !inbox.HasMore || inbox.UnreadCount != 3 || inbox.RequestsCount != 1 {
		t.Fatalf("inbox: %+v", inbox)
	}
	direct, org := inbox.Data[0], inbox.Data[1]
	if direct.Partner.Handle != "ada" || direct.Ahead != 2 || direct.LastMessage.ID != 7 || !direct.IsArchived || direct.PinnedAt == nil || direct.Target() != (Target{Handle: "ada"}) {
		t.Fatalf("direct row: %+v", direct)
	}
	if org.Kind != "org" || len(org.Members) != 2 || org.Members[0].Role != "maintainer" || org.MutedUntilAt == nil || org.Target() != (Target{Org: orgUUID}) {
		t.Fatalf("org row: %+v", org)
	}
}

func TestInboxRejectsUnknownTabsAndInvalidRows(t *testing.T) {
	client := fixtureClient(t, func(http.ResponseWriter, *http.Request) { t.Error("sent a request for an unknown tab") })
	if _, err := client.Inbox(t.Context(), InboxTab("all"), "", ""); err == nil {
		t.Fatal("accepted an unknown tab")
	}
	for _, body := range []string{
		`{}`,
		`{"data":[{"id":"","kind":"direct","partner":{"id":"u","handle":"ada"}}]}`,
		`{"data":[{"id":"c","kind":"direct","partner":null}]}`,
		`{"data":[{"id":"c","kind":"org","members":null}]}`,
		`{"data":[{"id":"c","kind":"group"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := client.Inbox(t.Context(), Open, "", ""); err == nil {
				t.Fatal("accepted an invalid inbox")
			}
		})
	}
}

func TestDirectThreadSendsPollingCursorsAndReadsMessages(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/api/v1/remotes/@ada/messages" || query.Get("after") != "7" || query.Get("reactions_after") != "3" || query.Has("before") {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"partner":{"id":"u1","handle":"ada","name":"Ada"},"messages":[
			{"id":8,"user_id":"u1","client_id":"c-8","body":"did @bob see https://laravel.com","body_segments":[{"type":"text","text":"did @bob see https://laravel.com"}],"mentions":[{"handle":"bob","offset":4,"length":4,"kind":"user"}],"reply_to_id":7,"reply_to":{"id":7,"user_id":"me","body":"look","images_count":1,"has_voice":false},"link_preview":{"url":"https://laravel.com","title":"Laravel","description":"PHP","image_url":"https://laravel.com/og.png","site_name":"Laravel","is_pending":false},"images":[{"id":"0195f1c2-8a4b-7c3d-9e2f-1a2b3c4d5e6f","width":10,"height":5,"description":"diff","url":"/api/v1/remotes/images/1?signature=x"}],"voice":{"id":"0195f1c2-8a4b-7c3d-9e2f-6f5e4d3c2b1a","duration_ms":1500,"waveform":[0,50,100],"url":"/api/v1/remotes/voices/2?signature=y"},"reactions":[{"user_id":"me","emoji":"❤️"}],"event":null,"subject_id":null,"pinned_at":null,"created_at":"2026-11-03T10:01:00Z"}
		],"has_older":false,"reactions_version":4,"reaction_changes":[{"id":5,"reactions":[{"user_id":"u1","emoji":"✅"}]}],"read_through_id":8,"partner_read_through_id":6,"partner_read_at":"2026-11-03T10:00:30Z","partner_typing":true,"is_archived":false,"pinned_at":null,"muted_until_at":null,"access":"open"}}`)
	})
	thread, err := client.RemoteThread(t.Context(), Target{Handle: "ada"}, ThreadQuery{After: 7, ReactionsAfter: 3})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Partner.Handle != "ada" || thread.ReactionsVersion != 4 || thread.ReadThroughID != 8 || thread.PartnerReadThroughID != 6 || !thread.PartnerTyping || thread.Access != "open" {
		t.Fatalf("thread: %+v", thread)
	}
	if len(thread.ReactionChanges) != 1 || thread.ReactionChanges[0].ID != 5 || thread.ReactionChanges[0].Reactions[0].Emoji != "✅" {
		t.Fatalf("reaction changes: %+v", thread.ReactionChanges)
	}
	message := thread.Messages[0]
	if message.ID != 8 || message.ReplyTo.ImagesCount != 1 || message.Mentions[0].Handle != "bob" || message.LinkPreview.Title != "Laravel" || message.Images[0].ID != "0195f1c2-8a4b-7c3d-9e2f-1a2b3c4d5e6f" || message.Images[0].Description != "diff" || message.Voice.DurationMS != 1500 || len(message.Voice.Waveform) != 3 || message.Reactions[0].UserID != "me" {
		t.Fatalf("message: %+v", message)
	}
}

func TestOrgThreadReadsMembersPinsTypingAndRole(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/remotes/orgs/"+orgUUID+"/messages" || r.URL.Query().Get("before") != "40" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"data":{"org":{"id":"`+orgUUID+`","name":null},"messages":[
			{"id":39,"user_id":"me","body":"core","event":"created","user":{"id":"me","handle":"me","name":"Me"},"subject":null,"created_at":"2026-11-01T10:00:00Z"}
		],"pinned":[{"id":39,"user_id":"me","body":"runbook","created_at":"2026-11-01T10:00:00Z","pinned_at":"2026-11-02T10:00:00Z"}],
		"members":[{"role":"maintainer","read_through_id":39,"user":{"id":"me","handle":"me","name":"Me"}}],"typing":["bob"],"has_older":true,"reactions_version":0,"reaction_changes":[],"read_through_id":39,"is_archived":false,"pinned_at":null,"muted_until_at":null,"role":"maintainer","access":"open"}}`)
	})
	thread, err := client.RemoteThread(t.Context(), Target{Org: orgUUID}, ThreadQuery{Before: 40})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Org.ID != orgUUID || thread.Org.Name != nil || !thread.HasOlder || thread.Role != "maintainer" || thread.Typing[0] != "bob" || len(thread.Pinned) != 1 || thread.Members[0].ReadThroughID != 39 || thread.Messages[0].Event != "created" || thread.Messages[0].User.Handle != "me" {
		t.Fatalf("org thread: %+v", thread)
	}
}

func TestRemoteThreadRejectsInvalidTargetsAndResponses(t *testing.T) {
	client := fixtureClient(t, func(http.ResponseWriter, *http.Request) { t.Error("sent a request for an invalid target") })
	for _, target := range []Target{{}, {Handle: "../x"}, {Org: "not a uuid"}, {Handle: "ada", Org: orgUUID}} {
		if _, err := client.RemoteThread(t.Context(), target, ThreadQuery{}); err == nil {
			t.Fatalf("accepted target %+v", target)
		}
	}
	for _, body := range []string{
		`{"data":{"partner":{"id":"u","handle":"ada"},"access":"open"}}`,
		`{"data":{"partner":{"id":"u","handle":"ada"},"messages":[{"id":0,"created_at":"2026-11-03T10:00:00Z"}],"access":"open"}}`,
		`{"data":{"partner":{"id":"u","handle":"ada"},"messages":[],"access":"maybe"}}`,
		`{"data":{"partner":{"id":"u","handle":"bob"},"messages":[],"access":"open"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
			if _, err := client.RemoteThread(t.Context(), Target{Handle: "ada"}, ThreadQuery{}); err == nil {
				t.Fatal("accepted an invalid thread")
			}
		})
	}
}

func TestConversationPreferencesUseTheirRoutes(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.Path+" "+string(body))
		switch r.URL.Path {
		case "/api/v1/remotes/@ada/mute":
			_, _ = io.WriteString(w, `{"muted_until_at":"2026-11-03T11:00:00Z"}`)
		case "/api/v1/remotes/orgs/" + orgUUID + "/pin":
			_, _ = io.WriteString(w, `{"pinned_at":null}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	ada, org := Target{Handle: "ada"}, Target{Org: orgUUID}
	if err := client.MarkRead(t.Context(), ada, 9); err != nil {
		t.Fatal(err)
	}
	if err := client.SetStashed(t.Context(), org, true); err != nil {
		t.Fatal(err)
	}
	if err := client.SetStashed(t.Context(), ada, false); err != nil {
		t.Fatal(err)
	}
	until, err := client.SetMute(t.Context(), ada, MuteHour)
	if err != nil || until == nil {
		t.Fatalf("mute: %v %v", until, err)
	}
	pinnedAt, err := client.SetConversationPin(t.Context(), org, false)
	if err != nil || pinnedAt != nil {
		t.Fatalf("pin: %v %v", pinnedAt, err)
	}
	if err := client.LeaveOrg(t.Context(), orgUUID, "me"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`PUT /api/v1/remotes/@ada/read {"through":9}`,
		"POST /api/v1/remotes/orgs/" + orgUUID + "/archive ",
		"DELETE /api/v1/remotes/@ada/archive ",
		`PUT /api/v1/remotes/@ada/mute {"duration":"hour"}`,
		`PUT /api/v1/remotes/orgs/` + orgUUID + `/pin {"pinned":false}`,
		"DELETE /api/v1/remotes/orgs/" + orgUUID + "/members/me ",
	}
	if len(requests) != len(want) {
		t.Fatalf("requests %q", requests)
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("request %d = %q, want %q", i, requests[i], want[i])
		}
	}
	if _, err := client.SetMute(t.Context(), ada, MuteDuration("year")); err == nil {
		t.Fatal("accepted an unknown mute duration")
	}
}

func TestMessageLinkPreviewReadsTheCard(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/remotes/messages/8/link-preview" {
			t.Errorf("unexpected request %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"link_preview":{"url":"https://laravel.com","title":"Laravel","is_pending":false}}`)
	})
	preview, err := client.MessageLinkPreview(t.Context(), 8)
	if err != nil || preview == nil || preview.Title != "Laravel" || preview.Pending {
		t.Fatalf("preview: %+v %v", preview, err)
	}
}
