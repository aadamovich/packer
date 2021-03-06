package common

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/packer/packer"
)

func TestDownloadClientVerifyChecksum(t *testing.T) {
	tf, err := ioutil.TempFile("", "packer")
	if err != nil {
		t.Fatalf("tempfile error: %s", err)
	}
	defer os.Remove(tf.Name())

	// "foo"
	checksum, err := hex.DecodeString("acbd18db4cc2f85cedef654fccc4a4d8")
	if err != nil {
		t.Fatalf("decode err: %s", err)
	}

	// Write the file
	tf.Write([]byte("foo"))
	tf.Close()

	config := &DownloadConfig{
		Hash:     md5.New(),
		Checksum: checksum,
	}

	d := NewDownloadClient(config, new(packer.NoopUi))
	result, err := d.VerifyChecksum(tf.Name())
	if err != nil {
		t.Fatalf("Verify err: %s", err)
	}

	if !result {
		t.Fatal("didn't verify")
	}
}

func TestDownloadClient_basic(t *testing.T) {
	tf, _ := ioutil.TempFile("", "packer")
	tf.Close()
	defer os.Remove(tf.Name())

	ts := httptest.NewServer(http.FileServer(http.Dir("./test-fixtures/root")))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL + "/basic.txt",
		TargetPath: tf.Name(),
		CopyFile:   true,
	}, new(packer.NoopUi))

	path, err := client.Get()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if string(raw) != "hello\n" {
		t.Fatalf("bad: %s", string(raw))
	}
}

func TestDownloadClient_checksumBad(t *testing.T) {
	checksum, err := hex.DecodeString("b2946ac92492d2347c6235b4d2611184")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	tf, _ := ioutil.TempFile("", "packer")
	tf.Close()
	defer os.Remove(tf.Name())

	ts := httptest.NewServer(http.FileServer(http.Dir("./test-fixtures/root")))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL + "/basic.txt",
		TargetPath: tf.Name(),
		Hash:       HashForType("md5"),
		Checksum:   checksum,
		CopyFile:   true,
	}, new(packer.NoopUi))

	if _, err := client.Get(); err == nil {
		t.Fatal("should error")
	}
}

func TestDownloadClient_checksumGood(t *testing.T) {
	checksum, err := hex.DecodeString("b1946ac92492d2347c6235b4d2611184")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	tf, _ := ioutil.TempFile("", "packer")
	tf.Close()
	defer os.Remove(tf.Name())

	ts := httptest.NewServer(http.FileServer(http.Dir("./test-fixtures/root")))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL + "/basic.txt",
		TargetPath: tf.Name(),
		Hash:       HashForType("md5"),
		Checksum:   checksum,
		CopyFile:   true,
	}, new(packer.NoopUi))

	path, err := client.Get()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if string(raw) != "hello\n" {
		t.Fatalf("bad: %s", string(raw))
	}
}

func TestDownloadClient_checksumNoDownload(t *testing.T) {
	checksum, err := hex.DecodeString("3740570a423feec44c2a759225a9fcf9")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	ts := httptest.NewServer(http.FileServer(http.Dir("./test-fixtures/root")))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL + "/basic.txt",
		TargetPath: "./test-fixtures/root/another.txt",
		Hash:       HashForType("md5"),
		Checksum:   checksum,
		CopyFile:   true,
	}, new(packer.NoopUi))
	path, err := client.Get()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// If this says "hello" it means we downloaded it. We faked out
	// the downloader above by giving it the checksum for "another", but
	// requested the download of "hello"
	if string(raw) != "another\n" {
		t.Fatalf("bad: %s", string(raw))
	}
}

func TestDownloadClient_notFound(t *testing.T) {
	tf, _ := ioutil.TempFile("", "packer")
	tf.Close()
	defer os.Remove(tf.Name())

	ts := httptest.NewServer(http.FileServer(http.Dir("./test-fixtures/root")))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL + "/not-found.txt",
		TargetPath: tf.Name(),
	}, new(packer.NoopUi))

	if _, err := client.Get(); err == nil {
		t.Fatal("should error")
	}
}

func TestDownloadClient_resume(t *testing.T) {
	tf, _ := ioutil.TempFile("", "packer")
	tf.Write([]byte("w"))
	tf.Close()
	defer os.Remove(tf.Name())

	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			rw.Header().Set("Accept-Ranges", "bytes")
			rw.WriteHeader(204)
			return
		}

		http.ServeFile(rw, r, "./test-fixtures/root/basic.txt")
	}))
	defer ts.Close()

	client := NewDownloadClient(&DownloadConfig{
		Url:        ts.URL,
		TargetPath: tf.Name(),
		CopyFile:   true,
	}, new(packer.NoopUi))

	path, err := client.Get()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if string(raw) != "wello\n" {
		t.Fatalf("bad: %s", string(raw))
	}
}

