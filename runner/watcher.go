package runner

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	logs "github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

type logWatcher struct {
	LogGroup, LogStreamName string
	Svc                     *logs.CloudWatchLogs
	Printer                 func(event *logs.FilteredLogEvent, c context.CancelFunc)
}

func (lw *logWatcher) findStreams() ([]*string, error) {
	params := &logs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(lw.LogGroup),
		LogStreamNamePrefix: aws.String(lw.LogStreamName),
		Descending:          aws.Bool(true),
	}

	streams := []*string{}
	err := lw.Svc.DescribeLogStreamsPages(params, func(page *logs.DescribeLogStreamsOutput, lastPage bool) bool {
		for _, stream := range page.LogStreams {
			streams = append(streams, stream.LogStreamName)
		}
		return lastPage
	})

	return streams, err
}

func (lw *logWatcher) printEventsAfter(ts int64, c context.CancelFunc) (int64, error) {
	var streams []*string
	var err error

	log.Printf("Printing events after %d", ts)

	// sometimes the stream takes a while to appear
	for i := 0; i < 30; i++ {
		streams, err = lw.findStreams()
		if err != nil || len(streams) == 0 {
			log.Printf("Log stream hasn't started yet")
			time.Sleep(time.Second * 2)
		}
	}

	if err != nil {
		return ts, err
	} else if len(streams) == 0 {
		return ts, errors.New("No stream found")
	}

	filterInput := &logs.FilterLogEventsInput{
		LogGroupName:   aws.String(lw.LogGroup),
		LogStreamNames: streams,
		StartTime:      aws.Int64(ts + 1),
	}

	err = lw.Svc.FilterLogEventsPages(filterInput,
		func(p *logs.FilterLogEventsOutput, lastPage bool) (shouldContinue bool) {
			for _, event := range p.Events {
				lw.Printer(event, c)
				if *event.Timestamp > ts {
					ts = *event.Timestamp
				}
			}
			return lastPage
		})

	return ts, err
}

func (lw *logWatcher) Watch(ctx context.Context) error {
	var after int64
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		select {
		case <-time.After(1 * time.Second):
			if after, err = lw.printEventsAfter(after, cancel); err != nil {
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}
