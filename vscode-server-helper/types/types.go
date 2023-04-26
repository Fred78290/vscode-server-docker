package types

import (
	"net/http"

	"github.com/Fred78290/vscode-server-helper/pagewriter"
)

const AuthRequestUserHeader = "X-Auth-Request-User"

type VsCodeServerApi interface {
	CodeSpaceExists(w http.ResponseWriter, req *http.Request)
	CodeSpaceReady(w http.ResponseWriter, req *http.Request)
	DeleteCodeSpace(w http.ResponseWriter, req *http.Request)
	CreateCodeSpace(w http.ResponseWriter, req *http.Request)
}

type ClientGenerator interface {
	VsCodeServerApi

	GetPageWriter() pagewriter.Writer

	ClientShouldCreateCodeSpace(w http.ResponseWriter, req *http.Request)
	ClientShouldDeleteCodeSpace(w http.ResponseWriter, req *http.Request)
	ClientDeleteCodeSpace(w http.ResponseWriter, req *http.Request)
	ClientCreateCodeSpace(w http.ResponseWriter, req *http.Request)
}
