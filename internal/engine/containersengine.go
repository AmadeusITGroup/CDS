package engine

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/features"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/scm"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

const (
	kCacheDir              = "cache"
	kContainersFlavoursUrl = "https://github.com/Amadeus-xDLC/devenv.cds_containers"
	kConfigSpecUrl         = "https://userguide-cds.cicd.rnd.amadeus.net/specifications/config.html"
	kImageBuildFile        = "Dockerfile"
	kPermFile              = fs.FileMode(0600)
	kPermDir               = fs.FileMode(0700)
	// https://code.visualstudio.com/docs/remote/devcontainerjson-reference#_variables-in-devcontainerjson
	kDevContainerVarRegXp  = `\$\{([A-Za-z_]+):([0-9A-Za-z_]+)\}`
	kDefaultUser           = "root"
	kConfAttregExp         = `\{\.([A-Za-z_]+)\}`
	KRootUsr               = "root"
	KPersistentVolumeMount = "source=/srv/data/home/${localEnv:USER}/workspace/kind-mount,target=/kind-mount,type=bind"
	kDefaultDevboxConfDir  = "${localEnv:HOME}/.devbox"
	kExecutingDebugMsg     = "Executing: %v"
	kPathNotExists         = "Path does not exist"
	kPodmanNetwork         = "podman"
	kNetworkExistsVerb     = "exists"
)

var (
	enVarsMutex           sync.Mutex
	envVars               map[string]string
	devcontainerNameMutex sync.Mutex
	sDevcontainerName     string // if no name is given in configuration the generated name depends on time hence the need of single generation.
)

type ContainersEngine struct {
	name                EngineName_t
	action              Action_t
	args                []string
	flags               EngineFlags_t
	cmds                []shexec.ExecuteEvent // should not be exposed to callers in order to control its state!
	user                containersEngineUser
	containerName       string
	format              Format_t
	execCmd             Execute_t
	fileCopyActionType  Copy_t
	sourceToCopy        string
	copyDestination     string
	pathToCheck         string
	runType             Run_t
	repository          containersScmInfo
	envVars             map[string]string
	destinationFilePerm fs.FileMode
	helmRepository      HelmRepository
	helmChart           HelmChart
	configMapType       string
	helmRelease         string
	customCommand       string
	envVarName          string
	orcNamespace        string
	azureDomainName     string
	networkCmd          Network_t
	networkName         string
	containerLabels     map[string]string
	resourceProvider    resourceProvider
	resolvedFeatures    []features.ResolvedFeature
}

type containersEngineUser struct {
	remoteUsr    string
	containerUsr string
	targetUsr    containerUser
}

type containersScmInfo struct {
	uri    string
	branch string
}

type containerUser struct {
	uid  string
	gid  string
	file string
}

type HelmRepository struct {
	Name string
	Url  string
}

type HelmChart struct {
	Name           string
	RepositoryName string
}

type envName string

const (
	localEnv     envName = "localEnv"
	containerEnv envName = "containerEnv"
)

type evaluableEnvVar struct {
	fullMatch string
	envName   envName
	varName   string
}

// ContainerEngineOption is a functional option for NewContainerEngine.
type ContainerEngineOption func(*ContainersEngine)

// WithResourceProvider sets a custom ResourceProvider for the engine.
func WithResourceProvider(rp resourceProvider) ContainerEngineOption {
	return func(ce *ContainersEngine) {
		ce.resourceProvider = rp
	}
}

func WithResolvedFeatures(rf []features.ResolvedFeature) ContainerEngineOption {
	return func(ce *ContainersEngine) {
		ce.resolvedFeatures = rf
	}
}

func NewContainerEngine(opts ...ContainerEngineOption) *ContainersEngine {
	ce := &ContainersEngine{user: newContainersEngineUser()}
	for _, opt := range opts {
		opt(ce)
	}

	if ce.resourceProvider == nil {
		ce.resourceProvider = newUnimplementedResourceProvider()
	}
	return ce
}

func newContainersEngineUser() containersEngineUser {
	return containersEngineUser{remoteUsr: ResolveRemoteUserFromConf(), containerUsr: ResolveContainerUser()}
}

func (ce *ContainersEngine) BuildCommands() ([]shexec.ExecuteEvent, error) {
	clog.Debug("ENGINE - GENERATE COMMANDS ...")
	var err error
	switch ce.action {
	case K_ACTION_CP:
		err = ce.cp()
	case K_ACTION_EXE:
		err = ce.execute()
	case K_ACTION_INSPECT:
		err = ce.inspect()
	case K_ACTION_PS:
		err = ce.ps()
	case K_ACTION_RUN:
		err = ce.run()
	case K_ACTION_START:
		err = ce.start()
	case K_ACTION_STOP:
		err = ce.stop()
	case K_ACTION_REMOVE:
		err = ce.remove()
	case K_ACTION_SYSTEM:
		err = ce.system()
	case K_ACTION_RENAME:
		err = ce.rename()
	case K_ACTION_NETWORK:
		err = ce.network()
	case K_ACTION_BUILD:
		err = ce.build()
	case K_ACTION_NaN:
		ce.cmds = []shexec.ExecuteEvent{}
	default:
		panic(fmt.Sprintf("The specified action (%v) is not implemented", ce.action))
	}

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Error while building commands for %v", ce.action), err)
	}
	return ce.cmds, nil
}

func (ce *ContainersEngine) ContainerName() string {
	return ce.getContainerName()
}

func (ce *ContainersEngine) RemoteUser() string {
	return ce.getRemoteUser()
}

func (ce *ContainersEngine) SetContainerName(name string) {
	ce.containerName = name
}

func (ce *ContainersEngine) SetFlag(key, value string) {
	if ce.flags == nil {
		ce.flags = make(EngineFlags_t)
	}
	if len(ce.flags[key]) == 0 {
		ce.flags[key] = []string{}
	}
	ce.flags[key] = append(ce.flags[key], value)
}

func (ce *ContainersEngine) SetName(name EngineName_t) {
	ce.name = name
}

func (ce *ContainersEngine) SetAction(action Action_t) {
	ce.action = action
}

func (ce *ContainersEngine) SetFormat(format Format_t) {
	ce.format = format
}

func (ce *ContainersEngine) PrintedFormat() Format_t {
	return ce.format
}

func (ce *ContainersEngine) SetExecuteCmd(xCmd Execute_t) {
	ce.execCmd = xCmd
}

func (ce *ContainersEngine) SetCopyActionType(fCp Copy_t) {
	ce.fileCopyActionType = fCp
}

func (ce *ContainersEngine) SetSourceCopyPath(src string) {
	ce.sourceToCopy = src
}

func (ce *ContainersEngine) getSourceCopyPath() string {
	return ce.sourceToCopy
}

func (ce *ContainersEngine) SetCopyFinalDestinationOnContainer(dst string) {
	ce.copyDestination = dst
}

func (ce *ContainersEngine) getCopyFinalDestinationOnContainer() string {
	return ce.copyDestination
}

func (ce *ContainersEngine) SetPathToCheck(path string) {
	ce.pathToCheck = path
}

func (ce *ContainersEngine) getPathToCheck() string {
	return ce.pathToCheck
}

func (ce *ContainersEngine) SetRunType(rType Run_t) {
	ce.runType = rType
}

func (ce *ContainersEngine) SetRemoteUser(rUser string) {
	ce.user.remoteUsr = rUser
}

