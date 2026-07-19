package music

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/bwmarrin/discordgo"
)

type StreamSession struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	done     chan struct{}
	finished chan struct{}
}

func NewStreamSession(audioURL string, vc *discordgo.VoiceConnection, volume float64) (*StreamSession, error) {
	args := []string{
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-vn",
		"-i", audioURL,
		"-af", fmt.Sprintf("volume=%.2f", volume),
		"-c:a", "libopus",
		"-b:a", "192k",
		"-application", "audio",
		"-compression_level", "5",
		"-ar", "48000",
		"-ac", "2",
		"-frame_duration", "20",
		"-f", "ogg",
		"-loglevel", "quiet",
		"pipe:1",
	}
	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg start: %w", err)
	}

	ss := &StreamSession{
		cmd:      cmd,
		stdout:   stdout,
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}

	go ss.stream(vc)
	return ss, nil
}

func (ss *StreamSession) stream(vc *discordgo.VoiceConnection) {
	defer close(ss.finished)
	defer ss.cmd.Wait()

	pktCh := make(chan []byte, 64)

	go func() {
		defer close(pktCh)
		for {
			packets, err := readOggPage(ss.stdout)
			if err != nil {
				return
			}
			for _, pkt := range packets {
				select {
				case pktCh <- pkt:
				case <-ss.done:
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ss.done:
			return
		case <-vc.Dead:
			return
		}

		select {
		case pkt, ok := <-pktCh:
			if !ok {
				return
			}
			select {
			case vc.OpusSend <- pkt:
			case <-ss.done:
				return
			case <-vc.Dead:
				return
			}
		case <-ss.done:
			return
		case <-vc.Dead:
			return
		}
	}
}

func readOggPage(r io.Reader) ([][]byte, error) {
	header := make([]byte, 27)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	if string(header[:4]) != "OggS" {
		return nil, fmt.Errorf("invalid Ogg page magic")
	}

	numSegments := int(header[26])
	segTable := make([]byte, numSegments)
	if _, err := io.ReadFull(r, segTable); err != nil {
		return nil, err
	}

	var dataSize int
	for _, l := range segTable {
		dataSize += int(l)
	}

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	if header[5]&0x02 != 0 {
		return nil, nil
	}

	var packets [][]byte
	var current []byte
	offset := 0
	for _, l := range segTable {
		if l == 0 {
			continue
		}
		current = append(current, data[offset:offset+int(l)]...)
		if l < 255 {
			packets = append(packets, current)
			current = nil
		}
		offset += int(l)
	}

	return packets, nil
}

func (ss *StreamSession) Stop() {
	close(ss.done)
	if ss.cmd != nil && ss.cmd.Process != nil {
		ss.cmd.Process.Kill()
	}
	<-ss.finished
}