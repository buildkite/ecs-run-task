package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/buildkite/ecs-run-task/parser"
)

type Override struct {
	Service string
	Command []string
}

type Runner struct {
	Service            string
	TaskName           string
	TaskDefinitionFile string
	Cluster            string
	LogGroupName       string
	Region             string
	Config             *aws.Config
	Overrides          []Override
}

func New() *Runner {
	return &Runner{
		Region: os.Getenv("AWS_REGION"),
		Config: aws.NewConfig(),
	}
}

func (r *Runner) Run() error {
	taskDefinitionInput, err := parser.Parse(r.TaskDefinitionFile, os.Environ())
	if err != nil {
		return err
	}

	streamPrefix := r.TaskName
	if streamPrefix == "" {
		streamPrefix = fmt.Sprintf("run_task_%d", time.Now().Nanosecond())
	}

	sess := session.Must(session.NewSession(r.Config))

	if err := createLogGroup(sess, r.LogGroupName); err != nil {
		return err
	}

	log.Printf("Setting tasks to use log group %s", r.LogGroupName)
	for _, def := range taskDefinitionInput.ContainerDefinitions {
		if def.LogConfiguration == nil {
			def.LogConfiguration = &ecs.LogConfiguration{
				LogDriver: aws.String("awslogs"),
				Options: map[string]*string{
					"awslogs-group":         aws.String(r.LogGroupName),
					"awslogs-region":        aws.String(r.Region),
					"awslogs-stream-prefix": aws.String(streamPrefix),
				},
			}
		}
	}

	svc := ecs.New(sess)
	cwl := cloudwatchlogs.New(sess)

	log.Printf("Registering a task for %s", *taskDefinitionInput.Family)
	resp, err := svc.RegisterTaskDefinition(taskDefinitionInput)
	if err != nil {
		return err
	}

	taskDefinition := fmt.Sprintf("%s:%d",
		*resp.TaskDefinition.Family, *resp.TaskDefinition.Revision)

	runTaskInput := &ecs.RunTaskInput{
		TaskDefinition: aws.String(taskDefinition),
		Cluster:        aws.String(r.Cluster),
		Count:          aws.Int64(1),
		Overrides: &ecs.TaskOverride{
			ContainerOverrides: []*ecs.ContainerOverride{},
		},
	}

	for _, override := range r.Overrides {
		if len(override.Command) > 0 {
			cmds := []*string{}

			if override.Service == "" {
				if len(taskDefinitionInput.ContainerDefinitions) != 1 {
					return fmt.Errorf("no service provided for override and cant determine default service with %d container definitions", len(taskDefinitionInput.ContainerDefinitions))
				}

				override.Service = *taskDefinitionInput.ContainerDefinitions[0].Name
				log.Printf("Assuming override applies to '%s'", override.Service)
			}

			for _, command := range override.Command {
				cmds = append(cmds, aws.String(command))
			}

			runTaskInput.Overrides.ContainerOverrides = append(
				runTaskInput.Overrides.ContainerOverrides,
				&ecs.ContainerOverride{
					Command: cmds,
					Name:    aws.String(override.Service),
				},
			)
		}
	}

	log.Printf("Running task %s", taskDefinition)
	runResp, err := svc.RunTask(runTaskInput)
	if err != nil {
		return fmt.Errorf("unable to run task: %d", err.Error())
	}

	taskARNs := []*string{}
	for _, t := range runResp.Tasks {
		taskARNs = append(taskARNs, t.TaskArn)
	}

	errs := make(chan error)
	go func() {
		defer close(errs)

		lw := &logWriter{
			LogGroupName: r.LogGroupName,
			StreamPrefix: streamPrefix,
			Svc:          cwl,
		}

		err = svc.WaitUntilTasksStopped(&ecs.DescribeTasksInput{
			Cluster: aws.String(r.Cluster),
			Tasks:   taskARNs,
		})
		if err != nil {
			errs <- err
			return
		}

		output, err := svc.DescribeTasks(&ecs.DescribeTasksInput{
			Cluster: aws.String(r.Cluster),
			Tasks:   taskARNs,
		})
		if err != nil {
			errs <- err
			return
		}

		for _, task := range output.Tasks {
			for _, container := range task.Containers {
				if err := lw.WriteFinished(task, container); err != nil {
					errs <- err
					return
				}
			}
		}

		for _, task := range output.Tasks {
			for _, container := range task.Containers {
				if *container.ExitCode != 0 {
					errs <- &exitError{
						fmt.Errorf(
							"Container %s exited with %d",
							*container.Name,
							*container.ExitCode,
						),
						int(*container.ExitCode),
					}
				}
			}
		}
	}()

	var wg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, task := range runResp.Tasks {
		for _, container := range task.Containers {
			printer := eventPrinter{
				Task:      task,
				Container: container,
			}
			lw := &logWatcher{
				LogGroup:      r.LogGroupName,
				LogStreamName: logStreamName(streamPrefix, task, container),
				Svc:           cwl,
				Printer:       printer.Print,
			}
			log.Printf("Watching %s/%s", lw.LogGroup, lw.LogStreamName)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := lw.Watch(ctx); err != nil {
					errs <- err
				}
			}()
		}
	}

	log.Printf("Waiting for exitcode")
	if err := <-errs; err != nil {
		log.Printf("Error from ch: %#v", err)
		// If the error is not an exit error, stop everything
		_, ok := err.(*exitError)
		if(!ok) {
			cancel()
			return err
		}
	}

	log.Printf("Waiting for logging to finish")
	wg.Wait()

	return err
}

