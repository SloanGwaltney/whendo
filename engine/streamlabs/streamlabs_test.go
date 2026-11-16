package streamlabs_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/streamlabs"
)

const (
	testRetryDelay     = 20 * time.Millisecond
	testRequestTimeout = 300 * time.Millisecond
	testBusyRetryDelay = 10 * time.Millisecond
	waitTimeout        = 5 * time.Second
	pollInterval       = 5 * time.Millisecond
	testActiveSceneID  = "scene_active"
	busyAnswerLine     = `{"id":null,"jsonrpc":"2.0","error":{"code":-32603,"message":"API server is busy. Try again later"}}`
)

// receivedRequest is a request as the fake Streamlabs read it.
type receivedRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Resource string            `json:"resource"`
		Args     []json.RawMessage `json:"args"`
	} `json:"params"`
}

// requestHandler answers one request by writing to the client itself, so a
// test can answer late, in pieces, several times or not at all.
type requestHandler func(client net.Conn, request receivedRequest)

// fakeStreamlabs imitates Streamlabs Desktop's local TCP API. It answers
// activeSceneId by default; tests add handlers for other methods.
type fakeStreamlabs struct {
	t        *testing.T
	listener net.Listener

	mutex    sync.Mutex
	handlers map[string]requestHandler
	requests []receivedRequest
	clients  []net.Conn
}

func newFakeStreamlabs(t *testing.T) *fakeStreamlabs {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting the fake Streamlabs: %v", err)
	}
	fake := &fakeStreamlabs{t: t, listener: listener, handlers: map[string]requestHandler{}}
	fake.handle("activeSceneId", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, testActiveSceneID)
	})
	go fake.acceptClients()
	t.Cleanup(fake.stop)
	return fake
}

func (fake *fakeStreamlabs) address() string {
	return fake.listener.Addr().String()
}

func (fake *fakeStreamlabs) handle(method string, handler requestHandler) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.handlers[method] = handler
}

func (fake *fakeStreamlabs) receivedRequests() []receivedRequest {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]receivedRequest(nil), fake.requests...)
}

func (fake *fakeStreamlabs) acceptClients() {
	for {
		client, err := fake.listener.Accept()
		if err != nil {
			return
		}
		fake.mutex.Lock()
		fake.clients = append(fake.clients, client)
		fake.mutex.Unlock()
		go fake.serve(client)
	}
}

func (fake *fakeStreamlabs) serve(client net.Conn) {
	reader := bufio.NewReader(client)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request receivedRequest
		err = json.Unmarshal(line, &request)
		if err != nil {
			fake.t.Errorf("the fake Streamlabs got a line that isn't a JSON request: %q", line)
			return
		}
		fake.mutex.Lock()
		fake.requests = append(fake.requests, request)
		handler := fake.handlers[request.Method]
		fake.mutex.Unlock()
		if handler == nil {
			fake.t.Errorf("the fake Streamlabs has no handler for %q", request.Method)
			continue
		}
		handler(client, request)
	}
}

func (fake *fakeStreamlabs) stop() {
	closeErr := fake.listener.Close()
	if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		fake.t.Errorf("stopping the fake Streamlabs: %v", closeErr)
	}
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, client := range fake.clients {
		closeClient(fake.t, client)
	}
}

func closeClient(t *testing.T, client net.Conn) {
	t.Helper()
	err := client.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		t.Errorf("the fake Streamlabs couldn't close a client: %v", err)
	}
}

func writeLine(t *testing.T, client net.Conn, line string) {
	t.Helper()
	_, err := client.Write([]byte(line + "\n"))
	if err != nil {
		t.Errorf("the fake Streamlabs couldn't write: %v", err)
	}
}

func resultLine(t *testing.T, requestID int64, result any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": requestID, "result": result})
	if err != nil {
		t.Fatalf("encoding a result: %v", err)
	}
	return string(encoded)
}

