package client

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fred78290/vscode-server-helper/context"
	"github.com/Fred78290/vscode-server-helper/pagewriter"
	"github.com/Fred78290/vscode-server-helper/types"
	"github.com/Fred78290/vscode-server-helper/utils"
	"github.com/drone/envsubst"
	"github.com/linki/instrumented_http"
	cookies "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/cookies"
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
	vscodeLabel    = "vscode-server/owner"
	deploymentName = "vscode-server"
	ingressName    = "vscode-server-%s"
	contentType    = "Content-Type"
	textPlain      = "text/plain"
	errorPage      = "Error occured: %v"
	userNotFound   = "Could not find codespace for user: %s"
)

var defaultConfigFlags = genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag().WithDiscoveryBurst(300).WithDiscoveryQPS(50.0)

type clientStatus int

const (
	clientStatusNone     clientStatus = 0
	clientStatusCreating clientStatus = 1
	clientStatusCreated  clientStatus = 2
	clientStatusDeleting clientStatus = 3
	clientStatusDeleted  clientStatus = 4
	clientStatusErrored  clientStatus = 5
)

type vscodeClient struct {
	Name   string
	Status clientStatus
	mutex  sync.Mutex
}

// vscodeClientGenerator provides clients
type vscodeClientGenerator struct {
	KubeConfig         string
	APIServerURL       string
	RequestTimeout     time.Duration
	DeletionTimeout    time.Duration
	MaxGracePeriod     time.Duration
	ObjectReadyTimeout time.Duration
	kubeClient         kubernetes.Interface
	clients            map[string]*vscodeClient
	cfg                *types.Config
	pagewriter         pagewriter.Writer
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
	opts := pagewriter.Opts{
		TemplatesPath: cfg.TemplatePath,
		Footer:        cfg.TemplateFooter,
		Version:       cfg.Version,
		Debug:         cfg.Debug,
	}

	if pageWriter, err := pagewriter.NewWriter(opts); err != nil {
		glog.Panicf("error initialising page writer: %v", err)
	} else {
		return &vscodeClientGenerator{
			KubeConfig:         cfg.KubeConfig,
			APIServerURL:       cfg.APIServerURL,
			RequestTimeout:     cfg.RequestTimeout,
			ObjectReadyTimeout: cfg.ObjectReadyTimeout,
			DeletionTimeout:    cfg.DeletionTimeout,
			MaxGracePeriod:     cfg.MaxGracePeriod,
			clients:            map[string]*vscodeClient{},
			cfg:                cfg,
			pagewriter:         pageWriter,
		}
	}

	return nil
}

func serveMissingUser(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(utils.ToJSON(&ErrorResponse{
		Status: -1,
		Error: ErrorObject{
			Code:   http.StatusNotFound,
			Reason: "Missing X-Auth-Request-User",
		},
	})))
}

func serveUserNotFound(w http.ResponseWriter, userName string) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(utils.ToJSON(&ErrorResponse{
		Status: -1,
		Error: ErrorObject{
			Code:   http.StatusNotFound,
			Reason: fmt.Sprintf("User %s not found", userName),
		},
	})))
}

func serveOperationRunning(w http.ResponseWriter, reason string) {
	w.WriteHeader(http.StatusAlreadyReported)
	w.Write([]byte(utils.ToJSON(&ErrorResponse{
		Status: 1,
		Error: ErrorObject{
			Code:   http.StatusAlreadyReported,
			Reason: reason,
		},
	})))
}

func serveError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(utils.ToJSON(&ErrorResponse{
		Status: -1,
		Error: ErrorObject{
			Code:   http.StatusInternalServerError,
			Reason: err.Error(),
		},
	})))
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

func (c *vscodeClient) lock() func() {
	c.mutex.Lock()

	return func() {
		c.mutex.Unlock()
	}
}

func (p *vscodeClientGenerator) getClientState(userName string) *vscodeClient {
	var client *vscodeClient
	var found bool

	if client, found = p.clients[userName]; !found {
		kubeclient, _ := p.KubeClient()

		client = &vscodeClient{
			Name:   userName,
			Status: clientStatusNone,
		}

		if ns, err := kubeclient.CoreV1().Namespaces().Get(context.Background(), client.Name, metav1.GetOptions{}); err == nil {
			if ns.Status.Phase == v1.NamespaceActive {
				client.Status = clientStatusCreated
			}
		}

		p.clients[userName] = client
	}

	return client
}

