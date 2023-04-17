package utils

import (
	"encoding/json"
	"os"
	"syscall"
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

func FileExistAndReadable(name string) bool {
	if len(name) == 0 {
		return false
	}

	if entry, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	} else {
		fm := entry.Mode()
		sys := entry.Sys().(*syscall.Stat_t)

		if (fm&(1<<2) != 0) || ((fm&(1<<5)) != 0 && os.Getegid() == int(sys.Gid)) || ((fm&(1<<8)) != 0 && (os.Geteuid() == int(sys.Uid))) {
			return true
		}
	}

	return false
}
