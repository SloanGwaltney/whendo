// Package scenes holds what every streaming program shares: the interface
// each program's connection provides, the setting that picks a source in a
// scene, and the steps that show and hide it. Automations never name the
// program, so a streamer who changes programs keeps their automations. See
// docs/design/streaming-software.md.
package scenes

import (
	"context"
	"sync"
)

// Program is the streaming software the streamer chose in Settings, if any.
type Program interface {
	// Chosen returns the chosen program, and false when none is chosen.
	Chosen() (ChosenProgram, bool)
}

// ChosenProgram is the program the streamer chose and its connection.
type ChosenProgram struct {
	// Name is what the streamer calls it, such as "Streamlabs Desktop".
	Name       string
	Connection Connection
}

// Connection is what each program's connection provides. Its errors are
// worded for the streamer, since they become the reason a step failed.
type Connection interface {
	IsConnected() bool
	// Scene returns the scene with sceneID, and false when there's none.
	Scene(ctx context.Context, sceneID string) (Scene, bool, error)
	// Scenes returns every scene. Programs may limit how often it can be
	// called, so steps call it only when Scene can't find a scene.
	Scenes(ctx context.Context) ([]Scene, error)
	// SetItemVisibility shows or hides one item, like clicking its eye icon.
	SetItemVisibility(ctx context.Context, item Item, isVisible bool) error
}

// Scene is a scene and every item in it, including items in folders.
type Scene struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

// Item is one source shown in a scene. The same source can be in a scene
// more than once, such as once on each of Streamlabs' two canvases.
type Item struct {
	ID         string `json:"id"`
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
	// FolderPath names the folders the item is in, outermost first, or is
	// empty at the top level.
	FolderPath []string `json:"folderPath"`
	IsVisible  bool     `json:"isVisible"`
	// Address is whatever the program needs to find the item again. Only
	// the program reads it.
	Address string `json:"-"`
}

// KnownScenes remembers the scenes the last list had, so a source that's
// gone can be pointed out without asking the program again. A source only
// counts as missing while a list is known: a program that can't be
// reached never makes one missing.
type KnownScenes struct {
	mutex sync.Mutex
	// scenes is nil until a list is known.
	scenes []Scene
}

// Remember keeps a list of scenes, replacing the last one.
func (known *KnownScenes) Remember(scenes []Scene) {
	rememberedScenes := append([]Scene{}, scenes...)
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.scenes = rememberedScenes
}

// Forget drops the last list, for when the program disconnects or the
// streamer chooses another, so nothing is called missing on old news.
func (known *KnownScenes) Forget() {
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.scenes = nil
}

// isMissing reports whether the last list confirms the source is gone,
// even by name.
func (known *KnownScenes) isMissing(source SourceInScene) bool {
	known.mutex.Lock()
	defer known.mutex.Unlock()
	if known.scenes == nil {
		return false
	}
	scene, isFound := findScene(known.scenes, source)
	if !isFound {
		return true
	}
	return len(FindItems(scene, source)) == 0
}
