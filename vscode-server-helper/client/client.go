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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/cli"
	"k8s.io/kubectl/pkg/cmd"
	"k8s.io/kubectl/pkg/cmd/plugin"
	"k8s.io/kubectl/pkg/cmd/util"
)

const (
	appLabel       = "app.kubernetes.io/name"
	deploymentName = "vscode-server"
	ingressName    = "vscode-server-%s"
	contentType    = "Content-Type"
	textPlain      = "text/plain"
)

var defaultConfigFlags = genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDiscoveryBurst(300).WithDiscoveryQPS(50.0)

// SingletonClientGenerator provides clients
type SingletonClientGenerator struct {
	KubeConfig         string
	APIServerURL       string
	RequestTimeout     time.Duration
	DeletionTimeout    time.Duration
	MaxGracePeriod     time.Duration
	ObjectReadyTimeout time.Duration
	kubeClient         kubernetes.Interface
	cfg                *types.Config
	kubeOnce           sync.Once
}

func encodeToYaml(obj runtime.Object) string {

	if obj == nil {
		return ""
	}

	var outBuffer bytes.Buffer
	writer := bufio.NewWriter(&outBuffer)

	e := k8sjson.NewYAMLSerializer(k8sjson.DefaultMetaFactory, nil, nil)

	if err := e.Encode(obj, writer); err != nil {
		return ""
	}

	writer.Flush()

	return outBuffer.String()
}

