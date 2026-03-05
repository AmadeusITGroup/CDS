package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

type EngineFlags_t map[string][]string

type Engine interface {
	BuildCommands() ([]shexec.ExecuteEvent, error)
}

type Action_t int
type EngineName_t int
type Format_t int
type Execute_t int
type Copy_t int
type Run_t int
type attribute_t int
type Network_t int

const (
	K_ACTION_CP Action_t = iota
	K_ACTION_EXE
	K_ACTION_INSPECT
	K_ACTION_PS
	K_ACTION_RUN
	K_ACTION_START
	K_ACTION_STOP
	K_ACTION_REMOVE
	K_ACTION_SYSTEM
	K_ACTION_RENAME
	K_ACTION_NETWORK
	K_ACTION_BUILD
	K_ACTION_NaN
)
const (
	K_PODMAN EngineName_t = iota
	K_DOCKER
)
const (
	K_FPORT Format_t = iota
	K_FCONTAINER_ID_STATUS
	K_FNetworks
)

const (
	K_EXEC_CMD_SSH Execute_t = iota
	K_EXEC_CMD_HOMEDIR
	K_EXEC_CMD_CLONE_SRC_REPO
	K_EXEC_CMD_ID
	K_EXEC_CMD_CHOWN
	K_EXEC_CMD_MKDIR
	K_EXEC_CMD_GET_KIND_KUBECONF
	K_EXEC_CMD_GET_KIND_CLUSTER_STATUS
	K_EXEC_CMD_RSH
	K_EXEC_CMD_GIT_CONFIG
	K_EXEC_CMD_SECURE_REGISTRY // TODO BK remove
	K_EXEC_CMD_CHECK_PATH_EXISTS
	K_EXEC_CMD_CHECK_SECRET_EXISTS
	K_EXEC_CMD_CREATE_SECRET
	K_EXEC_CMD_CHECK_SERVICE_ACCOUNT_EXISTS
	K_EXEC_CMD_PATCH_SERVICE_ACCOUNT
	K_EXEC_CMD_ENV
	K_EXEC_CMD_APPLY_INGRESS
	K_EXEC_CMD_CHMOD_DEST_FILE
	K_EXEC_HELM_ADD_REPO
	K_EXEC_HELM_INSTALL_CHART
	K_EXEC_HELM_UNINSTALL_CHART
	K_EXEC_ORC_GET_CONFIG_MAP
	K_EXEC_HELM_CHECK_DEPLOYED_RELEASE
	K_EXEC_ORC_GET_NAMESPACES
	K_EXEC_CMD_CHECK_ORC_REACHABLE_FROM_DEVCONTAINER
	K_EXEC_ORC_CREATE_NAMESPACE
	K_EXEC_CUSTOM_CMD
	K_EXEC_CMD_GET_ENV_VARIABLE
	K_EXEC_CMD_WAIT_FOR_ORC_NODE // TODO remove
	K_EXEC_CMD_WAIT_FOR_INGRESS  // TODO remove
	K_EXEC_CMD_CHECK_INGRESS_CONTROLLER_STATUS
	K_EXEC_CMD_BYPASS_PROXY_IN_AZURE
	K_EXEC_SHARE_SSH
	K_EXEC_UNSHARE_SSH
)

const (
	K_NETWORK_CONNECT Network_t = iota
	K_NETWORK_CREATE
	K_NETWORK_DISCONNECT
	K_NETWORK_EXISTS
	K_NETWORK_INSPECT
	K_NETWORK_LIST
	K_NETWORK_PRUNE
	K_NETWORK_RELOAD
	K_NETWORK_REMOVE
	K_NETWORK_UPDATE
)

// This deployment file comes from official nginx deployment file: https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.10.1/deploy/static/provider/kind/deploy.yaml
// In case a new version of the deployment is released, you just need to upload it to A1 artifactory in the following path.
const KIngressDeploymentFile = "https://repository.rnd.amadeus.net:443/artifactory/devenv-generic-prod-devenv-exp-nce/xdlc/devenv/cds_dependencies/ingress-deployment.yaml"