func (ce *ContainersEngine) SetTargetUserOnContainer(userId, groupId string) {
	ce.user.targetUsr = containerUser{uid: userId, gid: groupId}
}

func (ce *ContainersEngine) getTargetUserOnContainer() containerUser {
	return ce.user.targetUsr
}

func (ce *ContainersEngine) SetTargetUserFileOnContainer(file string) {
	ce.user.targetUsr.file = file
}

func (ce *ContainersEngine) AddArg(args ...string) {
	if len(ce.args) == 0 {
		ce.args = []string{}
	}
	ce.args = append(ce.args, args...)
}

func (ce *ContainersEngine) SetSourceRepoInfo(uri, branch string) {
	ce.repository = containersScmInfo{uri: uri, branch: branch}
}

func (ce *ContainersEngine) SetEnvVariables(vars map[string]string) {
	ce.envVars = vars
}

func (ce *ContainersEngine) getEnvVariables() map[string]string {
	return ce.envVars
}

func (ce *ContainersEngine) getDestinationFilePerm() fs.FileMode {
	return ce.destinationFilePerm
}

func (ce *ContainersEngine) SetDestinationFilePerm(perm fs.FileMode) {
	ce.destinationFilePerm = perm
}

func (ce *ContainersEngine) SetHelmChart(helmChart HelmChart) {
	ce.helmChart = helmChart
}

func (ce *ContainersEngine) getHelmChart() HelmChart {
	return ce.helmChart
}

func (ce *ContainersEngine) SetHelmRepository(helmRepo HelmRepository) {
	ce.helmRepository = helmRepo
}

func (ce *ContainersEngine) getHelmRepository() HelmRepository {
	return ce.helmRepository
}

func (ce *ContainersEngine) getConfigMapType() string {
	return ce.configMapType
}

func (ce *ContainersEngine) SetConfigMapType(configMapType string) {
	ce.configMapType = configMapType
}

func (ce *ContainersEngine) SetHelmRelease(helmRelease string) {
	ce.helmRelease = helmRelease
}

func (ce *ContainersEngine) getReleaseName() string {
	return ce.helmRelease
}

func (ce *ContainersEngine) getCustomCommand() string {
	return ce.customCommand
}

func (ce *ContainersEngine) SetCustomCommand(customCommand string) {
	ce.customCommand = customCommand
}

func (ce *ContainersEngine) SetEnvVarName(varName string) {
	ce.envVarName = varName
}

func (ce *ContainersEngine) getEnvVarName() string {
	return ce.envVarName
}

func (ce *ContainersEngine) SetOrcNamespace(ns string) {
	ce.orcNamespace = ns
}

func (ce *ContainersEngine) SetAzureDomainName(domainName string) {
	ce.azureDomainName = domainName
}

func (ce *ContainersEngine) getAzureDomainName() string {
	return ce.azureDomainName
}

func (ce *ContainersEngine) getOrcNamespace() string {
	if ce.orcNamespace == "" {
		ce.orcNamespace = cg.KOrchestrationDefaultNamespace
	}
	return ce.orcNamespace
}

func (ce *ContainersEngine) AddContainerLabels(labels map[string]string) {
	if ce.containerLabels == nil {
		ce.containerLabels = make(map[string]string)
	}
	for k, v := range labels {
		ce.containerLabels[k] = v
	}
}

func (ce *ContainersEngine) ContainerLabels() map[string]string {
	return ce.containerLabels
}

func (ce *ContainersEngine) cp() error {
	var err error
	var rEvent *RunEvent
	switch ce.fileCopyActionType {
	case K_CP_DEFAULT:
		rEvent, err = ce.copyHandler()
	default:
		return cerr.NewError("Unhandled copy command for container engine")
	}

	if err != nil {
		return err
	}

	ce.cmds = append(ce.cmds, rEvent)

	return nil
}

func (ce *ContainersEngine) execute() error {
	var rEvent *RunEvent
	var err error
	switch ce.execCmd {
	case K_EXEC_CMD_SSH:
		rEvent, err = ce.sshKeyHandler()
	case K_EXEC_CMD_HOMEDIR:
		rEvent, err = ce.homeDirHandler()
	case K_EXEC_CMD_CLONE_SRC_REPO:
		rEvent, err = ce.cloneHandler()
	case K_EXEC_CMD_ID:
		rEvent, err = ce.idHandler()
	case K_EXEC_CMD_CHOWN:
		rEvent, err = ce.chownHandler()
	case K_EXEC_CMD_MKDIR:
		rEvent, err = ce.mkdirHanlder()
	case K_EXEC_CMD_GET_KIND_KUBECONF:
		rEvent, err = ce.getKindKubeConfHandler()
	case K_EXEC_CMD_GET_KIND_CLUSTER_STATUS:
		rEvent, err = ce.getKindClusterStatusHandler()
	case K_EXEC_CMD_RSH:
		rEvent, err = ce.attachContainerHandler()
	case K_EXEC_CMD_GIT_CONFIG:
		rEvent, err = ce.gitConfigHandler()
	case K_EXEC_CMD_SECURE_REGISTRY:
		rEvent, err = ce.kubeRegistrySecretHandler()
	case K_EXEC_CMD_CHECK_PATH_EXISTS:
		rEvent, err = ce.checkPathExistsHandler()
	case K_EXEC_CMD_CHECK_SECRET_EXISTS:
		rEvent, err = ce.checkSecretExistsHandler()
	case K_EXEC_CMD_CREATE_SECRET:
		rEvent, err = ce.createSecretHandler()
	case K_EXEC_CMD_CHECK_SERVICE_ACCOUNT_EXISTS:
		rEvent, err = ce.checkServiceAccountExistsHandler()
	case K_EXEC_CMD_PATCH_SERVICE_ACCOUNT:
		rEvent, err = ce.patchServiceAccountHandler()
	case K_EXEC_CMD_ENV:
		rEvent, err = ce.envVariablesHandler()
	case K_EXEC_CMD_APPLY_INGRESS:
		rEvent, err = ce.applyIngressHandler()
	case K_EXEC_CMD_CHMOD_DEST_FILE:
		rEvent, err = ce.chmodHandler()
	case K_EXEC_CMD_CHECK_ORC_REACHABLE_FROM_DEVCONTAINER:
		rEvent, err = ce.chekOrchestrationIsReachable()
	case K_EXEC_HELM_ADD_REPO:
		rEvent, err = ce.addHelmRepo()
	case K_EXEC_HELM_INSTALL_CHART:
		rEvent, err = ce.installHelmChart()
	case K_EXEC_HELM_UNINSTALL_CHART:
		rEvent, err = ce.uninstallHelmChart()
	case K_EXEC_ORC_GET_CONFIG_MAP:
		rEvent, err = ce.getConfigMap()
	case K_EXEC_HELM_CHECK_DEPLOYED_RELEASE:
		rEvent, err = ce.checkHelmReleaseDeployed()
	case K_EXEC_ORC_GET_NAMESPACES:
		rEvent, err = ce.getAllNamespaces()
	case K_EXEC_ORC_CREATE_NAMESPACE:
		rEvent, err = ce.createOrcNamespace()
	case K_EXEC_CUSTOM_CMD:
		rEvent, err = ce.handleCustomCommand()
	case K_EXEC_CMD_GET_ENV_VARIABLE:
		rEvent, err = ce.getEnvVariableValue()
	case K_EXEC_CMD_WAIT_FOR_ORC_NODE:
		rEvent, err = ce.waitForOrcNode()
	case K_EXEC_CMD_WAIT_FOR_INGRESS:
		rEvent, err = ce.waitForIngressController()
	case K_EXEC_CMD_CHECK_INGRESS_CONTROLLER_STATUS:
		rEvent, err = ce.checkIngressControllerStatusHandler()
	case K_EXEC_CMD_BYPASS_PROXY_IN_AZURE:
		rEvent, err = ce.bypassProxyInAzure()
	case K_EXEC_SHARE_SSH:
		rEvent, err = ce.addTempSharedKey()
	case K_EXEC_UNSHARE_SSH:
		rEvent, err = ce.removeSharedKeys()

	default:
		return cerr.NewError(fmt.Sprintf("Failed to execute container engine command, action '%v' is not implemented", ce.execCmd))
	}
	if err != nil {
		return err
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) sshKeyHandler() (*RunEvent, error) {
	pubKey, err := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFile, containerconf.KindPubKey)

	if err != nil {
		return nil, cerr.AppendError("Failed reading public key to configure ssh", err)
	}
	rEvent := executeMap[K_EXEC_CMD_SSH]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getRemoteUser(), strings.Trim(string(pubKey), " \n")),
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) addTempSharedKey() (*RunEvent, error) {
	pubKey, err := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFile, containerconf.KindSharedKey)

	if err != nil {
		clog.Warn("Failed reading temp shared public key", err)
		return nil, cerr.AppendError("Failed reading temp shared public key to configure ssh", err)
	}

	rEvent := executeMap[K_EXEC_SHARE_SSH]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		engineAction(K_ACTION_EXE),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, strings.Trim(string(pubKey), " \n"), ce.getRemoteUser()),
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

