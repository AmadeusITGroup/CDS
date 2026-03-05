package engine

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

func chownFileInTarget(engineName, containerName, user, remoteUserUID, remoteUserGID, targetFile string) error {
	exChOwn := NewContainerEngine()
	SetEngineName(exChOwn, engineName)
	exChOwn.SetAction(K_ACTION_EXE)
	exChOwn.SetExecuteCmd(K_EXEC_CMD_CHOWN)
	exChOwn.SetTargetUserOnContainer(remoteUserUID, remoteUserGID)
	exChOwn.SetTargetUserFileOnContainer(targetFile)
	exChOwn.SetContainerName(containerName)
	exChOwn.SetRemoteUser(KRootUsr)

	if _, err := ExecuteCommand(exChOwn, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendErrorFmt("Failed to change .netrc file permissions in container (%s) for user (%s)\n%s",
			err, containerName, user)
	}

	return nil
}

func chmodFileInTarget(engineName, containerName, user, targetFile string, filePerm fs.FileMode) error {
	exchmod := NewContainerEngine()
	SetEngineName(exchmod, engineName)
	exchmod.SetAction(K_ACTION_EXE)
	exchmod.SetExecuteCmd(K_EXEC_CMD_CHMOD_DEST_FILE)
	exchmod.SetTargetUserFileOnContainer(targetFile)
	exchmod.SetContainerName(containerName)
	exchmod.SetRemoteUser(KRootUsr)
	exchmod.SetDestinationFilePerm(filePerm)

	if _, err := ExecuteCommand(exchmod, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendErrorFmt("Failed to change file permissions to (%d) for file (%s) in container (%s) \n", err, targetFile, containerName)
	}

	return nil
}

func MkdirInTarget(engineName, containerName /*user,*/, targetDir string) error { // TODO check that the user is not needed
	ceMkdir := NewContainerEngine()
	SetEngineName(ceMkdir, engineName)
	ceMkdir.SetAction(K_ACTION_EXE)
	ceMkdir.SetExecuteCmd(K_EXEC_CMD_MKDIR)
	ceMkdir.SetTargetUserFileOnContainer(targetDir)
	ceMkdir.SetContainerName(containerName)

	if _, err := ExecuteCommand(ceMkdir, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendErrorFmt("Failed to create directory '' in container '%s'",
			err, targetDir, containerName)
	}

	return nil
}

func getUserUidGid(engineName, containerName, user string) (string, string, error) {
	exId := NewContainerEngine()
	SetEngineName(exId, engineName)
	exId.SetAction(K_ACTION_EXE)
	exId.SetExecuteCmd(K_EXEC_CMD_ID)
	exId.SetContainerName(containerName)
	exId.SetRemoteUser(user)

	var idOutput string
	var idErr error
	if idOutput, idErr = ExecuteCommand(exId, shexec.RunLocalCmdWithOutput); idErr != nil {
		clog.Warn(fmt.Sprintf("Failed to execute id command in container (%s)\n%s",
			containerName, idErr),
		)
	}

	idRegExp := regexp.MustCompile(`uid=(?P<UID>\d+)\([\w|\-|\.]+\) gid=(?P<GID>\d+)\([\w|\-|\.]+\).+`)
	if !idRegExp.MatchString(idOutput) {
		clog.Warn(fmt.Sprintf("Failed to parse id command output: '%s'. Output doesn't match regexp", idOutput))
		return "", "", nil
	}
	matches := idRegExp.FindStringSubmatch(idOutput)
	idxUID := idRegExp.SubexpIndex("UID")
	idxGID := idRegExp.SubexpIndex("GID")
	if idxUID == -1 || idxGID == -1 {
		clog.Warn(fmt.Sprintf("Failed to parse id command output: '%s'. Unable to get both UID and GID", idOutput))
	}
	remoteUserUID := matches[idxUID]
	remoteUserGID := matches[idxGID]
	clog.Debug(fmt.Sprintf("remote user (%s) has UID %s and GID %s", user, remoteUserUID, remoteUserGID))

	return remoteUserUID, remoteUserGID, nil
}

func copyFileToTarget(sourceOnTargetHost, destinationOnContainer, engineName, containerName, user string) error {
	ecp := NewContainerEngine()
	SetEngineName(ecp, engineName)
	ecp.SetAction(K_ACTION_CP)
	ecp.SetCopyActionType(K_CP_DEFAULT)
	ecp.SetSourceCopyPath(sourceOnTargetHost)
	ecp.SetCopyFinalDestinationOnContainer(destinationOnContainer)
	ecp.SetContainerName(containerName)
	ecp.SetRemoteUser(user)

	if err := ExecuteCommands(ecp, shexec.RunLocalCmds); err != nil {
		return cerr.AppendErrorFmt("Failed to copy file to container (%s)\n%s",
			err, containerName)
	}

	return nil
}

