package shexec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
	termprint "github.com/amadeusitgroup/cds/internal/term"
)

type RemoteExecute interface {
	execute([]any, sessionHandler) ([]string, []error)
}

type RemoteAttach interface {
	execute([]any, sessionHandler) error
}
type sessionHandler func(*ssh.Client, ...any) (string, error)

type ListenerResult struct {
	Connection net.Conn
	Error      error
}

type target interface {
	FQDN() string
	HasPassword() bool
	Password() string
	PathToPrv() string
	PathToPub() string
	Port() int
	Username() string
}

var sshClients map[target]*ssh.Client = make(map[target]*ssh.Client)

func CloseAllSSHClients() {
	for _, sshClient := range sshClients {
		if sshClient != nil {
			_ = sshClient.Close()
		}
	}
}

func RunCmds(t target) func([]ExecuteEvent) error {
	hostName := t.FQDN()
	if isLocalHost(hostName) {
		return RunLocalCmds
	}
	if err := CheckValidSSHKeyPairExistence(t); err != nil {
		return func([]ExecuteEvent) error {
			return err
		}
	}
	return func(cmds []ExecuteEvent) error {
		if len(cmds) == 0 {
			return cerr.NewError("No command to execute, incorrect internal function usage")
		}

		_, errors := ExecuteMany(UsingKey(t), cmds)
		execErrors := []error{}
		for _, err := range errors {
			switch err.(type) {
			case nil:
				continue
			case *execErr:
				execErrors = append(execErrors, err)
			case *recoverErr:
				clog.Warn("recovery error:", err)
			default:
				execErrors = append(execErrors, err)
			}
		}

		if len(execErrors) == 0 {
			return nil
		}

		errorsReport := "Execution errors report:\n"
		for index, err := range execErrors {
			errorsReport += fmt.Sprintf("error %v: %v \n", index, err.Error())
		}
		return cerr.NewError(errorsReport)
	}
}

func RunCmd(t target) func([]ExecuteEvent) (string, error) {
	hostName := t.FQDN()
	if isLocalHost(hostName) {
		return RunLocalCmdWithOutput
	}
	if err := CheckValidSSHKeyPairExistence(t); err != nil {
		return func([]ExecuteEvent) (string, error) {
			return "", err
		}
	}
	return func(cmd []ExecuteEvent) (string, error) {
		if len(cmd) == 0 {
			return "", cerr.NewError("No command to execute, incorrect internal function usage")
		}
		if len(cmd) > 1 {
			return "", cerr.NewError("Multiple commands to execute, incorrect internal function usage")
		}

		return ExecuteOne(UsingKey(t), cmd[0])
	}
}

func ExecuteOne(method RemoteExecute, cmd ExecuteEvent) (string, error) {
	var cmds []any
	cmds = append(cmds, cmd)
	output, errors := method.execute(cmds, runSession)
	errors = cg.FilterNilFromSlice(errors)
	if len(errors) > 0 {
		return "", cerr.AppendMultipleErrors(fmt.Sprintf("Failed to execute commands (%s)", cmd.Cmd()), errors)
	}

	return output[0], nil
}

func ExecuteMany(method RemoteExecute, cmds []ExecuteEvent) ([]string, []error) {
	var commands []any
	for _, cmd := range cmds {
		commands = append(commands, cmd)
	}
	return method.execute(commands, runSession)
}

func UsingPassword(h target) RemoteExecute {
	return passwordExecuteCallback(func() (target, error) { return h, nil })
}

func UsingKey(h target) RemoteExecute {
	return keyExecuteCallback(func() (target, error) { return h, nil })
}

type passwordExecuteCallback func() (target, error)

