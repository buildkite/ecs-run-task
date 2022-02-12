package runner

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/ecs"
)

func TestAWSKeyValuePairForEnvEmpty(t *testing.T) {
	lookupEnv := func(key string) (string, bool) {
		t.Fatal("expected no calls to getenv")
		return "", false
	}

	kvp, err := awsKeyValuePairForEnv(lookupEnv, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %q", err.Error())
	}
	if len(kvp) != 0 {
		t.Fatalf("Expected no key/value pairs, got %d of them", len(kvp))
	}
}

func TestAWSKeyValuePairForEnv(t *testing.T) {
	currentEnv := map[string]string{
		"HOSTNAME": "my-hostname",
		"EMPTY":    "",
	}
	lookupEnv := func(key string) (string, bool) {
		v, ok := currentEnv[key]
		return v, ok
	}

	kvp, err := awsKeyValuePairForEnv(lookupEnv, []string{
		"HOSTNAME",
		"EMPTY",
		"PROVIDED=some-provided-value",
	})
	if err != nil {
		t.Fatal("Unexpected error: " + err.Error())
	}
	expected := map[string]string{
		"HOSTNAME": "my-hostname",
		"EMPTY":    "",
		"PROVIDED": "some-provided-value",
	}

	if len(kvp) != len(expected) {
		t.Fatalf("Unexpected number of key value pairs. Expected %d, actual %d", len(expected), len(kvp))
	}

	for _, pair := range kvp {
		name := *pair.Name
		value := *pair.Value
		expectedValue := expected[name]
		if value != expectedValue {
			t.Fatalf("Bad value for key %q. Expected %q, actual %q. Could be missing or could be duplicated", name, expectedValue, value)
		}

		// to ensure we aren't just getting the same one over and over again
		delete(expected, name)
	}
}

func TestAWSKeyValuePairForEnvMissing(t *testing.T) {
	lookupEnv := func(key string) (string, bool) {
		return "", false
	}

	_, err := awsKeyValuePairForEnv(lookupEnv, []string{
		"MISSING_VALUE",
	})

	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if err.Error() != `missing environment variable "MISSING_VALUE"` {
		t.Fatalf("bad error message returned: %q", err.Error())
	}
}

func TestWriteContainerFinishedMessageNotStopped(t *testing.T) {
	ctx := context.Background()
	w := &logWriter{}
	task := ecs.Task{}
	lastStatus := "NOTSTOPPED"
	container := ecs.Container{
		LastStatus: &lastStatus,
	}

	err := writeContainerFinishedMessage(ctx, w, &task, &container)

	if err.Error() != fmt.Sprintf("expected container to be STOPPED, got %s", lastStatus) {
		t.Fatalf("Expected a specific error, got %s", err)
	}
}

func TestWriteContainerFinishedMessageNilExitCodeUseContainerReason(t *testing.T) {
	ctx := context.Background()
	w := &logWriter{}
	task := ecs.Task{}
	lastStatus := "STOPPED"
	reason := "some aws container reason"
	container := ecs.Container{
		LastStatus: &lastStatus,
		ExitCode:   nil,
		Reason:     &reason,
	}

	err := writeContainerFinishedMessage(ctx, w, &task, &container)

	if err.Error() != reason {
		t.Fatalf("Expected a specific error, got %s", err)
	}
}

func TestWriteContainerFinishedMessageNilExitCodeUseTaskReason(t *testing.T) {
	ctx := context.Background()
	w := &logWriter{}
	reason := "some aws task reason"
	task := ecs.Task{
		StoppedReason: &reason,
	}
	lastStatus := "STOPPED"
	container := ecs.Container{
		LastStatus: &lastStatus,
		ExitCode:   nil,
		Reason:     nil,
	}

	err := writeContainerFinishedMessage(ctx, w, &task, &container)

	if err.Error() != reason {
		t.Fatalf("Expected a specific error, got %s", err)
	}
}
