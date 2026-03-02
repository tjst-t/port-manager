package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tjst-t/port-manager/internal/dashboard"
	"github.com/tjst-t/port-manager/internal/db"
	"github.com/tjst-t/port-manager/internal/port"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the live dashboard HTTP server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().String("addr", "", "Listen address (default: config serve_addr or :8080)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	app, err := initApp()
	if err != nil {
		return err
	}
	defer app.DB.Close()

	addr, _ := cmd.Flags().GetString("addr")
	if addr == "" {
		addr = app.Config.Dashboard.ServeAddr
	}
	if addr == "" {
		addr = ":8080"
	}

	checker := func(l db.Lease) bool {
		if l.PID > 0 {
			return port.IsProcessAlive(l.PID)
		}
		return port.IsPortListening(l.Port)
	}

	handler := dashboard.NewHandler(app.DB, app.Services, app.Config.Proxy, checker)

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Graceful shutdown on SIGTERM/SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-ctx.Done()
		fmt.Fprintln(os.Stderr, "serve: shutting down...")
		srv.Shutdown(context.Background())
	}()

	fmt.Fprintf(os.Stderr, "serve: listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}
