package command

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// Containers are reachable with `ssh <container name>` through blocks managed
// by cds in the user SSH config, delimited by these markers.
const (
	kManagedSSHHostBlockStart = "# >>> CDS managed host "
	kManagedSSHHostBlockEnd   = "# <<< CDS managed host "
)

type sshHostConfigEntry struct {
	alias        string
	hostName     string
	port         int
	user         string
	identityFile string
}

// upsertContainerSSHConfig makes the container reachable through SSH when it
// publishes its SSH port on hostName, and unreachable otherwise.
func upsertContainerSSHConfig(hostName string, container bo.Container) error {
	containerName := string(container.Name)
	port := container.Pmapping[bo.KSSHPortMapping]
	if port <= 0 {
		clog.Debug(fmt.Sprintf("Container %q does not publish its SSH port, it is not reachable through SSH", containerName))
		return removeContainerSSHConfig(containerName)
	}

	user := string(container.RemoteUser)
	if user == cg.EmptyStr {
		user = engine.KRootUsr
	}
	identityFile, _, _ := containerconf.DefaultSSHKeyPair()

	entry := sshHostConfigEntry{alias: containerName, hostName: hostName, port: port, user: user, identityFile: identityFile}
	for label, value := range map[string]string{"SSH host alias": entry.alias, "SSH host name": entry.hostName, "SSH user": entry.user} {
		if err := validateSSHConfigToken(label, value); err != nil {
			return err
		}
	}

	return updateSSHConfig(func(content string) string {
		content = removeManagedSSHHostBlock(content, entry.alias)
		if strings.TrimSpace(content) == cg.EmptyStr {
			return renderManagedSSHHostBlock(entry)
		}
		return renderManagedSSHHostBlock(entry) + "\n" + content
	})
}

// removeContainerSSHConfig removes the SSH config block of the container, if any.
func removeContainerSSHConfig(containerName string) error {
	if err := validateSSHConfigToken("SSH host alias", containerName); err != nil {
		return err
	}
	return updateSSHConfig(func(content string) string {
		return removeManagedSSHHostBlock(content, containerName)
	})
}

func updateSSHConfig(update func(content string) string) error {
	if err := cenv.EnsureSSHClientConfig(nil); err != nil {
		return err
	}
	path := cenv.SSHConfigPath()
	data, err := cos.ReadFile(path)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read SSH config %s", err, path)
	}

	content := string(data)
	updated := update(content)
	if updated == content {
		return nil
	}
	if updated != cg.EmptyStr && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if err := cos.WriteFile(path, []byte(updated), cg.KPermFile); err != nil {
		return cerr.AppendErrorFmt("Failed to write SSH config %s", err, path)
	}
	return nil
}

func renderManagedSSHHostBlock(entry sshHostConfigEntry) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s%s\n", kManagedSSHHostBlockStart, entry.alias)
	fmt.Fprintf(&builder, "Host %s\n", entry.alias)
	fmt.Fprintf(&builder, "    HostName %s\n", entry.hostName)
	fmt.Fprintf(&builder, "    Port %d\n", entry.port)
	fmt.Fprintf(&builder, "    User %s\n", entry.user)
	if entry.identityFile != cg.EmptyStr {
		fmt.Fprintf(&builder, "    IdentityFile %s\n", sshConfigValue(entry.identityFile))
		fmt.Fprintf(&builder, "    IdentitiesOnly yes\n")
	}
	fmt.Fprintf(&builder, "    StrictHostKeyChecking accept-new\n")
	fmt.Fprintf(&builder, "%s%s\n", kManagedSSHHostBlockEnd, entry.alias)
	return builder.String()
}

func removeManagedSSHHostBlock(content, alias string) string {
	start := kManagedSSHHostBlockStart + alias
	end := kManagedSSHHostBlockEnd + alias
	if !strings.Contains(content, start) {
		return content
	}

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
	if value == cg.EmptyStr || strings.ContainsAny(value, " \t\r\n\"'#") {
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
