package audio

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// VoiceManager manages the available HD voices for different languages
type VoiceManager struct {
	voices      map[string]*Voice
	voicesByLang map[string][]*Voice
	mu          sync.RWMutex
}

// Voice represents an HD voice configuration
type Voice struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	DisplayName     string   `json:"display_name"`
	Language        string   `json:"language"`
	LanguageCode    string   `json:"language_code"`
	Gender          string   `json:"gender"`
	Age             string   `json:"age"`
	Style           string   `json:"style"`
	Description     string   `json:"description"`
	SampleRate      int      `json:"sample_rate"`
	Capabilities    []string `json:"capabilities"`
	EmotionSupport  bool     `json:"emotion_support"`
	NeuralQuality   bool     `json:"neural_quality"`
}

// SupportedLanguages lists all 24+ supported languages
var SupportedLanguages = []string{
	"en-US", "en-GB", "en-AU", "en-IN", // English variants
	"es-ES", "es-MX", "es-US",          // Spanish variants
	"fr-FR", "fr-CA",                    // French variants
	"de-DE", "de-AT", "de-CH",          // German variants
	"it-IT",                             // Italian
	"pt-BR", "pt-PT",                    // Portuguese variants
	"nl-NL",                             // Dutch
	"pl-PL",                             // Polish
	"ru-RU",                             // Russian
	"ja-JP",                             // Japanese
	"ko-KR",                             // Korean
	"zh-CN", "zh-TW",                    // Chinese variants
	"ar-SA",                             // Arabic
	"hi-IN",                             // Hindi
	"tr-TR",                             // Turkish
}

