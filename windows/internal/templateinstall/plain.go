package templateinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Fingerprints of the template files in Planet's pinned Plain template
// b29140fd906877206a76edb5e9510f816d16e336, with CRLF normalized to LF.
var plainFiles = map[string]string{
	"archive.html":              "aec8d11dfc682bd5f73c9c6f543407c5fd7fdefc1efabd9210d85cd1d29ebbf8",
	"base.html":                 "689e5ada98d874ff1e828fb7750e47574a791299817cf280829310bdb8c1dc0b",
	"blog.html":                 "764a39bc413f79182889fd8617b13fb890a6e6203f43471cab7ec4fc96e126d5",
	"index.html":                "870054216fa8eeff4785965aba38330cb6d4d78aade138e0fe9531c4c1d494f5",
	"tags.html":                 "b33482d7b9f9ea027e19dab727bb916c60637fd83112e84d7f4af7cdc5968162",
	"modules/article_seo.html":  "9c23b07424f4655acaa30b1331270c9b56f3a16734e8cbb50d2c85df372eadab",
	"modules/article_tags.html": "da098ef30fdc770f19d04d02368166ca057fdea1e4744d4a39708f030f82a95d",
	"modules/nav.html":          "a56a89369701f6488c89916146f1d3aadb4069288a68ae2540d5674e9f9201df",
	"modules/pages.html":        "d465c3b74b79a599045480a8edcc65ab1b7a62c23a73fa74c6c7da8c9f68d34e",
	"modules/planet_seo.html":   "62b23916ccd1f8ee3f97a3e7f7a31cdf6ead3459787304c6ec5e3fceefd8b3e8",
	"modules/top.html":          "1ce4d5aa6e37c7952b587a44255f77b7de1e0ea9da71d193e853368907c2fbcf",
	"modules/video.js":          "b2be0657d532c73180e6dd1c2d512d4f9ff269ed63fbc7116e0b012efbfbb195",
}

func VerifyName(root, name string) error {
	if err := Verify(root); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(root, "template.json"))
	if err != nil {
		return err
	}
	var metadata struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &metadata); err != nil || metadata.Name != name {
		return errors.New("模板名称与原站点不匹配")
	}
	return nil
}

// The 8-bit and Grid files are pinned to the revisions in Planet's Package.resolved.
var eightBitFiles = map[string]string{
	"archive.html":             "a45dc20daa80517669e1810333dac2a64bb2f560f51469f01a96876c9e9c94f5",
	"base.html":                "56c29972966864f8a36a2788f717c6d700a9e2a1467cefeff8ba2442a2a5b441",
	"blog.html":                "0476edfe89c992ac9cc121b0f718fc0d62195d96a223b052ba045b9ed4f4834d",
	"index.html":               "2112191d1e94751b8ebed74db7ba95bd6e73fd438831d75dc5c18ee714d4401a",
	"modules/article_seo.html": "06aebd11257a76676da7aa79552b0177586bbee2049d3923aaceee9c8cc65794",
	"modules/nav.html":         "847320bfa086918a96104a65b0bef3f7269b346b8ada9b05f477124b9ce98df5",
	"modules/planet_seo.html":  "f909dc6cc879f44b1d4dd355b16dcd11c63ea9b6b7417b804e8bd0aa630cbd11",
	"modules/site_header.html": "76e81fd616881165c2d947d3621417f49fcfb4f411f662f5a74fac66e6d36c83",
	"tags.html":                "3041d3660b931ca2bf1ce9fc2bf42b16a2e67f71be6a3016cf732079373208f3",
}
var gridFiles = map[string]string{
	"archive.html":              "8eb532b3b095174c858449eaf4ac1d09d414c8a2673c89c164d1b2cf5f3d969f",
	"base.html":                 "2c4b26a52af54c6940affc4c35e5b5ab466c211db30513da7224fdc8c97cf908",
	"blog.html":                 "a03dc99b7e5d9c30d1b8a01a1dfb75e3da9b453949477cc5503a994b88ecd8cb",
	"index.html":                "6c179b019800250a6fab46f1a7e461630169e5a2e727f1f8a3b0ee61ddca668e",
	"modules/article_tags.html": "da098ef30fdc770f19d04d02368166ca057fdea1e4744d4a39708f030f82a95d",
	"modules/nav.html":          "0538e057a795611c3458c9092c20b0d792deb7e757b69e359e76450015cf0869",
	"modules/planet_seo.html":   "458d29ec153edb7524c3793e87413beac9e4934e01254b3379bdd84e807aac4e",
	"modules/post_seo.html":     "06539ff28e380818b9e08340448e06f5d9e14b13eca2e5d24bac8cec83a19da5",
	"modules/top.html":          "e20598c6ebbcd7db65f12506c802620d60a977d5e0918ecfb17b86aaf31ae08f",
	"tags.html":                 "819aaef1d399f04e526997953b1223fa845ba6cb5d3f7b91e11933741108efa8",
}

