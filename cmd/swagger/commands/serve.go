package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
	"github.com/gorilla/handlers"
	"github.com/toqueteos/webbrowser"
)

// 国内云端存储的swagger-ui的样式文件网络下载地址前缀
const DefaultSwaggerCloudStaticFileUrLPrefix string = "https://unpkg.com/swagger-ui-dist"
const DefaultRedocCloudStaticFileUrLPrefix string = "https://cdn.jsdelivr.net/npm/redoc/bundles"

// ServeCmd to serve a swagger spec with docs ui
type ServeCmd struct {
	BasePath  string `long:"base-path" description:"the base path to serve the spec and UI at"`
	Flavor    string `short:"F" long:"flavor" description:"the flavor of docs, can be swagger or redoc" default:"redoc" choice:"redoc" choice:"swagger"`
	DocURL    string `long:"doc-url" description:"override the url which takes a url query param to render the doc ui"`
	NoOpen    bool   `long:"no-open" description:"when present won't open the the browser to show the url"`
	NoUI      bool   `long:"no-ui" description:"when present, only the swagger spec will be served"`
	Flatten   bool   `long:"flatten" description:"when present, flatten the swagger spec before serving it"`
	Port      int    `long:"port" short:"p" description:"the port to serve this site" env:"PORT"`
	Host      string `long:"host" description:"the interface to serve this site, defaults to 0.0.0.0" default:"0.0.0.0" env:"HOST"`
	Path      string `long:"path" description:"the uri path at which the docs will be served" default:"docs"`
	SourceUrL string `long:"source_url" description:"specifies the path to the swaager-ui render style file download url prefix" short:"S"`
}

// Execute the serve command
func (s *ServeCmd) Execute(args []string) error {
	if len(args) == 0 {
		return errors.New("specify the spec to serve as argument to the serve command")
	}

	specDoc, err := loads.Spec(args[0])
	if err != nil {
		return err
	}

	if s.Flatten {
		specDoc, err = specDoc.Expanded(&spec.ExpandOptions{
			SkipSchemas:         false,
			ContinueOnError:     true,
			AbsoluteCircularRef: true,
		})

		if err != nil {
			return err
		}
	}

	b, err := json.MarshalIndent(specDoc.Spec(), "", "  ")
	if err != nil {
		return err
	}

	basePath := s.BasePath
	if basePath == "" {
		basePath = "/"
	}

	listener, err := net.Listen("tcp4", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)))
	if err != nil {
		return err
	}
	sh, sp, err := swag.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	if sh == "0.0.0.0" {
		sh = "localhost"
	}

	visit := s.DocURL
	handler := http.NotFoundHandler()

	// redoc格式使用
	var reDocUrlPrefix string
	// swagger-ui样式使用
	var swaggerUiUrlPrefix string
	if s.SourceUrL != "" {
		reDocUrlPrefix = s.SourceUrL
		swaggerUiUrlPrefix = s.SourceUrL
	} else {
		reDocUrlPrefix = DefaultRedocCloudStaticFileUrLPrefix
		swaggerUiUrlPrefix = DefaultSwaggerCloudStaticFileUrLPrefix
	}

	if !s.NoUI {
		if s.Flavor == "redoc" {
			handler = middleware.Redoc(middleware.RedocOpts{
				BasePath: basePath,
				SpecURL:  path.Join(basePath, "swagger.json"),
				Path:     s.Path,
				RedocURL: strings.Join([]string{reDocUrlPrefix, "redoc.standalone.js"}, "/"),
			}, handler)
			visit = fmt.Sprintf("http://%s:%d%s", sh, sp, path.Join(basePath, "docs"))
		} else if visit != "" || s.Flavor == "swagger" {
			handler = middleware.SwaggerUI(middleware.SwaggerUIOpts{
				BasePath: basePath,
				SpecURL:  path.Join(basePath, "swagger.json"),
				Path:     s.Path,
				// Update swagger-ui default config
				SwaggerURL:       strings.Join([]string{swaggerUiUrlPrefix, "swagger-ui-bundle.js"}, "/"),
				SwaggerPresetURL: strings.Join([]string{swaggerUiUrlPrefix, "swagger-ui-standalone-preset.js"}, "/"),
				SwaggerStylesURL: strings.Join([]string{swaggerUiUrlPrefix, "swagger-ui.css"}, "/"),
				Favicon16:        strings.Join([]string{swaggerUiUrlPrefix, "favicon-16x16.png"}, "/"),
				Favicon32:        strings.Join([]string{swaggerUiUrlPrefix, "favicon-32x32.png"}, "/"),
			}, handler)
			visit = fmt.Sprintf("http://%s:%d%s", sh, sp, path.Join(basePath, s.Path))
		}
	}

	handler = handlers.CORS()(middleware.Spec(basePath, b, handler))
	errFuture := make(chan error)
	go func() {
		docServer := new(http.Server)
		docServer.SetKeepAlivesEnabled(true)
		docServer.Handler = handler

		errFuture <- docServer.Serve(listener)
	}()

	if !s.NoOpen && !s.NoUI {
		err := webbrowser.Open(visit)
		if err != nil {
			return err
		}
	}
	log.Println("serving docs at", visit)
	return <-errFuture
}
