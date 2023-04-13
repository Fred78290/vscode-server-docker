package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	glog "github.com/sirupsen/logrus"
)

var phVersion = "v0.0.0-unset"
var phBuildDate = ""

func main() {
	cfg := NewConfig()

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
		var err error

		mux := http.NewServeMux()
		mux.HandleFunc(cfg.PrefixPath, func(w http.ResponseWriter, req *http.Request) {
			cfg.HandleRequest(w, req)
		})

		mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "text/plain")

			var builder strings.Builder

			for name, headers := range req.Header {
				for _, h := range headers {
					fmt.Fprintf(&builder, "%v: %v\n", name, h)
				}
			}

			w.Write([]byte(builder.String()))
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

		if err != nil {
			glog.Fatal("ListenAndServe: ", err)
		}
	}
}
