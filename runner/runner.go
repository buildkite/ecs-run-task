package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
)

// Override ..
type Override struct {
	Service string
	Command []string
}

// Runner ..
type Runner struct {
	Service           string
	TaskName          string
	TaskDefinition    string
	TaskDefinitionArn string
	Cluster           string
	LogGroupName      string
	Region            string
	Config            *aws.Config
	Overrides         []Override
	Fargate           bool
	SecurityGroups    []string
	Subnets           []string
	Environment       []string
	Count             int64
	Deregister        bool

	ecsconn *ecs.ECS
	cwlconn *cloudwatchlogs.CloudWatchLogs
}

// New creates a new instance of a runner
func New() *Runner {
	region := os.Getenv("AWS_REGION")
	config := aws.NewConfig()
	sess := session.Must(session.NewSession(config.WithRegion(region)))

	return &Runner{
		Region:  region,
		Config:  config,
		ecsconn: ecs.New(sess),
		cwlconn: cloudwatchlogs.New(sess),
	}
}

// Run runs the runner
func (r *Runner) Run(ctx context.Context) error {

	input := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(r.TaskDefinition),
	}
	result, err := r.ecsconn.DescribeTaskDefinition(input)
	if err != nil {
		return handleAwsError(err)
	}

	taskDefinition := result.TaskDefinition
	// taskDefinitionInput, err := parser.Parse(r.TaskDefinitionFile, os.Environ())
	// if err != nil {
	// 	return err
	// }

	streamPrefix := r.TaskName
	if streamPrefix == "" {
		streamPrefix = fmt.Sprintf("run_task_%d", time.Now().Nanosecond())
	}

	// if err := createLogGroup(sess, r.LogGroupName); err != nil {
	// 	return err
	// }

	// log.Printf("Setting tasks to use log group %s", r.LogGroupName)
	// for _, def := range taskDefinitionInput.ContainerDefinitions {
	// 	def.LogConfiguration = &ecs.LogConfiguration{
	// 		LogDriver: aws.String("awslogs"),
	// 		Options: map[string]*string{
	// 			"awslogs-group":         aws.String(r.LogGroupName),
	// 			"awslogs-region":        aws.String(r.Region),
	// 			"awslogs-stream-prefix": aws.String(streamPrefix),
	// 		},
	// 	}
	// }

	//svc := ecs.New(sess)

	// log.Printf("Registering a task for %s", *taskDefinitionInput.Family)
	// resp, err := svc.RegisterTaskDefinition(taskDefinitionInput)
	// if err != nil {
	// 	return err
	// }
	//
	// taskDefinition := fmt.Sprintf("%s:%d",
	// 	*resp.TaskDefinition.Family, *resp.TaskDefinition.Revision)
	//
	// defer func() {
	// 	if !r.Deregister {
	// 		return
	// 	}
	//
	// 	log.Printf("Deregistering task %s", taskDefinition)
	// 	_, err := svc.DeregisterTaskDefinition(&ecs.DeregisterTaskDefinitionInput{
	// 		TaskDefinition: &taskDefinition,
	// 	})
	// 	if err != nil {
	// 		log.Printf("Failed to deregister task %s: %s", taskDefinition, err.Error())
	// 		return
	// 	}
	// 	log.Printf("Successfully deregistered task %s", taskDefinition)
	// }()

	runTaskInput := &ecs.RunTaskInput{
		TaskDefinition: aws.String(r.TaskDefinition),
		Cluster:        aws.String(r.Cluster),
		Count:          aws.Int64(r.Count),
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: []*ecs.ContainerOverride{},
		},
	}
	if r.Fargate {
		runTaskInput.LaunchType = aws.String("FARGATE")
	}
	if len(r.Subnets) > 0 || len(r.SecurityGroups) > 0 {
		runTaskInput.NetworkConfiguration = &ecs.NetworkConfiguration{
			AwsvpcConfiguration: &ecs.AwsVpcConfiguration{
				Subnets:        awsStrings(r.Subnets),
				AssignPublicIp: aws.String("ENABLED"),
				SecurityGroups: awsStrings(r.SecurityGroups),
			},
		}
	}

	env, err := awsKeyValuePairForEnv(os.LookupEnv, r.Environment)
	if err != nil {
		return err
	}

	for _, override := range r.Overrides {
		if len(override.Command) > 0 {
			cmds := []*string{}

			if override.Service == "" {
				if len(taskDefinition.ContainerDefinitions) != 1 {
					return fmt.Errorf("No service provided for override and can't determine default service with %d container definitions", len(taskDefinition.ContainerDefinitions))
				}

				override.Service = *taskDefinition.ContainerDefinitions[0].Name
				log.Printf("Assuming override applies to '%s'", override.Service)
			}

			for _, command := range override.Command {
				cmds = append(cmds, aws.String(command))
			}

			runTaskInput.Overrides.ContainerOverrides = append(
				runTaskInput.Overrides.ContainerOverrides,
				&ecs.ContainerOverride{
					Command:     cmds,
					Name:        aws.String(override.Service),
					Environment: env,
				},
			)
		}
	}

	// If no overrides specified, but Environment variables were - should still be overridden
	if len(r.Overrides) == 0 {
		runTaskInput.Overrides.ContainerOverrides = append(
			runTaskInput.Overrides.ContainerOverrides,
			&ecs.ContainerOverride{
				Name:        taskDefinition.ContainerDefinitions[0].Name,
				Environment: env,
			},
		)
	}

	log.Printf("Running task %s", taskDefinition)
	runResp, err := r.ecsconn.RunTask(runTaskInput)
	if err != nil {
		return fmt.Errorf("Unable to run task: %s", err.Error())
	}

	//cwl := cloudwatchlogs.New(sess)
	var wg sync.WaitGroup

	// spawn a log watcher for each container
	for _, task := range runResp.Tasks {
		for _, container := range task.Containers {
			containerID := path.Base(*container.ContainerArn)
			watcher := &logWatcher{
				LogGroupName:   r.LogGroupName,
				LogStreamName:  logStreamName(streamPrefix, container, task),
				CloudWatchLogs: r.cwlconn,

				// watch for the finish message to terminate the logger
				Printer: func(ev *cloudwatchlogs.FilteredLogEvent) bool {
					finishedPrefix := fmt.Sprintf(
						"Container %s exited with",
						containerID,
					)
					if strings.HasPrefix(*ev.Message, finishedPrefix) {
						log.Printf("Found container finished message for %s: %s",
							containerID, *ev.Message)
						return false
					}
					fmt.Println(*ev.Message)
					return true
				},
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := watcher.Watch(ctx); err != nil {
					log.Printf("Log watcher returned error: %v", err)
				}
			}()
		}
	}

	var taskARNs []*string
	for _, task := range runResp.Tasks {
		log.Printf("Waiting until task %s has stopped", *task.TaskArn)
		taskARNs = append(taskARNs, task.TaskArn)
	}

	for {
		werr := r.ecsconn.WaitUntilTasksStopped(&ecs.DescribeTasksInput{
			Cluster: aws.String(r.Cluster),
			Tasks:   taskARNs,
		})
		if werr == nil {
			break
		}
		if !isAwsTimeOutError(werr) {
			return werr
		}
	}

	log.Printf("All tasks have stopped")

	output, err := r.ecsconn.DescribeTasks(&ecs.DescribeTasksInput{
		Cluster: aws.String(r.Cluster),
		Tasks:   taskARNs,
	})
	if err != nil {
		return err
	}

	// Get the final state of each task and container and write to cloudwatch logs
	for _, task := range output.Tasks {
		for _, container := range task.Containers {
			lw := &logWriter{
				LogGroupName:   r.LogGroupName,
				LogStreamName:  logStreamName(streamPrefix, container, task),
				CloudWatchLogs: r.cwlconn,
			}
			if err := writeContainerFinishedMessage(ctx, lw, task, container); err != nil {
				return err
			}
		}
	}

	log.Printf("Waiting for logs to finish")
	wg.Wait()

	// Determine exit code based on the first non-zero exit code
	for _, task := range output.Tasks {
		for _, container := range task.Containers {
			if *container.ExitCode != 0 {
				return &exitError{
					fmt.Errorf(
						"container %s exited with %d",
						*container.Name,
						*container.ExitCode,
					),
					int(*container.ExitCode),
				}
			}
		}
	}

	return err
}

