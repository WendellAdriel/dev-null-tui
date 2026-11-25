package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

func TestGuildDirectoryBoardAndMembersDecode(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/v1/guilds":
			_, _ = io.WriteString(w, `{"data":[{"handle":"laravel","name":"Laravel","description":"Artisans","emoji":"🟥","icon_url":null,"member_count":12,"is_joined":true,"is_muted":false,"is_paused":false,"html_url":"https://dev-null.sh/guilds/laravel"}],
				"meta":{"current_page":2,"last_page":3,"total":51}}`)
		case "/api/v1/guilds/café":
			_, _ = io.WriteString(w, `{"data":[{"id":"c1","body":"Queues in #laravel","author":{"id":"u1","handle":"ada"},"guild_handle":"café","guild":{"handle":"café","name":"Café","emoji":"☕","icon_url":null},
				"guild_mentions":{"laravel":{"handle":"laravel","name":"Laravel","emoji":"🟥","icon_url":"https://dev-null.sh/i"}}},
					{"id":"c2","body":"No guild here","author":{"id":"u1","handle":"ada"},"guild_mentions":[],"parent_id":"p1","parent":{"id":"p1","parent_id":"r1","deleted_at":"2026-11-25T10:00:00Z"}}],
				"guild":{"handle":"café","name":"Café","description":"Coffee","emoji":"☕","member_count":3,"is_joined":false,"is_muted":true,"is_paused":true},
				"pinned_commit":{"id":"c1","body":"Pinned","author":{"id":"u1","handle":"ada"}},"can_moderate":true,"meta":{"current_page":1,"per_page":30},"links":{"next":"https://dev-null.sh/api/v1/guilds/caf%C3%A9?page=2"}}`)
		case "/api/v1/guilds/laravel/members":
			_, _ = io.WriteString(w, `{"data":[{"id":"u1","handle":"taylor","name":"Taylor"},{"id":"u2","handle":"ada","name":"Ada"}],"moderator_id":"u1","meta":{"current_page":1,"last_page":2}}`)
		case "/api/v1/me/guilds":
			_, _ = io.WriteString(w, `{"data":[{"handle":"laravel","name":"Laravel","emoji":"🟥","icon_url":null,"missed_commits":4,"read_from":"2026-11-20T10:00:00Z","html_url":"https://dev-null.sh/guilds/laravel"}]}`)
		}
	})

	guilds, err := client.Guilds(t.Context(), "lara vel", 2)
	if err != nil || len(guilds.Data) != 1 || guilds.Page != 2 || guilds.LastPage != 3 || guilds.Total != 51 {
		t.Fatalf("directory: %+v %v", guilds, err)
	}
	if g := guilds.Data[0]; g.Handle != "laravel" || g.Members != 12 || !g.Joined || g.Muted || g.Paused || g.Emoji != "🟥" {
		t.Fatalf("guild: %+v", g)
	}

	board, err := client.GuildBoard(t.Context(), "café", 1)
	if err != nil || !board.CanModerate || board.Pinned == nil || board.Pinned.ID != "c1" || !board.Guild.Paused || !board.Guild.Muted || board.LastPage != 2 {
		t.Fatalf("board: %+v %v", board, err)
	}
	post := board.Data[0]
	if post.Guild == nil || post.Guild.Name != "Café" || post.GuildHandle != "café" {
		t.Fatalf("board post guild: %+v", post)
	}
	if mention, ok := post.GuildMentions["laravel"]; !ok || mention.Name != "Laravel" || mention.IconURL != "https://dev-null.sh/i" {
		t.Fatalf("guild mentions: %+v", post.GuildMentions)
	}

	if plain := board.Data[1]; len(plain.GuildMentions) != 0 {
		t.Fatalf("an empty guild_mentions list: %+v", plain.GuildMentions)
	}
	if reply := board.Data[1]; reply.Parent == nil || reply.Parent.ID != "p1" || reply.Parent.DeletedAt == nil || post.Parent != nil {
		t.Fatalf("a matching reply's parent: %+v", reply.Parent)
	}

	members, err := client.GuildMembers(t.Context(), "laravel", 1)
	if err != nil || len(members.Data) != 2 || members.ModeratorID != "u1" || members.LastPage != 2 {
		t.Fatalf("members: %+v %v", members, err)
	}

	joined, err := client.JoinedGuilds(t.Context())
	if err != nil || len(joined) != 1 || joined[0].Missed != 4 || joined[0].Handle != "laravel" {
		t.Fatalf("joined: %+v %v", joined, err)
	}

	want := []string{"GET /api/v1/guilds?page=2&q=lara+vel", "GET /api/v1/guilds/caf%C3%A9", "GET /api/v1/guilds/laravel/members", "GET /api/v1/me/guilds"}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(requests, "\n"))
	}
	for _, handle := range []string{"", "../me", "a/b", "1abc", strings.Repeat("a", 61)} {
		if _, err := client.GuildBoard(t.Context(), handle, 1); err == nil {
			t.Fatalf("accepted unsafe guild handle %q", handle)
		}
	}
}