func (p *vscodeClientGenerator) getRequestUser(req *http.Request) (*vscodeClient, bool) {
	if user, found := req.Header[types.AuthRequestUserHeader]; found {
		return p.getClientState(strings.ToLower(user[0])), true
	}

	return nil, false
}

func (p *vscodeClientGenerator) newRequestContext() *context.Context {
	return utils.NewRequestContext(p.RequestTimeout)
}

func (p *vscodeClientGenerator) GetPageWriter() pagewriter.Writer {
	return p.pagewriter
}

// KubeClient generates a kube client if it was not created before
func (p *vscodeClientGenerator) KubeClient() (kubernetes.Interface, error) {
	var err error
	p.kubeOnce.Do(func() {
		p.kubeClient, err = newKubeClient(p.KubeConfig, p.APIServerURL, p.RequestTimeout)
	})
	return p.kubeClient, err
}

func (p *vscodeClientGenerator) kubectl(args ...string) (string, error) {
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

func (p *vscodeClientGenerator) codeSpaceExists(client *vscodeClient) (bool, error) {

	var ns *v1.Namespace
	kubeclient, err := p.KubeClient()

	if err != nil {
		return false, err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, client.Name, metav1.GetOptions{})

	if kerr, ok := err.(*errors.StatusError); ok {
		if kerr.ErrStatus.Code == 404 {
			err = nil
		}
	}

	if err != nil || ns == nil || ns.Name == "" {
		return false, err
	}

	return true, nil
}

func (p *vscodeClientGenerator) waitNameSpaceReady(ctx *context.Context, client *vscodeClient) (bool, error) {
	var ns *v1.Namespace

	kubeclient, err := p.KubeClient()

	if err = utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		ns, err = kubeclient.CoreV1().Namespaces().Get(ctx, client.Name, metav1.GetOptions{})

		if err != nil {
			return false, err
		}

		return ns.Status.Phase == v1.NamespaceActive, nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (p *vscodeClientGenerator) waitIngressReady(ctx *context.Context, client *vscodeClient, name string) (bool, error) {
	var ing *networkingv1.Ingress
	kubeclient, err := p.KubeClient()

	if err = utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		if ing, err = kubeclient.NetworkingV1().Ingresses(client.Name).Get(ctx, name, metav1.GetOptions{}); err != nil {
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

func (p *vscodeClientGenerator) saveTemplate(yaml string) (string, error) {
	if f, err := os.CreateTemp("", "template.yml"); err != nil {
		return "", err
	} else {
		defer f.Close()

		_, err = f.WriteString(yaml)

		return f.Name(), err
	}
}

func (p *vscodeClientGenerator) deploymentReady(client *vscodeClient, name string) (bool, error) {
	var app *appv1.Deployment
	var pods *v1.PodList
	var startFailure = "app %s/%s start failure: %v"

	kubeclient, err := p.KubeClient()

	if err == nil {
		apps := kubeclient.AppsV1().Deployments(client.Name)

		if app, err = apps.Get(context.Background(), name, metav1.GetOptions{}); err == nil {

			for _, status := range app.Status.Conditions {
				if status.Type == appv1.DeploymentProgressing {
					if pods, err = kubeclient.CoreV1().Pods(client.Name).List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", p.cfg.VSCodeAppName)}); err == nil {
						for _, pod := range pods.Items {
							if pod.Status.Phase == v1.PodFailed {
								glog.Errorf(startFailure, client.Name, name, pod.Status.Message)

								return false, fmt.Errorf(startFailure, client.Name, name, pod.Status.Message)
							}

							for _, containerStatus := range pod.Status.ContainerStatuses {
								if !containerStatus.Ready && containerStatus.RestartCount > 4 {
									glog.Errorf(startFailure, client.Name, name, pod.Status.Message)

									return false, fmt.Errorf(startFailure, client.Name, name, pod.Status.Message)
								}
							}
						}
					}
				} else if status.Type == appv1.DeploymentAvailable {

					if b, e := strconv.ParseBool(string(status.Status)); e == nil {
						if b {
							glog.Debugf("app %s/%s marked ready with replicas=%d, read replicas=%d", client.Name, name, app.Status.Replicas, app.Status.ReadyReplicas)

							return app.Status.Replicas == app.Status.ReadyReplicas, nil
						}
					}

				} else if status.Type == appv1.DeploymentReplicaFailure {
					return false, fmt.Errorf("app %s/%s replica failure: %v", client.Name, name, status.String())
				}
			}
		}
	}

	return false, err
}

func (p *vscodeClientGenerator) waitDeploymentReady(ctx *context.Context, client *vscodeClient, name string) (bool, error) {

	if err := utils.PollImmediate(time.Second, p.ObjectReadyTimeout, func() (bool, error) {
		return p.deploymentReady(client, name)
	}); err != nil {
		err = fmt.Errorf("app %s/%s is not ready, %v", client.Name, name, err)

		return false, err
	}

	glog.Infof("The kubernetes app %s/%s is Ready", client.Name, name)

	return true, nil
}

func (p *vscodeClientGenerator) waitAppReady(client *vscodeClient) (bool, error) {
	ctx := p.newRequestContext()
	defer ctx.Cancel()

	glog.Infof("Wait kubernetes app %s/%s to be ready", client.Name, deploymentName)

	if ready, err := p.waitNameSpaceReady(ctx, client); err != nil || !ready {
		return ready, err
	} else if ready, err := p.waitDeploymentReady(ctx, client, p.cfg.VSCodeAppName); err != nil || !ready {
		return ready, err
	} else {
		return p.waitIngressReady(ctx, client, p.cfg.VSCodeAppName)
	}

}

func (p *vscodeClientGenerator) getNameSpaceConfigMapAndSecretsAndTls(client *vscodeClient) (string, string, string, string, error) {
	var gotCM *v1.ConfigMap
	var gotSecret *v1.Secret
	var tls *v1.Secret
	var ssh *v1.Secret
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      client.Name,
			Namespace: client.Name,
			Labels: map[string]string{
				appLabel:    deploymentName,
				vscodeLabel: client.Name,
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
			Name:      client.Name,
			Namespace: client.Name,
			Labels: map[string]string{
				appLabel:    deploymentName,
				vscodeLabel: client.Name,
			},
		},
	}

	kubeclient, err := p.KubeClient()

	if err != nil {
		return "", "", "", "", err
	}

	ctx := p.newRequestContext()
	defer ctx.Cancel()

	if gotCM, err = kubeclient.CoreV1().ConfigMaps(p.cfg.VSCodeServerNameSpace).Get(ctx, client.Name, metav1.GetOptions{}); err != nil {
		glog.Debugf("No configmap %s/%s found, %v", p.cfg.VSCodeServerNameSpace, client.Name, err)
	} else {
		glog.Debugf("Configmap %s/%s found", p.cfg.VSCodeServerNameSpace, client.Name)

		cm.Data = gotCM.Data
		cm.BinaryData = gotCM.BinaryData
	}

	if gotSecret, err = kubeclient.CoreV1().Secrets(p.cfg.VSCodeServerNameSpace).Get(ctx, p.cfg.VSCodeIngressSshSecret, metav1.GetOptions{}); err != nil {
		glog.Debugf("No secret %s/%s found, %v", p.cfg.VSCodeServerNameSpace, p.cfg.VSCodeIngressTlsSecret, err)
	} else {
		glog.Debugf("SSH %s/%s found", p.cfg.VSCodeServerNameSpace, p.cfg.VSCodeIngressTlsSecret)

		ssh = &v1.Secret{
			Type: "Opaque",
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.cfg.VSCodeIngressTlsSecret,
				Namespace: client.Name,
				Labels: map[string]string{
					appLabel:    deploymentName,
					vscodeLabel: client.Name,
				},
			},
			Data: gotSecret.Data,
		}
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
				Namespace: client.Name,
				Labels: map[string]string{
					appLabel:    deploymentName,
					vscodeLabel: client.Name,
				},
			},
			Data: gotSecret.Data,
		}
	}

	if gotSecret, err = kubeclient.CoreV1().Secrets(p.cfg.VSCodeServerNameSpace).Get(ctx, client.Name, metav1.GetOptions{}); err != nil {
		glog.Debugf("No secret %s/%s found, %v", p.cfg.VSCodeServerNameSpace, client.Name, err)
	} else {
		glog.Debugf("Secret %s/%s found", p.cfg.VSCodeServerNameSpace, client.Name)

		secret.Data = gotSecret.Data
	}

	return encodeToYaml(cm), encodeToYaml(secret), encodeToYaml(ssh), encodeToYaml(tls), nil
}

