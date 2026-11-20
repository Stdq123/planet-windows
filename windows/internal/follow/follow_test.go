package follow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"windows/internal/siteimage"
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
	if node.readCID != "bafytest" || len(node.readPaths) == 0 || node.readPaths[0] != "folder/site/planet.json" || node.pinned != "bafytest" || result.CID != "bafytest" || result.Feed.Items[0].ContentPath != "folder/site" {
		t.Fatalf("result=%+v node=%+v", result, node)
	}
}

type pathNode struct {
	data            []byte
	files           map[string][]byte
	readCID, pinned string
	readPaths       []string
}

func (n *pathNode) Resolve(string) (string, error) { return "bafytest/folder/site", nil }
func (n *pathNode) Read(cid, path string) ([]byte, error) {
	n.readCID = cid
	n.readPaths = append(n.readPaths, path)
	if n.files != nil {
		if b, ok := n.files[path]; ok {
			return b, nil
		}
		return nil, errors.New("not found")
	}
	return n.data, nil
}
func (n *pathNode) Pin(cid string) error { n.pinned = cid; return nil }

func TestFollowReadsPlanetAvatarAndFaviconFallback(t *testing.T) {
	var pngBuf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 120, B: 220, A: 255})
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	meta := []byte(`{"name":"Planet","articles":[]}`)

	avatarNode := &pathNode{files: map[string][]byte{
		"folder/site/planet.json": meta,
		"folder/site/avatar.png":  pngBuf.Bytes(),
	}}
	res, err := Fetch(avatarNode, "alice.eth", false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := siteimage.DecodePNG(res.Feed.IconPNG)
	if err != nil || decoded.Bounds().Dx() != 32 {
		t.Fatalf("avatar icon failed: err=%v", err)
	}

	header := make([]byte, 22)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint16(header[4:6], 1)
	header[6], header[7] = 32, 32
	binary.LittleEndian.PutUint16(header[10:12], 1)
	binary.LittleEndian.PutUint16(header[12:14], 32)
	binary.LittleEndian.PutUint32(header[14:18], uint32(pngBuf.Len()))
	binary.LittleEndian.PutUint32(header[18:22], 22)
	ico := append(header, pngBuf.Bytes()...)

	faviconNode := &pathNode{files: map[string][]byte{
		"folder/site/planet.json": meta,
		"folder/site/favicon.ico": ico,
	}}
	res, err = Fetch(faviconNode, "alice.eth", false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err = siteimage.DecodePNG(res.Feed.IconPNG)
	if err != nil || decoded.Bounds().Dx() != 32 {
		t.Fatalf("favicon fallback failed: err=%v", err)
	}
}
