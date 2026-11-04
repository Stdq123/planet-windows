package ipfs

import (
	"context"
	ensip15 "github.com/adraffy/go-ens-normalize/ensip15"
	"os"
	"strings"
	"testing"
)

// Opt in with PLANET_TEST_ENS_LIVE=1; mainnet records and network availability change.
func TestLiveENSContenthash(t *testing.T) {
	if os.Getenv("PLANET_TEST_ENS_LIVE") == "" {
		t.Skip("live Ethereum RPC test is opt-in")
	}
	names := os.Getenv("PLANET_TEST_ENS_NAMES")
	if names == "" {
		names = "Ｖｉｔａｌｉｋ.eth"
	}
	for _, original := range strings.Split(names, ",") {
		name, err := ensip15.Shared().Normalize(original)
		if err != nil {
			t.Logf("%s invalid: %v", original, err)
			continue
		}
		value, err := resolveENS(context.Background(), ensRPC(), name)
		if err != nil {
			t.Logf("%s: %v", name, err)
			continue
		}
		t.Logf("input %s normalized %s => %s", original, name, value)
		return
	}
	t.Fatal("none of the supplied real ENS names has a resolvable IPFS/IPNS contenthash")
}

// Exercises a real, canonically Unicode ENS name; owners may change its record.
func TestLiveENSUnicodeRecord(t *testing.T) {
	if os.Getenv("PLANET_TEST_ENS_LIVE") == "" {
		t.Skip("live Ethereum RPC test is opt-in")
	}
	name := "🚀🚀🚀.eth"
	value, err := resolveENS(context.Background(), ensRPC(), name)
	if err != nil && !strings.Contains(err.Error(), "没有设置内容记录") {
		t.Fatalf("real Unicode ENS lookup: %v", err)
	}
	if err != nil {
		t.Logf("%s exists; contenthash is empty", name)
	} else {
		t.Logf("%s => %s", name, value)
	}
}