// TODO fix the command
func (ce *ContainersEngine) removeSharedKeys() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_UNSHARE_SSH]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		engineAction(K_ACTION_EXE),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, "TODO", ce.getRemoteUser(), ce.getRemoteUser(), ce.getRemoteUser(), ce.getRemoteUser()),
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) homeDirHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_HOMEDIR]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) cloneHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CLONE_SRC_REPO]

	var branchOption string
	if len(ce.repository.branch) != 0 {
		branchOption = fmt.Sprintf("--branch %v", ce.repository.branch)
	}

	repo, _ := scm.ParseGitRepositoryUrl(ce.repository.uri)
	repoName := strings.Trim(string(repo.Name()), " \n")
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, repoName, repoName, strings.Trim(string(ce.repository.uri), " \n"), branchOption),
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

// Deprecated: Should not be used anymore, git config is now a file that is given entirely by the client and should not be generated by commands
func (ce *ContainersEngine) gitConfigHandler() (*RunEvent, error) {
	return nil, nil
}

func (ce *ContainersEngine) kubeRegistrySecretHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_SECURE_REGISTRY]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) checkPathExistsHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHECK_PATH_EXISTS]
	pathToCheck := ce.getPathToCheck()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, pathToCheck, kPathNotExists),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) checkSecretExistsHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHECK_SECRET_EXISTS]
	return ce.executeRaw(&rEvent)
}

func (ce *ContainersEngine) createSecretHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CREATE_SECRET]
	return ce.executeRaw(&rEvent)
}

func (ce *ContainersEngine) checkServiceAccountExistsHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHECK_SERVICE_ACCOUNT_EXISTS]
	return ce.executeRaw(&rEvent)
}

func (ce *ContainersEngine) patchServiceAccountHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_PATCH_SERVICE_ACCOUNT]
	return ce.executeRaw(&rEvent)
}

func (ce *ContainersEngine) checkIngressControllerStatusHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHECK_INGRESS_CONTROLLER_STATUS]
	return ce.executeRaw(&rEvent)
}

func (ce *ContainersEngine) executeRaw(rEvent *RunEvent) (*RunEvent, error) {
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return rEvent, nil
}

func (ce *ContainersEngine) envVariablesHandler() (*RunEvent, error) {
	if ce.getRemoteUser() != KRootUsr {
		clog.Warn("Can't use env variable handler. Currently, it supports only root updates.")
		return &RunEvent{}, nil
	}

	rEvent := executeMap[K_EXEC_CMD_ENV]
	var exportVariablesStr string
	envVars := ce.getEnvVariables()
	for key, val := range envVars {
		entry := fmt.Sprintf(`echo "export %s=%s"`, key, val)
		exportVariablesStr += fmt.Sprintf(`%s >> /etc/profile && %s >> /etc/zshenv;`, entry, entry)
	}
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, exportVariablesStr),
	)

	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) idHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_ID]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) chownHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHOWN]
	targetUsr := ce.getTargetUserOnContainer()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, targetUsr.uid, targetUsr.gid, targetUsr.file),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) chmodHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHMOD_DEST_FILE]
	targetUsr := ce.getTargetUserOnContainer()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getDestinationFilePerm(), targetUsr.file))

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) mkdirHanlder() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_MKDIR]
	targetUsr := ce.getTargetUserOnContainer()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, targetUsr.file),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) copyHandler() (*RunEvent, error) {
	srcFilePath := ce.getSourceCopyPath()
	cpCmd := fmt.Sprintf(
		`%v %v %v %v`,
		engineName(ce.name),
		engineAction(K_ACTION_CP),
		srcFilePath,
		fmt.Sprintf(`%v:%v`, ce.getContainerName(), strings.Trim(ce.getCopyFinalDestinationOnContainer(), " \n")),
	)
	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cpCmd))

	// output
	rEvent := &RunEvent{
		cmd: cpCmd,

		continueProcess: true,
		eventInfo:       fmt.Sprintf("Copy %s file to container", srcFilePath),
	}
	return rEvent, nil
}

func (ce *ContainersEngine) getKindKubeConfHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_GET_KIND_KUBECONF]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) getKindClusterStatusHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_GET_KIND_CLUSTER_STATUS]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) attachContainerHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_RSH]
	cmd := fmt.Sprintf(
		`%v %v -u %v -it %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) applyIngressHandler() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_APPLY_INGRESS]
	cmd := fmt.Sprintf(
		`%v %v -u %v -it %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	clog.Debug(fmt.Sprintf(kExecutingDebugMsg, cmd))

	rEvent.cmd = cmd
	rEvent.continueProcess = true

	return &rEvent, nil
}

func (ce *ContainersEngine) addHelmRepo() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_HELM_ADD_REPO]
	helmRepo := ce.getHelmRepository()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, helmRepo.Name, helmRepo.Url),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) installHelmChart() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_HELM_INSTALL_CHART]
	helmChart := ce.getHelmChart()
	helmNs := ce.getOrcNamespace()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, helmChart.Name, helmChart.RepositoryName, helmChart.Name, helmNs, helmNs),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) uninstallHelmChart() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_HELM_UNINSTALL_CHART]
	helmChart := ce.getHelmChart()
	helmNs := ce.getOrcNamespace()

	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, helmChart.Name, helmNs),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) getConfigMap() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_ORC_GET_CONFIG_MAP]
	helmNs := ce.getOrcNamespace()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getConfigMapType(), helmNs),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) checkHelmReleaseDeployed() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_HELM_CHECK_DEPLOYED_RELEASE]
	helmNs := ce.getOrcNamespace()
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getReleaseName(), helmNs),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) getAllNamespaces() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_ORC_GET_NAMESPACES]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) chekOrchestrationIsReachable() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_CHECK_ORC_REACHABLE_FROM_DEVCONTAINER]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) createOrcNamespace() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_ORC_CREATE_NAMESPACE]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getOrcNamespace()),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) handleCustomCommand() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CUSTOM_CMD]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getCustomCommand()),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) getEnvVariableValue() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_GET_ENV_VARIABLE]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getEnvVarName()),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil

}