func NewClientGenerator(cfg *types.Config) types.ClientGenerator {
	return &SingletonClientGenerator{
		KubeConfig:         cfg.KubeConfig,
		APIServerURL:       cfg.APIServerURL,
		RequestTimeout:     cfg.RequestTimeout,
		ObjectReadyTimeout: cfg.ObjectReadyTimeout,
		DeletionTimeout:    cfg.DeletionTimeout,
		MaxGracePeriod:     cfg.MaxGracePeriod,
		cfg:                cfg,
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

func (p *SingletonClientGenerator) kubectl(args ...string) (string, error) {
	var outBuffer bytes.Buffer
	var err error

	stdout := bufio.NewWriter(&outBuffer)

	util.BehaviorOnFatal(func(msg string, code int) {
		if len(msg) > 0 {
			// add newline if needed
			if !strings.HasSuffix(msg, "\n") {
				msg += "\n"
			}
			fmt.Fprint(stdout, msg)
		}
	})

	kubectl := cmd.NewDefaultKubectlCommandWithArgs(cmd.KubectlOptions{
		PluginHandler: cmd.NewDefaultPluginHandler(plugin.ValidPluginFilenamePrefixes),
		Arguments:     args,
		ConfigFlags:   defaultConfigFlags,
		IOStreams:     genericclioptions.IOStreams{In: os.Stdin, Out: stdout, ErrOut: stdout},
	})

	kubectl.SetArgs(args[1:])
	kubectl.SetIn(os.Stdin)
	kubectl.SetOut(stdout)
	kubectl.SetErr(stdout)

	err = cli.RunNoErrOutput(kubectl)

	return outBuffer.String(), err
}

func (p *SingletonClientGenerator) CodeSpaceExists(userName string) (bool, error) {
	var ns *v1.Namespace
	kubeclient, err := p.KubeClient()

	if err != nil {
		return false, err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, userName, metav1.GetOptions{})

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

func (p *SingletonClientGenerator) waitNameSpaceReady(ctx *context.Context, userName string) (bool, error) {
	var ns *v1.Namespace

	kubeclient, err := p.KubeClient()

	if err = utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, userName, metav1.GetOptions{})

		if err != nil {
			return false, err
		}

		return ns.Status.Phase == v1.NamespaceActive, nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (p *SingletonClientGenerator) waitIngressReady(ctx *context.Context, userName, name string) (bool, error) {
	var ing *networkingv1.Ingress
	kubeclient, err := p.KubeClient()

	if err = utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		if ing, err = kubeclient.NetworkingV1().Ingresses(userName).Get(ctx, name, metav1.GetOptions{}); err != nil {
			return false, err
		}

		for _, status := range ing.Status.LoadBalancer.Ingress {
			if status.IP != "" {
				return true, nil
			}
		}

		return false, nil
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (p *SingletonClientGenerator) saveTemplate(yaml string) (string, error) {
	if f, err := os.CreateTemp("", "template.yml"); err != nil {
		return "", err
	} else {
		defer f.Close()

		_, err = f.WriteString(yaml)

		return f.Name(), err
	}
}

func (p *SingletonClientGenerator) waitDeploymentReady(ctx *context.Context, userName, name string) (bool, error) {
	var app *appv1.Deployment
	kubeclient, err := p.KubeClient()

	if err = utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		apps := kubeclient.AppsV1().Deployments(userName)

		if app, err = apps.Get(ctx, name, metav1.GetOptions{}); err == nil {

			for _, status := range app.Status.Conditions {
				if status.Type == appv1.DeploymentAvailable {

					if b, e := strconv.ParseBool(string(status.Status)); e == nil {
						if b {
							glog.Debugf("app %s/%s marked ready with replicas=%d, read replicas=%d", userName, name, app.Status.Replicas, app.Status.ReadyReplicas)

							return app.Status.Replicas == app.Status.ReadyReplicas, nil
						}
					}

				} else if status.Type == appv1.DeploymentReplicaFailure {
					return false, fmt.Errorf("app %s/%s replica failure: %v", userName, name, status.String())
				}
			}
		}

		return false, err
	}); err != nil {
		err = fmt.Errorf("app %s/%s is not ready, %v", userName, name, err)

		return false, err
	}

	glog.Infof("The kubernetes app %s/%s is Ready", userName, name)

	return true, err
}

func (p *SingletonClientGenerator) waitAppReady(userName string) (bool, error) {
	ctx := p.newRequestContext()
	defer ctx.Cancel()

	glog.Infof("Wait kubernetes app %s/%s to be ready", userName, deploymentName)

	if ready, err := p.waitNameSpaceReady(ctx, userName); err != nil || !ready {
		return ready, err
	} else if ready, err := p.waitDeploymentReady(ctx, userName, userName); err != nil || !ready {
		return ready, err
	} else {
		return p.waitIngressReady(ctx, userName, userName)
	}

}

func (p *SingletonClientGenerator) getNameSpaceConfigMapAndSecretsAndTls(userName string) (string, string, string, error) {
	var gotCM *v1.ConfigMap
	var gotSecret *v1.Secret
	var tls *v1.Secret
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: userName,
			Labels: map[string]string{
				appLabel: userName,
			},
		},
	}

	secret := &v1.Secret{
		Type: "Opaque",
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      userName,
			Namespace: userName,
			Labels: map[string]string{
				appLabel: userName,
			},
		},
	}

	kubeclient, err := p.KubeClient()

	if err != nil {
		return "", "", "", err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	if gotCM, err = kubeclient.CoreV1().ConfigMaps(p.cfg.VSCodeServerNameSpace).Get(ctx, userName, metav1.GetOptions{}); err != nil {
		glog.Debugf("No configmap %s/%s found, %v", p.cfg.VSCodeServerNameSpace, userName, err)
	} else {
		glog.Debugf("Configmap %s/%s found", p.cfg.VSCodeServerNameSpace, userName)

		cm.Data = gotCM.Data
		cm.BinaryData = gotCM.BinaryData
	}

	if gotSecret, err = kubeclient.CoreV1().Secrets(p.cfg.VSCodeServerNameSpace).Get(ctx, p.cfg.VSCodeIngressTlsSecret, metav1.GetOptions{}); err != nil {
		glog.Debugf("No secret %s/%s found, %v", p.cfg.VSCodeServerNameSpace, p.cfg.VSCodeIngressTlsSecret, err)
	} else {
		glog.Debugf("Tls %s/%s found", p.cfg.VSCodeServerNameSpace, p.cfg.VSCodeIngressTlsSecret)

		tls = &v1.Secret{
			Type: "kubernetes.io/tls",
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.cfg.VSCodeIngressTlsSecret,
				Namespace: userName,
				Labels: map[string]string{
					appLabel: userName,
				},
			},
			Data: gotSecret.Data,
		}
	}

	if gotSecret, err = kubeclient.CoreV1().Secrets(p.cfg.VSCodeServerNameSpace).Get(ctx, userName, metav1.GetOptions{}); err != nil {
		glog.Debugf("No secret %s/%s found, %v", p.cfg.VSCodeServerNameSpace, userName, err)
	} else {
		glog.Debugf("Secret %s/%s found", p.cfg.VSCodeServerNameSpace, userName)

		secret.Data = gotSecret.Data
	}

	return encodeToYaml(cm), encodeToYaml(secret), encodeToYaml(tls), nil
}