func writeResult(t *testing.T, client net.Conn, requestID int64, result any) {
	t.Helper()
	writeLine(t, client, resultLine(t, requestID, result))
}

// startConnection runs a Connection to the fake until the test ends, and
// waits for it to connect.
func startConnection(t *testing.T, fake *fakeStreamlabs) *streamlabs.Connection {
	t.Helper()
	connection := newTestConnection(fake.address())
	runConnection(t, connection)
	waitForStatus(t, connection, streamlabs.ConnectionStatusConnected)
	return connection
}

func newTestConnection(address string) *streamlabs.Connection {
	return streamlabs.NewConnection(streamlabs.Config{
		Address:        address,
		RetryDelay:     testRetryDelay,
		RequestTimeout: testRequestTimeout,
		BusyRetryDelay: testBusyRetryDelay,
	})
}

func runConnection(t *testing.T, connection *streamlabs.Connection) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		connection.Run(ctx)
		close(stopped)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
}

func waitForStatus(t *testing.T, connection *streamlabs.Connection, status streamlabs.ConnectionStatus) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for connection.Status() != status {
		if time.Now().After(deadline) {
			t.Fatalf("status is %q, want %q", connection.Status(), status)
		}
		time.Sleep(pollInterval)
	}
}

func sceneWithHeadPets(sceneID string) map[string]any {
	return map[string]any{
		"_type":      "HELPER",
		"resourceId": fmt.Sprintf(`Scene["%s"]`, sceneID),
		"id":         sceneID,
		"name":       "Gameplay",
		"nodes": []map[string]any{
			{"id": "folder_1", "sceneId": sceneID, "sceneNodeType": "folder", "name": "Alerts", "childrenIds": []string{"item_1"}},
			{
				"id": "item_1", "sceneId": sceneID, "sceneNodeType": "item", "parentId": "folder_1",
				"sceneItemId": "item_1", "sourceId": "browser_source_1", "name": "Head Pets",
				"visible": false, "display": "horizontal",
				"resourceId": fmt.Sprintf(`SceneItem["%s", "item_1", "browser_source_1"]`, sceneID),
			},
		},
	}
}

func TestConnection_ConnectsOnceStreamlabsAnswers(t *testing.T) {
	fake := newFakeStreamlabs(t)

	startConnection(t, fake)

	requests := fake.receivedRequests()
	if len(requests) == 0 || requests[0].Method != "activeSceneId" || requests[0].Params.Resource != "ScenesService" {
		t.Fatalf("first request = %+v, want ScenesService.activeSceneId", requests)
	}
}

func TestConnection_WaitsWhileStreamlabsAcceptsButDoesNotAnswer(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var isStarted atomic.Bool
	fake.handle("activeSceneId", func(client net.Conn, request receivedRequest) {
		// Streamlabs opens its port before it has finished starting, and
		// answers nothing until then.
		if isStarted.Load() {
			writeResult(t, client, request.ID, testActiveSceneID)
		}
	})
	connection := newTestConnection(fake.address())
	runConnection(t, connection)

	time.Sleep(2 * testRequestTimeout)
	if connection.Status() != streamlabs.ConnectionStatusWaiting {
		t.Fatalf("status = %q while Streamlabs doesn't answer, want waiting", connection.Status())
	}
	isStarted.Store(true)
	waitForStatus(t, connection, streamlabs.ConnectionStatusConnected)
}

func TestConnection_WaitsWhileNothingIsListening(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	unusedAddress := listener.Addr().String()
	err = listener.Close()
	if err != nil {
		t.Fatalf("freeing the port: %v", err)
	}
	connection := newTestConnection(unusedAddress)
	runConnection(t, connection)

	time.Sleep(5 * testRetryDelay)

	if connection.Status() != streamlabs.ConnectionStatusWaiting {
		t.Fatalf("status = %q, want waiting", connection.Status())
	}
	_, _, err = connection.Scene(context.Background(), "scene_1")
	if !errors.Is(err, streamlabs.ErrNotConnected) {
		t.Fatalf("Scene error = %v, want ErrNotConnected", err)
	}
}

