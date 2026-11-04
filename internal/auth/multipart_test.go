package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMultipartPostSignsPayloadAndImageHashesInOrder(t *testing.T) {
	key, _ := GenerateKey()
	payload := []byte(`{"body":"Synthetic 界","image_descriptions":["first",null]}`)
	images := []Upload{{Name: "b.png", Type: "image/png", Data: []byte("second-bytes")}, {Name: "a.jpg", Type: "image/jpeg", Data: []byte("first-bytes")}}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("content type %q", r.Header.Get("Content-Type"))
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		var names []string
		var gotPayload []byte
		var hashes []string
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(part)
			names = append(names, part.FormName()+":"+part.FileName())
			if part.FormName() == "payload" {
				gotPayload = data
				continue
			}
			sum := sha256.Sum256(data)
			hashes = append(hashes, hex.EncodeToString(sum[:]))
		}
		if strings.Join(names, ",") != "payload:payload.json,images[]:b.png,images[]:a.jpg" || string(gotPayload) != string(payload) {
			t.Fatalf("parts %v payload %s", names, gotPayload)
		}
		first, second := sha256.Sum256(images[0].Data), sha256.Sum256(images[1].Data)
		line := hex.EncodeToString(first[:]) + "," + hex.EncodeToString(second[:])
		if strings.Join(hashes, ",") != line {
			t.Fatal("transmitted images differ from the signed hashes")
		}
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, line, string(payload)}, "\n")
		verifyNamespacedSignature(t, key, "devnull-api-images", r.Header.Get("X-SSH-Signature"), []byte(message))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"synthetic"}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	var result struct{ Data struct{ ID string } }
	if err := client.PostMultipart(t.Context(), "/api/v1/commits", payload, images, &result); err != nil || result.Data.ID != "synthetic" {
		t.Fatalf("multipart response: %+v %v", result, err)
	}
	if err := client.PostMultipart(t.Context(), "https://foreign.example/api/v1/commits", payload, images, &result); err == nil || calls != 1 {
		t.Fatal("multipart write escaped origin or replayed")
	}
	if err := client.PostMultipart(t.Context(), "/api/v1/commits", payload, nil, &result); err == nil || calls != 1 {
		t.Fatal("multipart write without images was sent")
	}
}

func TestVoicePostSendsOneVoicePartSignedByItsHash(t *testing.T) {
	key, _ := GenerateKey()
	payload := []byte(`{"client_id":"synthetic"}`)
	voice := Upload{Name: "voice.wav", Type: "audio/wav", Data: []byte("RIFF-synthetic")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for part, err := reader.NextPart(); err == nil; part, err = reader.NextPart() {
			names = append(names, part.FormName()+":"+part.FileName()+":"+part.Header.Get("Content-Type"))
		}
		if strings.Join(names, ",") != "payload:payload.json:application/json,voice:voice.wav:audio/wav" {
			t.Fatalf("parts %v", names)
		}
		sum := sha256.Sum256(voice.Data)
		message := strings.Join([]string{r.Header.Get("X-Devnull-Timestamp"), r.Header.Get("X-Devnull-Nonce"), r.Method, r.RequestURI, hex.EncodeToString(sum[:]), string(payload)}, "\n")
		verifyNamespacedSignature(t, key, "devnull-api-images", r.Header.Get("X-SSH-Signature"), []byte(message))
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"synthetic"}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	var result struct{ Data struct{ ID string } }
	if err := client.PostVoice(t.Context(), "/api/v1/remotes/@ada/messages", payload, voice, &result); err != nil || result.Data.ID != "synthetic" {
		t.Fatalf("voice response: %+v %v", result, err)
	}
}

func TestValidationMessagesIncludeAttachmentAndVoteFieldsOnly(t *testing.T) {
	key, _ := GenerateKey()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"errors":{"body":["Body short."],"images.1":["The image must be smaller."],"poll.options.0":["Too long."],"option":["This poll has closed."],"secret":["private value"]}}`)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, key)
	err := client.PostJSON(t.Context(), "/api/v1/commits", []byte(`{}`), new(any))
	var failure *APIError
	if !errors.As(err, &failure) || strings.Join(failure.BodyErrors, "|") != "Body short." {
		t.Fatalf("body errors: %v", err)
	}
	if got := strings.Join(failure.FieldErrors, "|"); got != "The image must be smaller.|This poll has closed.|Too long." {
		t.Fatalf("field errors %q", got)
	}
}
