package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/buildkite/ecs-run-task/runner"
	"github.com/urfave/cli/v2"
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
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Show debugging information",
		},
		&cli.StringFlag{
			Name:  "file, f",
			Usage: "Task definition file in JSON or YAML",
		},
		&cli.StringFlag{
			Name:  "task, t",
			Usage: "Existing Task definition Arn",
		},
		&cli.StringFlag{
			Name:  "name, n",
			Usage: "Task name",
		},
		&cli.StringFlag{
			Name:  "image, i",
			Usage: "Container image name to replace",
		},
		&cli.StringFlag{
			Name:  "cluster, c",
			Value: "default",
			Usage: "ECS cluster name",
		},
		&cli.StringFlag{
			Name:  "log-group, l",
			Value: "ecs-task-runner",
			Usage: "Cloudwatch Log Group Name to write logs to",
		},
		&cli.StringFlag{
			Name:  "service, s",
			Value: "",
			Usage: "service to replace cmd for",
		},
		&cli.BoolFlag{
			Name:  "fargate",
			Usage: "Specified if task is to be run under FARGATE as opposed to EC2",
		},
		&cli.StringFlag{
			Name:  "platform-version, p",
			Value: "",
			Usage: "the platform version the task should run (only for FARGATE)",
		},
		&cli.StringSliceFlag{
			Name:  "security-group",
			Usage: "Security groups to launch task in (required for FARGATE). Can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  "subnet",
			Usage: "Subnet to launch task in (required for FARGATE). Can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  "env, e",
			Usage: "An environment variable to add in the form `KEY=value` or `KEY` (shorthand for `KEY=$KEY` to pass through an env var from the current host). Can be specified multiple times",
		},
		&cli.BoolFlag{
			Name:  "inherit-env, E",
			Usage: "Inherit all of the environment variables from the calling shell",
		},
		&cli.IntFlag{
			Name:  "count, C",
			Value: 1,
			Usage: "Number of tasks to run",
		},
		&cli.IntFlag{
			Name:  "cpu",
			Value: 0,
			Usage: "Number of cpu units reserved for the container",
		},
		&cli.IntFlag{
			Name:  "memory",
			Value: 0,
			Usage: "Hard limit (in MiB) of memory available to the container",
		},
		&cli.StringFlag{
			Name:  "region, r",
			Usage: "AWS Region",
		},
		&cli.BoolFlag{
			Name:  "deregister",
			Usage: "Deregister task definition once done",
		},
	}

	app.Action = func(ctx *cli.Context) error {
		if (ctx.String("file") == "" && ctx.String("task") == "") || (ctx.String("file") != "" && ctx.String("task") != "") {
			fmt.Fprintf(os.Stderr, "ERROR: either --task or --file is needed\n\n")
			cli.ShowAppHelpAndExit(ctx, 1)
		}

		if !ctx.Bool("debug") {
			log.SetOutput(ioutil.Discard)
		}

		r := runner.New()
		r.TaskDefinitionFile = ctx.String("file")
		r.TaskDefinition = ctx.String("task")
		r.Cluster = ctx.String("cluster")
		r.Image = ctx.String("image")
		r.Service = ctx.String("service")
		r.TaskName = ctx.String("name")
		r.LogGroupName = ctx.String("log-group")
		r.Fargate = ctx.Bool("fargate")
		r.PlatformVersion = ctx.String("platform-version")
		r.SecurityGroups = ctx.StringSlice("security-group")
		r.Subnets = ctx.StringSlice("subnet")
		r.Environment = ctx.StringSlice("env")
		r.Count = ctx.Int64("count")
		r.Deregister = ctx.Bool("deregister")

		if r.Region == "" {
			r.Region = ctx.String("region")
		}

		if ctx.Bool("inherit-env") {
			for _, env := range os.Environ() {
				r.Environment = append(r.Environment, env)
			}
		}

		if args := ctx.Args(); args.Len() > 0 {
			r.Overrides = append(r.Overrides, runner.Override{
				Service: ctx.String("service"),
				Command: args.Slice(),
				Cpu: ctx.Int64("cpu"),
				Memory: ctx.Int64("memory"),
			})
		}

		if err := r.Run(context.Background()); err != nil {
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

