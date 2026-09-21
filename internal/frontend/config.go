package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kiltum/zxgo-v3/pkg/model"
)

// Version is the schema version of the files this package writes. Each file
// carries it, so a later version can tell what it is reading (D3).
const Version = 1

// ConfigDirName is the directory under the user's config directory that holds
// everything the front end persists.
const ConfigDirName = "zxgo-v3"

// Store reads and writes the front end's config directory: one JSON file per
// concern, each with a version, each falling back to embedded defaults (D3).
//
// A file that fails to parse, or that names something this build does not know, is
// reported and ignored - never fatal, and never rewritten behind the user's back
// as part of reading it. The app then behaves as a fresh install, and the next
// save replaces the file, which is what the report told the user would happen.
type Store struct {
	Dir string
}

// DefaultStore is the store in the user's config directory, creating nothing: a
// directory that does not exist yet is created on the first save, so a user who
// never changes a setting never gets one.
func DefaultStore() (Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Store{}, fmt.Errorf("frontend: locating the config directory: %w", err)
	}
	return Store{Dir: filepath.Join(base, ConfigDirName)}, nil
}

// EnsureDir creates the config directory if it is not there. Saving does this for
// itself, but the backend's ini files are written by the toolkit rather than
// through this store, so the directory has to exist before the windows open.
func (s Store) EnsureDir() error {
	if s.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("frontend: creating %s: %w", s.Dir, err)
	}
	return nil
}

// Path is the file a concern is stored in.
func (s Store) Path(name string) string { return filepath.Join(s.Dir, name+".json") }

// LayoutPath is the file the window layout lives in.
func (s Store) LayoutPath() string { return s.Path("layout") }

// layoutFile is the on-disk shape of a layout. It is separate from Layout so that
// the file can carry a version and name tools by string: a stored number would
// break the day the ToolID enum gained a member.
type layoutFile struct {
	Version int `json:"version"`
	// MenuVisible is a pointer so that a file which does not mention it keeps the
	// default rather than hiding the menubar: a plain bool would make "absent" and
	// "false" the same thing, and absent is the commoner of the two.
	MenuVisible *bool          `json:"menu_visible"`
	Main        windowRecord   `json:"main"`
	Tools       []windowRecord `json:"tools"`
}