func (ce *ContainersEngine) waitForIngressController() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_WAIT_FOR_INGRESS]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) bypassProxyInAzure() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_BYPASS_PROXY_IN_AZURE]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		fmt.Sprintf(rEvent.cmd, ce.getAzureDomainName()),
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) waitForOrcNode() (*RunEvent, error) {
	rEvent := executeMap[K_EXEC_CMD_WAIT_FOR_ORC_NODE]
	cmd := fmt.Sprintf(
		`%v %v -u %v -i %v %v`,
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		ce.getRemoteUser(),
		ce.getContainerName(),
		rEvent.cmd,
	)

	rEvent.cmd = cmd
	rEvent.continueProcess = true
	return &rEvent, nil
}

func (ce *ContainersEngine) inspect() error {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(K_ACTION_INSPECT),
		ce.getContainerName(),
		preComputedFormat(ce.format),
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       fmt.Sprintf("Inspecting container '%s'", ce.getContainerName()),
		continueProcess: true,
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) ps() error {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		"-a",
		preComputedFormat(ce.format),
	)
	rEvent := &RunEvent{
		cmd:             cmd,
		eventInfo:       "Listing containers",
		continueProcess: true,
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) genericRun() error {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		cg.VariadicJoin(" ", ce.args...),
	)
	rEvent := &RunEvent{
		cmd:             cmd,
		eventInfo:       "Creating container",
		continueProcess: true,
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) run() error {
	if ce.runType == K_RUN_GEN {
		if err := ce.genericRun(); err != nil {
			return cerr.AppendError("Failed to execute custom run command", err)
		}
		return nil
	}

	if err := ce.preServer(); err != nil {
		return cerr.AppendError("Failed to run pre server step of container run", err)
	}
	if err := ce.preContainerRun(); err != nil {
		return cerr.AppendError("Failed to run pre container step of container run", err)
	}
	if err := ce.containerRun(); err != nil {
		return cerr.AppendError("Failed to run podman run step of container run", err)
	}
	if err := ce.postContainerRun(); err != nil {
		return cerr.AppendError("Failed to run post container step of container run", err)
	}
	if err := ce.featuresRun(); err != nil {
		return cerr.AppendError("Failed to run features step of container run", err)
	}
	return nil
}

func (ce *ContainersEngine) genericContainerAction() error {
	rEvent, exist := actionMap[ce.action]
	if !exist {
		return cerr.AppendError("Failed to find action in actionMap", fmt.Errorf("seems like action %v does not have a mapped action", ce.action))
	}

	rEvent.cmd = cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		cg.VariadicJoin(" ", ce.args...),
	)

	// TODO:FixMe: find solution for the assumption that args are container names
	rEvent.eventInfo += cg.VariadicJoin(" ", ce.args...)

	ce.cmds = append(ce.cmds, &rEvent)
	return nil
}

func (ce *ContainersEngine) start() error {
	return ce.genericContainerAction()
}

func (ce *ContainersEngine) stop() error {
	return ce.genericContainerAction()
}

func (ce *ContainersEngine) remove() error {
	return ce.genericContainerAction()
}

func (ce *ContainersEngine) system() error {
	return ce.genericContainerAction()
}

func (ce *ContainersEngine) rename() error {
	ce.getRemoteUser()
	return ce.genericContainerAction()
}

func (ce *ContainersEngine) build() error {
	return ce.genericContainerAction()
}

// handles:
// 1 - .dot dir
// 2- features with a bootstrap action to execute
//
//	(Ex: podman sibling)
func (ce *ContainersEngine) preServer() error {
	if err := ce.addMandatoryConfigurationFiles(); err != nil {
		return cerr.AppendError("Failed to copy configuration files", err)
	}

	for _, f := range ce.resolvedFeatures {
		for _, relPath := range f.OnHost.Files {
			featureFileIdentifier := features.FeatureFileIdentifier(f.Name, f.Version, relPath)
			content, fetchErr := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFeature, featureFileIdentifier)
			if fetchErr != nil {
				clog.Warn(fmt.Sprintf("Failed to fetch feature file '%v': %v\n", relPath, fetchErr))
				continue
			}
			processedContent := ce.formatContent(content, relPath)
			tmpPath, tmpErr := cenv.CreateTempFileWithContent(bytes.NewReader(processedContent))
			if tmpErr != nil {
				clog.Warn(fmt.Sprintf("Failed to create temp file for '%v': %v\n", relPath, tmpErr))
				continue
			}
			if err := ce.executeFileOnHost(tmpPath); err != nil {
				clog.Warn(fmt.Sprintf("Failed to append file '%v': %v\n", relPath, err))
			}
		}
	}
	return nil
}

func (ce *ContainersEngine) addMandatoryConfigurationFiles() error {
	// ~/.config/containers/auth.json
	authFileData, err := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFile, containerconf.KindAuthFile)
	if err != nil {
		return cerr.AppendError("Failed to read auth file data to copy mandatory container configuration file", err)
	}

	authFilePath := filepath.Join(cenv.GetUserHomeDir(), ".config", "containers", cg.KContainerAuthFileName)
	if err := cos.WriteFile(authFilePath, authFileData, kPermFile); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to write auth file at '%s' to copy mandatory container configuration file", authFilePath), err)
	}

	return nil
}

// handles:
// 1 - initializeCommand from devcontainer.json
// Setup a default devbox configuration directory at ~/.devbox
// If made official ~/workspace should mimick this behavior which would need a proper implementation (make a function that make this process expand -> mkdir)
func (ce *ContainersEngine) preContainerRun() error {
	defaultDevboxConfDir, err := ce.expand(kDefaultDevboxConfDir)
	if err != nil {
		clog.Warn("preContainerRun", cerr.AppendError("Failed to expand default devbox configuration directory", err))
	} else {
		defaultDevboxConfDirPath := path.Clean(defaultDevboxConfDir)
		mkdirCmd := fmt.Sprintf("mkdir -p %v", defaultDevboxConfDirPath)
		rEvent := &RunEvent{
			cmd:       mkdirCmd,
			eventInfo: "Creating default devbox configuration directory",
		}
		ce.cmds = append(ce.cmds, rEvent)
	}
	if !containerconf.IsSet(containerconf.KInitializeCommand) {
		return nil
	}
	ic, err := ce.expand(containerconf.Get(containerconf.KInitializeCommand))
	if err != nil {
		return cerr.AppendError("Failed to resolve initializeCommand", err)
	}

	if len(ic) > 0 {
		icCmd := fmt.Sprintf(
			"/bin/sh -c '%v' ",
			ic,
		)

		rEvent := &RunEvent{
			cmd:       icCmd,
			eventInfo: "Running devcontainer initializeCommand",
		}
		ce.cmds = append(ce.cmds, rEvent)
	}

	return nil
}

func buildDockerfile(path, name string) error {
	buildArgs := []string{"-t", name, "-f", path}
	nce := NewContainerEngine()
	nce.SetAction(K_ACTION_BUILD)
	nce.AddArg(buildArgs...)
	_, err := ExecuteCommand(nce, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to build dockerfile image %s at %s", name, path), err)
	}
	return nil
}