func (p *vscodeClientGenerator) applyTemplate(yaml string) (err error) {
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

func (p *vscodeClientGenerator) deleteUserCodespace(client *vscodeClient) error {
	var out string
	var err error

	client.Status = clientStatusDeleting

	if out, err = p.kubectl("kubectl", "delete", "ns", client.Name); err != nil {
		client.Status = clientStatusErrored

		glog.Errorf("kubectl got an error: %s, %v", out, err)
	} else {
		client.Status = clientStatusDeleted
	}

	return err
}

func (p *vscodeClientGenerator) createUserCodespace(client *vscodeClient) error {
	var template []byte
	var err error
	var yaml string
	var configmap, secret, ssh, tls string
	ready := false

	client.Status = clientStatusCreating

	if configmap, secret, ssh, tls, err = p.getNameSpaceConfigMapAndSecretsAndTls(client); err != nil {
		glog.Errorf("Unable to get config map and secret for user: %s, %v", client.Name, err)
	} else {

		mapping := func(name string) string {
			var value string
			var found bool

			if value, found = os.LookupEnv(name); found {
				return value
			} else {
				switch name {
				case "ACCOUNT_NAMESPACE":
					return client.Name

				case "ACCOUNT_CONFIGMAP":
					return configmap

				case "ACCOUNT_SECRET":
					return secret

				case "ACCOUNT_SSSH_KEY":
					return ssh

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

				case "VSCODE_RUNNING_USER":
					return client.Name

				case "VSCODE_USER_HOME":
					return "/home/" + client.Name

				default:
					return fmt.Sprintf("$%s", name)
				}
			}
		}

		if template, err = os.ReadFile(p.cfg.VSCodeTemplatePath); err == nil {
			if yaml, err = envsubst.Eval(string(template), mapping); err == nil {
				if err = p.applyTemplate(yaml); err == nil {
					if ready, err = p.waitAppReady(client); err != nil || !ready {
						if err == nil {
							err = fmt.Errorf("vscode-server not ready for user: %s", client.Name)
						}

						p.deleteUserCodespace(client)
					}
				}
			}
		}
	}

	if err == nil {
		client.Status = clientStatusCreated
	} else {
		client.Status = clientStatusErrored
	}

	return err
}

func (p *vscodeClientGenerator) CodeSpaceExists(w http.ResponseWriter, req *http.Request) {

	if currentUser, found := p.getRequestUser(req); found {

		defer currentUser.lock()

		if exist, err := p.codeSpaceExists(currentUser); err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(utils.ToJSON(&ExistsResponse{
				Status: 0,
				Result: ExistsObject{
					Codespace: currentUser.Name,
					Exists:    exist,
				},
			})))
		} else {
			serveError(w, err)
		}
	} else {
		serveMissingUser(w)
	}
}

