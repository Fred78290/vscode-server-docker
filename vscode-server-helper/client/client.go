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
	deploymentName = "vscode-server"
	ingressName    = "vscode-server-%s"
	contentType    = "Content-Type"
	textPlain      = "text/plain"
	errorPage      = "Error occured: %v"
	userNotFound   = "Could not find codespace for user: %s"
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
	pagewriter         pagewriter.Writer
	kubeOnce           sync.Once
	lock               sync.Mutex
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
		return &SingletonClientGenerator{
			KubeConfig:         cfg.KubeConfig,
			APIServerURL:       cfg.APIServerURL,
			RequestTimeout:     cfg.RequestTimeout,
			ObjectReadyTimeout: cfg.ObjectReadyTimeout,
			DeletionTimeout:    cfg.DeletionTimeout,
			MaxGracePeriod:     cfg.MaxGracePeriod,
			cfg:                cfg,
			pagewriter:         pageWriter,
		}
	}

	return nil
}

func getRequestUser(req *http.Request) (string, bool) {
	if user, found := req.Header[types.AuthRequestUserHeader]; found {
		return strings.ToLower(user[0]), true
	}

	return "", false
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

func (p *SingletonClientGenerator) newRequestContext() *context.Context {
	return utils.NewRequestContext(p.RequestTimeout)
}

func (p *SingletonClientGenerator) GetPageWriter() pagewriter.Writer {
	return p.pagewriter
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

func (p *SingletonClientGenerator) codeSpaceExists(userName string) (bool, error) {

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

func (p *SingletonClientGenerator) deploymentReady(userName, name string) (bool, error) {
	var app *appv1.Deployment
	kubeclient, err := p.KubeClient()

	if err == nil {
		apps := kubeclient.AppsV1().Deployments(userName)

		if app, err = apps.Get(context.Background(), name, metav1.GetOptions{}); err == nil {

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
	}

	return false, err
}

func (p *SingletonClientGenerator) waitDeploymentReady(ctx *context.Context, userName, name string) (bool, error) {
	var app *appv1.Deployment
	kubeclient, err := p.KubeClient()

	if err == nil {
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
	}

	return true, err
}

func (p *SingletonClientGenerator) waitAppReady(userName string) (bool, error) {
	ctx := p.newRequestContext()
	defer ctx.Cancel()

	glog.Infof("Wait kubernetes app %s/%s to be ready", userName, deploymentName)

	if ready, err := p.waitNameSpaceReady(ctx, userName); err != nil || !ready {
		return ready, err
	} else if ready, err := p.waitDeploymentReady(ctx, userName, p.cfg.VSCodeAppName); err != nil || !ready {
		return ready, err
	} else {
		return p.waitIngressReady(ctx, userName, p.cfg.VSCodeAppName)
	}

}

func (p *SingletonClientGenerator) getNameSpaceConfigMapAndSecretsAndTls(userName string) (string, string, string, string, error) {
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
		return "", "", "", "", err
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
				Namespace: userName,
				Labels: map[string]string{
					appLabel: userName,
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

	return encodeToYaml(cm), encodeToYaml(secret), encodeToYaml(ssh), encodeToYaml(tls), nil
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
	var configmap, secret, ssh, tls string
	ready := false

	if configmap, secret, ssh, tls, err = p.getNameSpaceConfigMapAndSecretsAndTls(userName); err != nil {
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
				return userName

			case "VSCODE_USER_HOME":
				return "/home/" + userName

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

func (p *SingletonClientGenerator) CodeSpaceExists(w http.ResponseWriter, req *http.Request) {

	if currentUser, found := getRequestUser(req); found {

		p.lock.Lock()
		defer p.lock.Unlock()

		if exist, err := p.codeSpaceExists(currentUser); err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(utils.ToJSON(&ExistsResponse{
				Status: 0,
				Result: ExistsObject{
					Codespace: currentUser,
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

func (p *SingletonClientGenerator) CreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := getRequestUser(req); found {

		p.lock.Lock()
		defer p.lock.Unlock()

		if exist, err := p.codeSpaceExists(currentUser); err == nil {
			if !exist {
				if err = p.createUserCodespace(currentUser); err != nil {
					serveError(w, err)
					return
				}
			}

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
				Value:  currentUser,
			})

			w.WriteHeader(http.StatusOK)
			w.Write([]byte(utils.ToJSON(&ExistsResponse{
				Status: 0,
				Result: ExistsObject{
					Codespace: currentUser,
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

func (p *SingletonClientGenerator) DeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := getRequestUser(req); found {

		var err error
		var exists bool

		p.lock.Lock()
		defer p.lock.Unlock()

		if exists, err = p.codeSpaceExists(currentUser); err == nil {

			if exists {
				if err := p.deleteUserCodespace(currentUser); err == nil {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(utils.ToJSON(&DeletedResponse{
						Status: 0,
						Result: DeletedObject{
							Codespace: currentUser,
							Deleted:   true,
						},
					})))
				} else {
					serveError(w, err)
				}
			} else {
				serveUserNotFound(w, currentUser)
			}

		} else {
			serveError(w, err)
		}
	} else {
		serveMissingUser(w)
	}
}

func (p *SingletonClientGenerator) CodeSpaceReady(w http.ResponseWriter, req *http.Request) {

	if currentUser, found := getRequestUser(req); found {

		p.lock.Lock()
		defer p.lock.Unlock()

		if exist, err := p.codeSpaceExists(currentUser); err == nil {
			if exist {

				if ready, err := p.deploymentReady(currentUser, p.cfg.VSCodeAppName); err == nil {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(utils.ToJSON(&ReadyResponse{
						Status: 0,
						Result: ReadyObject{
							Codespace: currentUser,
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

func (p *SingletonClientGenerator) ClientShouldCreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := getRequestUser(req); found {
		var err error
		var exists bool

		p.lock.Lock()
		defer p.lock.Unlock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser),
				Messages: []interface{}{
					errorPage,
					err,
				},
			})
		} else if exists {
			var redirect *url.URL

			if p.cfg.RedirectURL != "" {
				redirect, _ = url.Parse(fmt.Sprintf(p.cfg.RedirectURL, currentUser, currentUser))
			} else {
				redirect = &url.URL{
					Host:   requestutil.GetRequestHost(req),
					Scheme: requestutil.GetRequestProto(req),
					Path:   fmt.Sprintf("/%s?folder=/workspace", currentUser),
				}
			}

			http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)
		} else {
			referer := req.Referer()

			if referer == "" {
				referer = "/"
			}

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:       http.StatusOK,
				RedirectURL:  referer,
				AppError:     fmt.Sprintf("Create codespace for user %s ?", currentUser),
				ButtonText:   "Create",
				ButtonCancel: "Cancel",
				ButtonAction: "/create",
				ButtonMethod: "GET",
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientDeleteCodeSpace HTML action do ask should delete codespace
func (p *SingletonClientGenerator) ClientShouldDeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := getRequestUser(req); found {
		var err error
		var exists bool

		p.lock.Lock()
		defer p.lock.Unlock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser),
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
				AppError:     fmt.Sprintf("Delete user %s ?", currentUser),
				ButtonText:   "Delete",
				ButtonCancel: "Cancel",
				ButtonAction: "/delete",
				ButtonMethod: "POST",
			})

		} else {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusNotFound,
				AppError: fmt.Sprintf(userNotFound, currentUser),
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientDeleteCodeSpace HTML action do delete codespace
func (p *SingletonClientGenerator) ClientDeleteCodeSpace(w http.ResponseWriter, req *http.Request) {
	if currentUser, found := getRequestUser(req); found {
		var err error
		var exists bool

		p.lock.Lock()
		defer p.lock.Unlock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser),
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
					AppError:     fmt.Sprintf("User %s deleted", currentUser),
					ButtonText:   "Sign out",
					ButtonCancel: "-",
					ButtonAction: p.cfg.VSCodeSignoutURL,
				})

			} else {
				p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
					Status:   http.StatusInternalServerError,
					AppError: fmt.Sprintf("Unable to delete user: %s", currentUser),
					Messages: []interface{}{
						errorPage,
						err,
					},
				})
			}

		} else {
			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusNotFound,
				AppError: fmt.Sprintf(userNotFound, currentUser),
			})
		}
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientCreateCodeSpace HTML action do create codespace
func (p *SingletonClientGenerator) ClientCreateCodeSpace(w http.ResponseWriter, req *http.Request) {
	var err error
	var exists bool
	var redirect *url.URL

	if currentUser, found := getRequestUser(req); found {

		p.lock.Lock()
		defer p.lock.Unlock()

		if exists, err = p.codeSpaceExists(currentUser); err != nil {

			p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
				Status:   http.StatusInternalServerError,
				AppError: fmt.Sprintf(userNotFound, currentUser),
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
					AppError: fmt.Sprintf("Could not create codespace for user: %s", currentUser),
					Messages: []interface{}{
						errorPage,
						err,
					},
				})

				return
			}

		}

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
			Value:  currentUser,
		})

		if p.cfg.RedirectURL != "" {
			redirect, _ = url.Parse(fmt.Sprintf(p.cfg.RedirectURL, currentUser, currentUser))
		} else {
			redirect = &url.URL{
				Host:   requestutil.GetRequestHost(req),
				Scheme: requestutil.GetRequestProto(req),
				Path:   fmt.Sprintf("/%s?folder=/workspace", currentUser, currentUser),
			}
		}

		http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)
	} else {
		p.ClientRequestUserMissing(w)
	}
}

// ClientCreateCodeSpace HTML action do report missing http header
func (p *SingletonClientGenerator) ClientRequestUserMissing(w http.ResponseWriter) {
	p.pagewriter.WriteErrorPage(w, pagewriter.ErrorPageOpts{
		Status:   http.StatusPreconditionRequired,
		AppError: "Missing header: X-Auth-Request-User",
	})
}
