package aistudio

import "github.com/tmc/aistudio/api"

// WithAPIKey sets the Google API Key for the client.
func WithAPIKey(key string) Option {
	return func(m *Model) error {
		m.apiKey = key
		if m.client == nil {
			m.client = &api.Client{}
		}
		m.client.APIKey = key
		return nil
	}
}

// WithModel sets the Gemini model name to use.
func WithModel(name string) Option {
	return func(m *Model) error {
		m.modelName = name
		return nil
	}
}

// WithAudioOutput enables/disables audio output and optionally sets the voice.
// Note: Voice configuration in BidiGenerateContent v1alpha setup is less defined than v1beta.
// This option primarily signals the intent to play audio if received.
func WithAudioOutput(enabled bool, voice ...string) Option {
	return func(m *Model) error {
		m.enableAudio = enabled
		if len(voice) > 0 && voice[0] != "" {
			m.voiceName = voice[0]
		} else if enabled {
			m.voiceName = DefaultVoice
		} else {
			m.voiceName = ""
		}
		// We don't set API config here, but rather in InitBidiStream based on m.enableAudio
		return nil
	}
}

// WithAudioPlayerCommand sets the external command used for audio playback.
func WithAudioPlayerCommand(cmd string) Option {
	return func(m *Model) error {
		m.playerCmd = cmd
		return nil
	}
}

// WithAudioPlaybackMode sets the audio playback mode.
func WithAudioPlaybackMode(mode AudioPlaybackMode) Option {
	return func(m *Model) error {
		m.audioPlaybackMode = mode
		return nil
	}
}

// WithLogo enables or disables the logo display.
func WithLogo(showLogo bool) Option {
	return func(m *Model) error {
		m.showLogo = showLogo
		return nil
	}
}

// WithLogMessages enables or disables the log messages display.
func WithLogMessages(show bool, maxEntries ...int) Option {
	return func(m *Model) error {
		m.showLogMessages = show

		// Set default maximum number of log messages if not specified
		if len(maxEntries) > 0 && maxEntries[0] > 0 {
			m.maxLogMessages = maxEntries[0]
		} else if m.maxLogMessages == 0 {
			m.maxLogMessages = 10 // Default to 10 entries
		}

		return nil
	}
}

// WithAudioStatus enables or disables the audio playback status display.
func WithAudioStatus(show bool) Option {
	return func(m *Model) error {
		m.showAudioStatus = show
		return nil
	}
}

// WithBidiStream enables or disables using the true bidirectional stream.
func WithBidiStream(enable bool) Option {
	return func(m *Model) error {
		m.useBidi = enable
		return nil
	}
}
