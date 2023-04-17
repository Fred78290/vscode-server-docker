package types

import (
	"net/http"

	"k8s.io/client-go/kubernetes"
)

type ClientGenerator interface {
	KubeClient() (kubernetes.Interface, error)
	CodeSpaceExists(userName string) (bool, error)
	CreateCodeSpace(userName string, w http.ResponseWriter, req *http.Request) error
}
