package transfer

import "testing"

// TestConfigClient checks the config-to-client plumbing without any network:
// overrides are applied, and zero values are left for the library to default.
func TestConfigClient(t *testing.T) {
	c := Config{RendezvousURL: "ws://example/v1", TransitRelay: "1.2.3.4:5", CodeWords: 4}.client()
	if c.AppID != AppID {
		t.Errorf("AppID = %q, want %q", c.AppID, AppID)
	}
	if c.RendezvousURL != "ws://example/v1" {
		t.Errorf("RendezvousURL = %q", c.RendezvousURL)
	}
	if c.TransitRelayAddress != "1.2.3.4:5" {
		t.Errorf("TransitRelayAddress = %q", c.TransitRelayAddress)
	}
	if c.PassPhraseComponentLength != 4 {
		t.Errorf("PassPhraseComponentLength = %d, want 4", c.PassPhraseComponentLength)
	}

	// Zero config: leave everything empty so wormhole-william uses its defaults.
	d := Config{}.client()
	if d.AppID != AppID {
		t.Errorf("default AppID = %q", d.AppID)
	}
	if d.RendezvousURL != "" || d.TransitRelayAddress != "" {
		t.Errorf("expected empty server overrides, got %q / %q", d.RendezvousURL, d.TransitRelayAddress)
	}
	if d.PassPhraseComponentLength != 0 {
		t.Errorf("CodeWords < 2 should be left unset, got %d", d.PassPhraseComponentLength)
	}
}
