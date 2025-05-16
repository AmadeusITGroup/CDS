package shexec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	// cannot declare this as const, would have been otherwise
	NAS_ITEMS = [12]string{
		"archive", "intdeliv", "mw",
		"obedeliv1", "projects", "projects1",
		"projteams", "releasing", "tmp",
		"tools", "tslahpdelivc", "tspprojteams"}
)

// given a local path to a file and a remote InRemotePath, this function
// will return the name of the file to put in the remote as well as
// the path said file will have on the remote
// Note: the remote is expected to be a Unix system
func InRemotePath(srcPath, dstDir string) (srcFile string, dstPath string) {
	_, srcFile = filepath.Split(srcPath)
	dstPath = path.Join(dstDir, srcFile)
	return
}

func NasPresent(target target) bool {
	events := []ExecuteEvent{}
	events = append(events, &DefaultShEvent{ExeCmd: "ls -w 0 /remote", Host: target, DescriptionCmd: "Checking if NAS is present"})

	stdout, err := RunCmd(target)(events)

	isNas := true

	for _, nas_item := range NAS_ITEMS {
		isNas = isNas || strings.Contains(stdout, nas_item)
	}

	return err == nil && isNas
}

func Rm(target target, path string) error {
	events := []ExecuteEvent{}
	events = append(events, &DefaultShEvent{ExeCmd: fmt.Sprintf("rm %s", path), Host: target, DescriptionCmd: fmt.Sprintf("Removing file '%s'", path)})

	_, err := RunCmd(target)(events)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to create directory '%s' on target '%s'", err, path, target.FQDN())
	}

	return nil
}

// try opening a ssh connection to the remote and close it
// relies on the underlying RemoteExecute to perform the auth
func ValidateRemote(method RemoteExecute) error {
	var cmds []any
	cmd := &DefaultShEvent{ExeCmd: "exit", DescriptionCmd: "Validate connection to remote"}
	cmds = append(cmds, cmd)
	_, errors := method.execute(cmds, runSession)
	errors = cg.FilterNilFromSlice(errors)
	if len(errors) > 0 {
		return cerr.AppendMultipleErrors("Failed to validate credentials", errors)
	}

	return nil
}

func getCurrentWorkingDir() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", cerr.AppendError("Failed to get current working directory", err)
	}
	return workingDir, nil
}

func getUserHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		clog.Error("Failed to determine user home directory !", err)
	}
	return homeDir
}

type execErr struct {
	Message string
}

func (ee *execErr) Error() string {
	return ee.Message
}

type recoverErr struct {
	Message string
}

func (re *recoverErr) Error() string {
	return re.Message
}

func isLocalHost(hostName string) bool {
	if hostName == cg.KLocalhost {
		return true
	}
	runtimeHostName, err := os.Hostname()
	if err != nil {
		clog.Warn("Unable to get hostname", err)
		return false
	}
	fqdn := strings.Split(hostName, ".")
	fromOS := strings.ToLower(runtimeHostName)
	fromFQDN := strings.ToLower(fqdn[0])
	return fromOS == fromFQDN
}