func (p *vscodeClientGenerator) CreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := p.getRequestUser(req); found {

		defer currentUser.lock()

		codespaceCreated := func(exists bool) {
			domain := cookies.GetCookieDomain(req, p.cfg.VSCodeCookieDomain)

			// If nothing matches, create the cookie with the shortest domain
			if domain == "" && len(p.cfg.VSCodeCookieDomain) > 0 {
				glog.Errorf("Warning: request host %q did not match any of the specific cookie domains of %q",
					requestutil.GetRequestHost(req),
					strings.Join(p.cfg.VSCodeCookieDomain, ","),
				)
				domain = p.cfg.VSCodeCookieDomain[len(p.cfg.VSCodeCookieDomain)-1]
			}

			http.SetCookie(w, &http.Cookie{
				Name:   "vscode_user",
				Domain: domain,
				Value:  currentUser.Name,
			})

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(utils.ToJSON(&ExistsResponse{
				Status: 0,
				Result: ExistsObject{
					Codespace: currentUser.Name,
					Exists:    exists,
				},
			})))
		}

		if currentUser.Status == clientStatusNone {
			if exist, err := p.codeSpaceExists(currentUser); err == nil {
				if !exist {
					if err = p.createUserCodespace(currentUser); err != nil {
						currentUser.Status = clientStatusErrored
						serveError(w, err)
						return
					}
				}

				currentUser.Status = clientStatusCreated

				codespaceCreated(exist)
			} else {
				serveError(w, err)
			}
		} else if currentUser.Status == clientStatusCreated {
			codespaceCreated(true)
		} else if currentUser.Status == clientStatusDeleted {
			codespaceCreated(false)
		} else if currentUser.Status == clientStatusCreating {
			serveOperationRunning(w, "Already creating")
		} else {
			serveError(w, fmt.Errorf("fatal error already occured"))
		}
	} else {
		serveMissingUser(w)
	}
}

