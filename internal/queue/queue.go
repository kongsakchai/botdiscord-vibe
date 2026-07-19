package queue

import "sync"

type Song struct {
	Title     string
	URL       string
	Duration  string
	Requester string
}

type Queue struct {
	mu    sync.Mutex
	songs []*Song
}

func New() *Queue {
	return &Queue{}
}

func (q *Queue) Add(s *Song) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.songs = append(q.songs, s)
}

func (q *Queue) Next() *Song {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.songs) == 0 {
		return nil
	}
	s := q.songs[0]
	q.songs = q.songs[1:]
	return s
}

func (q *Queue) Remove(index int) *Song {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index >= len(q.songs) {
		return nil
	}
	s := q.songs[index]
	q.songs = append(q.songs[:index], q.songs[index+1:]...)
	return s
}

func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.songs = q.songs[:0]
}

func (q *Queue) List() []*Song {
	q.mu.Lock()
	defer q.mu.Unlock()
	cp := make([]*Song, len(q.songs))
	copy(cp, q.songs)
	return cp
}

func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.songs)
}