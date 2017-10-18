# ecs-run-task

Runs a once-off ECS task and streams the output via Cloudwatch Logs.

Recommended for use with [aws-vault][] for authentication.

## Usage

```bash
$ aws-vault exec myprofile -- ecs-run-task --file examples/helloworld/taskdefinition.json

Hello from Docker!
...
```

## Installation

```bash
go get github.com/buildkite/ecs-run-task
```

[aws-vault]: https://github.com/99designs/aws-vault
