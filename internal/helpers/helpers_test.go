package helpers

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCreateWavHeader(t *testing.T) {
	// Test case 1: Standard mono 16-bit PCM at 24kHz
	testCase1 := struct {
		dataSize      int
		numChannels   int
		sampleRate    int
		bitsPerSample int
	}{
		dataSize:      1000,
		numChannels:   1,
		sampleRate:    24000,
		bitsPerSample: 16,
	}

	header1 := CreateWavHeader(testCase1.dataSize, testCase1.numChannels, testCase1.sampleRate, testCase1.bitsPerSample)

	// Header should be 44 bytes
	if len(header1) != 44 {
		t.Errorf("CreateWavHeader() returned header of length %d, want 44", len(header1))
	}

	// Verify RIFF header
	if !bytes.Equal(header1[0:4], []byte("RIFF")) {
		t.Errorf("header[0:4] = %v, want 'RIFF'", header1[0:4])
	}

	// Verify ChunkSize (file size - 8)
	chunkSize := binary.LittleEndian.Uint32(header1[4:8])
	expectedChunkSize := uint32(testCase1.dataSize + 36) // 36 bytes for header after ChunkSize
	if chunkSize != expectedChunkSize {
		t.Errorf("ChunkSize = %d, want %d", chunkSize, expectedChunkSize)
	}

	// Verify WAVE format
	if !bytes.Equal(header1[8:12], []byte("WAVE")) {
		t.Errorf("header[8:12] = %v, want 'WAVE'", header1[8:12])
	}

	// Verify fmt subchunk
	if !bytes.Equal(header1[12:16], []byte("fmt ")) {
		t.Errorf("header[12:16] = %v, want 'fmt '", header1[12:16])
	}

	// Verify format subchunk size (16 for PCM)
	subchunk1Size := binary.LittleEndian.Uint32(header1[16:20])
	if subchunk1Size != 16 {
		t.Errorf("Subchunk1Size = %d, want 16", subchunk1Size)
	}

	// Verify AudioFormat (1 for PCM)
	audioFormat := binary.LittleEndian.Uint16(header1[20:22])
	if audioFormat != 1 {
		t.Errorf("AudioFormat = %d, want 1", audioFormat)
	}

	// Verify NumChannels
	numChannels := binary.LittleEndian.Uint16(header1[22:24])
	if numChannels != uint16(testCase1.numChannels) {
		t.Errorf("NumChannels = %d, want %d", numChannels, testCase1.numChannels)
	}

	// Verify SampleRate
	sampleRate := binary.LittleEndian.Uint32(header1[24:28])
	if sampleRate != uint32(testCase1.sampleRate) {
		t.Errorf("SampleRate = %d, want %d", sampleRate, testCase1.sampleRate)
	}

	// Verify ByteRate (SampleRate * NumChannels * BitsPerSample/8)
	byteRate := binary.LittleEndian.Uint32(header1[28:32])
	expectedByteRate := uint32(testCase1.sampleRate * testCase1.numChannels * testCase1.bitsPerSample / 8)
	if byteRate != expectedByteRate {
		t.Errorf("ByteRate = %d, want %d", byteRate, expectedByteRate)
	}

	// Verify BlockAlign (NumChannels * BitsPerSample/8)
	blockAlign := binary.LittleEndian.Uint16(header1[32:34])
	expectedBlockAlign := uint16(testCase1.numChannels * testCase1.bitsPerSample / 8)
	if blockAlign != expectedBlockAlign {
		t.Errorf("BlockAlign = %d, want %d", blockAlign, expectedBlockAlign)
	}

	// Verify BitsPerSample
	bitsPerSample := binary.LittleEndian.Uint16(header1[34:36])
	if bitsPerSample != uint16(testCase1.bitsPerSample) {
		t.Errorf("BitsPerSample = %d, want %d", bitsPerSample, testCase1.bitsPerSample)
	}

	// Verify data subchunk
	if !bytes.Equal(header1[36:40], []byte("data")) {
		t.Errorf("header[36:40] = %v, want 'data'", header1[36:40])
	}

	// Verify data size
	dataSize := binary.LittleEndian.Uint32(header1[40:44])
	if dataSize != uint32(testCase1.dataSize) {
		t.Errorf("DataSize = %d, want %d", dataSize, testCase1.dataSize)
	}

	// Test case 2: Stereo 24-bit PCM at 48kHz
	testCase2 := struct {
		dataSize      int
		numChannels   int
		sampleRate    int
		bitsPerSample int
	}{
		dataSize:      2000,
		numChannels:   2,
		sampleRate:    48000,
		bitsPerSample: 24,
	}

	header2 := CreateWavHeader(testCase2.dataSize, testCase2.numChannels, testCase2.sampleRate, testCase2.bitsPerSample)

	// Verify a few key parameters to ensure they were set correctly
	numChannels2 := binary.LittleEndian.Uint16(header2[22:24])
	if numChannels2 != uint16(testCase2.numChannels) {
		t.Errorf("NumChannels = %d, want %d", numChannels2, testCase2.numChannels)
	}

	sampleRate2 := binary.LittleEndian.Uint32(header2[24:28])
	if sampleRate2 != uint32(testCase2.sampleRate) {
		t.Errorf("SampleRate = %d, want %d", sampleRate2, testCase2.sampleRate)
	}

	bitsPerSample2 := binary.LittleEndian.Uint16(header2[34:36])
	if bitsPerSample2 != uint16(testCase2.bitsPerSample) {
		t.Errorf("BitsPerSample = %d, want %d", bitsPerSample2, testCase2.bitsPerSample)
	}

	dataSize2 := binary.LittleEndian.Uint32(header2[40:44])
	if dataSize2 != uint32(testCase2.dataSize) {
		t.Errorf("DataSize = %d, want %d", dataSize2, testCase2.dataSize)
	}
}

func TestIsAudioTraceEnabled(t *testing.T) {
	// We can't directly test the init function, but we can test the IsAudioTraceEnabled function

	// By default, it should be false unless set via env var in the running process
	result := IsAudioTraceEnabled()

	// Create a basic test that the function can be called
	// The actual value will depend on whether the env var is set, which varies based on the environment
	t.Logf("IsAudioTraceEnabled() returned: %v", result)

	// Manual test with setting directly for testing
	audioTraceEnabled = 0
	if IsAudioTraceEnabled() {
		t.Error("IsAudioTraceEnabled() should return false when flag is 0")
	}

	audioTraceEnabled = 1
	if !IsAudioTraceEnabled() {
		t.Error("IsAudioTraceEnabled() should return true when flag is 1")
	}
}