// DefaultVoices provides the initial set of HD voices
func DefaultVoices() []*Voice {
	return []*Voice{
		// English voices
		{ID: "en-US-1", Name: "Olivia", DisplayName: "Olivia (US)", Language: "English", LanguageCode: "en-US", Gender: "female", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		{ID: "en-US-2", Name: "James", DisplayName: "James (US)", Language: "English", LanguageCode: "en-US", Gender: "male", Age: "adult", Style: "conversational", EmotionSupport: true, NeuralQuality: true},
		{ID: "en-GB-1", Name: "Emma", DisplayName: "Emma (UK)", Language: "English", LanguageCode: "en-GB", Gender: "female", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		{ID: "en-GB-2", Name: "Oliver", DisplayName: "Oliver (UK)", Language: "English", LanguageCode: "en-GB", Gender: "male", Age: "adult", Style: "friendly", EmotionSupport: true, NeuralQuality: true},
		{ID: "en-AU-1", Name: "Sophie", DisplayName: "Sophie (AU)", Language: "English", LanguageCode: "en-AU", Gender: "female", Age: "adult", Style: "casual", EmotionSupport: true, NeuralQuality: true},
		
		// Spanish voices
		{ID: "es-ES-1", Name: "Sofia", DisplayName: "Sofia (Spain)", Language: "Spanish", LanguageCode: "es-ES", Gender: "female", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		{ID: "es-MX-1", Name: "Diego", DisplayName: "Diego (Mexico)", Language: "Spanish", LanguageCode: "es-MX", Gender: "male", Age: "adult", Style: "friendly", EmotionSupport: true, NeuralQuality: true},
		
		// French voices
		{ID: "fr-FR-1", Name: "Marie", DisplayName: "Marie (France)", Language: "French", LanguageCode: "fr-FR", Gender: "female", Age: "adult", Style: "elegant", EmotionSupport: true, NeuralQuality: true},
		{ID: "fr-CA-1", Name: "Jean", DisplayName: "Jean (Canada)", Language: "French", LanguageCode: "fr-CA", Gender: "male", Age: "adult", Style: "casual", EmotionSupport: true, NeuralQuality: true},
		
		// German voices
		{ID: "de-DE-1", Name: "Anna", DisplayName: "Anna (Germany)", Language: "German", LanguageCode: "de-DE", Gender: "female", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		{ID: "de-DE-2", Name: "Max", DisplayName: "Max (Germany)", Language: "German", LanguageCode: "de-DE", Gender: "male", Age: "adult", Style: "friendly", EmotionSupport: true, NeuralQuality: true},
		
		// Japanese voices
		{ID: "ja-JP-1", Name: "Yuki", DisplayName: "Yuki (Japan)", Language: "Japanese", LanguageCode: "ja-JP", Gender: "female", Age: "adult", Style: "polite", EmotionSupport: true, NeuralQuality: true},
		{ID: "ja-JP-2", Name: "Takeshi", DisplayName: "Takeshi (Japan)", Language: "Japanese", LanguageCode: "ja-JP", Gender: "male", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		
		// Chinese voices
		{ID: "zh-CN-1", Name: "Xiaomei", DisplayName: "Xiaomei (China)", Language: "Chinese", LanguageCode: "zh-CN", Gender: "female", Age: "adult", Style: "friendly", EmotionSupport: true, NeuralQuality: true},
		{ID: "zh-CN-2", Name: "Wei", DisplayName: "Wei (China)", Language: "Chinese", LanguageCode: "zh-CN", Gender: "male", Age: "adult", Style: "professional", EmotionSupport: true, NeuralQuality: true},
		
		// TODO: Add more voices to reach 30+ total HD voices
		// TODO: Implement additional language variants (Hindi, Arabic, Portuguese, etc.)
		// TODO: Add child and elderly voice options
		// TODO: Implement voice cloning capabilities
		// TODO: Add celebrity and character voice options
		// TODO: Implement custom voice training support
	}
}

// NewVoiceManager creates a new voice manager
func NewVoiceManager() *VoiceManager {
	vm := &VoiceManager{
		voices:       make(map[string]*Voice),
		voicesByLang: make(map[string][]*Voice),
	}
	
	// Load default voices
	vm.LoadDefaultVoices()
	
	return vm
}

// LoadDefaultVoices loads the default set of HD voices
func (vm *VoiceManager) LoadDefaultVoices() {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	voices := DefaultVoices()
	for _, voice := range voices {
		vm.voices[voice.ID] = voice
		vm.voicesByLang[voice.LanguageCode] = append(vm.voicesByLang[voice.LanguageCode], voice)
	}
	
	log.Printf("[VOICE_MANAGER] Loaded %d voices across %d languages", len(voices), len(vm.voicesByLang))
}

// GetVoice retrieves a voice by ID
func (vm *VoiceManager) GetVoice(voiceID string) (*Voice, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	voice, exists := vm.voices[voiceID]
	if !exists {
		return nil, fmt.Errorf("voice not found: %s", voiceID)
	}
	
	return voice, nil
}

// GetVoicesByLanguage returns all voices for a specific language
func (vm *VoiceManager) GetVoicesByLanguage(languageCode string) ([]*Voice, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	voices, exists := vm.voicesByLang[languageCode]
	if !exists || len(voices) == 0 {
		return nil, fmt.Errorf("no voices found for language: %s", languageCode)
	}
	
	return voices, nil
}

// GetVoicesByGender returns voices filtered by gender
func (vm *VoiceManager) GetVoicesByGender(gender string) []*Voice {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var filtered []*Voice
	for _, voice := range vm.voices {
		if strings.EqualFold(voice.Gender, gender) {
			filtered = append(filtered, voice)
		}
	}
	
	return filtered
}

// GetVoicesWithEmotion returns voices that support emotion
func (vm *VoiceManager) GetVoicesWithEmotion() []*Voice {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var filtered []*Voice
	for _, voice := range vm.voices {
		if voice.EmotionSupport {
			filtered = append(filtered, voice)
		}
	}
	
	return filtered
}

// GetAllVoices returns all available voices
func (vm *VoiceManager) GetAllVoices() []*Voice {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var allVoices []*Voice
	for _, voice := range vm.voices {
		allVoices = append(allVoices, voice)
	}
	
	// Sort by language and name for consistent ordering
	sort.Slice(allVoices, func(i, j int) bool {
		if allVoices[i].LanguageCode != allVoices[j].LanguageCode {
			return allVoices[i].LanguageCode < allVoices[j].LanguageCode
		}
		return allVoices[i].Name < allVoices[j].Name
	})
	
	return allVoices
}

// GetSupportedLanguages returns all supported language codes
func (vm *VoiceManager) GetSupportedLanguages() []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var languages []string
	for lang := range vm.voicesByLang {
		languages = append(languages, lang)
	}
	
	sort.Strings(languages)
	return languages
}

// FindBestVoice finds the best matching voice based on criteria
func (vm *VoiceManager) FindBestVoice(languageCode, gender, style string) (*Voice, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	// Start with language filter
	candidates, exists := vm.voicesByLang[languageCode]
	if !exists || len(candidates) == 0 {
		// Try to find a close match (e.g., en-US for en)
		for lang, voices := range vm.voicesByLang {
			if strings.HasPrefix(lang, strings.Split(languageCode, "-")[0]) {
				candidates = voices
				break
			}
		}
		
		if len(candidates) == 0 {
			return nil, fmt.Errorf("no voices found for language: %s", languageCode)
		}
	}
	
	// Filter by gender if specified
	if gender != "" {
		var filtered []*Voice
		for _, voice := range candidates {
			if strings.EqualFold(voice.Gender, gender) {
				filtered = append(filtered, voice)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	
	// Filter by style if specified
	if style != "" {
		var filtered []*Voice
		for _, voice := range candidates {
			if strings.EqualFold(voice.Style, style) {
				filtered = append(filtered, voice)
			}
		}
		if len(filtered) > 0 {
			candidates = filtered
		}
	}
	
	// Return the first match (could be enhanced with scoring)
	// TODO: Implement voice scoring algorithm based on multiple factors
	// TODO: Add user preference learning for voice selection
	// TODO: Implement voice similarity matching
	// TODO: Add support for voice blending/morphing
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	
	return nil, fmt.Errorf("no matching voice found")
}

// AddVoice adds a new voice to the manager
func (vm *VoiceManager) AddVoice(voice *Voice) error {
	if voice.ID == "" {
		return fmt.Errorf("voice ID is required")
	}
	
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.voices[voice.ID] = voice
	vm.voicesByLang[voice.LanguageCode] = append(vm.voicesByLang[voice.LanguageCode], voice)
	
	log.Printf("[VOICE_MANAGER] Added voice: %s (%s)", voice.DisplayName, voice.ID)
	return nil
}

// RemoveVoice removes a voice from the manager
func (vm *VoiceManager) RemoveVoice(voiceID string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	voice, exists := vm.voices[voiceID]
	if !exists {
		return fmt.Errorf("voice not found: %s", voiceID)
	}
	
	// Remove from main map
	delete(vm.voices, voiceID)
	
	// Remove from language map
	langVoices := vm.voicesByLang[voice.LanguageCode]
	for i, v := range langVoices {
		if v.ID == voiceID {
			vm.voicesByLang[voice.LanguageCode] = append(langVoices[:i], langVoices[i+1:]...)
			break
		}
	}
	
	log.Printf("[VOICE_MANAGER] Removed voice: %s", voiceID)
	return nil
}

// ToJSON exports voices to JSON format
func (vm *VoiceManager) ToJSON() ([]byte, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	return json.MarshalIndent(vm.GetAllVoices(), "", "  ")
}

// LoadFromJSON loads voices from JSON data
func (vm *VoiceManager) LoadFromJSON(data []byte) error {
	var voices []*Voice
	if err := json.Unmarshal(data, &voices); err != nil {
		return fmt.Errorf("failed to parse voices JSON: %w", err)
	}
	
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// Clear existing voices
	vm.voices = make(map[string]*Voice)
	vm.voicesByLang = make(map[string][]*Voice)
	
	// Load new voices
	for _, voice := range voices {
		vm.voices[voice.ID] = voice
		vm.voicesByLang[voice.LanguageCode] = append(vm.voicesByLang[voice.LanguageCode], voice)
	}
	
	log.Printf("[VOICE_MANAGER] Loaded %d voices from JSON", len(voices))
	return nil
}