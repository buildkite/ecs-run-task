package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildkite/ecs-run-task/runner"
	"github.com/urfave/cli"
)

var (
	Version string
)

func main() {
	app := cli.NewApp()
	app.Name = "ecs-run-task"
	app.Usage = "run a once-off task on ECS and tail the output from cloudwatch"
	app.UsageText = "ecs-run-task [options] [command override]"
	app.Version = Version

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Show debugging information",
		},
		cli.StringFlag{
			Name:  "file, f",
			Usage: "Task definition file in JSON or YAML",
		},
		cli.StringFlag{
			Name:  "name, n",
			Usage: "Task name",
		},
		cli.StringFlag{
			Name:  "cluster, c",
			Value: "default",
			Usage: "ECS cluster name",
		},
		cli.StringFlag{
			Name:  "log-group, l",
			Value: "ecs-task-runner",
			Usage: "Cloudwatch Log Group Name to write logs to",
		},
		cli.StringFlag{
			Name:  "service, s",
			Value: "",
			Usage: "service to replace cmd for",
		},
	}

	app.Action = func(ctx *cli.Context) error {
		requireFlagValue(ctx, "file")

		if _, err := os.Stat(ctx.String("file")); err != nil {
			return cli.NewExitError(err, 1)
		}

		if !ctx.Bool("debug") {
			log.SetOutput(ioutil.Discard)
		}

		r := runner.New()
		r.TaskDefinitionFile = ctx.String("file")
		r.Cluster = ctx.String("cluster")
		r.TaskName = ctx.String("name")
		r.LogGroupName = ctx.String("log-group")

		if args := ctx.Args(); len(args) > 0 {
			r.Overrides = append(r.Overrides, runner.Override{
				Service: ctx.String("service"),
				Command: args,
			})
		}

		if err := r.Run(); err != nil {
			if ec, ok := err.(cli.ExitCoder); ok {
				return ec
			}
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func requireFlagValue(ctx *cli.Context, name string) {
	if ctx.String(name) == "" {
		fmt.Fprintf(os.Stderr, "ERROR: Required flag %q isn't set\n\n", name)
		cli.ShowAppHelpAndExit(ctx, 1)
	}
}
