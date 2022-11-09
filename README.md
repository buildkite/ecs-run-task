# ecs-run-task [![Build status](https://badge.buildkite.com/9c381bcb8ed121b115d89e2940a6daeedf0126f21f39ec69bd.svg?branch=master)](https://buildkite.com/buildkite/ecs-run-task)

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
   --task value, -t value         Existing Task definition Arn
   --name value, -n value         Task name
   --image value, -i value        Container image name to replace
   --cluster value, -c value      ECS cluster name (default: "default")
   --log-group value, -l value    Cloudwatch Log Group Name to write logs to (default: "ecs-task-runner")
   --service value, -s value      service to replace cmd for
   --fargate                      Specified if task is to be run under FARGATE as opposed to EC2
   --platform-version value       the platform version the task should run (only for FARGATE)
   --security-group value         Security groups to launch task in (required for FARGATE). Can be specified multiple times
   --subnet value                 Subnet to launch task in (required for FARGATE). Can be specified multiple times
   --env KEY=value, -e KEY=value  An environment variable to add in the form KEY=value or `KEY` (shorthand for `KEY=$KEY` to pass through an env var from the current host). Can be specified multiple times
   --inherit-env, -E              Inherit all of the environment variables from the calling shell
   --count value, -C value        Number of tasks to run (default: 1)
   --cpu value                    Number of cpu units reserved for the container (default: 0)
   --memory value                 Hard limit (in MiB) of memory available to the container (default: 0)
   --region value, -r value       AWS Region
   --deregister                   Deregister task definition once done
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
        - ecs:DeregisterTaskDefinition
        - ecs:RunTask
        - ecs:DescribeTasks
        - logs:DescribeLogGroups
        - logs:DescribeLogStreams
        - logs:CreateLogStream
        - logs:PutLogEvents
        - logs:FilterLogEvents
      Resource: '*'
```
