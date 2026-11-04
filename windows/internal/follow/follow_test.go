package follow

import (
	"strings"
	"testing"
)

type fakeNode struct {
	resolved string
	data     []byte
	pinned   string
}

func (n *fakeNode) Resolve(name string) (string, error)   { n.resolved = name; return "bafytest", nil }
func (n *fakeNode) Read(cid, path string) ([]byte, error) { return n.data, nil }
func (n *fakeNode) Pin(cid string) error                  { n.pinned = cid; return nil }
func TestFollowIPNSReadsPlanetAndPinsContent(t *testing.T) {
	node := &fakeNode{data: []byte(`{"id":"11111111-1111-4111-8111-111111111111","name":"Test Planet","about":"About","articles":[{"id":"22222222-2222-4222-8222-222222222222","link":"/22222222-2222-4222-8222-222222222222/","title":"Post","content":"**Hello**","created":1}]}`)}
	address := "planet://k51abcdefghijk"
	result, err := Fetch(node, address, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.resolved != "k51abcdefghijk" || node.pinned != "bafytest" || result.CID != "bafytest" || len(result.Feed.Items) != 1 || !strings.Contains(result.Feed.Items[0].Content, "<strong>Hello</strong>") {
		t.Fatalf("result=%+v node=%+v", result, node)
	}
	if result.Feed.Items[0].Link != "https://ipfs.io/ipns/k51abcdefghijk/22222222-2222-4222-8222-222222222222/" {
		t.Fatalf("link=%q", result.Feed.Items[0].Link)
	}
}
func TestFollowENSAndRejectUnsafePlanetPaths(t *testing.T) {
	n := &fakeNode{data: []byte(`{"name":"X","articles":[{"id":"post","link":"/../planet.key","title":"X","content":"hello"}]}`)}
	if _, err := Fetch(n, "alice.eth", false); err == nil {
		t.Fatal("unsafe article path accepted")
	}
	if n.resolved != "alice.eth" {
		t.Fatalf("ENS not passed to resolver: %s", n.resolved)
	}
}

func TestPlanetItemsRetainCIDAndRegisteredAttachments(t *testing.T) {
	n := &fakeNode{data: []byte(`{"name":"Site","articles":[{"id":"POST-1","link":"/POST-1/","title":"Post","content":"![image](photo.png)","attachments":["photo.png"]}]}`)}
	r, err := Fetch(n, "k51abcdefghijk", false)
	if err != nil {
		t.Fatal(err)
	}
	item := r.Feed.Items[0]
	if item.ContentCID != "bafytest" || item.ArticleID != "POST-1" || len(item.Attachments) != 1 || item.Attachments[0] != "photo.png" {
		t.Fatalf("item=%+v", item)
	}
}

func TestPlanetRecognizesRegisteredAudio(t *testing.T) {
	n := &fakeNode{data: []byte(`{"name":"Site","articles":[{"id":"POST-1","link":"/POST-1/","title":"Audio","audioFilename":"sound.mp3","attachments":["sound.mp3"]}]}`)}
	r, err := Fetch(n, "k51abcdefghijk", false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Feed.Items[0].AudioFilename != "sound.mp3" {
		t.Fatalf("item=%+v", r.Feed.Items[0])
	}
	n.data = []byte(`{"name":"Site","articles":[{"id":"POST-1","link":"/POST-1/","audioFilename":"../secret","attachments":[]}]}`)
	if _, err = Fetch(n, "k51abcdefghijk", false); err == nil {
		t.Fatal("unregistered audio accepted")
	}
}

func TestFollowUnicodeAndSubdirectory(t *testing.T) {
	name, err := Normalize(" ÉXAMPLE.eth ")
	if err != nil || name != "éxample.eth" {
		t.Fatalf("normalized=%q err=%v", name, err)
	}
	if _, err = Normalize("a_b.eth"); err == nil {
		t.Fatal("invalid ENS name accepted")
	}
	node := &pathNode{data: []byte(`{"name":"Nested","articles":[{"id":"POST","link":"/POST/","title":"Post","content":"Body","attachments":["photo.png"]}]}`)}
	result, err := Fetch(node, name, true)
	if err != nil {
		t.Fatal(err)
	}
	if node.readCID != "bafytest" || node.readPath != "folder/site/planet.json" || node.pinned != "bafytest" || result.CID != "bafytest" || result.Feed.Items[0].ContentPath != "folder/site" {
		t.Fatalf("result=%+v node=%+v", result, node)
	}
}

type pathNode struct {
	data                      []byte
	readCID, readPath, pinned string
}

func (n *pathNode) Resolve(string) (string, error) { return "bafytest/folder/site", nil }
func (n *pathNode) Read(cid, path string) ([]byte, error) {
	n.readCID = cid
	n.readPath = path
	return n.data, nil
}
func (n *pathNode) Pin(cid string) error { n.pinned = cid; return nil }
