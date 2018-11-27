# ecs-run-task

Runs a once-off ECS task and streams the output via Cloudwatch Logs.

## Usage

```
NAME:
   ecs-run-task - run a once-off task on ECS and tail the output from cloudwatch

USAGE:
   ecs-run-task [options] [command override]

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug                        Show debugging information
   --file value, -f value         Task definition file in JSON or YAML
   --name value, -n value         Task name
   --cluster value, -c value      ECS cluster name (default: "default")
   --log-group value, -l value    Cloudwatch Log Group Name to write logs to (default: "ecs-task-runner")
   --service value, -s value      service to replace cmd for
   --fargate                      Specified if task is to be run under FARGATE as opposed to EC2
   --security-group value         Security groups to launch task in (required for FARGATE). Can be specified multiple times
   --subnet value                 Subnet to launch task in (required for FARGATE). Can be specified multiple times
   --env KEY=value, -e KEY=value  An environment variable to add in the form KEY=value or `KEY` (shorthand for `KEY=$KEY` to pass through an env var from the current host). Can be specified multiple times
   --help, -h                     show help
   --version, -v                  print the version
```

### Example

```bash
$ aws-vault exec myprofile -- ecs-run-task --file examples/helloworld/taskdefinition.json echo "Hello from Docker!"

Hello from Docker!
...
```

## IAM Permissions

The following IAM permissions are required:

```yaml
- PolicyName: ECSRunTask
  PolicyDocument:
    Version: '2012-10-17'
    Statement:
    - Effect: Allow
      Action:
        - ecs:RegisterTaskDefinition
        - ecs:RunTask
        - ecs:DescribeTasks
        - logs:DescribeLogGroups
        - logs:DescribeLogStreams
        - logs:CreateLogStream
        - logs:PutLogEvents
        - logs:FilterLogEvents
      Resource: '*'
```

## Development

We're using Go 1.11 with [modules](https://github.com/golang/go/wiki/Modules).

```bash
export GO111MODULE=on
go get -u github.com/buildkite/ecs-run-task
```
