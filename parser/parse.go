package parser

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/buildkite/interpolate"
	"github.com/ghodss/yaml"
)

func Parse(file string, env []string) (*ecs.RegisterTaskDefinitionInput, error) {
	body, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	interpolated, err := interpolate.Interpolate(
		interpolate.EnvFromSlice(env),
		string(body),
	)
	if err != nil {
		return nil, err
	}

	unmarshaled, err := unmarshal([]byte(interpolated))
	if err != nil {
		return nil, err
	}

	// Return to json which aws will parse
	jsonBytes, err := json.Marshal(unmarshaled)
	if err != nil {
		return nil, err
	}

	var result ecs.RegisterTaskDefinitionInput

	// And then into the task definition 👌🏻 🤞🏻
	if err = json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func unmarshal(body []byte) (interface{}, error) {
	var unmarshaled interface{}

	err := yaml.Unmarshal(body, &unmarshaled)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse: %v", err)
	}

	return unmarshaled, nil
}
