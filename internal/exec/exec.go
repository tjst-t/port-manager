package exec

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ReplacePortPlaceholder replaces all occurrences of "{}" in args with the port number.
func ReplacePortPlaceholder(args []string, port int) []string {
	portStr := strconv.Itoa(port)
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = strings.ReplaceAll(arg, "{}", portStr)
	}
	return result
}

// Runner manages a child process with signal forwarding.
type Runner struct {
	cmd   *exec.Cmd
	done  chan error
	sigCh chan os.Signal
}

// Start launches a command with "{}" in args replaced by the port number.
// It returns a Runner that can be used to wait for startup confirmation and
// final completion. The caller must call either WaitStartup+Wait or Wait.
func Start(command string, args []string, port int) (*Runner, error) {
	replaced := ReplacePortPlaceholder(args, port)

	cmd := exec.Command(command, replaced...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	r := &Runner{
		cmd:   cmd,
		done:  make(chan error, 1),
		sigCh: make(chan os.Signal, 1),
	}

	signal.Notify(r.sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		r.done <- cmd.Wait()
	}()

	return r, nil
}

// WaitStartup waits for the given timeout duration. If the process exits
// during this period, it returns (false, err) where err is non-nil for
// abnormal exits or nil for exit code 0. If the process is still alive
// after the timeout, it returns (true, nil).
func (r *Runner) WaitStartup(timeout time.Duration) (alive bool, err error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case sig := <-r.sigCh:
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Signal(sig)
			}
		case err := <-r.done:
			// Process exited during startup window
			signal.Stop(r.sigCh)
			return false, err
		case <-timer.C:
			// Process still alive after timeout
			return true, nil
		}
	}
}

// Wait blocks until the child process exits, forwarding signals.
func (r *Runner) Wait() error {
	for {
		select {
		case sig := <-r.sigCh:
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Signal(sig)
			}
		case err := <-r.done:
			signal.Stop(r.sigCh)
			return err
		}
	}
}

// Run executes a command with "{}" in args replaced by the port number.
// It propagates SIGTERM and SIGINT to the child process and returns
// the child's exit code as an ExitError.
func Run(command string, args []string, port int) error {
	r, err := Start(command, args, port)
	if err != nil {
		return err
	}
	return r.Wait()
}
