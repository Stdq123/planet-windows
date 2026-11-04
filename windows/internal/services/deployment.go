package services

import "errors"

type DeploymentRunner interface {
	Cloudflare(dir string, cfg Config) (string, error)
	SSH(dir string, cfg Config) error
}
type NetworkDeploymentRunner struct{}

func (NetworkDeploymentRunner) Cloudflare(dir string, cfg Config) (string, error) {
	return NewCloudflareClient().Deploy(dir, cfg.Server, cfg.Name, cfg.Token)
}
func (NetworkDeploymentRunner) SSH(dir string, cfg Config) error {
	return DeployRsync(dir, cfg.Name, cfg.Server, cfg.DeleteRemote)
}

// DeployConfigured persists an uncertainty marker before a remote side effect.
// Automatic retries stop until an explicit user-triggered retry resolves it.
func DeployConfigured(root, siteID, kind, dir string, force bool, runner DeploymentRunner) (string, error) {
	if kind != "ssh" && kind != "cloudflare" {
		return "", errors.New("发布目标无效")
	}
	cfg, err := LoadConfig(root, siteID, kind)
	if err != nil {
		return "", err
	}
	if cfg.Token == "" {
		return "", errors.New("请先配置发布目标")
	}
	if cfg.PendingCID != "" && !force {
		return "", errors.New("上次发布结果不确定，请检查目标后手动重试")
	}
	cfg.PendingCID = "uncertain"
	if err = SaveConfig(root, siteID, kind, cfg); err != nil {
		return "", err
	}
	var result string
	if kind == "cloudflare" {
		result, err = runner.Cloudflare(dir, cfg)
	} else {
		err = runner.SSH(dir, cfg)
	}
	if err != nil {
		return "", err
	}
	cfg.PendingCID = ""
	if result != "" {
		cfg.LastURL = result
	}
	if err = SaveConfig(root, siteID, kind, cfg); err != nil {
		return result, errors.New("远程发布已完成，但本地状态保存失败，请检查目标后重试")
	}
	return result, nil
}