const (
	K_CP_DEFAULT Copy_t = iota
)

const (
	K_RUN_DEV_CONTAINER Run_t = iota
	K_RUN_GEN
)

const (
	k_ATTR_DEFAULT_SHELL attribute_t = iota
)

const (
	KCDSContainerBasicLabel      = "io.cds"
	KCDSContainerBasicLabelValue = "cds"

	KCDSContainerFlavourNameLabel = "io.cds.flavour"

	KCDSSchedulerLabel      = "io.cds.scheduler"
	KCDSSchedulerLabelValue = "odin-scheduler"
)

var (
	executeMap = map[Execute_t]RunEvent{
		K_EXEC_CMD_SSH: {
			cmd:       `/bin/sh -c 'mkdir -p ~%[1]v/.ssh && chmod 700 ~%[1]v/.ssh && { [ -f ~%[1]v/.ssh/authorized_keys ] && [ -n "$(tail -c 1 ~%[1]v/.ssh/authorized_keys)" ] && echo >> ~%[1]v/.ssh/authorized_keys || true; } && echo "%v" >> ~%[1]v/.ssh/authorized_keys && chmod 600 ~%[1]v/.ssh/authorized_keys'`,
			eventInfo: "Install SSH public key in container",
		},
		K_EXEC_CMD_HOMEDIR: {
			cmd:       `/bin/sh -c 'echo $HOME'`,
			eventInfo: "Get $HOME",
		},
		K_EXEC_CMD_CLONE_SRC_REPO: {
			cmd:       `/bin/sh -c 'if [ -d /workspace/%v ]; then echo "directory (%v) exists. Nothing much to do!"; else mkdir -p /workspace && cd /workspace && git clone --recurse-submodules %v %v; fi'`,
			eventInfo: "Clone source repository in container",
		},
		K_EXEC_CMD_ID: {
			cmd:       `/bin/sh -c 'id'`,
			eventInfo: "Get user UID and GID",
		},
		K_EXEC_CMD_CHOWN: {
			cmd:       `/bin/sh -c 'chown %v:%v %v'`,
			eventInfo: "Chown file",
		},
		K_EXEC_CMD_MKDIR: {
			cmd:       `/bin/sh -c 'mkdir -p %v'`,
			eventInfo: "Create directory",
		},
		K_EXEC_CMD_GET_KIND_KUBECONF: {
			cmd:       `kubectl config view --minify --flatten`,
			eventInfo: "Get kind kube configuration",
		},
		K_EXEC_CMD_GET_KIND_CLUSTER_STATUS: {
			cmd:       `kubectl get --raw="/readyz?verbose" || true`,
			eventInfo: "Get kind cluster status",
		},
		K_EXEC_CMD_RSH: {
			cmd:       `bash -l`,
			eventInfo: "Attach to a container",
		},
		K_EXEC_CMD_GIT_CONFIG: {
			cmd:       `/bin/sh -c 'git config --global user.name "%v" && git config --global user.email "%v"'`,
			eventInfo: "Setting the global git configuration file in the container",
		},
		K_EXEC_CMD_SECURE_REGISTRY: {
			cmd:       `/bin/sh -c 'kubectl get secret/cdsregistrycred || kubectl create secret docker-registry cdsregistrycred --docker-server=cds.sec.io --docker-username=user --docker-password=password;sleep 10;kubectl patch serviceaccount default -p \\'{"imagePullSecrets": [{"name": "cdsregistrycred"}]}\\''`,
			eventInfo: "Create cdsregistrycred secret in K8s cluster",
		},
		K_EXEC_CMD_CHECK_PATH_EXISTS: {
			cmd:       `/bin/sh -c 'ls %v || echo "%v"'`,
			eventInfo: "Check if path exists",
		},
		K_EXEC_CMD_CHECK_SECRET_EXISTS: {
			cmd:       `/bin/sh -c 'kubectl get secret/cdsregistrycred --no-headers || true'`,
			eventInfo: "Check if secret (cdsregistrycred) exists",
		},
		K_EXEC_CMD_CREATE_SECRET: {
			cmd:       `/bin/sh -c 'kubectl create secret docker-registry cdsregistrycred --docker-server=cds.sec.io --docker-username=user --docker-password=password'`,
			eventInfo: "Create secret (cdsregistrycred)",
		},
		K_EXEC_CMD_CHECK_SERVICE_ACCOUNT_EXISTS: {
			cmd:       `/bin/sh -c 'kubectl get serviceaccount default --no-headers || true'`,
			eventInfo: "Check if service account (default) exists",
		},
		K_EXEC_CMD_PATCH_SERVICE_ACCOUNT: {
			cmd:       `/bin/sh -c 'kubectl patch serviceaccount default -p \\'{"imagePullSecrets": [{"name": "cdsregistrycred"}]}\\''`,
			eventInfo: "Patch service account (default) with secret (cdsregistrycred)",
		},
		K_EXEC_CMD_ENV: {
			cmd:       `/bin/sh -c '%s'`,
			eventInfo: "Set env variables",
		},
		K_EXEC_CMD_APPLY_INGRESS: {
			cmd:       fmt.Sprintf(`sh -c "unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy; kubectl apply -f %s"`, KIngressDeploymentFile),
			eventInfo: "Applying Nginx Ingress",
		},
		K_EXEC_CMD_CHMOD_DEST_FILE: {
			cmd:       `/bin/sh -c 'chmod %o %s'`,
			eventInfo: "Apply permission to a given file in the container",
		},
		K_EXEC_HELM_ADD_REPO: {
			cmd:       `/bin/sh -c 'helm repo add %s %s && helm repo update'`,
			eventInfo: "Adding a helm repository to the helm config on the devcontainer",
		},
		K_EXEC_HELM_INSTALL_CHART: {
			cmd:       `/bin/sh -c 'helm install %s %s/%s --namespace %s --set cds.namespace=%s'`,
			eventInfo: "Installing helm chart",
		},
		K_EXEC_HELM_UNINSTALL_CHART: {
			cmd:       `/bin/sh -c 'helm uninstall %s --namespace %s'`,
			eventInfo: "Uninstalling helm chart",
		},
		K_EXEC_ORC_GET_CONFIG_MAP: {
			cmd:       `/bin/sh -c 'kubectl get cm --selector=config-map-type=%s -o yaml  --namespace %s'`,
			eventInfo: "Getting config map from orchestration engine",
		},
		K_EXEC_HELM_CHECK_DEPLOYED_RELEASE: {
			cmd:       `/bin/sh -c 'helm list --deployed -o yaml -f ^%s$ --namespace %s'`,
			eventInfo: "Check if release is deployed",
		},
		K_EXEC_ORC_GET_NAMESPACES: {
			cmd:       `/bin/sh -c "kubectl get namespaces -o=jsonpath='{.items[*].metadata.name}'"`,
			eventInfo: "Get all namespace available in orchestration engine",
		},
		K_EXEC_CMD_CHECK_ORC_REACHABLE_FROM_DEVCONTAINER: {
			cmd:       `/bin/sh -c "kubectl get service/kubernetes -o name || true"`,
			eventInfo: "Check that orchestration engine is reachable from devcontainer",
		},
		K_EXEC_ORC_CREATE_NAMESPACE: {
			cmd:       `/bin/sh -c "kubectl create namespace %s"`,
			eventInfo: "Create namespace",
		},
		K_EXEC_CUSTOM_CMD: {
			cmd:       `/bin/sh -c '%s'`,
			eventInfo: "",
		},
		K_EXEC_CMD_GET_ENV_VARIABLE: {
			cmd:       `/bin/zsh -c 'echo $%s'`,
			eventInfo: "Getting env variable value",
		},
		K_EXEC_CMD_WAIT_FOR_ORC_NODE: { // TODO delete
			cmd:       `/bin/sh -c 'kubectl wait --for=condition=Ready nodes --all --timeout=100s'`,
			eventInfo: "Wait for orchestration to be ready",
		},
		K_EXEC_CMD_WAIT_FOR_INGRESS: { // TODO delete
			cmd:       `/bin/sh -c 'kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=120s'`,
			eventInfo: "Wait for Ingress controller to be ready",
		},
		K_EXEC_CMD_CHECK_INGRESS_CONTROLLER_STATUS: {
			cmd:       `/bin/sh -c 'kubectl get -n ingress-nginx pod --selector=app.kubernetes.io/component=controller --template "{{range .items}}{{.status.phase}}{{\"\n\"}}{{end}}"'`,
			eventInfo: "Check ingress-nginx controller pod status",
		},
		K_EXEC_CMD_BYPASS_PROXY_IN_AZURE: {
			cmd:       `/bin/sh -c 'grep "^NO_PROXY=|^no_proxy=" /etc/environment && sed -i "s/\(^NO_PROXY=.*\)/\1,%[1]v/g" /etc/environment && sed -i "s/\(^no_proxy=.*\)/\1,%[1]v/g" /etc/environment || echo "NO_PROXY=$NO_PROXY,%[1]v" >> /etc/environment && echo "no_proxy=$no_proxy,%[1]v" >> /etc/environment'`,
			eventInfo: "Bypassing HTTP/S proxy in Azure",
		},
		K_EXEC_SHARE_SSH: {
			cmd:       `/bin/sh -c 'echo "%v" >> ~%v/.ssh/authorized_keys'`,
			eventInfo: "Add temporal shared SSH public key in container",
		},
		K_EXEC_UNSHARE_SSH: {
			cmd:       `/bin/sh -c 'grep -v "%v" ~%v/.ssh/authorized_keys > ~%v/.ssh/authorized_keys_temp && mv ~%v/.ssh/authorized_keys_temp ~%v/.ssh/authorized_keys'`,
			eventInfo: "Remove temporal shared SSH public key in container",
		},
	}

	actionMap = map[Action_t]RunEvent{
		K_ACTION_START: {
			eventInfo: "Starting container ",
		},
		K_ACTION_STOP: {
			eventInfo: "Stopping container ",
		},
		K_ACTION_REMOVE: {
			eventInfo: "Removing container ",
		},
		K_ACTION_SYSTEM: {
			eventInfo: "System ",
		},
		K_ACTION_RENAME: {
			eventInfo: "Renaming container ",
		},
		K_ACTION_BUILD: {
			eventInfo: "Building image ",
		},
	}
)

