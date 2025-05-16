package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/amadeusitgroup/cds/internal/agent"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var logger *slog.Logger

func init() {

	logger = createAgentLogger()

	if err := config.InitAgentConfig(); err != nil {
		clog.Error("Failed to initialize cds' agent configuration", err)
		os.Exit(1)
	}
}

var (
	port = flag.Int("port", 8087, "The server port")
)

func main() {

	lis, err := listener()

	if err != nil {
		clog.Error(fmt.Sprintf("Failed to listen to port %d", *port), err)
	}

	agentTLSConfig, errTLS := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CertFile:      cdstls.AgentServerCertFilePath,
		KeyFile:       cdstls.AgentServerKeyFilePath,
		CAFile:        cdstls.CAFilePath,
		ServerAddress: lis.Addr().String(),
		Server:        true, // Setting Server attribute to true enable authentication of clients at server side. Mutual TLS authentication use case
	})

	if errTLS != nil {
		clog.Error("Failed to setup TLS config", err)
		return
	}
	agentCreds := credentials.NewTLS(agentTLSConfig)
	agentSrv, _ := agent.NewAgent(
		agent.NewConfig(
			agent.WithLogger(logger),
		),
		grpc.Creds(agentCreds),
	)

	clog.Info(fmt.Sprintf("server listening at %v", lis.Addr()))

	done := make(chan any)
	var wg sync.WaitGroup

	serverShutdown := func() <-chan any {
		serverShutdown := make(chan any)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(serverShutdown)
			// TL;DR: Serve method is a blocking call.
			// From gRPC API doc: Serve accepts incoming connections on the listener lis, creating a new
			// ServerTransport and service goroutine for each. The service goroutines
			// read gRPC requests and then call the registered handlers to reply to them.
			// Serve returns when lis.Accept fails with fatal errors.  lis will be closed when
			// this method returns.
			if err := agentSrv.Serve(lis); err != nil {
				clog.Error("Failed to start server", err)
				return
			}
			clog.Info("gRPC server shutdown completed")
		}()
		return serverShutdown

	}()

	serverPreShutdown := func(done <-chan any) <-chan any {
		serverPreShutdown := make(chan any)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(serverPreShutdown)

			<-done
			clog.Info("Graceful gRPC server shutdown initiated...")
			agentSrv.GracefulStop()

		}()
		return serverPreShutdown
	}(done)

	signalReceived := func(done <-chan any) <-chan any {
		signalReceived := make(chan any)

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(signalReceived)
			signalChan := make(chan os.Signal, 1)
			defer close(signalChan)

			signal.Notify(signalChan,
				syscall.SIGINT,
				syscall.SIGQUIT,
				syscall.SIGTERM,
			)
			signal.Ignore(syscall.SIGHUP)

			select {
			case <-done:
				clog.Info("Signals goroutine done.")
			case sig := <-signalChan:
				clog.Info(fmt.Sprintf("Got signal: %v", sig))
			}
		}()
		return signalReceived
	}(done)

	output(lis.Addr().String())

	// Block main goroutine. It waits for an event to happen on the list of watched channels
	select {
	case <-serverPreShutdown:
	case <-serverShutdown:
	case <-signalReceived:
	}

	// Dispatch shutdown information to running child goroutines
	close(done)

	// Wait for notified goroutines to end gracefully
	wg.Wait()
}

func output(content string) {
	fmt.Println(content)
}

func createAgentLogger() *slog.Logger {
	return slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				Level:       slog.LevelDebug,
				ReplaceAttr: cerr.ReplaceAttrErr,
			},
		),
	)
}

func listener() (net.Listener, error) {

	if systemd.New(systemd.WithTarget(host.New(host.WithName(cg.KLocalhost)))).In() {
		// systemd use case
		listeners, err := systemd.Listeners()
		if err != nil {
			return nil, cerr.AppendError("cannot retrieve listeners", err)
		}

		if len(listeners) != 1 {
			return nil, cerr.NewError(fmt.Sprintf("expected 1 socket activation listener but got %d", len(listeners)))
		}

		lis := listeners[0]

		if tcpAddr, ok := lis.Addr().(*net.TCPAddr); ok {
			clog.Debug(fmt.Sprintf("server listening on port %d", tcpAddr.Port))
		}
		return lis, err
	}

	// bootstrap usecase
	flag.Parse()
	return net.Listen("tcp", fmt.Sprintf(":%d", *port))
}
