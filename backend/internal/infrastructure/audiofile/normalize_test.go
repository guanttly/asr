package audiofile

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeToWav16kMono(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.wav")
	outputPath := filepath.Join(tempDir, "output.wav")
	if err := writeTestWav(inputPath, 8000, 800); err != nil {
		t.Fatalf("write input wav: %v", err)
	}

	if err := NormalizeToWav16kMono(context.Background(), inputPath, outputPath); err != nil {
		t.Fatalf("NormalizeToWav16kMono returned error: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("stat normalized wav: %v", err)
	}
	if info.Size() <= 44 {
		t.Fatalf("expected normalized wav with payload, got size %d", info.Size())
	}

	duration, err := ProbeDuration(context.Background(), outputPath)
	if err != nil {
		t.Fatalf("probe normalized duration: %v", err)
	}
	if duration <= 0 {
		t.Fatalf("expected positive duration, got %v", duration)
	}
}

func writeTestWav(path string, sampleRate, sampleCount int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	dataSize := sampleCount * 2
	chunkSize := 36 + dataSize

	if _, err := file.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(chunkSize)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataSize)); err != nil {
		return err
	}

	for i := 0; i < sampleCount; i++ {
		if err := binary.Write(file, binary.LittleEndian, int16(i%32*256)); err != nil {
			return err
		}
	}

	return nil
}
