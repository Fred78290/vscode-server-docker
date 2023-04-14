package client

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Fred78290/vscode-server-helper/context"
	"github.com/Fred78290/vscode-server-helper/types"
	"github.com/Fred78290/vscode-server-helper/utils"
	requestutil "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/requests/util"

	"github.com/linki/instrumented_http"
	glog "github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SingletonClientGenerator provides clients
type SingletonClientGenerator struct {
	KubeConfig       string
	APIServerURL     string
	RequestTimeout   time.Duration
	DeletionTimeout  time.Duration
	MaxGracePeriod   time.Duration
	NodeReadyTimeout time.Duration
	kubeClient       kubernetes.Interface
	kubeOnce         sync.Once
}

func NewClientGenerator(cfg *types.Config) types.ClientGenerator {
	return &SingletonClientGenerator{
		KubeConfig:       cfg.KubeConfig,
		APIServerURL:     cfg.APIServerURL,
		RequestTimeout:   cfg.RequestTimeout,
		NodeReadyTimeout: cfg.NodeReadyTimeout,
		DeletionTimeout:  cfg.DeletionTimeout,
		MaxGracePeriod:   cfg.MaxGracePeriod,
	}
}

// getRestConfig returns the rest clients config to get automatically
// data if you run inside a cluster or by passing flags.
func getRestConfig(kubeConfig, apiServerURL string) (*rest.Config, error) {
	if kubeConfig == "" {
		if _, err := os.Stat(clientcmd.RecommendedHomeFile); err == nil {
			kubeConfig = clientcmd.RecommendedHomeFile
		}
	}

	glog.Debugf("apiServerURL: %s", apiServerURL)
	glog.Debugf("kubeConfig: %s", kubeConfig)

	// evaluate whether to use kubeConfig-file or serviceaccount-token
	var (
		config *rest.Config
		err    error
	)
	if kubeConfig == "" {
		glog.Infof("Using inCluster-config based on serviceaccount-token")
		config, err = rest.InClusterConfig()
	} else {
		glog.Infof("Using kubeConfig")
		config, err = clientcmd.BuildConfigFromFlags(apiServerURL, kubeConfig)
	}
	if err != nil {
		return nil, err
	}

	return config, nil
}

// newKubeClient returns a new Kubernetes client object. It takes a Config and
// uses APIServerURL and KubeConfig attributes to connect to the cluster. If
// KubeConfig isn't provided it defaults to using the recommended default.
func newKubeClient(kubeConfig, apiServerURL string, requestTimeout time.Duration) (kubernetes.Interface, error) {
	glog.Infof("Instantiating new Kubernetes client")

	config, err := getRestConfig(kubeConfig, apiServerURL)
	if err != nil {
		return nil, err
	}

	config.Timeout = requestTimeout * time.Second

	config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return instrumented_http.NewTransport(rt, &instrumented_http.Callbacks{
			PathProcessor: func(path string) string {
				parts := strings.Split(path, "/")
				return parts[len(parts)-1]
			},
		})
	}

	client, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, err
	}

	glog.Infof("Created Kubernetes client %s", config.Host)

	return client, err
}

func (p *SingletonClientGenerator) newRequestContext() *context.Context {
	return utils.NewRequestContext(p.RequestTimeout)
}

// KubeClient generates a kube client if it was not created before
func (p *SingletonClientGenerator) KubeClient() (kubernetes.Interface, error) {
	var err error
	p.kubeOnce.Do(func() {
		p.kubeClient, err = newKubeClient(p.KubeConfig, p.APIServerURL, p.RequestTimeout)
	})
	return p.kubeClient, err
}

func (p *SingletonClientGenerator) NameSpaceExists(namespace string) (bool, error) {
	return false, nil
}

func (p *SingletonClientGenerator) CreateNameSpace(namespace string) error {
	return nil
}

func (p *SingletonClientGenerator) CreateCodeSpace(currentUser string, cfg *types.Config, w http.ResponseWriter, req *http.Request) error {
	var err error
	var exists bool

	redirect := *cfg.GetRedirectURL()

	if exists, err = p.NameSpaceExists(currentUser); err != nil {

		w.Header().Add("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return err
	}

	if exists == false {

		if err = p.CreateNameSpace(currentUser); err != nil {
			w.Header().Add("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))

			return err
		}

	}

	if redirect.Host == "" {
		redirect.Host = requestutil.GetRequestHost(req)
		redirect.Scheme = requestutil.GetRequestProto(req)
		redirect.Path = fmt.Sprintf("/%s", currentUser)
	} else {
		redirect.Path = fmt.Sprintf(redirect.Path, currentUser)
	}

	http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)

	return nil
}
