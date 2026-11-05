package auth

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestAccountOrganizationsTellAnOlderServerFromNoMemberships(t *testing.T) {
	var older, member, none struct{ Data User }
	olderData, olderErr := os.ReadFile("testdata/current-user.json")
	memberData, memberErr := os.ReadFile("testdata/current-user-organizations.json")
	if olderErr != nil || memberErr != nil {
		t.Fatal(olderErr, memberErr)
	}
	if err := json.Unmarshal(olderData, &older); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(memberData, &member); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"data":{"organizations":[],"can_post_as_organization":true}}`), &none); err != nil {
		t.Fatal(err)
	}
	if older.Data.Organizations != nil || older.Data.CanPostAsOrganization != nil || older.Data.Live != nil {
		t.Fatalf("an older server's account read as organizations or live: %+v", older.Data)
	}
	if none.Data.Organizations == nil || len(*none.Data.Organizations) != 0 {
		t.Fatalf("no memberships read as an older server: %+v", none.Data.Organizations)
	}
	organizations := *member.Data.Organizations
	if len(organizations) != 1 || organizations[0].Handle != "laravel" || organizations[0].Role != "admin" || organizations[0].ID == "" || !*member.Data.CanPostAsOrganization {
		t.Fatalf("memberships: %+v", organizations)
	}
	live := member.Data.Live
	ends := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	if live == nil || live.Platform != "twitch" || live.URL != "https://twitch.tv/fixture" || !live.EndsAt.Equal(ends) {
		t.Fatalf("live: %+v", live)
	}
	if !live.On(ends.Add(-time.Minute)) || live.On(ends) || (*LiveStream)(nil).On(ends) {
		t.Fatal("a stream must count as live only until it ends")
	}
}
