package streamlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// The resource and methods Whendo calls. See the design's "The Requests
// Whendo Uses".
const (
	scenesServiceResource = "ScenesService"
	activeSceneIDMethod   = "activeSceneId"
	getSceneMethod        = "getScene"
	getScenesMethod       = "getScenes"
	setVisibilityMethod   = "setVisibility"
)

// NodeType says whether a scene node is an item, which shows a source, or
// a folder of other nodes.
type NodeType string

const (
	NodeTypeItem   NodeType = "item"
	NodeTypeFolder NodeType = "folder"
)

// Display is which canvas an item is on. Streamlabs' dual output adds a
// vertical canvas, with its own item for each source shown on it.
type Display string

const (
	DisplayHorizontal Display = "horizontal"
	DisplayVertical   Display = "vertical"
)

// Node is an item or folder in a scene. Fields that only items or only
// folders have are empty for the other.
type Node struct {
	ID       string   `json:"id"`
	SceneID  string   `json:"sceneId"`
	NodeType NodeType `json:"sceneNodeType"`
	// ParentID is the folder the node is in, or empty at the top level.
	ParentID string `json:"parentId"`
	// Name is a folder's name, or an item's source's name.
	Name      string  `json:"name"`
	SourceID  string  `json:"sourceId"`
	IsVisible bool    `json:"visible"`
	Display   Display `json:"display"`
	// ResourceID is what SetItemVisibility needs. It's only good until the
	// item is deleted, so it's never kept between runs.
	ResourceID string `json:"resourceId"`
}

// Scene is a scene with every item and folder in it, including those
// inside folders, as one flat list.
type Scene struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

// Scene returns the scene with sceneID, and false if there's no such scene
// in the scene collection Streamlabs has loaded.
func (connection *Connection) Scene(ctx context.Context, sceneID string) (Scene, bool, error) {
	result, err := connection.call(ctx, scenesServiceResource, getSceneMethod, []any{sceneID})
	if err != nil {
		return Scene{}, false, fmt.Errorf("getting scene %q: %w", sceneID, err)
	}
	if isNullResult(result) {
		return Scene{}, false, nil
	}
	var scene Scene
	err = json.Unmarshal(result, &scene)
	if err != nil {
		return Scene{}, false, fmt.Errorf("reading scene %q: %w", sceneID, err)
	}
	return scene, true, nil
}

// Scenes returns every scene in the scene collection Streamlabs has
// loaded. Streamlabs limits how often this can be called, so steps use
// Scene instead.
func (connection *Connection) Scenes(ctx context.Context) ([]Scene, error) {
	result, err := connection.call(ctx, scenesServiceResource, getScenesMethod, nil)
	if err != nil {
		return nil, fmt.Errorf("getting the scenes: %w", err)
	}
	var scenes []Scene
	err = json.Unmarshal(result, &scenes)
	if err != nil {
		return nil, fmt.Errorf("reading the scenes: %w", err)
	}
	return scenes, nil
}

// SetItemVisibility shows or hides the item with itemResourceID, like
// clicking its eye icon.
func (connection *Connection) SetItemVisibility(ctx context.Context, itemResourceID string, isVisible bool) error {
	_, err := connection.call(ctx, itemResourceID, setVisibilityMethod, []any{isVisible})
	if err != nil {
		return fmt.Errorf("setting the visibility of %s: %w", itemResourceID, err)
	}
	return nil
}

// isNullResult reports whether a result is null or missing, which is how
// Streamlabs answers a method that returns nothing.
func isNullResult(result json.RawMessage) bool {
	trimmedResult := bytes.TrimSpace(result)
	return len(trimmedResult) == 0 || bytes.Equal(trimmedResult, []byte("null"))
}
