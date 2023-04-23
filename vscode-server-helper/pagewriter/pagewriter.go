package pagewriter

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed default_logo.svg
var defaultLogoData string

type Writer interface {
	WriteErrorPage(rw http.ResponseWriter, opts ErrorPageOpts)
	ProxyErrorHandler(rw http.ResponseWriter, req *http.Request, proxyErr error)
	WriteRobotsTxt(rw http.ResponseWriter, req *http.Request)
}

// pageWriter implements the Writer interface
type pageWriter struct {
	*errorPageWriter
	*staticPageWriter
}

// Opts contains all options required to configure the template
// rendering within OAuth2 Proxy.
type Opts struct {
	// TemplatesPath is the path from which to load custom templates for the sign-in and error pages.
	TemplatesPath string

	// ProxyPrefix is the prefix under which pages are served.
	ProxyPrefix string

	// Footer is the footer to be displayed at the bottom of the page.
	// If not set, a default footer will be used.
	Footer string

	// Version is the version to be used in the default footer.
	Version string

	// Debug determines whether errors pages should be rendered with detailed
	// errors.
	Debug bool

	// CustomLogo is the path or URL to a logo to be displayed on the sign in page.
	// The logo can be either PNG, JPG/JPEG or SVG.
	// If a URL is used, image support depends on the browser.
	CustomLogo string
}

// NewWriter constructs a Writer from the options given to allow
// rendering of sign-in and error pages.
func NewWriter(opts Opts) (Writer, error) {
	if templates, err := loadTemplates(opts.TemplatesPath); err != nil {
		return nil, fmt.Errorf("error loading templates: %v", err)
	} else if logoData, err := loadCustomLogo(opts.CustomLogo); err != nil {
		return nil, fmt.Errorf("error loading logo: %v", err)
	} else {

		errorPage := &errorPageWriter{
			template: templates.Lookup("error.html"),
			footer:   opts.Footer,
			version:  opts.Version,
			debug:    opts.Debug,
			logoData: logoData,
		}

		if staticPages, err := newStaticPageWriter(opts.TemplatesPath, errorPage); err != nil {
			return nil, fmt.Errorf("error loading static page writer: %v", err)
		} else {
			return &pageWriter{
				errorPageWriter:  errorPage,
				staticPageWriter: staticPages,
			}, nil
		}
	}
}

// WriterFuncs is an implementation of the PageWriter interface based
// on override functions.
// If any of the funcs are not provided, a default implementation will be used.
// This is primarily for us in testing.
type WriterFuncs struct {
	ErrorPageFunc  func(rw http.ResponseWriter, opts ErrorPageOpts)
	ProxyErrorFunc func(rw http.ResponseWriter, req *http.Request, proxyErr error)
	RobotsTxtfunc  func(rw http.ResponseWriter, req *http.Request)
}

// WriteErrorPage implements the Writer interface.
// If the ErrorPageFunc is provided, this will be used, else a default
// implementation will be used.
func (w *WriterFuncs) WriteErrorPage(rw http.ResponseWriter, opts ErrorPageOpts) {
	if w.ErrorPageFunc != nil {
		w.ErrorPageFunc(rw, opts)
	} else {
		rw.WriteHeader(opts.Status)

		errMsg := fmt.Sprintf("%d - %v", opts.Status, opts.AppError)

		if _, err := rw.Write([]byte(errMsg)); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// ProxyErrorHandler implements the Writer interface.
// If the ProxyErrorFunc is provided, this will be used, else a default
// implementation will be used.
func (w *WriterFuncs) ProxyErrorHandler(rw http.ResponseWriter, req *http.Request, proxyErr error) {
	if w.ProxyErrorFunc != nil {
		w.ProxyErrorFunc(rw, req, proxyErr)
		return
	} else {
		w.WriteErrorPage(rw, ErrorPageOpts{
			Status:   http.StatusBadGateway,
			AppError: proxyErr.Error(),
		})
	}
}

// WriteRobotsTxt implements the Writer interface.
// If the RobotsTxtfunc is provided, this will be used, else a default
// implementation will be used.
func (w *WriterFuncs) WriteRobotsTxt(rw http.ResponseWriter, req *http.Request) {
	if w.RobotsTxtfunc != nil {
		w.RobotsTxtfunc(rw, req)
	} else if _, err := rw.Write([]byte("Allow: *")); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

// loadCustomLogo loads the logo file from the path and encodes it to an HTML
// entity or if a URL is provided then it's used directly,
// otherwise if no custom logo is provided, the OAuth2 Proxy Icon is used instead.
func loadCustomLogo(logoPath string) (string, error) {
	if logoPath == "" {
		// The default logo is an SVG so this will be valid to just return.
		return defaultLogoData, nil
	}

	if logoPath == "-" {
		// Return no logo when the custom logo is set to `-`.
		// This disables the logo rendering.
		return "", nil
	}

	if strings.HasPrefix(logoPath, "https://") {
		// Return img tag pointing to the URL.
		return fmt.Sprintf("<img src=\"%s\" alt=\"Logo\" />", logoPath), nil
	}

	logoData, err := os.ReadFile(logoPath)
	if err != nil {
		return "", fmt.Errorf("could not read logo file: %v", err)
	}

	extension := strings.ToLower(filepath.Ext(logoPath))
	switch extension {
	case ".svg":
		return string(logoData), nil
	case ".jpg", ".jpeg":
		return encodeImg(logoData, "jpeg"), nil
	case ".png":
		return encodeImg(logoData, "png"), nil
	default:
		return "", fmt.Errorf("unknown extension: %q, supported extensions are .svg, .jpg, .jpeg and .png", extension)
	}
}

// encodeImg takes the raw image data and converts it to an HTML Img tag with
// a base64 data source.
func encodeImg(data []byte, format string) string {
	b64Data := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("<img src=\"data:image/%s;base64,%s\" alt=\"Logo\" />", format, b64Data)
}