func TestConnection_ReconnectsAfterStreamlabsCloses(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var statusChanges []streamlabs.ConnectionStatus
	var statusMutex sync.Mutex
	connection := streamlabs.NewConnection(streamlabs.Config{
		Address:        fake.address(),
		RetryDelay:     testRetryDelay,
		RequestTimeout: testRequestTimeout,
		BusyRetryDelay: testBusyRetryDelay,
		OnStatusChange: func(status streamlabs.ConnectionStatus) {
			statusMutex.Lock()
			defer statusMutex.Unlock()
			statusChanges = append(statusChanges, status)
		},
	})
	runConnection(t, connection)
	waitForStatus(t, connection, streamlabs.ConnectionStatusConnected)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, sceneWithHeadPets("scene_1"))
	})

	fake.mutex.Lock()
	for _, client := range fake.clients {
		closeClient(t, client)
	}
	fake.mutex.Unlock()

	deadline := time.Now().Add(waitTimeout)
	for {
		statusMutex.Lock()
		changeCount := len(statusChanges)
		statusMutex.Unlock()
		if changeCount >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status changes = %v, want connected, waiting, connected", statusChanges)
		}
		time.Sleep(pollInterval)
	}
	_, isFound, err := connection.Scene(context.Background(), "scene_1")
	if err != nil || !isFound {
		t.Fatalf("Scene after reconnecting = %v, %v, want the scene", isFound, err)
	}
}

func TestScene_ReturnsItemsAndFolders(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, sceneWithHeadPets("scene_1"))
	})
	connection := startConnection(t, fake)

	scene, isFound, err := connection.Scene(context.Background(), "scene_1")

	if err != nil || !isFound {
		t.Fatalf("Scene = %v, %v, want the scene", isFound, err)
	}
	if scene.ID != "scene_1" || scene.Name != "Gameplay" || len(scene.Nodes) != 2 {
		t.Fatalf("scene = %+v", scene)
	}
	folder, item := scene.Nodes[0], scene.Nodes[1]
	if folder.NodeType != streamlabs.NodeTypeFolder || folder.Name != "Alerts" {
		t.Errorf("folder = %+v", folder)
	}
	expectedItem := streamlabs.Node{
		ID: "item_1", SceneID: "scene_1", NodeType: streamlabs.NodeTypeItem, ParentID: "folder_1",
		Name: "Head Pets", SourceID: "browser_source_1", IsVisible: false, Display: streamlabs.DisplayHorizontal,
		ResourceID: `SceneItem["scene_1", "item_1", "browser_source_1"]`,
	}
	if item != expectedItem {
		t.Errorf("item = %+v, want %+v", item, expectedItem)
	}
	lastRequest := fake.receivedRequests()[len(fake.receivedRequests())-1]
	if lastRequest.Params.Resource != "ScenesService" || len(lastRequest.Params.Args) != 1 || string(lastRequest.Params.Args[0]) != `"scene_1"` {
		t.Errorf("request = %+v, want ScenesService.getScene(\"scene_1\")", lastRequest)
	}
}

func TestScene_ReportsMissingSceneAsNotFound(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, nil)
	})
	connection := startConnection(t, fake)

	_, isFound, err := connection.Scene(context.Background(), "deleted_scene")

	if err != nil || isFound {
		t.Fatalf("Scene = %v, %v, want not found and no error", isFound, err)
	}
}

func TestScenes_ReturnsEveryScene(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScenes", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, []any{sceneWithHeadPets("scene_1"), sceneWithHeadPets("scene_2")})
	})
	connection := startConnection(t, fake)

	scenes, err := connection.Scenes(context.Background())

	if err != nil {
		t.Fatalf("Scenes: %v", err)
	}
	if len(scenes) != 2 || scenes[1].ID != "scene_2" || len(scenes[1].Nodes) != 2 {
		t.Fatalf("scenes = %+v", scenes)
	}
}