// TODO: Fix/refactor calls to event and com.Trace
func (rec passwordExecuteCallback) execute(cmds []any, sh sessionHandler) ([]string, []error) {
	host, _ := rec()
	client, err := connectSSHPassword(host)
	if err != nil {
		return nil, []error{cerr.AppendError("Couldn't connect to SSH using password", err)}
	}
	if host.HasPassword() {
		secret = host.Password()
	}

	secretValidated = true
	validationOnShexecPassword(true)

	outputs := []string{}
	errors := []error{}
	for _, cmd := range cmds {
		cmdEvent, ok := cmd.(ExecuteEvent)
		if !ok {
			errors = append(errors, cerr.NewError(
				fmt.Sprintf("variable cmd of type %T which does not implement interface ExecuteEvent", cmd)),
			)
			// TODO:FixMe: Notify Observer using a default Notification with terminate action
			return nil, errors
		}
		// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Executing '%v' - command: '%v'`, cmdEvent.Description(), cmdEvent.Cmd()),
		// 	event.KNotifTypeInfo)
		out, errExec := sh(client, cmdEvent.Cmd())
		if errExec != nil {
			if !cmdEvent.CarryOn() {
				errors = append(errors, &execErr{Message: errExec.Error()})
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Failed to execute '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeError)
				return nil, errors
			}
			if !cmdEvent.Recoverable() {
				clog.Warn("Couldn't recover error:", errExec)
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`No recover found for '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeWarn)
			}
			if _, errRecover := sh(client, cmdEvent.Recover()); errRecover != nil {
				errors = append(errors, errExec, errRecover)
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Failed to recover '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeWarn,
				// )
			}
			// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Succeeded to recover '%v'`, cmdEvent.Cmd()),
			// 	event.KNotifTypeInfo)
		}
		// clog.Trace(fmt.Sprintf("notify: %v", cmdEvent.Cmd()))
		// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Succeeded to execute '%v'`, cmdEvent.Description()),
		// 	event.KNotifTypeInfo)
		outputs = append(outputs, out)
		errors = append(errors, errExec)
	}
	return outputs, errors
}

type keyExecuteCallback func() (target, error)

func (kec keyExecuteCallback) execute(cmds []any, sh sessionHandler) ([]string, []error) {
	host, _ := kec()

	client, connectErr := connectSSHKey(host)

	if connectErr != nil {
		return nil, []error{connectErr}
	}

	outputs := []string{}
	errors := []error{}
	for _, cmd := range cmds {
		cmdEvent, ok := cmd.(ExecuteEvent)
		if !ok {
			errors = append(errors, cerr.NewError(
				fmt.Sprintf("variable cmd of type %T which does not implement interface ExecuteEvent", cmd)),
			)
			// TODO:FixMe: Notify Observer
			return nil, errors
		}
		tp := termprint.New()
		// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Executing '%v' - command: '%v'`, cmdEvent.Description(), cmdEvent.Cmd()),
		// 	event.KNotifTypeInfo)
		// we don't defer tpExitCallback() since it fail when there are multiple cmds
		tpExitCallback := tp.Printer(cmdEvent.Description())
		out, errExec := sh(client, cmdEvent.Cmd())
		if errExec != nil {
			tp.Status = termprint.KFail
			if !cmdEvent.CarryOn() {
				errors = append(errors, &execErr{Message: errExec.Error()})
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Failed to execute '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeError)
				tpExitCallback()
				return nil, errors
			}
			if !cmdEvent.Recoverable() {
				clog.Warn("Couldn't recover error:", errExec)
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`No recover found for '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeWarn)
			}
			if _, errRecover := sh(client, cmdEvent.Recover()); errRecover != nil {
				errors = append(errors, errExec, errRecover)
				// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Failed to recover '%v'`, cmdEvent.Cmd()),
				// 	event.KNotifTypeWarn)
			}
			// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Succeeded to recover '%v'`, cmdEvent.Cmd()),
			// 	event.KNotifTypeInfo)
		} else {
			tp.Status = termprint.KSuccess
			// event.NotifyChan <- cmdEvent.Notify(fmt.Sprintf(`Succeeded to execute '%v'`, cmdEvent.Description()),
			// 	event.KNotifTypeInfo)
		}
		outputs = append(outputs, out)
		errors = append(errors, errExec)
		tpExitCallback()
	}
	return outputs, errors
}

