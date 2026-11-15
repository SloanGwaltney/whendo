package automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CurrentFormatVersion is the version of the automation file format this
// version of Whendo writes. When it changes, files in older versions will
// need upgrading as they're read; see
// docs/design/automation-engine.md#saving-automations.
const CurrentFormatVersion = 1

const (
	automationFileExtension = ".json"
	// Writes go to a temporary file first; see Save.
	temporaryFilePattern = "*.tmp"
	// Owner read and write only: automations can hold things like reward
	// IDs that other users of the computer have no reason to see.
	automationFolderPermissions = 0o700
	fileIndentation             = "  "
)

var (
	// ErrNewerFormat means an automation file was written by a newer version
	// of Whendo, in a format this version can't read.
	ErrNewerFormat = errors.New("automation file is from a newer version of Whendo")
	// ErrUnknownFormat means an automation file doesn't say which format
	// version it's in.
	ErrUnknownFormat = errors.New("automation file has no format version")
)

// automationFile is the shape of an automation's file: the automation
// itself, plus the format version.
type automationFile struct {
	FormatVersion int `json:"formatVersion"`
	Automation
}

// LoadFailure is an automation file that couldn't be read.
type LoadFailure struct {
	FileName string
	Reason   error
}

// Store saves automations as files in one folder, one file per automation,
// named by its ID so renaming an automation doesn't move its file. It doesn't
// guard against concurrent use, so one caller must own it.
type Store struct {
	folder string
}

// NewStore returns a store for the given folder. The folder is created when
// the first automation is saved.
func NewStore(folder string) *Store {
	return &Store{folder: folder}
}

// Save writes an automation to its file. It writes to a temporary file in the
// same folder and renames it over the old one, so a crash mid-write never
// leaves a half-written automation. It checks only that the ID is safe as a
// file name; checking the rest is up to the caller, through Validate.
func (store *Store) Save(automation Automation) error {
	err := validateID(automation.ID)
	if err != nil {
		return fmt.Errorf("saving an automation: %w", err)
	}
	encodedFile, err := json.MarshalIndent(automationFile{FormatVersion: CurrentFormatVersion, Automation: automation}, "", fileIndentation)
	if err != nil {
		return fmt.Errorf("encoding automation %q: %w", automation.ID, err)
	}
	err = os.MkdirAll(store.folder, automationFolderPermissions)
	if err != nil {
		return fmt.Errorf("creating the automations folder: %w", err)
	}
	return store.writeFileAtomically(store.filePath(automation.ID), encodedFile)
}

func (store *Store) writeFileAtomically(filePath string, contents []byte) error {
	temporaryFile, err := os.CreateTemp(store.folder, temporaryFilePattern)
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
// rename makes them the automation's file.
func writeAndClose(file *os.File, contents []byte) error {
	_, writeErr := file.Write(contents)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

// LoadAll reads every automation in the folder. Files that can't be read are
// returned as failures rather than stopping the load, so one broken file
// doesn't hide every other automation. A missing folder means there are no
// automations yet.
func (store *Store) LoadAll() ([]Automation, []LoadFailure, error) {
	entries, err := os.ReadDir(store.folder)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the automations folder: %w", err)
	}
	var automations []Automation
	var failures []LoadFailure
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != automationFileExtension {
			continue
		}
		loadedAutomation, err := store.load(entry.Name())
		if err != nil {
			failures = append(failures, LoadFailure{FileName: entry.Name(), Reason: err})
			continue
		}
		automations = append(automations, loadedAutomation)
	}
	return automations, failures, nil
}

func (store *Store) load(fileName string) (Automation, error) {
	contents, err := os.ReadFile(filepath.Join(store.folder, fileName))
	if err != nil {
		return Automation{}, fmt.Errorf("reading the file: %w", err)
	}
	var file automationFile
	err = json.Unmarshal(contents, &file)
	if err != nil {
		return Automation{}, fmt.Errorf("reading the file: %w", err)
	}
	if file.FormatVersion == 0 {
		return Automation{}, ErrUnknownFormat
	}
	if file.FormatVersion > CurrentFormatVersion {
		return Automation{}, fmt.Errorf("%w: format version %d", ErrNewerFormat, file.FormatVersion)
	}
	// A file whose name doesn't match its ID would be saved to a second file
	// the next time it changed, leaving two copies.
	expectedID := strings.TrimSuffix(fileName, automationFileExtension)
	if file.ID != expectedID {
		return Automation{}, fmt.Errorf("the file is named %q but holds automation %q", fileName, file.ID)
	}
	return file.Automation, nil
}

// Delete removes an automation's file. Deleting an automation that has no
// file succeeds, since the result is the same.
func (store *Store) Delete(automationID string) error {
	err := validateID(automationID)
	if err != nil {
		return fmt.Errorf("deleting an automation: %w", err)
	}
	err = os.Remove(store.filePath(automationID))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("deleting automation %q: %w", automationID, err)
	}
	return nil
}

func (store *Store) filePath(automationID string) string {
	return filepath.Join(store.folder, automationID+automationFileExtension)
}
