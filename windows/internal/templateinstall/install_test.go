package templateinstall

import (
	"os"
	"path/filepath"
	"testing"
	"windows/internal/store"
)

func TestInstallValidTemplateWithoutHooks(t *testing.T) {
	source := os.Getenv("PLANET_TEST_TEMPLATE_ROOT")
	if source == "" {
		t.Skip("original Plain template unavailable")
	}
	root := t.TempDir()
	library, err := store.Open(filepath.Join(root, "app"))
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Example", "")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := Install(filepath.Join(root, "app"), source, site, nil, library)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(installed, "hooks")); !os.IsNotExist(err) {
		t.Fatal("hook should not be copied")
	}
	if err := os.WriteFile(filepath.Join(installed, "assets", "style.css"), []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(installed); err == nil {
		t.Fatal("modified template asset accepted")
	}
	sites, err := library.ListSites()
	if err != nil || sites[0].TemplateName != "Plain" || sites[0].TemplateID == "" {
		t.Fatalf("sites=%#v err=%v", sites, err)
	}
}

func TestInvalidTemplateDoesNotChangeSite(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Example", "")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "template.json"), []byte(`{"name":"Bad"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "templates", "index.html"), []byte(`{{ value|unknown_filter }}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "templates", "blog.html"), []byte(`{{ content_html }}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(root, source, site, nil, library); err == nil {
		t.Fatal("invalid template accepted")
	}
	sites, err := library.ListSites()
	if err != nil || sites[0].TemplateID != "" {
		t.Fatalf("site changed: %#v %v", sites, err)
	}
}

func TestUnverifiedTemplateIsRejected(t *testing.T) {
	root := t.TempDir()
	library, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := library.CreateSite("Example", "")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"template.json": `{"name":"Unknown"}`, "templates/index.html": `{{ planet.name }}`, "templates/blog.html": `{{ content_html }}`} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(root, source, site, nil, library); err == nil {
		t.Fatal("unverified template was accepted")
	}
}

func TestInstallOriginalTextTemplates(t *testing.T) {
	root := os.Getenv("PLANET_TEST_TEMPLATES_DIR")
	if root == "" {
		t.Skip("original templates unavailable")
	}
	for _, item := range []struct{ dir, name string }{{"gamedb", "8-bit"}, {"grid", "Grid"}} {
		t.Run(item.name, func(t *testing.T) {
			app := t.TempDir()
			library, err := store.Open(app)
			if err != nil {
				t.Fatal(err)
			}
			site, err := library.CreateSite("Test", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Install(app, filepath.Join(root, item.dir), site, nil, library); err != nil {
				t.Fatal(err)
			}
			sites, err := library.ListSites()
			if err != nil || sites[0].TemplateName != item.name {
				t.Fatalf("sites=%#v err=%v", sites, err)
			}
		})
	}
}
