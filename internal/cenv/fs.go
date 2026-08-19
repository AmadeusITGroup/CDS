package cenv

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kTmp        = "tmp"
	kTmpPattern = "tmp-file-*"
	kTmpMaxAge  = 24 * time.Hour
)

var managedTmpPrefixes = []string{
	strings.TrimSuffix(kTmpPattern, "*"),
	"cds-tmp-",
}

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

func CreateTempFileWithContent(reader io.Reader) (string, error) {
	tmpDir := ConfigDir(kTmp)
	if err := EnsureDir(tmpDir, cg.KPermDir); err != nil {
		return "", cerr.AppendErrorFmt("Failed ensuring tmp directory %s", err, tmpDir)
	}
	tmpFilePath, err := cos.CreateTempFileWithContent(tmpDir, kTmpPattern, reader)
	if err != nil {
		return "", cerr.AppendErrorFmt("Failed creating tmp file in local (%s,%s)", err, tmpDir, kTmpPattern)
	}

	return tmpFilePath, nil
}

func RemoveTempFile(path string) error {
	tmpDir := filepath.Clean(ConfigDir(kTmp))
	cleanPath := filepath.Clean(path)
	relPath, err := filepath.Rel(tmpDir, cleanPath)
	if err != nil || relPath == "." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) || relPath == ".." || filepath.IsAbs(relPath) {
		return cerr.NewError(fmt.Sprintf("refusing to remove non-temp file %s", path))
	}
	if err := cos.Fs.Remove(cleanPath); err != nil && !os.IsNotExist(err) {
		return cerr.AppendError(fmt.Sprintf("Failed to remove temporary file %s", cleanPath), err)
	}
	return nil
}

func CleanupStaleTempFiles() error {
	return cleanupStaleTempFiles(time.Now(), kTmpMaxAge)
}

func cleanupStaleTempFiles(now time.Time, maxAge time.Duration) error {
	tmpDir := ConfigDir(kTmp)
	entries, err := cos.ReadDir(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return cerr.AppendError(fmt.Sprintf("Failed to read temporary directory %s", tmpDir), err)
	}

	cutoff := now.Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() || !isManagedTempFile(entry.Name()) || entry.ModTime().After(cutoff) {
			continue
		}
		if err := cos.Fs.Remove(filepath.Join(tmpDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return cerr.AppendError(fmt.Sprintf("Failed to remove stale temporary file %s", entry.Name()), err)
		}
	}
	return nil
}

func isManagedTempFile(name string) bool {
	for _, prefix := range managedTmpPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
