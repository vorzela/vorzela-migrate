package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vorzela/vorm/config"
)

func TestDefaultPackageIsGen(t *testing.T) {
	c := config.Default()
	if c.Package != "gen" {
		t.Fatalf("got %q", c.Package)
	}
}

func TestLoadAndSetPackage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vorm")
	body := "PACKAGE=vormgen\nDRIVER=pq\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Package != "vormgen" || c.Driver != "pq" {
		t.Fatalf("%+v", c)
	}
	if c.OutDir != "./vorm/vormgen" {
		t.Fatalf("OutDir derived from PACKAGE: %q", c.OutDir)
	}
	if err := c.Set("PACKAGE", "appqueries"); err != nil {
		t.Fatal(err)
	}
	if c.Package != "appqueries" {
		t.Fatal(c.Package)
	}
	if c.OutDir != "./vorm/appqueries" {
		t.Fatalf("OutDir sync: %q", c.OutDir)
	}
}

func TestRejectBadPackage(t *testing.T) {
	c := config.Default()
	if err := c.Set("PACKAGE", "my-gen"); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".vorm")
	c := config.Default()
	c.Package = "vormgen"
	c.OutDir = "./vorm/vormgen"
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Package != "vormgen" {
		t.Fatal(got.Package)
	}
}
