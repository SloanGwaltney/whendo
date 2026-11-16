// Package appsettings keeps the settings on Whendo's Settings page in one
// JSON file. Secrets never go here; they belong in the operating system's
// credential store. See docs/design/mvp-ui.md#settings.
package appsettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/SloanGwaltney/whendo/engine/runhistory"
)

// CurrentFormatVersion is the version of the settings file format this
// version of Whendo writes, so a later version can recognize and upgrade
// older files.
const CurrentFormatVersion = 1

// The run history retention period the user can choose, in days. A day is
// the least that's useful, and a year is far more than anyone needs to look
// back on while keeping the folder a sensible size.
const (
	MinimumRunHistoryRetentionDays = 1
	MaximumRunHistoryRetentionDays = 365
)

const (
	// Writes go to a temporary file first; see Save.
	temporaryFilePattern      = "settings-*.tmp"
	settingsFolderPermissions = 0o700
	fileIndentation           = "  "
)

// StreamingSoftware is the program the streamer streams with, which the
// show and hide steps act on. See docs/design/streaming-software.md.
type StreamingSoftware string

const (
	// StreamingSoftwareNone means Whendo doesn't connect to any streaming
	// software. It's the default, so streamers who don't need it are never
	// nagged about a connection.
	StreamingSoftwareNone              StreamingSoftware = "none"
	StreamingSoftwareStreamlabsDesktop StreamingSoftware = "streamlabsDesktop"
)

// isKnown reports whether this version of Whendo supports the program.
func (software StreamingSoftware) isKnown() bool {
	return software == StreamingSoftwareNone || software == StreamingSoftwareStreamlabsDesktop
}

var (
	// ErrUnknownStreamingSoftware means the streaming software isn't one
	// Whendo supports.
	ErrUnknownStreamingSoftware = errors.New("streaming software isn't one Whendo supports")
	// ErrRetentionOutOfRange means a run history retention period is
	// outside the range the user can choose.
	ErrRetentionOutOfRange = fmt.Errorf("run history has to be kept for %d to %d days", MinimumRunHistoryRetentionDays, MaximumRunHistoryRetentionDays)
	// ErrNewerFormat means the settings file was written by a newer version
	// of Whendo, in a format this version can't read.
	ErrNewerFormat = errors.New("settings file is from a newer version of Whendo")
)

// Settings are the user's choices from the Settings page.
type Settings struct {
	// RunHistoryRetentionDays is how many days before today run history is
	// kept for.
	RunHistoryRetentionDays int `json:"runHistoryRetentionDays"`
	// StreamingSoftware is the program Whendo connects to for the show and
	// hide steps.
	StreamingSoftware StreamingSoftware `json:"streamingSoftware"`
}

// Defaults returns the settings a new user starts with.
func Defaults() Settings {
	return Settings{RunHistoryRetentionDays: runhistory.DefaultRetentionDays, StreamingSoftware: StreamingSoftwareNone}
}

// Validate checks that every setting has a value the user could choose.
func (settings Settings) Validate() error {
	if settings.RunHistoryRetentionDays < MinimumRunHistoryRetentionDays || settings.RunHistoryRetentionDays > MaximumRunHistoryRetentionDays {
		return fmt.Errorf("%w, not %d", ErrRetentionOutOfRange, settings.RunHistoryRetentionDays)
	}
	if !settings.StreamingSoftware.isKnown() {
		return fmt.Errorf("%w: %q", ErrUnknownStreamingSoftware, settings.StreamingSoftware)
	}
	return nil
}

// settingsFile is the shape of the settings file: the settings themselves,
// plus the format version.
type settingsFile struct {
	FormatVersion int `json:"formatVersion"`
	Settings
}

// Store reads and writes the settings file. It doesn't guard against
// concurrent use, so one caller must own it.
type Store struct {
	filePath string
}

// NewStore returns a store for the settings file at filePath. The file and
// its folder are created when the settings are first saved.
func NewStore(filePath string) *Store {
	return &Store{filePath: filePath}
}

// Load reads the saved settings. With no file yet, it returns the defaults.
// A setting the file leaves out, such as one added after it was written,
// keeps its default.
func (store *Store) Load() (Settings, error) {
	contents, err := os.ReadFile(store.filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("reading %s: %w", store.filePath, err)
	}
	loadedFile := settingsFile{FormatVersion: CurrentFormatVersion, Settings: Defaults()}
	err = json.Unmarshal(contents, &loadedFile)
	if err != nil {
		return Settings{}, fmt.Errorf("reading %s: %w", store.filePath, err)
	}
	if loadedFile.FormatVersion > CurrentFormatVersion {
		return Settings{}, fmt.Errorf("%w: version %d", ErrNewerFormat, loadedFile.FormatVersion)
	}
	err = loadedFile.Settings.Validate()
	if err != nil {
		return Settings{}, fmt.Errorf("reading %s: %w", store.filePath, err)
	}
	return loadedFile.Settings, nil
}

// Save checks the settings and writes them. It writes to a temporary file in
// the same folder and renames it over the old one, so a crash mid-write never
// leaves a half-written file.
func (store *Store) Save(settings Settings) error {
	err := settings.Validate()
	if err != nil {
		return err
	}
	encodedFile, err := json.MarshalIndent(settingsFile{FormatVersion: CurrentFormatVersion, Settings: settings}, "", fileIndentation)
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	folder := filepath.Dir(store.filePath)
	err = os.MkdirAll(folder, settingsFolderPermissions)
	if err != nil {
		return fmt.Errorf("creating the settings folder: %w", err)
	}
	return writeFileAtomically(folder, store.filePath, encodedFile)
}

func writeFileAtomically(folder string, filePath string, contents []byte) error {
	temporaryFile, err := os.CreateTemp(folder, temporaryFilePattern)
	if err != nil {
		return fmt.Errorf("creating a temporary file for %s: %w", filePath, err)
	}
	err = writeAndClose(temporaryFile, contents)
	if err == nil {
		err = os.Rename(temporaryFile.Name(), filePath)
	}
	if err != nil {
		removeErr := os.Remove(temporaryFile.Name())
		if removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("removing the temporary file: %w", removeErr))
		}
		return fmt.Errorf("writing %s: %w", filePath, err)
	}
	return nil
}

// writeAndClose syncs before closing so the contents are on disk before the
// rename makes them the settings file.
func writeAndClose(file *os.File, contents []byte) error {
	_, writeErr := file.Write(contents)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}