// 2 possible ways to to configure the devcontainer image:
//
// 1 - explicit image url in configuration that only needs, at best, to be expanded
//
// 2 - build the image from a dockerfile, which will be copied and built on the host before run
//
// returns the prepared image url
func (ce *ContainersEngine) prepareImage() (string, error) {
	if image := containerconf.Get(containerconf.KImage); image != nil {
		return ce.expand(image)
	}

	if containerconf.IsSet(containerconf.KBuild, containerconf.KBuildDockerfile) {
		// Prepare paths
		dockerFileData, err := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFile, containerconf.KindDockerfile)
		if err != nil {
			return "", cerr.AppendError("Failed to read dockerfile data to prepare image", err)
		}
		dockerFilePath, err := cenv.CreateTempFileWithContent(bytes.NewReader(dockerFileData))
		if err != nil {
			return "", cerr.AppendError("Failed to create temporary dockerfile", err)
		}
		// Image names need to be lowercase
		imageName := strings.ToLower(ce.getContainerName())
		if err := buildDockerfile(dockerFilePath, imageName); err != nil {
			return "", cerr.AppendError("Failed to build dockerfile", err)
		}
		// prepend localhost to image name to use same format as explicit image
		imageUrl := fmt.Sprintf("%s/%s", cg.KLocalhost, imageName)
		return imageUrl, nil
	}

	return "", cerr.NewError("No image specified")
}

