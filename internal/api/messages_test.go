package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/WendellAdriel/dev-null-cli/internal/auth"
)

const pushedMessage = `{"data":{"id":9,"user_id":"me","client_id":"0195f1c2-8a4b-7c3d-9e2f-000000000001","body":"ship it","body_segments":[{"type":"text","text":"ship it"}],"mentions":[],"reply_to_id":7,"reactions":[],"event":null,"created_at":"2026-11-03T10:02:00Z"}}`

func TestPushMessageSendsBodyClientIDAndReply(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/remotes/orgs/"+orgUUID+"/messages" || string(body) != `{"body":"ship it","client_id":"0195f1c2-8a4b-7c3d-9e2f-000000000001","reply_to_id":7}` {
			t.Errorf("unexpected request %s %s %s", r.Method, r.URL, body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, pushedMessage)
	})
	message, err := client.PushMessage(t.Context(), Target{Org: orgUUID}, NewMessage{Body: "  ship it\n", ClientID: "0195f1c2-8a4b-7c3d-9e2f-000000000001", ReplyToID: 7})
	if err != nil || message.ID != 9 || message.ClientID != "0195f1c2-8a4b-7c3d-9e2f-000000000001" {
		t.Fatalf("message %+v, err %v", message, err)
	}
}

func TestPushMessageWithImagesSendsThemInAMultipartPayload(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		var payload []byte
		images := 0
		for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
			switch part.FormName() {
			case "payload":
				payload, _ = io.ReadAll(part)
			case "images[]":
				images++
			}
		}
		if r.URL.Path != "/api/v1/remotes/@ada/messages" || images != 2 || string(payload) != `{"body":"","client_id":"c1d2e3f4-0000-4000-8000-000000000002","image_descriptions":["A chart",null]}` {
			t.Errorf("unexpected upload %s: %d images, payload %s", r.URL, images, payload)
		}
		_, _ = io.WriteString(w, pushedMessage)
	})
	images := []ImageFile{
		{Upload: auth.Upload{Name: "a.png", Type: "image/png", Data: []byte("a")}, Description: " A chart "},
		{Upload: auth.Upload{Name: "b.png", Type: "image/png", Data: []byte("b")}},
	}
	if _, err := client.PushMessage(t.Context(), Target{Handle: "ada"}, NewMessage{ClientID: "c1d2e3f4-0000-4000-8000-000000000002", Images: images}); err != nil {
		t.Fatal(err)
	}
}

func voiceWaveform() []int {
	waveform := make([]int, 48)
	for i := range waveform {
		waveform[i] = i * 2
	}
	return waveform
}

func TestPushMessageWithVoiceSendsItAloneWithItsWaveform(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		var payload, voice []byte
		var names []string
		for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
			names = append(names, part.FormName())
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "payload":
				payload = data
			case "voice":
				voice = data
			}
		}
		want := `{"client_id":"c1d2e3f4-0000-4000-8000-000000000003","reply_to_id":5,"voice_waveform":[0,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,44,46,48,50,52,54,56,58,60,62,64,66,68,70,72,74,76,78,80,82,84,86,88,90,92,94]}`
		if strings.Join(names, ",") != "payload,voice" || string(payload) != want || string(voice) != "RIFF-note" {
			t.Errorf("unexpected voice upload: parts %v, payload %s", names, payload)
		}
		_, _ = io.WriteString(w, pushedMessage)
	})
	draft := NewMessage{ClientID: "c1d2e3f4-0000-4000-8000-000000000003", ReplyToID: 5, Voice: &VoiceNote{WAV: []byte("RIFF-note"), Waveform: voiceWaveform()}}
	if _, err := client.PushMessage(t.Context(), Target{Handle: "ada"}, draft); err != nil {
		t.Fatal(err)
	}
}

