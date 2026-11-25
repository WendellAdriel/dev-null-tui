package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/WendellAdriel/dev-null-cli/internal/api"
)

type guildLog struct {
	calls  []string
	board  api.GuildBoard
	guilds []api.Guild
	saved  api.GuildSaved
	forms  []api.GuildForm
}

func guildApp(t *testing.T, board api.GuildBoard) (AppModel, *guildLog) {
	t.Helper()
	m := fixtureApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m = updated.(AppModel)
	log := &guildLog{board: board}
	for i := range 30 {
		log.guilds = append(log.guilds, api.Guild{Handle: fmt.Sprintf("g%02d", i), Name: fmt.Sprintf("Guild %02d", i), Description: "About guild", Emoji: "☕", Members: 30 - i})
	}
	m.guildCalls = guildCalls{
		directory: func(_ context.Context, q string, page int) (api.Guilds, error) {
			log.calls = append(log.calls, fmt.Sprintf("directory %q %d", q, page))
			result := api.Guilds{Data: log.guilds[(page-1)*24 : min(len(log.guilds), page*24)]}
			result.Page, result.LastPage, result.Total = page, 2, len(log.guilds)
			return result, nil
		},
		board: func(_ context.Context, handle string, page int) (api.GuildBoard, error) {
			log.calls = append(log.calls, fmt.Sprintf("board %s %d", handle, page))
			b := log.board
			b.Page, b.LastPage = 1, max(1, log.board.LastPage)
			return b, nil
		},
		members: func(_ context.Context, handle string, page int) (api.GuildMembers, error) {
			log.calls = append(log.calls, "members "+handle)
			members := api.GuildMembers{Data: []api.User{{ID: "mod", Handle: "taylor", Name: "Taylor"}, {ID: "u2", Handle: "ada", Name: "Ada"}}, ModeratorID: "mod"}
			members.Page, members.LastPage = 1, 1
			return members, nil
		},
		joined: func(context.Context) ([]api.JoinedGuild, error) {
			log.calls = append(log.calls, "joined")
			return []api.JoinedGuild{{GuildBadge: api.GuildBadge{Handle: "laravel", Name: "Laravel", Emoji: "🟥"}, Missed: 4}}, nil
		},
		membership: func(_ context.Context, handle string, on bool) error {
			log.calls = append(log.calls, fmt.Sprintf("membership %s %v", handle, on))
			return nil
		},
		mute: func(_ context.Context, handle string, on bool) error {
			log.calls = append(log.calls, fmt.Sprintf("mute %s %v", handle, on))
			return nil
		},
		read: func(_ context.Context, handle string) error {
			log.calls = append(log.calls, "read "+handle)
			return nil
		},
		pin: func(_ context.Context, handle, id string, on bool) error {
			log.calls = append(log.calls, fmt.Sprintf("pin %s %s %v", handle, id, on))
			return nil
		},
		create: func(_ context.Context, form api.GuildForm) (api.GuildSaved, error) {
			log.calls, log.forms = append(log.calls, "create"), append(log.forms, form)
			return log.saved, nil
		},
		update: func(_ context.Context, handle string, form api.GuildForm) (api.GuildSaved, error) {
			log.calls, log.forms = append(log.calls, "update "+handle), append(log.forms, form)
			return log.saved, nil
		},
	}
	return m, log
}

func laravelBoard(moderator, joined, paused bool) api.GuildBoard {
	post := fixtureCommit("b1")
	post.Body = "First board post"
	post.Guild = &api.GuildBadge{Handle: "laravel", Name: "Laravel", Emoji: "🟥"}
	pinned := fixtureCommit("p1")
	pinned.Body = "Read the rules"
	return api.GuildBoard{
		Guild:       api.Guild{Handle: "laravel", Name: "Laravel", Description: "Artisans of the web", Emoji: "🟥", Members: 12, Joined: joined, Paused: paused},
		Data:        []api.Commit{pinned, post},
		Pinned:      &pinned,
		CanModerate: moderator,
	}
}

func openBoard(t *testing.T, m AppModel, log *guildLog) AppModel {
	t.Helper()
	cmd := m.openGuild("laravel")
	m = deliver(m, runAll(cmd))
	if s := m.topProfile(); s == nil || !s.guild || !s.loaded {
		t.Fatalf("the board did not open: %v", log.calls)
	}
	return m
}

