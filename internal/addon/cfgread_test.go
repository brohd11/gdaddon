package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigMultilineMetadata(t *testing.T) {
	for _, filename := range pluginCfgNames {
		for _, section := range []string{"plugin", "addon", "package"} {
			for _, key := range []string{"require", "deps"} {
				t.Run(filename+"/"+section+"/"+key, func(t *testing.T) {
					dir := t.TempDir()
					body := "[plugin]\nname=\"Base\"\nversion=\"0.0.1\"\n[" + section + "]\n" +
						"name=\"Example\"\nversion=\"1.2.3\"\nsource=\"u/example\"\npath=\"addons/example\"\n" +
						key + "=[\n\"u/First@v1.0.0\", ; first\n\n# second follows\n  \"u/Second@v2.0.0\",\n]\n"
					if err := os.WriteFile(filepath.Join(dir, filename), []byte(body), 0o600); err != nil {
						t.Fatal(err)
					}
					deps, err := Dependencies(dir)
					if err != nil || len(deps) != 2 || deps[0].Repo != "First" || deps[1].Tag != "v2.0.0" {
						t.Fatalf("Dependencies = %+v, %v", deps, err)
					}
					if getLocalPluginName(dir) != "Example" || getLocalPluginVersion(dir) != "1.2.3" ||
						SourceURL(dir) != "https://github.com/u/example" || installDir(dir) != "addons/example" {
						t.Fatal("multiline array hid scalar metadata")
					}
					version, err := ReadConfigVersion(dir)
					if err != nil || version.Version != "1.2.3" {
						t.Fatalf("ReadConfigVersion = %+v, %v", version, err)
					}
				})
			}
		}
	}
}

func TestParseConfigArrays(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"empty", "require=[]", "[]"},
		{"CRLF and no final newline", "require=[\r\n\"u/r\",\r\n]", "[\r\n\"u/r\",\r\n]"},
		{"quoted brackets and comments", `require=["u/r/release@v1:x[0];#.zip", 'u/s]'] ; end`, `["u/r/release@v1:x[0];#.zip", 'u/s]']`},
		{"escaped quote", `require=["a\"]#;b", "c\\", "d"]`, `["a\"]#;b", "c\\", "d"]`},
		{"nested array", "require=[\n[\"a\", \"b\"], # ] ignored\n[\"c\"],\n]", "[\n[\"a\", \"b\"], \n[\"c\"],\n]"},
		{"comment before closing bracket", "require=[\n; ]\n# ]\n\"u/r\"\n] # end", "[\n\n\n\"u/r\"\n]"},
		{"last key wins", "require=[\n\"u/old\"\n]\nrequire=[\"u/new\"]", `["u/new"]`},
		{"scalar overrides array", "require=[\n\"u/old\"\n]\nrequire=u/new", "u/new"},
		{"quoted key", "\"require\"=[\n\"u/r\"\n]", "[\n\"u/r\"\n]"},
		{"token collision", "other=__gdaddon_array_0__\nrequire=[\"u/r\"]", `["u/r"]`},
		{"after multiline scalar", "description=\"\"\"text\nrequire=[fake\n\"\"\"\nrequire=[\n\"u/r\"\n]", "[\n\"u/r\"\n]"},
		{"after backtick scalar", "description=`text\nrequire=[fake\n`\nrequire=[\"u/r\"]", `["u/r"]`},
		{"after continuation", "description=text\\\nrequire=[fake\nrequire=[\"u/r\"]", `["u/r"]`},
		{"quoted scalar ignores continuation", "description=`text` ; \\\nrequire=[\n\"u/r\"\n]", "[\n\"u/r\"\n]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte("[plugin]\n" + tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if got := cfgSection(cfg).Key("require").Value(); got != tc.want {
				t.Fatalf("require = %q, want %q", got, tc.want)
			}
			if tc.name == "token collision" && cfgSection(cfg).Key("other").Value() != "__gdaddon_array_0__" {
				t.Fatal("scalar token collision")
			}
		})
	}
}

func TestParseConfigArrayErrors(t *testing.T) {
	for _, value := range []string{"[", "[\n\"u/r\"", "[\"u/r]", "[[\"u/r\"]", "[\"u/r\"] garbage", "[ # ]"} {
		_, err := parseConfig([]byte("[plugin]\nname=Demo\nrequire=" + value))
		if err == nil || !strings.Contains(err.Error(), "key require at line 3") {
			t.Fatalf("array %q: expected key and line in error, got %v", value, err)
		}
	}
}

func TestMultilineRequirePrecedence(t *testing.T) {
	for _, require := range []string{"[\n]", "[\n\"u/new\",\n]"} {
		dir := t.TempDir()
		body := "[plugin]\ndeps=[\n\"u/old\"\n]\n[addon]\nrequire=" + require
		if err := os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		deps, err := Dependencies(dir)
		if err != nil {
			t.Fatal(err)
		}
		if require == "[\n]" && len(deps) != 0 || require != "[\n]" && (len(deps) != 1 || deps[0].Repo != "new") {
			t.Fatalf("unexpected dependencies: %+v", deps)
		}
	}
}
