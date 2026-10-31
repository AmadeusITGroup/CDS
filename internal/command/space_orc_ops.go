package command

import (
	"context"
	"fmt"
	"io"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

// spcOrcRun builds (creates) the cluster, streaming progress.
type spcOrcRun struct {
	defaultCmd
	orcTarget
}

var _ baseCmd = (*spcOrcRun)(nil)

func (s *spcOrcRun) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "run",
			Aliases:       []string{"build", "create"},
			Short:         "Build the orchestration cluster on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcOrcRun) subCommands() []baseCmd { return s.subCmds }

func (s *spcOrcRun) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		stream, err := c.cluster.BuildCluster(ctx, &cdspb.BuildClusterRequest{})
		if err != nil {
			return cerr.AppendError("Failed to build cluster through agent", err)
		}
		resp, err := receiveClusterBuildResponse(stream)
		if err != nil {
			return err
		}
		if resp.GetError() != cg.EmptyStr {
			return cerr.NewError(resp.GetError())
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: fmt.Sprintf("Cluster %q built on %q", resp.GetName(), hostName)})
	}
	return callback.executeOn(hostName, withLongTimeout())
}

// receiveClusterBuildResponse drains the BuildCluster stream, logging progress
// and returning the terminal response.
func receiveClusterBuildResponse(stream cdspb.ClusterService_BuildClusterClient) (*cdspb.BuildClusterResponse, error) {
	var response *cdspb.BuildClusterResponse
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cerr.AppendError("Failed to receive cluster build progress", err)
		}
		if progress := event.GetProgress(); progress != nil {
			clog.Info(progress.GetMessage())
		}
		if r := event.GetResponse(); r != nil {
			response = r
		}
	}
	if response == nil {
		return nil, cerr.NewError("agent cluster build finished without a final response")
	}
	return response, nil
}

// spcOrcStart starts the cluster.
type spcOrcStart struct {
	defaultCmd
	orcTarget
}

var _ baseCmd = (*spcOrcStart)(nil)

func (s *spcOrcStart) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "start",
			Short:         "Start the orchestration cluster on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcOrcStart) subCommands() []baseCmd { return s.subCmds }

func (s *spcOrcStart) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.cluster.StartCluster(ctx, &cdspb.StartClusterRequest{})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName, withLongTimeout())
}

// spcOrcStop stops the cluster.
type spcOrcStop struct {
	defaultCmd
	orcTarget
}

var _ baseCmd = (*spcOrcStop)(nil)

func (s *spcOrcStop) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "stop",
			Short:         "Stop the orchestration cluster on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcOrcStop) subCommands() []baseCmd { return s.subCmds }

func (s *spcOrcStop) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.cluster.StopCluster(ctx, &cdspb.StopClusterRequest{})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName)
}

// spcOrcDelete deletes the cluster.
type spcOrcDelete struct {
	defaultCmd
	orcTarget
	force bool
}

var _ baseCmd = (*spcOrcDelete)(nil)

func (s *spcOrcDelete) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "delete",
			Aliases:       []string{"rm"},
			Short:         "Delete the orchestration cluster on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
		s.cmd.Flags().BoolVarP(&s.force, "force", "f", false, "tear the cluster down even when it is in use")
	}
	return s.cmd
}

func (s *spcOrcDelete) subCommands() []baseCmd { return s.subCmds }

func (s *spcOrcDelete) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.cluster.DeleteCluster(ctx, &cdspb.DeleteClusterRequest{Force: s.force})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName, withLongTimeout())
}

// spcOrcRebuild deletes and rebuilds the cluster.
type spcOrcRebuild struct {
	defaultCmd
	orcTarget
	force bool
}

var _ baseCmd = (*spcOrcRebuild)(nil)

func (s *spcOrcRebuild) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "rebuild",
			Short:         "Delete and rebuild the orchestration cluster on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
		s.cmd.Flags().BoolVarP(&s.force, "force", "f", false, "tear the cluster down even when it is in use")
	}
	return s.cmd
}

func (s *spcOrcRebuild) subCommands() []baseCmd { return s.subCmds }

func (s *spcOrcRebuild) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		if _, err := c.cluster.DeleteCluster(ctx, &cdspb.DeleteClusterRequest{Force: s.force}); err != nil {
			return cerr.AppendError("Failed to delete cluster before rebuild", err)
		}
		stream, err := c.cluster.BuildCluster(ctx, &cdspb.BuildClusterRequest{})
		if err != nil {
			return cerr.AppendError("Failed to rebuild cluster through agent", err)
		}
		resp, err := receiveClusterBuildResponse(stream)
		if err != nil {
			return err
		}
		if resp.GetError() != cg.EmptyStr {
			return cerr.NewError(resp.GetError())
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: fmt.Sprintf("Cluster %q rebuilt on %q", resp.GetName(), hostName)})
	}
	return callback.executeOn(hostName, withLongTimeout())
}
