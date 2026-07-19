package music

import (
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/kong/botdiscord/internal/queue"
)

type LoopMode int

const (
	LoopOff LoopMode = iota
	LoopOne
	LoopQueue
)

type Player struct {
	mu            sync.Mutex
	q             *queue.Queue
	current       *queue.Song
	session       *StreamSession
	vc            *discordgo.VoiceConnection
	loopMode      LoopMode
	paused        bool
	volume        float64
	skipRequested bool
}

func NewPlayer() *Player {
	return &Player{
		q:      queue.New(),
		volume: 1.0,
	}
}

func (p *Player) Play(song *queue.Song, vc *discordgo.VoiceConnection) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.session != nil {
		p.session.Stop()
		p.session = nil
	}
	p.current = song
	p.vc = vc
	p.paused = false
	p.skipRequested = false

	return p.startStreamLocked()
}

func (p *Player) Skip() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.skipRequested = true
	if p.session != nil {
		p.session.Stop()
		p.session = nil
	}
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.skipRequested = true
	if p.session != nil {
		p.session.Stop()
		p.session = nil
	}
	p.current = nil
	p.vc = nil
	p.q.Clear()
}

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.paused || p.session == nil {
		return
	}
	p.session.Stop()
	p.session = nil
	p.paused = true
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.paused || p.current == nil || p.vc == nil {
		return nil
	}
	p.paused = false
	p.skipRequested = false
	return p.startStreamLocked()
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session != nil
}

func (p *Player) IsPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paused
}

func (p *Player) NowPlaying() *queue.Song {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

func (p *Player) GetQueue() *queue.Queue {
	return p.q
}

func (p *Player) SetLoopMode(m LoopMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loopMode = m
}

func (p *Player) GetLoopMode() LoopMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loopMode
}

func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = v
}

func (p *Player) GetVolume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

func (p *Player) startStreamLocked() error {
	if p.current == nil || p.vc == nil {
		return nil
	}

	audioURL, err := GetAudioURL(p.current.URL)
	if err != nil {
		return err
	}

	ss, err := NewStreamSession(audioURL, p.vc, p.volume)
	if err != nil {
		return err
	}
	p.session = ss

	go func() {
		<-ss.finished
		p.onSongEnd()
	}()

	return nil
}

func (p *Player) onSongEnd() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.session = nil

	if p.skipRequested {
		p.skipRequested = false
	} else {
		switch p.loopMode {
		case LoopOne:
			if p.current != nil {
				p.startStreamLocked()
				return
			}
		case LoopQueue:
			if p.current != nil {
				p.q.Add(p.current)
			}
		}
	}

	next := p.q.Next()
	if next == nil {
		p.current = nil
		return
	}
	p.current = next
	p.startStreamLocked()
}