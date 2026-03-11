package browser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDownloadDir_UsesSystemTempDir(t *testing.T) {
	if got := defaultDownloadDir(); got != filepath.Join(os.TempDir(), "openacosmi", "downloads") {
		t.Fatalf("defaultDownloadDir() = %q", got)
	}
}