func TestGuildWritesUseTheirRoutes(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(body)))
		w.WriteHeader(http.StatusNoContent)
	})
	steps := []error{
		client.SetGuildMembership(t.Context(), "laravel", true),
		client.SetGuildMembership(t.Context(), "laravel", false),
		client.SetGuildMute(t.Context(), "laravel", true),
		client.SetGuildMute(t.Context(), "laravel", false),
		client.ReportGuild(t.Context(), "laravel", "  Spam icon  "),
		client.SetGuildPin(t.Context(), "laravel", "c1", true),
		client.SetGuildPin(t.Context(), "laravel", "c1", false),
		client.ReadGuild(t.Context(), "laravel"),
	}
	if err := errors.Join(steps...); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/v1/guilds/laravel/membership", "DELETE /api/v1/guilds/laravel/membership",
		"POST /api/v1/guilds/laravel/mute", "DELETE /api/v1/guilds/laravel/mute",
		`POST /api/v1/guilds/laravel/reports {"details":"Spam icon"}`,
		"PUT /api/v1/guilds/laravel/commits/c1/pin", "DELETE /api/v1/guilds/laravel/commits/c1/pin",
		"PUT /api/v1/guilds/laravel/read",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(requests, "\n"))
	}
	if err := client.ReportGuild(t.Context(), "laravel", "   "); err == nil {
		t.Fatal("sent an empty report")
	}
}

func TestCreatingAndEditingAGuildTellPublishedFromWaiting(t *testing.T) {
	var requests []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		kind := "json"
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			kind = "multipart"
			reader, err := r.MultipartReader()
			icons := 0
			for err == nil {
				var part *multipart.Part
				if part, err = reader.NextPart(); err == nil && part.FormName() == "images[]" {
					icons++
				}
			}
			if !errors.Is(err, io.EOF) || icons != 1 {
				t.Errorf("icon upload: %d icons, %v", icons, err)
			}
		}
		requests = append(requests, r.Method+" "+r.URL.Path+" "+kind)
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"message":"Your changes are waiting for review. They go live once an admin approves them."}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"handle":"phpcafe","name":"PHP Café","description":"Coffee","emoji":"☕","member_count":1,"is_joined":true}}`)
	})
	form := GuildForm{Name: "PHP Café", Description: "Coffee", Emoji: "☕"}
	saved, err := client.CreateGuild(t.Context(), form)
	if err != nil || saved.Guild == nil || saved.Guild.Handle != "phpcafe" || saved.Message != "" {
		t.Fatalf("created: %+v %v", saved, err)
	}
	form.Icon = &auth.Upload{Name: "icon.png", Type: "image/png", Data: []byte("png")}
	if _, err := client.CreateGuild(t.Context(), form); err != nil {
		t.Fatal(err)
	}
	saved, err = client.UpdateGuild(t.Context(), "phpcafe", GuildForm{Name: "PHP Café", Description: "Coffee", Emoji: "☕", RemoveIcon: true})
	if err != nil || saved.Guild != nil || !strings.Contains(saved.Message, "waiting for review") {
		t.Fatalf("waiting edit: %+v %v", saved, err)
	}
	want := []string{"POST /api/v1/guilds json", "POST /api/v1/guilds multipart", "PUT /api/v1/guilds/phpcafe json"}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(requests, "\n"))
	}
}

func TestGuildHashtagMatchesTheServer(t *testing.T) {
	for name, want := range map[string]string{"PHP Café": "phpcafé", "  Laravel!! ": "laravel", "Go_Lang 2": "go_lang2", "🚀": ""} {
		if got := GuildHashtag(name); got != want {
			t.Fatalf("GuildHashtag(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestBoardPostsSendTheGuildAndPausedGuildsExplainThemselves(t *testing.T) {
	var sent map[string]any
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"code":"guild_paused","message":"This guild is paused until it has a moderator."}`)
	})
	_, err := client.CreateCommit(t.Context(), NewCommit{Body: "Hello board", Guild: "laravel"})
	if sent["guild_handle"] != "laravel" {
		t.Fatalf("payload: %v", sent)
	}
	if err == nil || err.Error() != "This guild is paused until it has a moderator." {
		t.Fatalf("paused guild error: %v", err)
	}
	if _, err := client.CreateCommit(t.Context(), NewCommit{Body: "reply", ParentID: "c1", Guild: "laravel"}); err == nil {
		t.Fatal("a reply went to a board")
	}
	sent = nil
	_, _ = client.CreateCommit(t.Context(), NewCommit{Body: "Hello log"})
	if _, ok := sent["guild_handle"]; ok {
		t.Fatalf("a Log commit sent guild_handle: %v", sent)
	}
}
