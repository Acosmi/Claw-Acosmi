package wecom

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

func TestWecomMediaURLFallbackText(t *testing.T) {
	t.Parallel()

	if got := wecomMediaURLFallbackText("", false); got != "" {
		t.Fatalf("expected empty fallback for empty url, got=%q", got)
	}

	url := "https://example.com/image.png"
	got := wecomMediaURLFallbackText(url, false)
	if !strings.Contains(got, "媒体链接") || !strings.Contains(got, url) {
		t.Fatalf("fallback text should include url detail, got=%q", got)
	}

	if got2 := wecomMediaURLFallbackText(url, true); got2 != "" {
		t.Fatalf("expected empty fallback when media data is present, got=%q", got2)
	}
}

type mockWeComRoundTripper struct {
	mu sync.Mutex

	uploadErr bool
	sendErr   bool

	uploadTypes  []string
	uploadNames  []string
	sentMessages []map[string]interface{}
}

func (m *mockWeComRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/cgi-bin/gettoken":
		return jsonResponse(http.StatusOK, `{"errcode":0,"errmsg":"ok","access_token":"token-1","expires_in":7200}`), nil

	case "/cgi-bin/media/upload":
		uploadName := readMultipartUploadName(req)
		m.mu.Lock()
		m.uploadTypes = append(m.uploadTypes, req.URL.Query().Get("type"))
		m.uploadNames = append(m.uploadNames, uploadName)
		uploadErr := m.uploadErr
		m.mu.Unlock()
		if uploadErr {
			return jsonResponse(http.StatusOK, `{"errcode":40001,"errmsg":"upload failed"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"errcode":0,"errmsg":"ok","media_id":"mid-001"}`), nil

	case "/cgi-bin/message/send":
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		msg := map[string]interface{}{}
		_ = json.Unmarshal(body, &msg)

		m.mu.Lock()
		m.sentMessages = append(m.sentMessages, msg)
		sendErr := m.sendErr
		m.mu.Unlock()
		if sendErr {
			return jsonResponse(http.StatusOK, `{"errcode":50001,"errmsg":"send failed"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"errcode":0,"errmsg":"ok","msgid":"msg-001"}`), nil

	default:
		return jsonResponse(http.StatusNotFound, `{"errcode":404,"errmsg":"not found"}`), nil
	}
}

func (m *mockWeComRoundTripper) snapshot() (uploadTypes []string, messages []map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	uploadTypes = append(uploadTypes, m.uploadTypes...)
	uploadNames := append([]string(nil), m.uploadNames...)
	messages = append(messages, m.sentMessages...)
	_ = uploadNames
	return uploadTypes, messages
}

func (m *mockWeComRoundTripper) snapshotUploads() (uploadTypes []string, uploadNames []string, messages []map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	uploadTypes = append(uploadTypes, m.uploadTypes...)
	uploadNames = append(uploadNames, m.uploadNames...)
	messages = append(messages, m.sentMessages...)
	return uploadTypes, uploadNames, messages
}

func readMultipartUploadName(req *http.Request) string {
	mr, err := req.MultipartReader()
	if err != nil {
		return ""
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() == "media" {
			name := part.FileName()
			_, _ = io.Copy(io.Discard, part)
			return name
		}
		_, _ = io.Copy(io.Discard, part)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newWeComPluginForSendTests(rt *mockWeComRoundTripper) *WeComPlugin {
	client := &WeComClient{
		CorpID:  "corp-id",
		Secret:  "corp-secret",
		AgentID: 100001,
		Client: &http.Client{
			Transport: rt,
		},
	}
	plugin := NewWeComPlugin(nil)
	plugin.senders[channels.DefaultAccountID] = NewWeComSender(client, client.AgentID)
	return plugin
}

func textContentFromMessage(msg map[string]interface{}) string {
	textNode, _ := msg["text"].(map[string]interface{})
	content, _ := textNode["content"].(string)
	return content
}

func TestWeComSendMessage_BinaryMediaSuccess(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{}
	plugin := newWeComPluginForSendTests(rt)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "user-1",
		MediaData:     []byte{1, 2, 3, 4},
		MediaMimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil || result.Channel != string(channels.ChannelWeCom) {
		t.Fatalf("unexpected send result: %+v", result)
	}

	uploadTypes, sentMessages := rt.snapshot()
	if len(uploadTypes) != 1 || uploadTypes[0] != "image" {
		t.Fatalf("expected one image upload, got %+v", uploadTypes)
	}
	if len(sentMessages) != 1 {
		t.Fatalf("expected one message request, got %d", len(sentMessages))
	}
	if msgType, _ := sentMessages[0]["msgtype"].(string); msgType != "image" {
		t.Fatalf("expected image message type, got %+v", sentMessages[0]["msgtype"])
	}
}

func TestWeComSendMessage_BinaryMediaFailWithTextFallback(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{uploadErr: true}
	plugin := newWeComPluginForSendTests(rt)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "user-1",
		Text:          "hello",
		MediaData:     []byte{9, 9, 9},
		MediaMimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("SendMessage should fallback to text instead of returning error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}

	_, sentMessages := rt.snapshot()
	if len(sentMessages) != 1 {
		t.Fatalf("expected one text send, got %d", len(sentMessages))
	}
	if msgType, _ := sentMessages[0]["msgtype"].(string); msgType != "text" {
		t.Fatalf("fallback should send text message, got %q", msgType)
	}
	if content := textContentFromMessage(sentMessages[0]); content != "hello" {
		t.Fatalf("fallback text should keep original text, got %q", content)
	}
}

func TestWeComSendMessage_BinaryMediaFailWithoutTextReturnsError(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{uploadErr: true}
	plugin := newWeComPluginForSendTests(rt)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "user-1",
		MediaData:     []byte{7, 7, 7},
		MediaMimeType: "audio/ogg",
	})
	if err == nil {
		t.Fatalf("expected error when media-only send fails")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected structured SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrUpstream {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrUpstream)
	}
	if sendErr.Operation != "send.media" {
		t.Fatalf("operation=%s, want=send.media", sendErr.Operation)
	}
	if !sendErr.Retryable {
		t.Fatalf("expected retryable send error")
	}
}

func TestWeComSendMessage_MediaURLFallbackAppendedToText(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{}
	plugin := newWeComPluginForSendTests(rt)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:       "user-1",
		Text:     "hello",
		MediaURL: "https://example.com/a.png",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}

	uploadTypes, sentMessages := rt.snapshot()
	if len(uploadTypes) != 0 {
		t.Fatalf("url-only path should not upload binary media, got uploads=%+v", uploadTypes)
	}
	if len(sentMessages) != 1 {
		t.Fatalf("expected one text message, got %d", len(sentMessages))
	}
	if content := textContentFromMessage(sentMessages[0]); !strings.Contains(content, "媒体链接：https://example.com/a.png") {
		t.Fatalf("fallback text should include media URL, got %q", content)
	}
}

func TestWeComSendMessage_EmptyRequestInvalid(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{}
	plugin := newWeComPluginForSendTests(rt)

	_, err := plugin.SendMessage(channels.OutboundSendParams{To: "user-1"})
	if err == nil {
		t.Fatalf("expected invalid request error")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected structured SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrInvalidRequest {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrInvalidRequest)
	}
}

func TestWeComSendMessage_TextSendFailure(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{sendErr: true}
	plugin := newWeComPluginForSendTests(rt)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "user-1",
		Text: "hello",
	})
	if err == nil {
		t.Fatalf("expected text send error")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected structured SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrUpstream {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrUpstream)
	}
	if sendErr.Operation != "send.text" {
		t.Fatalf("operation=%s, want=send.text", sendErr.Operation)
	}
}

func TestWeComSendMessage_BinaryFilePreservesFileName(t *testing.T) {
	t.Parallel()

	rt := &mockWeComRoundTripper{}
	plugin := newWeComPluginForSendTests(rt)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "user-1",
		MediaData:     []byte("hello"),
		MediaMimeType: "text/markdown",
		MediaFileName: "idlefish_agent_design.md",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}

	uploadTypes, uploadNames, sentMessages := rt.snapshotUploads()
	if len(uploadTypes) != 1 || uploadTypes[0] != "file" {
		t.Fatalf("expected one file upload, got %+v", uploadTypes)
	}
	if len(uploadNames) != 1 || uploadNames[0] != "idlefish_agent_design.md" {
		t.Fatalf("expected upload filename preserved, got %+v", uploadNames)
	}
	if len(sentMessages) != 1 {
		t.Fatalf("expected one message request, got %d", len(sentMessages))
	}
}
