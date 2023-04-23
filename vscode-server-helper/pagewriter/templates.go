package pagewriter

import (
	// Import embed to allow importing default page templates
	_ "embed"

	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	glog "github.com/sirupsen/logrus"
)

const (
	errorTemplateName  = "error.html"
	signInTemplateName = "sign_in.html"
)

//go:embed error.html
var defaultErrorTemplate string

// loadTemplates adds the Sign In and Error templates from the custom template
// directory, or uses the defaults if they do not exist or the custom directory
// is not provided.
func loadTemplates(customDir string) (*template.Template, error) {
	var err error

	t := template.New("").Funcs(template.FuncMap{
		"ToUpper": strings.ToUpper,
		"ToLower": strings.ToLower,
	})

	if t, err = addTemplate(t, customDir, errorTemplateName, defaultErrorTemplate); err != nil {
		return nil, fmt.Errorf("could not add Error template: %v", err)
	}

	return t, nil
}

// addTemplate will add the template from the custom directory if provided,
// else it will add the default template.
func addTemplate(t *template.Template, customDir, fileName, defaultTemplate string) (*template.Template, error) {
	var err error

	filePath := filepath.Join(customDir, fileName)

	if customDir != "" && isFile(filePath) {

		if t, err = t.ParseFiles(filePath); err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %v", filePath, err)
		}

		return t, nil
	}

	if t, err = t.Parse(defaultTemplate); err != nil {
		// This should not happen.
		// Default templates should be tested and so should never fail to parse.
		glog.Panic("Could not parse defaultTemplate: ", err)
	}

	return t, nil
}

// isFile checks if the file exists and checks whether it is a regular file.
// If either of these fail then it cannot be used as a template file.
func isFile(fileName string) bool {
	if info, err := os.Stat(fileName); err != nil {
		glog.Errorf("Could not load file %s: %v, will use default template", fileName, err)
		return false
	} else {
		return info.Mode().IsRegular()
	}
}
