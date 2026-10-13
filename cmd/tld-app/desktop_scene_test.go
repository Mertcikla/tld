package main

import (
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	"testing"
)

func TestDesktopSceneLinksBeforeStartup(t *testing.T) {
	bridge := NewDesktopBridge()
	link, err := sceneurl.Encode([]byte(`{"comparison":{}}`), true)
	if err != nil {
		t.Fatal(err)
	}
	bridge.openSceneArgs([]string{"--unrelated", link})
	bridge.openSceneURL("tld://unknown#v1=YQ")
	batch := bridge.DrainSceneLinks()
	if len(batch) != 2 || batch[0].Route == "" || batch[0].Error != "" || batch[1].Route != "" || batch[1].Error == "" {
		t.Fatalf("queued links=%+v", batch)
	}
	if len(bridge.DrainSceneLinks()) != 0 {
		t.Fatal("batch delivered twice")
	}
	bridge.openSceneArgs([]string{link})
	if links := bridge.DrainSceneLinks(); len(links) != 1 || links[0].Route != batch[0].Route {
		t.Fatal("subsequent launch lost")
	}
}
