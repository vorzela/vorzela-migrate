package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/urfave/cli/v2"
	"github.com/vorzela/vorzela-migrate/v2/internal/gui"
	"github.com/vorzela/vorzela-migrate/v2/internal/output"
	"github.com/vorzela/vorzela-migrate/v2/internal/version"
)

// GuiCommand starts the local, token-protected web UI.
var GuiCommand = &cli.Command{
	Name:  "gui",
	Usage: "Open the local GUI (edit migrations, pick a saved database, confirm every write)",
	Description: `The GUI binds to loopback only by default, prints a one-time access URL,
stores database passwords encrypted under your user config directory, and
requires an explicit confirmation plan before every mutating action.

Examples:
  vm gui
  vm gui --no-open
  vm gui --store sqlite
  vm gui --addr 127.0.0.1:9000
  vm gui --dev-origin http://localhost:5173   # Vite dev server`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "addr",
			Usage: "loopback address to listen on",
			Value: "127.0.0.1:7845",
		},
		&cli.StringFlag{
			Name:  "store",
			Usage: "config store: yaml or sqlite",
			Value: "yaml",
		},
		&cli.StringFlag{
			Name:  "config",
			Usage: "explicit config file path (default: <config-dir>/gui.yaml or gui.db)",
		},
		&cli.StringFlag{
			Name:  "config-dir",
			Usage: "directory for GUI config, encryption key, backups and audit log",
		},
		&cli.StringFlag{
			Name:  "vm",
			Usage: "path to the vm binary (default: the running binary)",
		},
		&cli.BoolFlag{
			Name:  "no-open",
			Usage: "do not open a browser; print the access URL",
		},
		&cli.BoolFlag{
			Name:  "allow-remote",
			Usage: "allow a non-loopback bind (requires --tls-cert/--tls-key)",
		},
		&cli.StringFlag{
			Name:  "tls-cert",
			Usage: "TLS certificate for HTTPS (recommended with --allow-remote)",
		},
		&cli.StringFlag{
			Name:  "tls-key",
			Usage: "TLS private key for HTTPS",
		},
		&cli.StringFlag{
			Name:  "dev-origin",
			Usage: "extra allowed Origin during Vite development, e.g. http://localhost:5173",
		},
	},
	Action: func(c *cli.Context) error {
		storeKind := gui.StoreKind(c.String("store"))
		if storeKind != gui.StoreYAML && storeKind != gui.StoreSQLite {
			return fmt.Errorf("invalid --store %q (expected yaml or sqlite)", c.String("store"))
		}
		if c.String("tls-cert") != "" && c.String("tls-key") == "" {
			return fmt.Errorf("--tls-key is required with --tls-cert")
		}
		if c.String("tls-key") != "" && c.String("tls-cert") == "" {
			return fmt.Errorf("--tls-cert is required with --tls-key")
		}

		srv, err := gui.NewServer(gui.Options{
			Addr:        c.String("addr"),
			ConfigPath:  c.String("config"),
			StoreKind:   storeKind,
			ConfigDir:   c.String("config-dir"),
			VM:          c.String("vm"),
			NoOpen:      c.Bool("no-open"),
			AllowRemote: c.Bool("allow-remote"),
			TLSCert:     c.String("tls-cert"),
			TLSKey:      c.String("tls-key"),
			DevOrigin:   c.String("dev-origin"),
			Version:     version.CurrentVersion,
		})
		if err != nil {
			output.Error("Failed to start GUI: %v", err)
			return err
		}
		defer srv.Close()

		ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
		defer stop()
		if runtime.GOOS == "windows" {
			ctx, stop = signal.NotifyContext(c.Context, os.Interrupt)
			defer stop()
		}
		return srv.Run(ctx)
	},
}
