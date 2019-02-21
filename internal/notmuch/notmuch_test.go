package notmuch

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func tmpdir(t *testing.T) string {
	tmp, err := ioutil.TempDir("", "tmp")
	if err != nil {
		t.Fatalf("cannot create temp directory: %v", err)
	}
	return tmp
}

func cleanup(t *testing.T, tmp string) {
	if err := os.RemoveAll(tmp); err != nil {
		t.Error(err)
	}
}

func isDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("path is not a directory: %#v", stat)
	}
	return nil
}

func TestBasenameEncode(t *testing.T) {
	cases := []struct {
		name basename
		want string
	}{
		{
			name: basename{"scope", "permId"},
			want: "gotmuch-1-scope-permId",
		},
		{
			name: basename{"竹", "\n\t\a"},
			want: "gotmuch-1-=E7=AB=B9-=0A=09=07",
		},
	}
	for _, tc := range cases {
		if got := tc.name.encode(); got != tc.want {
			t.Errorf("%#v.encode() = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

func TestMkDirFarm(t *testing.T) {
	tmp := tmpdir(t)
	defer cleanup(t, tmp)

	farm := filepath.Join(tmp, "farm")
	if err := mkdirfarm(farm, 2); err != nil {
		t.Errorf("mkdirfarm(%#v) = %#v, want nil", farm, err)
	}

	if err := isDir(farm); err != nil {
		t.Errorf("isDir(%#v) = %v, want nil", farm, err)
	}

	// Test a smattering of the directories that should be there.
	for _, sub := range []string{"a/a", "p/p", "m/c"} {
		path := filepath.Join(farm, sub)
		if err := isDir(path); err != nil {
			t.Errorf("isDir(%#v) = %v, want nil", path, err)
		}
	}
}
