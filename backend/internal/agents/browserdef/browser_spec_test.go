package browserdef

import (
	"strings"
	"testing"
)

func TestToolInputSchema_ContainsFullActionSet(t *testing.T) {
	schema := string(ToolInputSchemaJSON())
	for _, token := range []string{
		"annotate_som",
		"start_gif_recording",
		"stop_gif_recording",
		"list_tabs",
		"create_tab",
		"close_tab",
		"switch_tab",
		"target_id",
	} {
		if !strings.Contains(schema, token) {
			t.Fatalf("browser schema should contain %q, got: %s", token, schema)
		}
	}
}

func TestSpec_DefaultsToExplicitVisualVerification(t *testing.T) {
	for _, action := range []string{"navigate", "click", "type", "click_ref", "fill_ref", "ai_browse", "switch_tab"} {
		if ActionAutoVerificationImage(action) {
			t.Fatalf("action %q should not auto-attach a verification image", action)
		}
	}
}

func TestUnavailableMessage_MentionsBrowserManagement(t *testing.T) {
	msg := UnavailableMessage()
	if !strings.Contains(msg, "browser-extension") {
		t.Fatalf("message should include setup guide URL, got: %q", msg)
	}
	if strings.Contains(msg, "26222") {
		t.Fatalf("message should not leak dev server port, got: %q", msg)
	}
	if !strings.Contains(msg, "Browser Management") {
		t.Fatalf("message should mention Browser Management, got: %q", msg)
	}
}