func TestABoardShowsTheWebsHeaderPinAndComposer(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, false))
	m.joinedGuilds = []api.JoinedGuild{{GuildBadge: api.GuildBadge{Handle: "laravel", Name: "Laravel"}, Missed: 4}}
	m = openBoard(t, m, log)
	if strings.Join(log.calls, ",") != "board laravel 1,read laravel" {
		t.Fatalf("calls: %v", log.calls)
	}
	if m.joinedGuilds[0].Missed != 0 {
		t.Fatal("opening a board must clear its missed count")
	}
	frame := frameText(m)
	for _, want := range []string{"Laravel guild", "#laravel · Public guild · 12 members", "Artisans of the web", "Include #laravel in any commit to post here.", "What do you want to share with the guild?", "Posted to this guild, not your profile log.", "Bulletin board · Latest commits", "Pinned to board", "Read the rules", "First board post", "Joined", "Report guild"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "posted in") {
		t.Fatalf("a board must not label its own posts:\n%s", frame)
	}
	if strings.Count(frame, "Read the rules") != 1 {
		t.Fatal("the pinned commit must not repeat in the board")
	}
	status := statusLineOf(m)
	for _, want := range []string{"Leave", "Post", "Members", "Report"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status %q lacks %q", status, want)
		}
	}
}

// statusLineOf is the status bar's text.
func statusLineOf(m AppModel) string {
	lines := strings.Split(frameText(m), "\n")
	return lines[len(lines)-1]
}

func TestBoardKeysJoinMuteAndMembers(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, false, false))
	m = openBoard(t, m, log)
	log.calls = nil
	m = press(t, m, "M")
	if !m.topProfile().board.Guild.Muted {
		t.Fatalf("M must mute a guild you didn't join: %v", log.calls)
	}
	m = press(t, m, "J")
	g := m.topProfile().board.Guild
	if !g.Joined || g.Muted || g.Members != 13 {
		t.Fatalf("J must join and clear the mute: %+v", g)
	}
	m = press(t, m, "M")
	m = press(t, m, "U")
	if s := m.topProfile(); !s.people || !s.guild {
		t.Fatal("U must open the members")
	}
	want := "mute laravel true,membership laravel true,members laravel"
	if got := strings.Join(log.calls, ","); got != want {
		t.Fatalf("calls:\n%s\nwant\n%s", got, want)
	}
	if frame := frameText(m); !strings.Contains(frame, "Laravel · Members") || !strings.Contains(frame, "@taylor (moderator)") {
		t.Fatalf("members:\n%s", frame)
	}
}

func TestModeratorsPinAndEditAndMembersReport(t *testing.T) {
	m, log := guildApp(t, laravelBoard(true, true, false))
	m = openBoard(t, m, log)
	log.calls = nil
	m = press(t, m, "j", "b")
	if len(log.calls) == 0 || log.calls[0] != "pin laravel b1 true" {
		t.Fatalf("b must pin the focused commit: %v", log.calls)
	}
	m = press(t, m, "R")
	if m.settings.dialog != "" {
		t.Fatal("a moderator can't report their own guild")
	}
	m = press(t, m, "E")
	if m.guildForm == nil || m.guildForm.editing == nil || m.guildForm.name.Value() != "Laravel" {
		t.Fatal("E must open the edit form")
	}
	if !strings.Contains(frameText(m), "#laravel") || !strings.Contains(frameText(m), "Save changes") {
		t.Fatalf("edit form:\n%s", frameText(m))
	}

	m, log = guildApp(t, laravelBoard(false, true, false))
	m = openBoard(t, m, log)
	m = press(t, m, "b", "E")
	if m.guildForm != nil || strings.Contains(strings.Join(log.calls, ","), "pin") {
		t.Fatal("members can't pin or edit")
	}
	m = press(t, m, "R")
	if m.settings.dialog != "guild-report" || !strings.Contains(frameText(m), "Report Laravel") || !strings.Contains(frameText(m), "Reporting does not hide the guild.") {
		t.Fatalf("report dialog:\n%s", frameText(m))
	}
}

func TestAPausedBoardHasNoComposer(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, true))
	m = openBoard(t, m, log)
	frame := frameText(m)
	if !strings.Contains(frame, "· Paused") || !strings.Contains(frame, "This guild is paused until it has a moderator.") || strings.Contains(frame, "What do you want to share") {
		t.Fatalf("paused board:\n%s", frame)
	}
	m = press(t, m, "c")
	if m.composer.open {
		t.Fatal("a paused board must not open the composer")
	}
}