func engineAction(a Action_t) string {
	return [...]string{"cp", "exec", "inspect", "ps", "run", "start", "stop", "rm", "system", "rename", "network", "build"}[a]
}

func networkAction(a Network_t) string {
	return [...]string{"connect", "create", "disconnect", "exists", "inspect", "list", "prune", "reload", "remove", "update"}[a]
}

func formatActionName(a string) string {
	return a
}

func engineAttribute(s string) attribute_t {
	switch s {
	case "defaultShell":
		return k_ATTR_DEFAULT_SHELL
	default:
		return -1
	}

}

func engineName(e EngineName_t) string {
	return [...]string{"podman", "docker"}[e]
}

func preComputedFormat(f Format_t) string {
	goTemplate := [...]string{
		`'{{range $port, $port_obj := .NetworkSettings.Ports}}"{{ $port }}":{{ range $pval := $port_obj }}"{{ $pval.HostPort }}",{{end}}{{end}}'`,
		`'{{ range . }}ID={{ .ID }},Name={{ .Names }},Status={{ .Status }};{{ end }}'`,
		`'{{ range $networkName, $networkData := .NetworkSettings.Networks }}{{ $networkName }},{{ end }}'`,
	}[f]
	return cg.VariadicJoin(" ", "--format", goTemplate)
}

