package store

import (
	"errors"
	"regexp"
	"strings"
)

type SiteWeb3Settings struct {
	AcceptsDonation           bool   `json:"acceptsDonation"`
	AcceptsDonationMessage    string `json:"acceptsDonationMessage"`
	AcceptsDonationETHAddress string `json:"acceptsDonationETHAddress"`
	JuiceboxEnabled           bool   `json:"juiceboxEnabled"`
	JuiceboxProjectID         int    `json:"juiceboxProjectID"`
}

var ethAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func (s *Store) UpdateSiteWeb3(siteID string, cfg SiteWeb3Settings) (Site, error) {
	cfg.AcceptsDonationETHAddress = strings.TrimSpace(cfg.AcceptsDonationETHAddress)
	cfg.AcceptsDonationMessage = strings.TrimSpace(cfg.AcceptsDonationMessage)
	if cfg.AcceptsDonationETHAddress != "" && !ethAddressPattern.MatchString(cfg.AcceptsDonationETHAddress) && !strings.HasSuffix(strings.ToLower(cfg.AcceptsDonationETHAddress), ".eth") {
		return Site{}, errors.New("以太坊捐赠地址格式无效，请输入 0x 地址或 .eth 域名")
	}
	if cfg.JuiceboxProjectID < 0 {
		return Site{}, errors.New("Juicebox 项目 ID 须为非负整数")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	next.Sites = append([]Site(nil), s.data.Sites...)
	for i := range next.Sites {
		site := &next.Sites[i]
		if site.ID != siteID || site.Removed || site.DeletionPending {
			continue
		}
		if site.ContentRevision == ^uint64(0) {
			return Site{}, errors.New("站点修订版本已达到上限")
		}
		site.AcceptsDonation = cfg.AcceptsDonation
		site.AcceptsDonationMessage = cfg.AcceptsDonationMessage
		site.AcceptsDonationETHAddress = cfg.AcceptsDonationETHAddress
		site.JuiceboxEnabled = cfg.JuiceboxEnabled
		site.JuiceboxProjectID = cfg.JuiceboxProjectID
		site.ContentRevision++
		site.PublishPending = true
		if err := s.write(next); err != nil {
			return Site{}, err
		}
		return cloneSite(*site), nil
	}
	return Site{}, errors.New("站点不存在")
}