// Platinum build 20 from the pinned PlanetSiteTemplates checkout.
var platinumFiles = map[string]string{
	"archive.html":               "23f7d884cb5f7d6e17a62fe77b7765c0a30d8c385e092f27af8760b4caec19d3",
	"base.html":                  "dd337da57933773d828b053f1c4ccc2c783c33402a3077c87560d95eb38cde1d",
	"blog.html":                  "693f2ebca5405891cf95dd764464548aff000606b1a27ab9cf67a1ca965a8a40",
	"index.html":                 "bf74c028d1c6780c12ef9b6c6c4a094cb2820a89f96936f70dbbff2c753cfdff",
	"tags.html":                  "484d9048106f7bde721132a615b5cfc37ed720f209c5b505735b6c6624d99571",
	"modules/alternates.html":    "49e50aa7698e111f08d27900ca157ae7b1a09a284c356086a099e46c1434a847",
	"modules/article_seo.html":   "9c23b07424f4655acaa30b1331270c9b56f3a16734e8cbb50d2c85df372eadab",
	"modules/buttons.html":       "6d9f425f8c8a055812c480cfa1a43c246cba3241770969a38d756441024120c4",
	"modules/glyph_archive.html": "94fbbeae550d46e6afb8a2128fce73eb331b919d21587d8c4538a42caae5f00c",
	"modules/glyph_badge.html":   "a2f4d47dc16eb037e63f0af79476fc156f333dd41f36a33472d5eb733a42e8d6",
	"modules/glyph_cross.html":   "5bcbf48b4fb0ba153573bda2416bde2eadeeca9310a0f2311018acaabc449de5",
	"modules/glyph_rss.html":     "2b612ebfc5f8bd4998adb5d87092a266e52bc482cba6e4baa25e5590f489155d",
	"modules/glyph_tags.html":    "6769e7c49c1b710a8fbf9a87fbf39360e061489cd5dc19093c83667a6fe0e33e",
	"modules/lang.html":          "38d1cf6473b1dda87bee5644047885b4e73c3a11ffda5afc9ace5ae0fbb0d48a",
	"modules/langs.html":         "2843bb62a58eae205f0dcac2b517e40abffcc169d758d65769ed1fcd6eb65fa0",
	"modules/planet_seo.html":    "62b23916ccd1f8ee3f97a3e7f7a31cdf6ead3459787304c6ec5e3fceefd8b3e8",
	"modules/replies.html":       "2771969fdf5f667f0214b0f632aa1249eda99c072ea1c1e4fd2b4dcff113f4a4",
	"modules/replies_head.html":  "8b0f823ec6d8d60a7a4eb88f1e21780c2e04daccb7db392af321dcd12815e7ef",
	"modules/sep.html":           "b66277e1d2f5cc12ce489392ddcf660c15956b5fb150def26c0c4a8f8c7933ab",
	"modules/todo.html":          "b26e887a02ea5205c6de36409b7de3c131855058f6d20cfbb9c572cea75a593a",
	"modules/type.html":          "4ca4ed820d5aa92964d3587304fb0ec9f14fd177cc041886adbcb35c3f475add",
}

