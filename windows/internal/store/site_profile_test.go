package store

import "testing"

func TestSiteProfileSavesDomainAndSocialWithoutChangingIdentity(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Site", "")
	if err != nil {
		t.Fatal(err)
	}
	p := SiteProfile{Domain: "Example.ETH", TwitterUsername: "@alice", GithubUsername: "alice-smith", TelegramUsername: "alice", MastodonUsername: "@alice@social.example", DiscordLink: "https://discord.gg/xyz"}
	updated, err := s.UpdateSiteProfile(site.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Domain != "example.eth" || updated.TwitterUsername != "alice" || updated.GithubUsername != "alice-smith" {
		t.Fatalf("profile not normalized: %+v", updated)
	}
	for _, domain := range []string{"https://example.com", "example.com/path", "localhost", "-bad.example", "127.0.0.1", "example.c"} {
		p.Domain = domain
		if _, err = s.UpdateSiteProfile(site.ID, p); err == nil {
			t.Errorf("invalid domain accepted: %q", domain)
		}
	}
	p.Domain = ""
	p.DiscordLink = "javascript:alert(1)"
	if _, err = s.UpdateSiteProfile(site.ID, p); err == nil {
		t.Fatal("unsafe social URL accepted")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	sites, err := reopened.ListSites()
	if err != nil || sites[0].Domain != "example.eth" {
		t.Fatalf("profile lost: %+v %v", sites, err)
	}
}

func TestCanonicalSiteBase(t *testing.T) {
	for _, tc := range []struct{ domain, ipns, want string }{
		{"example.eth", "k51test", "https://example.eth.limo/"},
		{"site.example.com", "k51test", "https://site.example.com/"},
		{"example.sol", "k51test", "https://example.sol.build/"},
		{"example.bit", "k51test", "https://example.bit.site/"},
		{"", "k51test", "https://ipfs.io/ipns/k51test/"},
	} {
		if got := CanonicalSiteBase(tc.domain, tc.ipns); got != tc.want {
			t.Errorf("%q: %q", tc.domain, got)
		}
	}
}
