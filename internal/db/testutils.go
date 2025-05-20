package db

import (
	"encoding/json"
	"io/fs"
	"os"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cos"
)

// Warning : These functions to manipulate files should be used only to test instance.go. Manipulating files in other parts of db package is prohibited.

func setupTest(t *testing.T, bom any) (teardown func()) {
	t.Helper()
	cos.SetMockedFileSystem()
	if bom == nil {
		if err := createFile(cenv.ConfigFile(kCdsStateFile)); err != nil {
			t.Fatal(err)
		}
		return func() {
			if err := removeFile(cenv.ConfigFile(kCdsStateFile)); err != nil {
				t.Fatal(err)
			}
			cos.SetRealFileSystem()
		}
	}
	if err := createConfigFile(bom, cenv.ConfigFile(kCdsStateFile)); err != nil {
		t.Fatal(err)
	}
	err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		resetContent()
		if err := removeFile(cenv.ConfigFile(kCdsStateFile)); err != nil {
			t.Fatal(err)
		}
		cos.SetRealFileSystem()
	}
}

func createFile(pathToFile string) error {
	_, err := cos.Fs.Create(pathToFile)
	return err
}

func removeFile(pathToFile string) error {
	err := cos.Fs.Remove(pathToFile)
	return err
}

func createConfigFile(bom any, pathToFile string) error {
	data, err := json.Marshal(bom)
	if err != nil {
		return err
	}

	file, err := cos.Fs.OpenFile(pathToFile, os.O_CREATE|os.O_RDWR, fs.FileMode(0600))
	if err != nil {
		return err
	}

	err = cos.WriteFile(pathToFile, data, fs.FileMode(0600))
	if err != nil {
		return err
	}

	defer func() {
		_ = file.Close()
	}()
	return nil
}
