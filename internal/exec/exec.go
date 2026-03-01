package exec

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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

// Run executes a command with "{}" in args replaced by the port number.
// It propagates SIGTERM and SIGINT to the child process and returns
// the child's exit code as an ExitError.
func Run(command string, args []string, port int) error {
	replaced := ReplacePortPlaceholder(args, port)

	cmd := exec.Command(command, replaced...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Set process group so signals can be forwarded
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting command: %w", err)
	}

	// Set up signal forwarding
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	for {
		select {
		case sig := <-sigCh:
			// Forward signal to child process
			if cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case err := <-done:
			signal.Stop(sigCh)
			return err
		}
	}
}
