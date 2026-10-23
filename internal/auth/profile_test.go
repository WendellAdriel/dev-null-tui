package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSignedIdentityIncludesOptionalPublicProfileFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"id":"fixture","name":"Synthetic Developer","username":"fixture","bio":"Building tools","company":"Fixture Labs","location":"Porto","commits_count":31,"following_count":48,"followers_count":0,"lane":"#123abc","pasteis_mode":true,"pasteis_remaining":10}}`)
	}))
	defer server.Close()
	key, _ := GenerateKey()
	client, _ := NewClient(server.URL, key)
	user, err := client.Me(t.Context())
	if err != nil || user.Bio != "Building tools" || user.Company != "Fixture Labs" || user.Location != "Porto" || user.Commits == nil || *user.Commits != 31 || user.Following == nil || *user.Following != 48 || user.Followers == nil || *user.Followers != 0 || string(user.Lane) != `"#123abc"` || !user.PasteisMode || user.PasteisRemaining == nil || *user.PasteisRemaining != 10 {
		t.Fatalf("profile decode failed: %v", err)
	}
}
