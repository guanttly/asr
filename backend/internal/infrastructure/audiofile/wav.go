package audiofile

import (
	"encoding/binary"
	"fmt"
	"os"
)

// WritePCM16MonoWAV writes raw 16-bit little-endian mono PCM data into a WAV container.
func WritePCM16MonoWAV(outputPath string, pcmData []byte, sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("sample rate must be greater than 0")
	}
	if len(pcmData)%2 != 0 {
		return fmt.Errorf("pcm data length must be aligned to 16-bit samples")
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	dataSize := len(pcmData)
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
	if _, err := file.Write(pcmData); err != nil {
		return err
	}

	return nil
}
