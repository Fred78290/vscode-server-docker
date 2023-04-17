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
	VSCodeHostname         string
	VSCodeTemplatePath     string
	VSCodeServerNameSpace  string
	RequestTimeout         time.Duration
	DeletionTimeout        time.Duration
	MaxGracePeriod         time.Duration
	ObjectReadyTimeout     time.Duration
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
	redirectURL            *url.URL
}

const (
	DefaultMaxGracePeriod     time.Duration = 120 * time.Second
	DefaultMaxRequestTimeout  time.Duration = 120 * time.Second
	DefaultMaxDeletionPeriod  time.Duration = 300 * time.Second
	DefaultObjectReadyTimeout time.Duration = 300 * time.Second
)

func NewConfig() *Config {
	return &Config{
		Listen:                 "0.0.0.0:8000",
		VSCodeHostname:         "localhost",
		VSCodeIngressTlsSecret: "vscode-server-ingress-tls",
		VSCodeTemplatePath:     "/vscode-server-helper/template.yaml",
		VSCodeServerNameSpace:  "vscode-server",
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

	app.Flag("log-format", "The format in which log messages are printed (default: text, options: text, json)").Default(cfg.LogFormat).EnumVar(&cfg.LogFormat, "text", "json")
	app.Flag("log-level", "Set the level of logging. (default: info, options: panic, debug, info, warning, error, fatal").Default(cfg.LogLevel).EnumVar(&cfg.LogLevel, allLogLevelsAsStrings()...)

	app.Flag("listen", "Listen address").Default(cfg.Listen).StringVar(&cfg.Listen)
	app.Flag("prefix", "the url root path that this helper should be nested under").Default(cfg.PrefixPath).StringVar(&cfg.PrefixPath)
	app.Flag("redirect-url", "redirect url if host is different of caller").Default(cfg.RedirectURL).StringVar(&cfg.RedirectURL)

	app.Flag("vscode-ingress-secret-tls", "the ingress tls used for vscode-server").Default(cfg.VSCodeIngressTlsSecret).StringVar(&cfg.VSCodeIngressTlsSecret)
	app.Flag("vscode-namespace", "the name space of vscode-server").Default(cfg.VSCodeServerNameSpace).StringVar(&cfg.VSCodeServerNameSpace)
	app.Flag("vscode-template-file", "the template used to create vscode-server").Default(cfg.VSCodeTemplatePath).StringVar(&cfg.VSCodeTemplatePath)
	app.Flag("vscode-hostname", "the hostname of vscode-server").Default(cfg.VSCodeHostname).StringVar(&cfg.VSCodeHostname)

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
		cfg.redirectURL, err = url.Parse(cfg.RedirectURL)
	} else {
		cfg.redirectURL = &url.URL{
			Path: "/",
		}
	}

	if !utils.FileExistAndReadable(cfg.VSCodeTemplatePath) {
		err = fmt.Errorf("template not found: %s", cfg.VSCodeTemplatePath)
	}

	return err
}

func (cfg *Config) GetRedirectURL() *url.URL {
	return cfg.redirectURL
}

func (cfg *Config) String() string {
	return utils.ToJSON(cfg)
}
