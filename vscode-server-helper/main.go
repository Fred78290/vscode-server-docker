package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"

	"github.com/Fred78290/vscode-server-helper/client"
	"github.com/Fred78290/vscode-server-helper/pagewriter"
	"github.com/Fred78290/vscode-server-helper/types"
	glog "github.com/sirupsen/logrus"
)

var phVersion = "v0.0.0-unset"
var phBuildDate = ""

func serveRobots(generator types.ClientGenerator, w http.ResponseWriter, req *http.Request) {
	generator.GetPageWriter().WriteRobotsTxt(w, req)
}

func serveEcho(generator types.ClientGenerator, w http.ResponseWriter, req *http.Request) {
	generator.GetPageWriter().WriteErrorPage(w, pagewriter.ErrorPageOpts{
		Status:       http.StatusOK,
		AppError:     "Echo",
		ButtonText:   "OK",
		ButtonCancel: "-",
	})
}

func serveLogout(generator types.ClientGenerator, w http.ResponseWriter, req *http.Request) {
	generator.GetPageWriter().WriteErrorPage(w, pagewriter.ErrorPageOpts{
		Status:       http.StatusOK,
		AppError:     "User signed out",
		ButtonText:   "OK",
		ButtonCancel: "-",
	})
}

func route(cfg *types.Config) *http.ServeMux {
	mux := http.NewServeMux()
	generator := client.NewClientGenerator(cfg)

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		serveRobots(generator, w, req)
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
		serveEcho(generator, w, req)
	})

	mux.HandleFunc("/logout", func(w http.ResponseWriter, req *http.Request) {
		serveLogout(generator, w, req)
	})

	mux.HandleFunc("/delete", func(w http.ResponseWriter, req *http.Request) {
		generator.ClientDeleteCodeSpace(w, req)
	})

	mux.HandleFunc("/create", func(w http.ResponseWriter, req *http.Request) {
		generator.ClientCreateCodeSpace(w, req)
	})

	mux.HandleFunc("/api/create", func(w http.ResponseWriter, req *http.Request) {
		generator.CreateCodeSpace(w, req)
	})

	mux.HandleFunc("/api/delete", func(w http.ResponseWriter, req *http.Request) {
		generator.DeleteCodeSpace(w, req)
	})

	mux.HandleFunc("/api/exists", func(w http.ResponseWriter, req *http.Request) {
		generator.CodeSpaceExists(w, req)
	})

	mux.HandleFunc("/api/ready", func(w http.ResponseWriter, req *http.Request) {
		generator.CodeSpaceReady(w, req)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		generator.ClientShouldCreateCodeSpace(w, req)
	})

	return mux
}

func serve(cfg *types.Config) error {
	var err error

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: route(cfg),
	}

	if cfg.UseTls {
		srv.TLSNextProto = make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0)
		srv.TLSConfig = &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			},
		}

		err = srv.ListenAndServeTLS(cfg.TlsCert, cfg.TlsKey)
	} else {

		err = srv.ListenAndServe()
	}

	return err
}

func main() {
	cfg := types.NewConfig(phVersion)

	if err := cfg.ParseFlags(os.Args[1:], phVersion); err != nil {
		log.Fatalf("flag parsing error: %v", err)
	}

	ll, err := glog.ParseLevel(cfg.LogLevel)
	if err != nil {
		glog.Fatalf("failed to parse log level: %v", err)
	}

	glog.SetLevel(ll)

	if cfg.LogFormat == "json" {
		glog.SetFormatter(&glog.JSONFormatter{})
	}

	glog.Infof("config: %s", cfg)

	if cfg.DisplayVersion {
		glog.Infof("The current version is:%s, build at:%s", phVersion, phBuildDate)
	} else {
		err := serve(cfg)

		if err != nil {
			glog.Fatal("ListenAndServe: ", err)
		}
	}
}
