# ecs-run-task

Runs a once-off ECS task and streams the output via Cloudwatch Logs.

Recommended for use with [aws-vault][] for authentication.

## Usage

```bash
$ aws-vault exec myprofile -- ecs-run-task --file examples/helloworld/taskdefinition.json echo "Hello from Docker!"

Hello from Docker!
...
```

## Installation

```bash
go get github.com/buildkite/ecs-run-task
```

[aws-vault]: https://github.com/99designs/aws-vault

## Dependency management

We're using [govendor](https://github.com/kardianos/govendor) to manage our Go dependencies. Install it with:

```bash
go get github.com/kardianos/govendor
```

If you introduce a new package, just add the import to your source file and run:

```bash
govendor fetch +missing
```

Or explicitly fetch it with a version using:

```bash
govendor fetch github.com/buildkite/go-buildkite@v2.0.0
```
