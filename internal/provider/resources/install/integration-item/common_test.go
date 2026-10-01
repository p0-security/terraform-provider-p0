package installintegrationitem

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mustDecode(t *testing.T, raw string) map[string]any {
	t.Helper()
	object, err := parseObject([]byte(raw))
	if err != nil {
		t.Fatalf("parseObject(%s): %s", raw, err)
	}
	return object
}

func TestReconcileConfig(t *testing.T) {
	item := `{"label":"primary","state":"installed","retries":12345678901234567890,` +
		`"service":{"type":"aws","lambda":"aws:arn","region":"us-west-2"},` +
		`"rules":[{"name":"a","id":"1"},{"name":"b","id":"2"}]}`

	cases := map[string]struct {
		prior types.String
		want  types.String
	}{
		"unset config stays unset": {
			prior: types.StringNull(),
			want:  types.StringNull(),
		},
		"formatting and key order are kept when values match": {
			prior: types.StringValue(`{ "service": { "lambda": "aws:arn", "type": "aws" } }`),
			want:  types.StringValue(`{ "service": { "lambda": "aws:arn", "type": "aws" } }`),
		},
		"large numbers compare exactly": {
			prior: types.StringValue(`{"retries":12345678901234567890}`),
			want:  types.StringValue(`{"retries":12345678901234567890}`),
		},
		"a changed nested value is reported with P0's value": {
			prior: types.StringValue(`{"service":{"lambda":"aws:other","type":"aws"}}`),
			want:  types.StringValue(`{"service":{"lambda":"aws:arn","type":"aws"}}`),
		},
		"fields P0 adds inside array elements are ignored": {
			prior: types.StringValue(`{"rules":[{"name":"a"},{"name":"b"}]}`),
			want:  types.StringValue(`{"rules":[{"name":"a"},{"name":"b"}]}`),
		},
		"an array of a different length is reported with P0's value": {
			prior: types.StringValue(`{"rules":[{"name":"a"}]}`),
			want:  types.StringValue(`{"rules":[{"id":"1","name":"a"},{"id":"2","name":"b"}]}`),
		},
		"a key configured as null matches one P0 dropped": {
			prior: types.StringValue(`{"service":{"type":"aws"},"optional":null}`),
			want:  types.StringValue(`{"service":{"type":"aws"},"optional":null}`),
		},
		"null array elements match the ones P0 dropped": {
			prior: types.StringValue(`{"rules":[null,{"name":"a"},{"name":"b"},null]}`),
			want:  types.StringValue(`{"rules":[null,{"name":"a"},{"name":"b"},null]}`),
		},
		"a key P0 does not store is dropped": {
			prior: types.StringValue(`{"service":{"type":"aws"},"typo":true}`),
			want:  types.StringValue(`{"service":{"type":"aws"}}`),
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := reconcileConfig(c.prior, mustDecode(t, item))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(c.want) {
				t.Errorf("reconcileConfig() = %s, want %s", got, c.want)
			}
		})
	}
}

func TestToMetadataMap(t *testing.T) {
	var diags diag.Diagnostics
	got := toMetadataMap(context.Background(), &diags, map[string]json.RawMessage{
		"roleName": json.RawMessage(`"P0Role"`),
		"count":    json.RawMessage(`2`),
		"absent":   json.RawMessage(`null`),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	want, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{
		"roleName": "P0Role",
		"count":    "2",
	})
	if !got.Equal(want) {
		t.Errorf("toMetadataMap() = %s, want %s", got, want)
	}

	empty := toMetadataMap(context.Background(), &diags, nil)
	if empty.IsNull() || len(empty.Elements()) != 0 {
		t.Errorf("toMetadataMap(nil) = %s, want an empty map", empty)
	}
}

func TestParseImportId(t *testing.T) {
	integration, component, id, err := parseImportId("example/iam-write/path/with/slashes")
	if err != nil {
		t.Fatal(err)
	}
	if integration != "example" || component != "iam-write" || id != "path/with/slashes" {
		t.Errorf("parseImportId() = %q, %q, %q", integration, component, id)
	}

	for _, invalid := range []string{"", "example", "example/iam-write", "example//id", "/iam-write/id"} {
		if _, _, _, err := parseImportId(invalid); err == nil {
			t.Errorf("parseImportId(%q) succeeded, want an error", invalid)
		}
	}
}

func TestDecodeObjectRejectsNonObjects(t *testing.T) {
	for _, invalid := range []string{`[]`, `"string"`, `{`, `{} {}`} {
		if _, err := parseObject([]byte(invalid)); err == nil {
			t.Errorf("parseObject(%s) succeeded, want an error", invalid)
		}
	}
}