func TestSetItemVisibility_CallsSetVisibilityOnTheItem(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("setVisibility", func(client net.Conn, request receivedRequest) {
		writeLine(t, client, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d}`, request.ID))
	})
	connection := startConnection(t, fake)
	itemResourceID := `SceneItem["scene_1", "item_1", "browser_source_1"]`

	err := connection.SetItemVisibility(context.Background(), itemResourceID, true)

	if err != nil {
		t.Fatalf("SetItemVisibility: %v", err)
	}
	lastRequest := fake.receivedRequests()[len(fake.receivedRequests())-1]
	if lastRequest.Params.Resource != itemResourceID || len(lastRequest.Params.Args) != 1 || string(lastRequest.Params.Args[0]) != "true" {
		t.Fatalf("request = %+v, want setVisibility(true) on the item", lastRequest)
	}
}

func TestRequests_AskAgainWhileStreamlabsIsBusy(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var answerCount atomic.Int32
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		if answerCount.Add(1) <= 3 {
			writeLine(t, client, busyAnswerLine)
			return
		}
		writeResult(t, client, request.ID, sceneWithHeadPets("scene_1"))
	})
	connection := startConnection(t, fake)

	_, isFound, err := connection.Scene(context.Background(), "scene_1")

	if err != nil || !isFound {
		t.Fatalf("Scene = %v, %v, want the scene after Streamlabs stops being busy", isFound, err)
	}
	if answerCount.Load() != 4 {
		t.Fatalf("Streamlabs was asked %d times, want 4", answerCount.Load())
	}
}

func TestRequests_AskAgainWhenOverTheRateLimit(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var answerCount atomic.Int32
	fake.handle("getScenes", func(client net.Conn, request receivedRequest) {
		if answerCount.Add(1) == 1 {
			writeLine(t, client, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32600,"message":"Reached the limit of calls for \"ScenesService.getScenes\""}}`, request.ID))
			return
		}
		writeResult(t, client, request.ID, []any{})
	})
	connection := startConnection(t, fake)

	_, err := connection.Scenes(context.Background())

	if err != nil {
		t.Fatalf("Scenes: %v", err)
	}
}

func TestRequests_FailWhenStreamlabsStaysBusy(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeLine(t, client, busyAnswerLine)
	})
	connection := startConnection(t, fake)

	_, _, err := connection.Scene(context.Background(), "scene_1")

	if !errors.Is(err, streamlabs.ErrBusy) {
		t.Fatalf("Scene error = %v, want ErrBusy", err)
	}
}

