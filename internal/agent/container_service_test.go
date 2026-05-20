package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockContainerOps struct {
	containers bo.Containers
	listErr    error
	getErr     error
	startErr   error
	stopErr    error
	deleteErr  error
	renameErr  error
	execOutput string
	execCode   int
	execErr    error
	deployErr  error

	lastStarted string
	lastStopped string
	lastDeleted string
	lastRenamed []string
	lastExecCmd string
	lastDeploy  deployContainerSpec
	progress    []deployProgress
}

func (m *mockContainerOps) ListContainers() (bo.Containers, error) {
	return m.containers, m.listErr
}

func (m *mockContainerOps) GetContainer(name string) (bo.Container, error) {
	if m.getErr != nil {
		return bo.Container{}, m.getErr
	}
	for _, c := range m.containers {
		if string(c.Name) == name {
			return c, nil
		}
	}
	return bo.Container{Name: bo.ContainerName(name)}, nil
}

func (m *mockContainerOps) StartContainer(name string) error {
	m.lastStarted = name
	return m.startErr
}

func (m *mockContainerOps) StopContainer(name string) error {
	m.lastStopped = name
	return m.stopErr
}

func (m *mockContainerOps) DeleteContainer(name string) error {
	m.lastDeleted = name
	return m.deleteErr
}

func (m *mockContainerOps) RenameContainer(name, newName string) error {
	m.lastRenamed = []string{name, newName}
	return m.renameErr
}

func (m *mockContainerOps) ExecuteInContainer(containerName, command, user string) (string, int, error) {
	m.lastExecCmd = command
	return m.execOutput, m.execCode, m.execErr
}

func (m *mockContainerOps) DeployContainer(spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error) {
	m.lastDeploy = spec
	for _, progress := range m.progress {
		if err := report(progress); err != nil {
			return deployContainerResult{containerName: spec.containerName}, err
		}
	}
	return deployContainerResult{
		containerName: spec.containerName,
		output:        "deployed",
	}, m.deployErr
}

func newTestContainerService(ops ContainerOps) *containerServiceServer {
	return &containerServiceServer{
		engineOps:  ops,
		stagingDir: "test-staging",
	}
}

func TestListContainers(t *testing.T) {
	tests := map[string]struct {
		ops      *mockContainerOps
		wantLen  int
		wantCode codes.Code
	}{
		"returns empty list": {
			ops:     &mockContainerOps{containers: bo.Containers{}},
			wantLen: 0,
		},
		"returns multiple containers": {
			ops: &mockContainerOps{
				containers: bo.Containers{
					{Id: "abc123", Name: "dev-1", Status: bo.KContainerStatusRunning},
					{Id: "def456", Name: "dev-2", Status: bo.KContainerStatusExited},
				},
			},
			wantLen: 2,
		},
		"engine error returns Internal": {
			ops:      &mockContainerOps{listErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.ListContainers(context.Background(), &cdspb.ListContainerRequest{})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Len(t, resp.GetContainers(), tc.wantLen)
		})
	}
}

func TestListContainers_MapsFields(t *testing.T) {
	ops := &mockContainerOps{
		containers: bo.Containers{
			{
				Id:     "abc123",
				Name:   "my-dev",
				Status: bo.KContainerStatusRunning,
				Pmapping: bo.PortMapping{
					"22/tcp": 2222,
					"80/tcp": 8080,
				},
			},
		},
	}
	svc := newTestContainerService(ops)
	resp, err := svc.ListContainers(context.Background(), &cdspb.ListContainerRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetContainers(), 1)

	c := resp.GetContainers()[0]
	assert.Equal(t, "abc123", c.GetId())
	assert.Equal(t, "my-dev", c.GetName())
	assert.Equal(t, "running", c.GetStatus())
	assert.Equal(t, int32(2222), c.GetPortMapping()["22/tcp"])
	assert.Equal(t, int32(8080), c.GetPortMapping()["80/tcp"])
}

func TestGetContainer(t *testing.T) {
	tests := map[string]struct {
		name     string
		ops      *mockContainerOps
		wantName string
		wantCode codes.Code
	}{
		"returns container by name": {
			name: "my-dev",
			ops: &mockContainerOps{
				containers: bo.Containers{
					{Id: "abc", Name: "my-dev", Status: bo.KContainerStatusRunning},
				},
			},
			wantName: "my-dev",
		},
		"empty name returns InvalidArgument": {
			name:     "",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"engine error returns Internal": {
			name:     "fail",
			ops:      &mockContainerOps{getErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.GetContainer(context.Background(), &cdspb.GetContainerRequest{ContainerName: tc.name})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, resp.GetContainer().GetName())
		})
	}
}

