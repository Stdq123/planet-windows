package ipfs

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ens "github.com/ensdomains/go-ens/v4"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

func TestResolveENSUnicodeThroughUniversalResolver(t *testing.T) {
	cid := "QmYwAPJzv5CZsnAzt8auVZRnGi2C4nw9QK9qYh6sP1tWzb"
	hash, err := ens.StringToContenthash("/ipfs/" + cid)
	if err != nil {
		t.Fatal(err)
	}
	bytesType, _ := abi.NewType("bytes", "", nil)
	addrType, _ := abi.NewType("address", "", nil)
	inner, _ := (abi.Arguments{{Type: bytesType}}).Pack(hash)
	outer, _ := (abi.Arguments{{Type: bytesType}, {Type: addrType}}).Pack(inner, common.Address{})
	var gotCall bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		result := "0x1"
		if req.Method == "eth_call" {
			gotCall = true
			if !strings.Contains(strings.ToLower(string(req.Params[0])), hex.EncodeToString([]byte("🚀"))) {
				t.Error("unicode name missing from resolver call")
			}
			result = "0x" + hex.EncodeToString(outer)
		} else if req.Method != "eth_chainId" {
			t.Errorf("unexpected RPC %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	result, err := resolveENS(context.Background(), server.URL, "🚀.eth")
	if err != nil {
		t.Fatal(err)
	}
	if !gotCall || !strings.HasPrefix(result, "/ipfs/") {
		t.Fatalf("result %q call %v", result, gotCall)
	}
}

func TestResolveENSEmptyContenthashReportsMissingRecord(t *testing.T) {
	bytesType, _ := abi.NewType("bytes", "", nil)
	addrType, _ := abi.NewType("address", "", nil)
	inner, _ := (abi.Arguments{{Type: bytesType}}).Pack([]byte{})
	outer, _ := (abi.Arguments{{Type: bytesType}, {Type: addrType}}).Pack(inner, common.Address{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		result := "0x1"
		if req.Method == "eth_call" {
			result = "0x" + hex.EncodeToString(outer)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	_, err := resolveENS(context.Background(), server.URL, "🚀🚀🚀.eth")
	if err == nil || !strings.Contains(err.Error(), "没有设置内容记录") {
		t.Fatalf("err=%v", err)
	}
}
