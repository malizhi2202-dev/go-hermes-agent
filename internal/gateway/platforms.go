package gateway

import (
	"net/http"

	"go-hermes-agent/internal/app"
)

// Route binds one inbound gateway path to one handler.
type Route struct {
	Path    string
	Handler http.HandlerFunc
}

// PlatformAdapter is the lightweight plugin contract for one messaging platform.
//
// The interface intentionally stays small so personal adapters can be added
// without learning the whole runtime. Each adapter owns only ingress
// validation, payload decoding, and response marshaling; business execution
// still flows through internal/app.
type PlatformAdapter interface {
	Name() string
	Routes() []Route
}

// BuiltInAdapters returns the built-in gateway adapters in one place so the
// server can register them uniformly.
func BuiltInAdapters(application *app.App) []PlatformAdapter {
	return []PlatformAdapter{
		NewWebhookAdapter(application),
		NewTelegramAdapter(application),
		NewSlackAdapter(application),
		NewWeixinAdapter(application),
	}
}

// RegisterPlatformRoutes attaches all adapter routes to one ServeMux.
func RegisterPlatformRoutes(mux *http.ServeMux, adapters ...PlatformAdapter) {
	for _, adapter := range adapters {
		for _, route := range adapter.Routes() {
			mux.HandleFunc(route.Path, route.Handler)
		}
	}
}