// handles:
// 1 - main podman run command
func (ce *ContainersEngine) containerRun() error {
	options := []string{}
	// Mandatory
	image, err := ce.prepareImage()
	parsedImage := cg.ParseImageString(image)
	if err != nil {
		return cerr.AppendError("Failed to expand image name from configuration", err)
	}

	if overrideImageTag, ok := containerconf.Get(containerconf.KOverrideImageTag).(string); ok && overrideImageTag != "" {
		clog.Warn(fmt.Sprintf("Overriding image tag with '%s' for image '%s'", overrideImageTag, parsedImage.ToString()))
		parsedImage.OverrideTag(overrideImageTag)
	}

	options = append(options, fmt.Sprintf("--name %v", ce.getContainerName()))

	options = append(options, fmt.Sprintf("-u %v", ce.getContainerUser()))

	options = append(options, fmt.Sprintf("--network=%v", kPodmanNetwork))

	if ce.ContainerLabels() != nil {
		for labelKey, labelValue := range ce.ContainerLabels() {
			options = append(options, fmt.Sprintf("--label %s=%s", labelKey, labelValue))
		}
	}

	// best effort
	if runArgs, ok := containerconf.Get(containerconf.KRunArgs).([]interface{}); ok {
		for _, arg := range runArgs {
			arg := arg.(string)
			if err := validateRunArg(arg); err != nil {
				clog.Warn(fmt.Sprintf("run argument '%s' skipped:\n", arg), err)
				continue
			}
			options = append(options, arg)
		}
	}

	if appPorts, ok := containerconf.Get(containerconf.KAppPort).([]interface{}); ok {
		for _, port := range appPorts {
			options = append(options, fmt.Sprintf("-p %s", port))
		}
	}

	mountsOptions, errMountOptions := ce.getMountOptions()
	if errMountOptions != nil {
		clog.Warn("[ContainerRun]", cerr.AppendError("Mount options skipped", errMountOptions))
	}
	options = append(options, mountsOptions...)

	// check if nas option has been given in cli arg or requested through .cds/profile.json
	// With rework this should come from the client preparing properly the configuration
	if containerconf.IsNasRequested() {
		// TODO:FixMe:
		// "-v /remote/dir1:/dst:ro [...] -v /remote:/remote:ro", if dst != "/remote",
		// no issues whatsoever but if dst is mounted in remote, you have this behavior:
		// "-v /remote:/remote/tmp [...] -v /remote:/remote" works, /remote/tmp in container is the same as /remote host
		// "-v /remote:/remote/newdir [...] -v /remote:/remote" fails - cannot create newdir in /remote
		options = append(options, "-v /remote:/remote:ro")
	}

	options = append(options, ce.args...)

	// <engine-cli-bin> <engine-cli-subcommand> [options] IMAGE [COMMAND [ARG...]]
	eCmd := cg.VariadicJoin(" ", engineName(ce.name), engineAction(ce.action))
	eOptions := cg.VariadicJoin(" ", options...)
	cCmd := `/bin/sh -c 'while sleep 1000; do :; done'`
	xCmd := cg.VariadicJoin(" ", eCmd, eOptions, parsedImage.ToString(), cCmd)
	rEvent := &RunEvent{
		cmd:       xCmd,
		eventInfo: fmt.Sprintf("Creating container '%s'", ce.getContainerName()),
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) getMountOptions() ([]string, error) {
	mounts, errMount := ce.mounts()

	if errMount != nil {
		return nil, cerr.AppendError("Failed to retrieve mounts from configuration and/or orchestration", errMount)
	}
	resultMounts := []string{}
	for _, mount := range mounts {
		expandedMount, _ := ce.expand(mount)
		srcDst := strings.Split(expandedMount, ",")
		if len(srcDst) < 2 {
			clog.Warn(fmt.Sprintf("Invalid mount configuration, '%v' is not a valid %v value\n", expandedMount, containerconf.KMounts))
			continue
		}
		src := strings.Split(srcDst[0], "=")
		dst := strings.Split(srcDst[1], "=")
		if len(src) != 2 {
			clog.Warn(fmt.Sprintf("Invalid mount configuration, '%v' is not a valid source value\n", srcDst[0]))
			continue
		}
		if len(dst) != 2 {
			clog.Warn(fmt.Sprintf("Invalid mount configuration, '%v' is not a valid destination value", srcDst[1]))
			continue
		}
		if len(src[1]) == 0 || len(dst[1]) == 0 {
			clog.Warn(fmt.Sprintf("Invalid mount configuration, '%v' was evaluated to '%s' which contains an empty value", mount, expandedMount))
			continue
		}
		mountArg := fmt.Sprintf("-v %v:%v", src[1], dst[1])
		if slices.Contains(resultMounts, mountArg) {
			clog.Warn(fmt.Sprintf("Invalid mount configuration, '%v' is bound twice! Skipping", expandedMount))
			continue
		}
		resultMounts = append(resultMounts, mountArg)
	}
	return resultMounts, nil
}

func (ce *ContainersEngine) mounts() ([]interface{}, error) {
	mounts := defaultMounts()
	mountsFromConfig, err := fromConfig()
	if err != nil {
		return nil, err
	}
	mounts = append(mounts, mountsFromConfig...)
	mountsFromOrchestration := fromOrchestration()
	if mountsFromOrchestration != nil {
		mounts = append(mounts, mountsFromOrchestration...)
	}
	return mounts, nil
}

func defaultMounts() []interface{} {
	return []interface{}{
		fmt.Sprintf("source=%v,target=/devbox,type=bind", kDefaultDevboxConfDir),
	}
}

func fromConfig() ([]interface{}, error) {
	mountKeyValue := containerconf.Get(containerconf.KMounts)
	if mountKeyValue == nil {
		return []interface{}{}, nil
	}

	mountsFromConfig, ok := mountKeyValue.([]interface{})
	if !ok {
		return nil, cerr.NewError("Failed to retrieve mounts from config")
	}
	return mountsFromConfig, nil
}

func fromOrchestration() []interface{} {
	if persistent, ok := containerconf.Get(containerconf.KOrchestration, containerconf.KPersistentVolumeClaim).(bool); ok && persistent {
		clog.Debug("Adding peristent volume to mounts list...")
		return []interface{}{KPersistentVolumeMount}
	}
	return nil
}

// handles:
// 1 - postCreateCommand from devcontainer.json
func (ce *ContainersEngine) postContainerRun() error {
	if !containerconf.IsSet(containerconf.KPostCreateCommand) {
		return nil
	}

	pcc, err := ce.expand(containerconf.Get(containerconf.KPostCreateCommand))
	if err != nil {
		return cerr.AppendError("Failed to resolve postCreateCommand", err)
	}

	if len(pcc) > 0 {
		pccCmd := fmt.Sprintf(
			"%v %v -u %v -i %v /bin/sh -c '%v' ",
			engineName(ce.name),
			formatActionName(engineAction(K_ACTION_EXE)),
			ce.getContainerUser(),
			ce.getContainerName(),
			pcc,
		)
		rEvent := &RunEvent{
			cmd: pccCmd,

			continueProcess: true,
			eventInfo:       "Running devcontainer postCreateCommand",
		}
		ce.cmds = append(ce.cmds, rEvent)
	}

	return nil
}

// handles:
// 1 - features from devcontainer.json
func (ce *ContainersEngine) featuresRun() error {
	if len(ce.resolvedFeatures) == 0 {
		return nil
	}

	featuresExecutionResults := cg.Map(ce.resolvedFeatures, func(f features.ResolvedFeature) error {
		return ce.executeFeatureOnContainer(f)
	})

	allFeatureExecOnContainerErrs := cg.FilterNilFromSlice(featuresExecutionResults)
	if len(allFeatureExecOnContainerErrs) > 0 {
		return cerr.AppendMultipleErrors("Failed to execute features on container", allFeatureExecOnContainerErrs)
	}
	return nil
}

func (ce *ContainersEngine) executeFeatureOnContainer(f features.ResolvedFeature) error {
	clog.Info(fmt.Sprintf("Adding feature %s", f.Name))

	executionErrs := []error{}
	for _, relPath := range f.OnContainer.Files {
		featureFileIdentifier := features.FeatureFileIdentifier(f.Name, f.Version, relPath)
		content, fetchErr := ce.resourceProvider.FetchFile(containerconf.ResourceTypeFeature, featureFileIdentifier)
		if fetchErr != nil {
			executionErrs = append(executionErrs, cerr.AppendErrorFmt("Failed to fetch file %q", fetchErr, relPath))
			continue
		}
		processedContent := ce.formatContent(content, relPath)
		tmpPath, tmpErr := cenv.CreateTempFileWithContent(bytes.NewReader(processedContent))
		if tmpErr != nil {
			executionErrs = append(executionErrs, cerr.AppendErrorFmt("Failed to create temporary file %q", tmpErr, relPath))
			continue
		}
		if err := ce.executeScriptOnContainerFromHost(tmpPath, f.OnContainer.As); err != nil {
			executionErrs = append(executionErrs, cerr.AppendErrorFmt("Failed to execute file %q", err, relPath))
		}
	}

	if len(executionErrs) > 0 {
		return cerr.AppendMultipleErrors("Failed to execute feature", executionErrs)
	}
	return nil
}

func (ce *ContainersEngine) expand(a interface{}) (string, error) {
	return expand(a)
}

func (ce *ContainersEngine) executeFileOnHost(fileLocationForExec string) error {
	cmd := fmt.Sprintf("bash %v; exit $rc", fileLocationForExec)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       "Running feature's onHost",
		continueProcess: true,
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) executeScriptOnContainerFromHost(pathOnHost, user string) error {
	var execCmdUsr string
	switch user {
	case containerconf.KRemoteUser:
		execCmdUsr = ce.getRemoteUser()
	case containerconf.KContainerUser:
		execCmdUsr = ce.getContainerUser()
	case KRootUsr:
		execCmdUsr = KRootUsr
	}

	cmd := fmt.Sprintf(
		"%v %v -u %v -i %v /bin/sh -l < %v",
		engineName(ce.name),
		formatActionName(engineAction(K_ACTION_EXE)),
		execCmdUsr,
		ce.getContainerName(),
		pathOnHost,
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       "Running feature's onContainer",
		continueProcess: true,
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

// formatContent processes template content in memory. Replacing older formatFile because `engine` package shouldn't deal with filesystem.
// If the filename ends with .tmpl, it replaces {.attributeName} patterns
// with resolved values from configuration/profile.
// Returns the processed content bytes.
func (ce *ContainersEngine) formatContent(content []byte, filename string) []byte {
	if !strings.HasSuffix(filename, ".tmpl") {
		return content
	}

	dataStr := string(content)
	re := regexp.MustCompile(kConfAttregExp)
	matches := re.FindAllString(dataStr, -1)
	for _, match := range matches {
		attributeValue := ce.resolveAttributeValue(match)
		dataStr = strings.ReplaceAll(dataStr, match, attributeValue)
	}
	return []byte(dataStr)
}

func (ce *ContainersEngine) getConfAttributeValue(s string) string {
	s = trimAttribute(s)

	attributeConfig := containerconf.Get(s)
	if attributeConfig == nil {
		clog.Debug(fmt.Sprintf("Could not find attribute in the configuration: '%s'", s))
		return ""
	}
	rVal, err := ce.expand(attributeConfig)
	if err != nil {
		clog.Warn(fmt.Sprintf("Failed to get value of attribute '%s':", s), err)
		return ""
	}
	return rVal
}

func (ce *ContainersEngine) getProfileAttributeValue(att string) string {
	att = trimAttribute(att)
	defaultShell, ok := containerconf.Get(containerconf.KCds, containerconf.KCdsDefaultShell).(string)
	var profileDefaultShell string
	if ok {
		profileDefaultShell = defaultShell
	}

	attribute := engineAttribute(att)
	switch attribute {
	case k_ATTR_DEFAULT_SHELL:
		return profileDefaultShell
	default:
		clog.Debug(fmt.Sprintf("Attribute '%s' is not supported in profile. Using configuration value", att))
		return ""
	}
}

func trimAttribute(att string) string {
	re := regexp.MustCompile(kConfAttregExp)
	substitution := "$1"
	return re.ReplaceAllString(att, substitution)
}

func (ce *ContainersEngine) resolveAttributeValue(s string) string {
	if attributeValue := ce.getProfileAttributeValue(s); attributeValue != "" {
		clog.Debug(fmt.Sprintf("Overriding '%s' with the value from profile: %s", s, attributeValue))
		return attributeValue
	}
	return ce.getConfAttributeValue(s)
}

func (ce *ContainersEngine) getContainerName() string {
	return ce.containerName
}

func (ce *ContainersEngine) getContainerUser() string {
	return ce.user.containerUsr
}

func ResolveContainerUser() string {
	if containerUser, err := expand(containerconf.Get(containerconf.KContainerUser)); err == nil {
		return containerUser
	}
	return kDefaultUser

}

func (ce *ContainersEngine) getRemoteUser() string {
	return ce.user.remoteUsr
}

func validateRunArg(arg string) error {
	switch arg {
	case "--userns=keep-id":
		keepIdfeature := "userns-keepid"
		if featuresNames, ok := containerconf.Get(containerconf.KFeatures).(map[string]interface{}); ok {
			for featureName := range featuresNames {
				if featureName == keepIdfeature {
					return nil
				}
			}
		}
		return cerr.NewError(fmt.Sprintf("missing feature '%s'", keepIdfeature))
	}
	return nil
}

func buildDevContainerName(projectName string) string {
	devcontainerNameMutex.Lock()
	defer devcontainerNameMutex.Unlock()
	if sDevcontainerName != "" {
		return sDevcontainerName
	}
	hostname, _ := os.Hostname()
	if containerNameInConfig, ok := containerconf.Get(containerconf.KName).(string); ok {
		sDevcontainerName = fmt.Sprintf("%s-%s-%s", projectName, containerNameInConfig, hostname)
	} else {
		clog.Warn("Failed to retrieve container name from .devcontainer, changing naming pattern")
		currentTime := time.Now()
		sDevcontainerName = fmt.Sprintf(
			"%s-%s-%d-%02d-%02dT%02d%02d%02d",
			projectName,
			hostname,
			currentTime.Year(),
			currentTime.Month(),
			currentTime.Day(),
			currentTime.Hour(),
			currentTime.Minute(),
			currentTime.Second(),
		)
	}
	return sDevcontainerName
}

func GetDevcontainerName(projectName string) string {
	return buildDevContainerName(projectName)
}

func ResolveRemoteUserFromConf() string {
	if remoteUsr, err := expand(containerconf.Get(containerconf.KRemoteUser)); err == nil {
		return remoteUsr
	}
	return kDefaultUser
}

func expand(a interface{}) (string, error) {
	val, ok := a.(string)
	if !ok {
		return "", cerr.NewError(fmt.Sprintf("Failed to expand value (%v), incorrect cast to string", a))
	}
	matches := FindExpandablePartInString(val)
	for _, match := range matches {
		envVarVal, err := GetVariableValue(match, "", bo.Container{})
		if err != nil {
			clog.Warn(fmt.Sprintf("Couldn't get environment variable value for %s", match.fullMatch), err)
			continue
		}
		val = strings.ReplaceAll(val, match.fullMatch, envVarVal)
	}
	return val, nil
}

func GetVariableValue(envVar evaluableEnvVar, engineName string, containerInfo bo.Container) (string, error) {
	value, err := func() (string, error) {
		switch envVar.envName {
		case localEnv:
			return getEnvValueFromHost(envVar.varName)
		case containerEnv:
			return getEnvValueFromContainer(envVar.varName, engineName, containerInfo)
		default:
			return "", cerr.NewError(fmt.Sprintf("Unknown environment variable type '%s'", envVar.envName))
		}
	}()

	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to retrieve value of environment variable %s", envVar.varName), err)
	}
	return value, nil
}

func fromMemoizeMap(varName string) (string, bool) {
	enVarsMutex.Lock()
	defer enVarsMutex.Unlock()
	if envVars == nil {
		envVars = make(map[string]string)
	}
	if val, ok := envVars[varName]; ok {
		return val, true
	}
	return "", false
}

func addVarToMap(key, val string) {
	enVarsMutex.Lock()
	defer enVarsMutex.Unlock()
	envVars[key] = val
}

func getEnvValueFromHost(varName string) (string, error) {
	if val, ok := fromMemoizeMap(varName); ok {
		return val, nil
	}
	cmd := fmt.Sprintf(`echo $%v`, varName)
	rEvent := &RunEvent{
		cmd:             cmd,
		eventInfo:       fmt.Sprintf("Retrieving env variable '%s'", varName),
		continueProcess: true,
	}

	stdout, execErr := shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{rEvent})

	if execErr != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to get local env variable (%s)", varName), execErr)
	}

	stdout = strings.Trim(stdout, "\n")
	addVarToMap(varName, stdout)
	return stdout, nil
}

