package frontend

import (
	"os"
	"path/filepath"
	"strings"
)

// The extensions the path flags are named after. A path given without one gets
// it appended on the way out and is looked for with it on the way in, so
// "-session game" and "-session game.zxstate" are the same request.
const (
	StateExt  = ".zxstate"
	ReplayExt = ".replay"
)

// StatePathFor resolves the path a session is read from or written to. An
// explicit -load-state or -save-state is that half's path and wins; -session
// supplies both halves at once. Empty means the half was not asked for.
func StatePathFor(explicit, session string) string {
	if explicit != "" {
		return explicit
	}
	return session
}

// SavePath names the file a save will write: the path as given when it already
// carries the format's extension, and the path with the extension appended
// otherwise. `-save-state game` writes game.zxstate rather than a file whose
// name does not say what is in it, and a typo in the extension is a new file
// rather than an overwritten one.
func SavePath(path, ext string) string {
	if path == "" || strings.EqualFold(filepath.Ext(path), ext) {
		return path
	}
	return path + ext
}

// LoadPath names the file a load will read: the path as given if that file
// exists, then the path with the extension appended if *that* exists, and the
// suffixed name otherwise - so a file that is not there is reported under the
// name a save would have written, which is the name the user is looking for.
//
// The literal path wins over the suffixed one, deliberately: a user who names an
// existing file means that file, whatever it is called, and `-session game`
// written by an earlier run is found by the second step.
func LoadPath(path, ext string) string {
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return SavePath(path, ext)
}
