package shexec

const (
	KChangeDirectory       = "cd %s"
	KListFilesAndDir       = "ls %s"
	KDisplayFileContent    = "cat %s"
	KCurrentPath           = "pwd"
	KMkDirWithIntermediate = "mkdir -p %s"
	KEchoHome              = "echo $HOME"
	KRunBashScriptFromHome = "bash ~/%s"
	KRemoveFolderWithForce = "rm -fr %s"
	KMkdirTmp              = "mktemp -d -p %s"
	KEnablePodmanSocket    = "systemctl enable --user podman.socket --now"
)
