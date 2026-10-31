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

// spcRegistryRun builds the secure registry, streaming progress.
type spcRegistryRun struct {
	defaultCmd
	regTarget
}

var _ baseCmd = (*spcRegistryRun)(nil)

func (s *spcRegistryRun) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "run",
			Aliases:       []string{"build", "create"},
			Short:         "Build the secure registry on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcRegistryRun) subCommands() []baseCmd { return s.subCmds }

func (s *spcRegistryRun) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		stream, err := c.registry.BuildRegistry(ctx, &cdspb.BuildRegistryRequest{Secure: true})
		if err != nil {
			return cerr.AppendError("Failed to build registry through agent", err)
		}
		resp, err := receiveRegistryBuildResponse(stream)
		if err != nil {
			return err
		}
		if resp.GetError() != cg.EmptyStr {
			return cerr.NewError(resp.GetError())
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: fmt.Sprintf("Registry %q built on %q", resp.GetName(), hostName)})
	}
	return callback.executeOn(hostName, withLongTimeout())
}

func receiveRegistryBuildResponse(stream cdspb.RegistryService_BuildRegistryClient) (*cdspb.BuildRegistryResponse, error) {
	var response *cdspb.BuildRegistryResponse
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cerr.AppendError("Failed to receive registry build progress", err)
		}
		if progress := event.GetProgress(); progress != nil {
			clog.Info(progress.GetMessage())
		}
		if r := event.GetResponse(); r != nil {
			response = r
		}
	}
	if response == nil {
		return nil, cerr.NewError("agent registry build finished without a final response")
	}
	return response, nil
}

// spcRegistryStart starts the registry.
type spcRegistryStart struct {
	defaultCmd
	regTarget
}

var _ baseCmd = (*spcRegistryStart)(nil)

func (s *spcRegistryStart) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "start",
			Short:         "Start the secure registry on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcRegistryStart) subCommands() []baseCmd { return s.subCmds }

func (s *spcRegistryStart) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.registry.StartRegistry(ctx, &cdspb.StartRegistryRequest{})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName, withLongTimeout())
}

// spcRegistryStop stops the registry.
type spcRegistryStop struct {
	defaultCmd
	regTarget
}

var _ baseCmd = (*spcRegistryStop)(nil)

func (s *spcRegistryStop) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "stop",
			Short:         "Stop the secure registry on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcRegistryStop) subCommands() []baseCmd { return s.subCmds }

func (s *spcRegistryStop) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.registry.StopRegistry(ctx, &cdspb.StopRegistryRequest{})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName)
}

// spcRegistryDelete deletes the registry.
type spcRegistryDelete struct {
	defaultCmd
	regTarget
	force bool
}

var _ baseCmd = (*spcRegistryDelete)(nil)

func (s *spcRegistryDelete) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "delete",
			Aliases:       []string{"rm"},
			Short:         "Delete the secure registry on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
		s.cmd.Flags().BoolVarP(&s.force, "force", "f", false, "tear the registry down even when it is in use")
	}
	return s.cmd
}

func (s *spcRegistryDelete) subCommands() []baseCmd { return s.subCmds }

func (s *spcRegistryDelete) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.registry.DeleteRegistry(ctx, &cdspb.DeleteRegistryRequest{Force: s.force})
		if err != nil {
			return err
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: resp.GetOutput()})
	}
	return callback.executeOn(hostName, withLongTimeout())
}
