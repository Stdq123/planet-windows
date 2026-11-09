package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"windows/internal/updates"
)

func TestKeygenWritesPrivateKeyOnceAndPrintsOnlyPublicKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "publisher.key")
	var output bytes.Buffer
	if err := run([]string{"keygen", "-out", path}, &output); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info, e := os.Stat(path); e != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private key permissions: %v %v", info, e)
		}
	}
	private, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(contents)))
	if err != nil || len(private) != ed25519.PrivateKeySize {
		t.Fatalf("invalid private key: %v", err)
	}
	public := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(private).Public().(ed25519.PublicKey))
	if !strings.Contains(output.String(), public) || strings.Contains(output.String(), strings.TrimSpace(string(contents))) {
		t.Fatal("keygen output should contain the public key only")
	}
	output.Reset()
	if err = run([]string{"keygen", "-out", path}, &output); err == nil {
		t.Fatal("overwrote private key")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, contents) {
		t.Fatalf("existing key changed: %v", err)
	}
}

func TestInvalidPrivateKeyNeverAppearsInErrors(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	installer := filepath.Join(dir, "setup.exe")
	out := filepath.Join(dir, "out.json")
	secret := "BAD-PRIVATE-KEY-SENTINEL"
	if e := os.WriteFile(key, []byte(secret), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(installer, []byte("installer"), 0600); e != nil {
		t.Fatal(e)
	}
	var output bytes.Buffer
	err := run([]string{"sign", "-private-key", key, "-installer", installer, "-url", "https://downloads.example.test/setup.exe", "-version", "1.2.3", "-out", out}, &output)
	if err == nil || strings.Contains(err.Error()+output.String(), secret) {
		t.Fatalf("private key error leaked content or was accepted: %v", err)
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("invalid key wrote output")
	}
}

func TestSignProducesVerifiableEnvelopeAndInstallerDigest(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	installer := filepath.Join(dir, "setup.exe")
	out := filepath.Join(dir, "release.json")
	var output bytes.Buffer
	if err := run([]string{"keygen", "-out", key}, &output); err != nil {
		t.Fatal(err)
	}
	content := []byte("test installer bytes\x00\xff")
	if err := os.WriteFile(installer, content, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"sign", "-private-key", key, "-installer", installer, "-url", "https://downloads.example.test/setup.exe", "-version", "1.2.3", "-out", out}
	output.Reset()
	before := time.Now().UTC().Add(-time.Second)
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	keyBytes, _ := os.ReadFile(key)
	private, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), strings.TrimSpace(string(keyBytes))) {
		t.Fatal("private key printed")
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Payload, Signature string }
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(ed25519.PrivateKey(private).Public().(ed25519.PublicKey), payload, signature) {
		t.Fatal("invalid payload signature")
	}
	public := base64.StdEncoding.EncodeToString(ed25519.PrivateKey(private).Public().(ed25519.PublicKey))
	if _, err = updates.Verify(data, updates.Config{FeedURL: "https://downloads.example.test/feed.json", PublicKey: public}, "0.2.0", time.Now()); err != nil {
		t.Fatalf("update client rejected signed envelope: %v", err)
	}
	var release struct {
		Schema                                              int
		App, Platform, Version, MinimumVersion, URL, SHA256 string
		Size                                                int64
		PublishedAt, ExpiresAt                              time.Time
	}
	if err = json.Unmarshal(payload, &release); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if release.Schema != 1 || release.App != "org.planetable.planetwindows" || release.Platform != "windows-amd64" || release.Version != "1.2.3" || release.MinimumVersion != "0.2.0" || release.URL != "https://downloads.example.test/setup.exe" || release.SHA256 != hex.EncodeToString(digest[:]) || release.Size != int64(len(content)) {
		t.Fatalf("unexpected release: %+v", release)
	}
	if release.PublishedAt.Before(before) || release.PublishedAt.After(time.Now().UTC().Add(time.Second)) || release.ExpiresAt.Sub(release.PublishedAt) != 7*24*time.Hour {
		t.Fatalf("incorrect validity: %+v", release)
	}
	if err = run(args, &output); err == nil {
		t.Fatal("overwrote envelope")
	}
	after, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatalf("existing envelope changed: %v", err)
	}
}