func (p *vscodeClientGenerator) DeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := p.getRequestUser(req); found {

		var err error
		var exists bool

		defer currentUser.lock()

		codespaceDeleted := func(deleted bool) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(utils.ToJSON(&DeletedResponse{
				Status: 0,
				Result: DeletedObject{
					Codespace: currentUser.Name,
					Deleted:   deleted,
				},
			})))

			currentUser.Status = clientStatusDeleted
		}

		if currentUser.Status == clientStatusCreated {
			if exists, err = p.codeSpaceExists(currentUser); err == nil {

				if exists {
					if err := p.deleteUserCodespace(currentUser); err == nil {
						codespaceDeleted(true)
					} else {
						currentUser.Status = clientStatusErrored

						serveError(w, err)
					}
				} else {
					currentUser.Status = clientStatusDeleted

					serveUserNotFound(w, currentUser.Name)
				}

			} else {
				currentUser.Status = clientStatusNone

				serveError(w, err)
			}

		} else if currentUser.Status == clientStatusDeleting {
			serveOperationRunning(w, "Already deleting")
		} else if currentUser.Status == clientStatusDeleted {
			codespaceDeleted(true)
		} else {
			serveError(w, fmt.Errorf("fatal error already occured"))
		}
	} else {
		serveMissingUser(w)
	}
}

func (p *vscodeClientGenerator) CodeSpaceReady(w http.ResponseWriter, req *http.Request) {

	if currentUser, found := p.getRequestUser(req); found {

		defer currentUser.lock()

		if exist, err := p.codeSpaceExists(currentUser); err == nil {
			if exist {

				if ready, err := p.deploymentReady(currentUser, p.cfg.VSCodeAppName); err == nil {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(utils.ToJSON(&ReadyResponse{
						Status: 0,
						Result: ReadyObject{
							Codespace: currentUser.Name,
							Ready:     ready,
						},
					})))
				} else {
					serveError(w, err)
				}

			} else {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(utils.ToJSON(&ErrorResponse{
					Status: -1,
					Error: ErrorObject{
						Code:   http.StatusNotFound,
						Reason: err.Error(),
					},
				})))
			}
		} else {
			serveError(w, err)
		}
	} else {
		serveMissingUser(w)
	}
}