func TestNewMessageValidatesWhatTheServerRefuses(t *testing.T) {
	image := ImageFile{Upload: auth.Upload{Name: "a.png", Type: "image/png", Data: []byte("a")}}
	for name, draft := range map[string]NewMessage{
		"empty":            {ClientID: "id"},
		"blank":            {Body: " \n ", ClientID: "id"},
		"too long":         {Body: strings.Repeat("é", MaxMessageLength+1), ClientID: "id"},
		"no client id":     {Body: "hi"},
		"four images":      {ClientID: "id", Images: []ImageFile{image, image, image, image}},
		"long describe":    {ClientID: "id", Images: []ImageFile{{Upload: image.Upload, Description: strings.Repeat("a", MaxDescriptionLength+1)}}},
		"voice and body":   {ClientID: "id", Body: "hi", Voice: &VoiceNote{WAV: []byte("a"), Waveform: voiceWaveform()}},
		"voice and images": {ClientID: "id", Images: []ImageFile{image}, Voice: &VoiceNote{WAV: []byte("a"), Waveform: voiceWaveform()}},
		"short waveform":   {ClientID: "id", Voice: &VoiceNote{WAV: []byte("a"), Waveform: voiceWaveform()[:47]}},
		"loud waveform":    {ClientID: "id", Voice: &VoiceNote{WAV: []byte("a"), Waveform: append(voiceWaveform()[:47], 101)}},
		"huge voice":       {ClientID: "id", Voice: &VoiceNote{WAV: make([]byte, 1950*1024+1), Waveform: voiceWaveform()}},
	} {
		if draft.Validate() == nil {
			t.Errorf("%s: accepted %+v", name, draft)
		}
	}
	for name, draft := range map[string]NewMessage{
		"limit":       {Body: strings.Repeat("é", MaxMessageLength), ClientID: "id"},
		"images only": {ClientID: "id", Images: []ImageFile{image}},
		"voice only":  {ClientID: "id", Voice: &VoiceNote{WAV: make([]byte, 1950*1024), Waveform: voiceWaveform()}},
	} {
		if err := draft.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPushMessageRefusalsCarryTheServerReason(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"@ada hasn't merged your request yet."}`)
	})
	_, err := client.PushMessage(t.Context(), Target{Handle: "ada"}, NewMessage{Body: "hi", ClientID: "id"})
	var failure *auth.APIError
	if !errors.As(err, &failure) || failure.Status != http.StatusForbidden || failure.Message != "@ada hasn't merged your request yet." {
		t.Fatalf("err %#v", err)
	}
}

func TestPushMessageRejectsAResponseWithoutTheMessage(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"data":{"id":0}}`) })
	if _, err := client.PushMessage(t.Context(), Target{Handle: "ada"}, NewMessage{Body: "hi", ClientID: "id"}); err == nil {
		t.Fatal("accepted an invalid message")
	}
}

func TestTypingAndPingUseTheirRoutes(t *testing.T) {
	var seen []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/typing") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":10,"user_id":"me","event":"ping","created_at":"2026-11-03T10:03:00Z"}}`)
	})
	if err := client.SendTyping(t.Context(), Target{Handle: "ada"}); err != nil {
		t.Fatal(err)
	}
	ping, err := client.Ping(t.Context(), Target{Org: orgUUID})
	if err != nil || ping.Event != "ping" {
		t.Fatalf("ping %+v, err %v", ping, err)
	}
	want := []string{"PUT /api/v1/remotes/@ada/typing", "POST /api/v1/remotes/orgs/" + orgUUID + "/ping"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("requests %v", seen)
	}
}

func TestReactionsAddRemoveAndReadUsage(t *testing.T) {
	var seen []string
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+string(body))
		if r.URL.Path == "/api/v1/remotes/reactions" {
			_, _ = io.WriteString(w, `{"usage":[{"emoji":"🚀","count":5},{"emoji":"✅","count":2}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"reactions":[{"id":1,"message_id":9,"user_id":"me","emoji":"🚀"}]}`)
	})
	added, err := client.SetReaction(t.Context(), 9, "🚀", true)
	if err != nil || len(added) != 1 || added[0].Emoji != "🚀" || added[0].UserID != "me" {
		t.Fatalf("added %+v, err %v", added, err)
	}
	if _, err := client.SetReaction(t.Context(), 9, "🚀", false); err != nil {
		t.Fatal(err)
	}
	usage, err := client.ReactionUsage(t.Context())
	if err != nil || len(usage) != 2 || usage[0] != (EmojiUse{Emoji: "🚀", Count: 5}) {
		t.Fatalf("usage %+v, err %v", usage, err)
	}
	want := []string{
		`POST /api/v1/remotes/messages/9/reactions {"emoji":"🚀"}`,
		`DELETE /api/v1/remotes/messages/9/reactions {"emoji":"🚀"}`,
		`GET /api/v1/remotes/reactions `,
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s", strings.Join(seen, "\n"))
	}
	if _, err := client.SetReaction(t.Context(), 0, "🚀", true); err == nil {
		t.Fatal("reacted to an invalid message")
	}
}
