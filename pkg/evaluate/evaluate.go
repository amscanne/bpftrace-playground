package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/bpftrace/playground/pkg/download"
)

// Request is the input to an evaluate request.
type Request struct {
	Version  string            `json:"version"`
	Code     string            `json:"code"`
	Files    map[string]string `json:"files"`
	Timeout  int               `json:"timeout"`
	Workload string            `json:"workload"`
}

// ExitData is the data for an exit message.
type ExitData struct {
	ExitCode int    `json:"exit_code"`
	Msg      string `json:"msg"`
}

// StreamResponse is the message sent over the websocket.
type StreamResponse struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins.
	},
}

type Evaluator struct {
	downloader *download.Manager
	maxTimeout int
	mu         sync.Mutex
}

func NewEvaluator(downloader *download.Manager, maxTimeout int) *Evaluator {
	return &Evaluator{downloader: downloader, maxTimeout: maxTimeout}
}

func (e *Evaluator) readRequest(conn *websocket.Conn) (*Request, error) {
	messageType, p, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	if messageType != websocket.TextMessage {
		return nil, fmt.Errorf("received non-text message: %d", messageType)
	}

	var req Request
	if err := json.Unmarshal(p, &req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &req, nil
}

func (e *Evaluator) writeFiles(tempDir string, files map[string]string) error {
	for name, content := range files {
		filePath := filepath.Join(tempDir, name)
		if !strings.HasPrefix(filePath, tempDir) {
			return fmt.Errorf("invalid file path (traversal attempt): %s", name)
		}
		if dirErr := os.MkdirAll(filepath.Dir(filePath), 0755); dirErr != nil {
			return fmt.Errorf("failed to create dir for file: %w", dirErr)
		}
		if writeErr := os.WriteFile(filePath, []byte(content), 0600); writeErr != nil {
			return fmt.Errorf("failed to write file: %w", writeErr)
		}
	}
	return nil
}

func (e *Evaluator) getArtifactName() (string, error) {
	if runtime.GOARCH == "amd64" {
		return "bpftrace-X64", nil
	} else if runtime.GOARCH == "arm64" {
		return "bpftrace-ARM64", nil
	}
	return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
}

func (e *Evaluator) streamOutput(wg *sync.WaitGroup, conn *websocket.Conn, ptmx *os.File) {
	defer wg.Done()
	defer ptmx.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buffer)
		if n > 0 {
			msg, _ := json.Marshal(StreamResponse{Type: "output", Data: string(buffer[:n])})
			if writeErr := conn.WriteMessage(websocket.TextMessage, msg); writeErr != nil {
				log.Printf("Failed to write message: %v", writeErr)
			}
		}
		if err != nil {
			return // Includes io.EOF.
		}
	}
}

func (e *Evaluator) sendExitMessage(conn *websocket.Conn, exitCode int, errMsg string) {
	var exitData []byte
	if errMsg != "" {
		exitData, _ = json.Marshal(ExitData{Msg: errMsg, ExitCode: exitCode})
	} else {
		exitData, _ = json.Marshal(ExitData{ExitCode: exitCode})
	}
	msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("Failed to write message: %v", err)
	}
}

func (e *Evaluator) ExecuteHandler(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	var wg sync.WaitGroup
	fail := func(err error) {
		wg.Wait()
		log.Printf("Failed execution: %v", err)
		e.sendExitMessage(conn, -1, err.Error())
	}

	req, err := e.readRequest(conn)
	if err != nil {
		log.Printf("%v", err)
		return
	}

	tempDir, err := os.MkdirTemp("", "bpftrace-")
	if err != nil {
		fail(fmt.Errorf("failed to create temp dir: %w", err))
		return
	}
	defer os.RemoveAll(tempDir)

	if writeErr := e.writeFiles(tempDir, req.Files); writeErr != nil {
		fail(writeErr)
		return
	}

	artifactName, err := e.getArtifactName()
	if err != nil {
		fail(err)
		return
	}

	bpftracePath, err := e.downloader.GetArtifact(context.Background(), artifactName, req.Version)
	if err != nil {
		fail(fmt.Errorf("failed to download binary: %w", err))
		return
	}

	timeout := req.Timeout
	if timeout < 0 || timeout > e.maxTimeout {
		timeout = e.maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	cmd := getCommand(ctx, req.Workload, bpftracePath, req.Code)
	cmd.Dir = tempDir // Run in the working directory.

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fail(fmt.Errorf("failed to start pty: %w", err))
		return
	}

	wg.Add(1)
	go e.streamOutput(&wg, conn, ptmx)

	wg.Add(1)
	cmdDone := make(chan error, 1)
	go func() {
		defer wg.Done()
		cmdDone <- cmd.Wait()
	}()

	select {
	case err := <-cmdDone:
		wg.Wait()
		if err != nil {
			fail(err)
		} else {
			e.sendExitMessage(conn, 0, "")
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		wg.Wait()
		e.sendExitMessage(conn, cmd.ProcessState.ExitCode(), "")
	}
}
