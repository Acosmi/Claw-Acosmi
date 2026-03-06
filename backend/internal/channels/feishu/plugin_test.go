package feishu

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

type fakeFeishuSender struct {
	textCalls  []feishuTextCall
	imageCalls []feishuKeyCall
	audioCalls []feishuKeyCall
	fileCalls  []feishuKeyCall

	textErr  error
	imageErr error
	audioErr error
	fileErr  error
}

type feishuTextCall struct {
	receiveID string
	idType    string
	text      string
}

type feishuKeyCall struct {
	receiveID string
	idType    string
	key       string
}

func (f *fakeFeishuSender) SendText(_ context.Context, receiveID, idType, text string) error {
	f.textCalls = append(f.textCalls, feishuTextCall{receiveID: receiveID, idType: idType, text: text})
	return f.textErr
}

func (f *fakeFeishuSender) SendCardWithID(_ context.Context, _, _, _ string) (string, error) {
	return "mock-card-msg-id", nil
}

func (f *fakeFeishuSender) PatchCard(_ context.Context, _, _ string) error { return nil }

func (f *fakeFeishuSender) SendImage(_ context.Context, receiveID, idType, imageKey string) error {
	f.imageCalls = append(f.imageCalls, feishuKeyCall{receiveID: receiveID, idType: idType, key: imageKey})
	return f.imageErr
}

func (f *fakeFeishuSender) SendAudio(_ context.Context, receiveID, idType, fileKey string) error {
	f.audioCalls = append(f.audioCalls, feishuKeyCall{receiveID: receiveID, idType: idType, key: fileKey})
	return f.audioErr
}

func (f *fakeFeishuSender) SendFile(_ context.Context, receiveID, idType, fileKey string) error {
	f.fileCalls = append(f.fileCalls, feishuKeyCall{receiveID: receiveID, idType: idType, key: fileKey})
	return f.fileErr
}

type feishuUploadRoundTripper struct {
	uploadImageErr bool
	uploadFileErr  bool

	imageUploadCount int
	fileUploadCount  int
	fileUploadNames  []string
}

func (rt *feishuUploadRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/open-apis/im/v1/images":
		rt.imageUploadCount++
		if rt.uploadImageErr {
			return jsonHTTPResponse(http.StatusOK, `{"code":999,"msg":"upload image failed"}`), nil
		}
		return jsonHTTPResponse(http.StatusOK, `{"code":0,"msg":"ok","data":{"image_key":"img-key-1"}}`), nil

	case "/open-apis/im/v1/files":
		rt.fileUploadCount++
		if name := readMultipartFileName(req); name != "" {
			rt.fileUploadNames = append(rt.fileUploadNames, name)
		}
		if rt.uploadFileErr {
			return jsonHTTPResponse(http.StatusOK, `{"code":999,"msg":"upload file failed"}`), nil
		}
		return jsonHTTPResponse(http.StatusOK, `{"code":0,"msg":"ok","data":{"file_key":"file-key-1"}}`), nil

	default:
		return jsonHTTPResponse(http.StatusNotFound, `{"code":404,"msg":"not found"}`), nil
	}
}

func readMultipartFileName(req *http.Request) string {
	mr, err := req.MultipartReader()
	if err != nil {
		return ""
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() == "file_name" {
			body, _ := io.ReadAll(part)
			return string(body)
		}
		_, _ = io.Copy(io.Discard, part)
	}
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func installFeishuHTTPClientForTest(t *testing.T, client *http.Client) {
	t.Helper()
	orig := httpClient
	httpClient = client
	t.Cleanup(func() {
		httpClient = orig
	})
}

func newFeishuClientForSendTests() *FeishuClient {
	return &FeishuClient{
		AppID:       "app-id",
		AppSecret:   "app-secret",
		Domain:      "feishu",
		cachedToken: "token-1",
		tokenExpiry: time.Now().Add(1 * time.Hour),
	}
}

func newFeishuPluginForSendTests(sender feishuMessageSender, client *FeishuClient) *FeishuPlugin {
	p := NewFeishuPlugin(nil)
	p.senders[channels.DefaultAccountID] = sender
	if client != nil {
		p.clients[channels.DefaultAccountID] = client
	}
	return p
}

func TestFeishuSendMessage_TextUsesOpenIDByDefault(t *testing.T) {
	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, nil)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "ou_user_1",
		Text: "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil || result.Channel != string(channels.ChannelFeishu) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(sender.textCalls) != 1 {
		t.Fatalf("expected one text call, got %d", len(sender.textCalls))
	}
	if sender.textCalls[0].idType != ReceiveIDTypeOpenID {
		t.Fatalf("expected open_id type, got %s", sender.textCalls[0].idType)
	}
}

func TestFeishuSendMessage_TextUsesChatIDForGroup(t *testing.T) {
	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, nil)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "oc_chat_1",
		Text: "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if len(sender.textCalls) != 1 {
		t.Fatalf("expected one text call, got %d", len(sender.textCalls))
	}
	if sender.textCalls[0].idType != ReceiveIDTypeChatID {
		t.Fatalf("expected chat_id type, got %s", sender.textCalls[0].idType)
	}
}

