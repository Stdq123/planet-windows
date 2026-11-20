package applog

import "testing"

func TestBufferAppendListAndClear(t *testing.T) {
	b := New(3)
	b.Append("IPFS", "started")
	b.Append("Planet", "published")
	b.Append("SSH Rsync", "synced")
	b.Append("IPFS", "reprovided")

	all := b.List("all")
	if len(all) != 3 {
		t.Fatalf("expected ring buffer capped at 3, got %d", len(all))
	}
	ipfsOnly := b.List("IPFS")
	if len(ipfsOnly) != 1 || ipfsOnly[0].Message != "reprovided" {
		t.Fatalf("unexpected IPFS logs: %+v", ipfsOnly)
	}
	b.Clear("IPFS")
	if len(b.List("IPFS")) != 0 || len(b.List("all")) != 2 {
		t.Fatalf("unexpected logs after selective clear: %+v", b.List("all"))
	}
}
