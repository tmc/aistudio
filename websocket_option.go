package aistudio

// WithWebSocket enables or disables WebSocket mode for live models.
// When enabled, live models will use WebSocket protocol instead of gRPC.
func WithWebSocket(enabled bool) Option {
	return func(m *Model) error {
		m.enableWebSocket = enabled
		return nil
	}
}
