//go:build noaudio

package audio

import "testing"

func TestNoAudio(t *testing.T) {
	if tr, err := Load([]byte("ID3")); tr != nil || err == nil {
		t.Errorf("Load: %v, %v", tr, err)
	}
	if Err() == nil {
		t.Error("Err: nil")
	}
}