// Copy the file from the host to the container.
// File destination will be destDirOnContainer/destFileNameOnContainer.
func CopyFileFromHostToContainer(pathToSourceCopyFileOnTargetHost, destDirOnContainer, destFileNameOnContainer, engineName string, containerInfo bo.Container, filePerm fs.FileMode) error {
	containerName := string(containerInfo.Name)

	destinationFileNameOnContainer := path.Base(pathToSourceCopyFileOnTargetHost)
	// Override default destination file name if requested
	if len(destFileNameOnContainer) > 0 {
		destinationFileNameOnContainer = destFileNameOnContainer
	}
	// In case one is tempted by filepath.Join, it is not adapted here!
	destinationFilePathOnContainer := path.Join("/", strings.Trim(string(destDirOnContainer), " \n"), destinationFileNameOnContainer)

	// Podman is not able to copy into non-existing paths. Potential parent folders are created when necessary.
	if err := MkdirInTarget(engineName, containerName /*string(containerInfo.User),*/, destDirOnContainer); err != nil { // TODO check that the user is not needed
		return cerr.AppendErrorFmt("Failed to create directory '%s' in container '%s'",
			err, destDirOnContainer, containerName)
	}

	if err := copyFileToTarget(pathToSourceCopyFileOnTargetHost, destinationFilePathOnContainer, engineName, containerName, string(containerInfo.RemoteUser)); err != nil {
		return cerr.AppendErrorFmt("Failed to copy %s file to container (%s)\n%s",
			err, destinationFileNameOnContainer, containerName)
	}
	remoteUserUID, remoteUserGID, errId := getUserUidGid(engineName, containerName, string(containerInfo.RemoteUser)) //TODO: Analyse ci.User -> ci.RemoteUser maybe regression maybe not. Change done on localhost 008 test
	if errId != nil {
		return cerr.AppendErrorFmt("Failed to retrieve uid/gid of user '%s' in container '%s'", errId, string(containerInfo.RemoteUser), containerName)
	}

	if errChown := chownFileInTarget(engineName, containerName, string(containerInfo.RemoteUser), remoteUserUID, remoteUserGID, destinationFilePathOnContainer); errChown != nil {
		return cerr.AppendErrorFmt("Failed to change ownership of file '%s' in container '%s'", errChown, destinationFilePathOnContainer, containerName)
	}

	if errChmod := chmodFileInTarget(engineName, containerName, string(containerInfo.RemoteUser), destinationFilePathOnContainer, filePerm); errChmod != nil {
		return cerr.AppendErrorFmt("Failed to change file permission of file '%s' to %d in container '%s'", errChmod, destinationFileNameOnContainer, filePerm, containerName)
	}

	return nil
}

func GetRemoteUserHomeDirOnContainer(engineName string, containerName string, user string) (string, error) {
	ex := NewContainerEngine()
	SetEngineName(ex, engineName)
	ex.SetAction(K_ACTION_EXE)
	ex.SetContainerName(containerName)
	ex.SetExecuteCmd(K_EXEC_CMD_HOMEDIR)
	ex.SetRemoteUser(user)

	var containerHomeDir string
	var err error
	containerHomeDir, err = ExecuteCommand(ex, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return "", cerr.AppendError(
			fmt.Sprintf("Failed to get HomeDir for user %v on container (%v)", user, containerName),
			err)
	}
	return strings.TrimSpace(containerHomeDir), nil
}

func IsContainerReady(containerName string, user string) bool {
	status, err := GetContainerStatus(containerName, "")
	if err != nil {
		clog.Warn(fmt.Sprintf("Failed to get current status of container %s:", containerName), err)
		return false
	}

	if status != bo.KContainerStatusRunning {
		return false
	}

	clog.Info(fmt.Sprintf("Checking home directory of user %s", user))
	homeDir, err := GetRemoteUserHomeDirOnContainer("", containerName, user)
	if err != nil {
		return false
	}
	if validHomeDir(homeDir, user) {
		return true
	}
	return false
}

func IsOrchestrationReachableFromContainer(containerName string, user string) bool {
	ex := NewContainerEngine()
	SetEngineName(ex, "")
	ex.SetAction(K_ACTION_EXE)
	ex.SetContainerName(containerName)
	ex.SetExecuteCmd(K_EXEC_CMD_CHECK_ORC_REACHABLE_FROM_DEVCONTAINER)
	ex.SetRemoteUser(user)

	var serviceName string
	var err error
	serviceName, err = ExecuteCommand(ex, shexec.RunLocalCmdWithOutput)
	if err != nil {
		clog.Warn(
			fmt.Sprintf("Failed to get check that orchestration is reachable from devcontainer %s", containerName),
			err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(serviceName), "service/kubernetes")
}

func validHomeDir(homeDir, remoteUser string) bool {
	// TODO BK improve this quick and dirty
	switch remoteUser {
	case KRootUsr:
		return strings.HasPrefix(homeDir, "/root")
	default:
		return strings.HasPrefix(homeDir, "/home")
	}
}
