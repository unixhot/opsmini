// OpsMini Server: control-plane process that accepts OpsMini Agent connections
// over gRPC and exposes an HTTP management API to dispatch execute tasks.
//
// This is a lightweight salt-master replacement for the OpsMini execution engine.
// It runs independently (default gRPC :8010, HTTP :8011) and is meant to be
// deployed on the OpsAnt host.
package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/opsmini/opsmini/internal/agent"
	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

func main() {
	listen := flag.String("listen", ":8010", "gRPC listen address (Agent dials here)")
	token := flag.String("token", "", "registration token; empty disables auth")
	httpAddr := flag.String("http", ":8011", "HTTP management address (dispatch execute tasks)")
	flag.Parse()

	srv := agent.NewServer(*token)

	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("gRPC listen %s: %v", *listen, err)
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("gRPC serve: %v", err)
		}
	}()
	log.Printf("opsmini-server gRPC listening on %s", *listen)

	mux := http.NewServeMux()

	// GET /agents — list connected agents with connection status.
	mux.HandleFunc("/agents", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"agents": srv.AgentStatuses()})
	})

	// POST /execute — dispatch a command or script to an agent and return the result.
	// Body: {"hostname":"...", "command":"...", "script":"...", "language":"shell|python", "timeout":30}
	mux.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Hostname string `json:"hostname"`
			Command  string `json:"command"`
			Script   string `json:"script"`
			Language string `json:"language"`
			Timeout  int32  `json:"timeout"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Hostname == "" {
			http.Error(w, "hostname is required", http.StatusBadRequest)
			return
		}
		task := &agentpb.ExecuteTask{
			JobId:   fmt.Sprintf("job-%d", time.Now().UnixNano()),
			Timeout: req.Timeout,
		}
		switch {
		case req.Command != "":
			task.Kind = &agentpb.ExecuteTask_Command{Command: &agentpb.CommandSpec{Command: req.Command}}
		case req.Script != "":
			task.Kind = &agentpb.ExecuteTask_Script{Script: &agentpb.ScriptSpec{Script: req.Script, Language: req.Language}}
		default:
			http.Error(w, "command or script is required", http.StatusBadRequest)
			return
		}
		result, err := srv.Execute(r.Context(), req.Hostname, task)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, result)
	})

	// POST /pull — pull a file from an agent.
	// Body: {"hostname":"...", "path":"/abs/path"}
	mux.HandleFunc("/pull", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Hostname string `json:"hostname"`
			Path     string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Hostname == "" || req.Path == "" {
			http.Error(w, "hostname and path are required", http.StatusBadRequest)
			return
		}
		data, err := srv.PullFile(r.Context(), req.Hostname, req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sum := md5.Sum(data)
		writeJSON(w, map[string]any{
			"path":           req.Path,
			"size":           len(data),
			"md5":            hex.EncodeToString(sum[:]),
			"content_base64": base64.StdEncoding.EncodeToString(data),
		})
	})

	// POST /push — push a local file (on the Server host) to an agent's path.
	// Body: {"hostname":"...", "src":"/local/source", "dest":"/remote/path", "mode":"0755"}
	mux.HandleFunc("/push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Hostname string `json:"hostname"`
			Src      string `json:"src"`
			Dest     string `json:"dest"`
			Mode     string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Hostname == "" || req.Src == "" || req.Dest == "" {
			http.Error(w, "hostname, src and dest are required", http.StatusBadRequest)
			return
		}
		if err := srv.PushFile(r.Context(), req.Hostname, req.Src, req.Dest, req.Mode); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data, err := os.ReadFile(req.Src)
		if err != nil {
			http.Error(w, "read back source: "+err.Error(), http.StatusInternalServerError)
			return
		}
		sum := md5.Sum(data)
		writeJSON(w, map[string]any{
			"src":  req.Src,
			"dest": req.Dest,
			"size": len(data),
			"md5":  hex.EncodeToString(sum[:]),
		})
	})

	log.Printf("opsmini-server HTTP management listening on %s", *httpAddr)
	log.Fatalf("http serve: %v", http.ListenAndServe(*httpAddr, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
