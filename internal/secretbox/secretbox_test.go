// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package secretbox

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func box(t *testing.T, fill byte) *Box {
	t.Helper()
	b, err := New(bytes.Repeat([]byte{fill}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	b := box(t, 1)
	for _, plain := range []string{"", "hunter2", strings.Repeat("ü€", 5000)} {
		enc, err := b.Encrypt(plain)
		if err != nil {
			t.Fatal(err)
		}
		if plain != "" && strings.Contains(enc, plain) {
			t.Fatal("the plaintext shows in the ciphertext")
		}
		got, err := b.Decrypt(enc)
		if err != nil || got != plain {
			t.Fatalf("round trip of %d bytes: %v", len(plain), err)
		}
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	b := box(t, 1)
	x, _ := b.Encrypt("same")
	y, _ := b.Encrypt("same")
	if x == y {
		t.Error("two encryptions of one value are equal: the nonce isn't random")
	}
}

func TestDecryptRefusesTamperingAndWrongKeys(t *testing.T) {
	b := box(t, 1)
	enc, _ := b.Encrypt("secret")
	if _, err := box(t, 2).Decrypt(enc); err == nil {
		t.Error("another key decrypted it")
	}
	raw, _ := base64.StdEncoding.DecodeString(enc)
	raw[len(raw)-1] ^= 1
	if _, err := b.Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Error("a tampered ciphertext decrypted")
	}
	for _, bad := range []string{"not base64!", base64.StdEncoding.EncodeToString([]byte("tiny"))} {
		if _, err := b.Decrypt(bad); err == nil {
			t.Errorf("Decrypt(%q) should fail", bad)
		}
	}
}

func TestParseKey(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, KeySize))
	if k, err := ParseKey(good); err != nil || len(k) != KeySize {
		t.Fatalf("ParseKey(good) = %d bytes, %v", len(k), err)
	}
	for _, bad := range []string{"", "%%%", base64.StdEncoding.EncodeToString(make([]byte, 16))} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q) should fail", bad)
		}
	}
	if _, err := New(make([]byte, 7)); err == nil {
		t.Error("New with a 7-byte key should fail")
	}
}
