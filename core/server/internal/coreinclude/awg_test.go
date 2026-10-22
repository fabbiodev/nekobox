//go:build with_awg

package coreinclude_test

import (
	"context"
	"nekobox_core/internal/awg"
	"nekobox_core/internal/coreinclude"
	"testing"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
)

func TestAWG31Registry(t *testing.T) {
	ctx := coreinclude.Context(context.Background())
	config := `{"endpoints":[{"type":"awg","tag":"proxy","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","address":["10.8.1.8/32"],"header_protection_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","content_padding_addition":"10-100","random_trailers":true,"disable_cookies":true,"s1":32,"s2":32,"s3":32,"s4":32,"peers":[{"public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","persistent_keepalive_interval":"25-35"}]}]}`
	parsed, err := json.UnmarshalExtendedContext[option.Options](ctx, []byte(config))
	if err != nil {
		t.Fatal(err)
	}
	opts, ok := parsed.Endpoints[0].Options.(*awg.Options)
	if !ok {
		t.Fatalf("wrong AWG implementation: %T", parsed.Endpoints[0].Options)
	}
	if opts.ContentPaddingAddition != "10-100" || !opts.RandomTrailers || !opts.DisableCookies || opts.Peers[0].PersistentKeepaliveInterval != "25-35" {
		t.Fatal("AWG 3.1 fields lost during config decoding")
	}
}
