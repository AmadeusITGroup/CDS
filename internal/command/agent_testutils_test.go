package command

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/amadeusitgroup/cds/internal/tls/tlstest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// fakeAgent implements the agent services in memory.
type fakeAgent struct {
	cdspb.UnimplementedAgentInfoServiceServer
	cdspb.UnimplementedContainerServiceServer
	cdspb.UnimplementedArtifactServiceServer

	mu         sync.Mutex
	containers map[string]*cdspb.Container
	staged     map[string][]byte
	calls      []string
	deploys    []*cdspb.DeployContainerRequest
	rebuilds   []*cdspb.RebuildContainerRequest
	executed   []*cdspb.ExecuteInContainerRequest
	deployErr  string
}

func (f *fakeAgent) record(call string) {
	f.calls = append(f.calls, call)
}

func (f *fakeAgent) GetVersion(context.Context, *cdspb.GetVersionRequest) (*cdspb.GetVersionResponse, error) {
	return &cdspb.GetVersionResponse{Current: cg.Version}, nil
}

func (f *fakeAgent) ListContainers(context.Context, *cdspb.ListContainerRequest) (*cdspb.ListContainersResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	response := &cdspb.ListContainersResponse{}
	for _, c := range f.containers {
		// like the agent, the listing is a summary without port mapping
		response.Containers = append(response.Containers, &cdspb.Container{Id: c.Id, Name: c.Name, Status: c.Status, Host: c.Host})
	}
	return response, nil
}

func (f *fakeAgent) container(name string) (*cdspb.Container, error) {
	c, ok := f.containers[name]
	if !ok {
		return nil, status.Errorf(5, "container %s not found", name)
	}
	return c, nil
}

func (f *fakeAgent) GetContainer(_ context.Context, req *cdspb.GetContainerRequest) (*cdspb.GetContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.container(req.GetContainerName())
	if err != nil {
		return nil, err
	}
	return &cdspb.GetContainerResponse{Container: c}, nil
}

func (f *fakeAgent) setStatus(name, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.container(name)
	if err != nil {
		return err
	}
	c.Status = status
	return nil
}

func (f *fakeAgent) StartContainer(_ context.Context, req *cdspb.StartContainerRequest) (*cdspb.StartContainerResponse, error) {
	f.record("start " + req.GetContainerName())
	return &cdspb.StartContainerResponse{}, f.setStatus(req.GetContainerName(), "running")
}

func (f *fakeAgent) StopContainer(_ context.Context, req *cdspb.StopContainerRequest) (*cdspb.StopContainerResponse, error) {
	f.record("stop " + req.GetContainerName())
	return &cdspb.StopContainerResponse{}, f.setStatus(req.GetContainerName(), "exited")
}

func (f *fakeAgent) DeleteContainer(_ context.Context, req *cdspb.DeleteContainerRequest) (*cdspb.DeleteContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("delete %s %s", req.GetContainerName(), req.GetProjectName()))
	delete(f.containers, req.GetContainerName())
	return &cdspb.DeleteContainerResponse{}, nil
}

func (f *fakeAgent) RenameContainer(_ context.Context, req *cdspb.RenameContainerRequest) (*cdspb.RenameContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, err := f.container(req.GetContainerName())
	if err != nil {
		return nil, err
	}
	delete(f.containers, req.GetContainerName())
	c.Name = req.GetNewContainerName()
	f.containers[c.Name] = c
	return &cdspb.RenameContainerResponse{}, nil
}

func (f *fakeAgent) ExecuteInContainer(_ context.Context, req *cdspb.ExecuteInContainerRequest) (*cdspb.ExecuteInContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = append(f.executed, req)
	return &cdspb.ExecuteInContainerResponse{}, nil
}

func (f *fakeAgent) DeployContainer(req *cdspb.DeployContainerRequest, stream grpc.ServerStreamingServer[cdspb.DeployContainerEvent]) error {
	f.mu.Lock()
	f.deploys = append(f.deploys, req)
	if f.deployErr == cg.EmptyStr {
		f.containers[req.GetContainerName()] = &cdspb.Container{
			Id: "id-" + req.GetContainerName(), Name: req.GetContainerName(), Status: "running",
			PortMapping: map[string]int32{"22/tcp": 2222},
		}
	}
	f.mu.Unlock()

	if err := stream.Send(&cdspb.DeployContainerEvent{Payload: &cdspb.DeployContainerEvent_Progress{
		Progress: &cdspb.DeployContainerProgress{Phase: "init", Message: "deploying"},
	}}); err != nil {
		return err
	}
	return stream.Send(&cdspb.DeployContainerEvent{Payload: &cdspb.DeployContainerEvent_Response{
		Response: &cdspb.DeployContainerResponse{ContainerName: req.GetContainerName(), Error: f.deployErr},
	}})
}

