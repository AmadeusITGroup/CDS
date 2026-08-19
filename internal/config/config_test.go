package config

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/spf13/afero"
)

func setupConfigTestFS(t *testing.T) {
	t.Helper()

	cos.Fs = afero.NewMemMapFs()
	cenv.SetConfigDirForClient()
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(func() {
		cenv.SetConfigDirForClient()
		cos.SetRealFileSystem()
	})
}

func setupAgentConfigTestFS(t *testing.T) {
	t.Helper()

	setupConfigTestFS(t)
	cenv.SetConfigDirForAgent()
	t.Cleanup(cenv.SetConfigDirForClient)
}