func getEnvValueFromContainer(varName string, engineName string, containerInfo bo.Container) (string, error) {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_CMD_GET_ENV_VARIABLE)
	ce.SetContainerName(string(containerInfo.Name))
	ce.SetRemoteUser(string(containerInfo.RemoteUser))
	ce.SetEnvVarName(varName)

	value, err := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to get environment variable %s from container", varName), err)
	}
	return value, nil
}

func FindExpandablePartInString(text string) []evaluableEnvVar {
	re := regexp.MustCompile(kDevContainerVarRegXp)
	matches := re.FindAllStringSubmatch(text, -1)
	var result []evaluableEnvVar
	for _, match := range matches {
		// match[0] is the full match, match[1] is the envName, match[2] is the varName
		result = append(result, evaluableEnvVar{
			fullMatch: match[0],
			envName:   envName(match[1]),
			varName:   match[2],
		})
	}
	return result
}

func GetHostHomeDir() string {
	homeDir, err := getEnvValueFromHost(containerconf.KHomeEnvVariable)
	if err != nil {
		clog.Warn("Fail to get the HOME environment variable from host", err)
		return ""
	}
	return homeDir
}

func GetContainerStatus(containerName, containerEngineName string) (bo.ContainerStatus, error) {
	es := NewContainerEngine()
	SetEngineName(es, containerEngineName)
	es.SetAction(K_ACTION_PS)
	es.SetFormat(K_FCONTAINER_ID_STATUS)

	output, exCmdErr := ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if exCmdErr != nil {
		return bo.KContainerStatusUnknown,
			cerr.AppendError("Failed to get registry container info",
				exCmdErr,
			)
	}

	containersOnHostInfo, parseErr := ParseContainersInfo(output, es.PrintedFormat())
	if parseErr != nil {
		return bo.KContainerStatusUnknown,
			cerr.AppendError("Failed to get registry container info",
				parseErr,
			)
	}

	for _, foundContainer := range containersOnHostInfo {
		if foundContainer.Name == bo.ContainerName(containerName) {
			return foundContainer.Status, nil
		}
	}
	return bo.KContainerStatusDeleted, nil
}

// Now that we have a profile sub document in the devcontainer. It is important to review the devcontainer.json parsing (all variables should be expanded once in a preprocessing step)
// This will imply refactoring parts of this file...
// To workaround this limitation, the following function is supposed to ease expanding variables values outside of engine package:

func ExpandAttributeValue(txt string) string {
	val, expandErr := expand(txt)
	if expandErr != nil {
		return ""
	}
	return val
}

func BypassHTTPProxyInAzure(engineName string, containerName string, domainName string) error {
	// if !com.IsAzure(target.Name) {
	// 	clog.Debug(fmt.Sprintf("Host %s is not in Azure. Skip bypass http/s proxy", target.Name))
	// 	return nil
	// }

	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_CMD_BYPASS_PROXY_IN_AZURE)
	ce.SetContainerName(containerName)
	ce.SetRemoteUser(KRootUsr)
	ce.SetAzureDomainName(domainName)

	if _, err := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendError("Failed to bypass HTTP/HTTPS proxy for Azure", err)
	}
	return nil
}

func AddTempSharedKeys(projectName string, engineName string, containerName string) error {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_SHARE_SSH)
	ce.SetContainerName(containerName)

	if _, err := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendError("Failed to add temp key to authorised_keys", err)
	}

	return nil
}

func DeleteSharedKeys(projectName string, engineName string, containerName string) error {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_UNSHARE_SSH)
	ce.SetContainerName(containerName)

	if _, err := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); err != nil {
		return cerr.AppendError("Failed to add temp key to authorised_keys", err)
	}

	return nil
}