func TestSignRejectsBadInputsWithoutPrintingPrivateKey(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	installer := filepath.Join(dir, "setup.exe")
	var output bytes.Buffer
	if err := run([]string{"keygen", "-out", key}, &output); err != nil {
		t.Fatal(err)
	}
	secret, _ := os.ReadFile(key)
	if err := os.WriteFile(installer, []byte("installer"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, version, url, days string }{
		{"bad-version", "1.2", "https://downloads.example.test/setup.exe", "7"},
		{"non-https", "1.2.3", "http://downloads.example.test/setup.exe", "7"},
		{"credentials", "1.2.3", "https://user:pass@downloads.example.test/setup.exe", "7"},
		{"not-exe", "1.2.3", "https://downloads.example.test/setup.zip", "7"},
		{"prerelease-leading-zero", "1.2.3-rc.01", "https://downloads.example.test/setup.exe", "7"},
		{"build-metadata", "1.2.3+build", "https://downloads.example.test/setup.exe", "7"},
		{"overflow", "4294967296.0.0", "https://downloads.example.test/setup.exe", "7"},
		{"leading-zero", "01.2.3", "https://downloads.example.test/setup.exe", "7"},
		{"zero-days", "1.2.3", "https://downloads.example.test/setup.exe", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(dir, tc.name+".json")
			var captured bytes.Buffer
			err := run([]string{"sign", "-private-key", key, "-installer", installer, "-url", tc.url, "-version", tc.version, "-valid-days", tc.days, "-out", out}, &captured)
			if err == nil {
				t.Fatal("invalid input accepted")
			}
			if strings.Contains(captured.String()+err.Error(), strings.TrimSpace(string(secret))) {
				t.Fatal("private key leaked")
			}
			if _, err = os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("failed signing left output file")
			}
		})
	}
}

func TestSignRejectsMinimumVersionAboveRelease(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	installer := filepath.Join(dir, "installer.exe")
	out := filepath.Join(dir, "out.json")
	var captured bytes.Buffer
	if e := run([]string{"keygen", "-out", key}, &captured); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(installer, []byte("installer"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"sign", "-private-key", key, "-installer", installer, "-url", "https://downloads.example.test/setup.exe", "-version", "1.2.3", "-minimum-version", "1.2.4", "-out", out}, &captured); e == nil {
		t.Fatal("minimum above release accepted")
	}
	if _, e := os.Stat(out); !os.IsNotExist(e) {
		t.Fatal("invalid release wrote output")
	}
}

func TestSignPrereleaseEnvelopeAndMinimumVersion(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	installer := filepath.Join(dir, "installer.exe")
	var output bytes.Buffer
	if err := run([]string{"keygen", "-out", key}, &output); err != nil {
		t.Fatal(err)
	}
	public := strings.TrimSpace(output.String())
	if err := os.WriteFile(installer, []byte("test installer"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version, minimum string
		allowed          bool
	}{
		{"0.0.1-alpha.2", "0.0.1-alpha.1", true},
		{"0.0.1", "0.0.1-alpha.2", true},
		{"0.0.1-alpha.2", "0.0.1", false},
		{"0.0.1-alpha.2", "0.0.1-alpha.10", false},
	} {
		t.Run(tc.version+"_"+tc.minimum, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "feed.json")
			err := run([]string{"sign", "-private-key", key, "-installer", installer, "-url", "https://downloads.example.test/setup.exe", "-version", tc.version, "-minimum-version", tc.minimum, "-out", out}, &output)
			if (err == nil) != tc.allowed {
				t.Fatalf("sign: %v; allowed=%v", err, tc.allowed)
			}
			if !tc.allowed {
				if _, err := os.Stat(out); !os.IsNotExist(err) {
					t.Fatal("rejected version wrote a manifest")
				}
				return
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			r, err := updates.Verify(data, updates.Config{FeedURL: "https://downloads.example.test/feed.json", PublicKey: public}, tc.minimum, time.Now())
			if err != nil || r.Version != tc.version {
				t.Fatalf("client rejected publisher's prerelease: %+v, %v", r, err)
			}
		})
	}
}