func TestFeishuSendMessage_BinaryImageSuccessWithText(t *testing.T) {
	rt := &feishuUploadRoundTripper{}
	installFeishuHTTPClientForTest(t, &http.Client{
		Transport: rt,
		Timeout:   3 * time.Second,
	})

	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, newFeishuClientForSendTests())

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "oc_chat_1",
		Text:          "caption",
		MediaData:     []byte{0x89, 0x50, 0x4E, 0x47},
		MediaMimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}
	if rt.imageUploadCount != 1 || rt.fileUploadCount != 0 {
		t.Fatalf("expected image upload only, got image=%d file=%d", rt.imageUploadCount, rt.fileUploadCount)
	}
	if len(sender.imageCalls) != 1 || sender.imageCalls[0].key != "img-key-1" {
		t.Fatalf("expected one image send with uploaded key, got %+v", sender.imageCalls)
	}
	if len(sender.textCalls) != 1 || sender.textCalls[0].text != "caption" {
		t.Fatalf("expected text append after media success, got %+v", sender.textCalls)
	}
}

func TestFeishuSendMessage_BinaryAudioSuccess(t *testing.T) {
	rt := &feishuUploadRoundTripper{}
	installFeishuHTTPClientForTest(t, &http.Client{
		Transport: rt,
		Timeout:   3 * time.Second,
	})

	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, newFeishuClientForSendTests())

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "ou_user_1",
		MediaData:     []byte{0x11, 0x22, 0x33},
		MediaMimeType: "audio/ogg",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if rt.fileUploadCount != 1 {
		t.Fatalf("expected file upload for audio, got %d", rt.fileUploadCount)
	}
	if len(sender.audioCalls) != 1 || sender.audioCalls[0].key != "file-key-1" {
		t.Fatalf("expected one audio send with uploaded file key, got %+v", sender.audioCalls)
	}
	if len(sender.textCalls) != 0 {
		t.Fatalf("unexpected text send for media-only success: %+v", sender.textCalls)
	}
}

func TestFeishuSendMessage_BinaryFilePreservesFileName(t *testing.T) {
	rt := &feishuUploadRoundTripper{}
	installFeishuHTTPClientForTest(t, &http.Client{
		Transport: rt,
		Timeout:   3 * time.Second,
	})

	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, newFeishuClientForSendTests())

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "oc_chat_1",
		MediaData:     []byte("# report\n"),
		MediaFileName: "idlefish_agent_design.md",
		MediaMimeType: "text/markdown",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if rt.fileUploadCount != 1 {
		t.Fatalf("expected file upload, got %d", rt.fileUploadCount)
	}
	if len(rt.fileUploadNames) != 1 || rt.fileUploadNames[0] != "idlefish_agent_design.md" {
		t.Fatalf("expected uploaded file name to be preserved, got %+v", rt.fileUploadNames)
	}
	if len(sender.fileCalls) != 1 || sender.fileCalls[0].key != "file-key-1" {
		t.Fatalf("expected one file send with uploaded key, got %+v", sender.fileCalls)
	}
}

func TestFeishuSendMessage_BinaryFailWithoutTextReturnsStructuredError(t *testing.T) {
	rt := &feishuUploadRoundTripper{uploadImageErr: true}
	installFeishuHTTPClientForTest(t, &http.Client{
		Transport: rt,
		Timeout:   3 * time.Second,
	})

	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, newFeishuClientForSendTests())

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "oc_chat_1",
		MediaData:     []byte{0x89, 0x50, 0x4E, 0x47},
		MediaMimeType: "image/png",
	})
	if err == nil {
		t.Fatalf("expected structured error when media fails and no text fallback")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrUpstream {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrUpstream)
	}
	if sendErr.Operation != "send.media" {
		t.Fatalf("operation=%s, want=send.media", sendErr.Operation)
	}
}

func TestFeishuSendMessage_BinaryFailFallsBackToText(t *testing.T) {
	rt := &feishuUploadRoundTripper{uploadImageErr: true}
	installFeishuHTTPClientForTest(t, &http.Client{
		Transport: rt,
		Timeout:   3 * time.Second,
	})

	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, newFeishuClientForSendTests())

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "ou_user_1",
		Text:          "fallback",
		MediaData:     []byte{0x89, 0x50, 0x4E, 0x47},
		MediaMimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendMessage should fallback to text: %v", err)
	}
	if len(sender.textCalls) != 1 || sender.textCalls[0].text != "fallback" {
		t.Fatalf("expected fallback text send, got %+v", sender.textCalls)
	}
}

func TestFeishuSendMessage_TextSendFailureMapped(t *testing.T) {
	sender := &fakeFeishuSender{textErr: errors.New("send text failed")}
	plugin := newFeishuPluginForSendTests(sender, nil)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "ou_user_1",
		Text: "hello",
	})
	if err == nil {
		t.Fatalf("expected text send failure")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected structured send error, got %v", err)
	}
	if sendErr.Code != channels.SendErrUpstream {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrUpstream)
	}
	if sendErr.Operation != "send.text" {
		t.Fatalf("operation=%s, want=send.text", sendErr.Operation)
	}
}

func TestFeishuSendMessage_EmptyRequestInvalid(t *testing.T) {
	sender := &fakeFeishuSender{}
	plugin := newFeishuPluginForSendTests(sender, nil)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To: "ou_user_1",
	})
	if err == nil {
		t.Fatalf("expected invalid request error")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected structured send error, got %v", err)
	}
	if sendErr.Code != channels.SendErrInvalidRequest {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrInvalidRequest)
	}
}

func TestFeishuSendMessage_SenderUnavailable(t *testing.T) {
	plugin := NewFeishuPlugin(nil)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:   "ou_user_1",
		Text: "hello",
	})
	if err == nil {
		t.Fatalf("expected unavailable error")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrUnavailable {
		t.Fatalf("code=%s, want=%s", sendErr.Code, channels.SendErrUnavailable)
	}
}
