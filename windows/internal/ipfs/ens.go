package ipfs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	ens "github.com/ensdomains/go-ens/v4"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// CCIP responses are untrusted: never allow them to connect to private addresses.
func publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		if !addr.IP.IsGlobalUnicast() || addr.IP.IsPrivate() || addr.IP.IsLoopback() || addr.IP.IsLinkLocalUnicast() {
			return nil, errors.New("ENS 解析服务指向私有网络")
		}
	}
	var last error
	for _, addr := range addresses {
		var d net.Dialer
		conn, e := d.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if e == nil {
			return conn, nil
		}
		last = e
	}
	if last == nil {
		last = errors.New("ENS 解析服务无法解析")
	}
	return nil, last
}
func resolveENS(ctx context.Context, endpoint, name string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("ENS RPC 地址无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := rpc.DialOptions(ctx, endpoint, rpc.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}))
	if err != nil {
		return "", err
	}
	defer client.Close()
	backend := ethclient.NewClient(client)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = publicDial
	ccipClient := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("ENS 解析重定向无效")
		}
		return nil
	}}
	resolver, err := ens.NewUniversalResolver(ctx, backend, ens.WithHTTPClient(ccipClient), ens.WithMaxHops(3))
	if err != nil {
		return "", err
	}
	hash, err := resolver.Contenthash(ctx, name)
	if err != nil {
		return "", fmt.Errorf("ENS contenthash 解析失败：%w", err)
	}
	if len(hash) == 0 {
		return "", errors.New("ENS 名称没有设置内容记录")
	}
	result, err := ens.ContenthashToString(hash)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(result, "/ipfs/") && !strings.HasPrefix(result, "/ipns/") {
		return "", errors.New("ENS 内容不是 IPFS 或 IPNS")
	}
	return result, nil
}
func ensRPC() string {
	if v := strings.TrimSpace(os.Getenv("PLANET_ENS_RPC_URL")); v != "" {
		return v
	}
	return "https://ethereum-rpc.publicnode.com"
}
