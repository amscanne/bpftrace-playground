package service

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/bpftrace/playground/pkg/download"
	"github.com/bpftrace/playground/pkg/evaluate"
	"github.com/bpftrace/playground/pkg/workloads"
)

type Server struct {
	router    *mux.Router
	evaluator *evaluate.Evaluator
	template  *template.Template
}

type PageData struct {
	Code     string
	Workload string
	Version  string
	Timeout  string
	Files    string
}

//go:embed templates/*.html
var templateFiles embed.FS

func NewServer(downloader *download.Manager, maxTimeout int) (*Server, error) {
	tmpl, err := template.ParseFS(templateFiles, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	s := &Server{
		router:    mux.NewRouter(),
		evaluator: evaluate.NewEvaluator(downloader, maxTimeout),
		template:  tmpl,
	}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.router.HandleFunc("/execute", s.evaluator.ExecuteHandler)
	s.router.HandleFunc("/workloads", s.workloadsHandler)
	s.router.HandleFunc("/", s.embedHandler)
}

func (s *Server) workloadsHandler(w http.ResponseWriter, r *http.Request) {
	// Just return the workloads as a JSON-encoded list.
	workloads := workloads.List()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(workloads); err != nil {
		http.Error(w, "Failed to encode workloads", http.StatusInternalServerError)
		return
	}
}

func (s *Server) embedHandler(w http.ResponseWriter, r *http.Request) {
	codeB64 := r.URL.Query().Get("code")
	filesB64 := r.URL.Query().Get("files")
	workload := r.URL.Query().Get("workload")
	version := r.URL.Query().Get("version")
	timeout := r.URL.Query().Get("timeout")

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
		const emptyJSON = "{}"
		files = emptyJSON
	}

	if version == "" {
		version = "master"
	}
	if timeout == "" {
		timeout = "3000" // Three seconds.
	}

	data := PageData{
		Code:     code,
		Workload: workload,
		Version:  version,
		Timeout:  timeout,
		Files:    files,
	}

	err := s.template.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func Main(port string, downloader *download.Manager, maxTimeout int) error {
	s, err := NewServer(downloader, maxTimeout)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	log.Printf("Listening on port %s", port)
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      s,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		return fmt.Errorf("failed to listen and serve: %w", err)
	}
	return nil
}
