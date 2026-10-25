//go:build noaudio

package audio

import (
	"errors"
	"time"
)

var errNoAudio = errors.New("audio: built without sound")

type Track struct{}

func Load([]byte) (*Track, error) { return nil, errNoAudio }

func Err() error { return errNoAudio }

func (*Track) Play() {}

func (*Track) Pause() {}

func (*Track) Rewind() {}

func (*Track) Seek(time.Duration) {}

func (*Track) Playing() bool { return false }

func (*Track) Position() time.Duration { return 0 }

func (*Track) Length() time.Duration { return 0 }

func (*Track) Err() error { return errNoAudio }

func (*Track) Close() {}