func TestPostingFromABoardSendsTheGuild(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, false))
	m = openBoard(t, m, log)
	m = press(t, m, "c")
	if !m.composer.open || m.composer.guild == nil || m.composer.guild.Handle != "laravel" {
		t.Fatal("c on a board must compose for it")
	}
	if !strings.Contains(frameText(m), "Post to 🟥 Laravel") {
		t.Fatalf("composer title:\n%s", frameText(m))
	}
	m.composer.input.SetValue("Hello guild")
	if draft := m.composerDraft(); draft.Guild != "laravel" {
		t.Fatalf("draft: %+v", draft)
	}
	m.submit = func(_ context.Context, draft api.NewCommit) (api.Commit, error) {
		c := fixtureCommit("board-new")
		c.Body, c.Author.ID, c.Status = draft.Body, "fixture", "approved"
		c.GuildHandle, c.Guild = draft.Guild, &api.GuildBadge{Handle: draft.Guild, Name: "Laravel", Emoji: "🟥"}
		return c, nil
	}
	logBefore := len(m.feeds[0].commits)
	next, cmd := m.Update(keyMsg("ctrl+s"))
	m = deliver(next.(AppModel), runAll(cmd))
	if board := m.topProfile().lists[0].commits; len(board) == 0 || board[0].ID != "board-new" {
		t.Fatalf("the post must lead its board: %+v", board)
	}
	if len(m.feeds[0].commits) != logBefore {
		t.Fatal("a board post must stay out of the Log")
	}

	m.composeTo("", api.Commit{})
	if m.composerDraft().Guild != "" {
		t.Fatal("a Log commit must not go to the last board")
	}
}

func TestAKeptBoardDraftStaysOnItsBoard(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, false))
	m = keepTyped(t, press(t, openBoard(t, m, log), "c"), "for the board")
	saved := m.account.store.CommitDrafts(m.account.user.ID)
	if draft, ok := saved["guild:laravel"]; !ok || draft.Guild == nil || draft.Guild.Handle != "laravel" {
		t.Fatalf("board draft: %+v", saved)
	}
	if _, ok := saved["commit"]; ok {
		t.Fatal("a board draft must not take the Log's draft")
	}

	m.composeTo("", api.Commit{})
	if m.composer.input.Value() != "" || m.composer.guild != nil {
		t.Fatal("a Log commit must not open the board draft")
	}
	m.clearComposer()
	m.reopenDraft("guild:laravel")
	if m.composer.input.Value() != "for the board" || m.composer.guild == nil || m.composer.guild.Handle != "laravel" {
		t.Fatalf("reopened draft: %q %+v", m.composer.input.Value(), m.composer.guild)
	}
	if label := draftKindLabel(saved["guild:laravel"]); label != "Post to 🟥 Laravel" {
		t.Fatalf("Drafts label: %q", label)
	}
}

func TestTheDirectoryListsSearchesAndPages(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, false, false))
	m, _ = m.handleKey("g")
	m = press(t, m, "u")
	if !m.directoryOnTop() || len(m.directory.guilds) != 24 {
		t.Fatalf("g u must open the directory: %v", log.calls)
	}
	frame := frameText(m)
	for _, want := range []string{"Guilds", "Public groups, biggest first.", "Create a guild", "30 guilds", "Page 1 of 2", "Guild 00", "#g00", "30 members"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	m = press(t, m, "n")
	if m.directory.page != 2 || m.directory.guilds[0].Handle != "g24" {
		t.Fatalf("n must load page 2: %v", log.calls)
	}
	m = through(m, "/")
	for _, r := range "lara" {
		next, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = deliver(next.(AppModel), runAll(cmd))
	}
	m = through(m, "enter")
	if m.directory.query != "lara" || log.calls[len(log.calls)-1] != `directory "lara" 1` {
		t.Fatalf("search: %v", log.calls)
	}
	m = press(t, m, "J")
	if got := log.calls[len(log.calls)-1]; got != "membership g00 true" || !m.directory.guilds[0].Joined {
		t.Fatalf("J must join the selected card: %v", log.calls)
	}
	m = press(t, m, "enter")
	if s := m.topProfile(); s == nil || s.handle != "g00" {
		t.Fatal("enter must open the board")
	}
}

