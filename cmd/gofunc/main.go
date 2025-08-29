package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/andrebq/gofunc/installers"
	"github.com/andrebq/gofunc/pkg/uploader"

	"github.com/andrebq/gofunc/server"
	"github.com/urfave/cli/v2"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	app := newApp()
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func newApp() *cli.App {
	app := cli.NewApp()
	app.Name = "gofunc"
	app.Usage = "A simple CLI for Go lambdas"
	app.Commands = []*cli.Command{
		serveCmd(),
		uploadCmd(),
		installCmd(),
	}
	return app
}

func installCmd() *cli.Command {
	return &cli.Command{
		Name:  "install",
		Usage: "Install the GoFunc server in your scheduler of preference",
		Subcommands: []*cli.Command{
			installK8SCmd(),
		},
	}
}

func installK8SCmd() *cli.Command {
	var yamlFile string
	var namespace string
	var name string = "gofunc"
	var image string = "andrebq/gofunc:latest"
	return &cli.Command{
		Name:  "k8s",
		Usage: "Renders a GoFunc server manifest on Kubernetes, you must provide a Yaml file to be used as template (or stdin).\nThe template is assumed to be trustworthy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "yaml-template",
				Usage:       "Path to yaml template, use '-' for stdin",
				Destination: &yamlFile,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "namespace",
				Usage:       "Kubernetes namespace",
				Destination: &namespace,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Kubernetes resource name",
				Value:       name,
				Destination: &name,
			},
			&cli.StringFlag{
				Name:        "image",
				Usage:       "Container image",
				Value:       image,
				Destination: &image,
			},
		},
		Action: func(ctx *cli.Context) error {
			return installers.K8S(ctx.Context, ctx.App.Writer, yamlFile, name, namespace, image)
		},
	}
}

func uploadCmd() *cli.Command {
	var dir string = "."
	var name string = ""
	var addr string = "http://127.0.0.1:9000"

	return &cli.Command{
		Name:  "upload",
		Usage: "Zip a directory (respecting .gofaasignore and .gitignore) and upload it to the server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "dir",
				Usage:       "Directory to upload",
				Destination: &dir,
				Value:       dir,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "name",
				Usage:       "Function name (used in the upload path)",
				Destination: &name,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "addr",
				Usage:       "Server address (including scheme and port)",
				Destination: &addr,
				Value:       addr,
			},
		},
		Action: func(ctx *cli.Context) error {
			return uploader.Upload(ctx.Context, addr, name, dir)
		},
	}
}

func serveCmd() *cli.Command {
	var bindPort uint = 9000
	var bindAddr string = "0.0.0.0"
	var baseDir string
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the GoFunc server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "address",
				Usage:       "Address to bind the server",
				Destination: &bindAddr,
				Value:       bindAddr,
				EnvVars:     []string{"BIND_ADDR"},
			},
			&cli.UintFlag{
				Name:        "port",
				Usage:       "Port to bind the server",
				EnvVars:     []string{"BIND_PORT"},
				Destination: &bindPort,
				Value:       bindPort,
			},
			&cli.StringFlag{
				Name:        "base-dir",
				Usage:       "Base directory for functions",
				Destination: &baseDir,
				Value:       baseDir,
				EnvVars:     []string{"BASE_DIR"},
				Required:    true,
			},
		},
		Action: func(ctx *cli.Context) error {
			return server.Run(ctx.Context, bindAddr, bindPort, baseDir)
		},
	}
}