// Sepia build 90 from the pinned PlanetSiteTemplates checkout.
var sepiaFiles = map[string]string{
	"archive.html":              "223efbc52a895ee9c0636dbac79b11869842429dedd20170a96eb327997c64c2",
	"base.html":                 "54ba1abfa1847f8bc9314ee5facb8ec645583fa715e0f3c52c1b6f2763e74bb9",
	"blog.html":                 "7fe72f33df1183e429ee04cabad6108bd6617e328af4e61ea597a9395f4a605e",
	"index.html":                "96935f516570cdeef2493a38edde45c25e9beef1279ef7ea2659dfe469427245",
	"modules/donate_modal.html": "6f07ff5e9a4d373618d3312615ef46780e00287794fd495f7d7b004f456ff86b",
	"modules/item.html":         "27cf7455e943e8972bf4e4ea6a8d78fbc3fdbb87f551a95c5a2392da2d628dc9",
	"modules/planet_seo.html":   "86d73bd6dd2ce49da5df466a0f9c2279324ec0fb5ce00c0e224f93bf1e9bd316",
	"modules/post_seo.html":     "e52e95b34333650ccf00f4b10977c9d3faee32255f5368af52bd55fd067c3b9c",
	"modules/site_header.html":  "b08a0b739a7caeaeba79f34990d6667f0c3fbee000882d03058103f64410f817",
	"modules/video.js":          "83f8db44e2847dd83c214319e5e115f8a6adea673c84d0a57da4f7b754189673",
	"tags.html":                 "fa83c9d89bc44ca6e0e60e81eeeafe43241e55dc7f96dc445cbc627a6cd08e24",
}

func verifyFiles(root string, files map[string]string) error {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		expected, ok := files[filepath.ToSlash(rel)]
		if !ok {
			return errors.New("模板包含未验证的文件")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("模板包含不支持的文件")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
		if hex.EncodeToString(sum[:]) != expected {
			return errors.New("模板文件与已验证版本不同")
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(files) {
		return errors.New("模板缺少已验证的文件")
	}
	return nil
}

// Verify rejects changes or unsupported versions before every render.
func Verify(root string) error {
	path := filepath.Join(root, "template.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return errors.New("模板信息无效")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")))
	var files map[string]string
	var assetsHash string
	switch hex.EncodeToString(sum[:]) {
	case "b103b8288e67811257d00dfccafeb0a0ee5d463345244e5a373191a8cfc68ca5":
		files = plainFiles
		assetsHash = "f9ba47c3bdbb7f119cb74d6ccc914bacf515d7f283c6c15821b26cd0b03d412b"
	case "053e214456c43dd5ed5aaa54ab2e9cd08bca722c0073d5fa0d16a7f541296e71":
		files = eightBitFiles
		assetsHash = "02ad2edb71ffd95172cdbf6620b403910e2bcdf23b2b27e6affa54331a7c7fc7"
	case "37f7b376ef903d66599e9bc8702baaa53d48e8a6e5b75932e324b4ae56221985":
		files = gridFiles
		assetsHash = "2a252f1b8643c076b43c38b108bdae2ed95bb4d5ef19719b7fdf4bfda40c34f5"
	case "9e2e3c5221eb22b0af3a8ec32da25c9e74f5a904883b0732e80b3c0a8ff53bbe":
		files = platinumFiles
		// Normalize the pinned RSS stylesheet just like the other text assets.
		assetsHash = "f6a58e4b6de14450e0e811c4f43712579d817b00bef5cb4cf4be41cbf8c04770"
	case "c86d5d5047c56326478143d5ef4ce6bd9c6de79b9643b72cb394ea4accb75454":
		files = sepiaFiles
		assetsHash = "146e7dffea1e9fcca7b3470cbd5d1d9bc710520a4d8de16bb7cccb6544540bd1"
	default:
		return errors.New("模板信息与已验证版本不同")
	}
	if err := verifyFiles(filepath.Join(root, "templates"), files); err != nil {
		return err
	}
	return verifyAssets(filepath.Join(root, "assets"), assetsHash)
}

func verifyAssets(root, expected string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("模板资源目录无效")
	}
	hash := sha256.New()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			return errors.New("模板资源文件无效")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".css" || ext == ".js" || ext == ".svg" || ext == ".html" || ext == ".json" || ext == ".xsl" {
			data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		}
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("模板资源缺失或与已验证版本不同")
	}
	return nil
}