func ParseContainerInfo(raw string, info *bo.Container, f Format_t) error {
	switch f {
	case K_FPORT:
		return parseInspectFormattedOutput(raw, info)
	}
	clog.Warn(fmt.Sprintf("Format of value %v is not handled", f))
	return nil
}

func parseInspectFormattedOutput(raw string, info *bo.Container) error {
	// TODO:Refactor: link the parsing to the gotemplate: preComputedFormat
	mappings := strings.Split(raw, ",")
	mappings = mappings[:len(mappings)-1]
	for _, mapping := range mappings {
		if len(mapping) == 0 {
			continue
		}
		ports := strings.Split(mapping, ":")
		if len(ports) != 2 {
			return cerr.NewError(fmt.Sprintf(`Unable to parse raw data: (%v)`, raw))
		}
		cPort := strings.Trim(ports[0], `"`)
		hPort, err := strconv.Atoi(strings.Trim(ports[1], `",`))
		if err != nil {
			return cerr.NewError(fmt.Sprintf(`Unable to parse host port from raw data: (%v)`, raw))
		}
		err = info.AddPort(cPort, hPort)
		if err != nil {
			return err
		}
	}
	return nil
}

func ParseContainersInfo(raw string, f Format_t) (bo.Containers, error) {
	switch f {
	case K_FCONTAINER_ID_STATUS:
		return parsePsFormattedOutput(raw)
	}
	return nil, cerr.NewError(fmt.Sprintf("Format of value %v is not handled", f))
}

