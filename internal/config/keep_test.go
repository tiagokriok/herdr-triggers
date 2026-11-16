package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKeepAndUpdatesDefaults(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(file)
	if err != nil || c.Keep() != 50 || !c.Updates() {
		t.Fatalf("defaults: keep %d updates %v err %v", c.Keep(), c.Updates(), err)
	}
	c.SetKeep(0)
	c.SetUpdates(false)
	if err := Save(file, c); err != nil {
		t.Fatal(err)
	}
	c, err = Load(file)
	if err != nil || c.Keep() != 0 || c.Updates() {
		t.Fatalf("saved: keep %d updates %v err %v", c.Keep(), c.Updates(), err)
	}
	os.WriteFile(file, []byte(`{"keepRuns":-1}`), 0o644)
	if _, err := Load(file); err == nil {
		t.Fatal("negative keepRuns accepted")
	}
}
