package types

import (
	"go/types"
	"net/http"

	"k8s.io/client-go/kubernetes"
)

type ClientGenerator interface {
	KubeClient() (kubernetes.Interface, error)
	NameSpaceExists(namespace string) (bool, error)
	CreateNameSpace(namespace string) error
	CreateCodeSpace(currentUser string, cfg *types.Config, w http.ResponseWriter, req *http.Request) error
}
