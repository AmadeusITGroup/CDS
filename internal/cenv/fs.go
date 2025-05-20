package cenv

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

func EnsureDir(path string, perm fs.FileMode) error {
	if fs.FileMode(0100)&perm == 0 {
		clog.Warn(fmt.Sprintf("Permissions for file '%s' don't grant execute perm to owner, directory won't be accessible", path))
	}

	if cos.Exists(path) {
		info, errInfo := cos.Fs.Stat(path)
		if errInfo != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to determine directory information for '%s'", path), errInfo)
		}

		if !info.IsDir() {
			return cerr.NewError(fmt.Sprintf("Target path '%s' already exist and is not a directory ", path))
		}

		if info.Mode().Perm() != perm {
			if errChmod := cos.Fs.Chmod(path, perm); errChmod != nil {
				return cerr.AppendError(fmt.Sprintf("Failed to apply permissions (%v) for directory '%s'", perm, path), errChmod)
			}
		}
		return nil
	}

	// On Windows deepest filepath is C://
	// meaning that C:// == filepath.Dir("C://")
	// However afero's NewMemMapFs is empty therefore Ensuredir has to create Root folder if it doesn't exist to avoid infinite loop
	if path != filepath.Dir(path) {
		if errEnsureParent := EnsureDir(filepath.Dir(path), perm); errEnsureParent != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to create parent directory for directory '%s'", path), errEnsureParent)
		}
	}

	if errMkdir := cos.Fs.Mkdir(path, perm); errMkdir != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to create directory for path '%s'", path), errMkdir)
	}

	return nil
}

func EnsureFile(path string, perm fs.FileMode) error {
	if cos.Exists(path) {
		info, errInfo := cos.Fs.Stat(path)
		if errInfo != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to determine file information for '%s'", path), errInfo)
		}

		if info.IsDir() {
			return cerr.NewError(fmt.Sprintf("Target path '%s' is a directory, cannot create file at this path", path))
		}

		if info.Mode().Perm() != perm {
			if errChmod := cos.Fs.Chmod(path, perm); errChmod != nil {
				return cerr.AppendError(fmt.Sprintf("Failed to apply permissions (%v) for file '%s'", perm, path), errChmod)
			}
		}
		return nil
	}

	if errEnsureParent := EnsureDir(filepath.Dir(path), cg.KPermDir); errEnsureParent != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to create parent directory for file '%s'", path), errEnsureParent)
	}

	file, errCreate := cos.Fs.Create(path)
	if errCreate != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to create directory for path '%s'", path), errCreate)
	}

	defer func() {
		_ = file.Close()
	}()

	if errChmod := cos.Fs.Chmod(file.Name(), perm); errChmod != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to apply permissions (%v) for file '%s'", perm, path), errChmod)
	}

	return nil
}

func CopyDir(src, dst string) error {
	return cos.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// copy to this path
		outputPath := filepath.Join(dst, strings.TrimPrefix(path, src))

		if info.IsDir() {
			return cos.Fs.MkdirAll(outputPath, info.Mode())
		}

		inputFile, errOpen := cos.Fs.Open(path)
		if errOpen != nil {
			return errOpen
		}
		defer func() {
			_ = inputFile.Close()
		}()

		// create output
		outputFile, errOpenOut := cos.Fs.Create(outputPath)
		if errOpenOut != nil {
			return errOpenOut
		}
		defer func() {
			_ = outputFile.Close()
		}()

		errChmod := cos.Fs.Chmod(outputFile.Name(), info.Mode())
		if errChmod != nil {
			return errChmod
		}

		_, err = io.Copy(outputFile, inputFile)
		return err
	})
}

func SmartCopy(sourcePath, destinationPath string) error {
	if runtime.GOOS == "windows" {
		info, errInfo := cos.Fs.Stat(sourcePath)
		if errInfo != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to determine directory information for '%s'", destinationPath), errInfo)
		}
		if info.IsDir() {
			if errCopy := CopyDir(sourcePath, destinationPath); errCopy != nil {
				return cerr.AppendErrorFmt("Failed to copy '%s' to '%s'", errCopy, sourcePath, destinationPath)
			}
			return nil
		}
		if errCopy := CopyFile(sourcePath, destinationPath); errCopy != nil {
			return cerr.AppendErrorFmt("Failed to copy '%s' to '%s'", errCopy, sourcePath, destinationPath)
		}
		return nil
	}
	if errSymlink := os.Symlink(sourcePath, destinationPath); errSymlink != nil {
		return cerr.AppendErrorFmt("Failed to create symbolic link from '%s' to '%s'", errSymlink, sourcePath, destinationPath)
	}

	return nil
}

func CopyFile(src, dst string) error {

	inputFile, errOpen := cos.Fs.Open(src)
	if errOpen != nil {
		return errOpen
	}
	defer func() {
		_ = inputFile.Close()
	}()

	outputFile, errOpenOut := cos.Fs.Create(dst)
	if errOpenOut != nil {
		return errOpenOut
	}
	defer func() {
		_ = outputFile.Close()
	}()

	info, _ := cos.Fs.Stat(src)
	errChmod := cos.Fs.Chmod(outputFile.Name(), info.Mode())
	if errChmod != nil {
		return errChmod
	}

	_, err := io.Copy(outputFile, inputFile)
	return err
}
