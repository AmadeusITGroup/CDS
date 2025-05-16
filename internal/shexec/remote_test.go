package shexec

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/stretchr/testify/assert"
)

func TestDestination(t *testing.T) {
	var src, dst, file, remoteDst, expectedFile, expectedRemoteDst string

	// filepath behavior is not alterable, eg we cannot use it to manipulate windows paths on linux
	switch runtime.GOOS {
	case "windows":
		src = `C:\Users\user\.cds\cache\features\somefeat\0.1.0\pre\script.sh`
		dst = "."
		expectedFile = "script.sh"
		expectedRemoteDst = "script.sh"

		file, remoteDst = InRemotePath(src, dst)
		assert.Equal(t, expectedFile, file)
		assert.Equal(t, expectedRemoteDst, remoteDst)

		src = `C:\Users\user\.cds\cache\features\somefeat\0.1.0\pre\script.sh`
		dst = "workspace"
		expectedFile = "script.sh"
		expectedRemoteDst = "workspace/script.sh"

		file, remoteDst = InRemotePath(src, dst)
		assert.Equal(t, expectedFile, file)
		assert.Equal(t, expectedRemoteDst, remoteDst)
	default:
		src = "/home/user/.cds/cache/feature/somefeat/1.0.0/pre/script.sh"
		dst = "workspace"
		expectedFile = "script.sh"
		expectedRemoteDst = "workspace/script.sh"

		file, remoteDst = InRemotePath(src, dst)
		assert.Equal(t, expectedFile, file)
		assert.Equal(t, expectedRemoteDst, remoteDst)

		src = "/home/user/.cds/cache/feature/somefeat/1.0.0/pre/script.sh"
		dst = "."
		expectedFile = "script.sh"
		expectedRemoteDst = "script.sh"

		file, remoteDst = InRemotePath(src, dst)
		assert.Equal(t, expectedFile, file)
		assert.Equal(t, expectedRemoteDst, remoteDst)
	}
}

func TestGeneratePubKeyEcdsa(t *testing.T) {
	basepath := filepath.Join("..", "tests", "resources", "common", "remote")
	//clean any pre-existing file
	pubKey := filepath.Join(basepath, "test_ecdsa.pub")
	if cos.Exists(pubKey) {
		if err := cos.Fs.Remove(pubKey); err != nil {
			t.Fatal(err)
		}
	}

	src := filepath.Join(basepath, "test_ecdsa")
	ref := filepath.Join(basepath, "test_ecdsa.pub.ref")

	dst, err := GeneratePublicKey(src)

	if err != nil {
		t.Fatal(err)
	}

	dstBytes, errReadDst := cos.ReadFile(dst)
	if errReadDst != nil {
		t.Fatal(errReadDst)
	}

	refBytes, errReadRef := cos.ReadFile(ref)
	if errReadRef != nil {
		t.Fatal(errReadRef)
	}

	refBytes, _ = addCdsWatermarkToPublicKey(refBytes)
	assert.Equal(t, string(refBytes), string(dstBytes))
}

func TestGeneratePubKeyEd25519(t *testing.T) {
	basepath := filepath.Join("..", "tests", "resources", "common", "remote")
	// clean any pre-existing file
	pubKey := filepath.Join(basepath, "test_ed25519.pub")
	if cos.Exists(pubKey) {
		if err := cos.Fs.Remove(pubKey); err != nil {
			t.Fatal(err)
		}
	}

	src := filepath.Join(basepath, "test_ed25519")
	ref := filepath.Join(basepath, "test_ed25519.pub.ref")

	dst, err := GeneratePublicKey(src)

	if err != nil {
		t.Fatal(err)
	}

	dstBytes, errReadDst := cos.ReadFile(dst)
	if errReadDst != nil {
		t.Fatal(errReadDst)
	}

	refBytes, errReadRef := cos.ReadFile(ref)
	if errReadRef != nil {
		t.Fatal(errReadRef)
	}

	refBytes, _ = addCdsWatermarkToPublicKey(refBytes)
	assert.Equal(t, string(refBytes), string(dstBytes))
}