func TestStartContainer(t *testing.T) {
	tests := map[string]struct {
		name     string
		ops      *mockContainerOps
		wantCode codes.Code
	}{
		"starts successfully": {
			name: "my-dev",
			ops:  &mockContainerOps{},
		},
		"empty name returns InvalidArgument": {
			name:     "",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"engine error returns Internal": {
			name:     "fail",
			ops:      &mockContainerOps{startErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.StartContainer(context.Background(), &cdspb.StartContainerRequest{ContainerName: tc.name})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, resp.GetOutput(), tc.name)
			assert.Equal(t, tc.name, tc.ops.lastStarted)
		})
	}
}

func TestStopContainer(t *testing.T) {
	tests := map[string]struct {
		name     string
		ops      *mockContainerOps
		wantCode codes.Code
	}{
		"stops successfully": {
			name: "my-dev",
			ops:  &mockContainerOps{},
		},
		"empty name returns InvalidArgument": {
			name:     "",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"engine error returns Internal": {
			name:     "fail",
			ops:      &mockContainerOps{stopErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.StopContainer(context.Background(), &cdspb.StopContainerRequest{ContainerName: tc.name})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, resp.GetOutput(), tc.name)
			assert.Equal(t, tc.name, tc.ops.lastStopped)
		})
	}
}

func TestDeleteContainer(t *testing.T) {
	tests := map[string]struct {
		name     string
		ops      *mockContainerOps
		wantCode codes.Code
	}{
		"deletes successfully": {
			name: "my-dev",
			ops:  &mockContainerOps{},
		},
		"empty name returns InvalidArgument": {
			name:     "",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"engine error returns Internal": {
			name:     "fail",
			ops:      &mockContainerOps{deleteErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.DeleteContainer(context.Background(), &cdspb.DeleteContainerRequest{ContainerName: tc.name})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, resp.GetOutput(), tc.name)
			assert.Equal(t, tc.name, tc.ops.lastDeleted)
		})
	}
}

func TestDeleteContainerRemovesProjectStaging(t *testing.T) {
	stagingDir := t.TempDir()
	projectDir := filepath.Join(stagingDir, "project-a")
	require.NoError(t, os.MkdirAll(projectDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "artifact.txt"), []byte("staged"), 0600))

	ops := &mockContainerOps{}
	svc := &containerServiceServer{
		engineOps:  ops,
		stagingDir: stagingDir,
	}

	resp, err := svc.DeleteContainer(context.Background(), &cdspb.DeleteContainerRequest{
		ContainerName: "container-a",
		ProjectName:   "project-a",
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetOutput(), "container-a")
	assert.Equal(t, "container-a", ops.lastDeleted)

	_, err = os.Stat(projectDir)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteContainerRejectsInvalidProjectBeforeDeleting(t *testing.T) {
	ops := &mockContainerOps{}
	svc := newTestContainerService(ops)

	_, err := svc.DeleteContainer(context.Background(), &cdspb.DeleteContainerRequest{
		ContainerName: "container-a",
		ProjectName:   "../project-a",
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Empty(t, ops.lastDeleted)
}

func TestRenameContainer(t *testing.T) {
	tests := map[string]struct {
		name     string
		newName  string
		ops      *mockContainerOps
		wantCode codes.Code
	}{
		"renames successfully": {
			name:    "my-dev",
			newName: "my-new-dev",
			ops:     &mockContainerOps{},
		},
		"empty current name returns InvalidArgument": {
			newName:  "my-new-dev",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"empty new name returns InvalidArgument": {
			name:     "my-dev",
			ops:      &mockContainerOps{},
			wantCode: codes.InvalidArgument,
		},
		"engine error returns Internal": {
			name:     "fail",
			newName:  "my-new-dev",
			ops:      &mockContainerOps{renameErr: assert.AnError},
			wantCode: codes.Internal,
		},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.RenameContainer(context.Background(), &cdspb.RenameContainerRequest{
				ContainerName:    tc.name,
				NewContainerName: tc.newName,
			})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, resp.GetOutput(), tc.newName)
			assert.Equal(t, []string{tc.name, tc.newName}, tc.ops.lastRenamed)
		})
	}
}

func TestBoContainerToProto(t *testing.T) {
	c := bo.Container{
		Id:     "abc",
		Name:   "dev",
		Status: bo.KContainerStatusExited,
		Pmapping: bo.PortMapping{
			"22/tcp": 2222,
		},
	}
	pb := boContainerToProto(c, "my-host")
	assert.Equal(t, "abc", pb.GetId())
	assert.Equal(t, "dev", pb.GetName())
	assert.Equal(t, "exited", pb.GetStatus())
	assert.Equal(t, "my-host", pb.GetHost())
	assert.Equal(t, int32(2222), pb.GetPortMapping()["22/tcp"])
}

func TestExecuteInContainer(t *testing.T) {
	tests := map[string]struct {
		containerName string
		command       string
		user          string
		ops           *mockContainerOps
		wantOutput    string
		wantExitCode  int32
		wantCode      codes.Code
	}{
		"executes successfully": {
			containerName: "my-dev",
			command:       "echo hello",
			ops:           &mockContainerOps{execOutput: "hello\n", execCode: 0},
			wantOutput:    "hello\n",
			wantExitCode:  0,
		},
		"returns output and exit code on failure": {
			containerName: "my-dev",
			command:       "exit 42",
			ops:           &mockContainerOps{execOutput: "error output", execCode: 42, execErr: assert.AnError},
			wantOutput:    "error output",
			wantExitCode:  42,
		},
		"empty container_name returns InvalidArgument": {
			containerName: "",
			command:       "echo hi",
			ops:           &mockContainerOps{},
			wantCode:      codes.InvalidArgument,
		},
		"empty command returns InvalidArgument": {
			containerName: "my-dev",
			command:       "",
			ops:           &mockContainerOps{},
			wantCode:      codes.InvalidArgument,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := newTestContainerService(tc.ops)
			resp, err := svc.ExecuteInContainer(context.Background(), &cdspb.ExecuteInContainerRequest{
				ContainerName: tc.containerName,
				Command:       tc.command,
				User:          tc.user,
			})
			if tc.wantCode != 0 {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOutput, resp.GetOutput())
			assert.Equal(t, tc.wantExitCode, resp.GetExitCode())
		})
	}
}

