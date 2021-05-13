package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
)

func TestLogsWatcherTimesOutWhenNoStreamIsFound(t *testing.T) {
	w := logWatcher{
		LogGroupName:  "my-group",
		LogStreamName: "my-stream",
		Timeout:       time.Millisecond * 50,
		Interval:      time.Millisecond * 5,
		CloudWatchLogs: &mockCloudWatchLogs{
			logStreams: []*cloudwatchlogs.LogStream{},
		},
	}

	err := w.Watch(context.Background())
	if err == nil || err.Error() != `Timed out waiting for stream my-stream` {
		t.Fatalf("bad error %v", err)
	}
}

func TestLogsWatcherWaitsForStreamToStart(t *testing.T) {
	events := []*cloudwatchlogs.FilteredLogEvent{}
	ts := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	cwlc := &mockCloudWatchLogs{
		logStreams:      []*cloudwatchlogs.LogStream{},
		filterLogEvents: []*cloudwatchlogs.FilteredLogEvent{},
	}

	w := logWatcher{
		LogGroupName:  "my-group",
		LogStreamName: "my-stream",
		Printer: func(ev *cloudwatchlogs.FilteredLogEvent) bool {
			events = append(events, ev)
			return true
		},
		CloudWatchLogs: cwlc,
		Timeout:        time.Millisecond * 50,
		Interval:       time.Millisecond * 5,
	}

	// make the log stream appear after a while
	go func() {
		<-time.After(time.Millisecond * 20)

		cwlc.Lock()
		defer cwlc.Unlock()

		cwlc.logStreams = append(cwlc.logStreams, &cloudwatchlogs.LogStream{
			Arn:           aws.String("my-stream-arn"),
			LogStreamName: aws.String("my-stream"),
		})

		<-time.After(time.Millisecond * 20)

		cwlc.filterLogEvents = append(cwlc.filterLogEvents, &cloudwatchlogs.FilteredLogEvent{
			EventId:       aws.String("my-event"),
			LogStreamName: aws.String("my-log-stream"),
			Timestamp:     aws.Int64(ts.UnixNano() / int64(time.Millisecond)),
		})
	}()

	// stop watching after a while
	go func() {
		<-time.After(time.Millisecond * 100)
		w.Stop()
	}()

	if err := w.Watch(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(events) != 1 {
		t.Fatalf("bad event count, got %d", len(events))
	}
}

func TestLogsWatcherRespectsContext(t *testing.T) {
	cwlc := &mockCloudWatchLogs{
		logStreams: []*cloudwatchlogs.LogStream{{
			Arn:           aws.String("my-stream-arn"),
			LogStreamName: aws.String("my-stream"),
		}},
	}

	w := logWatcher{
		LogGroupName:   "my-group",
		LogStreamName:  "my-stream",
		CloudWatchLogs: cwlc,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	err := w.Watch(ctx)
	if err != context.DeadlineExceeded {
		t.Fatalf("bad error %v", err)
	}
}

func TestLogsWriterAppendsMessage(t *testing.T) {
	cwlc := &mockCloudWatchLogs{
		logStreams: []*cloudwatchlogs.LogStream{{
			Arn:                 aws.String("my-stream-arn"),
			LogStreamName:       aws.String("my-stream"),
			UploadSequenceToken: aws.String("my-token"),
		}},
	}

	w := logWriter{
		LogGroupName:   "my-group",
		LogStreamName:  "my-stream",
		Timeout:        time.Millisecond * 50,
		Interval:       time.Millisecond * 5,
		CloudWatchLogs: cwlc,
	}

	err := w.WriteString(context.Background(), "llamas rock")
	if err != nil {
		t.Fatal(err)
	}

	if l := len(cwlc.inputLogEvents); l != 1 {
		t.Fatal("bad number of input log events", l)
	}
}

type mockCloudWatchLogs struct {
	sync.Mutex

	logStreams      []*cloudwatchlogs.LogStream
	filterLogEvents []*cloudwatchlogs.FilteredLogEvent
	inputLogEvents  []*cloudwatchlogs.InputLogEvent
}

func (cw *mockCloudWatchLogs) DescribeLogStreams(input *cloudwatchlogs.DescribeLogStreamsInput) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	cw.Lock()
	defer cw.Unlock()

	output := &cloudwatchlogs.DescribeLogStreamsOutput{
		LogStreams: []*cloudwatchlogs.LogStream{},
	}

	for _, stream := range cw.logStreams {
		if strings.HasPrefix(*stream.LogStreamName, *input.LogStreamNamePrefix) {
			output.LogStreams = append(output.LogStreams, stream)
		}
	}

	return output, nil
}

func (cw *mockCloudWatchLogs) DescribeLogStreamsPages(input *cloudwatchlogs.DescribeLogStreamsInput,
	fn func(*cloudwatchlogs.DescribeLogStreamsOutput, bool) bool) error {

	output, err := cw.DescribeLogStreams(input)
	if err != nil {
		return err
	}

	fn(output, true)
	return nil
}

func (cw *mockCloudWatchLogs) FilterLogEventsPages(input *cloudwatchlogs.FilterLogEventsInput,
	fn func(*cloudwatchlogs.FilterLogEventsOutput, bool) bool) error {

	for {
		output := &cloudwatchlogs.FilterLogEventsOutput{
			Events: []*cloudwatchlogs.FilteredLogEvent{},
		}

		cw.Lock()

		if len(cw.filterLogEvents) > 0 {
			var e *cloudwatchlogs.FilteredLogEvent
			e, cw.filterLogEvents = cw.filterLogEvents[0], cw.filterLogEvents[1:]
			output.Events = append(output.Events, e)
		}

		if !fn(output, len(cw.filterLogEvents) == 0) {
			cw.Unlock()
			return nil
		}

		cw.Unlock()
	}
}

func (cw *mockCloudWatchLogs) PutLogEvents(input *cloudwatchlogs.PutLogEventsInput) (*cloudwatchlogs.PutLogEventsOutput, error) {
	cw.Lock()
	defer cw.Unlock()
	cw.inputLogEvents = append(cw.inputLogEvents, input.LogEvents...)
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}
