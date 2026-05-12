package tools_test

import (
	"context"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

// TestListAddressKeys_HappyPath_GopenpgpRegression locks the fork->upstream
// swap (gopenpgp/v2 v2.10.0-proton -> v2.10.0, go-crypto v1.4.1-proton ->
// v1.4.1). The tool's silent-failure design (internal/tools/keys.go) means
// any behavioural drift in crypto.NewKey / GetFingerprint /
// GetArmoredPublicKey would land as empty fingerprint and public_key_armored
// fields with no error.
//
// The dev server in testharness generates a real PGP keypair when it creates
// the harness's primary user, so a single end-to-end call exercises the full
// path: API JSON -> Key.UnmarshalJSON (NewKeyFromArmored) -> binary
// PrivateKey -> crypto.NewKey -> GetFingerprint / GetArmoredPublicKey. The
// armored output round-trips through crypto.NewKeyFromArmored to the same
// fingerprint to catch armoring-format regressions.
func TestListAddressKeys_HappyPath_GopenpgpRegression(t *testing.T) {
	h := testharness.Boot(t, "user@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	addrsOut, err := h.Call(ctx, "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("proton_list_addresses: %v", err)
	}
	addrs, ok := addrsOut["addresses"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatalf("expected at least one address, got %#v", addrsOut)
	}
	first, ok := addrs[0].(map[string]any)
	if !ok {
		t.Fatalf("address[0] not an object: %#v", addrs[0])
	}
	addrID, _ := first["id"].(string)
	if addrID == "" {
		t.Fatalf("address[0].id empty: %#v", first)
	}

	keysOut, err := h.Call(ctx, "proton_list_address_keys", map[string]any{"address_id": addrID})
	if err != nil {
		t.Fatalf("proton_list_address_keys: %v", err)
	}
	keys, ok := keysOut["keys"].([]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected at least one key, got %#v", keysOut)
	}
	k0, ok := keys[0].(map[string]any)
	if !ok {
		t.Fatalf("key[0] not an object: %#v", keys[0])
	}

	fp, _ := k0["fingerprint"].(string)
	armored, _ := k0["public_key_armored"].(string)
	if fp == "" {
		t.Fatalf("fingerprint empty - regression in crypto.NewKey/GetFingerprint")
	}
	if armored == "" {
		t.Fatalf("public_key_armored empty - regression in GetArmoredPublicKey")
	}

	pk, err := crypto.NewKeyFromArmored(armored)
	if err != nil {
		t.Fatalf("round-trip NewKeyFromArmored on tool output: %v", err)
	}
	if got := pk.GetFingerprint(); got != fp {
		t.Fatalf("fingerprint round-trip mismatch: armored=%q tool=%q", got, fp)
	}
}

// TestListAddressKeys_SilentFailureContract verifies the gopenpgp contract
// that internal/tools/keys.go's best-effort fingerprint extraction relies
// on: crypto.NewKey on non-PGP bytes must return (nil, error) or (nil, nil)
// so the inline guard
//
//	if pk, perr := crypto.NewKey(k.PrivateKey); perr == nil && pk != nil { ... }
//
// reliably skips fingerprint/armoring without panicking or short-circuiting
// the rest of the response.
//
// The contract is asserted directly rather than through the harness because
// go-proton-api's Key.UnmarshalJSON calls crypto.NewKeyFromArmored on the
// wire PrivateKey field and surfaces parse errors before the tool runs - so
// an interceptor cannot deliver garbage bytes to keys.go through normal API
// flow. This test instead locks the upstream behaviour the silent-failure
// path depends on.
func TestListAddressKeys_SilentFailureContract(t *testing.T) {
	cases := [][]byte{
		[]byte("not a pgp key"),
		nil,
		[]byte{},
		{0x00, 0x01, 0x02, 0x03},
	}
	for _, in := range cases {
		pk, err := crypto.NewKey(in)
		if err == nil && pk != nil {
			t.Fatalf("crypto.NewKey(%q): want error or nil key, got pk=%v", in, pk)
		}
	}
}
