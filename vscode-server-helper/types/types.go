package types

import (
	"net/http"

	"github.com/Fred78290/vscode-server-helper/pagewriter"
)

type ClientGenerator interface {
	GetPageWriter() pagewriter.Writer
	CodeSpaceExists(userName string) (bool, error)
	ShouldDeleteCodeSpace(userName string, w http.ResponseWriter, req *http.Request)
	DeleteCodeSpace(userName string, w http.ResponseWriter, req *http.Request)
	CreateCodeSpace(userName string, w http.ResponseWriter, req *http.Request)
	RequestUserMissing(w http.ResponseWriter, req *http.Request)
}