func runSession(client *ssh.Client, cmd ...any) (string, error) {
	commands := cmd[:]
	if len(commands) == 0 || len(commands) > 1 {
		return "", cerr.NewError("not appropriate nb of commands")
	}
	command, ok := cmd[0].(string)
	if !ok {
		return "", cerr.NewError("command is not a string")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", cerr.AppendError("Failed to create session", err)
	}
	defer func() {
		_ = session.Close()
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		return stderr.String(), cerr.AppendError(
			fmt.Sprintf("Failed to run command (%s):\nstdout: %s\nstderr: %s", command, stdout.String(), stderr.String()),
			err)
	}

	return stdout.String(), nil
}

// Choose an authentication method for the given private key.
func getAuthMethod(privateKeyPath string) (ssh.AuthMethod, error) {
	key, err := cos.ReadFile(privateKeyPath)
	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Cannot read private key file %s", privateKeyPath), err)
	}

	// Parse the private key.
	signer, err := ssh.ParsePrivateKey(key)

	// Did we get a PassphraseMissingError?
	var passphraseMissingError *ssh.PassphraseMissingError
	if errors.As(err, &passphraseMissingError) {
		// This private key is protected with a passphrase.  Let's
		// try harder.

		agent_socket_path, have_agent := os.LookupEnv("SSH_AUTH_SOCK")
		if have_agent {
			// If there is a SSH agent, use it.
			agent_conn, err := net.Dial("unix", agent_socket_path)
			if err != nil {
				return nil, cerr.AppendError(fmt.Sprintf("Cannot connect to SSH agent at %s", agent_socket_path), err)
			}
			agentClient := agent.NewClient(agent_conn)
			return ssh.PublicKeysCallback(agentClient.Signers), nil
		}

		// Let's prompt for the passphrase from the user.
		// But we can do that only if we have a terminal.
		stdin := int(syscall.Stdin) // Convert to int in case of Windows Handle
		if !term.IsTerminal(stdin) {
			return nil, cerr.AppendError(fmt.Sprintf("%s requires a passphrase, and we don't have a terminal to prompt for the passphrase", privateKeyPath), err)
		}

		fmt.Print("Passphrase: ")
		passphrase, err := term.ReadPassword(stdin)
		if err != nil {
			return nil, cerr.AppendError("Cannot read passphrase from stdin", err)
		}
		signer, err := ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
		if err != nil {
			return nil, cerr.AppendError(fmt.Sprintf("Cannot parse private key with passphrase %s", privateKeyPath), err)
		}
		return ssh.PublicKeys(signer), nil
	}

	if err != nil {
		// There was some error trying to parse the private key.
		return nil, cerr.AppendError(fmt.Sprintf("Could not parse SSH private key %s", privateKeyPath), err)
	}

	// Parsing the private key has succeeded, we have a signer.
	return ssh.PublicKeys(signer), nil
}

// connect to a host and returns an SSH client, it's up to the caller to close the client !
func connectSSHKey(host target) (*ssh.Client, error) {
	if client, ok := sshClients[host]; ok && client != nil {
		return client, nil
	}

	authMethod, err := getAuthMethod(host.PathToPrv())
	if err != nil {
		errMessage := "Could not find a suitable authentication method"
		if missingKeyPairErr := CheckValidSSHKeyPairExistence(host); missingKeyPairErr != nil {
			errMessage += fmt.Sprintf("\n %s", missingKeyPairErr.Error())
		}
		return nil, cerr.AppendError(errMessage, err)
	}

	// Create client config
	config := &ssh.ClientConfig{
		User: host.FQDN(),
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: provideHostKeyCallback(),
	}

	// default to 22 unless specified
	sshPort := 22
	if host.Port() != 0 {
		sshPort = host.Port()
	}

	clog.Debug(fmt.Sprintf("SSH - CONNECTING TO HOST AS (%s@%s with key %s)", host.Username(), host.FQDN(), host.PathToPrv()))
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host.FQDN(), sshPort), config)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to open SSH connection to %s@%v:22", host.Username(), host.FQDN()), err)
	}
	registerNewSSHClient(host, client)
	return client, nil
}

