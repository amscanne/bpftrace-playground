package evaluate

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amscanne/bpftrace-playground/pkg/download"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// Request is the input to an evaluate request.
type Request struct {
	Version string            `json:"version"`
	Code    string            `json:"code"`
	Files   map[string]string `json:"files"`
	Timeout int               `json:"timeout"`
}

// ExitData is the data for an exit message.
type ExitData struct {
	ExitCode int `json:"exit_code"`
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
	mu         sync.Mutex
}

func NewEvaluator(downloader *download.Manager) *Evaluator {
	return &Evaluator{downloader: downloader}
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

	messageType, p, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read message: %v", err)
		return // Internal error.
	}

	if messageType != websocket.TextMessage {
		log.Printf("Received non-text message: %d", messageType)
		return // Internal error.
	}

	var req Request
	if err := json.Unmarshal(p, &req); err != nil {
		log.Printf("Failed to unmarshal request: %v", err)
		return // Internal error.
	}

	tempDir, err := os.MkdirTemp("", "bpftrace-")
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		return // Internal error.
	}
	defer os.RemoveAll(tempDir)

	for name, content := range req.Files {
		filePath := filepath.Join(tempDir, name)
		if !strings.HasPrefix(filePath, tempDir) {
			log.Printf("Invalid file path (traversal attempt): %s", name)
			return // Internal error.
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			log.Printf("Failed to create dir for file: %v", err)
			return // Internal error.
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			log.Printf("Failed to write file: %v", err)
			return // Internal error.
		}
	}

	bpftracePath, err := e.downloader.Get(req.Version)
	if err != nil {
		log.Printf("Failed to download binary: %v", err)
		return // Internal error.
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.Timeout)*time.Millisecond)
	defer cancel()

	cmd := getCommand(ctx, bpftracePath, req.Code)
	cmd.Dir = tempDir

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start pty: %v", err) // Internal error.
		exitData, _ := json.Marshal(ExitData{ExitCode: -1})
		msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
		conn.WriteMessage(websocket.TextMessage, msg)
		return
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ptmx.Close()
		buffer := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buffer)
			if n > 0 {
				msg, _ := json.Marshal(StreamResponse{Type: "output", Data: string(buffer[:n])})
				conn.WriteMessage(websocket.TextMessage, msg)
			}
			if err != nil {
				return // Includes io.EOF.
			}
		}
	}()

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
			if exitError, ok := err.(*exec.ExitError); ok {
				exitData, _ := json.Marshal(ExitData{ExitCode: exitError.ExitCode()})
				msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
				conn.WriteMessage(websocket.TextMessage, msg)
			} else {
				exitData, _ := json.Marshal(ExitData{ExitCode: cmd.ProcessState.ExitCode()})
				msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
				conn.WriteMessage(websocket.TextMessage, msg)
			}
		} else {
			exitData, _ := json.Marshal(ExitData{ExitCode: 0})
			msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		wg.Wait()
		exitData, _ := json.Marshal(ExitData{ExitCode: cmd.ProcessState.ExitCode()})
		msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}