func TestYourGuildsSitInTheNavigationAndOpenTheirBoard(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, false))
	m = deliver(m, runAll(m.refreshJoinedGuilds()))
	frame := frameText(m)
	if !strings.Contains(frame, "Your guilds") || !strings.Contains(frame, "Laravel") || !strings.Contains(frame, "Guilds") {
		t.Fatalf("navigation:\n%s", frame)
	}
	nav, _, _ := m.columns()
	_, areas := m.navigationLayout(nav, m.mainHeight(), m.account.colors())
	for _, area := range areas {
		if area.action == "guild:laravel" {
			return
		}
	}
	t.Fatalf("no clickable guild row: %v", log.calls)
}

func TestCreatingAGuildPreviewsItsHashtagAndOpensTheBoard(t *testing.T) {
	m, log := guildApp(t, laravelBoard(true, true, false))
	m, _ = m.openDirectory()
	m = press(t, m, "C")
	if m.guildForm == nil {
		t.Fatal("C must open the create form")
	}
	if !strings.Contains(frameText(m), "Your hashtag appears here") {
		t.Fatalf("form:\n%s", frameText(m))
	}
	typeInto := func(s string) {
		for _, r := range s {
			next, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			m = deliver(next.(AppModel), runAll(cmd))
		}
	}
	typeInto("PHP Café")
	if !strings.Contains(frameText(m), "#phpcafé") {
		t.Fatalf("hashtag preview:\n%s", frameText(m))
	}
	m = through(m, "tab")
	typeInto("Coffee and code")
	m = through(m, "ctrl+s")
	if !strings.Contains(m.guildForm.status, "Choose an emoji") {
		t.Fatalf("status: %q", m.guildForm.status)
	}
	m = through(m, "tab")
	typeInto("coffee")
	m = through(m, "enter")
	if m.guildForm.emojiValue != "☕" {
		t.Fatalf("emoji: %q", m.guildForm.emojiValue)
	}
	log.saved = api.GuildSaved{Message: "Your guild is waiting for review. It goes live once an admin approves it."}
	m = through(m, "ctrl+s")
	if m.guildForm != nil || !strings.Contains(frameText(m), "waiting for review") {
		t.Fatalf("a 202 must close the form with the server's message:\n%s", frameText(m))
	}
	if f := log.forms[0]; f.Name != "PHP Café" || f.Description != "Coffee and code" || f.Emoji != "☕" {
		t.Fatalf("form: %+v", f)
	}

	m = press(t, m, "C")
	typeInto("Laravel")
	m.guildForm.description.SetValue("Artisans")
	m.guildForm.emojiValue = "🟥"
	log.saved = api.GuildSaved{Guild: &api.Guild{Handle: "laravel", Name: "Laravel"}}
	m = through(m, "ctrl+s")
	if s := m.topProfile(); s == nil || s.handle != "laravel" {
		t.Fatalf("a 201 must open the new board: %v", log.calls)
	}
}

func TestGuildScreensFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}} {
		m, log := guildApp(t, laravelBoard(true, true, false))
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(AppModel)
		m = deliver(m, runAll(m.refreshJoinedGuilds()))
		m, _ = m.openDirectory()
		m = deliver(m, runAll(m.fetchDirectory(1)))
		checkFrame(t, m, size)
		m = openBoard(t, m, log)
		checkFrame(t, m, size)
		m = press(t, m, "U")
		checkFrame(t, m, size)
		m = press(t, m, "esc", "E")
		checkFrame(t, m, size)
		m = through(m, "tab", "tab")
		if frame := frameText(m); !strings.Contains(frame, "› Emoji") {
			t.Fatalf("at %dx%d the focused emoji field is off screen:\n%s", size[0], size[1], frame)
		}
		// A status that wraps still shows in full, its last word included.
		m.guildForm.status = "Use a display name whose hashtag starts with a letter and contains only letters, numbers, and underscores."
		if frame := frameText(m); !strings.Contains(frame, "underscores.") {
			t.Fatalf("at %dx%d the wrapped status was cut:\n%s", size[0], size[1], frame)
		}
	}
}