func logStreamName(streamPrefix string, task *ecs.Task, container *ecs.Container) string {
	return fmt.Sprintf(
		"%s/%s/%s",
		streamPrefix,
		*container.Name,
		path.Base(*task.TaskArn),
	)
}

type eventPrinter struct {
	Task      *ecs.Task
	Container *ecs.Container
}

func (p *eventPrinter) Print(ev *cloudwatchlogs.FilteredLogEvent, cancel context.CancelFunc) {
	finishedPrefix := fmt.Sprintf(
		"Container %s exited with",
		path.Base(*p.Container.ContainerArn),
	)

	if strings.HasPrefix(*ev.Message, finishedPrefix) {
		log.Printf("Finished: %s", *ev.Message)
		cancel()
		return
	}

	fmt.Println(*ev.Message)
}

type logWriter struct {
	Svc          *cloudwatchlogs.CloudWatchLogs
	LogGroupName string
	StreamPrefix string
}

func (lw *logWriter) NextSequenceToken(streamPrefix string) (*string, error) {
	streams, err := lw.Svc.DescribeLogStreams(&cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(lw.LogGroupName),
		LogStreamNamePrefix: aws.String(streamPrefix),
	})
	if err != nil {
		return nil, err
	}
	if len(streams.LogStreams) == 0 {
		return nil, fmt.Errorf("Failed to find stream %s", streamPrefix)
	}
	return streams.LogStreams[0].UploadSequenceToken, nil
}

func (lw *logWriter) WriteFinished(task *ecs.Task, container *ecs.Container) error {
	streamName := logStreamName(lw.StreamPrefix, task, container)
	sequence, err := lw.NextSequenceToken(streamName)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf(
		"Container %s exited with %d",
		path.Base(*container.ContainerArn),
		*container.ExitCode,
	)
	_, err = lw.Svc.PutLogEvents(&cloudwatchlogs.PutLogEventsInput{
		SequenceToken: sequence,
		LogGroupName:  aws.String(lw.LogGroupName),
		LogStreamName: aws.String(streamName),
		LogEvents: []*cloudwatchlogs.InputLogEvent{
			&cloudwatchlogs.InputLogEvent{
				Message:   aws.String(msg),
				Timestamp: aws.Int64(aws.TimeUnixMilli(time.Now())),
			},
		},
	})
	return err
}

func createLogGroup(sess *session.Session, logGroup string) error {
	cwl := cloudwatchlogs.New(sess)
	groups, err := cwl.DescribeLogGroups(&cloudwatchlogs.DescribeLogGroupsInput{
		Limit:              aws.Int64(1),
		LogGroupNamePrefix: aws.String(logGroup),
	})
	if err != nil {
		return err
	}
	if len(groups.LogGroups) == 0 {
		log.Printf("Creating log group %s", logGroup)
		_, err = cwl.CreateLogGroup(&cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroup),
		})
		if err != nil {
			return err
		}
	} else {
		log.Printf("Log group %s exists", logGroup)
	}
	return nil
}

type exitError struct {
	error
	exitCode int
}

func (ee *exitError) ExitCode() int {
	return ee.exitCode
}
