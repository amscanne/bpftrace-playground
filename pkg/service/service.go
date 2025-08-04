package service

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/amscanne/bpftrace-playground/pkg/download"
	"github.com/amscanne/bpftrace-playground/pkg/evaluate"
	"github.com/gorilla/mux"
)

type Server struct {
	router     *mux.Router
	evaluator  *evaluate.Evaluator
	template   *template.Template
	maxTimeout int
}

type PageData struct {
	Code     string
	Workload string
	Version  string
	Timeout  int
	Files    string
}

//go:embed templates/*.html
var templateFiles embed.FS

func NewServer(cacheDir string, maxCache int, maxTimeout int) (*Server, error) {
	downloader, err := download.NewManager(cacheDir, maxCache)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.ParseFS(templateFiles, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	s := &Server{
		router:     mux.NewRouter(),
		evaluator:  evaluate.NewEvaluator(downloader),
		template:   tmpl,
		maxTimeout: maxTimeout,
	}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.router.HandleFunc("/execute", s.evaluator.ExecuteHandler)
	s.router.HandleFunc("/", s.embedHandler)
}

func (s *Server) embedHandler(w http.ResponseWriter, r *http.Request) {
	codeB64 := r.URL.Query().Get("code")
	filesB64 := r.URL.Query().Get("files")
	workload := r.URL.Query().Get("workload")
	version := r.URL.Query().Get("version")
	timeoutStr := r.URL.Query().Get("timeout")

	var code string
	if codeB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(codeB64)
		if err != nil {
			http.Error(w, "Failed to decode code parameter", http.StatusBadRequest)
			return
		}
		code = string(decoded)
	} else {
		code = ""
	}

	var files string
	if filesB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(filesB64)
		if err != nil {
			http.Error(w, "Failed to decode files parameter", http.StatusBadRequest)
			return
		}
		files = string(decoded)
	} else {
		files = "{}"
	}

	if version == "" {
		version = "latest"
	}

	timeout := s.maxTimeout
	if timeoutStr != "" {
		parsedTimeout, err := strconv.Atoi(timeoutStr)
		if err != nil {
			http.Error(w, "Bad timeout: "+err.Error(), http.StatusBadRequest)
			return
		} else if parsedTimeout >= 0 && parsedTimeout < s.maxTimeout {
			timeout = parsedTimeout
		}
	}

	data := PageData{
		Code:     code,
		Workload: workload,
		Version:  version,
		Timeout:  timeout,
		Files:    files,
	}

	err := s.template.ExecuteTemplate(w, "templates/index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func Main(port string, cacheDir string, maxCache int, maxTimeout int) error {
	s, err := NewServer(cacheDir, maxCache, maxTimeout)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, s); err != nil {
		return fmt.Errorf("failed to listen and serve: %w", err)
	}
	return nil
}
