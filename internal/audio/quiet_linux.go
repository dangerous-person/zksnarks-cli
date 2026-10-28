//go:build !noaudio

package audio

import (
	"sync"

	"github.com/ebitengine/purego"
)

var hushOnce sync.Once

func init() { hush = func() { hushOnce.Do(hushALSA) } }

func hushALSA() {
	defer func() { recover() }()
	for _, name := range []string{"libasound.so.2", "libasound.so"} {
		lib, err := purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			continue
		}
		var set func(handler uintptr) int32
		purego.RegisterLibFunc(&set, lib, "snd_lib_error_set_handler")
		set(purego.NewCallback(func(file, line, function, errnum, format uintptr) {}))
		return
	}
}
