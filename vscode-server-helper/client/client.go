package client

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fred78290/vscode-server-helper/context"
	"github.com/Fred78290/vscode-server-helper/types"
	"github.com/Fred78290/vscode-server-helper/utils"
	"github.com/drone/envsubst"
	"github.com/linki/instrumented_http"
	requestutil "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/requests/util"
	glog "github.com/sirupsen/logrus"

	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli"
	"k8s.io/kubectl/pkg/cmd"
	"k8s.io/kubectl/pkg/cmd/plugin"
)

const appLabel = "app.kubernetes.io/name"

var defaultConfigFlags = genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDiscoveryBurst(300).WithDiscoveryQPS(50.0)

// SingletonClientGenerator provides clients
type SingletonClientGenerator struct {
	KubeConfig       string
	APIServerURL     string
	RequestTimeout   time.Duration
	DeletionTimeout  time.Duration
	MaxGracePeriod   time.Duration
	NodeReadyTimeout time.Duration
	kubeClient       kubernetes.Interface
	cfg              *types.Config
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
		cfg:              cfg,
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
	var ns *v1.Namespace
	kubeclient, err := p.KubeClient()

	if err != nil {
		return false, err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})

	if kerr, ok := err.(*errors.StatusError); ok {
		if kerr.ErrStatus.Code == 404 {
			err = nil
		}
	}

	if err != nil || ns.Name == "" {
		return false, err
	}

	return true, nil
}

func (p *SingletonClientGenerator) waitAppReady(namespace, name string) (bool, error) {
	var ns *v1.Namespace
	var app *appv1.Deployment
	ready := false
	kubeclient, err := p.KubeClient()

	if err == nil {

		ctx := p.newRequestContext()
		defer ctx.Cancel()

		glog.Infof("Wait kubernetes app %s/%s to be ready", namespace, name)

		if err = utils.PollImmediate(time.Second, p.NodeReadyTimeout, func() (bool, error) {
			ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})

			if err != nil {
				return false, err
			}

			return ns.Status.Phase == v1.NamespaceActive, nil
		}); err != nil {
			return false, err
		}

		if err = utils.PollImmediate(time.Second, p.NodeReadyTimeout, func() (bool, error) {
			apps := kubeclient.AppsV1().Deployments(namespace)

			app, err = apps.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			for _, status := range app.Status.Conditions {
				if status.Type == appv1.DeploymentAvailable {
					if b, e := strconv.ParseBool(string(status.Status)); e == nil {
						if b {
							glog.Debugf("app %s/%s marked ready with replicas=%d, read replicas=%d", namespace, name, app.Status.Replicas, app.Status.ReadyReplicas)
							return app.Status.Replicas == app.Status.ReadyReplicas, nil
						}
					}
				} else if status.Type == appv1.DeploymentReplicaFailure {
					return false, fmt.Errorf("app %s/%s replica failure: %v", namespace, name, status.String())
				}
			}

			return false, nil
		}); err == nil {
			glog.Infof("The kubernetes app %s/%s is Ready", namespace, name)
			ready = true
		} else {
			err = fmt.Errorf("app %s/%s is not ready, %v", namespace, name, err)
		}
	}

	return ready, err
}

func (p *SingletonClientGenerator) getNameSpaceConfigMapAndSecrets(namespace string) (configmap string, secrets string, err error) {
	var cm *v1.ConfigMap
	var secret *v1.Secret
	kubeclient, err := p.KubeClient()

	if err != nil {
		return "", "", err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	if cm, err = kubeclient.CoreV1().ConfigMaps(p.cfg.VSCodeServerNameSpace).Get(ctx, namespace, metav1.GetOptions{}); err != nil {
		glog.Debugf("No configmap %s/%s found, %v", p.cfg.VSCodeServerNameSpace, namespace, err)

		cm = &v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
				Labels: map[string]string{
					appLabel: namespace,
				},
			},
		}
	} else {
		glog.Debugf("Configmap %s/%s found, %s", p.cfg.VSCodeServerNameSpace, namespace, utils.ToJSON(cm))

		cm.ObjectMeta = metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
			Labels: map[string]string{
				appLabel: namespace,
			},
		}
	}

	if secret, err = kubeclient.CoreV1().Secrets(p.cfg.VSCodeServerNameSpace).Get(ctx, namespace, metav1.GetOptions{}); err != nil {
		glog.Debugf("No secret %s/%s found, %v", p.cfg.VSCodeServerNameSpace, namespace, err)

		secret = &v1.Secret{
			Type: "Opaque",
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
				Labels: map[string]string{
					appLabel: namespace,
				},
			},
		}
	} else {
		glog.Debugf("Secret %s/%s found, %s", p.cfg.VSCodeServerNameSpace, namespace, utils.ToJSON(secret))

		secret.ObjectMeta = metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
			Labels: map[string]string{
				appLabel: namespace,
			},
		}
	}

	return utils.ToYAML(cm), utils.ToYAML(secret), nil
}

func (p *SingletonClientGenerator) CreateNameSpace(namespace string) error {
	var template []byte
	var err error
	var yaml string
	var configmap, secret string

	if configmap, secret, err = p.getNameSpaceConfigMapAndSecrets(namespace); err != nil {
		glog.Errorf("Unable to get config map and secret for user: %s, %v", namespace, err)
		return err
	}

	mapping := func(name string) string {
		if name == "ACCOUNT_NAMESPACE" {
			return namespace
		} else if name == "VSCODE_NAMESPACE" {
			return p.cfg.VSCodeServerNameSpace
		} else if name == "ACCOUNT_CONFIGMAP" {
			return configmap
		} else if name == "ACCOUNT_SECRET" {
			return secret
		}

		return os.Getenv(name)
	}

	if template, err = os.ReadFile(p.cfg.VSCodeTemplatePath); err == nil {
		if yaml, err = envsubst.Eval(string(template), mapping); err == nil {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			var ready bool

			args := []string{"kubectl", "apply", "-f", "-"}
			kubectl := cmd.NewDefaultKubectlCommandWithArgs(cmd.KubectlOptions{
				PluginHandler: cmd.NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes),
				Arguments:     args,
				ConfigFlags:   defaultConfigFlags,
				IOStreams:     genericclioptions.IOStreams{In: strings.NewReader(yaml), Out: bufio.NewWriter(&stdout), ErrOut: bufio.NewWriter(&stderr)},
			})

			if err = cli.RunNoErrOutput(kubectl); err == nil {
				if ready, err = p.waitAppReady(namespace, "vscode-server"); err == nil {
					if !ready {
						err = fmt.Errorf("vscode-server not ready for user: %s", namespace)
					}
				}
			} else {
				glog.Errorf("kubectl got an error: %s, %v", stderr.String(), err)
			}
		}
	}

	return err
}

func (p *SingletonClientGenerator) CreateCodeSpace(currentUser string, w http.ResponseWriter, req *http.Request) error {
	var err error
	var exists bool

	redirect := *p.cfg.GetRedirectURL()

	if exists, err = p.NameSpaceExists(currentUser); err != nil {

		w.Header().Add("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return err
	}

	if !exists {

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
