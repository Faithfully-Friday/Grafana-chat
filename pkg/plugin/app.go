package plugin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// Make sure App implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. Plugin should not implement all these interfaces - only those which are
// required for a particular task.
var (
	_ backend.CallResourceHandler   = (*App)(nil)
	_ instancemgmt.InstanceDisposer = (*App)(nil)
	_ backend.CheckHealthHandler    = (*App)(nil)
)

// settings holds the plugin configuration set on the config page.
type settings struct {
	// BaseURL is the A2A JSON-RPC endpoint of the agent.
	BaseURL string `json:"a2aBaseUrl"`
	// StreamingEnabled toggles message/stream vs blocking message/send.
	StreamingEnabled *bool `json:"streamingEnabled"`
	// APIKey comes from secureJsonData.
	APIKey string `json:"-"`
}

func (s settings) streamingOn() bool {
	return s.StreamingEnabled == nil || *s.StreamingEnabled
}

// App is the backend of the chat app plugin.
type App struct {
	backend.CallResourceHandler
	settings settings
	store    *Store
	client   *A2AClient
}

// NewApp creates a new *App instance.
func NewApp(_ context.Context, instanceSettings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	var cfg settings
	if len(instanceSettings.JSONData) > 0 {
		if err := json.Unmarshal(instanceSettings.JSONData, &cfg); err != nil {
			return nil, err
		}
	}
	cfg.APIKey = instanceSettings.DecryptedSecureJSONData["apiKey"]

	store, err := OpenStore()
	if err != nil {
		return nil, err
	}

	app := &App{
		settings: cfg,
		store:    store,
		client:   NewA2AClient(cfg.BaseURL, cfg.APIKey),
	}

	// Use a httpadapter (provided by the SDK) for resource calls. This allows us
	// to use a *http.ServeMux for resource calls, so we can map multiple routes
	// to CallResource without having to implement extra logic.
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	app.CallResourceHandler = httpadapter.New(mux)

	return app, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created.
func (a *App) Dispose() {
	if a.store != nil {
		_ = a.store.Close()
	}
}

// CheckHealth handles health checks sent from Grafana to the plugin.
func (a *App) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if a.settings.BaseURL == "" {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "A2A endpoint not configured",
		}, nil
	}
	if _, err := a.client.FetchAgentCard(ctx); err != nil {
		log.DefaultLogger.Warn("agent card fetch failed", "error", err)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Cannot reach A2A agent: " + err.Error(),
		}, nil
	}
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Connected to A2A agent",
	}, nil
}
