package store

import "testing"

func TestUpdateSiteWeb3ValidatesAndPersists(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site, err := s.CreateSite("Web3 Blog", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSiteWeb3(site.ID, SiteWeb3Settings{AcceptsDonation: true, AcceptsDonationETHAddress: "not-an-eth-address"}); err == nil {
		t.Fatal("expected invalid ETH address to be rejected")
	}
	updated, err := s.UpdateSiteWeb3(site.ID, SiteWeb3Settings{
		AcceptsDonation:           true,
		AcceptsDonationMessage:    "Support us",
		AcceptsDonationETHAddress: "vitalik.eth",
		JuiceboxEnabled:           true,
		JuiceboxProjectID:         42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.AcceptsDonation || updated.AcceptsDonationETHAddress != "vitalik.eth" || !updated.JuiceboxEnabled || updated.JuiceboxProjectID != 42 {
		t.Fatalf("unexpected Web3 site settings: %+v", updated)
	}
}
