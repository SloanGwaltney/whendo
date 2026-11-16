package streamlabs

import (
	"context"
	"errors"

	"github.com/SloanGwaltney/whendo/engine/scenes"
)

// ProgramName is what streamers call Streamlabs Desktop.
const ProgramName = "Streamlabs Desktop"

// Program presents the connection as streaming software for the show and
// hide steps. See docs/design/streaming-software.md#the-interface.
type Program struct {
	connection *Connection
}

var _ scenes.Connection = Program{}

// NewProgram returns the connection as streaming software.
func NewProgram(connection *Connection) Program {
	return Program{connection: connection}
}

// IsConnected reports whether Streamlabs Desktop is connected.
func (program Program) IsConnected() bool {
	return program.connection.Status() == ConnectionStatusConnected
}

// Scene returns the scene with sceneID, and false when there's none.
func (program Program) Scene(ctx context.Context, sceneID string) (scenes.Scene, bool, error) {
	scene, isFound, err := program.connection.Scene(ctx, sceneID)
	if err != nil {
		return scenes.Scene{}, false, withoutRequestDetails(err)
	}
	if !isFound {
		return scenes.Scene{}, false, nil
	}
	return convertScene(scene), true, nil
}

// Scenes returns every scene in the loaded scene collection.
func (program Program) Scenes(ctx context.Context) ([]scenes.Scene, error) {
	streamlabsScenes, err := program.connection.Scenes(ctx)
	if err != nil {
		return nil, withoutRequestDetails(err)
	}
	convertedScenes := make([]scenes.Scene, 0, len(streamlabsScenes))
	for _, scene := range streamlabsScenes {
		convertedScenes = append(convertedScenes, convertScene(scene))
	}
	return convertedScenes, nil
}

// SetItemVisibility shows or hides one item.
func (program Program) SetItemVisibility(ctx context.Context, item scenes.Item, isVisible bool) error {
	err := program.connection.SetItemVisibility(ctx, item.Address, isVisible)
	if err != nil {
		return withoutRequestDetails(err)
	}
	return nil
}

// withoutRequestDetails keeps only the part of an error a streamer can act
// on, since it becomes the reason a step failed in run history. The
// requests' own context, such as resource IDs, means nothing to them and is
// already in the logs' reach through the connection.
func withoutRequestDetails(err error) error {
	var requestError *RequestError
	if errors.As(err, &requestError) {
		return requestError
	}
	for _, knownError := range []error{ErrNotConnected, ErrNoAnswer, ErrBusy, ErrConnectionLost} {
		if errors.Is(err, knownError) {
			return knownError
		}
	}
	return err
}

// convertScene turns Streamlabs' flat list of nodes into the scene's items,
// naming the folders each item is in.
func convertScene(scene Scene) scenes.Scene {
	foldersByID := map[string]Node{}
	for _, node := range scene.Nodes {
		if node.NodeType == NodeTypeFolder {
			foldersByID[node.ID] = node
		}
	}
	items := []scenes.Item{}
	for _, node := range scene.Nodes {
		if node.NodeType != NodeTypeItem {
			continue
		}
		items = append(items, scenes.Item{
			ID:         node.ID,
			SourceID:   node.SourceID,
			SourceName: node.Name,
			FolderPath: folderPath(node, foldersByID),
			IsVisible:  node.IsVisible,
			Address:    node.ResourceID,
		})
	}
	return scenes.Scene{ID: scene.ID, Name: scene.Name, Items: items}
}

// folderPath names the folders a node is in, outermost first. It stops at
// a folder it has already seen, so a broken parent chain can't loop.
func folderPath(node Node, foldersByID map[string]Node) []string {
	var path []string
	seenFolderIDs := map[string]bool{}
	parentID := node.ParentID
	for parentID != "" && !seenFolderIDs[parentID] {
		folder, isFolder := foldersByID[parentID]
		if !isFolder {
			break
		}
		seenFolderIDs[parentID] = true
		path = append([]string{folder.Name}, path...)
		parentID = folder.ParentID
	}
	return path
}
