package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	managedSSHHostBlockStartPrefix = "# >>> CDS managed host "
	managedSSHHostBlockEndPrefix   = "# <<< CDS managed host "
)

type sshHostConfigEntry struct {
	alias        string
	hostName     string
	port         int
	user         string
	identityFile string
}

func upsertProjectContainerSSHConfig(projectName string, container bo.Container, requirePort bool) error {
	containerName := strings.TrimSpace(string(container.Name))
	port := container.Pmapping[bo.KSSHPortMapping]
	if port <= 0 {
		if err := removeManagedSSHHostEntry(containerName); err != nil {
			return err
		}
		if requirePort {
			return cerr.NewError(fmt.Sprintf("Container %q does not expose an SSH port mapping", containerName))
		}
		clog.Warn(fmt.Sprintf("Container %q does not expose an SSH port mapping; skipping SSH host config", containerName))
		return nil
	}

	remoteUser := strings.TrimSpace(string(container.RemoteUser))
	if remoteUser == cg.EmptyStr {
		remoteUser = engine.KRootUsr
	}

	return upsertManagedSSHHostEntry(sshHostConfigEntry{
		alias:        containerName,
		hostName:     projectContainerSSHHostName(projectName),
		port:         port,
		user:         remoteUser,
		identityFile: defaultSSHIdentityFile(),
	})
}

func projectContainerSSHHostName(projectName string) string {
	hostName := strings.TrimSpace(projectAgentHost(projectName))
	if hostName == cg.EmptyStr {
		return cg.KLocalhost
	}
	return hostName
}

func upsertManagedSSHHostEntry(entry sshHostConfigEntry) error {
	if err := validateSSHConfigToken("SSH host alias", entry.alias); err != nil {
		return err
	}
	if err := validateSSHConfigToken("SSH host name", entry.hostName); err != nil {
		return err
	}
	if err := validateSSHConfigToken("SSH user", entry.user); err != nil {
		return err
	}
	if entry.port <= 0 {
		return cerr.NewError(fmt.Sprintf("Invalid SSH port %d for host %q", entry.port, entry.alias))
	}

	configPath, content, err := readSSHConfig()
	if err != nil {
		return err
	}

	cleaned := removeManagedSSHHostBlock(content, entry.alias)
	nextContent := renderManagedSSHHostBlock(entry)
	if strings.TrimSpace(cleaned) != cg.EmptyStr {
		nextContent += "\n" + strings.TrimLeft(cleaned, "\n")
	}
	return writeSSHConfig(configPath, nextContent)
}

func removeManagedSSHHostEntry(alias string) error {
	if strings.TrimSpace(alias) == cg.EmptyStr {
		return nil
	}
	if err := validateSSHConfigToken("SSH host alias", alias); err != nil {
		return err
	}

	configPath, content, err := readSSHConfig()
	if err != nil {
		return err
	}
	return writeSSHConfig(configPath, removeManagedSSHHostBlock(content, alias))
}

func readSSHConfig() (string, string, error) {
	if err := cenv.EnsureSSHClientConfig(nil); err != nil {
		return cg.EmptyStr, cg.EmptyStr, err
	}
	configPath, err := defaultSSHConfigPath()
	if err != nil {
		return cg.EmptyStr, cg.EmptyStr, err
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return cg.EmptyStr, cg.EmptyStr, cerr.AppendError(fmt.Sprintf("Failed to read SSH config %s", configPath), err)
	}
	return configPath, string(content), nil
}

func writeSSHConfig(configPath, content string) error {
	if content != cg.EmptyStr && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(configPath, []byte(content), cg.KPermFile); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to write SSH config %s", configPath), err)
	}
	return nil
}

func defaultSSHConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to determine user home directory", err)
	}
	return filepath.Join(homeDir, ".ssh", "config"), nil
}

func renderManagedSSHHostBlock(entry sshHostConfigEntry) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s%s\n", managedSSHHostBlockStartPrefix, entry.alias)
	fmt.Fprintf(&builder, "Host %s\n", entry.alias)
	fmt.Fprintf(&builder, "    HostName %s\n", entry.hostName)
	fmt.Fprintf(&builder, "    Port %d\n", entry.port)
	fmt.Fprintf(&builder, "    User %s\n", entry.user)
	if entry.identityFile != cg.EmptyStr {
		fmt.Fprintf(&builder, "    IdentityFile %s\n", sshConfigValue(entry.identityFile))
		fmt.Fprintf(&builder, "    IdentitiesOnly yes\n")
	}
	fmt.Fprintf(&builder, "    StrictHostKeyChecking accept-new\n")
	fmt.Fprintf(&builder, "%s%s\n", managedSSHHostBlockEndPrefix, entry.alias)
	return builder.String()
}

func removeManagedSSHHostBlock(content, alias string) string {
	start := managedSSHHostBlockStartPrefix + alias
	end := managedSSHHostBlockEndPrefix + alias
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		switch {
		case trimmed == start:
			skipping = true
		case skipping && trimmed == end:
			skipping = false
		case !skipping:
			kept = append(kept, line)
		}
	}
	return strings.TrimLeft(strings.Join(kept, "\n"), "\n")
}

func validateSSHConfigToken(label, value string) error {
	value = strings.TrimSpace(value)
	if value == cg.EmptyStr || strings.ContainsAny(value, " \t\r\n") {
		return cerr.NewError(fmt.Sprintf("%s %q is not valid for SSH config", label, value))
	}
	return nil
}

func sshConfigValue(value string) string {
	if !strings.ContainsAny(value, " \t\"\\") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func defaultSSHIdentityFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cg.EmptyStr
	}
	for _, fileName := range []string{"id_rsa", "id_ed25519"} {
		privateKeyPath := filepath.Join(homeDir, ".ssh", fileName)
		publicKeyPath := privateKeyPath + ".pub"
		if fileExists(privateKeyPath) && fileExists(publicKeyPath) {
			return privateKeyPath
		}
	}
	return cg.EmptyStr
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
