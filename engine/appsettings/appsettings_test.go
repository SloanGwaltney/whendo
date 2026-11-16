package appsettings_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
)

const settingsFileName = "settings.json"

func newStore(t *testing.T) (*appsettings.Store, string) {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "whendo", settingsFileName)
	return appsettings.NewStore(filePath), filePath
}

func writeFileOrFail(t *testing.T, filePath string, contents string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(filePath), 0o700)
	if err == nil {
		err = os.WriteFile(filePath, []byte(contents), 0o600)
	}
	if err != nil {
		t.Fatalf("writing %s: %v", filePath, err)
	}
}

func TestLoad_ReturnsDefaultsWhenThereIsNoFile(t *testing.T) {
	store, _ := newStore(t)

	settings, err := store.Load()

	if err != nil || settings != appsettings.Defaults() {
		t.Errorf("got %+v, error %v; want the defaults", settings, err)
	}
}

func TestSave_SettingsAreLoadedBack(t *testing.T) {
	store, _ := newStore(t)
	saved := appsettings.Settings{RunHistoryRetentionDays: 7, StreamingSoftware: appsettings.StreamingSoftwareStreamlabsDesktop}

	err := store.Save(saved)
	if err != nil {
		t.Fatalf("Save returned an error: %v", err)
	}
	loaded, err := store.Load()

	if err != nil || loaded != saved {
		t.Errorf("got %+v, error %v; want %+v", loaded, err, saved)
	}
}

func TestSave_RefusesRetentionOutsideTheRange(t *testing.T) {
	for _, retentionDays := range []int{0, appsettings.MaximumRunHistoryRetentionDays + 1} {
		store, filePath := newStore(t)

		err := store.Save(appsettings.Settings{RunHistoryRetentionDays: retentionDays, StreamingSoftware: appsettings.StreamingSoftwareNone})

		if !errors.Is(err, appsettings.ErrRetentionOutOfRange) {
			t.Errorf("saving %d days gave error %v, want ErrRetentionOutOfRange", retentionDays, err)
		}
		if _, statErr := os.Stat(filePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("saving %d days wrote the file anyway", retentionDays)
		}
	}
}

func TestSave_RefusesUnknownStreamingSoftware(t *testing.T) {
	store, _ := newStore(t)
	settings := appsettings.Defaults()
	settings.StreamingSoftware = "someOtherProgram"

	err := store.Save(settings)

	if !errors.Is(err, appsettings.ErrUnknownStreamingSoftware) {
		t.Errorf("got error %v, want ErrUnknownStreamingSoftware", err)
	}
}

func TestLoad_ChoosesNoStreamingSoftwareForFilesWrittenBeforeTheChoiceExisted(t *testing.T) {
	store, filePath := newStore(t)
	writeFileOrFail(t, filePath, `{"formatVersion": 1, "runHistoryRetentionDays": 7}`)

	settings, err := store.Load()

	if err != nil || settings.StreamingSoftware != appsettings.StreamingSoftwareNone || settings.RunHistoryRetentionDays != 7 {
		t.Errorf("got %+v, error %v; want 7 days and no streaming software", settings, err)
	}
}

func TestLoad_KeepsDefaultsForSettingsTheFileLeavesOut(t *testing.T) {
	store, filePath := newStore(t)
	writeFileOrFail(t, filePath, `{"formatVersion": 1}`)

	settings, err := store.Load()

	if err != nil || settings != appsettings.Defaults() {
		t.Errorf("got %+v, error %v; want the defaults", settings, err)
	}
}

func TestLoad_ReturnsErrorForFileFromNewerVersion(t *testing.T) {
	store, filePath := newStore(t)
	writeFileOrFail(t, filePath, `{"formatVersion": 99, "runHistoryRetentionDays": 7}`)

	_, err := store.Load()

	if !errors.Is(err, appsettings.ErrNewerFormat) {
		t.Errorf("got error %v, want ErrNewerFormat", err)
	}
}

func TestLoad_ReturnsErrorForUnreadableFile(t *testing.T) {
	store, filePath := newStore(t)
	writeFileOrFail(t, filePath, `{"formatVersion": 1, "runHistoryRet`)

	_, err := store.Load()

	if err == nil {
		t.Error("got no error for a file cut short")
	}
}
