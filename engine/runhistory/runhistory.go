// Package runhistory keeps a record of every run in files, one per day, at
// <folder>/<YYYY-MM-DD>.jsonl, with one finished run per line. Runs are
// written once, when they end, and read newest first, a day at a time, so
// files with the standard library do everything a database would here. See
// docs/design/automation-engine.md#run-history.
package runhistory

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SloanGwaltney/whendo/engine/runner"
)

const (
	// DayFormat is how days are written in file names and passed to
	// ReadDay: the year, month and day, such as "2026-11-15".
	DayFormat = "2006-01-02"
	// DefaultRetentionDays is how many days of run history are kept unless
	// the user chooses otherwise.
	DefaultRetentionDays = 30

	historyFileExtension = ".jsonl"
	pruneInterval        = 24 * time.Hour
	// Owner read and write only: run history holds viewers' names and
	// messages, which other users of the computer have no reason to see.
	historyFolderPermissions = 0o700
	historyFilePermissions   = 0o600
)

// ErrInvalidDay means a day isn't written in DayFormat.
var ErrInvalidDay = errors.New("day isn't in YYYY-MM-DD format")

// Store reads and writes run history in one folder. It's safe for
// concurrent use, since runs end on many goroutines at once.
type Store struct {
	folder string
	logger *slog.Logger
	// writeMutex keeps two runs ending at once from interleaving their
	// lines.
	writeMutex sync.Mutex
}

// NewStore returns a store for the given folder. The folder is created when
// the first run is written.
func NewStore(folder string, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{folder: folder, logger: logger}
}

// Write adds a finished run to the file for the day it ended, in local time,
// since that's the day the streamer will look for it.
func (store *Store) Write(record runner.RunRecord) error {
	encodedRecord, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encoding run %q: %w", record.RunID, err)
	}
	day := record.EndedAt.Local().Format(DayFormat)
	store.writeMutex.Lock()
	defer store.writeMutex.Unlock()
	err = os.MkdirAll(store.folder, historyFolderPermissions)
	if err != nil {
		return fmt.Errorf("creating the run history folder: %w", err)
	}
	return appendLine(store.filePath(day), encodedRecord)
}

// appendLine adds one line to the end of a file. If a crash cut the last
// line short, it first ends that line, so the damage stays on the broken
// line instead of spoiling the new one too.
func appendLine(filePath string, line []byte) (err error) {
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, historyFilePermissions)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer func() {
		closeErr := file.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing %s: %w", filePath, closeErr))
		}
	}()
	isLastLineCutShort, err := endsWithoutNewline(file)
	if err != nil {
		return fmt.Errorf("checking the end of %s: %w", filePath, err)
	}
	var contents []byte
	if isLastLineCutShort {
		contents = append(contents, '\n')
	}
	contents = append(contents, line...)
	contents = append(contents, '\n')
	// One write per record, so a record is never split across writes.
	_, err = file.Write(contents)
	if err != nil {
		return fmt.Errorf("writing to %s: %w", filePath, err)
	}
	return nil
}

func endsWithoutNewline(file *os.File) (bool, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	if fileInfo.Size() == 0 {
		return false, nil
	}
	lastByte := make([]byte, 1)
	_, err = file.ReadAt(lastByte, fileInfo.Size()-1)
	if err != nil {
		return false, err
	}
	return lastByte[0] != '\n', nil
}

// Days returns every day that has run history, newest first.
func (store *Store) Days() ([]string, error) {
	entries, err := os.ReadDir(store.folder)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the run history folder: %w", err)
	}
	var days []string
	for _, entry := range entries {
		day, isHistoryFile := dayOfFile(entry)
		if isHistoryFile {
			days = append(days, day)
		}
	}
	// Days in DayFormat sort by date when sorted as text.
	slices.Sort(days)
	slices.Reverse(days)
	return days, nil
}

func dayOfFile(entry fs.DirEntry) (string, bool) {
	if entry.IsDir() || filepath.Ext(entry.Name()) != historyFileExtension {
		return "", false
	}
	day := strings.TrimSuffix(entry.Name(), historyFileExtension)
	_, err := time.Parse(DayFormat, day)
	return day, err == nil
}

// ReadDay returns the runs that ended on a day, newest first. A day with no
// runs has no file and returns nothing. A line that can't be read, such as
// one a crash cut short, is logged and skipped, so one bad record doesn't
// hide the rest of the day.
func (store *Store) ReadDay(day string) ([]runner.RunRecord, error) {
	_, err := time.Parse(DayFormat, day)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidDay, day)
	}
	filePath := store.filePath(day)
	contents, err := os.ReadFile(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}
	records, err := store.parseRecords(filePath, contents)
	if err != nil {
		return nil, err
	}
	slices.Reverse(records)
	return records, nil
}

func (store *Store) parseRecords(filePath string, contents []byte) ([]runner.RunRecord, error) {
	var records []runner.RunRecord
	reader := bufio.NewReader(bytes.NewReader(contents))
	for lineNumber := 1; ; lineNumber++ {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("reading %s: %w", filePath, readErr)
		}
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var record runner.RunRecord
			err := json.Unmarshal(line, &record)
			if err != nil {
				store.logger.Warn("skipped a run history record that couldn't be read", "file", filePath, "line", lineNumber, "error", err)
			} else {
				records = append(records, record)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return records, nil
		}
	}
}

// Prune deletes the files of days more than retentionDays before today, so
// today and the retentionDays days before it are kept.
func (store *Store) Prune(retentionDays int, now time.Time) error {
	days, err := store.Days()
	if err != nil {
		return err
	}
	today := now.Local()
	oldestKeptDay := time.Date(today.Year(), today.Month(), today.Day()-retentionDays, 0, 0, 0, 0, time.Local).Format(DayFormat)
	var pruneErrors []error
	for _, day := range days {
		if day >= oldestKeptDay {
			continue
		}
		err := os.Remove(store.filePath(day))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			pruneErrors = append(pruneErrors, fmt.Errorf("deleting run history for %s: %w", day, err))
		}
	}
	return errors.Join(pruneErrors...)
}

// KeepPruned prunes right away and then once a day until ctx is cancelled.
// retentionDays is asked each time, since the user can change it while
// Whendo runs. A failed prune is logged and tried again the next day, since
// an old file that lingers does no harm.
func (store *Store) KeepPruned(ctx context.Context, retentionDays func() int) {
	store.pruneAndLog(retentionDays())
	ticker := time.NewTicker(pruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.pruneAndLog(retentionDays())
		}
	}
}

func (store *Store) pruneAndLog(retentionDays int) {
	err := store.Prune(retentionDays, time.Now())
	if err != nil {
		store.logger.Error("pruning run history failed", "error", err)
	}
}

func (store *Store) filePath(day string) string {
	return filepath.Join(store.folder, day+historyFileExtension)
}
