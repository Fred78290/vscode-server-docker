package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/alecthomas/kingpin"
	requestutil "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/requests/util"
	glog "github.com/sirupsen/logrus"
)

type Config struct {
	Listen                string
	RedirectURL           string
	KubeConfig            string
	VSCodeTemplatePath    string
	VSCodeServerNameSpace string
	RequestTimeout        time.Duration
	DeletionTimeout       time.Duration
	MaxGracePeriod        time.Duration
	NodeReadyTimeout      time.Duration
	MinCpus               string
	MinMemory             string
	MaxCpus               string
	MaxMemory             string
	UseTls                bool
	TlsKey                string
	TlsCert               string
	PrefixPath            string
	DisplayVersion        bool
	LogFormat             string
	LogLevel              string
	redirectURL           *url.URL
}

const (
	DefaultMaxGracePeriod    time.Duration = 120 * time.Second
	DefaultMaxRequestTimeout time.Duration = 120 * time.Second
	DefaultMaxDeletionPeriod time.Duration = 300 * time.Second
	DefaultNodeReadyTimeout  time.Duration = 300 * time.Second
)

func NewConfig() *Config {
	return &Config{
		Listen:                "0.0.0.0:8000",
		VSCodeTemplatePath:    "/vscode-server-helper/template.yaml",
		VSCodeServerNameSpace: "vscode-server",
		PrefixPath:            "/create-space",
		RequestTimeout:        DefaultMaxRequestTimeout,
		DeletionTimeout:       DefaultMaxDeletionPeriod,
		MaxGracePeriod:        DefaultMaxGracePeriod,
		NodeReadyTimeout:      DefaultNodeReadyTimeout,
		MinCpus:               "500m",
		MinMemory:             "512Mi",
		MaxCpus:               "4",
		MaxMemory:             "8G",
		DisplayVersion:        false,
		LogFormat:             "text",
		LogLevel:              glog.InfoLevel.String(),
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
	app := kingpin.New("vscode-server-helper", "Kubernetes AWS autoscaler create EC2 instances at demand for autoscaling.\n\nNote that all flags may be replaced with env vars - `--flag` -> `VMWARE_AUTOSCALER_FLAG=1` or `--flag value` -> `VMWARE_AUTOSCALER_FLAG=value`")

	app.HelpFlag.Short('h')
	app.DefaultEnvars()

	app.Flag("version", "Display version and exit").BoolVar(&cfg.DisplayVersion)

	app.Flag("log-format", "The format in which log messages are printed (default: text, options: text, json)").Default(cfg.LogFormat).EnumVar(&cfg.LogFormat, "text", "json")
	app.Flag("log-level", "Set the level of logging. (default: info, options: panic, debug, info, warning, error, fatal").Default(cfg.LogLevel).EnumVar(&cfg.LogLevel, allLogLevelsAsStrings()...)

	app.Flag("listen", "Listen address").Default(cfg.Listen).StringVar(&cfg.Listen)
	app.Flag("prefix", "the url root path that this helper should be nested under").Default(cfg.PrefixPath).StringVar(&cfg.PrefixPath)
	app.Flag("vscode-namespace", "the name space of vscode-server").Default(cfg.VSCodeServerNameSpace).StringVar(&cfg.VSCodeServerNameSpace)
	app.Flag("vscode-template-file", "the template used to create vscode-server").Default(cfg.VSCodeTemplatePath).StringVar(&cfg.VSCodeTemplatePath)

	app.Flag("use-tls", "Tell to use https instead http").Default("false").BoolVar(&cfg.UseTls)
	app.Flag("tls-key-file", "Locate the tls key file").Default(cfg.TlsKey).StringVar(&cfg.TlsKey)
	app.Flag("tls-cert-file", "Locate the tls cert file").Default(cfg.TlsCert).StringVar(&cfg.TlsCert)

	app.Flag("kubeconfig", "Retrieve target cluster configuration from a Kubernetes configuration file (default: auto-detect)").Default(cfg.KubeConfig).StringVar(&cfg.KubeConfig)
	app.Flag("request-timeout", "Request timeout when calling Kubernetes APIs. 0s means no timeout").Default(DefaultMaxRequestTimeout.String()).DurationVar(&cfg.RequestTimeout)
	app.Flag("deletion-timeout", "Deletion timeout when delete node. 0s means no timeout").Default(DefaultMaxDeletionPeriod.String()).DurationVar(&cfg.DeletionTimeout)
	app.Flag("node-ready-timeout", "Node ready timeout to wait for a node to be ready. 0s means no timeout").Default(DefaultNodeReadyTimeout.String()).DurationVar(&cfg.NodeReadyTimeout)
	app.Flag("max-grace-period", "Maximum time evicted pods will be given to terminate gracefully.").Default(DefaultMaxGracePeriod.String()).DurationVar(&cfg.MaxGracePeriod)

	app.Flag("min-cpus", "Limits: minimum cpu (default: 1)").Default(cfg.MinCpus).StringVar(&cfg.MinCpus)
	app.Flag("max-cpus", "Limits: max cpu (default: 24)").Default(cfg.MaxCpus).StringVar(&cfg.MaxCpus)
	app.Flag("min-memory", "Limits: minimum memory in MB (default: 1G)").Default(cfg.MinMemory).StringVar(&cfg.MinMemory)
	app.Flag("max-memory", "Limits: max memory in MB (default: 24G)").Default(cfg.MaxMemory).StringVar(&cfg.MaxMemory)

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

	return err
}

func (cfg *Config) String() string {
	return ToJSON(cfg)
}

func (cfg *Config) HandleRequest(w http.ResponseWriter, req *http.Request) error {
	redirect := *cfg.redirectURL

	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}

	if cfg.redirectURL.Host == "" {
		redirect.Host = requestutil.GetRequestHost(req)
		redirect.Scheme = requestutil.GetRequestProto(req)
	}

	http.Redirect(w, req, redirect.String(), http.StatusTemporaryRedirect)

	return nil
}
