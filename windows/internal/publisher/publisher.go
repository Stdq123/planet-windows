// Package publisher builds and publishes a snapshot using a Kubo node.
package publisher

import (
	"fmt"
	"os"

	"windows/internal/attachments"
	"windows/internal/sitegen"
	"windows/internal/store"
)

type Node interface {
	EnsureKey(string) (string, error)
	LookupKey(string) (string, error)
	Add(string) (string, error)
	PublishOnline(string, string) (string, error)
}

type Result struct {
	CID      string `json:"cid"`
	IPNS     string `json:"ipns"`
	Revision uint64 `json:"revision"`
	Pending  bool   `json:"pending"`
}

func Publish(node Node, tempParent string, site store.Site, articles []store.Article) (Result, error) {
	return PublishWithTemplate(node, tempParent, site, articles, "")
}

func PublishWithTemplate(node Node, tempParent string, site store.Site, articles []store.Article, templateDir string) (Result, error) {
	return PublishWithAssets(node, tempParent, site, articles, templateDir, "")
}

func PublishWithAssets(node Node, tempParent string, site store.Site, articles []store.Article, templateDir, attachmentRoot string) (Result, error) {
	dir, err := os.MkdirTemp(tempParent, "planet-publish-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)
	if templateDir != "" {
		err = sitegen.RenderWithTemplate(dir, site, articles, templateDir)
	} else {
		err = sitegen.Render(dir, site, articles)
	}
	if err != nil {
		return Result{}, fmt.Errorf("生成网站失败：%w", err)
	}
	if attachmentRoot != "" {
		if err := attachments.CopyToSite(attachmentRoot, dir, articles); err != nil {
			return Result{}, fmt.Errorf("复制附件失败：%w", err)
		}
	} else {
		for _, article := range articles {
			if len(article.Attachments) > 0 {
				return Result{}, fmt.Errorf("附件存储位置未设置")
			}
		}
	}
	return PublishDirectory(node, dir, site)
}

// PublishDirectory publishes an already prepared public website after verifying its identity.
func PublishDirectory(node Node, dir string, site store.Site) (Result, error) {
	var err error
	var key string
	if site.IPNS != "" {
		key, err = node.LookupKey(site.ID)
		if err == nil && key != site.IPNS {
			return Result{}, fmt.Errorf("站点密钥缺失或与原 IPNS 身份不匹配")
		}
	} else {
		key, err = node.EnsureKey(site.ID)
	}
	if err != nil {
		return Result{}, fmt.Errorf("无法读取站点密钥：%w", err)
	}
	cid, err := node.Add(dir)
	if err != nil {
		return Result{}, fmt.Errorf("添加网站到 IPFS 失败：%w", err)
	}
	name, err := node.PublishOnline(site.ID, cid)
	if err != nil {
		return Result{}, fmt.Errorf("IPNS 发布失败：%w", err)
	}
	if name != key {
		return Result{}, fmt.Errorf("发布的 IPNS 名称与站点密钥不匹配")
	}
	return Result{CID: cid, IPNS: name}, nil
}
