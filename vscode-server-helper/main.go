package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Fred78290/vscode-server-helper/client"
	"github.com/Fred78290/vscode-server-helper/pagewriter"
	"github.com/Fred78290/vscode-server-helper/types"
	glog "github.com/sirupsen/logrus"
)

const authRequestUserHeader = "X-Auth-Request-User"

var phVersion = "v0.0.0-unset"
var phBuildDate = ""

func serve(cfg *types.Config) error {
	var err error

	generator := client.NewClientGenerator(cfg)

	mux := http.NewServeMux()

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, req *http.Request) {
		generator.GetPageWriter().WriteRobotsTxt(w, req)
	})

	mux.HandleFunc("/echo", func(w http.ResponseWriter, req *http.Request) {
		generator.GetPageWriter().WriteErrorPage(w, pagewriter.ErrorPageOpts{
			Status:       http.StatusOK,
			AppError:     "Echo",
			ButtonText:   "OK",
			ButtonCancel: "-",
		})
	})

	mux.HandleFunc("/logout", func(w http.ResponseWriter, req *http.Request) {
		generator.GetPageWriter().WriteErrorPage(w, pagewriter.ErrorPageOpts{
			Status:       http.StatusOK,
			AppError:     "User signed out",
			ButtonText:   "OK",
			ButtonCancel: "-",
		})
	})

	mux.HandleFunc("/delete", func(w http.ResponseWriter, req *http.Request) {
		if user, found := req.Header[authRequestUserHeader]; found {
			currentUser := strings.ToLower(user[0])

			if req.Method == "GET" {
				generator.ShouldDeleteCodeSpace(currentUser, w, req)
			} else if req.Method == "POST" {
				generator.DeleteCodeSpace(currentUser, w, req)
			}
		} else {
			generator.RequestUserMissing(w, req)
		}
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if user, found := req.Header[authRequestUserHeader]; found {
			generator.CreateCodeSpace(strings.ToLower(user[0]), w, req)
		} else {
			generator.RequestUserMissing(w, req)
		}
	})

	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: mux,
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