func connectSSHPassword(host target) (*ssh.Client, error) {
	if client, ok := sshClients[host]; ok && client != nil {
		return client, nil
	}
	hostname := host.FQDN()
	username := host.Username()

	var authMethod ssh.AuthMethod
	if host.HasPassword() {
		authMethod = ssh.Password(host.Password())
	} else {
		authMethod = ssh.RetryableAuthMethod(ssh.PasswordCallback(retryableGetSecret(hostname)), maxPasswordTries)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			authMethod,
		},
		HostKeyCallback: provideHostKeyCallback(),
	}
	// default to 22 unless specified
	sshPort := 22
	if host.Port() != 0 {
		sshPort = host.Port()
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host.FQDN(), sshPort), config)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("unable to connect to %s@%v:22", username, hostname), err)
	}

	registerNewSSHClient(host, client)
	return client, nil
}

func registerNewSSHClient(host target, client *ssh.Client) {
	sshClients[host] = client
}

func CopyKey(method RemoteExecute, src, dst string) error {
	if err := CopyFile(method, src, dst); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to copy SSH key (%s) to remote (%s)", src, dst), err)
	}
	_, remoteDst := InRemotePath(src, dst)
	var cmds []any
	copyKeyCmd := `mkdir -p ~/.ssh && cat %v >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && chmod 700 ~/.ssh && rm %v`
	cmd := &DefaultShEvent{
		ExeCmd:         fmt.Sprintf(copyKeyCmd, remoteDst, remoteDst),
		DescriptionCmd: "Copying SSH public key",
	}
	cmds = append(cmds, cmd)

	// WARNING this will open a new SSH session in addition of the one created by CopyFile*
	// hence the need for global secret
	_, errors := method.execute(cmds, runSession)
	errors = cg.FilterNilFromSlice(errors)
	if len(errors) > 0 {
		return cerr.AppendMultipleErrors(fmt.Sprintf("Failed to execute commands over session (%v)", cmds), errors)
	}

	return nil
}

func CopyFile(method RemoteExecute, src, dst string) error {
	var dummy []any
	dummy = append(dummy, &CopyFileEvent{
		FileName:       src,
		DefaultShEvent: DefaultShEvent{ExeCmd: "copy of file", DescriptionCmd: "Copying file to host"},
	})
	fileName, remoteDst := InRemotePath(src, dst)

	_, errs := method.execute(dummy,
		func(client *ssh.Client, a ...any) (string, error) {
			session, errS := client.NewSession()
			if errS != nil {
				return "", cerr.AppendError("Failed to create a session", errS)
			}
			defer func() {
				_ = session.Close()
			}()

			file, errF := cos.Fs.Open(src)
			if errF != nil {
				return "", cerr.AppendError(fmt.Sprintf("Failed to read source file (%s)", src), errF)
			}
			defer func() {
				_ = file.Close()
			}()

			stat, _ := file.Stat()

			wg := sync.WaitGroup{}

			wg.Add(1)
			go func() {
				defer wg.Done()
				hostIn, errStdin := session.StdinPipe()
				if errStdin != nil {
					clog.Error("unable to open a pipe:", errStdin)
				}
				defer func() {
					_ = hostIn.Close()
				}()

				_, _ = fmt.Fprintf(hostIn, "C0600 %d %s\n", stat.Size(), fileName)

				_, err := io.Copy(hostIn, file)
				if err != nil {
					clog.Error("", err)
				}
				_, _ = fmt.Fprint(hostIn, "\x00")

			}()

			err := session.Run(fmt.Sprintf("/usr/bin/scp -t %v", remoteDst))
			if err != nil {
				return "", err
			}
			wg.Wait()
			return "", nil
		})

	errs = cg.FilterNilFromSlice(errs)
	if len(errs) > 0 {
		return cerr.AppendMultipleErrors(fmt.Sprintf("Failed to copy file (%s) to %s", src, dst), errs)
	}

	return nil
}

