package auth

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestAccountReadsItsOrganizations(t *testing.T) {
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
	if err := json.Unmarshal([]byte(`{"data":{"organizations":[]}}`), &none); err != nil {
		t.Fatal(err)
	}
	if len(older.Data.Organizations) != 0 || len(none.Data.Organizations) != 0 || older.Data.Live != nil {
		t.Fatalf("an account without memberships read as organizations or live: %+v %+v", older.Data, none.Data)
	}
	organizations := member.Data.Organizations
	if len(organizations) != 1 || organizations[0].Handle != "laravel" || organizations[0].Role != "admin" || organizations[0].ID == "" {
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