func (p *vscodeClientGenerator) ClientShouldCreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := p.getRequestUser(req); found {
		var err error
		var exists bool

		defer currentUser.lock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})
		} else if exists {
			p.codeSpaceCreated(currentUser, w, req)
		} else {
			referer := req.Referer()

			if referer == "" {
				referer = "/"
			}

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:       http.StatusOK,
				RedirectURL:  referer,
				AppError:     fmt.Sprintf("Create codespace for user %s ?", currentUser.Name),
				ButtonText:   "Create",
				ButtonCancel: "Cancel",
				ButtonAction: "/create",
				ButtonTarget: "_blank",
				ButtonMethod: "GET",
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientDeleteCodeSpace HTML action do ask should delete codespace
func (p *vscodeClientGenerator) ClientShouldDeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := p.getRequestUser(req); found {
		var err error
		var exists bool

		defer currentUser.lock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})

		} else if exists {

			referer := req.Referer()

			if referer == "" {
				referer = "/"
			}

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:       http.StatusOK,
				RedirectURL:  referer,
				AppError:     fmt.Sprintf("Delete user %s ?", currentUser.Name),
				ButtonText:   "Delete",
				ButtonCancel: "Cancel",
				ButtonAction: "/delete",
				ButtonMethod: "POST",
			})

		} else {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusNotFound,
				AppError: fmt.Sprintf(userNotFound, currentUser.Name),
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientDeleteCodeSpace HTML action do delete codespace
func (p *vscodeClientGenerator) ClientDeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := p.getRequestUser(req); found {
		var err error
		var exists bool

		defer currentUser.lock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})

		} else if exists {

			if err = p.deleteUserCodespace(currentUser); err == nil {
				p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
					Status:       http.StatusOK,
					RedirectURL:  p.cfg.VSCodeSignoutURL,
					AppError:     fmt.Sprintf("User %s deleted", currentUser.Name),
					ButtonText:   "Sign out",
					ButtonCancel: "-",
					ButtonAction: p.cfg.VSCodeSignoutURL,
				})

			} else {
				p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
					Status:   http.StatusInternalServerError,
					AppError: fmt.Sprintf("Unable to delete user: %s", currentUser.Name),
					Messages: []interface{}{
						errorPage,
						err,
					},
				})
			}

		} else {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusNotFound,
				AppError: fmt.Sprintf(userNotFound, currentUser.Name),
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

func (p *vscodeClientGenerator) codeSpaceCookie(currentUser *vscodeClient, w http.ResponseWriter, req *http.Request) {
	domain := cookies.GetCookieDomain(req, p.cfg.VSCodeCookieDomain)

	// If nothing matches, create the cookie with the shortest domain
	if domain == "" && len(p.cfg.VSCodeCookieDomain) > 0 {
		glog.Errorf("Warning: request host %q did not match any of the specific cookie domains of %q",
			requestutil.GetRequestHost(req),
			strings.Join(p.cfg.VSCodeCookieDomain, ","),
		)
		domain = p.cfg.VSCodeCookieDomain[len(p.cfg.VSCodeCookieDomain)-1]
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "vscode_user",
		Domain: domain,
		Value:  currentUser.Name,
	})
}

func (p *vscodeClientGenerator) codeSpaceCreated(currentUser *vscodeClient, w http.ResponseWriter, req *http.Request) {
	var redirect *url.URL

	p.codeSpaceCookie(currentUser, w, req)

	if p.cfg.RedirectURL != "" {
		redirect, _ = url.Parse(fmt.Sprintf(p.cfg.RedirectURL, currentUser, currentUser))
	} else {
		redirect = &url.URL{
			Host:   requestutil.GetRequestHost(req),
			Scheme: requestutil.GetRequestProto(req),
			Path:   fmt.Sprintf("/%s?folder=/workspace", currentUser.Name),
		}
	}

	http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)
}

// ClientCreateCodeSpace HTML action do create codespace
func (p *vscodeClientGenerator) ClientCreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	var err error
	var exists bool

	if currentUser, found := p.getRequestUser(req); found {
		defer currentUser.lock()

		if currentUser.Status == clientStatusNone {
			if exists, err = p.codeSpaceExists(currentUser); err != nil {

				p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
					Status:   http.StatusInternalServerError,
					AppError: fmt.Sprintf(userNotFound, currentUser.Name),
					Messages: []interface{}{
						errorPage,
						err,
					},
				})

				return
			}

			if !exists {
				if err = p.createUserCodespace(currentUser); err != nil {

					p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
						Status:   http.StatusInternalServerError,
						AppError: fmt.Sprintf("Could not create codespace for user: %s", currentUser.Name),
						Messages: []interface{}{
							errorPage,
							err,
						},
					})

					return
				}

			}

			p.codeSpaceCreated(currentUser, w, req)
		} else if currentUser.Status == clientStatusCreated {
			p.codeSpaceCreated(currentUser, w, req)
		} else if currentUser.Status == clientStatusCreating {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusAlreadyReported,
				AppError: fmt.Sprintf("codespace for user: %s is creating", currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})
		} else if currentUser.Status == clientStatusDeleting {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusAlreadyReported,
				AppError: fmt.Sprintf("codespace for user: %s is deleting", currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})
		} else if currentUser.Status == clientStatusDeleted {
			p.codeSpaceCookie(currentUser, w, req)

			http.Redirect(w, req, "/", http.StatusTemporaryRedirect)
		} else {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusNotAcceptable,
				AppError: fmt.Sprintf("codespace for user: %s is in error state", currentUser.Name),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientCreateCodeSpace HTML action do report missing http header
func (p *vscodeClientGenerator) ClientRequestUserMissing(w http.ResponseWriter) {
	p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
		Status:   http.StatusPreconditionRequired,
		AppError: "Missing header: X-Auth-Request-User",
	})
}
