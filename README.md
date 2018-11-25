# ecs-run-task

Runs a once-off ECS task and streams the output via Cloudwatch Logs.

## Usage

```bash
$ aws-vault exec myprofile -- ecs-run-task --file examples/helloworld/taskdefinition.json echo "Hello from Docker!"

Hello from Docker!
...
```

## Development

We're using Go 1.11 with GO111MODULE=on.

```bash
export GO111MODULE=on
go get -u github.com/buildkite/ecs-run-task
```
