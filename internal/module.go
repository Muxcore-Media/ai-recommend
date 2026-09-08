package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Muxcore-Media/core/pkg/contracts"
	meshv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/mesh/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"google.golang.org/grpc"
)

const moduleVersion = "0.1.0"

type Module struct {
	id, grpcAddr, httpAddr string
	cfgMu                  sync.RWMutex
	last                   []Suggestion
	grpcSrv                *grpc.Server
	lis                    net.Listener
	httpSrv                *http.Server
}

func NewModule() *Module { return New(Config{}) }

type Config struct{ ID, GRPCAddr, HTTPAddr string }

func New(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "ai-recommend"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9764"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:9765"
	}
	if v := os.Getenv("AI_RECOMMEND_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("MUXCORE_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	return &Module{id: cfg.ID, grpcAddr: cfg.GRPCAddr, httpAddr: cfg.HTTPAddr}
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID: m.id, Name: "AI Recommend", Version: moduleVersion,
		Roles: []string{"ai"}, Description: "AI library-based suggestions of media to add",
		Author: "Muxcore-Media", Capabilities: []string{"ai.recommend", "settings"},
		MinCoreVersion: MinCoreVersion, HTTPAddr: m.grpcAddr,
	}
}

func (m *Module) Init(context.Context) error { return nil }

func (m *Module) Start(ctx context.Context) error {
	return startHTTPGRPC(ctx, m.id, &m.grpcAddr, &m.httpAddr, &m.lis, &m.grpcSrv, &m.httpSrv, m, m.routes)
}

func (m *Module) Stop(ctx context.Context) error { return stopServers(ctx, m.grpcSrv, m.httpSrv) }
func (m *Module) Health(context.Context) error   { return nil }
func (m *Module) GRPCListenAddr() string         { return m.grpcAddr }
func (m *Module) HTTPListenAddr() string         { return m.httpAddr }
func (m *Module) Settings() []contracts.SettingDef {
	return []contracts.SettingDef{{Key: "catalog", Label: "Catalog", Type: contracts.SettingTypeString, Value: "builtin", Group: "AI"}}
}
func (m *Module) UpdateSetting(key, value string) error {
	if key != "catalog" {
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

func (m *Module) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/suggest", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Library []LibraryItem `json:"library"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		got := suggestAdditions(body.Library, nil)
		m.cfgMu.Lock()
		m.last = got
		m.cfgMu.Unlock()
		writeJSON(w, map[string]any{"suggestions": got, "clusters": tasteClusters(body.Library)})
	})
	mux.HandleFunc("GET /v1/last", func(w http.ResponseWriter, _ *http.Request) {
		m.cfgMu.RLock()
		defer m.cfgMu.RUnlock()
		writeJSON(w, map[string]any{"suggestions": m.last})
	})
}

func (m *Module) handleMesh(_ context.Context, method string, payload []byte) ([]byte, error) {
	if method != "Suggest" {
		return nil, fmt.Errorf("unknown method %s", method)
	}
	var body struct {
		Library []LibraryItem `json:"library"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	got := suggestAdditions(body.Library, nil)
	m.cfgMu.Lock()
	m.last = got
	m.cfgMu.Unlock()
	return json.Marshal(map[string]any{"suggestions": got, "clusters": tasteClusters(body.Library)})
}

type settingsMesh interface {
	Settings() []contracts.SettingDef
	UpdateSetting(key, value string) error
	handleMesh(ctx context.Context, method string, payload []byte) ([]byte, error)
}

func startHTTPGRPC(
	ctx context.Context, id string, grpcAddr, httpAddr *string, lis *net.Listener,
	grpcSrv **grpc.Server, httpSrv **http.Server, h settingsMesh, routes func(*http.ServeMux),
) error {
	var lc net.ListenConfig
	gLis, err := lc.Listen(ctx, "tcp", *grpcAddr)
	if err != nil {
		return err
	}
	*lis = gLis
	*grpcAddr = gLis.Addr().String()
	*grpcSrv = grpc.NewServer()
	meshv1.RegisterModuleMeshServer(*grpcSrv, &genericMesh{id: id, settings: modulesdk.SettingsHandlerFromProvider(h), h: h})
	go func() {
		slog.Info("gRPC listening", "id", id, "addr", *grpcAddr)
		if e := (*grpcSrv).Serve(gLis); e != nil {
			slog.Error("gRPC", "error", e)
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", okHealth)
	mux.HandleFunc("GET /healthz", okHealth)
	routes(mux)
	hLis, err := lc.Listen(ctx, "tcp", *httpAddr)
	if err != nil {
		return err
	}
	*httpAddr = hLis.Addr().String()
	*httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("HTTP listening", "id", id, "addr", *httpAddr)
		if e := (*httpSrv).Serve(hLis); e != nil && e != http.ErrServerClosed {
			slog.Error("HTTP", "error", e)
		}
	}()
	return nil
}

type genericMesh struct {
	meshv1.UnimplementedModuleMeshServer
	id       string
	settings modulesdk.SettingsHandler
	h        settingsMesh
}

func (s *genericMesh) Call(ctx context.Context, req *meshv1.CallRequest) (*meshv1.CallResponse, error) {
	switch req.GetMethod() {
	case "Settings":
		raw, err := json.Marshal(s.settings.List())
		if err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: raw}, nil
	case "UpdateSetting":
		var body struct{ Key, Value string }
		if err := json.Unmarshal(req.GetPayload(), &body); err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		if err := s.settings.Update(body.Key, body.Value); err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: []byte(`{"ok":true}`)}, nil
	default:
		raw, err := s.h.handleMesh(ctx, req.GetMethod(), req.GetPayload())
		if err != nil {
			return &meshv1.CallResponse{Error: err.Error()}, nil
		}
		return &meshv1.CallResponse{Payload: raw}, nil
	}
}

func (s *genericMesh) StreamCall(meshv1.ModuleMesh_StreamCallServer) error {
	return fmt.Errorf("StreamCall not supported")
}

func stopServers(ctx context.Context, g *grpc.Server, h *http.Server) error {
	if g != nil {
		g.GracefulStop()
	}
	if h != nil {
		return h.Shutdown(ctx)
	}
	return nil
}

func okHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func readJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return false
	}
	if err := json.Unmarshal(body, dest); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
