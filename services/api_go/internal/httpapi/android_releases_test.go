package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/releases"
)

func releaseTestAPK(t *testing.T, code int, name, packageName string) []byte {
	t.Helper()
	stringsPool := []string{"manifest", "package", packageName, "versionCode", "versionName", name, "http://schemas.android.com/apk/res/android"}
	var stringData []byte
	pool := make([]byte, 28+len(stringsPool)*4)
	binary.LittleEndian.PutUint16(pool, 1)
	binary.LittleEndian.PutUint16(pool[2:], 28)
	binary.LittleEndian.PutUint32(pool[8:], uint32(len(stringsPool)))
	binary.LittleEndian.PutUint32(pool[16:], 0x100)
	binary.LittleEndian.PutUint32(pool[20:], uint32(len(pool)))
	for i, value := range stringsPool {
		binary.LittleEndian.PutUint32(pool[28+i*4:], uint32(len(stringData)))
		stringData = append(stringData, byte(len(value)), byte(len(value)))
		stringData = append(stringData, value...)
		stringData = append(stringData, 0)
	}
	pool = append(pool, stringData...)
	for len(pool)%4 != 0 {
		pool = append(pool, 0)
	}
	binary.LittleEndian.PutUint32(pool[4:], uint32(len(pool)))
	tag := make([]byte, 36+3*20)
	binary.LittleEndian.PutUint16(tag, 0x0102)
	binary.LittleEndian.PutUint16(tag[2:], 16)
	binary.LittleEndian.PutUint32(tag[4:], uint32(len(tag)))
	binary.LittleEndian.PutUint32(tag[8:], 1)
	binary.LittleEndian.PutUint32(tag[12:], 0xffffffff)
	binary.LittleEndian.PutUint32(tag[16:], 0xffffffff)
	binary.LittleEndian.PutUint16(tag[24:], 20)
	binary.LittleEndian.PutUint16(tag[26:], 20)
	binary.LittleEndian.PutUint16(tag[28:], 3)
	attribute := func(index int, namespace, key uint32, kind byte, value uint32) {
		data := tag[36+index*20:]
		binary.LittleEndian.PutUint32(data, namespace)
		binary.LittleEndian.PutUint32(data[4:], key)
		binary.LittleEndian.PutUint32(data[8:], 0xffffffff)
		binary.LittleEndian.PutUint16(data[12:], 8)
		data[15] = kind
		binary.LittleEndian.PutUint32(data[16:], value)
	}
	attribute(0, 0xffffffff, 1, 3, 2)
	attribute(1, 6, 3, 0x10, uint32(code))
	attribute(2, 6, 4, 3, 5)
	manifest := make([]byte, 8)
	binary.LittleEndian.PutUint16(manifest, 3)
	binary.LittleEndian.PutUint16(manifest[2:], 8)
	manifest = append(manifest, pool...)
	manifest = append(manifest, tag...)
	binary.LittleEndian.PutUint32(manifest[4:], uint32(len(manifest)))
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	for _, entry := range []struct {
		name  string
		value []byte
	}{{"AndroidManifest.xml", manifest}, {"classes.dex", []byte("dex\n035\x00fixture")}} {
		file, err := archive.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(entry.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestAndroidReleaseHTTPFlow(t *testing.T) {
	ctx := context.Background()
	service, db, closeDB := testAuthService(t)
	t.Cleanup(closeDB)
	root := t.TempDir()
	store, err := releases.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	login := func(username string, role auth.Role) string {
		t.Helper()
		if _, err := service.CreateUser(ctx, username, username, "ReleasePass!", role); err != nil {
			t.Fatal(err)
		}
		pair, err := service.Login(ctx, auth.LoginInput{Username: username, Password: "ReleasePass!", Device: auth.Device{ID: "release-test", Name: "Test", Platform: "android", AppVersion: "1.0"}})
		if err != nil {
			t.Fatal(err)
		}
		return pair.AccessToken
	}
	adminToken := login("release-admin", auth.RoleAdmin)
	userToken := login("release-user", auth.RoleUser)
	handler := New(Dependencies{Auth: service, AndroidReleases: store})
	const adminPath = "/api/v1/admin/app/android"
	request := func(method, path string, body any, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := performJSON(handler, method, path, body, token)
		if response.Code != want {
			t.Fatalf("%s %s status=%d want=%d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response
	}
	for _, methodPath := range []struct{ method, path string }{{"GET", "/release"}, {"POST", "/apk"}, {"PUT", "/release"}, {"POST", "/publish"}, {"PUT", "/policy"}} {
		request(methodPath.method, adminPath+methodPath.path, nil, "", 401)
		request(methodPath.method, adminPath+methodPath.path, nil, userToken, 403)
	}
	empty := request("GET", "/api/v1/app/android/version", nil, "", 200)
	if !strings.Contains(empty.Body.String(), `"release":null`) || empty.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("empty public release or cache policy incorrect")
	}
	oversize := httptest.NewRequest("POST", adminPath+"/apk", nil)
	oversize.ContentLength = releases.MaxAPKSize + (1 << 20) + 1
	oversize.Header.Set("Authorization", "Bearer "+adminToken)
	tooLarge := httptest.NewRecorder()
	handler.ServeHTTP(tooLarge, oversize)
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("oversized upload was not rejected before reading")
	}
	upload := func(data []byte, want int) releases.State {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("apk", "app.apk")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(data)
		writer.Close()
		req := httptest.NewRequest("POST", adminPath+"/apk", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+adminToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("upload=%d want=%d: %s", response.Code, want, response.Body.String())
		}
		var state releases.State
		if want == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
		}
		return state
	}
	upload([]byte("not an APK"), 400)
	upload(releaseTestAPK(t, 2, "1.1", "com.example.other"), 400)
	apk := releaseTestAPK(t, 2, "1.1", releases.PackageName)
	state := upload(apk, 200)
	draft := *state.Draft
	if draft.VersionCode != 2 || draft.VersionName != "1.1" || draft.Size != int64(len(apk)) || draft.PackageName != releases.PackageName {
		t.Fatalf("identity=%+v", draft)
	}
	hash := sha256.Sum256(apk)
	if draft.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("upload hash mismatch")
	}
	request("GET", draft.DownloadURL, nil, "", 404)
	if public := request("GET", "/api/v1/app/android/version", nil, "", 200); strings.Contains(public.Body.String(), draft.ID) {
		t.Fatal("draft exposed before publication")
	}
	input := releases.DraftInput{DraftID: draft.ID, VersionCode: 2, VersionName: "1.1", Changelog: "修复与优化", ForceUpdate: true}
	bad := input
	bad.DraftID = "old-draft"
	request("PUT", adminPath+"/release", bad, adminToken, 409)
	bad = input
	bad.VersionCode = 3
	request("PUT", adminPath+"/release", bad, adminToken, 400)
	bad = input
	bad.Changelog = strings.Repeat("更", 10001)
	request("PUT", adminPath+"/release", bad, adminToken, 400)
	input.Changelog = strings.Repeat("修复与优化", 1600)
	request("PUT", adminPath+"/release", input, adminToken, 200)
	request("POST", adminPath+"/publish", map[string]any{"draftId": "old-draft"}, adminToken, 409)
	request("POST", adminPath+"/publish", map[string]any{"draftId": draft.ID}, adminToken, 200)
	public := request("GET", "/api/v1/app/android/version", nil, "", 200)
	if !strings.Contains(public.Body.String(), `"forceUpdate":true`) || strings.Contains(public.Body.String(), `"draft"`) {
		t.Fatal("public release wrong")
	}
	request("PUT", adminPath+"/policy", map[string]any{"releaseId": "old-release", "forceUpdate": false}, adminToken, 409)
	request("PUT", adminPath+"/policy", map[string]any{"releaseId": draft.ID}, adminToken, 400)
	request("PUT", adminPath+"/policy", map[string]any{"releaseId": draft.ID, "forceUpdate": false}, adminToken, 200)
	state, err = store.State(ctx)
	if err != nil || state.Release.ForceUpdate || !state.Draft.ForceUpdate {
		t.Fatalf("policy did not stay separate from draft: %+v %v", state, err)
	}
	download := request("GET", draft.DownloadURL, nil, "", 200)
	if !bytes.Equal(download.Body.Bytes(), apk) || download.Header().Get("ETag") != `"`+draft.SHA256+`"` {
		t.Fatal("download content/hash headers")
	}
	rangeReq := httptest.NewRequest("GET", draft.DownloadURL, nil)
	rangeReq.Header.Set("Range", "bytes=1-8")
	rangeResponse := httptest.NewRecorder()
	handler.ServeHTTP(rangeResponse, rangeReq)
	if rangeResponse.Code != 206 || !bytes.Equal(rangeResponse.Body.Bytes(), apk[1:9]) {
		t.Fatal("APK Range failed")
	}
	head := request("HEAD", draft.DownloadURL, nil, "", 200)
	if head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" {
		t.Fatal("APK HEAD failed")
	}
	etagReq := httptest.NewRequest("GET", draft.DownloadURL, nil)
	etagReq.Header.Set("If-None-Match", download.Header().Get("ETag"))
	etagResponse := httptest.NewRecorder()
	handler.ServeHTTP(etagResponse, etagReq)
	if etagResponse.Code != 304 {
		t.Fatal("APK ETag revalidation failed")
	}
	restarted, err := releases.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.State(ctx)
	if err != nil || loaded.Release.ID != draft.ID || loaded.Release.ForceUpdate || loaded.Draft.Changelog != input.Changelog {
		t.Fatal("release state lost after restart")
	}
	// Replacing unpublished drafts cleans their files; old published downloads remain valid.
	next := upload(releaseTestAPK(t, 3, "1.2", releases.PackageName), 200)
	firstDraftID := next.Draft.ID
	next = upload(releaseTestAPK(t, 4, "1.3", releases.PackageName), 200)
	if _, err := os.Stat(filepath.Join(root, "releases", "android", firstDraftID+".apk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("replaced draft file remains")
	}
	request("POST", adminPath+"/publish", map[string]any{"draftId": firstDraftID}, adminToken, 409)
	request("GET", draft.DownloadURL, nil, "", 200)
	request("POST", adminPath+"/publish", map[string]any{"draftId": next.Draft.ID}, adminToken, 200)
	old := upload(apk, 200)
	request("POST", adminPath+"/publish", map[string]any{"draftId": old.Draft.ID}, adminToken, 409)
	// A different APK with the current version code cannot replace an installed release.
	sameCode := upload(releaseTestAPK(t, 4, "changed", releases.PackageName), 200)
	request("POST", adminPath+"/publish", map[string]any{"draftId": sameCode.Draft.ID}, adminToken, 409)
	files, err := filepath.Glob(filepath.Join(root, "releases", "android", ".upload-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("temporary uploads leaked")
	}
}

type releaseErrorReader struct{ data []byte }

func (reader *releaseErrorReader) Read(data []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(data, reader.data)
	reader.data = reader.data[n:]
	return n, nil
}

func TestAndroidUploadCancellationAndMalformedBody(t *testing.T) {
	_, db, closeDB := testAuthService(t)
	t.Cleanup(closeDB)
	root := t.TempDir()
	store, err := releases.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Upload(ctx, bytes.NewReader([]byte("test"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled upload=%v", err)
	}
	if _, err := store.Upload(context.Background(), &releaseErrorReader{data: []byte("partial")}); err == nil {
		t.Fatal("accepted truncated source")
	}
	files, err := os.ReadDir(filepath.Join(root, "releases", "android"))
	if err != nil || len(files) != 0 {
		t.Fatal("failed upload files leaked")
	}
}

type releaseBlockedReader struct {
	started chan struct{}
	release chan struct{}
}

func (reader *releaseBlockedReader) Read(data []byte) (int, error) {
	close(reader.started)
	<-reader.release
	return 0, io.EOF
}

func TestAndroidUploadsDoNotOverlap(t *testing.T) {
	_, db, closeDB := testAuthService(t)
	t.Cleanup(closeDB)
	root := t.TempDir()
	store, err := releases.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	reader := &releaseBlockedReader{started: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := store.Upload(context.Background(), reader)
		result <- err
	}()
	<-reader.started
	_, secondErr := store.Upload(context.Background(), bytes.NewReader(nil))
	close(reader.release)
	firstErr := <-result
	if !errors.Is(secondErr, releases.ErrBusy) || !errors.Is(firstErr, releases.ErrInvalidAPK) {
		t.Fatalf("overlapping uploads: first=%v second=%v", firstErr, secondErr)
	}
	entries, err := os.ReadDir(filepath.Join(root, "releases", "android"))
	if err != nil || len(entries) != 0 {
		t.Fatal("concurrent upload left temporary files")
	}
}

type releaseDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (writer *releaseDeadlineWriter) SetReadDeadline(deadline time.Time) error {
	writer.deadline = deadline
	return nil
}

func TestStatusRecorderPreservesUploadReadDeadline(t *testing.T) {
	underlying := &releaseDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	wrapped := &statusRecorder{ResponseWriter: underlying, status: 200}
	deadline := time.Now().Add(time.Minute)
	if err := http.NewResponseController(wrapped).SetReadDeadline(deadline); err != nil || !underlying.deadline.Equal(deadline) {
		t.Fatalf("read deadline not propagated: %v", err)
	}
}
