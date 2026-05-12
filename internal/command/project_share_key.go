package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

const projectSharedKeyUser = "root"

func ensureProjectSharedKeyPair(projectName string) (shexec.KeyPair, error) {
	keyPair := projectSharedKeyPairPaths(projectName)
	if _, err := os.Stat(keyPair.PathToPrv); err == nil {
		if _, pubErr := os.Stat(keyPair.PathToPub); pubErr == nil {
			return keyPair, nil
		}
		publicKeyPath, pubErr := shexec.GeneratePublicKey(keyPair.PathToPrv)
		if pubErr != nil {
			return shexec.KeyPair{}, pubErr
		}
		keyPair.PathToPub = publicKeyPath
		return keyPair, nil
	}

	return shexec.GenerateKeyPair(projectSharedKeySuffix(projectName))
}

func projectSharedKeyPairPaths(projectName string) shexec.KeyPair {
	suffix := projectSharedKeySuffix(projectName)
	return shexec.KeyPair{
		PathToPrv: filepath.Join(cenv.GetUserHomeDir(), ".ssh", fmt.Sprintf("id_rsa_%s", suffix)),
		PathToPub: filepath.Join(cenv.GetUserHomeDir(), ".ssh", fmt.Sprintf("id_rsa_%s.pub", suffix)),
	}
}

func projectSharedKeySuffix(projectName string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, projectName)
	if strings.Trim(sanitized, "_-") == "" {
		return "cds_share_project"
	}
	return "cds_share_" + sanitized
}

func projectSharedPublicKey(projectName string) (string, shexec.KeyPair, error) {
	keyPair, err := ensureProjectSharedKeyPair(projectName)
	if err != nil {
		return "", shexec.KeyPair{}, err
	}
	publicKey, err := os.ReadFile(keyPair.PathToPub)
	if err != nil {
		return "", shexec.KeyPair{}, cerr.AppendError(fmt.Sprintf("Failed to read shared public key %s", keyPair.PathToPub), err)
	}
	return strings.TrimSpace(string(publicKey)), keyPair, nil
}

func existingProjectSharedPublicKey(projectName string) (string, shexec.KeyPair, error) {
	keyPair := projectSharedKeyPairPaths(projectName)
	if _, err := os.Stat(keyPair.PathToPub); err != nil {
		return "", shexec.KeyPair{}, cerr.AppendError(fmt.Sprintf("Shared public key %s is not available", keyPair.PathToPub), err)
	}
	publicKey, err := os.ReadFile(keyPair.PathToPub)
	if err != nil {
		return "", shexec.KeyPair{}, cerr.AppendError(fmt.Sprintf("Failed to read shared public key %s", keyPair.PathToPub), err)
	}
	return strings.TrimSpace(string(publicKey)), keyPair, nil
}

func installSharedKeyCommand(remoteUser, publicKey string) string {
	script := fmt.Sprintf(`set -e; user=%s; key=%s; if ! id "$user" >/dev/null 2>&1; then if command -v useradd >/dev/null 2>&1; then useradd -m -s /bin/bash "$user"; elif command -v adduser >/dev/null 2>&1; then adduser -D "$user"; fi; fi; home_dir="$(getent passwd "$user" | cut -d: -f6)"; if [ -z "$home_dir" ]; then home_dir="/home/$user"; fi; group="$(id -gn "$user")"; mkdir -p "$home_dir/.ssh"; chmod 700 "$home_dir/.ssh"; touch "$home_dir/.ssh/authorized_keys"; grep -Fxq "$key" "$home_dir/.ssh/authorized_keys" || echo "$key" >> "$home_dir/.ssh/authorized_keys"; chmod 600 "$home_dir/.ssh/authorized_keys"; chown -R "$user:$group" "$home_dir/.ssh"`, shellQuote(remoteUser), shellQuote(publicKey))
	return script
}

func removeSharedKeyCommand(remoteUser, publicKey string) string {
	script := fmt.Sprintf(`set -e; user=%s; key=%s; home_dir="$(getent passwd "$user" | cut -d: -f6)"; if [ -z "$home_dir" ]; then home_dir="/home/$user"; fi; auth="$home_dir/.ssh/authorized_keys"; if [ -f "$auth" ]; then tmp="$(mktemp)"; grep -Fxv "$key" "$auth" > "$tmp" || true; cat "$tmp" > "$auth"; rm -f "$tmp"; chmod 600 "$auth"; if id "$user" >/dev/null 2>&1; then chown "$user:$(id -gn "$user")" "$auth"; fi; fi`, shellQuote(remoteUser), shellQuote(publicKey))
	return script
}

func removeProjectSharedKeyPair(projectName string) {
	keyPair := projectSharedKeyPairPaths(projectName)
	for _, path := range []string{keyPair.PathToPrv, keyPair.PathToPub} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			clog.Warn(fmt.Sprintf("Failed to remove shared key file %s", path), err)
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
