package common

import "testing"

func TestNormalizeProviderIdForAuth(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "qwen", want: "qwen-portal"},
		{input: "minimax", want: "minimax-portal"},
		{input: "volcengine-plan", want: "volcengine"},
		{input: "byteplus-plan", want: "byteplus"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeProviderIdForAuth(tt.input); got != tt.want {
				t.Fatalf("NormalizeProviderIdForAuth(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
