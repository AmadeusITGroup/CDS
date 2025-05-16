package cos

import (
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// Wrapper to use afero file system utilities https://github.com/spf13/afero
// All the available functions are listed here : https://github.com/spf13/afero#using-aferos-utility-functions

type File afero.File

var Fs = afero.NewOsFs()

func ReadFile(filename string) ([]byte, error) {
	return afero.ReadFile(Fs, filename)
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(Fs, filename, data, perm)
}

func Exists(path string) bool {
	result, _ := afero.Exists(Fs, path)
	return result
}

func NotExist(path string) bool {
	return !Exists(path)
}

func Rename(oldname string, newName string) error {
	return Fs.Rename(oldname, newName)
}

func Walk(root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(Fs, root, walkFn)
}

func ReadDir(dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(Fs, dirname)
}

func SetMockedFileSystem() {
	Fs = afero.NewMemMapFs()
}

func SetRealFileSystem() {
	Fs = afero.NewOsFs()
}
