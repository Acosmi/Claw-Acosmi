package dingtalk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Acosmi/ClawAcosmi/internal/channels"
)

func TestCanSendDingTalkImageURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		url     string
		mime    string
		allowed bool
	}{
		{name: "empty", url: "", mime: "image/png", allowed: false},
		{name: "non-http", url: "file:///tmp/a.png", mime: "image/png", allowed: false},
		{name: "image-mime", url: "https://example.com/resource", mime: "image/png", allowed: true},
		{name: "image-suffix", url: "https://example.com/a.jpeg?x=1", mime: "", allowed: true},
		{name: "non-image", url: "https://example.com/a.pdf", mime: "application/pdf", allowed: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := canSendDingTalkImageURL(tc.url, tc.mime)
			if got != tc.allowed {
				t.Fatalf("canSendDingTalkImageURL(%q,%q)=%v, want %v", tc.url, tc.mime, got, tc.allowed)
			}
		})
	}
}

func TestDingTalkMediaFallbackFromOutbound(t *testing.T) {
	t.Parallel()

	params := channels.OutboundSendParams{
		MediaData:     []byte{1, 2, 3},
		MediaMimeType: "image/png",
		MediaURL:      "https://example.com/a.png",
	}

	withURL := dingtalkMediaFallbackFromOutbound(params, true)
	if !strings.Contains(withURL, "媒体链接") {
		t.Fatalf("expected fallback with URL detail, got: %s", withURL)
	}
	if !strings.Contains(withURL, "image/png") {
		t.Fatalf("expected fallback to include mime, got: %s", withURL)
	}

	withoutURL := dingtalkMediaFallbackFromOutbound(params, false)
	if strings.Contains(withoutURL, "媒体链接") {
		t.Fatalf("expected fallback without URL detail, got: %s", withoutURL)
	}
}

func TestDingTalkUploadHelpers(t *testing.T) {
	t.Parallel()

	if got := dingtalkUploadTypeForMime("image/png"); got != "image" {
		t.Fatalf("dingtalkUploadTypeForMime(image/png)=%q, want image", got)
	}
	if got := dingtalkUploadTypeForMime("audio/mpeg"); got != "file" {
		t.Fatalf("dingtalkUploadTypeForMime(audio/mpeg)=%q, want file", got)
	}
	if got := dingtalkDefaultUploadFileName("image", "image/jpeg"); !strings.HasPrefix(got, "upload.") {
		t.Fatalf("dingtalkDefaultUploadFileName should generate extension name, got %q", got)
	}
	if got := dingtalkFileTypeFromName("report.PDF"); got != "pdf" {
		t.Fatalf("dingtalkFileTypeFromName(report.PDF)=%q, want pdf", got)
	}
	if got := dingtalkFileTypeFromName(""); got != "bin" {
		t.Fatalf("dingtalkFileTypeFromName(empty)=%q, want bin", got)
	}
}

type fakeDingTalkSender struct {
	uploadMediaID string
	uploadErr     error

	uploadTypes []string
	uploadNames []string

	groupTexts []string
	o2oTexts   []string

	groupImages []string
	o2oImages   []string

	groupFiles []string
	o2oFiles   []string

	groupTextErr  error
	o2oTextErr    error
	groupImageErr error
	o2oImageErr   error
	groupFileErr  error
	o2oFileErr    error
}

func (f *fakeDingTalkSender) SendGroupMessage(ctx context.Context, openConversationID, text string) error {
	f.groupTexts = append(f.groupTexts, text)
	return f.groupTextErr
}

func (f *fakeDingTalkSender) SendOToMessage(ctx context.Context, userIDs []string, text string) error {
	f.o2oTexts = append(f.o2oTexts, text)
	return f.o2oTextErr
}

func (f *fakeDingTalkSender) SendGroupImage(ctx context.Context, openConversationID, photoURL string) error {
	f.groupImages = append(f.groupImages, photoURL)
	return f.groupImageErr
}

