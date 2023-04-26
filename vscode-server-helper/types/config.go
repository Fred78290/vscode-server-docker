package types

import (
	"fmt"
	"net/url"
	"time"

	"github.com/Fred78290/vscode-server-helper/utils"
	"github.com/alecthomas/kingpin"
	glog "github.com/sirupsen/logrus"
)

type Config struct {
	Listen                 string
	RedirectURL            string
	KubeConfig             string
	APIServerURL           string
	VSCodeIngressTlsSecret string
	VSCodeIngressSshSecret string
	VSCodeHostname         string
	VSCodeTemplatePath     string
	VSCodeServerNameSpace  string
	VSCodeAppName          string
	VSCodeSignoutURL       string
	VSCodeCookieDomain     []string
	RequestTimeout         time.Duration
	DeletionTimeout        time.Duration
	MaxGracePeriod         time.Duration
	ObjectReadyTimeout     time.Duration
	TemplatePath           string
	TemplateBanner         string
	TemplateFooter         string
	PersistentVolumeSize   string
	MinCpus                string
	MinMemory              string
	MaxCpus                string
	MaxMemory              string
	UseTls                 bool
	TlsKey                 string
	TlsCert                string
	PrefixPath             string
	DisplayVersion         bool
	LogFormat              string
	LogLevel               string
	Debug                  bool
	Version                string
}

const (
	DefaultMaxGracePeriod     time.Duration = 120 * time.Second
	DefaultMaxRequestTimeout  time.Duration = 120 * time.Second
	DefaultMaxDeletionPeriod  time.Duration = 300 * time.Second
	DefaultObjectReadyTimeout time.Duration = 300 * time.Second
)

func NewConfig(version string) *Config {
	return &Config{
		Listen:                 "0.0.0.0:8000",
		VSCodeHostname:         "localhost",
		VSCodeIngressTlsSecret: "vscode-server-ingress-tls",
		VSCodeIngressSshSecret: "vscode-server-sshd",
		VSCodeTemplatePath:     "/vscode-server-helper/template.yaml",
		VSCodeServerNameSpace:  "vscode-server",
		VSCodeAppName:          "vscode-server",
		VSCodeSignoutURL:       "/oauth2/sign_out?rd=/logout",
		PrefixPath:             "/create-space",
		RequestTimeout:         DefaultMaxRequestTimeout,
		DeletionTimeout:        DefaultMaxDeletionPeriod,
		MaxGracePeriod:         DefaultMaxGracePeriod,
		ObjectReadyTimeout:     DefaultObjectReadyTimeout,
		PersistentVolumeSize:   "10G",
		MinCpus:                "500m",
		MinMemory:              "512Mi",
		MaxCpus:                "4",
		MaxMemory:              "8G",
		DisplayVersion:         false,
		LogFormat:              "text",
		LogLevel:               glog.InfoLevel.String(),
		Version:                version,
	}
}

// allLogLevelsAsStrings returns all logrus levels as a list of strings
func allLogLevelsAsStrings() []string {
	var levels []string
	for _, level := range glog.AllLevels {
		levels = append(levels, level.String())
	}
	return levels
}

