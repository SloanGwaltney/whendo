package scenes

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
)

// SourceSettingKind is a source in a scene, stored as a SourceInScene. The
// designer shows a picker of the program's scenes and sources for it.
const SourceSettingKind automationsetting.Kind = "scenes.sceneSource"

// ErrUnreadableSource means a source setting doesn't hold a scene and a
// source, which only a hand-edited file can cause.
var ErrUnreadableSource = errors.New("the source setting doesn't hold a scene and a source")

// SourceInScene is a source picked in one scene. IDs find it, so renaming
// the scene or the source doesn't break an automation. Names are shown
// when the program isn't connected, and find it again when its IDs don't,
// such as for a source deleted and made again with the same name.
type SourceInScene struct {
	SceneID    string `json:"sceneId"`
	SceneName  string `json:"sceneName"`
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
}

// ReadSource reads a source setting.
func ReadSource(settings automationsetting.Values, settingID string) (SourceInScene, error) {
	rawValue, isPresent := settings[settingID]
	if !isPresent {
		return SourceInScene{}, fmt.Errorf("%w: %q has no value", automationsetting.ErrSettingMissing, settingID)
	}
	var source SourceInScene
	err := json.Unmarshal(rawValue, &source)
	if err != nil {
		return SourceInScene{}, fmt.Errorf("%w: %w", ErrUnreadableSource, err)
	}
	if !source.hasScene() || !source.hasSource() {
		return SourceInScene{}, ErrUnreadableSource
	}
	return source, nil
}

func (source SourceInScene) hasScene() bool {
	return source.SceneID != "" || source.SceneName != ""
}

func (source SourceInScene) hasSource() bool {
	return source.SourceID != "" || source.SourceName != ""
}

// FindItems returns every item in the scene that shows the source: by the
// source's ID, or by its name if no item has the ID. Every match counts,
// since showing only one copy of a source would be a surprise.
func FindItems(scene Scene, source SourceInScene) []Item {
	itemsWithID := matchingItems(scene, func(item Item) bool {
		return source.SourceID != "" && item.SourceID == source.SourceID
	})
	if len(itemsWithID) > 0 {
		return itemsWithID
	}
	return matchingItems(scene, func(item Item) bool {
		return source.SourceName != "" && item.SourceName == source.SourceName
	})
}

func matchingItems(scene Scene, isMatch func(Item) bool) []Item {
	var matches []Item
	for _, item := range scene.Items {
		if isMatch(item) {
			matches = append(matches, item)
		}
	}
	return matches
}

// findScene finds the source's scene in a list: by ID, or by name if no
// scene has the ID.
func findScene(scenes []Scene, source SourceInScene) (Scene, bool) {
	for _, scene := range scenes {
		if source.SceneID != "" && scene.ID == source.SceneID {
			return scene, true
		}
	}
	for _, scene := range scenes {
		if source.SceneName != "" && scene.Name == source.SceneName {
			return scene, true
		}
	}
	return Scene{}, false
}