func (f *fakeDingTalkSender) SendOToImage(ctx context.Context, userIDs []string, photoURL string) error {
	f.o2oImages = append(f.o2oImages, photoURL)
	return f.o2oImageErr
}

func (f *fakeDingTalkSender) SendGroupFile(ctx context.Context, openConversationID, mediaID, fileName, fileType string) error {
	f.groupFiles = append(f.groupFiles, mediaID+"|"+fileName+"|"+fileType)
	return f.groupFileErr
}

func (f *fakeDingTalkSender) SendOToFile(ctx context.Context, userIDs []string, mediaID, fileName, fileType string) error {
	f.o2oFiles = append(f.o2oFiles, mediaID+"|"+fileName+"|"+fileType)
	return f.o2oFileErr
}

func (f *fakeDingTalkSender) UploadMedia(ctx context.Context, mediaType, fileName string, data []byte) (string, error) {
	f.uploadTypes = append(f.uploadTypes, mediaType)
	f.uploadNames = append(f.uploadNames, fileName)
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	if strings.TrimSpace(f.uploadMediaID) == "" {
		return "mock-media-id", nil
	}
	return f.uploadMediaID, nil
}

func newPluginWithFakeSender(sender dingTalkMessageSender) *DingTalkPlugin {
	plugin := NewDingTalkPlugin(nil)
	plugin.senders[channels.DefaultAccountID] = sender
	return plugin
}

func TestDingTalkSendMessage_BinaryImageSuccess(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadMediaID: "mid-image"}
	plugin := newPluginWithFakeSender(sender)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "cid_group_001",
		MediaData:     []byte{1, 2, 3},
		MediaMimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil || result.Channel != string(channels.ChannelDingTalk) {
		t.Fatalf("unexpected send result: %+v", result)
	}
	if len(sender.uploadTypes) != 1 || sender.uploadTypes[0] != "image" {
		t.Fatalf("expected one image upload, got %+v", sender.uploadTypes)
	}
	if len(sender.groupImages) != 1 || sender.groupImages[0] != "mid-image" {
		t.Fatalf("expected group image send with uploaded media id, got %+v", sender.groupImages)
	}
	if len(sender.groupTexts) != 0 {
		t.Fatalf("unexpected fallback text when media send succeeded: %+v", sender.groupTexts)
	}
}

func TestDingTalkSendMessage_BinaryFilePreservesFileName(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadMediaID: "mid-file"}
	plugin := newPluginWithFakeSender(sender)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "cid_group_001",
		MediaData:     []byte("hello"),
		MediaMimeType: "text/markdown",
		MediaFileName: "idlefish_agent_design.md",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if len(sender.uploadNames) != 1 || sender.uploadNames[0] != "idlefish_agent_design.md" {
		t.Fatalf("expected upload name preserved, got %+v", sender.uploadNames)
	}
}

func TestDingTalkSendMessage_BinaryUploadFailFallback(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadErr: errors.New("upload failed")}
	plugin := newPluginWithFakeSender(sender)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "ou_user_001",
		MediaData:     []byte{9, 9, 9},
		MediaMimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("SendMessage should fallback to text instead of returning error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}
	if len(sender.o2oTexts) != 1 {
		t.Fatalf("expected one fallback text message, got %+v", sender.o2oTexts)
	}
	if !strings.Contains(sender.o2oTexts[0], "媒体附件") {
		t.Fatalf("fallback text should explain media degradation, got: %s", sender.o2oTexts[0])
	}
}

func TestDingTalkSendMessage_TextStillSendsWhenBinaryFails(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadErr: errors.New("upload failed")}
	plugin := newPluginWithFakeSender(sender)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "ou_user_001",
		Text:          "hello",
		MediaData:     []byte{9, 9, 9},
		MediaMimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("SendMessage unexpected error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}
	if len(sender.o2oTexts) != 1 || sender.o2oTexts[0] != "hello" {
		t.Fatalf("expected only original text to be sent, got %+v", sender.o2oTexts)
	}
}

