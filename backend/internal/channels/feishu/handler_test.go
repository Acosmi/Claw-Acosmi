package feishu

import "testing"

func TestExtractMultimodalMessage_PostPreservesInlineImages(t *testing.T) {
	msg := &FeishuMessageEvent{
		Message: &FeishuMessageInfo{
			MessageID:   "msg-post-1",
			MessageType: "post",
			Content: `{
				"zh_cn": {
					"title": "初始化向导",
					"content": [
						[
							{"tag":"text","text":"还有3个截图，"},
							{"tag":"a","text":"入口","href":"https://example.com/wizard"}
						],
						[
							{"tag":"img","image_key":"img-key-1"},
							{"tag":"img","image_key":"img-key-2"}
						]
					]
				}
			}`,
		},
	}

	result := ExtractMultimodalMessage(msg)
	if result == nil {
		t.Fatal("expected parsed message")
	}
	if result.MessageType != "text" {
		t.Fatalf("messageType=%q, want text", result.MessageType)
	}
	wantText := "初始化向导\n还有3个截图，入口(https://example.com/wizard)"
	if result.Text != wantText {
		t.Fatalf("text=%q, want %q", result.Text, wantText)
	}
	if len(result.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(result.Attachments))
	}
	if result.Attachments[0].Category != "image" || result.Attachments[0].FileKey != "img-key-1" {
		t.Fatalf("unexpected first attachment: %+v", result.Attachments[0])
	}
	if result.Attachments[1].Category != "image" || result.Attachments[1].FileKey != "img-key-2" {
		t.Fatalf("unexpected second attachment: %+v", result.Attachments[1])
	}
}

func TestExtractMultimodalMessage_PostImageOnlyStillKeepsAttachment(t *testing.T) {
	msg := &FeishuMessageEvent{
		Message: &FeishuMessageInfo{
			MessageID:   "msg-post-2",
			MessageType: "post",
			Content: `{
				"zh_cn": {
					"content": [
						[
							{"tag":"img","image_key":"img-only-1"}
						]
					]
				}
			}`,
		},
	}

	result := ExtractMultimodalMessage(msg)
	if result == nil {
		t.Fatal("expected parsed message")
	}
	if result.Text != "" {
		t.Fatalf("text=%q, want empty", result.Text)
	}
	if len(result.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result.Attachments))
	}
	if result.Attachments[0].FileKey != "img-only-1" {
		t.Fatalf("unexpected attachment: %+v", result.Attachments[0])
	}
}