func parsePsFormattedOutput(raw string) (bo.Containers, error) {
	// TODO:Refactor: link the parsing to the gotemplate: preComputedFormat
	info := bo.Containers{}
	containersRawData := strings.Split(raw, ";")
	containersRawData = containersRawData[:len(containersRawData)-1]
	for _, cRawData := range containersRawData {
		if len(cRawData) == 0 {
			continue
		}
		cData := strings.Split(cRawData, ",")
		containerData := parseContainerRawData(cData)
		info = append(info, containerData)
	}
	return info, nil
}

func parseContainerRawData(crd []string) bo.Container {
	// TODO:Refactor: link the parsing to the gotemplate: preComputedFormat
	rContainer := bo.Container{}
	for _, elem := range crd {
		val := parseValue(elem)
		switch {
		case strings.HasPrefix(elem, "ID"):
			rContainer.Id = bo.ContainerID(val)
		case strings.HasPrefix(elem, "Name"):
			rContainer.Name = bo.ContainerName(val)
		case strings.HasPrefix(elem, "Status"):
			switch {
			case strings.HasPrefix(val, "Up"):
				rContainer.Status = bo.KContainerStatusRunning
			case strings.HasPrefix(val, "Exited"):
				rContainer.Status = bo.KContainerStatusExited
			}
		}
	}
	return rContainer
}

func parseValue(s string) string {
	val := strings.Split(s, "=")
	if len(val) != 2 {
		return ""
	}
	return val[1]
}

func ExecuteCommands(engine Engine, execute func([]shexec.ExecuteEvent) error) error {
	cmds, err := engine.BuildCommands()
	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to build commands (%v)", cmds), err)
	}
	return execute(cmds)
}

func ExecuteCommand(engine Engine, execute func([]shexec.ExecuteEvent) (string, error)) (string, error) {
	cmds, err := engine.BuildCommands()
	if len(cmds) > 1 {
		return "", cerr.AppendError(fmt.Sprintf("Function called for more than one command (%v)", cmds), err)
	}
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to build command (%v)", cmds), err)
	}
	return execute(cmds)
}

func SetEngineName(e *ContainersEngine, name string) {
	switch name {
	case "docker":
		e.SetName(K_DOCKER)
	default:
		e.SetName(K_PODMAN)
	}
}
