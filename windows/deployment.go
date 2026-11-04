package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"windows/internal/publisher"
	"windows/internal/services"
	"windows/internal/store"
)

type DeploymentInfo struct {
	Configured   bool   `json:"configured"`
	Name         string `json:"name"`
	Server       string `json:"server"`
	AutoDeploy   bool   `json:"autoDeploy"`
	DeleteRemote bool   `json:"deleteRemote"`
	Pending      bool   `json:"pending"`
	LastURL      string `json:"lastUrl"`
}

func (l *Library) deploymentSite(siteID string) (store.Site, []store.Article, string, error) {
	s, err := l.open()
	if err != nil {
		return store.Site{}, nil, "", err
	}
	site, articles, _, err := s.PublicationSnapshot(siteID)
	if err != nil {
		return store.Site{}, nil, "", err
	}
	if site.Imported {
		return store.Site{}, nil, "", errors.New("导入站点的外部发布暂不支持")
	}
	root, err := mygo.App.Path(mygo.PathUserData)
	return site, articles, root, err
}

func (l *Library) DeploymentInfo(siteID, kind string) (DeploymentInfo, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if kind != "ssh" && kind != "cloudflare" {
		return DeploymentInfo{}, errors.New("发布目标无效")
	}
	_, _, root, err := l.deploymentSite(siteID)
	if err != nil {
		return DeploymentInfo{}, err
	}
	cfg, err := services.LoadConfig(root, siteID, kind)
	if err != nil {
		return DeploymentInfo{}, err
	}
	return DeploymentInfo{Configured: cfg.Token != "", Name: cfg.Name, Server: cfg.Server, AutoDeploy: cfg.AutoDeploy, DeleteRemote: cfg.DeleteRemote, Pending: cfg.PendingCID != "", LastURL: cfg.LastURL}, nil
}

// ConfigureDeployment stores a target locally; credentials never enter backups.
func (l *Library) ConfigureDeployment(siteID, kind, name, server, token string, auto, deleteRemote bool) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if kind != "ssh" && kind != "cloudflare" {
		return errors.New("发布目标无效")
	}
	_, _, root, err := l.deploymentSite(siteID)
	if err != nil {
		return err
	}
	old, err := services.LoadConfig(root, siteID, kind)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	server = strings.TrimSpace(server)
	var cfg services.Config
	if kind == "cloudflare" {
		if token == "" {
			if old.Name != name || old.Server != server || old.Token == "" {
				return errors.New("更换 Cloudflare 账号或项目时请重新输入令牌")
			}
			token = old.Token
		}
		cfg = services.Config{Name: name, Server: server, Token: token, AutoDeploy: auto}
	} else {
		cfg = services.Config{Name: name, Server: server, Token: "ssh-agent", AutoDeploy: auto, DeleteRemote: deleteRemote}
	}
	if old.Name == cfg.Name && old.Server == cfg.Server {
		cfg.PendingCID = old.PendingCID
		cfg.LastURL = old.LastURL
	}
	return services.SaveConfig(root, siteID, kind, cfg)
}

func (l *Library) DisableDeployment(siteID, kind string) error {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if kind != "ssh" && kind != "cloudflare" {
		return errors.New("发布目标无效")
	}
	_, _, root, err := l.deploymentSite(siteID)
	if err != nil {
		return err
	}
	return services.SaveConfig(root, siteID, kind, services.Config{})
}

func (l *Library) SelectSSHKey(ctx context.Context) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: "选择 SSH 私钥"})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (l *Library) stageRemote(root string, site store.Site, articles []store.Article) (string, error) {
	templateDir, err := l.templatePath(site)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "planet-remote-publish-")
	if err != nil {
		return "", err
	}
	if err = publisher.StagePublicSite(dir, site, articles, templateDir, root); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// DeploySite is an explicit manual deployment of the last saved snapshot.
func (l *Library) DeploySite(siteID, kind string) (string, error) {
	l.publicMu.Lock()
	defer l.publicMu.Unlock()
	if kind != "ssh" && kind != "cloudflare" {
		return "", errors.New("发布目标无效")
	}
	site, articles, root, err := l.deploymentSite(siteID)
	if err != nil {
		return "", err
	}
	if site.Domain == "" && site.IPNS == "" {
		for _, article := range articles {
			if article.ArticleType != 1 && article.AudioFilename != "" {
				return "", errors.New("播客音频需要绝对地址；请先配置站点域名或发布 IPNS")
			}
		}
	}
	dir, err := l.stageRemote(root, site, articles)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	return services.DeployConfigured(root, siteID, kind, dir, true, services.NetworkDeploymentRunner{})
}

func (l *Library) autoDeploy(root string, site store.Site, articles []store.Article) string {
	if site.Imported {
		return ""
	}
	enabled := []string{}
	var warnings []string
	for _, kind := range []string{"ssh", "cloudflare"} {
		cfg, err := services.LoadConfig(root, site.ID, kind)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("读取 %s 配置失败：%v", kind, err))
			continue
		}
		if cfg.Token != "" && cfg.AutoDeploy {
			enabled = append(enabled, kind)
		}
	}
	if len(enabled) == 0 {
		return strings.Join(warnings, "；")
	}
	dir, err := l.stageRemote(root, site, articles)
	if err != nil {
		return strings.Join(append(warnings, "生成外部发布文件失败："+err.Error()), "；")
	}
	defer os.RemoveAll(dir)
	for _, kind := range enabled {
		if _, err = services.DeployConfigured(root, site.ID, kind, dir, false, services.NetworkDeploymentRunner{}); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s 发布未完成：%v", kind, err))
		}
	}
	return strings.Join(warnings, "；")
}