func TestShortSidebarsKeepYourCountsBeforeYourGuilds(t *testing.T) {
	m, _ := guildApp(t, laravelBoard(false, true, false))
	commits, following, followers := 12, 3, 4
	m.account.user.Commits, m.account.user.Following, m.account.user.Followers = &commits, &following, &followers
	p := m.account.colors()
	for _, joined := range []int{0, 2, 5, 7} {
		m.joinedGuilds = nil
		for i := range joined {
			m.joinedGuilds = append(m.joinedGuilds, api.JoinedGuild{GuildBadge: api.GuildBadge{Handle: fmt.Sprintf("j%d", i+1), Name: fmt.Sprintf("Joined %d", i+1), Emoji: "☕"}})
		}
		for height := 18; height <= 45; height++ {
			without := m
			without.joinedGuilds = nil
			alone, _ := without.navigationLayout(25, height, p)
			alone = ansi.Strip(alone)
			with, _ := m.navigationLayout(25, height, p)
			with = ansi.Strip(with)
			if lines := strings.Count(with, "\n") + 1; lines != height {
				t.Fatalf("%d guilds at %d rows: %d lines", joined, height, lines)
			}
			for _, count := range []string{"12 commits", "3 following", "4 followers"} {
				if strings.Contains(alone, count) && !strings.Contains(with, count) {
					t.Fatalf("%d guilds at %d rows dropped %q:\n%s", joined, height, count, with)
				}
			}
			if strings.Contains(with, "Your guilds") && !strings.Contains(with, "Joined 1") {
				t.Fatalf("%d guilds at %d rows: a heading with no guild under it:\n%s", joined, height, with)
			}
		}
	}
}

func checkFrame(t *testing.T, m AppModel, size [2]int) {
	t.Helper()
	lines := strings.Split(frameText(m), "\n")
	if len(lines) != size[1] {
		t.Fatalf("%dx%d: %d lines", size[0], size[1], len(lines))
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > size[0] {
			t.Fatalf("%dx%d: a %d-cell line: %q", size[0], size[1], w, line)
		}
	}
}

// through sends keys the way the terminal does, through Update, so overlays and text fields get them first.
func through(m AppModel, keys ...string) AppModel {
	for _, k := range keys {
		m = pressThrough(m, k)
	}
	return m
}

func TestPinningAPagedBoardReloadsFromTheFirstPage(t *testing.T) {
	board := laravelBoard(true, true, false)
	board.LastPage = 2
	m, log := guildApp(t, board)
	m = openBoard(t, m, log)
	m.topProfile().lists[0].cursors["2"] = true
	next, cmd := m.handleKey("b")
	m = next
	// The pin's result starts the reload, so follow-up commands are delivered too.
	for msgs, round := runAll(cmd), 0; len(msgs) > 0 && round < 4; round++ {
		var more []tea.Msg
		for _, msg := range msgs {
			updated, follow := m.update(msg)
			m = updated.(AppModel)
			more = append(more, runAll(follow)...)
		}
		msgs = more
	}
	if l := m.topProfile().lists[0]; l.status != "" || l.next != "2" {
		t.Fatalf("after a pin the board reads %q with next %q", l.status, l.next)
	}
}

func TestTheSidebarListsFiveGuildsAndTheRestUnderMoreGuilds(t *testing.T) {
	m, log := guildApp(t, laravelBoard(false, true, false))
	m.joinedGuilds = nil
	for i := range 7 {
		m.joinedGuilds = append(m.joinedGuilds, api.JoinedGuild{GuildBadge: api.GuildBadge{Handle: fmt.Sprintf("j%d", i+1), Name: fmt.Sprintf("Joined %d", i+1), Emoji: "☕"}})
	}
	frame := frameText(m)
	if !strings.Contains(frame, "Joined 5") || strings.Contains(frame, "Joined 6") || !strings.Contains(frame, "g 5") || !strings.Contains(frame, "More guilds") {
		t.Fatalf("sidebar:\n%s", frame)
	}
	m = through(keys(m, "g"), "2")
	if s := m.topProfile(); s == nil || s.handle != "j2" || !slices.Contains(log.calls, "board j2 1") {
		t.Fatalf("g 2 did not open the second guild: %v", log.calls)
	}
	m = through(keys(m, "g"), "m")
	if m.picker == nil || len(m.picker.rows) != 7 || !strings.Contains(frameText(m), "Your guilds") {
		t.Fatalf("g m did not list every joined guild")
	}
	m = through(m, "G", "enter")
	if s := m.topProfile(); m.picker != nil || s == nil || s.handle != "j7" {
		t.Fatalf("enter on the last row did not open its board: %v", log.calls)
	}
}