func StartContainer(containerName string) error {
	es := NewContainerEngine()
	es.SetAction(K_ACTION_START)
	es.AddArg(containerName)

	if _, exCmdErr := ExecuteCommand(es, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to start container %s", containerName),
			exCmdErr,
		)
	}
	return nil
}

func StopContainer(containerName string) error {
	es := NewContainerEngine()
	es.SetAction(K_ACTION_STOP)
	es.AddArg(containerName)

	if _, exCmdErr := ExecuteCommand(es, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to stop container %s", containerName),
			exCmdErr,
		)
	}
	return nil
}

func DeleteContainer(containerName string) error {
	es := NewContainerEngine()
	es.SetAction(K_ACTION_REMOVE)
	es.AddArg(containerName)

	es.AddArg("-f")
	es.AddArg(containerName)
	if _, exCmdErr := ExecuteCommand(es, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to delete container %s", containerName),
			exCmdErr,
		)
	}
	return nil
}

func CheckPathExistsInContainer(path string, containerName string, user string) (bool, error) {
	ecp := NewContainerEngine()
	ecp.SetAction(K_ACTION_EXE)
	ecp.SetExecuteCmd(K_EXEC_CMD_CHECK_PATH_EXISTS)
	ecp.SetPathToCheck(path)
	ecp.SetContainerName(containerName)
	ecp.SetRemoteUser(user)

	stdout, err := ExecuteCommand(ecp, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return false, err
	}

	return !strings.Contains(stdout, kPathNotExists), nil
}

func (ce *ContainersEngine) network() error {
	var rEvent *RunEvent
	var err error
	switch ce.networkCmd {
	case K_NETWORK_CONNECT:
		rEvent, err = ce.connectNetwork()
	case K_NETWORK_CREATE:
		rEvent, err = ce.createNetwork()
	case K_NETWORK_EXISTS:
		rEvent, err = ce.networkExists()
	case K_NETWORK_DISCONNECT:
		rEvent, err = ce.disconnectFromNetwork()
	default:
		return cerr.NewError(fmt.Sprintf("Failed to execute network command, action '%v' is not implemented", networkAction(ce.networkCmd)))

	}
	if err != nil {
		return err
	}
	ce.cmds = append(ce.cmds, rEvent)
	return nil
}

func (ce *ContainersEngine) connectNetwork() (*RunEvent, error) {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		networkAction(ce.networkCmd),
		ce.networkName,
		ce.containerName,
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       fmt.Sprintf("Connecting container '%s' to network '%s'", ce.containerName, ce.networkName),
		continueProcess: true,
	}
	return rEvent, nil
}

func (ce *ContainersEngine) createNetwork() (*RunEvent, error) {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		networkAction(ce.networkCmd),
		ce.networkName,
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       fmt.Sprintf("Creating network '%s'", ce.networkName),
		continueProcess: true,
	}
	return rEvent, nil
}

func (ce *ContainersEngine) networkExists() (*RunEvent, error) {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		networkAction(ce.networkCmd),
		ce.networkName,
		fmt.Sprintf("&& echo %s || true", kNetworkExistsVerb),
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       fmt.Sprintf("Checking if network '%s' exists", ce.networkName),
		continueProcess: true,
	}
	return rEvent, nil
}

func (ce *ContainersEngine) disconnectFromNetwork() (*RunEvent, error) {
	cmd := cg.VariadicJoin(" ", engineName(ce.name),
		engineAction(ce.action),
		networkAction(K_NETWORK_DISCONNECT),
		ce.networkName,
		ce.containerName,
	)
	rEvent := &RunEvent{
		cmd: cmd,

		eventInfo:       fmt.Sprintf("Disconnecting container '%s' from network '%s'", ce.containerName, ce.networkName),
		continueProcess: true,
	}
	return rEvent, nil
}

func (ce *ContainersEngine) SetNetworkName(networkName string) {
	ce.networkName = networkName
}

func (ce *ContainersEngine) SetNetworkCmd(cmd Network_t) {
	ce.networkCmd = cmd
}

// Returns the list of networks the container is connected to using inspect with K_FNetworks format
func GetContainerNetworks(engineName, containerName string) ([]string, error) {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_INSPECT)
	ce.SetContainerName(containerName)

	ce.SetFormat(K_FNetworks)
	out, exCmdErr := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput)
	if exCmdErr != nil {
		return nil, cerr.AppendError(
			fmt.Sprintf("Failed to list networks for container %s", containerName),
			exCmdErr,
		)
	}
	networksRaw := strings.Split(strings.TrimSpace(out), ",")
	networks := cg.FilterNilFromSlice(networksRaw)
	return networks, nil
}

// ConnectContainerToNetwork connects a container to a network using '$engine network connect $network $container'
func ConnectContainerToNetwork(engineName, containerName, networkName string) error {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_CONNECT)
	ce.SetNetworkName(networkName)
	ce.SetContainerName(containerName)

	if out, exCmdErr := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to add container %s to network %s. Output: %v", containerName, networkName, out),
			exCmdErr,
		)
	}
	return nil
}

// CreateNetwork creates a network using '$engine network create $network'
func CreateNetwork(engineName, networkName string) error {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_CREATE)
	ce.SetNetworkName(networkName)

	if out, exCmdErr := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to create network %s. Output: %v", networkName, out),
			exCmdErr,
		)
	}
	return nil
}

// NetworkExists checks if a network exists using '$engine network exists $network'
func NetworkExists(engineName, networkName string) bool {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_EXISTS)
	ce.SetNetworkName(networkName)

	// Error is ignored as the output of the command will be empty in case of error leading to exactly the same table of result
	// | Condition  | out      | Err  | return |
	// |------------|----------|------|--------|
	// | exists     | "exists" | nil  | true   |
	// | not exists | ""       | nil  | false  |
	// | error      | ""       | err  | false  |
	out, _ := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput)
	return strings.Contains(out, kNetworkExistsVerb)
}

// DisconnectContainerFromNetwork disconnects a container from a network using '$engine network disconnect $network $container'
func DisconnectContainerFromNetwork(engineName, containerName, networkName string) error {
	ce := NewContainerEngine()
	SetEngineName(ce, engineName)
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_DISCONNECT)
	ce.SetNetworkName(networkName)
	ce.SetContainerName(containerName)

	if out, exCmdErr := ExecuteCommand(ce, shexec.RunLocalCmdWithOutput); exCmdErr != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to disconnect container %s from network %s. Output: %v", containerName, networkName, out),
			exCmdErr,
		)
	}
	return nil
}

func ReplaceCurrentNetworks(engineName, containerName, networkName string) error {
	networks, err := GetContainerNetworks(engineName, containerName)
	if err != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to get current networks for container %s", containerName),
			err,
		)
	}

	isContainerAlreadyConnected := slices.Contains(networks, networkName)
	if !isContainerAlreadyConnected {
		err := ConnectContainerToNetwork(engineName, containerName, networkName)
		if err != nil {
			return cerr.AppendErrorFmt("Couldn't connect '%s' to network '%s'", err, containerName, networkName)
		}
	}

	for _, network := range networks {
		if network == networkName {
			continue
		}
		err := DisconnectContainerFromNetwork(engineName, containerName, network)
		if err != nil {
			return cerr.AppendErrorFmt("Couldn't disconnect '%s' from network '%s'", err, containerName, network)
		}
	}
	return nil
}