func (cfg *Config) ParseFlags(args []string, version string) error {
	app := kingpin.New("vscode-server-helper", "VSCode server helper to create codespace for user.\n\nNote that all flags may be replaced with env vars - `--flag` -> `VSCODE_SERVER_HELPER_FLAG=1` or `--flag value` -> `VSCODE_SERVER_HELPER_FLAG=value`")

	app.HelpFlag.Short('h')
	app.DefaultEnvars()

	app.Flag("version", "Display version and exit").BoolVar(&cfg.DisplayVersion)
	app.Flag("debug", "Debug mode").BoolVar(&cfg.Debug)
	app.Flag("custom-templates-dir", "path to custom html templates").Default(cfg.TemplatePath).StringVar(&cfg.TemplatePath)
	app.Flag("banner", "custom banner string. Use \"-\" to disable default banner.").Default(cfg.TemplateBanner).StringVar(&cfg.TemplateBanner)
	app.Flag("footer", "custom footer string. Use \"-\" to disable default footer.").Default(cfg.TemplateFooter).StringVar(&cfg.TemplateFooter)

	app.Flag("log-format", "The format in which log messages are printed (default: text, options: text, json)").Default(cfg.LogFormat).EnumVar(&cfg.LogFormat, "text", "json")
	app.Flag("log-level", "Set the level of logging. (default: info, options: panic, debug, info, warning, error, fatal").Default(cfg.LogLevel).EnumVar(&cfg.LogLevel, allLogLevelsAsStrings()...)

	app.Flag("listen", "Listen address").Default(cfg.Listen).StringVar(&cfg.Listen)
	app.Flag("prefix", "the url root path that this helper should be nested under").Default(cfg.PrefixPath).StringVar(&cfg.PrefixPath)
	app.Flag("redirect-url", "redirect url if host is different of caller").Default(cfg.RedirectURL).StringVar(&cfg.RedirectURL)

	app.Flag("vscode-ingress-secret-tls", "the ingress tls used for vscode-server").Default(cfg.VSCodeIngressTlsSecret).StringVar(&cfg.VSCodeIngressTlsSecret)
	app.Flag("vscode-secret-ssh", "the ssh key used for vscode-server").Default(cfg.VSCodeIngressSshSecret).StringVar(&cfg.VSCodeIngressSshSecret)
	app.Flag("vscode-namespace", "the name space of vscode-server").Default(cfg.VSCodeServerNameSpace).StringVar(&cfg.VSCodeServerNameSpace)
	app.Flag("vscode-app-name", "the deployment and ingress name of vscode-server").Default(cfg.VSCodeAppName).StringVar(&cfg.VSCodeAppName)
	app.Flag("vscode-template-file", "the template used to create vscode-server").Default(cfg.VSCodeTemplatePath).StringVar(&cfg.VSCodeTemplatePath)
	app.Flag("vscode-hostname", "the hostname of vscode-server").Default(cfg.VSCodeHostname).StringVar(&cfg.VSCodeHostname)
	app.Flag("vscode-cookie-domain", "Optional cookie domains to force cookies to (ie: `.yourcompany.com`). The longest domain matching the request's host will be used (or the shortest cookie domain if there is no match).").StringsVar(&cfg.VSCodeCookieDomain)
	app.Flag("vscode-sign-out", "Optional signout URL").Default(cfg.VSCodeSignoutURL).StringVar(&cfg.VSCodeSignoutURL)

	app.Flag("use-tls", "Tell to use https instead http").Default("false").BoolVar(&cfg.UseTls)
	app.Flag("tls-key-file", "Locate the tls key file").Default(cfg.TlsKey).StringVar(&cfg.TlsKey)
	app.Flag("tls-cert-file", "Locate the tls cert file").Default(cfg.TlsCert).StringVar(&cfg.TlsCert)

	app.Flag("server", "The Kubernetes API server to connect to (default: auto-detect)").Default(cfg.APIServerURL).StringVar(&cfg.APIServerURL)
	app.Flag("kubeconfig", "Retrieve target cluster configuration from a Kubernetes configuration file (default: auto-detect)").Default(cfg.KubeConfig).StringVar(&cfg.KubeConfig)
	app.Flag("request-timeout", "Request timeout when calling Kubernetes APIs. 0s means no timeout").Default(DefaultMaxRequestTimeout.String()).DurationVar(&cfg.RequestTimeout)
	app.Flag("deletion-timeout", "Deletion timeout when delete node. 0s means no timeout").Default(DefaultMaxDeletionPeriod.String()).DurationVar(&cfg.DeletionTimeout)
	app.Flag("node-ready-timeout", "Node ready timeout to wait for a node to be ready. 0s means no timeout").Default(DefaultObjectReadyTimeout.String()).DurationVar(&cfg.ObjectReadyTimeout)
	app.Flag("max-grace-period", "Maximum time evicted pods will be given to terminate gracefully.").Default(DefaultMaxGracePeriod.String()).DurationVar(&cfg.MaxGracePeriod)

	app.Flag("volume-size", "Limits: persistent volume size (default: 500m)").Default(cfg.PersistentVolumeSize).StringVar(&cfg.PersistentVolumeSize)
	app.Flag("min-cpus", "Limits: minimum cpu (default: 500m)").Default(cfg.MinCpus).StringVar(&cfg.MinCpus)
	app.Flag("max-cpus", "Limits: max cpu (default: 4)").Default(cfg.MaxCpus).StringVar(&cfg.MaxCpus)
	app.Flag("min-memory", "Limits: minimum memory in MB (default: 512Mi)").Default(cfg.MinMemory).StringVar(&cfg.MinMemory)
	app.Flag("max-memory", "Limits: max memory in MB (default: 8G)").Default(cfg.MaxMemory).StringVar(&cfg.MaxMemory)

	_, err := app.Parse(args)
	if err != nil {
		return err
	}

	if cfg.RedirectURL != "" {
		_, err = url.Parse(fmt.Sprintf(cfg.RedirectURL, "test0", "test1"))
	}

	if !utils.FileExistAndReadable(cfg.VSCodeTemplatePath) {
		err = fmt.Errorf("template not found: %s", cfg.VSCodeTemplatePath)
	}

	return err
}

func (cfg *Config) String() string {
	return utils.ToJSON(cfg)
}