func TestDingTalkSendMessage_InvalidEmpty(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{}
	plugin := newPluginWithFakeSender(sender)

	_, err := plugin.SendMessage(channels.OutboundSendParams{
		To: "ou_user_001",
	})
	if err == nil {
		t.Fatalf("expected invalid request error")
	}
	sendErr, ok := channels.AsSendError(err)
	if !ok || sendErr == nil {
		t.Fatalf("expected SendError, got %v", err)
	}
	if sendErr.Code != channels.SendErrInvalidRequest {
		t.Fatalf("expected invalid_request code, got %s", sendErr.Code)
	}
}

func TestDingTalkSendMessage_ImageURLFailFallback(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{o2oImageErr: errors.New("image send failed")}
	plugin := newPluginWithFakeSender(sender)

	result, err := plugin.SendMessage(channels.OutboundSendParams{
		To:            "ou_user_001",
		MediaURL:      "https://example.com/a.png",
		MediaMimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("SendMessage should fallback to text instead of returning error: %v", err)
	}
	if result == nil {
		t.Fatalf("result should not be nil")
	}
	if len(sender.o2oTexts) != 1 {
		t.Fatalf("expected one fallback text message, got %+v", sender.o2oTexts)
	}
	if !strings.Contains(sender.o2oTexts[0], "媒体链接：https://example.com/a.png") {
		t.Fatalf("fallback text should include media URL, got: %s", sender.o2oTexts[0])
	}
}

func TestDingTalkSendDispatchReplyMedia_GroupUploadsAllItems(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadMediaID: "mid-upload"}
	plugin := newPluginWithFakeSender(sender)
	reply := &channels.DispatchReply{
		MediaItems: []channels.ChannelMediaItem{
			{Data: []byte{1, 2, 3}, MimeType: "image/png"},
			{Data: []byte{9, 9, 9}, MimeType: "application/pdf"},
		},
	}

	if err := plugin.sendDispatchReplyMedia(context.Background(), sender, reply, "cid_group_002", "ou_user_002", true); err != nil {
		t.Fatalf("sendDispatchReplyMedia unexpected error: %v", err)
	}
	if len(sender.uploadTypes) != 2 {
		t.Fatalf("expected two uploads, got %+v", sender.uploadTypes)
	}
	if sender.uploadTypes[0] != "image" || sender.uploadTypes[1] != "file" {
		t.Fatalf("unexpected upload types: %+v", sender.uploadTypes)
	}
	if len(sender.groupImages) != 1 || sender.groupImages[0] != "mid-upload" {
		t.Fatalf("expected one group image send, got %+v", sender.groupImages)
	}
	if len(sender.groupFiles) != 1 || !strings.Contains(sender.groupFiles[0], "mid-upload|") {
		t.Fatalf("expected one group file send, got %+v", sender.groupFiles)
	}
}

func TestDingTalkSendDispatchReplyMedia_LegacyFieldsFallback(t *testing.T) {
	t.Parallel()

	sender := &fakeDingTalkSender{uploadMediaID: "mid-image-legacy"}
	plugin := newPluginWithFakeSender(sender)
	reply := &channels.DispatchReply{
		MediaData:     []byte{7, 7, 7},
		MediaMimeType: "image/jpeg",
	}

	if err := plugin.sendDispatchReplyMedia(context.Background(), sender, reply, "cid_group_003", "ou_user_003", false); err != nil {
		t.Fatalf("sendDispatchReplyMedia unexpected error: %v", err)
	}
	if len(sender.o2oImages) != 1 || sender.o2oImages[0] != "mid-image-legacy" {
		t.Fatalf("expected o2o image send by legacy field, got %+v", sender.o2oImages)
	}
}