func TestRequests_ReturnStreamlabsErrors(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("setVisibility", func(client net.Conn, request receivedRequest) {
		writeLine(t, client, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32602,"message":"resource not found: SceneItem"}}`, request.ID))
	})
	connection := startConnection(t, fake)

	err := connection.SetItemVisibility(context.Background(), `SceneItem["gone"]`, true)

	var requestError *streamlabs.RequestError
	if !errors.As(err, &requestError) {
		t.Fatalf("error = %v, want a RequestError", err)
	}
	if requestError.Code != -32602 || requestError.Message != "resource not found: SceneItem" {
		t.Fatalf("RequestError = %+v", requestError)
	}
}

func TestRequests_FailWhenStreamlabsDoesNotAnswer(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {})
	connection := startConnection(t, fake)

	_, _, err := connection.Scene(context.Background(), "scene_1")

	if !errors.Is(err, streamlabs.ErrNoAnswer) {
		t.Fatalf("Scene error = %v, want ErrNoAnswer", err)
	}
	if connection.Status() != streamlabs.ConnectionStatusConnected {
		t.Fatalf("status = %q after one unanswered request, want connected", connection.Status())
	}
}

func TestRequests_IgnoreLateAnswersToEarlierRequests(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var unansweredRequestID atomic.Int64
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		if unansweredRequestID.Load() == 0 {
			unansweredRequestID.Store(request.ID)
			return
		}
		writeResult(t, client, unansweredRequestID.Load(), sceneWithHeadPets("stale_scene"))
		writeResult(t, client, request.ID, sceneWithHeadPets("scene_1"))
	})
	connection := startConnection(t, fake)
	_, _, err := connection.Scene(context.Background(), "stale_scene")
	if !errors.Is(err, streamlabs.ErrNoAnswer) {
		t.Fatalf("first Scene error = %v, want ErrNoAnswer", err)
	}

	scene, _, err := connection.Scene(context.Background(), "scene_1")

	if err != nil || scene.ID != "scene_1" {
		t.Fatalf("second Scene = %+v, %v, want scene_1", scene, err)
	}
}

func TestRequests_ReadAnswersSplitAcrossWritesAndEvents(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		eventLine := `{"jsonrpc":"2.0","result":{"_type":"EVENT","emitter":"STREAM","resourceId":"ScenesService.sceneSwitched","data":{}}}`
		answer := eventLine + "\n" + resultLine(t, request.ID, sceneWithHeadPets("scene_1")) + "\n"
		splitAt := len(answer) / 2
		_, err := client.Write([]byte(answer[:splitAt]))
		if err != nil {
			t.Errorf("writing the first half: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
		_, err = client.Write([]byte(answer[splitAt:]))
		if err != nil {
			t.Errorf("writing the second half: %v", err)
		}
	})
	connection := startConnection(t, fake)

	scene, isFound, err := connection.Scene(context.Background(), "scene_1")

	if err != nil || !isFound || scene.ID != "scene_1" {
		t.Fatalf("Scene = %+v, %v, %v", scene, isFound, err)
	}
}

func TestRequests_AreSentOneAtATimeAsSingleLines(t *testing.T) {
	fake := newFakeStreamlabs(t)
	var unansweredCount atomic.Int32
	var hasOverlapped atomic.Bool
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		if unansweredCount.Add(1) > 1 {
			hasOverlapped.Store(true)
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			unansweredCount.Add(-1)
			writeResult(t, client, request.ID, sceneWithHeadPets("scene_1"))
		}()
	})
	connection := startConnection(t, fake)
	const concurrentRequestCount = 5

	var waitGroup sync.WaitGroup
	for range concurrentRequestCount {
		waitGroup.Go(func() {
			_, _, err := connection.Scene(context.Background(), "scene_1")
			if err != nil {
				t.Errorf("Scene: %v", err)
			}
		})
	}
	waitGroup.Wait()

	if hasOverlapped.Load() {
		t.Fatal("a request was sent before the one before it was answered")
	}
	seenIDs := map[int64]bool{}
	for _, request := range fake.receivedRequests() {
		if request.JSONRPC != "2.0" || request.ID == 0 || seenIDs[request.ID] {
			t.Errorf("request %+v needs version 2.0 and a new, nonzero ID", request)
		}
		seenIDs[request.ID] = true
	}
}

func TestRequests_FailWhenTheConnectionIsLostWhileWaiting(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		closeClient(t, client)
	})
	connection := startConnection(t, fake)

	_, _, err := connection.Scene(context.Background(), "scene_1")

	if !errors.Is(err, streamlabs.ErrConnectionLost) {
		t.Fatalf("Scene error = %v, want ErrConnectionLost", err)
	}
}

func sceneWithNestedFolders() map[string]any {
	return map[string]any{
		"id":   "scene_1",
		"name": "Gameplay",
		"nodes": []map[string]any{
			{"id": "folder_outer", "sceneId": "scene_1", "sceneNodeType": "folder", "name": "Overlays"},
			{"id": "folder_inner", "sceneId": "scene_1", "sceneNodeType": "folder", "name": "Alerts", "parentId": "folder_outer"},
			{"id": "item_1", "sceneId": "scene_1", "sceneNodeType": "item", "parentId": "folder_inner", "sourceId": "browser_source_1", "name": "Head Pets", "visible": true, "resourceId": `SceneItem["scene_1", "item_1", "browser_source_1"]`},
			{"id": "item_2", "sceneId": "scene_1", "sceneNodeType": "item", "sourceId": "webcam_1", "name": "Webcam", "resourceId": `SceneItem["scene_1", "item_2", "webcam_1"]`},
		},
	}
}

func TestProgram_TurnsNodesIntoItemsWithTheirFolders(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, sceneWithNestedFolders())
	})
	program := streamlabs.NewProgram(startConnection(t, fake))

	scene, isFound, err := program.Scene(context.Background(), "scene_1")

	if err != nil || !isFound {
		t.Fatalf("Scene = %v, %v", isFound, err)
	}
	if scene.ID != "scene_1" || scene.Name != "Gameplay" || len(scene.Items) != 2 {
		t.Fatalf("scene = %+v, want Gameplay with its two items and no folders", scene)
	}
	headPets, webcam := scene.Items[0], scene.Items[1]
	if headPets.ID != "item_1" || headPets.SourceID != "browser_source_1" || headPets.SourceName != "Head Pets" || !headPets.IsVisible {
		t.Errorf("Head Pets = %+v", headPets)
	}
	if fmt.Sprint(headPets.FolderPath) != "[Overlays Alerts]" {
		t.Errorf("Head Pets' folders = %v, want Overlays then Alerts", headPets.FolderPath)
	}
	if len(webcam.FolderPath) != 0 || webcam.IsVisible {
		t.Errorf("Webcam = %+v, want hidden at the top level", webcam)
	}
}

func TestProgram_ShowsAnItemByItsResourceID(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {
		writeResult(t, client, request.ID, sceneWithNestedFolders())
	})
	fake.handle("setVisibility", func(client net.Conn, request receivedRequest) {
		writeLine(t, client, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d}`, request.ID))
	})
	program := streamlabs.NewProgram(startConnection(t, fake))
	scene, _, err := program.Scene(context.Background(), "scene_1")
	if err != nil {
		t.Fatalf("Scene: %v", err)
	}

	err = program.SetItemVisibility(context.Background(), scene.Items[1], true)

	if err != nil {
		t.Fatalf("SetItemVisibility: %v", err)
	}
	lastRequest := fake.receivedRequests()[len(fake.receivedRequests())-1]
	if lastRequest.Params.Resource != `SceneItem["scene_1", "item_2", "webcam_1"]` {
		t.Fatalf("resource = %q, want the webcam's resource ID", lastRequest.Params.Resource)
	}
}

func TestProgram_GivesErrorsWithoutRequestDetails(t *testing.T) {
	fake := newFakeStreamlabs(t)
	fake.handle("getScene", func(client net.Conn, request receivedRequest) {})
	program := streamlabs.NewProgram(startConnection(t, fake))

	_, _, err := program.Scene(context.Background(), "scene_1")

	if !errors.Is(err, streamlabs.ErrNoAnswer) || err.Error() != "Streamlabs Desktop didn't answer" {
		t.Fatalf("error = %q, want just that Streamlabs Desktop didn't answer", err)
	}
}

func TestProgram_ReportsWhetherItIsConnected(t *testing.T) {
	fake := newFakeStreamlabs(t)
	connection := newTestConnection(fake.address())
	program := streamlabs.NewProgram(connection)
	if program.IsConnected() {
		t.Fatal("connected before running")
	}

	runConnection(t, connection)
	waitForStatus(t, connection, streamlabs.ConnectionStatusConnected)

	if !program.IsConnected() {
		t.Fatal("not connected while the connection is")
	}
}