func TestDownloadClient_usesDefaultUserAgent(t *testing.T) {
	tf, err := ioutil.TempFile("", "packer")
	if err != nil {
		t.Fatalf("tempfile error: %s", err)
	}
	tf.Close()
	defer os.Remove(tf.Name())

	defaultUserAgent := ""
	asserted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if defaultUserAgent == "" {
			defaultUserAgent = r.UserAgent()
		} else {
			incomingUserAgent := r.UserAgent()
			if incomingUserAgent != defaultUserAgent {
				t.Fatalf("Expected user agent %s, got: %s", defaultUserAgent, incomingUserAgent)
			}

			asserted = true
		}
	}))

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	_, err = httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	config := &DownloadConfig{
		Url:        server.URL,
		TargetPath: tf.Name(),
		CopyFile:   true,
	}

	client := NewDownloadClient(config, new(packer.NoopUi))
	_, err = client.Get()
	if err != nil {
		t.Fatal(err)
	}

	if !asserted {
		t.Fatal("User-Agent never observed")
	}
}

func TestDownloadClient_setsUserAgent(t *testing.T) {
	tf, err := ioutil.TempFile("", "packer")
	if err != nil {
		t.Fatalf("tempfile error: %s", err)
	}
	tf.Close()
	defer os.Remove(tf.Name())

	asserted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asserted = true
		if r.UserAgent() != "fancy user agent" {
			t.Fatalf("Expected useragent fancy user agent, got: %s", r.UserAgent())
		}
	}))
	config := &DownloadConfig{
		Url:        server.URL,
		TargetPath: tf.Name(),
		UserAgent:  "fancy user agent",
		CopyFile:   true,
	}

	client := NewDownloadClient(config, new(packer.NoopUi))
	_, err = client.Get()
	if err != nil {
		t.Fatal(err)
	}

	if !asserted {
		t.Fatal("HTTP request never made")
	}
}

func TestHashForType(t *testing.T) {
	if h := HashForType("md5"); h == nil {
		t.Fatalf("md5 hash is nil")
	} else {
		h.Write([]byte("foo"))
		result := h.Sum(nil)

		expected := "acbd18db4cc2f85cedef654fccc4a4d8"
		actual := hex.EncodeToString(result)
		if actual != expected {
			t.Fatalf("bad hash: %s", actual)
		}
	}

	if h := HashForType("sha1"); h == nil {
		t.Fatalf("sha1 hash is nil")
	} else {
		h.Write([]byte("foo"))
		result := h.Sum(nil)

		expected := "0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"
		actual := hex.EncodeToString(result)
		if actual != expected {
			t.Fatalf("bad hash: %s", actual)
		}
	}

	if h := HashForType("sha256"); h == nil {
		t.Fatalf("sha256 hash is nil")
	} else {
		h.Write([]byte("foo"))
		result := h.Sum(nil)

		expected := "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae"
		actual := hex.EncodeToString(result)
		if actual != expected {
			t.Fatalf("bad hash: %s", actual)
		}
	}

	if h := HashForType("sha512"); h == nil {
		t.Fatalf("sha512 hash is nil")
	} else {
		h.Write([]byte("foo"))
		result := h.Sum(nil)

		expected := "f7fbba6e0636f890e56fbbf3283e524c6fa3204ae298382d624741d0dc6638326e282c41be5e4254d8820772c5518a2c5a8c0c7f7eda19594a7eb539453e1ed7"
		actual := hex.EncodeToString(result)
		if actual != expected {
			t.Fatalf("bad hash: %s", actual)
		}
	}

	if HashForType("fake") != nil {
		t.Fatalf("fake hash is not nil")
	}
}

