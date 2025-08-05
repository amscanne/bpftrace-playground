package evaluate

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
		exitData, _ := json.Marshal(ExitData{Msg: err.Error(), ExitCode: -1})
		msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("Failed to write message: %v", err)
			return
		}
	}

	messageType, p, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read message: %v", err)
		return
	}

	if messageType != websocket.TextMessage {
		log.Printf("Received non-text message: %d", messageType)
		return
	}

	var req Request
	if err := json.Unmarshal(p, &req); err != nil {
		log.Printf("Failed to unmarshal request: %v", err)
		return
	}

	tempDir, err := os.MkdirTemp("", "bpftrace-")
	if err != nil {
		fail(fmt.Errorf("Failed to create temp dir: %v", err))
		return
	}
	defer os.RemoveAll(tempDir)

	for name, content := range req.Files {
		filePath := filepath.Join(tempDir, name)
		if !strings.HasPrefix(filePath, tempDir) {
			fail(fmt.Errorf("Invalid file path (traversal attempt): %s", name))
			return
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			fail(fmt.Errorf("Failed to create dir for file: %v", err))
			return
		}
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			fail(fmt.Errorf("Failed to write file: %v", err))
			return
		}
	}

	bpftracePath, err := e.downloader.Get(req.Version)
	if err != nil {
		fail(fmt.Errorf("Failed to download binary: %v", err))
		return
	}

	timeout := req.Timeout
	if timeout < 0 || timeout > e.maxTimeout {
		timeout = e.maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	cmd := getCommand(ctx, bpftracePath, req.Code)
	cmd.Dir = tempDir // Run in the working directory.

	ptmx, err := pty.Start(cmd)
	if err != nil {
		fail(fmt.Errorf("Failed to start pty: %v", err))
		return
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ptmx.Close()
		buffer := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buffer)
			if n > 0 {
				msg, _ := json.Marshal(StreamResponse{Type: "output", Data: string(buffer[:n])})
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Printf("Failed to write message: %v", err)
				}
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
			fail(err)
		} else {
			// Successfully run the program.
			exitData, _ := json.Marshal(ExitData{ExitCode: 0})
			msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Failed to write message: %v", err)
			}
		}
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		wg.Wait()
		// Non-zero exit code, but execution has completed.
		exitData, _ := json.Marshal(ExitData{ExitCode: cmd.ProcessState.ExitCode()})
		msg, _ := json.Marshal(StreamResponse{Type: "exit", Data: string(exitData)})
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("Failed to write message: %v", err)
		}
	}
}
