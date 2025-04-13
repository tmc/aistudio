package helpers

import (
	"encoding/binary"
	"log"
	"os"
	"sync/atomic"
)

// --- Audio Tracing ---
var audioTraceEnabled int32 // Use atomic for safe check across goroutines

func init() {
	if os.Getenv("AISTUDIO_AUDIO_TRACE") == "1" {
		atomic.StoreInt32(&audioTraceEnabled, 1)
		log.Println("--- Detailed audio pipeline tracing enabled (AISTUDIO_AUDIO_TRACE=1) ---")
	}
}

// IsAudioTraceEnabled checks if detailed audio tracing is enabled via environment variable.
func IsAudioTraceEnabled() bool {
	return atomic.LoadInt32(&audioTraceEnabled) == 1
}

// CreateWavHeader creates a simple WAV header for the given parameters.
// dataSize is the size of the raw audio data chunk only.
func CreateWavHeader(dataSize, numChannels, sampleRate, bitsPerSample int) []byte {
	header := make([]byte, 44)
	totalSize := uint32(dataSize + 36) // 36 = bytes remaining after ChunkSize field (44 - 8)
	byteRate := uint32(sampleRate * numChannels * bitsPerSample / 8)
	blockAlign := uint16(numChannels * bitsPerSample / 8)

	// RIFF Header ("RIFF" chunk descriptor)
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], totalSize) // ChunkSize
	copy(header[8:12], []byte("WAVE"))                    // Format

	// Format Subchunk ("fmt " subchunk)
	copy(header[12:16], []byte("fmt "))                   // Subchunk1ID
	binary.LittleEndian.PutUint32(header[16:20], 16)      // Subchunk1Size for PCM
	binary.LittleEndian.PutUint16(header[20:22], 1)       // AudioFormat 1 for PCM
	binary.LittleEndian.PutUint16(header[22:24], uint16(numChannels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))

	// Data Subchunk ("data" subchunk)
	copy(header[36:40], []byte("data"))              // Subchunk2ID
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize)) // Subchunk2Size

	return header
}