func TestDeployContainer(t *testing.T) {
	t.Run("empty name returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{ProjectName: "test-project", ContainerName: ""}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("empty project returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ContainerName:      "test-container",
			DevcontainerConfig: []byte(`{"image":"busybox:latest"}`),
		}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid project returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ProjectName:        "../test-project",
			ContainerName:      "test-container",
			DevcontainerConfig: []byte(`{"image":"busybox:latest"}`),
		}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("empty config returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ContainerName:      "test-container",
			ProjectName:        "test-project",
			DevcontainerConfig: nil,
		}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Empty(t, stream.events)
	})

	t.Run("invalid config returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ContainerName:      "test-container",
			ProjectName:        "test-project",
			DevcontainerConfig: []byte("{not-json"),
		}, stream)
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Empty(t, stream.events)
	})

	t.Run("sends progress and response events", func(t *testing.T) {
		ops := &mockContainerOps{
			progress: []deployProgress{{phase: "execute", message: "Creating container"}},
		}
		svc := newTestContainerService(ops)
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ContainerName:      "test-container",
			ProjectName:        "test-project",
			Labels:             map[string]string{"io.cds": "cds"},
			DevcontainerConfig: []byte(`{"image":"busybox:latest"}`),
		}, stream)
		require.NoError(t, err)
		require.Len(t, stream.events, 3, "expected init progress, execution progress, and response")

		first := stream.events[0]
		assert.NotNil(t, first.GetProgress(), "first event should be progress")
		assert.Equal(t, "init", first.GetProgress().GetPhase())

		second := stream.events[1]
		assert.NotNil(t, second.GetProgress(), "second event should be progress")
		assert.Equal(t, "execute", second.GetProgress().GetPhase())

		last := stream.events[len(stream.events)-1]
		assert.NotNil(t, last.GetResponse(), "last event should be response")
		assert.Equal(t, "test-container", last.GetResponse().GetContainerName())
		assert.Empty(t, last.GetResponse().GetError())
		assert.Equal(t, "test-project", ops.lastDeploy.projectName)
		assert.Equal(t, "cds", ops.lastDeploy.labels["io.cds"])
		assert.NotNil(t, ops.lastDeploy.resourceProvider)
	})

	t.Run("streams application error response on deploy failure", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{deployErr: assert.AnError})
		stream := &mockDeployStream{}
		err := svc.DeployContainer(&cdspb.DeployContainerRequest{
			ContainerName:      "test-container",
			ProjectName:        "test-project",
			DevcontainerConfig: []byte(`{"image":"busybox:latest"}`),
		}, stream)
		require.NoError(t, err)
		require.NotEmpty(t, stream.events)

		last := stream.events[len(stream.events)-1]
		require.NotNil(t, last.GetResponse())
		assert.Contains(t, last.GetResponse().GetError(), assert.AnError.Error())
	})
}

func TestRebuildContainer(t *testing.T) {
	t.Run("empty name returns InvalidArgument", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		_, err := svc.RebuildContainer(context.Background(), &cdspb.RebuildContainerRequest{ContainerName: ""})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("returns Unimplemented until cached deploy state exists", func(t *testing.T) {
		svc := newTestContainerService(&mockContainerOps{})
		_, err := svc.RebuildContainer(context.Background(), &cdspb.RebuildContainerRequest{ContainerName: "my-dev"})
		require.Error(t, err)
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	})
}

// mockDeployStream captures events sent during DeployContainer streaming.
type mockDeployStream struct {
	events []*cdspb.DeployContainerEvent
	grpc.ServerStreamingServer[cdspb.DeployContainerEvent]
}

func (m *mockDeployStream) Send(event *cdspb.DeployContainerEvent) error {
	m.events = append(m.events, event)
	return nil
}
