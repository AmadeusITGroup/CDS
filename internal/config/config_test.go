package config

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/spf13/afero"
)

func setupConfigTestFS(t *testing.T) {
	t.Helper()

	cos.Fs = afero.NewMemMapFs()
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(func() {
		cos.SetRealFileSystem()
	})
}