func isAwsTimeOutError(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "ResourceNotReady" {
			return true
		}
	}
	return false
}

func logStreamName(logStreamPrefix string, container *ecs.Container, task *ecs.Task) string {
	return fmt.Sprintf(
		"%s/%s/%s",
		logStreamPrefix,
		*container.Name,
		path.Base(*task.TaskArn),
	)
}

func writeContainerFinishedMessage(ctx context.Context, w *logWriter, task *ecs.Task, container *ecs.Container) error {
	if *container.LastStatus != `STOPPED` {
		return fmt.Errorf("expected container to be STOPPED, got %s", *container.LastStatus)
	}
	if container.ExitCode == nil {
		return errors.New(*container.Reason)
	}
	return w.WriteString(ctx, fmt.Sprintf(
		"Container %s exited with %d",
		path.Base(*container.ContainerArn),
		*container.ExitCode,
	))
}

type exitError struct {
	error
	exitCode int
}

func (ee *exitError) ExitCode() int {
	return ee.exitCode
}

func awsStrings(ss []string) []*string {
	out := make([]*string, len(ss))
	for i := range ss {
		out[i] = &ss[i]
	}
	return out
}

func awsKeyValuePairForEnv(lookupEnv func(key string) (string, bool), wanted []string) ([]*ecs.KeyValuePair, error) {
	var kvp []*ecs.KeyValuePair
	for _, s := range wanted {
		parts := strings.SplitN(s, "=", 2)
		key := parts[0]
		var value string
		if len(parts) == 2 {
			value = parts[1]
		} else {
			v2, ok := lookupEnv(parts[0])
			if !ok {
				return nil, fmt.Errorf("missing environment variable %q", key)
			}
			value = v2
		}

		kvp = append(kvp, &ecs.KeyValuePair{
			Name:  &key,
			Value: &value,
		})
	}

	return kvp, nil
}

func handleAwsError(err error) error {
	if aerr, ok := err.(awserr.Error); ok {
		switch aerr.Code() {
		case ecs.ErrCodeServerException:
			fmt.Println(ecs.ErrCodeServerException, aerr.Error())
		case ecs.ErrCodeClientException:
			fmt.Println(ecs.ErrCodeClientException, aerr.Error())
		case ecs.ErrCodeInvalidParameterException:
			fmt.Println(ecs.ErrCodeInvalidParameterException, aerr.Error())
		default:
			fmt.Println(aerr.Error())
		}
	} else {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
	}
	return err
}
