//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	childEnvKey	= "__CLUSTER_PROBE_CHILD__"
	childEnvValue	= "1"
)

type Executor struct {
	verbose bool
}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) SetVerbose(v bool) {
	e.verbose = v
}

func (e *Executor) IsSupported() bool {

	if IsChild() {
		return true
	}

	if os.Geteuid() == 0 {
		return true
	}

	return false
}

func (e *Executor) RequiresRoot() bool {
	return os.Geteuid() != 0
}

func IsChild() bool {
	return os.Getenv(childEnvKey) == childEnvValue
}

func (e *Executor) Run(fn func() error) error {
	if IsChild() {

		return e.runChild(fn)
	}

	return e.execChild()
}

func (e *Executor) execChild() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = append(os.Environ(), childEnvKey+"="+childEnvValue)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:	syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,

		Unshareflags:	syscall.CLONE_NEWNS,
	}

	if e.verbose {
		fmt.Fprintln(os.Stderr, "[container] Re-executing in isolated namespaces...")
	}

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {

			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("child process failed: %w", err)
	}

	return nil
}

func (e *Executor) runChild(fn func() error) error {
	if e.verbose {
		fmt.Fprintln(os.Stderr, "[container] Running in isolated namespace")
	}

	if err := syscall.Sethostname([]byte("cluster-probe")); err != nil {

		if e.verbose {
			fmt.Fprintf(os.Stderr, "[container] Warning: could not set hostname: %v\n", err)
		}
	}

	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[container] Warning: could not make root private: %v\n", err)
		}
	}

	hostPath := "/host"
	if err := os.MkdirAll(hostPath, 0755); err != nil {
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[container] Warning: could not create /host: %v\n", err)
		}
	}

	if err := syscall.Mount("/", hostPath, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[container] Warning: could not bind mount host root: %v\n", err)
		}
	} else if e.verbose {
		fmt.Fprintln(os.Stderr, "[container] Host filesystem mounted at /host")
	}

	e.ensureHostPaths()

	return fn()
}

func (e *Executor) ensureHostPaths() {

	paths := []string{
		"/host/root/.kube/config",
	}

	if user := os.Getenv("USER"); user != "" && user != "root" {
		paths = append(paths, filepath.Join("/host/home", user, ".kube/config"))
	}
	if home := os.Getenv("HOME"); home != "" {
		paths = append(paths, filepath.Join("/host", home, ".kube/config"))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if e.verbose {
				fmt.Fprintf(os.Stderr, "[container] Found: %s\n", p)
			}
		}
	}
}