func (p *SingletonClientGenerator) applyTemplate(yaml string) (err error) {
	var out string
	var templateFile string

	fmt.Println(yaml)

	if templateFile, err = p.saveTemplate(yaml); err != nil {
		return err
	}

	defer func() {
		os.Remove(templateFile)
	}()

	if out, err = p.kubectl("kubectl", "apply", "-f", templateFile); err != nil {
		glog.Errorf("kubectl got an error: %s, %v", out, err)
	}

	return err
}

func (p *SingletonClientGenerator) deleteUserCodespace(userName string) error {
	var out string
	var err error

	if out, err = p.kubectl("kubectl", "delete", "ns", userName); err != nil {
		glog.Errorf("kubectl got an error: %s, %v", out, err)
	}

	return err
}

func (p *SingletonClientGenerator) createUserCodespace(userName string) error {
	var template []byte
	var err error
	var yaml string
	var configmap, secret, tls string
	ready := false

	if configmap, secret, tls, err = p.getNameSpaceConfigMapAndSecretsAndTls(userName); err != nil {
		glog.Errorf("Unable to get config map and secret for user: %s, %v", userName, err)
		return err
	}

	mapping := func(name string) string {
		var value string
		var found bool

		if value, found = os.LookupEnv(name); found {
			return value
		} else {
			switch name {
			case "ACCOUNT_NAMESPACE":
				return userName

			case "ACCOUNT_CONFIGMAP":
				return configmap

			case "ACCOUNT_SECRET":
				return secret

			case "INGRESS_SECRET_TLS":
				return tls

			case "VSCODE_NAMESPACE":
				return p.cfg.VSCodeServerNameSpace

			case "VSCODE_HOSTNAME":
				return p.cfg.VSCodeHostname

			case "VSCODE_PVC_SIZE":
				return p.cfg.PersistentVolumeSize

			case "VSCODE_CPU_MAX":
				return p.cfg.MaxCpus

			case "VSCODE_CPU_REQUEST":
				return p.cfg.MinCpus

			case "VSCODE_MEM_MAX":
				return p.cfg.MaxMemory

			case "VSCODE_MEM_REQUEST":
				return p.cfg.MinMemory

			default:
				return fmt.Sprintf("$%s", name)
			}
		}
	}

	if template, err = os.ReadFile(p.cfg.VSCodeTemplatePath); err == nil {
		if yaml, err = envsubst.Eval(string(template), mapping); err == nil {
			if err = p.applyTemplate(yaml); err == nil {
				if ready, err = p.waitAppReady(userName); err != nil || !ready {
					if err == nil {
						err = fmt.Errorf("vscode-server not ready for user: %s", userName)
					}

					p.deleteUserCodespace(userName)
				}
			}
		}
	}

	return err
}

func (p *SingletonClientGenerator) DeleteCodeSpace(currentUser string, w http.ResponseWriter, req *http.Request) error {
	var err error
	var exists bool

	if exists, err = p.CodeSpaceExists(currentUser); err != nil {

		w.Header().Add(contentType, textPlain)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Unable to find user: %s, %v", currentUser, err.Error())))

	} else if exists {

		if err = p.deleteUserCodespace(currentUser); err == nil {
			w.Header().Add(contentType, textPlain)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("User %s deleted", currentUser)))
		} else {
			w.Header().Add(contentType, textPlain)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("Unable to delete user: %s, %v", currentUser, err.Error())))
		}

	} else {
		w.Header().Add(contentType, textPlain)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(fmt.Sprintf("User %s not found", currentUser)))
	}

	return err
}

func (p *SingletonClientGenerator) CreateCodeSpace(currentUser string, w http.ResponseWriter, req *http.Request) error {
	var err error
	var exists bool

	redirect := *p.cfg.GetRedirectURL()

	if exists, err = p.CodeSpaceExists(currentUser); err != nil {

		w.Header().Add(contentType, textPlain)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))

		return err
	}

	if !exists {

		if err = p.createUserCodespace(currentUser); err != nil {
			w.Header().Add(contentType, textPlain)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))

			return err
		}

	}

	if redirect.Host == "" {
		redirect.Host = requestutil.GetRequestHost(req)
		redirect.Scheme = requestutil.GetRequestProto(req)
		redirect.Path = fmt.Sprintf("/%s?folder=/home/vscode-server/sources", currentUser)
	} else {
		redirect.Path = fmt.Sprintf(redirect.Path, currentUser)
	}

	http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)

	return nil
}