// TestDownloadFileUrl tests a special case where we use a local file for
// iso_url. In this case we can still verify the checksum but we should not
// delete the file if the checksum fails. Instead we'll just error and let the
// user fix the checksum.
func TestDownloadFileUrl(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Unable to detect working directory: %s", err)
	}
	cwd = filepath.ToSlash(cwd)

	// source_path is a file path and source is a network path
	sourcePath := fmt.Sprintf("%s/test-fixtures/fileurl/%s", cwd, "cake")

	filePrefix := "file://"
	if runtime.GOOS == "windows" {
		filePrefix += "/"
	}

	source := fmt.Sprintf(filePrefix + sourcePath)
	t.Logf("Trying to download %s", source)

	config := &DownloadConfig{
		Url: source,
		// This should be wrong. We want to make sure we don't delete
		Checksum: []byte("nope"),
		Hash:     HashForType("sha256"),
		CopyFile: false,
	}

	client := NewDownloadClient(config, new(packer.NoopUi))

	// Verify that we fail to match the checksum
	_, err = client.Get()
	if err.Error() != "checksums didn't match expected: 6e6f7065" {
		t.Fatalf("Unexpected failure; expected checksum not to match. Error was \"%v\"", err)
	}

	if _, err = os.Stat(sourcePath); err != nil {
		t.Errorf("Could not stat source file: %s", sourcePath)
	}
}

// SimulateFileUriDownload is a simple utility function that converts a uri
// into a testable file path whilst ignoring a correct checksum match, stripping
// UNC path info, and then calling stat to ensure the correct file exists.
//    (used by TestFileUriTransforms)
func SimulateFileUriDownload(t *testing.T, uri string) (string, error) {
	// source_path is a file path and source is a network path
	source := fmt.Sprintf(uri)
	t.Logf("Trying to download %s", source)

	config := &DownloadConfig{
		Url: source,
		// This should be wrong. We want to make sure we don't delete
		Checksum: []byte("nope"),
		Hash:     HashForType("sha256"),
		CopyFile: false,
	}

	// go go go
	client := NewDownloadClient(config, new(packer.NoopUi))
	path, err := client.Get()

	// ignore any non-important checksum errors if it's not a unc path
	if !strings.HasPrefix(path, "\\\\") && err.Error() != "checksums didn't match expected: 6e6f7065" {
		t.Fatalf("Unexpected failure; expected checksum not to match")
	}

	// if it's a unc path, then remove the host and share name so we don't have
	// to force the user to enable ADMIN$ and Windows File Sharing
	if strings.HasPrefix(path, "\\\\") {
		res := strings.SplitN(path, "/", 3)
		path = "/" + res[2]
	}

	if _, err = os.Stat(path); err != nil {
		t.Errorf("Could not stat source file: %s", path)
	}
	return path, err
}

// TestFileUriTransforms tests the case where we use a local file uri
// for iso_url. There's a few different formats that a file uri can exist as
// and so we try to test the most useful and common ones.
func TestFileUriTransforms(t *testing.T) {
	const testpath = /* have your */ "test-fixtures/fileurl/cake" /* and eat it too */
	const host = "localhost"

	var cwd string
	var volume string
	var share string

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Unable to detect working directory: %s", err)
		return
	}
	cwd = filepath.ToSlash(cwd)
	volume = filepath.VolumeName(cwd)
	share = volume

	// if a volume was found (on windows), replace the ':' from
	// C: to C$ to convert it into a hidden windows share.
	if len(share) > 1 && share[len(share)-1] == ':' {
		share = share[:len(share)-1] + "$"
	}
	cwd = cwd[len(volume):]

	t.Logf("TestFileUriTransforms : Running with cwd : '%s'", cwd)
	t.Logf("TestFileUriTransforms : Running with volume : '%s'", volume)

	// ./relative/path -> ./relative/path
	// /absolute/path -> /absolute/path
	// c:/windows/absolute -> c:/windows/absolute
	testcases := []string{
		"./%s",
		cwd + "/%s",
		volume + cwd + "/%s",
	}

	// all regular slashed testcases
	for _, testcase := range testcases {
		uri := "file://" + fmt.Sprintf(testcase, testpath)
		t.Logf("TestFileUriTransforms : Trying Uri '%s'", uri)
		res, err := SimulateFileUriDownload(t, uri)
		if err != nil {
			t.Errorf("Unable to transform uri '%s' into a path : %v", uri, err)
		}
		t.Logf("TestFileUriTransforms : Result Path '%s'", res)
	}

	// smb protocol depends on platform support which currently
	// only exists on windows.
	if runtime.GOOS == "windows" {
		// ...and finally the oddball windows native path
		// smb://host/sharename/file -> \\host\sharename\file
		testcase := host + "/" + share + "/" + cwd[1:] + "/%s"
		uri := "smb://" + fmt.Sprintf(testcase, testpath)
		t.Logf("TestFileUriTransforms : Trying Uri '%s'", uri)
		res, err := SimulateFileUriDownload(t, uri)
		if err != nil {
			t.Errorf("Unable to transform uri '%s' into a path", uri)
			return
		}
		t.Logf("TestFileUriTransforms : Result Path '%s'", res)
	}
}
