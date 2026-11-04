// update-manifest creates an offline Ed25519 signing key and release envelope.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type release struct {
	Schema         int       `json:"schema"`
	App            string    `json:"app"`
	Platform       string    `json:"platform"`
	Version        string    `json:"version"`
	MinimumVersion string    `json:"minimumVersion"`
	URL            string    `json:"url"`
	SHA256         string    `json:"sha256"`
	Size           int64     `json:"size"`
	PublishedAt    time.Time `json:"publishedAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func parseVersion(s string) ([3]uint64, error) {
	var version [3]uint64
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return version, errors.New("invalid version")
	}
	for i, part := range parts {
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(number, 10) != part {
			return version, errors.New("invalid version")
		}
		version[i] = number
	}
	return version, nil
}

func writeNew(path string, contents []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	return writeCreated(f, path, contents)
}

func writeCreated(f *os.File, path string, contents []byte) error {
	_, err := f.Write(contents)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("expected keygen or sign")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	switch args[0] {
	case "keygen":
		out := flags.String("out", "", "private key file")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *out == "" {
			return errors.New("keygen requires -out")
		}
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return errors.New("key generation failed")
		}
		data := []byte(base64.StdEncoding.EncodeToString(private) + "\n")
		file, err := createPrivateFile(*out)
		if err != nil {
			return fmt.Errorf("cannot create private key file: %w", err)
		}
		if err = writeCreated(file, *out, data); err != nil {
			return fmt.Errorf("cannot create private key file: %w", err)
		}
		_, err = fmt.Fprintln(output, base64.StdEncoding.EncodeToString(public))
		return err
	case "sign":
		keyPath := flags.String("private-key", "", "private key file")
		installer := flags.String("installer", "", "installer file")
		downloadURL := flags.String("url", "", "installer URL")
		version := flags.String("version", "", "release version")
		out := flags.String("out", "", "manifest file")
		minimum := flags.String("minimum-version", "0.2.0", "minimum version")
		days := flags.Int("valid-days", 7, "validity in days")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *keyPath == "" || *installer == "" || *downloadURL == "" || *version == "" || *out == "" {
			return errors.New("sign requires -private-key, -installer, -url, -version and -out")
		}
		v, err := parseVersion(*version)
		if err != nil {
			return errors.New("invalid release version")
		}
		min, err := parseVersion(*minimum)
		if err != nil {
			return errors.New("invalid minimum version")
		}
		for i := range v {
			if v[i] < min[i] {
				return errors.New("minimum version exceeds release version")
			}
			if v[i] > min[i] {
				break
			}
		}
		if *days < 1 || *days > 30 {
			return errors.New("valid-days must be between 1 and 30")
		}
		u, err := url.Parse(*downloadURL)
		if err != nil || len(*downloadURL) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || !strings.HasSuffix(strings.ToLower(u.Path), ".exe") {
			return errors.New("installer URL must be HTTPS with an .exe path and without credentials or fragment")
		}
		keyFile, err := os.Open(*keyPath)
		if err != nil {
			return errors.New("cannot read private key file")
		}
		encoded, err := io.ReadAll(io.LimitReader(keyFile, 4097))
		closeErr := keyFile.Close()
		if err != nil || closeErr != nil || len(encoded) > 4096 {
			return errors.New("cannot read private key file")
		}
		private, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(private) != ed25519.PrivateKeySize || !bytes.Equal(private, ed25519.NewKeyFromSeed(private[:ed25519.SeedSize])) {
			return errors.New("invalid private key file")
		}
		f, err := os.Open(*installer)
		if err != nil {
			return errors.New("cannot read installer")
		}
		info, statErr := f.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			f.Close()
			return errors.New("installer must be a regular file")
		}
		const maxInstallerSize int64 = 512 << 20
		hash := sha256.New()
		size, err := io.Copy(hash, io.LimitReader(f, maxInstallerSize+1))
		closeErr = f.Close()
		if err != nil || closeErr != nil || size == 0 || size > maxInstallerSize {
			return errors.New("installer size or read failed")
		}
		now := time.Now().UTC().Truncate(time.Second)
		r := release{Schema: 1, App: "org.planetable.planetwindows", Platform: "windows-amd64", Version: *version, MinimumVersion: *minimum, URL: *downloadURL, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size, PublishedAt: now, ExpiresAt: now.Add(time.Duration(*days) * 24 * time.Hour)}
		payload, err := json.Marshal(r)
		if err != nil {
			return errors.New("cannot encode release")
		}
		sig := ed25519.Sign(ed25519.PrivateKey(private), payload)
		envelope := struct {
			Payload   string `json:"payload"`
			Signature string `json:"signature"`
		}{base64.StdEncoding.EncodeToString(payload), base64.StdEncoding.EncodeToString(sig)}
		data, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return errors.New("cannot encode envelope")
		}
		data = append(data, '\n')
		if err = writeNew(*out, data, 0644); err != nil {
			return fmt.Errorf("cannot create envelope file: %w", err)
		}
		return nil
	default:
		return errors.New("expected keygen or sign")
	}
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