func (f *fakeAgent) RebuildContainer(_ context.Context, req *cdspb.RebuildContainerRequest) (*cdspb.RebuildContainerResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rebuilds = append(f.rebuilds, req)
	return &cdspb.RebuildContainerResponse{ContainerName: req.GetContainerName()}, nil
}

func (f *fakeAgent) PrepareArtifacts(_ context.Context, req *cdspb.PrepareArtifactsRequest) (*cdspb.PrepareArtifactsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	response := &cdspb.PrepareArtifactsResponse{}
	for _, desc := range req.GetDescriptors() {
		if _, ok := f.staged[desc.GetIdentifier()]; ok {
			response.Found = append(response.Found, desc)
		} else {
			response.Missing = append(response.Missing, desc)
		}
	}
	return response, nil
}

func (f *fakeAgent) UploadArtifact(stream grpc.ClientStreamingServer[cdspb.UploadArtifactChunk, cdspb.UploadArtifactResponse]) error {
	var desc *cdspb.ArtifactDescriptor
	var content []byte
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if chunk.GetDescriptor_() != nil {
			desc = chunk.GetDescriptor_()
		}
		content = append(content, chunk.GetContent()...)
	}
	f.mu.Lock()
	f.staged[desc.GetIdentifier()] = content
	f.record("upload " + desc.GetIdentifier())
	f.mu.Unlock()
	return stream.SendAndClose(&cdspb.UploadArtifactResponse{Received: desc})
}

// setupTestAgent isolates HOME and the cds configuration in temporary
// directories and serves a fakeAgent over mutual TLS, registered as the
// localhost agent, considered started for the current command.
func setupTestAgent(t *testing.T, certHosts ...string) *fakeAgent {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CDS_CONFIG_PATH", home)
	certsDir := filepath.Dir(cdstls.CAFilePath())
	tlstest.WriteCerts(t, certsDir, certHosts...)

	tlsConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CAFile:   cdstls.CAFilePath(),
		CertFile: filepath.Join(certsDir, "agent-srv.pem"),
		KeyFile:  filepath.Join(certsDir, "agent-srv-key.pem"),
		Server:   true,
	})
	require.NoError(t, err)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	agent := &fakeAgent{containers: map[string]*cdspb.Container{}, staged: map[string][]byte{}}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	cdspb.RegisterAgentInfoServiceServer(server, agent)
	cdspb.RegisterContainerServiceServer(server, agent)
	cdspb.RegisterArtifactServiceServer(server, agent)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	_, port, err := net.SplitHostPort(lis.Addr().String())
	require.NoError(t, err)
	require.NoError(t, config.UpsertAgentForHostInConfig(cg.KLocalhost, config.NewAgent(
		config.WithTargetAddress(net.JoinHostPort(cg.KLocalhost, port)),
	)))
	sStartedAgents[cg.KLocalhost] = true
	t.Cleanup(func() { delete(sStartedAgents, cg.KLocalhost) })
	return agent
}

// setupTestProject registers a project deployed on localhost whose
// devcontainer.json is config, and the user default SSH key pair.
func setupTestProject(t *testing.T, projectName, devcontainer string) {
	t.Helper()
	home := os.Getenv("HOME")
	configDir := filepath.Join(t.TempDir(), ".devcontainer")
	require.NoError(t, os.MkdirAll(configDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "devcontainer.json"), []byte(devcontainer), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("private"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519.pub"), []byte("ssh-ed25519 AAAATEST user@test\n"), 0600))

	// the db is only used in memory
	require.NoError(t, db.AddProjectUsingConfDir(projectName, configDir))
	require.NoError(t, db.SetProjectHost(projectName, cg.KLocalhost))
	t.Cleanup(func() { _, _ = db.DeleteProject(projectName) })
}

func readSSHConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".ssh", "config"))
	if os.IsNotExist(err) {
		return cg.EmptyStr
	}
	require.NoError(t, err)
	return string(data)
}
