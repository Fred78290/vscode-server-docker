package utils

import (
	"encoding/json"
	"time"

	"github.com/Fred78290/vscode-server-helper/context"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/wait"
)

// ToYAML serialize interface to yaml
func ToYAML(v interface{}) string {
	if v == nil {
		return ""
	}

	b, _ := yaml.Marshal(v)

	return string(b)
}

// ToJSON serialize interface to json
func ToJSON(v interface{}) string {
	if v == nil {
		return ""
	}

	b, _ := json.Marshal(v)

	return string(b)
}

func NewRequestContext(requestTimeout time.Duration) *context.Context {
	return context.NewContext(time.Duration(requestTimeout.Seconds()))
}

func PollImmediate(interval, timeout time.Duration, condition wait.ConditionFunc) error {
	if timeout == 0 {
		return wait.PollImmediateInfinite(interval, condition)
	} else {
		return wait.PollImmediate(interval, timeout, condition)
	}
}