func DownloadFile(method RemoteExecute, src, dst string) error {
	var dummy []any
	dummy = append(dummy, &CopyFileEvent{
		FileName:       dst,
		DefaultShEvent: DefaultShEvent{ExeCmd: "Download of file", DescriptionCmd: "Downloading file to host"},
	})

	_, errs := method.execute(dummy,
		func(client *ssh.Client, a ...any) (string, error) {
			file, errF := cos.Fs.Create(dst)
			if errF != nil {
				return "", cerr.AppendError(fmt.Sprintf("Failed to create destination file (%s)", dst), errF)
			}
			defer func() {
				_ = file.Close()
			}()

			session, errS := client.NewSession()
			if errS != nil {
				return "", cerr.AppendError("Failed to create a session", errS)
			}
			defer func() {
				_ = session.Close()
			}()

			data, err := session.Output(fmt.Sprintf("cat %v", src))
			if err != nil {
				return "", err
			}
			if _, err := file.Write(data); err != nil {
				clog.Warn("", err)
			}
			return "", nil
		})

	errs = cg.FilterNilFromSlice(errs)
	if len(errs) > 0 {
		return cerr.AppendMultipleErrors(fmt.Sprintf("Failed to copy file (%s) to %s", src, dst), errs)
	}

	return nil
}

func ForwardPort(host target, local string, remote string, timeout time.Duration) error {
	localListener, err := net.Listen("tcp", local)
	if err != nil {
		return err
	}
	client, connectErr := connectSSHKey(host)

	if connectErr != nil {
		return connectErr
	}
	localConn := make(chan ListenerResult, 1)
	keepAlive := true
	clog.Info(fmt.Sprintf("Port forward server will time out if %v spent without requests", timeout))
	for keepAlive {
		go func() {
			local, err := localListener.Accept()
			localConn <- ListenerResult{
				Connection: local,
				Error:      err,
			}
		}()
		select {
		case <-time.After(timeout):
			keepAlive = false
			clog.Warn(fmt.Sprintf("Server timed out because no requests has be received for %v", timeout))
		case localConn := <-localConn:
			if localConn.Error != nil {
				return err
			}
			go forwardConnToRemote(localConn.Connection, client, remote)
		}
	}
	return nil
}

// TODO:Analyse: this is started from a goroutine -> should use an error channel instead of returning an error that is never going to be used

func forwardConnToRemote(conn net.Conn, client *ssh.Client, remote string) {
	sendDone := make(chan bool)
	receiveDone := make(chan bool)
	sshEndConn, err := client.Dial("tcp", remote)
	defer func() {
		if sshEndConn != nil {
			_ = sshEndConn.Close()
		}
	}()
	if err != nil {
		clog.Warn(fmt.Sprintf("Couldn't contact %v on ssh server", remote), err)
		return
	}

	go func() {
		defer func() { sendDone <- true }()
		_, err = io.Copy(sshEndConn, conn)
		if err != nil {
			clog.Debug(fmt.Sprintf("io.copy failed : %v", err))
			return
		}
	}()

	// Copy sshConn.Reader to localConn.Writer
	go func() {
		defer func() { receiveDone <- true }()
		_, err = io.Copy(conn, sshEndConn)
		if err != nil {
			clog.Debug(fmt.Sprintf("io.copy failed : %v", err))
			return
		}
	}()

	<-sendDone
	<-receiveDone
}
