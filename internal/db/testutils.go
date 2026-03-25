package db

import (
	"encoding/json"
	"io/fs"
	"os"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/source"
)

// Warning : These functions to manipulate files should be used only to test instance.go. Manipulating files in other parts of db package is prohibited.

const testDBPath = "/testdata/db.json"

func setupTest(t *testing.T, bom any) (teardown func()) {
	t.Helper()
	cos.SetMockedFileSystem()

	src, err := source.NewLocalSource(testDBPath)
	if err != nil {
		t.Fatal(err)
	}

	if bom == nil {
		if err := createFile(testDBPath); err != nil {
			t.Fatal(err)
		}
		return func() {
			if err := removeFile(testDBPath); err != nil {
				t.Fatal(err)
			}
			cos.SetRealFileSystem()
		}
	}
	if err := createConfigFile(bom, testDBPath); err != nil {
		t.Fatal(err)
	}
	err = Load(src)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		resetContent()
		if err := removeFile(testDBPath); err != nil {
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