type windowRecord struct {
	Tool string `json:"tool"`
	Open bool   `json:"open"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// LoadLayout reads the window layout, or returns nil when there is none.
//
// Nil rather than a zero Layout, and that is the whole point of the signature: a
// zero layout means "every tool closed, the menubar hidden", so a caller that could
// not tell the two apart would hide the menubar on a first launch and leave a user
// with no way to find the windows. A missing file is not an error - it is a first
// launch, and the defaults are the answer - so it is reported as "nothing to
// restore" instead.
func (s Store) LoadLayout() (*Layout, error) {
	path := s.LayoutPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("frontend: reading %s: %w", path, err)
	}

	var file layoutFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("frontend: %s is not readable as a layout: %w", path, err)
	}
	if file.Version > Version {
		return nil, fmt.Errorf("frontend: %s was written by a newer version (%d)",
			path, file.Version)
	}

	layout := file.layout()
	return &layout, nil
}

// layout turns the file's shape into the front end's, dropping anything this build
// does not recognise rather than failing on it: an unknown tool name is a file
// from another version, and the tools that are known are still worth restoring.
func (f layoutFile) layout() Layout {
	l := Layout{MenuVisible: true}
	if f.MenuVisible != nil {
		l.MenuVisible = *f.MenuVisible
	}
	if id, ok := ParseToolID(f.Main.Tool); ok && id == ToolMain {
		l.Main = WindowState{ID: ToolMain, Rect: f.Main.rect()}
	}
	for _, rec := range f.Tools {
		id, ok := ParseToolID(rec.Tool)
		if !ok || id == ToolMain {
			continue
		}
		l.Tools = append(l.Tools, WindowState{
			ID:   id,
			Open: rec.Open,
			Rect: rec.rect(),
		})
	}
	return l
}

func (r windowRecord) rect() Rect { return Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }

// SaveLayout writes the layout, replacing whatever was there.
//
// It writes to a temporary file and renames it into place, the same way the session
// is written (D11): a crash during the write then leaves the previous layout rather
// than half of a new one. Losing a layout is not serious - a file that fails to
// parse is ignored and the defaults come back - but a truncated one is a report the
// user has to read for no reason.
func (s Store) SaveLayout(l Layout) error {
	menu := l.MenuVisible
	file := layoutFile{
		Version:     Version,
		MenuVisible: &menu,
		Main:        recordFor(l.Main),
	}
	for _, w := range l.Tools {
		file.Tools = append(file.Tools, recordFor(w))
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("frontend: encoding the layout: %w", err)
	}
	data = append(data, '\n')
	return s.writeFile(s.LayoutPath(), data)
}

func recordFor(w WindowState) windowRecord {
	return windowRecord{
		Tool: w.ID.String(),
		Open: w.Open,
		X:    w.Rect.X,
		Y:    w.Rect.Y,
		W:    w.Rect.W,
		H:    w.Rect.H,
	}
}

// KeymapPath is the file the user's key bindings live in.
func (s Store) KeymapPath() string { return s.Path("keymap") }

// keymapFile is the on-disk shape of the user's bindings: a map from the way a
// binding is written ("cmd+shift+q") to the way an action is written
// ("screenshot"). Both spellings are the ones ParseBinding and ParseAction read, so
// a person can edit the file by hand, and a test can write one.
//
// It holds the user's *changes*, not the whole table: everything it does not mention
// keeps its default, which is what lets a keymap file survive a version that binds a
// new action (D3). An entry mapped to "none" is an explicit unbinding.
type keymapFile struct {
	Version  int               `json:"version"`
	Bindings map[string]string `json:"bindings"`
}

// LoadKeymap reads the user's bindings.
//
// A missing file returns an empty keymap and no error: a user who has never rebound
// anything has no file, and the defaults are the answer. A file that cannot be read
// at all is an error. A file that is *partly* readable returns what it could and an
// error naming what it could not - the same shape as a session with unknown chunks:
// the good entries are applied and the rest are reported, because a typo in one
// binding should not take the other twenty with it.
func (s Store) LoadKeymap() (Keymap, error) {
	path := s.KeymapPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("frontend: reading %s: %w", path, err)
	}

	var file keymapFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("frontend: %s is not readable as a keymap: %w", path, err)
	}
	if file.Version > Version {
		return nil, fmt.Errorf("frontend: %s was written by a newer version (%d)",
			path, file.Version)
	}

	km := make(Keymap, len(file.Bindings))
	var bad []string
	for text, actionText := range file.Bindings {
		binding, err := ParseBinding(text)
		if err != nil {
			bad = append(bad, text)
			continue
		}
		action, ok := ParseAction(actionText)
		if !ok {
			bad = append(bad, text+" = "+actionText)
			continue
		}
		km[binding] = action
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return km, fmt.Errorf("frontend: %s: %d entr%s not understood: %s",
			path, len(bad), plural(len(bad), "y", "ies"), strings.Join(bad, ", "))
	}
	return km, nil
}

// SaveKeymap writes the user's bindings, replacing whatever was there. Like the
// layout it goes to a temporary file and is renamed into place (D11's rule, applied
// to anything small that is worth not losing halfway).
//
// What it is given is the *diff* against the defaults - see DiffBindings - so an
// entry that is back to its default is left out rather than written down, and a
// removed default is written as "none".
func (s Store) SaveKeymap(km Keymap) error {
	file := keymapFile{Version: Version, Bindings: make(map[string]string, len(km))}
	for binding, action := range km {
		file.Bindings[binding.String()] = action.String()
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("frontend: encoding the keymap: %w", err)
	}
	data = append(data, '\n')
	return s.writeFile(s.KeymapPath(), data)
}

// DiffBindings reports how current differs from base, which is what turns the table
// the app holds into the file the next run reads.
//
// A binding the user took away is recorded as ActNone rather than left out: leaving
// it out would mean "no opinion", and the default would come back.
func DiffBindings(base, current Keymap) Keymap {
	diff := make(Keymap)
	for binding, action := range current {
		if base[binding] != action {
			diff[binding] = action
		}
	}
	for binding := range base {
		if _, ok := current[binding]; !ok {
			diff[binding] = ActNone
		}
	}
	return diff
}

// writeFile writes data atomically: a temporary file in the same directory, flushed
// and renamed over the target.
func (s Store) writeFile(path string, data []byte) error {
	if err := s.EnsureDir(); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.Dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("frontend: creating a temporary %s: %w", filepath.Base(path), err)
	}
	name := tmp.Name()
	defer func() {
		// Only reached when the rename below did not happen.
		if tmp != nil {
			tmp.Close()
			os.Remove(name)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("frontend: writing %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("frontend: flushing %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("frontend: closing %s: %w", name, err)
	}
	tmp = nil

	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("frontend: replacing %s: %w", path, err)
	}
	return nil
}

// plural picks the ending for a count, so a report reads as English.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// SettingsPath is the file the front end's settings live in.
func (s Store) SettingsPath() string { return s.Path("settings") }

// settingsFile is the on-disk shape of the settings: the last file chosen for each
// kind of dialog, and the machine the settings window configures. The kind is written
// as its name rather than its number, so a file survives the enum gaining a member.
type settingsFile struct {
	Version  int               `json:"version"`
	LastFile map[string]string `json:"last_file"`
	// Session is a pointer for the same reason the layout's menu_visible is: a file that
	// does not mention it keeps the default rather than turning it off.
	Session *bool `json:"session,omitempty"`

	// The machine and its switches, as the settings window writes them. Each is omitted
	// when the settings have no opinion, which is what keeps a model's own default - the
	// Pentagon's TurboSound - in force for a file that does not mention it. A pointer is
	// what makes "no opinion" expressible: a plain bool would turn off every switch the
	// file does not name.
	Model  string `json:"model,omitempty"`
	Z80    string `json:"z80,omitempty"`
	MEMPTR string `json:"memptr,omitempty"`
	// The switches are the same struct the settings hold, so the JSON names are written once
	// and an embedded struct's fields are flattened into this object.
	machineSwitches
}

// LoadSettings reads the settings. A missing file is a user who has never opened a
// file dialog, which is not an error and not a reason to refuse to start.
func (s Store) LoadSettings() (Settings, error) {
	path := s.SettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("frontend: reading %s: %w", path, err)
	}

	var file settingsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return Settings{}, fmt.Errorf("frontend: %s is not readable as settings: %w", path, err)
	}
	if file.Version > Version {
		return Settings{}, fmt.Errorf("frontend: %s was written by a newer version (%d)",
			path, file.Version)
	}

	out := Settings{Session: file.Session}
	var bad []string

	// The machine and the two CPU behaviours are checked against what this build knows, and a
	// value it does not know is reported rather than kept: a model that is not in the registry
	// would be a startup failure at the next launch, and a typo in a hand-edited file should be
	// visible now rather than at the next restart.
	if file.Model != "" {
		if _, ok := model.AllModels[file.Model]; ok {
			out.Model = file.Model
		} else {
			bad = append(bad, "model: "+file.Model)
		}
	}
	if file.Z80 != "" {
		switch file.Z80 {
		case Z80NMOS, Z80CMOS:
			out.Z80 = file.Z80
		default:
			bad = append(bad, "z80: "+file.Z80)
		}
	}
	if file.MEMPTR != "" {
		switch file.MEMPTR {
		case MEMPTRReal, MEMPTRDocumented:
			out.MEMPTR = file.MEMPTR
		default:
			bad = append(bad, "memptr: "+file.MEMPTR)
		}
	}
	for _, c := range switchChoices {
		c.set(&out.machineSwitches, copyBool(c.get(file.machineSwitches)))
	}

	// An entry naming a kind this build does not know is dropped rather than refused:
	// it is a file from a version that had one more dialog, and the rest of it is
	// still worth reading.
	// The report names the entry *and its value*: a key on its own can be useless, and
	// "unknown: /tapes/game.tap" says which entry it was and what is being ignored,
	// where a bare "unknown" says neither.
	for name, last := range file.LastFile {
		kind, ok := ParseDialogKind(name)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: %s", name, last))
			continue
		}
		if out.LastFile == nil {
			out.LastFile = make(map[DialogKind]string)
		}
		out.LastFile[kind] = last
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return out, fmt.Errorf("frontend: %s: %d entr%s not understood: %s",
			path, len(bad), plural(len(bad), "y", "ies"), strings.Join(bad, ", "))
	}
	return out, nil
}

// SaveSettings writes the settings, atomically as everything else is.
func (s Store) SaveSettings(set Settings) error {
	file := settingsFile{
		Version:  Version,
		LastFile: make(map[string]string, len(set.LastFile)),
		Session:  set.Session,
		Model:    set.Model,
		Z80:      set.Z80,
		MEMPTR:   set.MEMPTR,
	}
	// A value this build does not have is left out rather than written, for the same reason an
	// unnameable dialog kind is: the next load would report it as unreadable. That goes for a
	// model that is not in the registry, a Z80 variant that is not one of the two names, and a
	// MEMPTR behaviour that is not one of its two.
	if _, ok := model.AllModels[set.Model]; !ok {
		file.Model = ""
	}
	if _, ok := set.Z80Choice(); !ok {
		file.Z80 = ""
	}
	if _, ok := set.MEMPTRRealChoice(); !ok {
		file.MEMPTR = ""
	}
	for _, c := range switchChoices {
		if v := c.get(set.machineSwitches); v != nil {
			c.set(&file.machineSwitches, copyBool(v))
		}
	}
	for kind, last := range set.LastFile {
		// An unnameable kind is left out rather than written: writing it would put an
		// entry in the file that the next load reports as unreadable.
		if _, ok := ParseDialogKind(kind.String()); !ok {
			continue
		}
		file.LastFile[kind.String()] = last
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("frontend: encoding the settings: %w", err)
	}
	data = append(data, '\n')
	return s.writeFile(s.SettingsPath(), data)
}

// MatrixPath is the file the PC-to-ZX keyboard mapping lives in.
//
// It is a file of its own rather than a section of keymap.json, for D3's reason: they are
// two concerns with two writers - one is what a *host* key does, the other is which *ZX*
// key it stands for - and one file written by two of them would have each save dropping
// the other's section.
func (s Store) MatrixPath() string { return s.Path("matrix") }

// matrixFile is the on-disk shape: the host key, spelled as keymap.json spells it, mapped
// to the ZX keys it stands for, spelled by their legends ("caps-shift+0"). A value of
// "none" takes the host key off the machine.
type matrixFile struct {
	Version int               `json:"version"`
	Keys    map[string]string `json:"keys"`
}

// LoadMatrix reads the user's keyboard mapping, or returns nothing when there is no file.
//
// Like the action keymap it holds the *changes*: everything the file does not mention
// keeps the default the emulator ships, so a user who only wants BACKSPACE to be
// something else writes one line.
func (s Store) LoadMatrix() (MatrixMap, error) {
	path := s.MatrixPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("frontend: reading %s: %w", path, err)
	}

	var file matrixFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("frontend: %s is not readable as a keyboard map: %w", path, err)
	}
	if file.Version > Version {
		return nil, fmt.Errorf("frontend: %s was written by a newer version (%d)",
			path, file.Version)
	}

	m := make(MatrixMap, len(file.Keys))
	var bad []string
	for keyText, chordText := range file.Keys {
		key, ok := ParseKey(keyText)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: %s", keyText, chordText))
			continue
		}
		chord, ok := ParseZXChord(chordText)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: %s", keyText, chordText))
			continue
		}
		m[key] = chord
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return m, fmt.Errorf("frontend: %s: %d entr%s not understood: %s",
			path, len(bad), plural(len(bad), "y", "ies"), strings.Join(bad, ", "))
	}
	return m, nil
}

// SaveMatrix writes the user's keyboard mapping. It is given the diff against the
// defaults, so an entry that is back to its default is left out - see DiffMatrix.
func (s Store) SaveMatrix(m MatrixMap) error {
	file := matrixFile{Version: Version, Keys: make(map[string]string, len(m))}
	for key, chord := range m {
		file.Keys[key.Name()] = chord.String()
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("frontend: encoding the keyboard map: %w", err)
	}
	data = append(data, '\n')
	return s.writeFile(s.MatrixPath(), data)
}

// DiffMatrix reports how a keyboard mapping differs from the default one, which is what
// turns the table the front end holds into the file the next run reads. A host key the
// user took off the machine is recorded as a chord of nothing rather than left out:
// leaving it out would mean "no opinion" and the default would come back.
func DiffMatrix(base, current MatrixMap) MatrixMap {
	diff := make(MatrixMap)
	for key, chord := range current {
		if !chordEqual(base[key], chord) {
			diff[key] = chord
		}
	}
	for key := range base {
		if _, ok := current[key]; !ok {
			diff[key] = Chord{}
		}
	}
	return diff
}

func chordEqual(a, b Chord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
