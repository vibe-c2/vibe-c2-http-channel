package config

import (
	"context"
	"log"
	"sync"
	"time"

	coreProfile "github.com/vibe-c2/vibe-c2-golang-channel-core/pkg/profile"
	"github.com/fsnotify/fsnotify"
)

type ProfilesState struct {
	mu       sync.RWMutex
	profiles []coreProfile.Profile
}

func NewProfilesState(initial []coreProfile.Profile) *ProfilesState {
	cp := append([]coreProfile.Profile(nil), initial...)
	return &ProfilesState{profiles: cp}
}

func (s *ProfilesState) Profiles() []coreProfile.Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]coreProfile.Profile(nil), s.profiles...)
}

func (s *ProfilesState) Replace(next []coreProfile.Profile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = append([]coreProfile.Profile(nil), next...)
}

func StartProfilesWatcher(ctx context.Context, dir string, state *ProfilesState, logger *log.Logger) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return err
	}

	reload := func() {
		profiles, err := LoadProfiles(dir)
		if err != nil {
			logger.Printf("profiles watcher: reload failed: %v", err)
			return
		}
		state.Replace(profiles)
		logger.Printf("profiles watcher: loaded profiles=%d", len(profiles))
	}

	go func() {
		defer w.Close()
		defer logger.Printf("profiles watcher: stopped")

		var debounce <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				debounce = time.After(150 * time.Millisecond)
			case err, ok := <-w.Errors:
				if ok {
					logger.Printf("profiles watcher: fsnotify error: %v", err)
				}
			case <-debounce:
				reload()
				debounce = nil
			}
		}
	}()

	return nil
}
